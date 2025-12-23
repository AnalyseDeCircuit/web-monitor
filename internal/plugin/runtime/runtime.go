// Package runtime manages plugin container lifecycle.
// It handles creating, starting, stopping, and removing plugin containers
// using the Docker API, based on manifest.json configuration.
package runtime

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/AnalyseDeCircuit/opskernel/internal/plugin/registry"
)

// ContainerState represents the runtime state of a plugin container
type ContainerState string

const (
	StateUnknown  ContainerState = "unknown"
	StateCreated  ContainerState = "created"
	StateRunning  ContainerState = "running"
	StateStopped  ContainerState = "stopped"
	StateNotFound ContainerState = "not_found"
	StateError    ContainerState = "error"
)

// PluginInstance represents a running plugin instance
type PluginInstance struct {
	Name          string
	ContainerName string
	ContainerID   string
	State         ContainerState
	HostPort      int
	InternalPort  int
	BaseURL       string
	Proxy         *httputil.ReverseProxy
	Error         string
	StartedAt     *time.Time
}

// Runtime manages plugin container lifecycle
type Runtime struct {
	docker    DockerClient
	instances map[string]*PluginInstance
	portAlloc map[int]string // port -> plugin name
	mu        sync.RWMutex
	portMu    sync.Mutex
}

// DockerClient interface abstracts Docker operations
// This allows for easier testing and potential alternative implementations
type DockerClient interface {
	// Container lifecycle
	CreateContainer(ctx context.Context, config *ContainerCreateConfig) (string, error)
	StartContainer(ctx context.Context, containerID string) error
	StopContainer(ctx context.Context, containerID string, timeout int) error
	RemoveContainer(ctx context.Context, containerID string, force bool) error

	// Container inspection
	InspectContainer(ctx context.Context, containerID string) (*ContainerInfo, error)
	ListContainers(ctx context.Context, all bool) ([]ContainerListItem, error)

	// Network
	ConnectNetwork(ctx context.Context, networkID, containerID string) error
	DisconnectNetwork(ctx context.Context, networkID, containerID string, force bool) error

	// Image
	ImageExists(ctx context.Context, image string) (bool, error)
	PullImage(ctx context.Context, image string) error
}

// ContainerCreateConfig holds parameters for creating a container
type ContainerCreateConfig struct {
	Name       string
	Image      string
	Env        []string
	Labels     map[string]string
	HostPort   int
	Port       int
	Volumes    []VolumeBinding
	Devices    []DeviceBinding
	Network    string
	Resources  *ResourceConfig
	Security   *SecurityOptions
	ExtraHosts []string
	WorkingDir string
	Entrypoint []string
	Command    []string
	Restart    string
}

// VolumeBinding represents a volume mount
type VolumeBinding struct {
	Source   string
	Target   string
	ReadOnly bool
	Type     string // "bind" or "volume"
}

// DeviceBinding represents a device mapping
type DeviceBinding struct {
	Host        string
	Container   string
	Permissions string
}

// ResourceConfig holds resource limits
type ResourceConfig struct {
	Memory     int64 // bytes
	MemorySwap int64 // bytes, -1 for unlimited
	CPUs       float64
	CPUShares  int64
	PidsLimit  int64
}

// SecurityOptions holds security configuration
type SecurityOptions struct {
	Privileged      bool
	CapAdd          []string
	CapDrop         []string
	ReadOnlyRootfs  bool
	NoNewPrivileges bool
	SecurityOpt     []string
	User            string
}

// ContainerInfo holds container inspection data
type ContainerInfo struct {
	ID          string
	Name        string
	State       ContainerState
	Running     bool
	Ports       map[int]int // container port -> host port
	StartedAt   time.Time
	FinishedAt  time.Time
	ExitCode    int
	Error       string
	Labels      map[string]string
	NetworkMode string
}

// ContainerListItem represents a container in the list
type ContainerListItem struct {
	ID     string
	Names  []string
	Image  string
	State  string
	Status string
	Labels map[string]string
}

const (
	hostPortRangeStart = 38100
	hostPortRangeEnd   = 38199

	// Labels used to identify OpsKernel-managed containers
	LabelManagedBy   = "opskernel.managed"
	LabelPluginName  = "opskernel.plugin.name"
	LabelPluginVer   = "opskernel.plugin.version"
	LabelManifestVer = "opskernel.manifest.version"
)

// NewRuntime creates a new plugin runtime manager
func NewRuntime(docker DockerClient) *Runtime {
	return &Runtime{
		docker:    docker,
		instances: make(map[string]*PluginInstance),
		portAlloc: make(map[int]string),
	}
}

// SyncState synchronizes runtime state with Docker
func (r *Runtime) SyncState(ctx context.Context) error {
	containers, err := r.docker.ListContainers(ctx, true)
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Find OpsKernel-managed containers
	for _, c := range containers {
		if c.Labels[LabelManagedBy] != "true" {
			continue
		}

		pluginName := c.Labels[LabelPluginName]
		if pluginName == "" {
			continue
		}

		// Get container info
		info, err := r.docker.InspectContainer(ctx, c.ID)
		if err != nil {
			continue
		}

		state := StateUnknown
		switch strings.ToLower(c.State) {
		case "running":
			state = StateRunning
		case "created", "paused":
			state = StateCreated
		case "exited", "dead":
			state = StateStopped
		}

		instance := &PluginInstance{
			Name:          pluginName,
			ContainerID:   c.ID,
			ContainerName: strings.TrimPrefix(c.Names[0], "/"),
			State:         state,
		}

		// Extract port mapping
		for containerPort, hostPort := range info.Ports {
			instance.InternalPort = containerPort
			instance.HostPort = hostPort
			break
		}

		if instance.HostPort > 0 {
			instance.BaseURL = fmt.Sprintf("http://127.0.0.1:%d", instance.HostPort)
			r.portAlloc[instance.HostPort] = pluginName
		}

		if state == StateRunning && instance.BaseURL != "" {
			r.setupProxy(instance)
		}

		r.instances[pluginName] = instance
	}

	return nil
}

// CreateAndStart creates and starts a plugin container from manifest.
// It implements idempotent startup:
//   - If container exists and is running → return existing instance (no-op)
//   - If container exists but stopped → start it
//   - If container does not exist → create and start
//
// This approach avoids Docker 409 conflicts and supports crash recovery.
func (r *Runtime) CreateAndStart(ctx context.Context, manifest *registry.Manifest, extraEnv map[string]string) (*PluginInstance, error) {
	containerName := manifest.ContainerNameOrDefault()

	// Step 1: Check if we already have a running instance in memory
	r.mu.RLock()
	if existing, ok := r.instances[manifest.Name]; ok && existing.State == StateRunning {
		r.mu.RUnlock()
		return existing, nil
	}
	r.mu.RUnlock()

	// Step 2: Check actual container state in Docker (handles restart/crash recovery)
	existingInfo, err := r.docker.InspectContainer(ctx, containerName)
	if err == nil {
		// Container exists - try to start it (idempotent)
		return r.startExistingContainer(ctx, manifest, existingInfo)
	}
	// If error is not "not found", it might be a transient Docker error
	if !IsNotFoundError(err) {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	// Step 3: Container does not exist - create new one
	return r.createNewContainer(ctx, manifest, containerName, extraEnv)
}

// startExistingContainer starts an existing container (idempotent)
func (r *Runtime) startExistingContainer(ctx context.Context, manifest *registry.Manifest, info *ContainerInfo) (*PluginInstance, error) {
	// If already running, just sync state
	if info.State == StateRunning {
		return r.syncExistingInstance(manifest, info)
	}

	// Start the stopped container
	if err := r.docker.StartContainer(ctx, info.ID); err != nil {
		return nil, fmt.Errorf("failed to start existing container: %w", err)
	}

	// Re-inspect to get updated port mappings
	updatedInfo, err := r.docker.InspectContainer(ctx, info.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect after start: %w", err)
	}

	return r.syncExistingInstance(manifest, updatedInfo)
}

// syncExistingInstance syncs runtime state with an existing container
func (r *Runtime) syncExistingInstance(manifest *registry.Manifest, info *ContainerInfo) (*PluginInstance, error) {
	var hostPort int
	for _, hp := range info.Ports {
		hostPort = hp
		break
	}

	// Allocate/reclaim the port
	if hostPort > 0 {
		r.portMu.Lock()
		r.portAlloc[hostPort] = manifest.Name
		r.portMu.Unlock()
	}

	now := time.Now()
	instance := &PluginInstance{
		Name:          manifest.Name,
		ContainerName: info.Name,
		ContainerID:   info.ID,
		State:         info.State,
		HostPort:      hostPort,
		InternalPort:  manifest.Docker.Port,
		StartedAt:     &now,
	}

	if hostPort > 0 {
		instance.BaseURL = fmt.Sprintf("http://127.0.0.1:%d", hostPort)
		r.setupProxy(instance)
	}

	r.mu.Lock()
	r.instances[manifest.Name] = instance
	r.mu.Unlock()

	return instance, nil
}

// createNewContainer creates and starts a new container
func (r *Runtime) createNewContainer(ctx context.Context, manifest *registry.Manifest, containerName string, extraEnv map[string]string) (*PluginInstance, error) {
	// Allocate host port
	hostPort := r.allocatePort(manifest.Name, 0)
	if hostPort == 0 {
		return nil, fmt.Errorf("failed to allocate host port")
	}

	// Build create config from manifest
	config := r.buildCreateConfig(manifest, containerName, hostPort)

	// Add extra environment variables
	for k, v := range extraEnv {
		config.Env = append(config.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Check if image exists
	exists, err := r.docker.ImageExists(ctx, manifest.Docker.Image)
	if err != nil {
		r.releasePort(hostPort)
		return nil, fmt.Errorf("failed to check image: %w", err)
	}
	if !exists {
		// Try to pull
		if err := r.docker.PullImage(ctx, manifest.Docker.Image); err != nil {
			r.releasePort(hostPort)
			return nil, fmt.Errorf("failed to pull image %s: %w", manifest.Docker.Image, err)
		}
	}

	// Create container (with conflict handling)
	containerID, err := r.docker.CreateContainer(ctx, config)
	if err != nil {
		// Handle 409 conflict: container name already in use
		// This can happen in race conditions or if SyncState hasn't run
		if IsConflictError(err) {
			// Try to recover by inspecting and starting the existing container
			existingInfo, inspectErr := r.docker.InspectContainer(ctx, containerName)
			if inspectErr == nil {
				r.releasePort(hostPort) // Release the port we allocated, will reclaim from existing
				return r.startExistingContainer(ctx, manifest, existingInfo)
			}
		}
		r.releasePort(hostPort)
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// Start container
	if err := r.docker.StartContainer(ctx, containerID); err != nil {
		// Cleanup on failure
		_ = r.docker.RemoveContainer(ctx, containerID, true)
		r.releasePort(hostPort)
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	now := time.Now()
	instance := &PluginInstance{
		Name:          manifest.Name,
		ContainerName: containerName,
		ContainerID:   containerID,
		State:         StateRunning,
		HostPort:      hostPort,
		InternalPort:  manifest.Docker.Port,
		BaseURL:       fmt.Sprintf("http://127.0.0.1:%d", hostPort),
		StartedAt:     &now,
	}

	r.setupProxy(instance)

	r.mu.Lock()
	r.instances[manifest.Name] = instance
	r.mu.Unlock()

	return instance, nil
}

// Start starts an existing plugin container
func (r *Runtime) Start(ctx context.Context, name string) error {
	r.mu.RLock()
	instance, ok := r.instances[name]
	r.mu.RUnlock()

	if !ok || instance.ContainerID == "" {
		return fmt.Errorf("plugin container not found: %s", name)
	}

	if instance.State == StateRunning {
		return nil
	}

	if err := r.docker.StartContainer(ctx, instance.ContainerID); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	r.mu.Lock()
	instance.State = StateRunning
	now := time.Now()
	instance.StartedAt = &now
	instance.Error = ""
	r.setupProxy(instance)
	r.mu.Unlock()

	return nil
}

// Stop stops a running plugin container
func (r *Runtime) Stop(ctx context.Context, name string) error {
	r.mu.RLock()
	instance, ok := r.instances[name]
	r.mu.RUnlock()

	if !ok || instance.ContainerID == "" {
		return nil // Not an error if not found
	}

	if instance.State != StateRunning {
		return nil
	}

	// Try to stop - ignore errors if container doesn't exist
	_ = r.docker.StopContainer(ctx, instance.ContainerID, 10)

	r.mu.Lock()
	instance.State = StateStopped
	instance.Proxy = nil
	r.mu.Unlock()

	return nil
}

// Remove removes a plugin container
func (r *Runtime) Remove(ctx context.Context, name string, force bool) error {
	r.mu.Lock()
	instance, ok := r.instances[name]
	if !ok {
		r.mu.Unlock()
		return nil
	}

	// Release port
	if instance.HostPort > 0 {
		delete(r.portAlloc, instance.HostPort)
	}

	delete(r.instances, name)
	r.mu.Unlock()

	// Try to remove - ignore errors if container doesn't exist
	if instance.ContainerID != "" {
		_ = r.docker.RemoveContainer(ctx, instance.ContainerID, force)
	}

	return nil
}

// GetInstance returns the runtime instance for a plugin
func (r *Runtime) GetInstance(name string) (*PluginInstance, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instance, ok := r.instances[name]
	if !ok {
		return nil, false
	}

	// Return a copy
	cp := *instance
	return &cp, true
}

// GetProxy returns the reverse proxy for a running plugin
func (r *Runtime) GetProxy(name string) *httputil.ReverseProxy {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if instance, ok := r.instances[name]; ok && instance.State == StateRunning {
		return instance.Proxy
	}
	return nil
}

// IsRunning checks if a plugin is running
func (r *Runtime) IsRunning(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if instance, ok := r.instances[name]; ok {
		return instance.State == StateRunning
	}
	return false
}

// ListInstances returns all plugin instances
func (r *Runtime) ListInstances() []*PluginInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*PluginInstance, 0, len(r.instances))
	for _, inst := range r.instances {
		cp := *inst
		list = append(list, &cp)
	}
	return list
}

// buildCreateConfig converts a manifest to container create config
func (r *Runtime) buildCreateConfig(manifest *registry.Manifest, containerName string, hostPort int) *ContainerCreateConfig {
	config := &ContainerCreateConfig{
		Name:     containerName,
		Image:    manifest.Docker.Image,
		HostPort: hostPort,
		Port:     manifest.Docker.Port,
		Network:  manifest.Docker.Network,
		Labels: map[string]string{
			LabelManagedBy:   "true",
			LabelPluginName:  manifest.Name,
			LabelPluginVer:   manifest.Version,
			LabelManifestVer: manifest.ManifestVersion,
		},
		Restart: "unless-stopped",
	}

	// Add user-defined labels
	for k, v := range manifest.Docker.Labels {
		config.Labels[k] = v
	}

	// Environment variables
	for k, v := range manifest.Docker.Env {
		config.Env = append(config.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Volumes
	for _, v := range manifest.Docker.Volumes {
		config.Volumes = append(config.Volumes, VolumeBinding{
			Source:   v.Source,
			Target:   v.Target,
			ReadOnly: v.ReadOnly,
			Type:     v.Type,
		})
	}

	// Devices
	for _, d := range manifest.Docker.Devices {
		config.Devices = append(config.Devices, DeviceBinding{
			Host:        d.Host,
			Container:   d.Container,
			Permissions: d.Permissions,
		})
	}

	// Resources
	if manifest.Docker.Resources != nil {
		config.Resources = &ResourceConfig{
			Memory:    parseMemory(manifest.Docker.Resources.Memory),
			CPUs:      parseCPUs(manifest.Docker.Resources.CPUs),
			CPUShares: manifest.Docker.Resources.CPUShares,
			PidsLimit: manifest.Docker.Resources.PidsLimit,
		}
	}

	// Security
	if manifest.Docker.Security != nil {
		config.Security = &SecurityOptions{
			Privileged:      manifest.Docker.Security.Privileged,
			CapAdd:          manifest.Docker.Security.CapAdd,
			CapDrop:         manifest.Docker.Security.CapDrop,
			ReadOnlyRootfs:  manifest.Docker.Security.ReadOnlyRootfs,
			NoNewPrivileges: manifest.Docker.Security.NoNewPrivileges,
			SecurityOpt:     manifest.Docker.Security.SecurityOpt,
			User:            manifest.Docker.Security.User,
		}
	}

	// Other settings
	config.ExtraHosts = manifest.Docker.ExtraHosts
	config.WorkingDir = manifest.Docker.WorkingDir
	config.Entrypoint = manifest.Docker.Entrypoint
	config.Command = manifest.Docker.Command

	if manifest.Docker.RestartPolicy != "" {
		config.Restart = manifest.Docker.RestartPolicy
	}

	return config
}

func (r *Runtime) setupProxy(instance *PluginInstance) {
	if instance.BaseURL == "" {
		return
	}

	target, err := url.Parse(instance.BaseURL)
	if err != nil {
		return
	}

	instance.Proxy = httputil.NewSingleHostReverseProxy(target)

	// Customize error handler
	instance.Proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		http.Error(w, fmt.Sprintf("Plugin proxy error: %v", err), http.StatusBadGateway)
	}
}

func (r *Runtime) allocatePort(name string, preferred int) int {
	r.portMu.Lock()
	defer r.portMu.Unlock()

	// Try preferred port first
	if preferred >= hostPortRangeStart && preferred <= hostPortRangeEnd {
		if _, used := r.portAlloc[preferred]; !used {
			r.portAlloc[preferred] = name
			return preferred
		}
	}

	// Find next available
	for port := hostPortRangeStart; port <= hostPortRangeEnd; port++ {
		if _, used := r.portAlloc[port]; !used {
			r.portAlloc[port] = name
			return port
		}
	}

	return 0
}

func (r *Runtime) releasePort(port int) {
	r.portMu.Lock()
	defer r.portMu.Unlock()
	delete(r.portAlloc, port)
}

// parseMemory converts memory string to bytes
func parseMemory(s string) int64 {
	if s == "" {
		return 0
	}

	s = strings.ToLower(strings.TrimSpace(s))
	var multiplier int64 = 1

	if strings.HasSuffix(s, "g") {
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	} else if strings.HasSuffix(s, "m") {
		multiplier = 1024 * 1024
		s = s[:len(s)-1]
	} else if strings.HasSuffix(s, "k") {
		multiplier = 1024
		s = s[:len(s)-1]
	}

	var value int64
	fmt.Sscanf(s, "%d", &value)
	return value * multiplier
}

// parseCPUs converts CPU string to float
func parseCPUs(s string) float64 {
	if s == "" {
		return 0
	}

	var value float64
	fmt.Sscanf(s, "%f", &value)
	return value
}
