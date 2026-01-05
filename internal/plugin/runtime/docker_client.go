// Package runtime provides a Docker client implementation for the plugin runtime.
package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// Docker Error Types - Machine-readable error classification
// ============================================================================

// DockerErrorKind represents the category of Docker error
type DockerErrorKind string

const (
	ErrKindNotFound    DockerErrorKind = "not_found"   // 404: container/image doesn't exist
	ErrKindConflict    DockerErrorKind = "conflict"    // 409: name conflict, already exists
	ErrKindTemporary   DockerErrorKind = "temporary"   // 5xx: server error, retryable
	ErrKindTimeout     DockerErrorKind = "timeout"     // Request timeout
	ErrKindUnreachable DockerErrorKind = "unreachable" // Cannot connect to Docker
	ErrKindReadOnly    DockerErrorKind = "read_only"   // Docker is in read-only mode
	ErrKindBadRequest  DockerErrorKind = "bad_request" // 400: invalid parameters
	ErrKindForbidden   DockerErrorKind = "forbidden"   // 403: permission denied
	ErrKindUnknown     DockerErrorKind = "unknown"     // Other errors
)

// DockerError represents a typed Docker API error
type DockerError struct {
	Kind       DockerErrorKind
	StatusCode int
	Message    string
	Operation  string // e.g., "create", "start", "stop"
	Target     string // container name/ID or image name
	Retryable  bool
	Cause      error
}

func (e *DockerError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("docker %s %s: %s (cause: %v)", e.Operation, e.Target, e.Message, e.Cause)
	}
	return fmt.Sprintf("docker %s %s: %s", e.Operation, e.Target, e.Message)
}

func (e *DockerError) Unwrap() error {
	return e.Cause
}

// newDockerError creates a typed Docker error from HTTP response
func newDockerError(operation, target string, statusCode int, body string) *DockerError {
	err := &DockerError{
		Operation:  operation,
		Target:     target,
		StatusCode: statusCode,
		Message:    body,
	}

	switch {
	case statusCode == 404:
		err.Kind = ErrKindNotFound
		err.Retryable = false
	case statusCode == 409:
		err.Kind = ErrKindConflict
		err.Retryable = false // Conflict needs explicit resolution
	case statusCode == 400:
		err.Kind = ErrKindBadRequest
		err.Retryable = false
	case statusCode == 403:
		err.Kind = ErrKindForbidden
		err.Retryable = false
	case statusCode >= 500:
		err.Kind = ErrKindTemporary
		err.Retryable = true
	default:
		err.Kind = ErrKindUnknown
		err.Retryable = false
	}

	return err
}

// newConnectionError creates an error for connection failures
func newConnectionError(operation, target string, cause error) *DockerError {
	kind := ErrKindUnreachable
	retryable := true

	// Check for timeout
	if netErr, ok := cause.(net.Error); ok && netErr.Timeout() {
		kind = ErrKindTimeout
	}

	return &DockerError{
		Kind:      kind,
		Operation: operation,
		Target:    target,
		Message:   "connection failed",
		Retryable: retryable,
		Cause:     cause,
	}
}

// IsNotFoundError checks if error is a 404 not found
func IsNotFoundError(err error) bool {
	if de, ok := err.(*DockerError); ok {
		return de.Kind == ErrKindNotFound
	}
	// Fallback: check error message for backward compatibility
	return strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404")
}

// IsConflictError checks if error is a 409 conflict
func IsConflictError(err error) bool {
	if de, ok := err.(*DockerError); ok {
		return de.Kind == ErrKindConflict
	}
	return strings.Contains(err.Error(), "409") || strings.Contains(err.Error(), "Conflict")
}

// IsTemporaryError checks if error is retryable
func IsTemporaryError(err error) bool {
	if de, ok := err.(*DockerError); ok {
		return de.Retryable
	}
	return false
}

// IsTimeoutError checks if error is a timeout
func IsTimeoutError(err error) bool {
	if de, ok := err.(*DockerError); ok {
		return de.Kind == ErrKindTimeout
	}
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	return false
}

// ============================================================================
// Retry Configuration
// ============================================================================

const (
	// defaultMaxRetries is the default number of retries for transient errors
	defaultMaxRetries = 3
	// defaultRetryDelay is the base delay between retries
	defaultRetryDelay = 500 * time.Millisecond
	// maxRetryDelay is the maximum delay between retries (with exponential backoff)
	maxRetryDelay = 5 * time.Second
)

// retryableOp executes an operation with retry for transient errors
// Uses exponential backoff: delay * 2^attempt (capped at maxRetryDelay)
func retryableOp(ctx context.Context, operation string, fn func() error) error {
	var lastErr error
	delay := defaultRetryDelay

	for attempt := 0; attempt <= defaultMaxRetries; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		// Check if error is retryable
		if !IsTemporaryError(lastErr) {
			return lastErr
		}

		// Don't retry if context is done
		if ctx.Err() != nil {
			return lastErr
		}

		// Log retry attempt
		if attempt < defaultMaxRetries {
			fmt.Printf("Docker %s: transient error, retrying in %v (attempt %d/%d): %v\n",
				operation, delay, attempt+1, defaultMaxRetries, lastErr)

			select {
			case <-ctx.Done():
				return lastErr
			case <-time.After(delay):
			}

			// Exponential backoff
			delay *= 2
			if delay > maxRetryDelay {
				delay = maxRetryDelay
			}
		}
	}

	return lastErr
}

// ============================================================================
// Docker Client Implementation
// ============================================================================

// DefaultDockerClient implements DockerClient using the Docker Engine API
type DefaultDockerClient struct {
	client    *http.Client
	baseURL   string
	apiPrefix string
	readOnly  bool
	mu        sync.Mutex
	initDone  bool
}

// NewDefaultDockerClient creates a new Docker client
func NewDefaultDockerClient() *DefaultDockerClient {
	return &DefaultDockerClient{}
}

func (d *DefaultDockerClient) init() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.initDone {
		return
	}
	d.initDone = true

	// Determine connection mode
	dockerHost := os.Getenv("DOCKER_HOST")
	d.readOnly = os.Getenv("DOCKER_READ_ONLY") == "true"

	tr := &http.Transport{
		ResponseHeaderTimeout: 15 * time.Second,
		MaxIdleConns:          20,
		IdleConnTimeout:       30 * time.Second,
	}

	if dockerHost == "" || strings.HasPrefix(dockerHost, "unix://") {
		// Unix socket mode
		sockPath := "/var/run/docker.sock"
		if strings.HasPrefix(dockerHost, "unix://") {
			sockPath = strings.TrimPrefix(dockerHost, "unix://")
		}

		tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			dialer := &net.Dialer{Timeout: 5 * time.Second}
			return dialer.DialContext(ctx, "unix", sockPath)
		}
		d.baseURL = "http://docker"
	} else if strings.HasPrefix(dockerHost, "tcp://") {
		d.baseURL = "http://" + strings.TrimPrefix(dockerHost, "tcp://")
	} else if strings.HasPrefix(dockerHost, "http://") || strings.HasPrefix(dockerHost, "https://") {
		d.baseURL = dockerHost
	} else {
		d.baseURL = "http://docker"
	}

	d.client = &http.Client{
		Timeout:   15 * time.Second,
		Transport: tr,
	}

	// Detect API version
	if apiVer := os.Getenv("DOCKER_API_VERSION"); apiVer != "" {
		if !strings.HasPrefix(apiVer, "v") {
			apiVer = "v" + apiVer
		}
		d.apiPrefix = "/" + apiVer
	}
}

func (d *DefaultDockerClient) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	d.init()

	fullPath := d.baseURL + d.apiPrefix + path
	req, err := http.NewRequestWithContext(ctx, method, fullPath, body)
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return d.client.Do(req)
}

// CreateContainer creates a new container
// Returns typed DockerError for conflict (409) and other errors
func (d *DefaultDockerClient) CreateContainer(ctx context.Context, config *ContainerCreateConfig) (string, error) {
	d.init()

	if d.readOnly {
		return "", &DockerError{
			Kind:      ErrKindReadOnly,
			Operation: "create",
			Target:    config.Name,
			Message:   "docker is in read-only mode",
			Retryable: false,
		}
	}

	// Build Docker API create request
	createReq := buildDockerCreateRequest(config)

	body, err := json.Marshal(createReq)
	if err != nil {
		return "", err
	}

	path := fmt.Sprintf("/containers/create?name=%s", url.QueryEscape(config.Name))
	resp, err := d.doRequest(ctx, "POST", path, bytes.NewReader(body))
	if err != nil {
		return "", newConnectionError("create", config.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", newDockerError("create", config.Name, resp.StatusCode, string(respBody))
	}

	var result struct {
		ID       string   `json:"Id"`
		Warnings []string `json:"Warnings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.ID, nil
}

// StartContainer starts a container with retry for transient errors
// 304 (already started) is treated as success for idempotency
func (d *DefaultDockerClient) StartContainer(ctx context.Context, containerID string) error {
	d.init()

	if d.readOnly {
		return &DockerError{
			Kind:      ErrKindReadOnly,
			Operation: "start",
			Target:    containerID,
			Message:   "docker is in read-only mode",
			Retryable: false,
		}
	}

	return retryableOp(ctx, "start "+containerID, func() error {
		path := fmt.Sprintf("/containers/%s/start", url.PathEscape(containerID))
		resp, err := d.doRequest(ctx, "POST", path, nil)
		if err != nil {
			return newConnectionError("start", containerID, err)
		}
		defer resp.Body.Close()

		// 304 = already started (idempotent success)
		if resp.StatusCode == 304 {
			return nil
		}
		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			return newDockerError("start", containerID, resp.StatusCode, string(body))
		}

		return nil
	})
}

// StopContainer stops a container
// 304 (already stopped) and 404 (doesn't exist) are treated as success for idempotency
func (d *DefaultDockerClient) StopContainer(ctx context.Context, containerID string, timeout int) error {
	d.init()

	if d.readOnly {
		return &DockerError{
			Kind:      ErrKindReadOnly,
			Operation: "stop",
			Target:    containerID,
			Message:   "docker is in read-only mode",
			Retryable: false,
		}
	}

	path := fmt.Sprintf("/containers/%s/stop?t=%d", url.PathEscape(containerID), timeout)
	resp, err := d.doRequest(ctx, "POST", path, nil)
	if err != nil {
		return newConnectionError("stop", containerID, err)
	}
	defer resp.Body.Close()

	// 304 = already stopped, 404 = doesn't exist (both are idempotent success)
	if resp.StatusCode == 304 || resp.StatusCode == 404 {
		return nil
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return newDockerError("stop", containerID, resp.StatusCode, string(body))
	}

	return nil
}

// RemoveContainer removes a container
// 404 (doesn't exist) is treated as success for idempotency
func (d *DefaultDockerClient) RemoveContainer(ctx context.Context, containerID string, force bool) error {
	d.init()

	if d.readOnly {
		return &DockerError{
			Kind:      ErrKindReadOnly,
			Operation: "remove",
			Target:    containerID,
			Message:   "docker is in read-only mode",
			Retryable: false,
		}
	}

	forceParam := "0"
	if force {
		forceParam = "1"
	}

	path := fmt.Sprintf("/containers/%s?force=%s&v=1", url.PathEscape(containerID), forceParam)
	resp, err := d.doRequest(ctx, "DELETE", path, nil)
	if err != nil {
		return newConnectionError("remove", containerID, err)
	}
	defer resp.Body.Close()

	// 404 = already removed (idempotent success)
	if resp.StatusCode == 404 {
		return nil
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return newDockerError("remove", containerID, resp.StatusCode, string(body))
	}

	return nil
}

// InspectContainer gets container details
func (d *DefaultDockerClient) InspectContainer(ctx context.Context, containerID string) (*ContainerInfo, error) {
	d.init()

	path := fmt.Sprintf("/containers/%s/json", url.PathEscape(containerID))
	resp, err := d.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, newConnectionError("inspect", containerID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, newDockerError("inspect", containerID, resp.StatusCode, string(body))
	}

	var raw struct {
		ID    string `json:"Id"`
		Name  string `json:"Name"`
		State struct {
			Status     string    `json:"Status"`
			Running    bool      `json:"Running"`
			StartedAt  time.Time `json:"StartedAt"`
			FinishedAt time.Time `json:"FinishedAt"`
			ExitCode   int       `json:"ExitCode"`
			Error      string    `json:"Error"`
		} `json:"State"`
		NetworkSettings struct {
			Ports map[string][]struct {
				HostIP   string `json:"HostIp"`
				HostPort string `json:"HostPort"`
			} `json:"Ports"`
		} `json:"NetworkSettings"`
		HostConfig struct {
			NetworkMode string `json:"NetworkMode"`
		} `json:"HostConfig"`
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	state := StateUnknown
	switch strings.ToLower(raw.State.Status) {
	case "running":
		state = StateRunning
	case "created", "paused", "restarting":
		state = StateCreated
	case "exited", "dead", "removing":
		state = StateStopped
	}

	info := &ContainerInfo{
		ID:          raw.ID,
		Name:        strings.TrimPrefix(raw.Name, "/"),
		State:       state,
		Running:     raw.State.Running,
		StartedAt:   raw.State.StartedAt,
		FinishedAt:  raw.State.FinishedAt,
		ExitCode:    raw.State.ExitCode,
		Error:       raw.State.Error,
		Labels:      raw.Config.Labels,
		NetworkMode: raw.HostConfig.NetworkMode,
		Ports:       make(map[int]int),
	}

	// Parse port mappings
	for portProto, bindings := range raw.NetworkSettings.Ports {
		parts := strings.Split(portProto, "/")
		if len(parts) == 0 {
			continue
		}
		var containerPort int
		fmt.Sscanf(parts[0], "%d", &containerPort)

		for _, b := range bindings {
			var hostPort int
			fmt.Sscanf(b.HostPort, "%d", &hostPort)
			if hostPort > 0 {
				info.Ports[containerPort] = hostPort
				break
			}
		}
	}

	return info, nil
}

// ListContainers lists all containers
func (d *DefaultDockerClient) ListContainers(ctx context.Context, all bool) ([]ContainerListItem, error) {
	d.init()

	path := "/containers/json"
	if all {
		path += "?all=1"
	}

	resp, err := d.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("docker list error %d: %s", resp.StatusCode, string(body))
	}

	var raw []struct {
		ID     string            `json:"Id"`
		Names  []string          `json:"Names"`
		Image  string            `json:"Image"`
		State  string            `json:"State"`
		Status string            `json:"Status"`
		Labels map[string]string `json:"Labels"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	items := make([]ContainerListItem, len(raw))
	for i, c := range raw {
		items[i] = ContainerListItem{
			ID:     c.ID,
			Names:  c.Names,
			Image:  c.Image,
			State:  c.State,
			Status: c.Status,
			Labels: c.Labels,
		}
	}

	return items, nil
}

// ConnectNetwork connects a container to a network
func (d *DefaultDockerClient) ConnectNetwork(ctx context.Context, networkID, containerID string) error {
	d.init()

	if d.readOnly {
		return fmt.Errorf("docker is in read-only mode")
	}

	body := fmt.Sprintf(`{"Container":"%s"}`, containerID)
	path := fmt.Sprintf("/networks/%s/connect", url.PathEscape(networkID))
	resp, err := d.doRequest(ctx, "POST", path, strings.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("docker network connect error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// DisconnectNetwork disconnects a container from a network
func (d *DefaultDockerClient) DisconnectNetwork(ctx context.Context, networkID, containerID string, force bool) error {
	d.init()

	if d.readOnly {
		return fmt.Errorf("docker is in read-only mode")
	}

	body := fmt.Sprintf(`{"Container":"%s","Force":%t}`, containerID, force)
	path := fmt.Sprintf("/networks/%s/disconnect", url.PathEscape(networkID))
	resp, err := d.doRequest(ctx, "POST", path, strings.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("docker network disconnect error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// ImageExists checks if an image exists locally
func (d *DefaultDockerClient) ImageExists(ctx context.Context, image string) (bool, error) {
	d.init()

	path := fmt.Sprintf("/images/%s/json", url.PathEscape(image))
	resp, err := d.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return false, nil
	}
	if resp.StatusCode >= 400 {
		return false, fmt.Errorf("docker image inspect error %d", resp.StatusCode)
	}

	return true, nil
}

// PullImage pulls an image from a registry
func (d *DefaultDockerClient) PullImage(ctx context.Context, image string) error {
	d.init()

	if d.readOnly {
		return fmt.Errorf("docker is in read-only mode")
	}

	path := fmt.Sprintf("/images/create?fromImage=%s", url.QueryEscape(image))
	resp, err := d.doRequest(ctx, "POST", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("docker pull error %d: %s", resp.StatusCode, string(body))
	}

	// Read response to ensure pull completes
	io.Copy(io.Discard, resp.Body)

	return nil
}

// buildDockerCreateRequest builds the Docker API container create request body
func buildDockerCreateRequest(config *ContainerCreateConfig) map[string]interface{} {
	req := map[string]interface{}{
		"Image":  config.Image,
		"Labels": config.Labels,
	}

	// Environment
	if len(config.Env) > 0 {
		req["Env"] = config.Env
	}

	// Working directory
	if config.WorkingDir != "" {
		req["WorkingDir"] = config.WorkingDir
	}

	// Entrypoint
	if len(config.Entrypoint) > 0 {
		req["Entrypoint"] = config.Entrypoint
	}

	// Command
	if len(config.Command) > 0 {
		req["Cmd"] = config.Command
	}

	// ExposedPorts
	if config.Port > 0 {
		req["ExposedPorts"] = map[string]interface{}{
			fmt.Sprintf("%d/tcp", config.Port): struct{}{},
		}
	}

	// HostConfig
	hostConfig := map[string]interface{}{}

	// Port bindings - bind to 127.0.0.1 only for security
	if config.HostPort > 0 && config.Port > 0 {
		hostConfig["PortBindings"] = map[string]interface{}{
			fmt.Sprintf("%d/tcp", config.Port): []map[string]string{
				{"HostIp": "127.0.0.1", "HostPort": fmt.Sprintf("%d", config.HostPort)},
			},
		}
	}

	// Network mode
	if config.Network != "" {
		hostConfig["NetworkMode"] = config.Network
	}

	// Binds (volumes)
	var binds []string
	for _, v := range config.Volumes {
		bind := fmt.Sprintf("%s:%s", v.Source, v.Target)
		if v.ReadOnly {
			bind += ":ro"
		}
		binds = append(binds, bind)
	}
	if len(binds) > 0 {
		hostConfig["Binds"] = binds
	}

	// Devices
	if len(config.Devices) > 0 {
		var devices []map[string]string
		for _, d := range config.Devices {
			dev := map[string]string{
				"PathOnHost":        d.Host,
				"PathInContainer":   d.Container,
				"CgroupPermissions": d.Permissions,
			}
			if dev["PathInContainer"] == "" {
				dev["PathInContainer"] = d.Host
			}
			if dev["CgroupPermissions"] == "" {
				dev["CgroupPermissions"] = "rwm"
			}
			devices = append(devices, dev)
		}
		hostConfig["Devices"] = devices
	}

	// Extra hosts
	if len(config.ExtraHosts) > 0 {
		hostConfig["ExtraHosts"] = config.ExtraHosts
	}

	// Restart policy
	if config.Restart != "" {
		hostConfig["RestartPolicy"] = map[string]interface{}{
			"Name": config.Restart,
		}
	}

	// Resources
	if config.Resources != nil {
		if config.Resources.Memory > 0 {
			hostConfig["Memory"] = config.Resources.Memory
		}
		if config.Resources.CPUs > 0 {
			// NanoCPUs is CPUs * 1e9
			hostConfig["NanoCpus"] = int64(config.Resources.CPUs * 1e9)
		}
		if config.Resources.CPUShares > 0 {
			hostConfig["CpuShares"] = config.Resources.CPUShares
		}
		if config.Resources.PidsLimit > 0 {
			hostConfig["PidsLimit"] = config.Resources.PidsLimit
		}
	}

	// ShmSize (shared memory)
	if config.ShmSize > 0 {
		hostConfig["ShmSize"] = config.ShmSize
	}

	// Security
	if config.Security != nil {
		if config.Security.Privileged {
			hostConfig["Privileged"] = true
		}
		if len(config.Security.CapAdd) > 0 {
			hostConfig["CapAdd"] = config.Security.CapAdd
		}
		if len(config.Security.CapDrop) > 0 {
			hostConfig["CapDrop"] = config.Security.CapDrop
		}
		if config.Security.ReadOnlyRootfs {
			hostConfig["ReadonlyRootfs"] = true
		}
		if len(config.Security.SecurityOpt) > 0 {
			hostConfig["SecurityOpt"] = config.Security.SecurityOpt
		}
		if config.Security.User != "" {
			req["User"] = config.Security.User
		}
	}

	req["HostConfig"] = hostConfig

	return req
}
