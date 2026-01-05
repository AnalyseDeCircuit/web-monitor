// Package plugin provides the main plugin management interface.
// This is the v2 plugin system with modular architecture.
package plugin

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AnalyseDeCircuit/opskernel/internal/auth"
	"github.com/AnalyseDeCircuit/opskernel/internal/plugin/gateway"
	"github.com/AnalyseDeCircuit/opskernel/internal/plugin/policy"
	"github.com/AnalyseDeCircuit/opskernel/internal/plugin/registry"
	"github.com/AnalyseDeCircuit/opskernel/internal/plugin/runtime"
	"github.com/AnalyseDeCircuit/opskernel/internal/plugin/store"
)

// ============================================================================
// Per-Plugin Mutex - Prevents concurrent operations on the same plugin
// ============================================================================

// pluginLocks provides per-plugin mutual exclusion for state-changing operations.
// This prevents race conditions like double-create or double-remove.
type pluginLocks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newPluginLocks() *pluginLocks {
	return &pluginLocks{
		locks: make(map[string]*sync.Mutex),
	}
}

// Lock acquires the lock for a specific plugin
func (p *pluginLocks) Lock(name string) {
	p.mu.Lock()
	lock, ok := p.locks[name]
	if !ok {
		lock = &sync.Mutex{}
		p.locks[name] = lock
	}
	p.mu.Unlock()
	lock.Lock()
}

// Unlock releases the lock for a specific plugin
func (p *pluginLocks) Unlock(name string) {
	p.mu.Lock()
	lock, ok := p.locks[name]
	p.mu.Unlock()
	if ok {
		lock.Unlock()
	}
}

// ============================================================================
// ManagerV2 - Main Plugin Manager
// ============================================================================

// ManagerV2 is the new modular plugin manager.
// It coordinates registry, store, policy, runtime, and gateway modules.
// The core NEVER executes plugin code or host commands - it only schedules containers.
type ManagerV2 struct {
	registry *registry.Registry
	store    *store.Store
	policy   *policy.Policy
	runtime  *runtime.Runtime
	gateway  *gateway.Gateway

	pluginsDir  string
	mu          sync.RWMutex
	pluginLocks *pluginLocks // Per-plugin mutex for state-changing operations
}

// Config holds configuration for the plugin manager
type Config struct {
	PluginsDir string // Directory containing plugin manifests
	DataDir    string // Directory for state persistence

	// Policy options
	RequireConfirmation bool
	AllowCriticalRisk   bool
	AllowPrivileged     bool
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		PluginsDir:          "/app/plugins",
		DataDir:             "/data",
		RequireConfirmation: true,
		AllowCriticalRisk:   true,
		AllowPrivileged:     true,
	}
}

// NewManagerV2 creates a new modular plugin manager
func NewManagerV2(cfg *Config) *ManagerV2 {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Initialize modules
	reg := registry.NewRegistry(cfg.PluginsDir)

	statePath := cfg.DataDir + "/plugins-state.json"
	st := store.NewStore(statePath)

	pol := &policy.Policy{
		RequireConfirmation: cfg.RequireConfirmation,
		AllowCriticalRisk:   cfg.AllowCriticalRisk,
		AllowPrivileged:     cfg.AllowPrivileged,
	}

	dockerClient := runtime.NewDefaultDockerClient()
	rt := runtime.NewRuntime(dockerClient)

	gw := gateway.NewGateway(reg, rt, st)

	return &ManagerV2{
		registry:    reg,
		store:       st,
		policy:      pol,
		runtime:     rt,
		gateway:     gw,
		pluginsDir:  cfg.PluginsDir,
		pluginLocks: newPluginLocks(),
	}
}

// LoadPlugins discovers and loads all plugins
func (m *ManagerV2) LoadPlugins(pluginDir string) error {
	if pluginDir != "" {
		m.pluginsDir = pluginDir
	}

	fmt.Printf("Plugin system v2: scanning %s\n", m.pluginsDir)

	// Update registry path
	m.registry = registry.NewRegistry(m.pluginsDir)

	// 1. Discover manifests
	if err := m.registry.Discover(); err != nil {
		return fmt.Errorf("failed to discover plugins: %w", err)
	}

	// 2. Load persisted state
	if err := m.store.Load(); err != nil {
		fmt.Printf("Warning: failed to load plugin state: %v\n", err)
	}

	// Try to migrate from legacy enabled.json
	legacyPath := "/data/plugins-enabled.json"
	if err := m.store.MigrateFromLegacy(legacyPath); err != nil {
		fmt.Printf("Warning: failed to migrate legacy state: %v\n", err)
	}

	// 3. Sync runtime state with Docker
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := m.runtime.SyncState(ctx); err != nil {
		fmt.Printf("Warning: failed to sync container states: %v\n", err)
	}

	// 4. Start enabled plugins
	m.startEnabledPlugins(ctx)

	// Start background reconciler (conservative default: 30s)
	// It will attempt to start expected-but-stopped plugins and stop containers
	// for plugins that are explicitly disabled in the store.
	go m.StartReconciler(30 * time.Second)

	// Update gateway with new registry
	m.gateway = gateway.NewGateway(m.registry, m.runtime, m.store)

	fmt.Printf("Plugin system v2: loaded %d plugins\n", m.registry.Count())
	return nil
}

// RefreshRegistry implements passive reload: re-scans the plugins directory
// and updates the registry. This should be called before ListPlugins when
// the caller wants to see newly added/modified plugins.
func (m *ManagerV2) RefreshRegistry() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Re-create registry and discover
	m.registry = registry.NewRegistry(m.pluginsDir)
	if err := m.registry.Discover(); err != nil {
		return fmt.Errorf("failed to refresh plugin registry: %w", err)
	}

	// Update gateway with new registry
	m.gateway = gateway.NewGateway(m.registry, m.runtime, m.store)

	return nil
}

// startEnabledPlugins starts all plugins that are enabled
func (m *ManagerV2) startEnabledPlugins(ctx context.Context) {
	enabled := m.store.GetEnabledPlugins()
	for _, name := range enabled {
		manifest, exists := m.registry.Get(name)
		if !exists {
			continue
		}

		// Check if already running
		if m.runtime.IsRunning(name) {
			continue
		}

		// Start the plugin
		extraEnv := m.getPluginEnv(name)
		if _, err := m.runtime.CreateAndStart(ctx, manifest, extraEnv); err != nil {
			fmt.Printf("Failed to start plugin %s: %v\n", name, err)
			m.store.SetError(name, err.Error())
		} else {
			fmt.Printf("Started plugin: %s\n", name)
			m.store.ClearError(name)
		}
	}
}

// ============================================================================
// Plugin Queries
// ============================================================================

// PluginInfo represents plugin information for the API
type PluginInfo struct {
	Name        string                `json:"name"`
	Version     string                `json:"version"`
	Description string                `json:"description,omitempty"`
	Author      string                `json:"author,omitempty"`
	Risk        registry.RiskLevel    `json:"risk"`
	Permissions []registry.Permission `json:"permissions,omitempty"`
	AdminOnly   bool                  `json:"adminOnly"`

	// State
	Enabled   bool                   `json:"enabled"`
	Confirmed bool                   `json:"confirmed"`
	Running   bool                   `json:"running"`
	State     runtime.ContainerState `json:"state"`
	Error     string                 `json:"error,omitempty"`

	// UI
	Icon     string `json:"icon,omitempty"`
	NavTitle string `json:"navTitle,omitempty"`
	ProxyURL string `json:"proxyUrl,omitempty"`

	// Deprecation warning
	DeprecationWarning string `json:"deprecationWarning,omitempty"`
}

// GetPlugin returns information about a plugin
func (m *ManagerV2) GetPlugin(name string) (*PluginInfo, bool) {
	manifest, exists := m.registry.Get(name)
	if !exists {
		return nil, false
	}

	return m.buildPluginInfo(manifest), true
}

// ListPlugins returns all plugins
func (m *ManagerV2) ListPlugins() []*PluginInfo {
	manifests := m.registry.List()
	list := make([]*PluginInfo, 0, len(manifests))

	for _, manifest := range manifests {
		list = append(list, m.buildPluginInfo(manifest))
	}

	// Sort by name
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})

	return list
}

// ListPluginsForRole returns plugins visible to a specific role
func (m *ManagerV2) ListPluginsForRole(role string) []*PluginInfo {
	all := m.ListPlugins()
	if role == "admin" {
		return all
	}

	filtered := make([]*PluginInfo, 0)
	for _, p := range all {
		if !p.AdminOnly {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func (m *ManagerV2) buildPluginInfo(manifest *registry.Manifest) *PluginInfo {
	state := m.store.Get(manifest.Name)
	instance, _ := m.runtime.GetInstance(manifest.Name)

	info := &PluginInfo{
		Name:        manifest.Name,
		Version:     manifest.Version,
		Description: manifest.Description,
		Author:      manifest.Author,
		Risk:        manifest.Risk,
		Permissions: manifest.Permissions,
		AdminOnly:   manifest.AdminOnly,
		Enabled:     state.Enabled,
		Confirmed:   state.Confirmed,
		Error:       state.LastError,
	}

	if instance != nil {
		info.Running = instance.State == runtime.StateRunning
		info.State = instance.State
		if instance.Error != "" {
			info.Error = instance.Error
		}
	}

	if manifest.UI != nil {
		info.Icon = manifest.UI.Icon
		info.NavTitle = manifest.UI.Title
	}

	if state.Enabled {
		base := fmt.Sprintf("/plugins/%s", manifest.Name)
		if manifest.UI != nil && manifest.UI.Path != "" {
			// Handle path that might start with /
			path := manifest.UI.Path
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}
			info.ProxyURL = base + path
		} else {
			info.ProxyURL = base + "/"
		}
	}

	// Check for deprecation warning
	if m.registry.IsDeprecatedFormat(manifest.Name) {
		info.DeprecationWarning = "This plugin uses deprecated plugin.json format. Please migrate to manifest.json"
	}

	return info
}

// ============================================================================
// Plugin Operations
// ============================================================================

// EnableRequest represents a request to enable a plugin
type EnableRequest struct {
	Name     string `json:"name"`
	Username string `json:"username"`
	Role     string `json:"role"`

	// Confirmation (required for first-time enable)
	Confirmation *store.Confirmation `json:"confirmation,omitempty"`
}

// EnableResult represents the result of an enable operation
type EnableResult struct {
	Success              bool                       `json:"success"`
	Message              string                     `json:"message,omitempty"`
	RequiresConfirmation bool                       `json:"requiresConfirmation,omitempty"`
	ConfirmationPrompt   *policy.ConfirmationPrompt `json:"confirmationPrompt,omitempty"`
}

// EnablePlugin enables a plugin
// Uses per-plugin lock to prevent concurrent enable operations
func (m *ManagerV2) EnablePlugin(ctx context.Context, req *EnableRequest) (*EnableResult, error) {
	// Acquire per-plugin lock to prevent concurrent operations
	m.pluginLocks.Lock(req.Name)
	defer m.pluginLocks.Unlock(req.Name)

	manifest, exists := m.registry.Get(req.Name)
	if !exists {
		return nil, fmt.Errorf("plugin not found: %s", req.Name)
	}

	state := m.store.Get(req.Name)

	// Check policy
	checkReq := &policy.CheckEnableRequest{
		PluginName:   req.Name,
		UserRole:     req.Role,
		Username:     req.Username,
		Confirmation: req.Confirmation,
	}

	result := m.policy.CheckEnable(manifest, state, checkReq)
	if !result.Allowed {
		if result.RequiresConfirmation {
			return &EnableResult{
				Success:              false,
				Message:              result.Reason,
				RequiresConfirmation: true,
				ConfirmationPrompt:   result.ConfirmationPrompt,
			}, nil
		}
		return nil, fmt.Errorf("policy denied: %s", result.Reason)
	}

	// Save confirmation if provided
	if req.Confirmation != nil && req.Confirmation.ExplicitApproval {
		if err := m.store.SetConfirmed(req.Name, req.Confirmation); err != nil {
			return nil, fmt.Errorf("failed to save confirmation: %w", err)
		}
	}

	// Create and start container (idempotent - handles existing containers)
	extraEnv := m.getPluginEnv(req.Name)
	_, err := m.runtime.CreateAndStart(ctx, manifest, extraEnv)
	if err != nil {
		m.store.SetError(req.Name, err.Error())
		return nil, fmt.Errorf("failed to start plugin: %w", err)
	}

	// Update state
	if err := m.store.SetEnabled(req.Name, true); err != nil {
		return nil, fmt.Errorf("failed to save state: %w", err)
	}
	m.store.ClearError(req.Name)

	return &EnableResult{
		Success: true,
		Message: fmt.Sprintf("Plugin %s enabled successfully", req.Name),
	}, nil
}

// DisablePlugin disables a plugin
// Uses per-plugin lock to prevent concurrent disable operations
func (m *ManagerV2) DisablePlugin(ctx context.Context, name, username, role string) error {
	// Acquire per-plugin lock to prevent concurrent operations
	m.pluginLocks.Lock(name)
	defer m.pluginLocks.Unlock(name)

	manifest, exists := m.registry.Get(name)
	if !exists {
		return fmt.Errorf("plugin not found: %s", name)
	}

	state := m.store.Get(name)

	// Check policy
	result := m.policy.CheckDisable(manifest, state, &policy.CheckEnableRequest{
		PluginName: name,
		UserRole:   role,
		Username:   username,
	})
	if !result.Allowed {
		return fmt.Errorf("policy denied: %s", result.Reason)
	}

	// Stop container (idempotent - ignores if not running)
	if err := m.runtime.Stop(ctx, name); err != nil {
		return fmt.Errorf("failed to stop plugin: %w", err)
	}

	// Update state
	if err := m.store.SetEnabled(name, false); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	return nil
}

// TogglePlugin enables or disables a plugin (legacy API compatibility)
// Uses per-plugin lock to prevent concurrent toggle operations
func (m *ManagerV2) TogglePlugin(name string, enabled bool) error {
	// Use a reasonable timeout for legacy API
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if enabled {
		// Get manifest to auto-acknowledge all permissions
		manifest, exists := m.registry.Get(name)
		if !exists {
			return fmt.Errorf("plugin not found: %s", name)
		}

		// Build permissions list
		perms := make([]string, 0, len(manifest.Permissions))
		for _, p := range manifest.Permissions {
			perms = append(perms, string(p))
		}

		_, err := m.EnablePlugin(ctx, &EnableRequest{
			Name:     name,
			Username: "system",
			Role:     "admin",
			// For legacy API, auto-confirm with all permissions acknowledged
			Confirmation: &store.Confirmation{
				PluginName:              name,
				Username:                "system",
				AcknowledgedRisk:        string(manifest.Risk),
				AcknowledgedPermissions: perms,
				ExplicitApproval:        true,
				Timestamp:               time.Now(),
			},
		})
		return err
	}

	return m.DisablePlugin(ctx, name, "system", "admin")
}

// StartPlugin starts a plugin (legacy API compatibility)
func (m *ManagerV2) StartPlugin(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return m.runtime.Start(ctx, name)
}

// StopPlugin stops a plugin (legacy API compatibility)
func (m *ManagerV2) StopPlugin(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return m.runtime.Stop(ctx, name)
}

// UninstallPlugin removes a plugin container
// Uses per-plugin lock to prevent concurrent uninstall operations
func (m *ManagerV2) UninstallPlugin(name string, removeData bool) (*InstallResultV2, error) {
	// Acquire per-plugin lock to prevent concurrent operations
	m.pluginLocks.Lock(name)
	defer m.pluginLocks.Unlock(name)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Stop first if running (idempotent)
	_ = m.runtime.Stop(ctx, name)

	// Remove container (idempotent)
	if err := m.runtime.Remove(ctx, name, true); err != nil {
		// Only fail for non-transient errors
		if !runtime.IsNotFoundError(err) && !runtime.IsTemporaryError(err) {
			return &InstallResultV2{
				Success: false,
				Errors:  []string{err.Error()},
			}, nil
		}
		// Log but continue for transient errors
		fmt.Printf("Warning: failed to remove container for %s: %v\n", name, err)
	}

	// Update state
	m.store.SetEnabled(name, false)

	// Note: removeData is acknowledged but conservative default is to NOT delete data volumes.
	// Future enhancement: implement manifest-declared cleanable resources.
	// For now, only container and anonymous volumes are removed (Docker's v=1 flag).

	return &InstallResultV2{
		Success: true,
		Message: fmt.Sprintf("Plugin %s uninstalled successfully", name),
	}, nil
}

// InstallResultV2 represents the result of an install/uninstall operation (v2)
type InstallResultV2 struct {
	Success bool     `json:"success"`
	Message string   `json:"message,omitempty"`
	Errors  []string `json:"errors,omitempty"`
}

// InstallPlugin is a no-op in v2 (containers are created on enable)
// This exists for legacy API compatibility
func (m *ManagerV2) InstallPlugin(name string) (*InstallResultV2, error) {
	// In v2, installation happens on enable
	// This method exists for API compatibility
	return &InstallResultV2{
		Success: true,
		Message: "Plugin is ready. Use enable to start the container.",
	}, nil
}

// GetManifest returns a plugin's manifest (legacy API compatibility)
func (m *ManagerV2) GetManifest(name string) (*registry.Manifest, bool) {
	return m.registry.Get(name)
}

// ============================================================================
// HTTP Proxy
// ============================================================================

// ServeHTTP proxies requests to a plugin (new path: /plugins/{name}/...)
func (m *ManagerV2) ServeHTTP(w http.ResponseWriter, r *http.Request, pluginName string) {
	// Check plugin exists and is enabled
	state := m.store.Get(pluginName)
	if !state.Enabled {
		http.Error(w, "Plugin is disabled", http.StatusForbidden)
		return
	}

	if !m.runtime.IsRunning(pluginName) {
		http.Error(w, "Plugin is not running", http.StatusServiceUnavailable)
		return
	}

	proxy := m.runtime.GetProxy(pluginName)
	if proxy == nil {
		http.Error(w, "Plugin proxy not available", http.StatusServiceUnavailable)
		return
	}

	// Strip prefix and proxy
	// Support both /plugins/{name}/... and /api/plugins/{name}/... for backward compatibility
	var prefix string
	if strings.HasPrefix(r.URL.Path, "/plugins/") {
		prefix = "/plugins/" + pluginName
	} else {
		prefix = "/api/plugins/" + pluginName
	}
	http.StripPrefix(prefix, proxy).ServeHTTP(w, r)
}

// Gateway returns the gateway for direct access
func (m *ManagerV2) Gateway() *gateway.Gateway {
	return m.gateway
}

// getPluginEnv generates extra environment variables for a plugin
func (m *ManagerV2) getPluginEnv(name string) map[string]string {
	env := make(map[string]string)

	// Generate a JWT for the plugin to access the core API
	// We use a special username "plugin-<name>" and role "admin"
	token, err := auth.GenerateJWT("plugin-"+name, "admin")
	if err == nil {
		env["MONITOR_TOKEN"] = token
	}

	return env
}

// StartReconciler starts a background loop that periodically calls ReconcileOnce.
// The interval is conservative; choose larger if your environment is heavy.
func (m *ManagerV2) StartReconciler(interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		// Run reconcile with timeout to avoid hanging
		ctx, cancel := context.WithTimeout(context.Background(), interval/2)
		_ = m.ReconcileOnce(ctx)
		cancel()
	}
}

// ReconcileOnce performs one reconciliation between desired state (Store)
// and actual runtime state. Conservative actions only:
//   - If store says Enabled and runtime not running -> try to start (CreateAndStart)
//   - If store says Disabled and runtime running -> stop the container
//
// Returns error if reconciliation encountered persistent failures.
func (m *ManagerV2) ReconcileOnce(ctx context.Context) error {
	expected := m.store.GetExpectedRunningPlugins()

	// Build a set for quick lookup
	expectedSet := make(map[string]struct{}, len(expected))
	for _, n := range expected {
		expectedSet[n] = struct{}{}
	}

	// 1) Ensure expected plugins are running
	for _, name := range expected {
		// Acquire per-plugin lock to avoid races with manual operations
		m.pluginLocks.Lock(name)
		func() {
			defer m.pluginLocks.Unlock(name)

			if m.runtime.IsRunning(name) {
				return
			}

			manifest, ok := m.registry.Get(name)
			if !ok {
				// Nothing to do if manifest missing
				return
			}

			extraEnv := m.getPluginEnv(name)
			if _, err := m.runtime.CreateAndStart(ctx, manifest, extraEnv); err != nil {
				// Record reconcile error but continue with others
				m.store.RecordReconcileError(name, err)
				fmt.Printf("Reconcile: failed to start plugin %s: %v\n", name, err)
			} else {
				m.store.RecordReconcileSuccess(name)
			}
		}()
	}

	// 2) Stop containers that are running but should not be
	instances := m.runtime.ListInstances()
	for _, inst := range instances {
		if inst == nil {
			continue
		}
		name := inst.Name
		if _, shouldRun := expectedSet[name]; !shouldRun {
			// plugin not expected to run -> stop it (acquire lock)
			m.pluginLocks.Lock(name)
			func() {
				defer m.pluginLocks.Unlock(name)
				// Double-check store state
				if m.store.IsEnabled(name) {
					return
				}
				// Stop is idempotent
				if err := m.runtime.Stop(ctx, name); err != nil {
					m.store.RecordReconcileError(name, err)
					fmt.Printf("Reconcile: failed to stop plugin %s: %v\n", name, err)
				} else {
					m.store.RecordReconcileSuccess(name)
				}
			}()
		}
	}

	return nil
}

// ============================================================================
// Cleanup
// ============================================================================

// Cleanup stops all running plugins
func (m *ManagerV2) Cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, manifest := range m.registry.List() {
		if m.runtime.IsRunning(manifest.Name) {
			_ = m.runtime.Stop(ctx, manifest.Name)
		}
	}
}
