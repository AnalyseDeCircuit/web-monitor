// Package registry provides v2 manifest schema and conversion.
// This file implements the adapter pattern: ManifestV2 -> Manifest (v1 internal).
package registry

import (
	"encoding/json"
)

// ============================================================================
// V2 Manifest Schema (matches manifest-v2.schema.json)
// ============================================================================

// ManifestV2 represents the v2 manifest schema.
// This is the parsing structure; it gets converted to Manifest for runtime use.
type ManifestV2 struct {
	Schema          string               `json:"$schema,omitempty"`
	ManifestVersion string               `json:"manifestVersion"` // Must be "2"
	Metadata        MetadataV2           `json:"metadata"`
	Requirements    *RequirementsV2      `json:"requirements,omitempty"`
	Security        SecurityV2           `json:"security"`
	Container       ContainerV2          `json:"container"`
	API             *APIV2               `json:"api,omitempty"`
	UI              *UIConfigV2          `json:"ui,omitempty"`
	Lifecycle       *LifecycleV2         `json:"lifecycle,omitempty"`
	I18n            map[string]I18nEntry `json:"i18n,omitempty"`
}

// MetadataV2 contains plugin identity and metadata.
type MetadataV2 struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	DisplayName string   `json:"displayName,omitempty"`
	Description string   `json:"description,omitempty"`
	Author      string   `json:"author,omitempty"`
	License     string   `json:"license,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
	Repository  string   `json:"repository,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Category    string   `json:"category,omitempty"`
}

// RequirementsV2 defines runtime dependencies.
type RequirementsV2 struct {
	MinCoreVersion string            `json:"minCoreVersion,omitempty"`
	Platforms      []string          `json:"platforms,omitempty"`
	Dependencies   map[string]string `json:"dependencies,omitempty"` // plugin-name: version
}

// SecurityV2 defines security classification and permissions.
type SecurityV2 struct {
	Risk               RiskLevel    `json:"risk"`
	AdminOnly          bool         `json:"adminOnly,omitempty"`
	Permissions        []Permission `json:"permissions,omitempty"`
	DataClassification string       `json:"dataClassification,omitempty"`
	AuditLog           bool         `json:"auditLog,omitempty"`
}

// ContainerV2 defines Docker container configuration.
type ContainerV2 struct {
	Image         string             `json:"image"`
	Port          int                `json:"port"`
	Protocol      string             `json:"protocol,omitempty"` // "http" or "https" (default: "http")
	ContainerName string             `json:"containerName,omitempty"`
	Env           map[string]string  `json:"env,omitempty"`
	Volumes       []VolumeMountV2    `json:"volumes,omitempty"`
	Devices       []DeviceMapping    `json:"devices,omitempty"`
	Network       string             `json:"network,omitempty"`
	Resources     *ResourceLimits    `json:"resources,omitempty"`
	Security      *SecurityConfig    `json:"security,omitempty"`
	ShmSize       string             `json:"shmSize,omitempty"` // Shared memory size, e.g. "512m"
	ExtraHosts    []string           `json:"extraHosts,omitempty"`
	WorkingDir    string             `json:"workingDir,omitempty"`
	Entrypoint    []string           `json:"entrypoint,omitempty"`
	Command       []string           `json:"command,omitempty"`
	Labels        map[string]string  `json:"labels,omitempty"`
	RestartPolicy string             `json:"restartPolicy,omitempty"`
	HealthCheck   *HealthCheckConfig `json:"healthCheck,omitempty"` // Moved here in v2
}

// APIV2 defines API endpoint configuration.
type APIV2 struct {
	Health    *HealthEndpointV2 `json:"health,omitempty"`
	Entry     string            `json:"entry,omitempty"`
	BasePath  string            `json:"basePath,omitempty"`
	WebSocket *WebSocketV2      `json:"websocket,omitempty"`
	OpenAPI   string            `json:"openapi,omitempty"`
	CORS      *CORSV2           `json:"cors,omitempty"`
}

// HealthEndpointV2 defines the health check endpoint.
type HealthEndpointV2 struct {
	Path       string `json:"path,omitempty"`
	StatusCode int    `json:"statusCode,omitempty"`
}

// WebSocketV2 defines WebSocket configuration.
type WebSocketV2 struct {
	Path      string   `json:"path,omitempty"`
	Protocols []string `json:"protocols,omitempty"`
}

// CORSV2 defines CORS configuration.
type CORSV2 struct {
	AllowOrigins []string `json:"allowOrigins,omitempty"`
	AllowMethods []string `json:"allowMethods,omitempty"`
	AllowHeaders []string `json:"allowHeaders,omitempty"`
}

// UIConfigV2 defines web UI integration.
type UIConfigV2 struct {
	Title     string `json:"title,omitempty"`
	Icon      string `json:"icon,omitempty"`
	ShowInNav bool   `json:"showInNav,omitempty"`
	NavOrder  int    `json:"navOrder,omitempty"`
	Sandbox   string `json:"sandbox,omitempty"`
	Theme     string `json:"theme,omitempty"`
	Width     string `json:"width,omitempty"`
}

// LifecycleV2 defines lifecycle hooks.
type LifecycleV2 struct {
	Install   *LifecyclePhaseV2 `json:"install,omitempty"`
	Upgrade   *LifecyclePhaseV2 `json:"upgrade,omitempty"`
	Uninstall *LifecyclePhaseV2 `json:"uninstall,omitempty"`
}

// LifecyclePhaseV2 defines a lifecycle phase.
type LifecyclePhaseV2 struct {
	Hooks          []LifecycleHookV2 `json:"hooks,omitempty"`
	Message        string            `json:"message,omitempty"`
	ConfirmMessage string            `json:"confirmMessage,omitempty"`
	PreserveData   bool              `json:"preserveData,omitempty"`
}

// LifecycleHookV2 defines a lifecycle hook.
type LifecycleHookV2 struct {
	Type    string `json:"type"` // "exec", "http", "ensure-directory", etc.
	Command string `json:"command,omitempty"`
	URL     string `json:"url,omitempty"`
	Path    string `json:"path,omitempty"`
	Mode    string `json:"mode,omitempty"`
}

// I18nEntry defines localized strings.
type I18nEntry struct {
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
}

// VolumeMountV2 defines a container volume or bind mount in V2 format.
// V2 uses more intuitive field names than V1.
type VolumeMountV2 struct {
	// For named volumes:
	Name string `json:"name,omitempty"` // Volume name (creates docker volume)

	// For bind mounts:
	HostPath string `json:"hostPath,omitempty"` // Host path to bind mount

	// Common fields:
	ContainerPath string `json:"containerPath"` // Path inside container
	ReadOnly      bool   `json:"readOnly,omitempty"`
	Description   string `json:"description,omitempty"` // Human-readable description
}

// ============================================================================
// V2 -> V1 Adapter (converts to internal Manifest for runtime compatibility)
// ============================================================================

// ConvertV2ToV1 converts a ManifestV2 to the internal Manifest (v1) structure.
// This allows all existing runtime code to work without changes.
func ConvertV2ToV1(v2 *ManifestV2) *Manifest {
	m := &Manifest{
		ManifestVersion: "2", // Keep original version marker for tracking
		Name:            v2.Metadata.Name,
		Version:         v2.Metadata.Version,
		Description:     v2.Metadata.Description,
		Author:          v2.Metadata.Author,
		License:         v2.Metadata.License,
		Homepage:        v2.Metadata.Homepage,
		Risk:            v2.Security.Risk,
		Permissions:     v2.Security.Permissions,
		AdminOnly:       v2.Security.AdminOnly,
		Tags:            v2.Metadata.Tags,
		Category:        v2.Metadata.Category,

		// Docker config mapping
		Docker: DockerConfig{
			Image:         v2.Container.Image,
			Port:          v2.Container.Port,
			Protocol:      v2.Container.Protocol,
			ContainerName: v2.Container.ContainerName,
			Env:           v2.Container.Env,
			Volumes:       convertVolumesV2ToV1(v2.Container.Volumes),
			Devices:       v2.Container.Devices,
			Network:       v2.Container.Network,
			Resources:     v2.Container.Resources,
			Security:      v2.Container.Security,
			ShmSize:       v2.Container.ShmSize,
			ExtraHosts:    v2.Container.ExtraHosts,
			WorkingDir:    v2.Container.WorkingDir,
			Entrypoint:    v2.Container.Entrypoint,
			Command:       v2.Container.Command,
			Labels:        v2.Container.Labels,
			RestartPolicy: v2.Container.RestartPolicy,
		},
	}

	// HealthCheck: v2 can define in container.healthCheck or api.health
	if v2.Container.HealthCheck != nil {
		m.HealthCheck = v2.Container.HealthCheck
	} else if v2.API != nil && v2.API.Health != nil {
		m.HealthCheck = &HealthCheckConfig{
			Path:       v2.API.Health.Path,
			StatusCode: v2.API.Health.StatusCode,
		}
	}

	// UI config mapping
	if v2.UI != nil {
		m.UI = &UIConfig{
			Title:     v2.UI.Title,
			Icon:      v2.UI.Icon,
			ShowInNav: v2.UI.ShowInNav,
			Sandbox:   v2.UI.Sandbox,
		}
		// v2 Entry path is in api.entry, not ui.path
		if v2.API != nil && v2.API.Entry != "" {
			m.UI.Path = v2.API.Entry
		}
	} else if v2.API != nil && v2.API.Entry != "" {
		m.UI = &UIConfig{Path: v2.API.Entry}
	}

	return m
}

// ParseManifestV2 parses JSON data as a v2 manifest.
func ParseManifestV2(data []byte) (*ManifestV2, error) {
	var v2 ManifestV2
	if err := json.Unmarshal(data, &v2); err != nil {
		return nil, err
	}
	return &v2, nil
}

// IsV2Manifest checks if the JSON data appears to be a v2 manifest.
func IsV2Manifest(data []byte) bool {
	var check struct {
		ManifestVersion string `json:"manifestVersion"`
	}
	if err := json.Unmarshal(data, &check); err != nil {
		return false
	}
	return check.ManifestVersion == "2"
}

// convertVolumesV2ToV1 converts V2 volume mounts to V1 format.
func convertVolumesV2ToV1(v2Volumes []VolumeMountV2) []VolumeMount {
	if len(v2Volumes) == 0 {
		return nil
	}

	v1Volumes := make([]VolumeMount, 0, len(v2Volumes))
	for _, v := range v2Volumes {
		var vol VolumeMount
		vol.Target = v.ContainerPath
		vol.ReadOnly = v.ReadOnly

		if v.HostPath != "" {
			// Bind mount: hostPath specified
			vol.Type = "bind"
			vol.Source = v.HostPath
		} else if v.Name != "" {
			// Named volume: name specified
			vol.Type = "volume"
			vol.Source = v.Name
		}

		v1Volumes = append(v1Volumes, vol)
	}
	return v1Volumes
}
