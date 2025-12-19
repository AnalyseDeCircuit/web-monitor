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
	"strings"
	"sync"
	"time"

	"github.com/AnalyseDeCircuit/web-monitor/internal/docker"
)

type Plugin struct {
	Name      string `json:"name"`
	Port      int    `json:"port"`
	Enabled   bool   `json:"enabled"`
	Running   bool   `json:"running"`
	AdminOnly bool   `json:"adminOnly"`
	Risk      string `json:"risk"` // "low" | "high"
	Mode      string `json:"mode"` // "exec" | "docker"
	// Docker mode fields
	ContainerName string                 `json:"containerName,omitempty"`
	BaseURL       string                 `json:"baseUrl,omitempty"`
	Process       *os.Process            `json:"-"`
	Proxy         *httputil.ReverseProxy `json:"-"`
	execPath      string
	defaultOn     bool
}

const defaultPluginsConfigPath = "/etc/webmonitor/plugins.json"

type containerPluginSpec struct {
	ContainerName string `json:"containerName"`
	BaseURL       string `json:"baseUrl"`
}

type containersConfig struct {
	Containers map[string]containerPluginSpec `json:"containers"`
}

func readContainersConfig() (map[string]containerPluginSpec, string, bool, error) {
	path := strings.TrimSpace(os.Getenv("WEBMONITOR_PLUGINS_CONFIG"))
	if path == "" {
		path = defaultPluginsConfigPath
	}
	// Check if path is a directory (Docker creates dir if mount source doesn't exist).
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return nil, path, false, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, path, false, nil
		}
		return nil, path, false, err
	}
	var cfg containersConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, path, true, err
	}
	if len(cfg.Containers) == 0 {
		return nil, path, true, nil
	}
	return cfg.Containers, path, true, nil
}

func normalizeBaseURL(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Accept "127.0.0.1:38101" and turn into http://...
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s
	}
	return "http://" + s
}

type Manager struct {
	pluginDir string
	plugins   map[string]*Plugin
	mu        sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		plugins: make(map[string]*Plugin),
	}
}

// LoadPlugins scans plugins from the given directory, loads persisted enabled state,
// then starts enabled plugins.
func (m *Manager) LoadPlugins(pluginDir string) error {
	m.pluginDir = pluginDir
	// Ensure plugin directory exists
	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		if err := os.MkdirAll(pluginDir, 0755); err != nil {
			return fmt.Errorf("failed to create plugin directory: %v", err)
		}
	}

	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// Assume the file is an executable plugin
		name := entry.Name()
		execPath := filepath.Join(pluginDir, name)

		// Skip non-executable files (simple check)
		info, err := entry.Info()
		if err != nil || info.Mode()&0111 == 0 {
			continue
		}

		adminOnly, risk, defaultOn := classifyPlugin(name)
		m.mu.Lock()
		m.plugins[name] = &Plugin{
			Name:      name,
			Port:      0,
			Enabled:   defaultOn,
			Running:   false,
			AdminOnly: adminOnly,
			Risk:      risk,
			Mode:      "exec",
			execPath:  execPath,
			defaultOn: defaultOn,
		}
		m.mu.Unlock()
	}

	// Load container-based plugins from plugins.json (containers section).
	if containers, cfgPath, _, err := readContainersConfig(); err != nil {
		fmt.Printf("Failed to read plugin containers config %s: %v\n", cfgPath, err)
	} else if len(containers) > 0 {
		for name, spec := range containers {
			adminOnly, risk, defaultOn := classifyPlugin(name)
			baseURL := normalizeBaseURL(spec.BaseURL)
			if spec.ContainerName == "" || baseURL == "" {
				fmt.Printf("Invalid container plugin spec for %s in %s (containerName/baseUrl required)\n", name, cfgPath)
				continue
			}
			m.mu.Lock()
			// Prefer container plugin over exec plugin with same name.
			m.plugins[name] = &Plugin{
				Name:          name,
				Port:          0,
				Enabled:       defaultOn,
				Running:       false,
				AdminOnly:     adminOnly,
				Risk:          risk,
				Mode:          "docker",
				ContainerName: spec.ContainerName,
				BaseURL:       baseURL,
				defaultOn:     defaultOn,
			}
			m.mu.Unlock()
		}

		// Best-effort: refresh running state from Docker.
		if list, err := docker.ListContainers(); err == nil {
			byName := make(map[string]bool, len(list))
			for _, c := range list {
				running := strings.EqualFold(c.State, "running")
				for _, n := range c.Names {
					byName[strings.TrimPrefix(n, "/")] = running
				}
			}
			m.mu.Lock()
			for _, p := range m.plugins {
				if p.Mode != "docker" {
					continue
				}
				if running, ok := byName[p.ContainerName]; ok {
					p.Running = running
					if running {
						target, err := url.Parse(p.BaseURL)
						if err == nil {
							p.Proxy = httputil.NewSingleHostReverseProxy(target)
						}
					}
				}
			}
			m.mu.Unlock()
		}
	}

	// Apply persisted enabled state (overrides defaults)
	if state, err := loadEnabledState(); err == nil {
		m.mu.Lock()
		for name, enabled := range state {
			if p, ok := m.plugins[name]; ok {
				p.Enabled = enabled
			}
		}
		m.mu.Unlock()
	}

	// Start enabled plugins
	for _, p := range m.ListPlugins() {
		if p.Enabled {
			if err := m.StartPlugin(p.Name); err != nil {
				fmt.Printf("Failed to start plugin %s: %v\n", p.Name, err)
			}
		}
	}
	return nil
}

func classifyPlugin(name string) (adminOnly bool, risk string, defaultOn bool) {
	switch name {
	case "webshell":
		return true, "high", false
	case "filemanager":
		// File management is also sensitive; keep it admin-only.
		return true, "high", false
	default:
		return false, "low", true
	}
}

func (m *Manager) GetPlugin(name string) (*Plugin, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.plugins[name]
	if !ok {
		return nil, false
	}
	// Return a shallow copy so callers can't mutate internal state.
	cp := *p
	return &cp, true
}

func (m *Manager) StartPlugin(name string) error {
	m.mu.Lock()
	p, ok := m.plugins[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("plugin not found")
	}
	if p.Running {
		m.mu.Unlock()
		return nil
	}
	mode := p.Mode
	containerName := p.ContainerName
	baseURL := p.BaseURL
	execPath := p.execPath
	m.mu.Unlock()

	if mode == "docker" {
		if containerName == "" || baseURL == "" {
			return fmt.Errorf("docker plugin misconfigured")
		}
		if err := docker.ContainerAction(containerName, "start"); err != nil {
			return fmt.Errorf("failed to start docker plugin container %s: %v", containerName, err)
		}
		target, err := url.Parse(baseURL)
		if err != nil {
			return fmt.Errorf("invalid plugin baseUrl: %v", err)
		}
		proxy := httputil.NewSingleHostReverseProxy(target)
		m.mu.Lock()
		p, ok = m.plugins[name]
		if ok {
			p.Proxy = proxy
			p.Running = true
			p.Port = 0
		}
		m.mu.Unlock()
		time.Sleep(500 * time.Millisecond)
		return nil
	}

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
	p, ok = m.plugins[name]
	if ok {
		p.Port = port
		p.Process = cmd.Process
		p.Proxy = proxy
		p.Running = true
	}
	m.mu.Unlock()

	fmt.Printf("Plugin %s started on port %d\n", name, port)
	// Wait a bit for the plugin to start listening
	time.Sleep(500 * time.Millisecond)
	return nil
}

func (m *Manager) StopPlugin(name string) error {
	m.mu.Lock()
	p, ok := m.plugins[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("plugin not found")
	}
	mode := p.Mode
	containerName := p.ContainerName
	proc := p.Process
	wasRunning := p.Running
	p.Process = nil
	p.Proxy = nil
	p.Port = 0
	p.Running = false
	m.mu.Unlock()

	if mode == "docker" {
		if wasRunning && containerName != "" {
			if err := docker.ContainerAction(containerName, "stop"); err != nil {
				return fmt.Errorf("failed to stop docker plugin container %s: %v", containerName, err)
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

// ServeHTTP handles plugin request forwarding
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

	// Strip the prefix so the plugin sees /... instead of /api/plugins/<name>/...
	// The router should have already handled the matching, but we need to strip the prefix for the proxy
	// Assuming the route is /api/plugins/<name>/*path
	prefix := "/api/plugins/" + pluginName
	http.StripPrefix(prefix, plugin.Proxy).ServeHTTP(w, r)
}

// ListPlugins returns a list of loaded plugins
func (m *Manager) ListPlugins() []*Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]*Plugin, 0, len(m.plugins))
	for _, p := range m.plugins {
		cp := *p
		list = append(list, &cp)
	}
	return list
}

// ListPluginsForRole returns plugins filtered for a given role.
// Non-admin users will only see non-adminOnly plugins.
func (m *Manager) ListPluginsForRole(role string) []*Plugin {
	plugins := m.ListPlugins()
	if role == "admin" {
		return plugins
	}
	filtered := make([]*Plugin, 0, len(plugins))
	for _, p := range plugins {
		if p.AdminOnly {
			continue
		}
		filtered = append(filtered, p)
	}
	return filtered
}

// TogglePlugin enables or disables a plugin
func (m *Manager) TogglePlugin(name string, enabled bool) error {
	// Validate plugin exists and get current state.
	m.mu.RLock()
	plugin, exists := m.plugins[name]
	if !exists {
		m.mu.RUnlock()
		return fmt.Errorf("plugin not found")
	}
	currentEnabled := plugin.Enabled
	m.mu.RUnlock()

	// No-op
	if currentEnabled == enabled {
		return nil
	}

	if enabled {
		// Start first; only mark enabled after a successful start.
		if err := m.StartPlugin(name); err != nil {
			return err
		}
		m.mu.Lock()
		if p, ok := m.plugins[name]; ok {
			p.Enabled = true
		}
		m.mu.Unlock()
	} else {
		// Disable immediately to block access, then stop.
		m.mu.Lock()
		if p, ok := m.plugins[name]; ok {
			p.Enabled = false
		}
		m.mu.Unlock()
		if err := m.StopPlugin(name); err != nil {
			return err
		}
	}

	if err := saveEnabledState(m.currentEnabledState()); err != nil {
		return err
	}
	return nil
}

func (m *Manager) currentEnabledState() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state := make(map[string]bool, len(m.plugins))
	for name, p := range m.plugins {
		state[name] = p.Enabled
	}
	return state
}

func (m *Manager) Cleanup() {
	// Stop all running plugins best-effort.
	for _, p := range m.ListPlugins() {
		_ = m.StopPlugin(p.Name)
	}
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

const enabledStatePath = "/data/plugins.json"

func loadEnabledState() (map[string]bool, error) {
	b, err := os.ReadFile(enabledStatePath)
	if err != nil {
		return nil, err
	}
	var state map[string]bool
	if err := json.Unmarshal(b, &state); err != nil {
		return nil, err
	}
	return state, nil
}

func saveEnabledState(state map[string]bool) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	// Best-effort: ensure directory exists
	_ = os.MkdirAll(filepath.Dir(enabledStatePath), 0755)
	return os.WriteFile(enabledStatePath, data, 0666)
}
