package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AnalyseDeCircuit/opskernel/internal/docker"
)

// Plugin represents a loaded plugin instance
type Plugin struct {
	// From manifest
	Name        string     `json:"name"`
	Version     string     `json:"version"`
	Description string     `json:"description,omitempty"`
	Type        PluginType `json:"type"`
	Risk        string     `json:"risk"`
	AdminOnly   bool       `json:"adminOnly"`

	// Runtime state
	State         PluginState `json:"state"`
	Enabled       bool        `json:"enabled"`
	Running       bool        `json:"running"`
	ContainerName string      `json:"containerName,omitempty"`
	BaseURL       string      `json:"baseUrl,omitempty"`
	HostPort      int         `json:"hostPort,omitempty"`
	Error         string      `json:"error,omitempty"`

	// Internal
	Mode     string                 `json:"mode"` // "docker" or "exec"
	Port     int                    `json:"port,omitempty"`
	Process  *os.Process            `json:"-"`
	Proxy    *httputil.ReverseProxy `json:"-"`
	manifest *Manifest              `json:"-"`
	execPath string                 `json:"-"`
}

// Manager handles plugin lifecycle
type Manager struct {
	pluginsDir    string               // Directory containing plugin manifests
	installedDir  string               // Directory for installed plugin state
	plugins       map[string]*Plugin   // Loaded plugins by name
	manifests     map[string]*Manifest // Available manifests by name
	mu            sync.RWMutex
	hostPortAlloc map[int]string // Track allocated host ports
	portMu        sync.Mutex
}

const (
	defaultPluginsDir    = "/app/plugins"
	defaultInstalledDir  = "/data/plugins/installed"
	defaultPluginsConfig = "/etc/opskernel/plugins.json"
	enabledStatePath     = "/data/plugins-enabled.json"
	hostPortRangeStart   = 38100
	hostPortRangeEnd     = 38199
)

// NewManager creates a new plugin manager
func NewManager() *Manager {
	return &Manager{
		plugins:       make(map[string]*Plugin),
		manifests:     make(map[string]*Manifest),
		hostPortAlloc: make(map[int]string),
	}
}

// LoadPlugins discovers and loads all plugins
func (m *Manager) LoadPlugins(pluginDir string) error {
	m.pluginsDir = pluginDir
	m.installedDir = defaultInstalledDir

	// Ensure directories exist
	for _, dir := range []string{m.pluginsDir, m.installedDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Printf("Warning: could not create directory %s: %v\n", dir, err)
		}
	}

	// 1. Discover manifests from plugins directory
	m.discoverManifests()

	// 2. Sync container states from Docker
	m.syncContainerStates()

	// 3. Apply persisted enabled state
	m.applyEnabledState()

	// 4. Start enabled plugins
	m.startEnabledPlugins()

	return nil
}

// discoverManifests scans for plugin.json files in subdirectories
func (m *Manager) discoverManifests() {
	fmt.Printf("Scanning plugins directory: %s\n", m.pluginsDir)

	entries, err := os.ReadDir(m.pluginsDir)
	if err != nil {
		fmt.Printf("Warning: could not read plugins directory %s: %v\n", m.pluginsDir, err)
		return
	}

	fmt.Printf("Found %d entries in plugins directory\n", len(entries))

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		manifestPath := filepath.Join(m.pluginsDir, entry.Name(), "plugin.json")
		manifest, err := loadManifest(manifestPath)
		if err != nil {
			// Not a valid plugin directory, skip
			fmt.Printf("  - %s: no valid plugin.json (%v)\n", entry.Name(), err)
			continue
		}

		if err := manifest.Validate(); err != nil {
			fmt.Printf("Warning: invalid manifest for %s: %v\n", entry.Name(), err)
			continue
		}

		fmt.Printf("  + Loaded plugin: %s (v%s, type=%s)\n", manifest.Name, manifest.Version, manifest.Type)

		m.mu.Lock()
		m.manifests[manifest.Name] = manifest
		m.registerPluginFromManifest(manifest)
		m.mu.Unlock()
	}

	fmt.Printf("Total plugins loaded: %d\n", len(m.manifests))
}

// loadManifest reads and parses a plugin.json file
func loadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	return &manifest, nil
}

// registerPluginFromManifest creates a Plugin instance from a Manifest
func (m *Manager) registerPluginFromManifest(manifest *Manifest) {
	containerName := manifest.ContainerNameOrDefault()
	hostPort := m.allocateHostPort(manifest.Name, manifest.Container.HostPort)

	plugin := &Plugin{
		Name:          manifest.Name,
		Version:       manifest.Version,
		Description:   manifest.Description,
		Type:          manifest.Type,
		Risk:          manifest.Risk,
		AdminOnly:     manifest.AdminOnly || manifest.IsPrivileged(),
		State:         StateInstalled,
		Enabled:       false,
		Running:       false,
		ContainerName: containerName,
		BaseURL:       manifest.BaseURL(hostPort),
		HostPort:      hostPort,
		Mode:          "docker",
		manifest:      manifest,
	}

	// High-risk plugins default to disabled
	if manifest.Risk == "high" || manifest.IsPrivileged() {
		plugin.Enabled = false
	}

	m.plugins[manifest.Name] = plugin
}

// syncContainerStates queries Docker to update running states
func (m *Manager) syncContainerStates() {
	containers, err := docker.ListContainers()
	if err != nil {
		fmt.Printf("Warning: could not list Docker containers: %v\n", err)
		return
	}

	// Build lookup map: container name -> running state
	containerStates := make(map[string]bool)
	for _, c := range containers {
		running := strings.EqualFold(c.State, "running")
		for _, name := range c.Names {
			containerStates[strings.TrimPrefix(name, "/")] = running
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, plugin := range m.plugins {
		if plugin.Mode != "docker" || plugin.ContainerName == "" {
			continue
		}

		if running, exists := containerStates[plugin.ContainerName]; exists {
			plugin.Running = running
			if running {
				plugin.State = StateRunning
				// Setup proxy
				if target, err := url.Parse(plugin.BaseURL); err == nil {
					plugin.Proxy = httputil.NewSingleHostReverseProxy(target)
				}
			} else {
				plugin.State = StateStopped
			}
		} else {
			// Container doesn't exist
			plugin.State = StateInstalled
			plugin.Running = false
		}
	}
}

// applyEnabledState loads and applies persisted enabled states
func (m *Manager) applyEnabledState() {
	state, err := loadEnabledState()
	if err != nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for name, enabled := range state {
		if plugin, exists := m.plugins[name]; exists {
			plugin.Enabled = enabled
		}
	}
}

// startEnabledPlugins starts all plugins that are enabled
func (m *Manager) startEnabledPlugins() {
	plugins := m.ListPlugins()
	for _, p := range plugins {
		if p.Enabled {
			if err := m.StartPlugin(p.Name); err != nil {
				fmt.Printf("Failed to start plugin %s: %v\n", p.Name, err)
			}
		}
	}
}

// allocateHostPort assigns a host port for a plugin
func (m *Manager) allocateHostPort(name string, preferred int) int {
	m.portMu.Lock()
	defer m.portMu.Unlock()

	// If preferred port is specified and available, use it
	if preferred >= hostPortRangeStart && preferred <= hostPortRangeEnd {
		if _, used := m.hostPortAlloc[preferred]; !used {
			m.hostPortAlloc[preferred] = name
			return preferred
		}
	}

	// Find next available port in range
	for port := hostPortRangeStart; port <= hostPortRangeEnd; port++ {
		if _, used := m.hostPortAlloc[port]; !used {
			m.hostPortAlloc[port] = name
			return port
		}
	}

	// Fallback to dynamic port
	return 0
}

// releaseHostPort releases an allocated port
func (m *Manager) releaseHostPort(port int) {
	m.portMu.Lock()
	defer m.portMu.Unlock()
	delete(m.hostPortAlloc, port)
}

// ============================================================================
// Plugin Operations
// ============================================================================

// GetPlugin returns a plugin by name
func (m *Manager) GetPlugin(name string) (*Plugin, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugin, exists := m.plugins[name]
	if !exists {
		return nil, false
	}

	// Return a copy
	cp := *plugin
	return &cp, true
}

// ListPlugins returns all loaded plugins
func (m *Manager) ListPlugins() []*Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*Plugin, 0, len(m.plugins))
	for _, p := range m.plugins {
		cp := *p
		list = append(list, &cp)
	}

	// Sort by name for consistent ordering
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})

	return list
}

// ListPluginsForRole returns plugins visible to a specific role
func (m *Manager) ListPluginsForRole(role string) []*Plugin {
	plugins := m.ListPlugins()
	if role == "admin" {
		return plugins
	}

	filtered := make([]*Plugin, 0)
	for _, p := range plugins {
		if !p.AdminOnly {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// StartPlugin starts a plugin
func (m *Manager) StartPlugin(name string) error {
	m.mu.Lock()
	plugin, exists := m.plugins[name]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("plugin not found: %s", name)
	}

	if plugin.Running {
		m.mu.Unlock()
		return nil
	}

	mode := plugin.Mode
	containerName := plugin.ContainerName
	baseURL := plugin.BaseURL
	execPath := plugin.execPath
	m.mu.Unlock()

	if mode == "docker" {
		return m.startDockerPlugin(name, containerName, baseURL)
	}

	return m.startExecPlugin(name, execPath)
}

// startDockerPlugin starts a Docker-based plugin
func (m *Manager) startDockerPlugin(name, containerName, baseURL string) error {
	if containerName == "" || baseURL == "" {
		return fmt.Errorf("docker plugin misconfigured: missing containerName or baseURL")
	}

	// Try to start the container
	if err := docker.ContainerAction(containerName, "start"); err != nil {
		// If container doesn't exist and we have a manifest, try to create it
		if strings.Contains(err.Error(), "No such container") {
			m.mu.RLock()
			plugin := m.plugins[name]
			manifest := plugin.manifest
			m.mu.RUnlock()

			if manifest != nil {
				if createErr := m.createPluginContainer(manifest); createErr != nil {
					return fmt.Errorf("failed to create container: %v (original: %v)", createErr, err)
				}
				// Retry start
				if err := docker.ContainerAction(containerName, "start"); err != nil {
					return fmt.Errorf("failed to start container after creation: %v", err)
				}
			} else {
				return fmt.Errorf("container does not exist and no manifest available: %v", err)
			}
		} else {
			return fmt.Errorf("failed to start container %s: %v", containerName, err)
		}
	}

	// Setup proxy
	target, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("invalid baseURL: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	m.mu.Lock()
	if p, ok := m.plugins[name]; ok {
		p.Proxy = proxy
		p.Running = true
		p.State = StateRunning
		p.Error = ""
	}
	m.mu.Unlock()

	// Wait for plugin to be ready
	time.Sleep(500 * time.Millisecond)
	return nil
}

// startExecPlugin starts an executable plugin
func (m *Manager) startExecPlugin(name, execPath string) error {
	port, err := getFreePort()
	if err != nil {
		return fmt.Errorf("failed to get free port: %v", err)
	}

	cmd := exec.Command(execPath)
	cmd.Env = append(os.Environ(), fmt.Sprintf("PLUGIN_PORT=%d", port))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start plugin process: %v", err)
	}

	target, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	proxy := httputil.NewSingleHostReverseProxy(target)

	m.mu.Lock()
	if p, ok := m.plugins[name]; ok {
		p.Port = port
		p.Process = cmd.Process
		p.Proxy = proxy
		p.Running = true
		p.State = StateRunning
	}
	m.mu.Unlock()

	fmt.Printf("Plugin %s started on port %d\n", name, port)
	time.Sleep(500 * time.Millisecond)
	return nil
}

// StopPlugin stops a plugin
func (m *Manager) StopPlugin(name string) error {
	m.mu.Lock()
	plugin, exists := m.plugins[name]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("plugin not found: %s", name)
	}

	mode := plugin.Mode
	containerName := plugin.ContainerName
	proc := plugin.Process
	wasRunning := plugin.Running

	plugin.Process = nil
	plugin.Proxy = nil
	plugin.Port = 0
	plugin.Running = false
	plugin.State = StateStopped
	m.mu.Unlock()

	if mode == "docker" {
		if wasRunning && containerName != "" {
			if err := docker.ContainerAction(containerName, "stop"); err != nil {
				return fmt.Errorf("failed to stop container %s: %v", containerName, err)
			}
		}
		return nil
	}

	if wasRunning && proc != nil {
		_ = proc.Kill()
		fmt.Printf("Plugin %s stopped (PID %d)\n", name, proc.Pid)
	}
	return nil
}

// TogglePlugin enables or disables a plugin
func (m *Manager) TogglePlugin(name string, enabled bool) error {
	m.mu.RLock()
	plugin, exists := m.plugins[name]
	if !exists {
		m.mu.RUnlock()
		return fmt.Errorf("plugin not found: %s", name)
	}
	currentEnabled := plugin.Enabled
	m.mu.RUnlock()

	if currentEnabled == enabled {
		return nil
	}

	if enabled {
		if err := m.StartPlugin(name); err != nil {
			return err
		}
		m.mu.Lock()
		if p, ok := m.plugins[name]; ok {
			p.Enabled = true
			p.State = StateEnabled
		}
		m.mu.Unlock()
	} else {
		m.mu.Lock()
		if p, ok := m.plugins[name]; ok {
			p.Enabled = false
			p.State = StateDisabled
		}
		m.mu.Unlock()

		if err := m.StopPlugin(name); err != nil {
			return err
		}
	}

	return saveEnabledState(m.currentEnabledState())
}

// ============================================================================
// Container Management
// ============================================================================

// createPluginContainer creates a Docker container for a plugin based on its manifest
func (m *Manager) createPluginContainer(manifest *Manifest) error {
	containerName := manifest.ContainerNameOrDefault()

	// For now, we just log that auto-creation would happen
	// Full implementation requires Docker API create endpoint
	fmt.Printf("Would create container %s from image %s (auto-create not yet implemented)\n",
		containerName, manifest.Container.Image)

	return fmt.Errorf("container auto-creation not yet implemented - please run: docker compose up -d --no-start plugin-%s", manifest.Name)
}

// ============================================================================
// HTTP Proxy
// ============================================================================

// ServeHTTP proxies requests to a plugin
func (m *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request, pluginName string) {
	m.mu.RLock()
	plugin, exists := m.plugins[pluginName]
	m.mu.RUnlock()

	if !exists {
		http.NotFound(w, r)
		return
	}

	if !plugin.Enabled {
		http.Error(w, "Plugin is disabled", http.StatusForbidden)
		return
	}

	if !plugin.Running || plugin.Proxy == nil {
		http.Error(w, "Plugin is not running", http.StatusServiceUnavailable)
		return
	}

	prefix := "/api/plugins/" + pluginName
	http.StripPrefix(prefix, plugin.Proxy).ServeHTTP(w, r)
}

// ============================================================================
// Install / Uninstall Operations
// ============================================================================

// InstallResult contains the result of an installation
type InstallResult struct {
	Success bool     `json:"success"`
	Message string   `json:"message,omitempty"`
	Errors  []string `json:"errors,omitempty"`
}

// InstallPlugin installs a plugin by name (executes install hooks for privileged plugins)
func (m *Manager) InstallPlugin(name string) (*InstallResult, error) {
	m.mu.RLock()
	manifest, exists := m.manifests[name]
	plugin := m.plugins[name]
	m.mu.RUnlock()

	if !exists || manifest == nil {
		return nil, fmt.Errorf("plugin manifest not found: %s", name)
	}

	// Check current state
	if plugin != nil && plugin.State == StateRunning {
		return &InstallResult{Success: true, Message: "Plugin already installed and running"}, nil
	}

	fmt.Printf("Installing plugin: %s v%s\n", manifest.Name, manifest.Version)

	// Validate hooks before execution
	if err := ValidateHooks(manifest); err != nil {
		return &InstallResult{Success: false, Errors: []string{err.Error()}}, err
	}

	// Execute install hooks (for privileged plugins)
	if manifest.IsPrivileged() && manifest.Install != nil {
		fmt.Printf("Executing install hooks for privileged plugin %s...\n", name)
		executor := NewHookExecutor(manifest)
		if err := executor.ExecuteInstallHooks(); err != nil {
			return &InstallResult{
				Success: false,
				Message: "Installation failed",
				Errors:  []string{err.Error()},
			}, err
		}
	}

	// Update plugin state
	m.mu.Lock()
	if p, ok := m.plugins[name]; ok {
		p.State = StateInstalled
		p.Error = ""
	}
	m.mu.Unlock()

	return &InstallResult{
		Success: true,
		Message: fmt.Sprintf("Plugin %s installed successfully", name),
	}, nil
}

// UninstallPlugin uninstalls a plugin (executes uninstall hooks, stops container)
func (m *Manager) UninstallPlugin(name string, removeData bool) (*InstallResult, error) {
	m.mu.RLock()
	manifest := m.manifests[name]
	plugin := m.plugins[name]
	m.mu.RUnlock()

	if plugin == nil {
		return nil, fmt.Errorf("plugin not found: %s", name)
	}

	fmt.Printf("Uninstalling plugin: %s\n", name)

	// Stop the plugin first
	if plugin.Running {
		if err := m.StopPlugin(name); err != nil {
			fmt.Printf("Warning: failed to stop plugin: %v\n", err)
		}
	}

	// Execute uninstall hooks (for privileged plugins)
	var errs []string
	if manifest != nil && manifest.IsPrivileged() && manifest.Uninstall != nil {
		fmt.Printf("Executing uninstall hooks for privileged plugin %s...\n", name)
		executor := NewHookExecutor(manifest)
		if err := executor.ExecuteUninstallHooks(); err != nil {
			errs = append(errs, err.Error())
		}
	}

	// Remove container if requested
	if removeData && plugin.ContainerName != "" {
		if err := docker.ContainerAction(plugin.ContainerName, "remove"); err != nil {
			errs = append(errs, fmt.Sprintf("failed to remove container: %v", err))
		}
	}

	// Update state
	m.mu.Lock()
	if p, ok := m.plugins[name]; ok {
		p.State = StateAvailable
		p.Enabled = false
		p.Running = false
	}
	m.mu.Unlock()

	// Persist enabled state
	saveEnabledState(m.currentEnabledState())

	if len(errs) > 0 {
		return &InstallResult{
			Success: false,
			Message: "Uninstallation completed with errors",
			Errors:  errs,
		}, nil
	}

	return &InstallResult{
		Success: true,
		Message: fmt.Sprintf("Plugin %s uninstalled successfully", name),
	}, nil
}

// GetManifest returns a plugin's manifest
func (m *Manager) GetManifest(name string) (*Manifest, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	manifest, exists := m.manifests[name]
	return manifest, exists
}

// ============================================================================
// Utility Functions
// ============================================================================

func (m *Manager) currentEnabledState() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state := make(map[string]bool, len(m.plugins))
	for name, p := range m.plugins {
		state[name] = p.Enabled
	}
	return state
}

// Cleanup stops all running plugins
func (m *Manager) Cleanup() {
	for _, p := range m.ListPlugins() {
		if p.Running {
			_ = m.StopPlugin(p.Name)
		}
	}
}

func normalizeBaseURL(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s
	}
	return "http://" + s
}

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func loadEnabledState() (map[string]bool, error) {
	data, err := os.ReadFile(enabledStatePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var state map[string]bool
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return state, nil
}

func saveEnabledState(state map[string]bool) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(enabledStatePath), 0755); err != nil {
		return err
	}

	return os.WriteFile(enabledStatePath, data, 0644)
}
