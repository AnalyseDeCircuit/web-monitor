// Package docker 提供Docker容器管理功能
package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/AnalyseDeCircuit/opskernel/pkg/types"
)

var (
	dockerClient        *http.Client
	dockerOnce          sync.Once
	dockerReadOnlyMode  bool
	dockerSockPath      string
	dockerBaseURL       string
	dockerAPIPrefix     string
	dockerAPIDetectMu   sync.Mutex
	dockerAPIDetectDone bool
)

const (
	dockerDialTimeout    = 5 * time.Second
	dockerRequestTimeout = 15 * time.Second
)

func resolveDockerSocketPath() (string, error) {
	if v := strings.TrimSpace(os.Getenv("DOCKER_SOCK")); v != "" {
		return v, nil
	}
	if v := strings.TrimSpace(os.Getenv("DOCKER_HOST")); v != "" {
		// Common format: unix:///var/run/docker.sock
		if strings.HasPrefix(v, "unix://") {
			p := strings.TrimPrefix(v, "unix://")
			if p == "" {
				return "", fmt.Errorf("invalid DOCKER_HOST: %q", v)
			}
			return p, nil
		}
		return "", fmt.Errorf("unsupported DOCKER_HOST (only unix:// supported): %q", v)
	}
	return "/var/run/docker.sock", nil
}

func resolveDockerBaseURL() (string, bool, error) {
	// Returns (baseURL, isUnix, error)
	if v := strings.TrimSpace(os.Getenv("DOCKER_HOST")); v != "" {
		// Supported:
		// - unix:///var/run/docker.sock
		// - tcp://127.0.0.1:2375 (for docker-socket-proxy)
		// - http://127.0.0.1:2375
		// - https://...
		if strings.HasPrefix(v, "unix://") {
			return "http://docker", true, nil
		}
		if strings.HasPrefix(v, "tcp://") {
			return "http://" + strings.TrimPrefix(v, "tcp://"), false, nil
		}
		if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
			return v, false, nil
		}
		return "", false, fmt.Errorf("unsupported DOCKER_HOST: %q", v)
	}

	// Default: unix socket mode.
	return "http://docker", true, nil
}

func resolveDockerAPIPrefix() string {
	// Accept DOCKER_API_VERSION like "1.41" or "v1.41"; default empty (no prefix).
	v := strings.TrimSpace(os.Getenv("DOCKER_API_VERSION"))
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return "/" + v
}

func detectDockerAPIPrefix(ctx context.Context) {
	dockerAPIDetectMu.Lock()
	if dockerAPIDetectDone {
		dockerAPIDetectMu.Unlock()
		return
	}
	// Mark done no matter what to avoid repeated probes under load.
	dockerAPIDetectDone = true
	dockerAPIDetectMu.Unlock()

	if dockerAPIPrefix != "" {
		return
	}

	client := getDockerClient()
	base := dockerBaseURL
	if base == "" {
		base = "http://docker"
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, dockerRequestTimeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/version", nil)
	if err != nil {
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}

	var v struct {
		APIVersion string `json:"ApiVersion"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return
	}
	api := strings.TrimSpace(v.APIVersion)
	if api == "" {
		return
	}
	if !strings.HasPrefix(api, "v") {
		api = "v" + api
	}
	dockerAPIPrefix = "/" + api
}

// getDockerClient 获取Docker客户端
func getDockerClient() *http.Client {
	dockerOnce.Do(func() {
		baseURL, isUnix, err := resolveDockerBaseURL()
		if err != nil {
			log.Printf("Docker host config error: %v", err)
			// Keep a sane default.
			baseURL = "http://docker"
			isUnix = true
		}
		dockerBaseURL = baseURL

		if isUnix {
			dockerSockPath, err = resolveDockerSocketPath()
			if err != nil {
				log.Printf("Docker socket config error: %v", err)
				// Keep a sane default so endpoints can still attempt to work.
				dockerSockPath = "/var/run/docker.sock"
			}
			log.Printf("Docker mode: unix socket (%s)", dockerSockPath)
		} else {
			log.Printf("Docker mode: tcp/http (%s)", dockerBaseURL)
			log.Println("WARNING: Remote Docker API access is high risk; prefer docker-socket-proxy bound to 127.0.0.1")
		}

		// Check if read-only mode is enforced via env var.
		if os.Getenv("DOCKER_READ_ONLY") == "true" {
			dockerReadOnlyMode = true
			log.Println("Docker read-only mode enabled: write operations (start/stop/remove) will be blocked")
		}
		// Security warning: Direct access to docker.sock grants root-level host access.
		// For production, consider:
		// - Running with minimal permissions (docker group, not root)
		// - Using a sidecar container with limited Docker API access
		// - Mounting /var/run/docker.sock read-only when possible
		log.Println("WARNING: Direct docker.sock access detected. Ensure secure deployment configuration.")

		dockerAPIPrefix = resolveDockerAPIPrefix()
		tr := &http.Transport{
			ResponseHeaderTimeout: dockerRequestTimeout,
			MaxIdleConns:          20,
			IdleConnTimeout:       30 * time.Second,
		}
		if isUnix {
			dialer := &net.Dialer{Timeout: dockerDialTimeout}
			tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, "unix", dockerSockPath)
			}
		}
		dockerClient = &http.Client{Timeout: dockerRequestTimeout, Transport: tr}
	})
	return dockerClient
}

// dockerRequest 发送Docker API请求
func dockerRequest(ctx context.Context, method, path string) (*http.Response, error) {
	client := getDockerClient()
	if ctx == nil {
		ctx = context.Background()
	}
	var cancel context.CancelFunc
	if _, ok := ctx.Deadline(); !ok {
		ctx, cancel = context.WithTimeout(ctx, dockerRequestTimeout)
	}
	base := dockerBaseURL
	if base == "" {
		base = "http://docker"
	}

	paths := []string{path}
	// If API version prefix is configured and path is not already versioned, try versioned first.
	if dockerAPIPrefix != "" && !strings.HasPrefix(path, "/v") {
		paths = append([]string{dockerAPIPrefix + path}, paths...)
	}

	var resp *http.Response
	var err error
	for i, p := range paths {
		req, reqErr := http.NewRequestWithContext(ctx, method, base+p, nil)
		if reqErr != nil {
			err = reqErr
			break
		}
		resp, err = client.Do(req)
		if err != nil {
			break
		}
		// If not 404 or this is last attempt, return.
		if resp.StatusCode != http.StatusNotFound || i == len(paths)-1 {
			break
		}
		// Close body before retrying with fallback.
		resp.Body.Close()
	}

	// If we got a 404 on an unversioned request, try to auto-detect ApiVersion and retry once.
	if err == nil && resp != nil && resp.StatusCode == http.StatusNotFound && dockerAPIPrefix == "" && !strings.HasPrefix(path, "/v") {
		_ = resp.Body.Close()
		detectDockerAPIPrefix(ctx)
		if dockerAPIPrefix != "" {
			req, reqErr := http.NewRequestWithContext(ctx, method, base+dockerAPIPrefix+path, nil)
			if reqErr != nil {
				err = reqErr
			} else {
				resp, err = client.Do(req)
			}
		}
	}

	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	if cancel != nil && resp != nil && resp.Body != nil {
		resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
	}
	return resp, nil
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	if c.cancel != nil {
		c.cancel()
	}
	return err
}

// ListContainers 列出所有容器
func ListContainers() ([]types.DockerContainer, error) {
	resp, err := dockerRequest(context.Background(), "GET", "/containers/json?all=1")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Docker API error: %d", resp.StatusCode)
	}

	var containers []types.DockerContainer
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, fmt.Errorf("failed to decode Docker response: %v", err)
	}

	return containers, nil
}

// ListImages 列出所有镜像
func ListImages() ([]types.DockerImage, error) {
	resp, err := dockerRequest(context.Background(), "GET", "/images/json")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Docker API error: %d", resp.StatusCode)
	}

	var images []types.DockerImage
	if err := json.NewDecoder(resp.Body).Decode(&images); err != nil {
		return nil, fmt.Errorf("failed to decode Docker response: %v", err)
	}

	return images, nil
}

// ContainerAction 执行容器操作
func ContainerAction(containerID, action string) error {
	// Block write operations in read-only mode.
	if dockerReadOnlyMode {
		return fmt.Errorf("Docker read-only mode is enabled; action '%s' is not allowed", action)
	}
	if containerID == "" {
		return fmt.Errorf("container id is required")
	}
	escapedID := url.PathEscape(containerID)
	var path string
	method := "POST"
	switch action {
	case "start":
		path = fmt.Sprintf("/containers/%s/start", escapedID)
	case "stop":
		path = fmt.Sprintf("/containers/%s/stop", escapedID)
	case "restart":
		path = fmt.Sprintf("/containers/%s/restart", escapedID)
	case "remove":
		path = fmt.Sprintf("/containers/%s", escapedID)
		method = "DELETE"
	default:
		return fmt.Errorf("invalid action: %s", action)
	}

	resp, err := dockerRequest(context.Background(), method, path)
	if err != nil {
		return fmt.Errorf("failed to connect to Docker: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Docker error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// RemoveImage 删除镜像（默认非强制）。
// imageRef 可以是镜像 ID（如 sha256:...）或引用（如 repo:tag）。
func RemoveImage(imageRef string, force bool, noprune bool) error {
	// Block write operations in read-only mode.
	if dockerReadOnlyMode {
		return fmt.Errorf("Docker read-only mode is enabled; image removal is not allowed")
	}
	if imageRef == "" {
		return fmt.Errorf("image reference is required")
	}

	forceVal := "0"
	if force {
		forceVal = "1"
	}
	nopruneVal := "0"
	if noprune {
		nopruneVal = "1"
	}

	path := fmt.Sprintf("/images/%s?force=%s&noprune=%s", url.PathEscape(imageRef), forceVal, nopruneVal)
	resp, err := dockerRequest(context.Background(), "DELETE", path)
	if err != nil {
		return fmt.Errorf("failed to connect to Docker: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Docker error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetContainerStats 获取容器统计信息
func GetContainerStats(containerID string) (map[string]interface{}, error) {
	if containerID == "" {
		return nil, fmt.Errorf("container id is required")
	}
	escapedID := url.PathEscape(containerID)
	resp, err := dockerRequest(context.Background(), "GET", fmt.Sprintf("/containers/%s/stats?stream=false", escapedID))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Docker API error: %d", resp.StatusCode)
	}

	var stats map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("failed to decode Docker stats: %v", err)
	}

	return stats, nil
}

// GetContainerLogs 获取容器日志
func GetContainerLogs(containerID string, lines int) (string, error) {
	if containerID == "" {
		return "", fmt.Errorf("container id is required")
	}
	if lines <= 0 {
		lines = 100
	}
	if lines > 5000 {
		lines = 5000
	}
	escapedID := url.PathEscape(containerID)
	path := fmt.Sprintf("/containers/%s/logs?stdout=true&stderr=true&tail=%d", escapedID, lines)
	resp, err := dockerRequest(context.Background(), "GET", path)
	if err != nil {
		return "", fmt.Errorf("failed to connect to Docker: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Docker API error: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read logs: %v", err)
	}

	return string(body), nil
}

// PruneSystem 清理Docker系统 (containers, images, networks, build cache)
func PruneSystem() (map[string]interface{}, error) {
	// Block write operations in read-only mode.
	if dockerReadOnlyMode {
		return nil, fmt.Errorf("Docker read-only mode is enabled; prune is not allowed")
	}

	result := make(map[string]interface{})
	var totalSpaceReclaimed uint64

	// Prune stopped containers
	if resp, err := dockerRequest(context.Background(), "POST", "/containers/prune"); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var containerResult map[string]interface{}
			if json.NewDecoder(resp.Body).Decode(&containerResult) == nil {
				if deleted, ok := containerResult["ContainersDeleted"].([]interface{}); ok {
					result["ContainersDeleted"] = len(deleted)
				}
				if space, ok := containerResult["SpaceReclaimed"].(float64); ok {
					totalSpaceReclaimed += uint64(space)
				}
			}
		}
	}

	// Prune dangling images
	if resp, err := dockerRequest(context.Background(), "POST", "/images/prune"); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var imageResult map[string]interface{}
			if json.NewDecoder(resp.Body).Decode(&imageResult) == nil {
				if deleted, ok := imageResult["ImagesDeleted"].([]interface{}); ok {
					result["ImagesDeleted"] = len(deleted)
				}
				if space, ok := imageResult["SpaceReclaimed"].(float64); ok {
					totalSpaceReclaimed += uint64(space)
				}
			}
		}
	}

	// Prune unused networks
	if resp, err := dockerRequest(context.Background(), "POST", "/networks/prune"); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var networkResult map[string]interface{}
			if json.NewDecoder(resp.Body).Decode(&networkResult) == nil {
				if deleted, ok := networkResult["NetworksDeleted"].([]interface{}); ok {
					result["NetworksDeleted"] = len(deleted)
				}
			}
		}
	}

	// Prune build cache
	if resp, err := dockerRequest(context.Background(), "POST", "/build/prune"); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var buildResult map[string]interface{}
			if json.NewDecoder(resp.Body).Decode(&buildResult) == nil {
				if space, ok := buildResult["SpaceReclaimed"].(float64); ok {
					totalSpaceReclaimed += uint64(space)
				}
			}
		}
	}

	result["SpaceReclaimed"] = totalSpaceReclaimed
	return result, nil
}
