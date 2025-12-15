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
	"sync"

	"github.com/AnalyseDeCircuit/web-monitor/pkg/types"
)

var (
	dockerClient       *http.Client
	dockerOnce         sync.Once
	dockerReadOnlyMode bool
)

// getDockerClient 获取Docker客户端
func getDockerClient() *http.Client {
	dockerOnce.Do(func() {
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
		dockerClient = &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", "/var/run/docker.sock")
				},
			},
		}
	})
	return dockerClient
}

// dockerRequest 发送Docker API请求
func dockerRequest(method, path string) (*http.Response, error) {
	client := getDockerClient()
	req, err := http.NewRequest(method, "http://docker"+path, nil)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

// ListContainers 列出所有容器
func ListContainers() ([]types.DockerContainer, error) {
	resp, err := dockerRequest("GET", "/containers/json?all=1")
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
	resp, err := dockerRequest("GET", "/images/json")
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
	var path string
	method := "POST"
	switch action {
	case "start":
		path = fmt.Sprintf("/containers/%s/start", containerID)
	case "stop":
		path = fmt.Sprintf("/containers/%s/stop", containerID)
	case "restart":
		path = fmt.Sprintf("/containers/%s/restart", containerID)
	case "remove":
		path = fmt.Sprintf("/containers/%s", containerID)
		method = "DELETE"
	default:
		return fmt.Errorf("invalid action: %s", action)
	}

	resp, err := dockerRequest(method, path)
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
	resp, err := dockerRequest("DELETE", path)
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
	resp, err := dockerRequest("GET", fmt.Sprintf("/containers/%s/stats?stream=false", containerID))
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
	path := fmt.Sprintf("/containers/%s/logs?stdout=true&stderr=true&tail=%d", containerID, lines)
	resp, err := dockerRequest("GET", path)
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

// PruneSystem 清理Docker系统
func PruneSystem() (map[string]interface{}, error) {
	resp, err := dockerRequest("POST", "/system/prune")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Docker API error: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode prune result: %v", err)
	}

	return result, nil
}
