// Package registry defines the v1 plugin manifest schema and helpers.
package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ManifestVersion is the current manifest schema version.
const ManifestVersion = "1"

// RiskLevel defines the risk classification of a plugin.
type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

// Permission defines a capability the plugin requires.
type Permission string

// Host-level permissions.
const (
	PermHostNetwork   Permission = "host:network"   // Access host network namespace (e.g. --network=host)
	PermHostPID       Permission = "host:pid"       // Access host PID namespace
	PermHostIPC       Permission = "host:ipc"       // Access host IPC namespace
	PermHostMount     Permission = "host:mount"     // Mount host paths
	PermHostPrivilege Permission = "host:privilege" // Run with --privileged

	// Docker permissions.
	PermDockerSocket Permission = "docker:socket" // Access Docker socket

	// Data permissions.
	PermDataRead  Permission = "data:read"  // Read application data
	PermDataWrite Permission = "data:write" // Write application data

	// Network permissions.
	PermNetLocal    Permission = "net:local"    // Local network access only
	PermNetInternet Permission = "net:internet" // Outbound internet access

	// File permissions.
	PermFileSystem Permission = "fs:write" // Write to container filesystem

	// User permissions.
	PermUserRoot Permission = "user:root" // Run as root inside container
)

// VolumeMount defines a container volume or bind mount.
type VolumeMount struct {
	Type     string `json:"type"`               // "bind" or "volume"
	Source   string `json:"source"`             // Host path or volume name
	Target   string `json:"target"`             // Container path
	ReadOnly bool   `json:"readOnly,omitempty"` // Read-only mount
}

// DeviceMapping defines a device to map into the container.
type DeviceMapping struct {
	Host        string `json:"host"`                  // Host device path
	Container   string `json:"container,omitempty"`   // Container device path (defaults to same as host)
	Permissions string `json:"permissions,omitempty"` // rwm
}

// ResourceLimits defines container resource constraints.
type ResourceLimits struct {
	Memory            string `json:"memory,omitempty"`            // e.g. "128m", "1g"
	MemoryReservation string `json:"memoryReservation,omitempty"` // soft limit
	CPUs              string `json:"cpus,omitempty"`              // e.g. "0.5", "2"
	CPUShares         int64  `json:"cpuShares,omitempty"`
	PidsLimit         int64  `json:"pidsLimit,omitempty"`
}

// SecurityConfig defines container security settings.
type SecurityConfig struct {
	CapAdd          []string `json:"capAdd,omitempty"`
	CapDrop         []string `json:"capDrop,omitempty"`
	ReadOnlyRootfs  bool     `json:"readOnlyRootfs,omitempty"`
	NoNewPrivileges bool     `json:"noNewPrivileges,omitempty"`
	SecurityOpt     []string `json:"securityOpt,omitempty"`
	Privileged      bool     `json:"privileged,omitempty"`
	User            string   `json:"user,omitempty"` // UID:GID or username
}

// DockerConfig defines how to run the plugin container.
// OpsKernel will use these parameters to create/start the container dynamically.
type DockerConfig struct {
	Image         string            `json:"image"`                   // Required
	Port          int               `json:"port"`                    // Internal port plugin listens on
	ContainerName string            `json:"containerName,omitempty"` // Optional override (default: opskernel-plugin-{name})
	Env           map[string]string `json:"env,omitempty"`
	Volumes       []VolumeMount     `json:"volumes,omitempty"`
	Devices       []DeviceMapping   `json:"devices,omitempty"`
	Network       string            `json:"network,omitempty"` // "bridge", "host", "none" or custom
	Resources     *ResourceLimits   `json:"resources,omitempty"`
	Security      *SecurityConfig   `json:"security,omitempty"`
	ExtraHosts    []string          `json:"extraHosts,omitempty"`
	WorkingDir    string            `json:"workingDir,omitempty"`
	Entrypoint    []string          `json:"entrypoint,omitempty"`
	Command       []string          `json:"command,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	RestartPolicy string            `json:"restartPolicy,omitempty"` // e.g. "unless-stopped"
}

// UIConfig defines how the plugin appears in the web UI.
type UIConfig struct {
	Path      string `json:"path,omitempty"`      // Entry path (default: "/")
	Icon      string `json:"icon,omitempty"`      // e.g. "terminal", "database"
	Title     string `json:"title,omitempty"`     // Navigation title (default: plugin name)
	ShowInNav bool   `json:"showInNav,omitempty"` // Whether to show in sidebar
	Sandbox   string `json:"sandbox,omitempty"`   // Iframe sandbox attributes
}

// HealthCheckConfig defines how to check if the plugin is healthy.
type HealthCheckConfig struct {
	Path        string `json:"path,omitempty"`        // e.g. "/health" or "/"
	StatusCode  int    `json:"statusCode,omitempty"`  // Expected status code (default 200)
	Interval    string `json:"interval,omitempty"`    // e.g. "30s"
	Timeout     string `json:"timeout,omitempty"`     // e.g. "5s"
	Retries     int    `json:"retries,omitempty"`     // Retries before unhealthy
	StartPeriod string `json:"startPeriod,omitempty"` // Startup grace period
}

// Manifest represents the plugin manifest.json v1 schema.
// This is the single source of truth for plugin configuration.
type Manifest struct {
	ManifestVersion string             `json:"manifestVersion"`
	Name            string             `json:"name"`
	Version         string             `json:"version"`
	Description     string             `json:"description,omitempty"`
	Author          string             `json:"author,omitempty"`
	License         string             `json:"license,omitempty"`
	Homepage        string             `json:"homepage,omitempty"`
	Risk            RiskLevel          `json:"risk"` // Required
	Permissions     []Permission       `json:"permissions,omitempty"`
	AdminOnly       bool               `json:"adminOnly,omitempty"`
	Docker          DockerConfig       `json:"docker"` // Required
	UI              *UIConfig          `json:"ui,omitempty"`
	HealthCheck     *HealthCheckConfig `json:"healthCheck,omitempty"`
	Tags            []string           `json:"tags,omitempty"`
	Category        string             `json:"category,omitempty"`
}

// namePattern validates plugin names: lowercase, alnum + hyphen, start with letter.
var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`)

// Validate checks if the manifest is structurally valid.
func (m *Manifest) Validate() error {
	// Schema version
	if m.ManifestVersion != "" && m.ManifestVersion != ManifestVersion {
		return fmt.Errorf("unsupported manifest version: %s (expected %s)", m.ManifestVersion, ManifestVersion)
	}

	// Name
	if m.Name == "" {
		return fmt.Errorf("plugin name is required")
	}
	if len(m.Name) < 2 || len(m.Name) > 64 {
		return fmt.Errorf("plugin name must be 2-64 characters")
	}
	if !namePattern.MatchString(m.Name) {
		return fmt.Errorf("plugin name must be lowercase alphanumeric with hyphens, start with letter")
	}

	// Version
	if m.Version == "" {
		return fmt.Errorf("plugin version is required")
	}

	// Risk
	switch m.Risk {
	case RiskLow, RiskMedium, RiskHigh, RiskCritical:
		// ok
	case "":
		return fmt.Errorf("risk level is required (low/medium/high/critical)")
	default:
		return fmt.Errorf("invalid risk level: %s", m.Risk)
	}

	// Docker basics
	if m.Docker.Image == "" {
		return fmt.Errorf("docker.image is required")
	}
	if m.Docker.Port <= 0 || m.Docker.Port > 65535 {
		return fmt.Errorf("invalid docker.port: %d (must be 1-65535)", m.Docker.Port)
	}

	// Volumes
	for i, v := range m.Docker.Volumes {
		if v.Type != "bind" && v.Type != "volume" {
			return fmt.Errorf("volume[%d]: type must be 'bind' or 'volume'", i)
		}
		if v.Source == "" {
			return fmt.Errorf("volume[%d]: source is required", i)
		}
		if v.Target == "" {
			return fmt.Errorf("volume[%d]: target is required", i)
		}
	}

	// Permissions consistency.
	if err := m.validatePermissions(); err != nil {
		return err
	}

	return nil
}

// validatePermissions ensures declared permissions match the actual configuration.
func (m *Manifest) validatePermissions() error {
	permSet := make(map[Permission]bool)
	for _, p := range m.Permissions {
		permSet[p] = true
	}

	// Host network requires explicit permission.
	if m.Docker.Network == "host" && !permSet[PermHostNetwork] {
		return fmt.Errorf("host network mode requires 'host:network' permission")
	}

	// Privileged requires explicit permission.
	if m.Docker.Security != nil && m.Docker.Security.Privileged && !permSet[PermHostPrivilege] {
		return fmt.Errorf("privileged container requires 'host:privilege' permission")
	}

	return nil
}

// ContainerNameOrDefault returns the container name, or a generated default.
func (m *Manifest) ContainerNameOrDefault() string {
	if m.Docker.ContainerName != "" {
		return m.Docker.ContainerName
	}
	return "opskernel-plugin-" + m.Name
}

// IsHighRisk returns true if the plugin is high or critical risk.
func (m *Manifest) IsHighRisk() bool {
	return m.Risk == RiskHigh || m.Risk == RiskCritical
}

// RequiresExplicitApproval returns true if the plugin needs explicit user confirmation.
func (m *Manifest) RequiresExplicitApproval() bool {
	// All plugins require approval in the default policy, but
	// high-risk or host-level permissions may be highlighted.
	if m.IsHighRisk() {
		return true
	}
	return m.HasDangerousPermissions()
}

// HasDangerousPermissions returns true if the plugin has any dangerous permissions.
func (m *Manifest) HasDangerousPermissions() bool {
	for _, p := range m.Permissions {
		switch p {
		case PermHostPrivilege, PermHostNetwork, PermHostPID, PermHostIPC, PermDockerSocket:
			return true
		}
	}
	return false
}

// SecuritySummary provides a human-readable summary of plugin security.
type SecuritySummary struct {
	Risk         RiskLevel    `json:"risk"`
	Permissions  []Permission `json:"permissions"`
	AdminOnly    bool         `json:"adminOnly"`
	Warnings     []string     `json:"warnings,omitempty"`
	DockerParams []string     `json:"dockerParams,omitempty"`
}

// getDockerParamsSummary builds a short description of Docker settings.
func (m *Manifest) getDockerParamsSummary() []string {
	var params []string
	params = append(params, fmt.Sprintf("image: %s", m.Docker.Image))
	params = append(params, fmt.Sprintf("port: %d", m.Docker.Port))
	if m.Docker.Network != "" && m.Docker.Network != "bridge" {
		params = append(params, fmt.Sprintf("network: %s", m.Docker.Network))
	}
	if m.Docker.Resources != nil {
		if m.Docker.Resources.Memory != "" {
			params = append(params, fmt.Sprintf("memory: %s", m.Docker.Resources.Memory))
		}
		if m.Docker.Resources.CPUs != "" {
			params = append(params, fmt.Sprintf("cpus: %s", m.Docker.Resources.CPUs))
		}
	}
	if len(m.Docker.Volumes) > 0 {
		params = append(params, fmt.Sprintf("volumes: %d mounts", len(m.Docker.Volumes)))
	}
	if len(m.Docker.Devices) > 0 {
		params = append(params, fmt.Sprintf("devices: %d mapped", len(m.Docker.Devices)))
	}
	return params
}

// GetSecuritySummary returns a human-readable security summary.
func (m *Manifest) GetSecuritySummary() SecuritySummary {
	summary := SecuritySummary{
		Risk:        m.Risk,
		Permissions: m.Permissions,
		AdminOnly:   m.AdminOnly,
	}

	var warnings []string

	if m.Docker.Security != nil {
		if m.Docker.Security.Privileged {
			warnings = append(warnings, "Runs in privileged mode (full host access)")
		}
		if len(m.Docker.Security.CapAdd) > 0 {
			warnings = append(warnings, fmt.Sprintf("Adds capabilities: %v", m.Docker.Security.CapAdd))
		}
	}

	if m.Docker.Network == "host" {
		warnings = append(warnings, "Uses host network (can see all network traffic)")
	}

	for _, v := range m.Docker.Volumes {
		if v.Type == "bind" && !v.ReadOnly {
			warnings = append(warnings, fmt.Sprintf("Writable bind mount: %s", v.Source))
		}
	}

	summary.Warnings = warnings
	summary.DockerParams = m.getDockerParamsSummary()
	return summary
}

// LoadManifest reads and parses a manifest.json file.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if manifest.ManifestVersion == "" {
		manifest.ManifestVersion = ManifestVersion
	}
	return &manifest, nil
}

// loadLegacyPluginJSON loads the old plugin.json format and converts it to v1 Manifest.
func loadLegacyPluginJSON(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Legacy structure (from old plugin.json).
	var legacy struct {
		Name        string   `json:"name"`
		Version     string   `json:"version"`
		Description string   `json:"description"`
		Author      string   `json:"author"`
		Type        string   `json:"type"`
		Risk        string   `json:"risk"`
		AdminOnly   bool     `json:"adminOnly"`
		Permissions []string `json:"permissions"`
		Container   struct {
			Image         string            `json:"image"`
			Port          int               `json:"port"`
			HostPort      int               `json:"hostPort"`
			ContainerName string            `json:"containerName"`
			Env           map[string]string `json:"env"`
			Network       string            `json:"network"`
			Volumes       []struct {
				Type     string `json:"type"`
				Source   string `json:"source"`
				Target   string `json:"target"`
				ReadOnly bool   `json:"readonly"`
			} `json:"volumes"`
			Security *struct {
				Privileged      bool     `json:"privileged"`
				NoNewPrivileges bool     `json:"noNewPrivileges"`
				ReadOnlyRootfs  bool     `json:"readOnlyRootfs"`
				CapDrop         []string `json:"capDrop"`
				CapAdd          []string `json:"capAdd"`
			} `json:"security"`
			Resources *struct {
				CPU    string `json:"cpu"`
				Memory string `json:"memory"`
			} `json:"resources"`
		} `json:"container"`
		UI *struct {
			Entry    string `json:"entry"`
			Icon     string `json:"icon"`
			NavTitle string `json:"navTitle"`
		} `json:"ui"`
	}

	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	manifest := &Manifest{
		ManifestVersion: ManifestVersion,
		Name:            legacy.Name,
		Version:         legacy.Version,
		Description:     legacy.Description,
		Author:          legacy.Author,
		AdminOnly:       legacy.AdminOnly,
		Risk:            RiskLow,
		Docker: DockerConfig{
			Image:         legacy.Container.Image,
			Port:          legacy.Container.Port,
			ContainerName: legacy.Container.ContainerName,
			Env:           legacy.Container.Env,
			Network:       legacy.Container.Network,
		},
	}

	// Convert risk level.
	switch strings.ToLower(legacy.Risk) {
	case "low":
		manifest.Risk = RiskLow
	case "medium":
		manifest.Risk = RiskMedium
	case "high":
		manifest.Risk = RiskHigh
	case "critical":
		manifest.Risk = RiskCritical
	default:
		// Fallback based on type.
		if legacy.Type == "privileged" {
			manifest.Risk = RiskHigh
		} else {
			manifest.Risk = RiskLow
		}
	}

	// Permissions.
	for _, p := range legacy.Permissions {
		manifest.Permissions = append(manifest.Permissions, Permission(p))
	}

	// Volumes.
	for _, v := range legacy.Container.Volumes {
		manifest.Docker.Volumes = append(manifest.Docker.Volumes, VolumeMount{
			Type:     v.Type,
			Source:   v.Source,
			Target:   v.Target,
			ReadOnly: v.ReadOnly,
		})
	}

	// Security.
	if legacy.Container.Security != nil {
		manifest.Docker.Security = &SecurityConfig{
			Privileged:      legacy.Container.Security.Privileged,
			NoNewPrivileges: legacy.Container.Security.NoNewPrivileges,
			ReadOnlyRootfs:  legacy.Container.Security.ReadOnlyRootfs,
			CapDrop:         legacy.Container.Security.CapDrop,
			CapAdd:          legacy.Container.Security.CapAdd,
		}
	}

	// Resources.
	if legacy.Container.Resources != nil {
		manifest.Docker.Resources = &ResourceLimits{
			CPUs:   legacy.Container.Resources.CPU,
			Memory: legacy.Container.Resources.Memory,
		}
	}

	// UI.
	if legacy.UI != nil {
		manifest.UI = &UIConfig{
			Path:      legacy.UI.Entry,
			Icon:      legacy.UI.Icon,
			Title:     legacy.UI.NavTitle,
			ShowInNav: true,
		}
	}

	return manifest, nil
}

// LoadManifestWithMigration attempts to load manifest.json, falling back to
// plugin.json with a deprecation warning.
func LoadManifestWithMigration(pluginDir string) (*Manifest, string, error) {
	manifestPath := filepath.Join(pluginDir, "manifest.json")
	if _, err := os.Stat(manifestPath); err == nil {
		manifest, err := LoadManifest(manifestPath)
		if err != nil {
			return nil, manifestPath, err
		}
		return manifest, manifestPath, nil
	}

	// Fallback to plugin.json (deprecated).
	pluginJSONPath := filepath.Join(pluginDir, "plugin.json")
	if _, err := os.Stat(pluginJSONPath); err == nil {
		manifest, err := loadLegacyPluginJSON(pluginJSONPath)
		if err != nil {
			return nil, pluginJSONPath, err
		}
		return manifest, pluginJSONPath + " (DEPRECATED)", nil
	}

	return nil, "", fmt.Errorf("no manifest.json or plugin.json found in %s", pluginDir)
}
