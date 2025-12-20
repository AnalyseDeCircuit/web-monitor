package plugin

import "fmt"

// PluginType defines the plugin security level
type PluginType string

const (
	PluginTypeNormal     PluginType = "normal"
	PluginTypePrivileged PluginType = "privileged"
)

// PluginState represents the current state of a plugin
type PluginState string

const (
	StateAvailable PluginState = "available" // Can be installed
	StatePending   PluginState = "pending"   // Awaiting approval (privileged)
	StateInstalled PluginState = "installed" // Container created, not enabled
	StateEnabled   PluginState = "enabled"   // Enabled, proxy registered
	StateRunning   PluginState = "running"   // Container running
	StateDisabled  PluginState = "disabled"  // Disabled by user
	StateStopped   PluginState = "stopped"   // Container stopped
	StateError     PluginState = "error"     // Error state
)

// Manifest represents a plugin's metadata and configuration
type Manifest struct {
	// Basic info
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	Author      string `json:"author,omitempty"`

	// Classification
	Type      PluginType `json:"type"`                // "normal" or "privileged"
	Risk      string     `json:"risk,omitempty"`      // "low", "medium", "high"
	AdminOnly bool       `json:"adminOnly,omitempty"` // Requires admin role

	// Container configuration
	Container ContainerConfig `json:"container"`

	// UI integration
	UI *UIConfig `json:"ui,omitempty"`

	// Permissions required (for privileged plugins)
	Permissions []string `json:"permissions,omitempty"`

	// Lifecycle hooks (for privileged plugins)
	Install   *InstallConfig   `json:"install,omitempty"`
	Uninstall *UninstallConfig `json:"uninstall,omitempty"`
}

// ContainerConfig defines how to run the plugin container
type ContainerConfig struct {
	// Image reference (e.g., "ghcr.io/user/plugin:v1.0.0" or local "web-monitor-plugin-xxx:latest")
	Image string `json:"image"`

	// Internal port the plugin listens on
	Port int `json:"port"`

	// Optional: explicit container name (default: web-monitor-plugin-{name})
	ContainerName string `json:"containerName,omitempty"`

	// Environment variables
	Env map[string]string `json:"env,omitempty"`

	// Volume mounts
	Volumes []VolumeMount `json:"volumes,omitempty"`

	// Resource limits
	Resources *ResourceLimits `json:"resources,omitempty"`

	// Security options (for privileged plugins)
	Security *SecurityConfig `json:"security,omitempty"`

	// Network mode (default: bridge with internal access only)
	Network string `json:"network,omitempty"`

	// Host port binding (default: auto-assign from 38100-38199 range)
	HostPort int `json:"hostPort,omitempty"`
}

// VolumeMount defines a container volume mount
type VolumeMount struct {
	Type     string `json:"type"`   // "bind" or "volume"
	Source   string `json:"source"` // Host path or volume name
	Target   string `json:"target"` // Container path
	ReadOnly bool   `json:"readonly,omitempty"`
}

// ResourceLimits defines container resource constraints
type ResourceLimits struct {
	Memory string `json:"memory,omitempty"` // e.g., "128m", "1g"
	CPU    string `json:"cpu,omitempty"`    // e.g., "0.5", "2"
}

// SecurityConfig defines container security settings
type SecurityConfig struct {
	CapAdd          []string `json:"capAdd,omitempty"`
	CapDrop         []string `json:"capDrop,omitempty"`
	ReadOnlyRootfs  bool     `json:"readOnlyRootfs,omitempty"`
	NoNewPrivileges bool     `json:"noNewPrivileges,omitempty"`
	SecurityOpt     []string `json:"securityOpt,omitempty"`
	Privileged      bool     `json:"privileged,omitempty"` // Strongly discouraged
}

// UIConfig defines how the plugin appears in the web UI
type UIConfig struct {
	Entry    string `json:"entry,omitempty"`    // Entry path (default: "/")
	Icon     string `json:"icon,omitempty"`     // FontAwesome icon name
	NavTitle string `json:"navTitle,omitempty"` // Navigation menu title
}

// InstallConfig defines installation behavior for privileged plugins
type InstallConfig struct {
	RequiresApproval bool          `json:"requiresApproval,omitempty"`
	Hooks            []InstallHook `json:"hooks,omitempty"`
}

// InstallHook represents a declarative installation action
type InstallHook struct {
	Type string `json:"type"` // Hook type

	// Hook-specific parameters (varies by type)
	User      string `json:"user,omitempty"`
	Shell     string `json:"shell,omitempty"`
	Algorithm string `json:"algorithm,omitempty"`
	Path      string `json:"path,omitempty"`
	KeyPath   string `json:"keyPath,omitempty"`
	Content   string `json:"content,omitempty"`
	Mode      string `json:"mode,omitempty"`
	Command   string `json:"command,omitempty"` // Only for whitelisted commands
}

// Supported hook types
const (
	HookEnsureUser          = "ensure-user"
	HookGenerateSSHKey      = "generate-ssh-key"
	HookAuthorizeKey        = "authorize-key"
	HookRemoveAuthorizedKey = "remove-authorized-key"
	HookCreateDirectory     = "create-directory"
	HookWriteConfig         = "write-config"
	HookRemoveFile          = "remove-file"
)

// UninstallConfig defines uninstallation behavior
type UninstallConfig struct {
	Hooks []InstallHook `json:"hooks,omitempty"`
}

// DefaultManifest returns a manifest with sensible defaults for normal plugins
func DefaultManifest(name string) *Manifest {
	return &Manifest{
		Name:    name,
		Version: "1.0.0",
		Type:    PluginTypeNormal,
		Risk:    "low",
		Container: ContainerConfig{
			Port: 8080,
			Resources: &ResourceLimits{
				Memory: "128m",
				CPU:    "0.5",
			},
		},
	}
}

// ContainerNameOrDefault returns the container name, or generates a default
func (m *Manifest) ContainerNameOrDefault() string {
	if m.Container.ContainerName != "" {
		return m.Container.ContainerName
	}
	return "web-monitor-plugin-" + m.Name
}

// BaseURL returns the plugin's base URL for proxying
func (m *Manifest) BaseURL(hostPort int) string {
	port := hostPort
	if port == 0 {
		port = m.Container.HostPort
	}
	if port == 0 {
		port = 38100 // fallback
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

// Validate checks if the manifest is valid
func (m *Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("plugin name is required")
	}
	if m.Container.Image == "" {
		return fmt.Errorf("container image is required")
	}
	if m.Container.Port <= 0 || m.Container.Port > 65535 {
		return fmt.Errorf("invalid container port: %d", m.Container.Port)
	}
	if m.Type == "" {
		m.Type = PluginTypeNormal
	}
	if m.Type != PluginTypeNormal && m.Type != PluginTypePrivileged {
		return fmt.Errorf("invalid plugin type: %s", m.Type)
	}
	// Privileged plugins must declare permissions
	if m.Type == PluginTypePrivileged && len(m.Permissions) == 0 {
		return fmt.Errorf("privileged plugins must declare permissions")
	}
	return nil
}

// IsPrivileged returns true if this is a privileged plugin
func (m *Manifest) IsPrivileged() bool {
	return m.Type == PluginTypePrivileged
}

// RequiresApproval returns true if installation requires admin approval
func (m *Manifest) RequiresApproval() bool {
	return m.IsPrivileged() && m.Install != nil && m.Install.RequiresApproval
}
