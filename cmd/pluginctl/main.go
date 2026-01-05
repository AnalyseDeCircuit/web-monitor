// Package main provides CLI tools for plugin management.
// Usage: pluginctl validate <plugin-dir>
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Exit codes
const (
	ExitOK              = 0
	ExitValidationError = 1
	ExitUsageError      = 2
	ExitFileError       = 3
)

// Colors for terminal output
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
)

// ========================================================================
// Manifest V2 Schema (matches manifest-v2.schema.json)
// ========================================================================

type ManifestV2 struct {
	Schema          string               `json:"$schema,omitempty"`
	ManifestVersion string               `json:"manifestVersion"`
	Metadata        MetadataV2           `json:"metadata"`
	Requirements    *RequirementsV2      `json:"requirements,omitempty"`
	Security        SecurityV2           `json:"security"`
	Container       ContainerV2          `json:"container"`
	API             *APIV2               `json:"api,omitempty"`
	UI              *UIConfigV2          `json:"ui,omitempty"`
	Lifecycle       *LifecycleV2         `json:"lifecycle,omitempty"`
	I18n            map[string]I18nEntry `json:"i18n,omitempty"`
}

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

type RequirementsV2 struct {
	MinCoreVersion string            `json:"minCoreVersion,omitempty"`
	Platforms      []string          `json:"platforms,omitempty"`
	Dependencies   map[string]string `json:"dependencies,omitempty"`
}

type SecurityV2 struct {
	Risk               string   `json:"risk"`
	AdminOnly          bool     `json:"adminOnly,omitempty"`
	Permissions        []string `json:"permissions,omitempty"`
	DataClassification string   `json:"dataClassification,omitempty"`
	AuditLog           bool     `json:"auditLog,omitempty"`
}

type ContainerV2 struct {
	Image         string               `json:"image"`
	Port          int                  `json:"port"`
	ContainerName string               `json:"containerName,omitempty"`
	Env           map[string]string    `json:"env,omitempty"`
	Volumes       []VolumeMount        `json:"volumes,omitempty"`
	Devices       []DeviceMapping      `json:"devices,omitempty"`
	Network       string               `json:"network,omitempty"`
	Resources     *ResourceLimits      `json:"resources,omitempty"`
	Security      *ContainerSecurityV2 `json:"security,omitempty"`
	ExtraHosts    []string             `json:"extraHosts,omitempty"`
	WorkingDir    string               `json:"workingDir,omitempty"`
	Entrypoint    []string             `json:"entrypoint,omitempty"`
	Command       []string             `json:"command,omitempty"`
	Labels        map[string]string    `json:"labels,omitempty"`
	RestartPolicy string               `json:"restartPolicy,omitempty"`
	HealthCheck   *HealthCheckConfigV2 `json:"healthCheck,omitempty"`
}

type VolumeMount struct {
	Type     string `json:"type"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"readOnly,omitempty"`
}

type DeviceMapping struct {
	Host        string `json:"host"`
	Container   string `json:"container,omitempty"`
	Permissions string `json:"permissions,omitempty"`
}

type ResourceLimits struct {
	Memory            string `json:"memory,omitempty"`
	MemoryReservation string `json:"memoryReservation,omitempty"`
	CPUs              string `json:"cpus,omitempty"`
	CPUShares         int64  `json:"cpuShares,omitempty"`
	PidsLimit         int64  `json:"pidsLimit,omitempty"`
}

type ContainerSecurityV2 struct {
	CapAdd          []string `json:"capAdd,omitempty"`
	CapDrop         []string `json:"capDrop,omitempty"`
	ReadOnlyRootfs  bool     `json:"readOnlyRootfs,omitempty"`
	NoNewPrivileges bool     `json:"noNewPrivileges,omitempty"`
	SecurityOpt     []string `json:"securityOpt,omitempty"`
	Privileged      bool     `json:"privileged,omitempty"`
	User            string   `json:"user,omitempty"`
}

type HealthCheckConfigV2 struct {
	Path        string `json:"path,omitempty"`
	StatusCode  int    `json:"statusCode,omitempty"`
	Interval    string `json:"interval,omitempty"`
	Timeout     string `json:"timeout,omitempty"`
	Retries     int    `json:"retries,omitempty"`
	StartPeriod string `json:"startPeriod,omitempty"`
}

type APIV2 struct {
	Health    *HealthEndpoint `json:"health,omitempty"`
	Entry     string          `json:"entry,omitempty"`
	BasePath  string          `json:"basePath,omitempty"`
	WebSocket *WebSocketV2    `json:"websocket,omitempty"`
	OpenAPI   string          `json:"openapi,omitempty"`
	CORS      *CORSV2         `json:"cors,omitempty"`
}

type HealthEndpoint struct {
	Path       string `json:"path,omitempty"`
	StatusCode int    `json:"statusCode,omitempty"`
}

type WebSocketV2 struct {
	Path      string   `json:"path,omitempty"`
	Protocols []string `json:"protocols,omitempty"`
}

type CORSV2 struct {
	AllowOrigins []string `json:"allowOrigins,omitempty"`
	AllowMethods []string `json:"allowMethods,omitempty"`
	AllowHeaders []string `json:"allowHeaders,omitempty"`
}

type UIConfigV2 struct {
	Title     string `json:"title,omitempty"`
	Icon      string `json:"icon,omitempty"`
	ShowInNav bool   `json:"showInNav,omitempty"`
	NavOrder  int    `json:"navOrder,omitempty"`
	Sandbox   string `json:"sandbox,omitempty"`
	Theme     string `json:"theme,omitempty"`
	Width     string `json:"width,omitempty"`
}

type LifecycleV2 struct {
	Install   *LifecyclePhase `json:"install,omitempty"`
	Upgrade   *LifecyclePhase `json:"upgrade,omitempty"`
	Uninstall *LifecyclePhase `json:"uninstall,omitempty"`
}

type LifecyclePhase struct {
	Hooks          []LifecycleHook `json:"hooks,omitempty"`
	Message        string          `json:"message,omitempty"`
	ConfirmMessage string          `json:"confirmMessage,omitempty"`
	PreserveData   bool            `json:"preserveData,omitempty"`
}

type LifecycleHook struct {
	Type    string `json:"type"`
	Command string `json:"command,omitempty"`
	URL     string `json:"url,omitempty"`
	Path    string `json:"path,omitempty"`
	Mode    string `json:"mode,omitempty"`
}

type I18nEntry struct {
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
}

// ========================================================================
// Manifest V1 Schema (legacy)
// ========================================================================

type ManifestV1 struct {
	ManifestVersion string         `json:"manifestVersion"`
	Name            string         `json:"name"`
	Version         string         `json:"version"`
	Description     string         `json:"description,omitempty"`
	Author          string         `json:"author,omitempty"`
	License         string         `json:"license,omitempty"`
	Homepage        string         `json:"homepage,omitempty"`
	Risk            string         `json:"risk"`
	Permissions     []string       `json:"permissions,omitempty"`
	AdminOnly       bool           `json:"adminOnly,omitempty"`
	Docker          DockerConfigV1 `json:"docker"`
	UI              *UIConfigV1    `json:"ui,omitempty"`
	HealthCheck     *HealthCheckV1 `json:"healthCheck,omitempty"`
	Tags            []string       `json:"tags,omitempty"`
	Category        string         `json:"category,omitempty"`
}

type DockerConfigV1 struct {
	Image         string               `json:"image"`
	Port          int                  `json:"port"`
	ContainerName string               `json:"containerName,omitempty"`
	Env           map[string]string    `json:"env,omitempty"`
	Volumes       []VolumeMount        `json:"volumes,omitempty"`
	Devices       []DeviceMapping      `json:"devices,omitempty"`
	Network       string               `json:"network,omitempty"`
	Resources     *ResourceLimits      `json:"resources,omitempty"`
	Security      *ContainerSecurityV2 `json:"security,omitempty"`
	ExtraHosts    []string             `json:"extraHosts,omitempty"`
	WorkingDir    string               `json:"workingDir,omitempty"`
	Entrypoint    []string             `json:"entrypoint,omitempty"`
	Command       []string             `json:"command,omitempty"`
	Labels        map[string]string    `json:"labels,omitempty"`
	RestartPolicy string               `json:"restartPolicy,omitempty"`
}

type UIConfigV1 struct {
	Path      string `json:"path,omitempty"`
	Title     string `json:"title,omitempty"`
	Icon      string `json:"icon,omitempty"`
	ShowInNav bool   `json:"showInNav,omitempty"`
	Sandbox   string `json:"sandbox,omitempty"`
}

type HealthCheckV1 struct {
	Path        string `json:"path,omitempty"`
	StatusCode  int    `json:"statusCode,omitempty"`
	Interval    string `json:"interval,omitempty"`
	Timeout     string `json:"timeout,omitempty"`
	Retries     int    `json:"retries,omitempty"`
	StartPeriod string `json:"startPeriod,omitempty"`
}

// ========================================================================
// Validation Engine
// ========================================================================

// ValidationError represents a single validation failure
type ValidationError struct {
	Path    string // JSON path like "security.permissions[0]"
	Message string
	Value   interface{}
}

func (e *ValidationError) String() string {
	if e.Value != nil {
		return fmt.Sprintf("%s: %s (got: %v)", e.Path, e.Message, e.Value)
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// Validator validates manifests
type Validator struct {
	errors []ValidationError
}

// AddError adds a validation error
func (v *Validator) AddError(path, msg string, value interface{}) {
	v.errors = append(v.errors, ValidationError{Path: path, Message: msg, Value: value})
}

// HasErrors returns true if there are validation errors
func (v *Validator) HasErrors() bool {
	return len(v.errors) > 0
}

// Errors returns all validation errors
func (v *Validator) Errors() []ValidationError {
	return v.errors
}

// Regex patterns
var (
	namePattern     = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`)
	semverPattern   = regexp.MustCompile(`^\d+\.\d+\.\d+(-[a-zA-Z0-9.-]+)?(\+[a-zA-Z0-9.-]+)?$`)
	durationPattern = regexp.MustCompile(`^\d+(s|m|h)$`)
	memoryPattern   = regexp.MustCompile(`^\d+[kmgKMG]?$`)
	cpuPattern      = regexp.MustCompile(`^(\d+\.?\d*|\d*\.?\d+)$`)
)

// Valid risk levels
var validRisks = map[string]bool{
	"low": true, "medium": true, "high": true, "critical": true,
}

// Valid permissions
var validPermissions = map[string]bool{
	"host:network":      true,
	"host:pid":          true,
	"host:ipc":          true,
	"host:mount":        true,
	"host:privilege":    true,
	"host:ssh":          true,
	"docker:socket":     true,
	"docker:management": true,
	"data:read":         true,
	"data:write":        true,
	"data:delete":       true,
	"net:local":         true,
	"net:internet":      true,
	"fs:read":           true,
	"fs:write":          true,
	"user:root":         true,
	"gpu:access":        true,
	"device:usb":        true,
}

// Valid volume types
var validVolumeTypes = map[string]bool{
	"bind": true, "volume": true,
}

// Valid restart policies
var validRestartPolicies = map[string]bool{
	"": true, "no": true, "always": true, "unless-stopped": true, "on-failure": true,
}

// ValidateV2 validates a v2 manifest
func (v *Validator) ValidateV2(m *ManifestV2) {
	// manifestVersion
	if m.ManifestVersion != "2" {
		v.AddError("manifestVersion", "must be \"2\"", m.ManifestVersion)
	}

	// metadata (required)
	v.validateMetadataV2(&m.Metadata)

	// security (required)
	v.validateSecurityV2(&m.Security)

	// container (required)
	v.validateContainerV2(&m.Container)

	// api (optional)
	if m.API != nil {
		v.validateAPIV2(m.API)
	}

	// ui (optional)
	if m.UI != nil {
		v.validateUIV2(m.UI)
	}

	// lifecycle (optional)
	if m.Lifecycle != nil {
		v.validateLifecycleV2(m.Lifecycle)
	}

	// Cross-field validations
	v.crossValidateV2(m)
}

func (v *Validator) validateMetadataV2(m *MetadataV2) {
	// name (required)
	if m.Name == "" {
		v.AddError("metadata.name", "is required", nil)
	} else if len(m.Name) < 2 {
		v.AddError("metadata.name", "must be at least 2 characters", m.Name)
	} else if len(m.Name) > 64 {
		v.AddError("metadata.name", "must be at most 64 characters", m.Name)
	} else if !namePattern.MatchString(m.Name) {
		v.AddError("metadata.name", "must be lowercase, start with letter, contain only a-z, 0-9, hyphen, and end with alphanumeric", m.Name)
	}

	// version (required)
	if m.Version == "" {
		v.AddError("metadata.version", "is required", nil)
	} else if !semverPattern.MatchString(m.Version) {
		v.AddError("metadata.version", "must be valid semver (e.g., \"1.0.0\")", m.Version)
	}

	// displayName (optional, but recommend if different from name)
	if m.DisplayName != "" && len(m.DisplayName) > 128 {
		v.AddError("metadata.displayName", "must be at most 128 characters", nil)
	}

	// description (optional)
	if m.Description != "" && len(m.Description) > 1024 {
		v.AddError("metadata.description", "must be at most 1024 characters", nil)
	}

	// category (optional, validate if provided)
	validCategories := map[string]bool{
		"": true, "monitoring": true, "development": true, "security": true,
		"networking": true, "database": true, "devops": true, "utility": true,
	}
	if m.Category != "" && !validCategories[m.Category] {
		v.AddError("metadata.category", "must be one of: monitoring, development, security, networking, database, devops, utility", m.Category)
	}
}

func (v *Validator) validateSecurityV2(s *SecurityV2) {
	// risk (required)
	if s.Risk == "" {
		v.AddError("security.risk", "is required", nil)
	} else if !validRisks[s.Risk] {
		v.AddError("security.risk", "must be one of: low, medium, high, critical", s.Risk)
	}

	// permissions (optional, validate each)
	for i, perm := range s.Permissions {
		if !validPermissions[perm] {
			v.AddError(fmt.Sprintf("security.permissions[%d]", i), "invalid permission", perm)
		}
	}

	// dataClassification (optional)
	validClassifications := map[string]bool{
		"": true, "public": true, "internal": true, "confidential": true, "restricted": true,
	}
	if s.DataClassification != "" && !validClassifications[s.DataClassification] {
		v.AddError("security.dataClassification", "must be one of: public, internal, confidential, restricted", s.DataClassification)
	}
}

func (v *Validator) validateContainerV2(c *ContainerV2) {
	// image (required)
	if c.Image == "" {
		v.AddError("container.image", "is required", nil)
	}

	// port (required, valid range)
	if c.Port <= 0 || c.Port > 65535 {
		v.AddError("container.port", "must be between 1 and 65535", c.Port)
	}

	// containerName (optional, validate format if provided)
	if c.ContainerName != "" {
		if matched, _ := regexp.MatchString(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`, c.ContainerName); !matched {
			v.AddError("container.containerName", "must start with alphanumeric and contain only a-zA-Z0-9_.-", c.ContainerName)
		}
	}

	// volumes (optional, validate each)
	for i, vol := range c.Volumes {
		v.validateVolume(fmt.Sprintf("container.volumes[%d]", i), &vol)
	}

	// devices (optional, validate each)
	for i, dev := range c.Devices {
		if dev.Host == "" {
			v.AddError(fmt.Sprintf("container.devices[%d].host", i), "is required", nil)
		}
	}

	// network (optional)
	validNetworks := map[string]bool{
		"": true, "bridge": true, "host": true, "none": true,
	}
	if c.Network != "" && !validNetworks[c.Network] && !strings.HasPrefix(c.Network, "container:") {
		// Allow custom network names, just warn about host mode
	}

	// resources (optional)
	if c.Resources != nil {
		v.validateResources("container.resources", c.Resources)
	}

	// restartPolicy (optional)
	if !validRestartPolicies[c.RestartPolicy] {
		v.AddError("container.restartPolicy", "must be one of: no, always, unless-stopped, on-failure", c.RestartPolicy)
	}

	// healthCheck (optional)
	if c.HealthCheck != nil {
		v.validateHealthCheck("container.healthCheck", c.HealthCheck)
	}
}

func (v *Validator) validateVolume(path string, vol *VolumeMount) {
	if !validVolumeTypes[vol.Type] {
		v.AddError(path+".type", "must be \"bind\" or \"volume\"", vol.Type)
	}
	if vol.Source == "" {
		v.AddError(path+".source", "is required", nil)
	}
	if vol.Target == "" {
		v.AddError(path+".target", "is required", nil)
	}
	// For bind mounts, source should be absolute path
	if vol.Type == "bind" && !strings.HasPrefix(vol.Source, "/") && !strings.HasPrefix(vol.Source, "${") {
		v.AddError(path+".source", "bind mount source should be an absolute path", vol.Source)
	}
}

func (v *Validator) validateResources(path string, r *ResourceLimits) {
	if r.Memory != "" && !memoryPattern.MatchString(r.Memory) {
		v.AddError(path+".memory", "invalid format (e.g., \"128m\", \"1g\")", r.Memory)
	}
	if r.MemoryReservation != "" && !memoryPattern.MatchString(r.MemoryReservation) {
		v.AddError(path+".memoryReservation", "invalid format", r.MemoryReservation)
	}
	if r.CPUs != "" && !cpuPattern.MatchString(r.CPUs) {
		v.AddError(path+".cpus", "must be a number (e.g., \"0.5\", \"2\")", r.CPUs)
	}
	if r.PidsLimit < 0 {
		v.AddError(path+".pidsLimit", "must be >= 0", r.PidsLimit)
	}
}

func (v *Validator) validateHealthCheck(path string, h *HealthCheckConfigV2) {
	if h.Interval != "" && !durationPattern.MatchString(h.Interval) {
		v.AddError(path+".interval", "must be duration (e.g., \"30s\", \"1m\")", h.Interval)
	}
	if h.Timeout != "" && !durationPattern.MatchString(h.Timeout) {
		v.AddError(path+".timeout", "must be duration (e.g., \"5s\")", h.Timeout)
	}
	if h.StartPeriod != "" && !durationPattern.MatchString(h.StartPeriod) {
		v.AddError(path+".startPeriod", "must be duration (e.g., \"10s\")", h.StartPeriod)
	}
	if h.Retries < 0 {
		v.AddError(path+".retries", "must be >= 0", h.Retries)
	}
	if h.StatusCode != 0 && (h.StatusCode < 100 || h.StatusCode > 599) {
		v.AddError(path+".statusCode", "must be valid HTTP status code (100-599)", h.StatusCode)
	}
}

func (v *Validator) validateAPIV2(a *APIV2) {
	// websocket (optional)
	if a.WebSocket != nil {
		if a.WebSocket.Path != "" && !strings.HasPrefix(a.WebSocket.Path, "/") {
			v.AddError("api.websocket.path", "must start with /", a.WebSocket.Path)
		}
	}
}

func (v *Validator) validateUIV2(u *UIConfigV2) {
	// navOrder (optional)
	if u.NavOrder < 0 || u.NavOrder > 999 {
		v.AddError("ui.navOrder", "must be between 0 and 999", u.NavOrder)
	}
	// width (optional)
	validWidths := map[string]bool{
		"": true, "narrow": true, "medium": true, "wide": true, "full": true,
	}
	if u.Width != "" && !validWidths[u.Width] {
		v.AddError("ui.width", "must be one of: narrow, medium, wide, full", u.Width)
	}
}

func (v *Validator) validateLifecycleV2(l *LifecycleV2) {
	if l.Install != nil {
		v.validateLifecyclePhase("lifecycle.install", l.Install)
	}
	if l.Upgrade != nil {
		v.validateLifecyclePhase("lifecycle.upgrade", l.Upgrade)
	}
	if l.Uninstall != nil {
		v.validateLifecyclePhase("lifecycle.uninstall", l.Uninstall)
	}
}

func (v *Validator) validateLifecyclePhase(path string, p *LifecyclePhase) {
	validHookTypes := map[string]bool{
		"exec": true, "http": true, "ensure-directory": true,
		"generate-ssh-key": true, "notify": true,
	}
	for i, hook := range p.Hooks {
		hookPath := fmt.Sprintf("%s.hooks[%d]", path, i)
		if hook.Type == "" {
			v.AddError(hookPath+".type", "is required", nil)
		} else if !validHookTypes[hook.Type] {
			v.AddError(hookPath+".type", "must be one of: exec, http, ensure-directory, generate-ssh-key, notify", hook.Type)
		}
	}
}

func (v *Validator) crossValidateV2(m *ManifestV2) {
	// Risk level vs permissions consistency
	hasHighRiskPerm := false
	for _, perm := range m.Security.Permissions {
		if perm == "host:network" || perm == "host:privilege" || perm == "docker:socket" {
			hasHighRiskPerm = true
			break
		}
	}
	if hasHighRiskPerm && m.Security.Risk != "high" && m.Security.Risk != "critical" {
		v.AddError("security.risk", "should be 'high' or 'critical' when using dangerous permissions", m.Security.Risk)
	}

	// host network requires permission
	if m.Container.Network == "host" && !containsPermission(m.Security.Permissions, "host:network") {
		v.AddError("container.network", "host network requires 'host:network' permission", nil)
	}

	// privileged mode requires permission
	if m.Container.Security != nil && m.Container.Security.Privileged && !containsPermission(m.Security.Permissions, "host:privilege") {
		v.AddError("container.security.privileged", "privileged mode requires 'host:privilege' permission", nil)
	}

	// adminOnly should be true for high/critical risk
	if (m.Security.Risk == "high" || m.Security.Risk == "critical") && !m.Security.AdminOnly {
		v.AddError("security.adminOnly", "should be true for high/critical risk plugins (warning)", nil)
	}
}

func containsPermission(perms []string, target string) bool {
	for _, p := range perms {
		if p == target {
			return true
		}
	}
	return false
}

// ValidateV1 validates a v1 manifest
func (v *Validator) ValidateV1(m *ManifestV1) {
	// name (required)
	if m.Name == "" {
		v.AddError("name", "is required", nil)
	} else if !namePattern.MatchString(m.Name) {
		v.AddError("name", "must be lowercase, start with letter, contain only a-z, 0-9, hyphen", m.Name)
	}

	// version (required)
	if m.Version == "" {
		v.AddError("version", "is required", nil)
	} else if !semverPattern.MatchString(m.Version) {
		v.AddError("version", "must be valid semver", m.Version)
	}

	// risk (required)
	if m.Risk == "" {
		v.AddError("risk", "is required", nil)
	} else if !validRisks[m.Risk] {
		v.AddError("risk", "must be one of: low, medium, high, critical", m.Risk)
	}

	// docker.image (required)
	if m.Docker.Image == "" {
		v.AddError("docker.image", "is required", nil)
	}

	// docker.port (required)
	if m.Docker.Port <= 0 || m.Docker.Port > 65535 {
		v.AddError("docker.port", "must be between 1 and 65535", m.Docker.Port)
	}

	// permissions
	for i, perm := range m.Permissions {
		if !validPermissions[perm] {
			v.AddError(fmt.Sprintf("permissions[%d]", i), "invalid permission", perm)
		}
	}

	// volumes
	for i, vol := range m.Docker.Volumes {
		v.validateVolume(fmt.Sprintf("docker.volumes[%d]", i), &vol)
	}

	// resources
	if m.Docker.Resources != nil {
		v.validateResources("docker.resources", m.Docker.Resources)
	}
}

// ========================================================================
// Main CLI
// ========================================================================

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(ExitUsageError)
	}

	command := os.Args[1]

	switch command {
	case "validate":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "%sError: missing plugin directory%s\n", colorRed, colorReset)
			fmt.Fprintf(os.Stderr, "Usage: pluginctl validate <plugin-dir>\n")
			os.Exit(ExitUsageError)
		}
		os.Exit(cmdValidate(os.Args[2]))
	case "check-version":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Usage: pluginctl check-version <plugin-dir>\n")
			os.Exit(ExitUsageError)
		}
		os.Exit(cmdCheckVersion(os.Args[2]))
	case "help", "--help", "-h":
		printUsage()
		os.Exit(ExitOK)
	default:
		fmt.Fprintf(os.Stderr, "%sUnknown command: %s%s\n", colorRed, command, colorReset)
		printUsage()
		os.Exit(ExitUsageError)
	}
}

func printUsage() {
	fmt.Println(`pluginctl - OpsKernel Plugin CLI Tool

Usage:
  pluginctl <command> [arguments]

Commands:
  validate <plugin-dir>     Validate plugin manifest
  check-version <plugin-dir>  Check manifest version (returns "1" or "2")
  help                      Show this help

Examples:
  pluginctl validate plugins/webshell
  pluginctl check-version plugins/my-plugin

Exit codes:
  0  Success
  1  Validation failed
  2  Usage error
  3  File error`)
}

func cmdValidate(pluginDir string) int {
	// Find manifest file
	manifestPath := filepath.Join(pluginDir, "manifest.json")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		// Try plugin.json (legacy)
		legacyPath := filepath.Join(pluginDir, "plugin.json")
		if _, err := os.Stat(legacyPath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "%sError: No manifest.json or plugin.json found in %s%s\n", colorRed, pluginDir, colorReset)
			return ExitFileError
		}
		manifestPath = legacyPath
		fmt.Fprintf(os.Stderr, "%s[DEPRECATION] Using plugin.json - please migrate to manifest.json%s\n", colorYellow, colorReset)
	}

	// Read file
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%sError reading %s: %v%s\n", colorRed, manifestPath, err, colorReset)
		return ExitFileError
	}

	// Detect version
	var versionCheck struct {
		ManifestVersion string `json:"manifestVersion"`
	}
	if err := json.Unmarshal(data, &versionCheck); err != nil {
		fmt.Fprintf(os.Stderr, "%sError parsing JSON: %v%s\n", colorRed, err, colorReset)
		return ExitFileError
	}

	validator := &Validator{}

	switch versionCheck.ManifestVersion {
	case "2":
		var m ManifestV2
		if err := json.Unmarshal(data, &m); err != nil {
			fmt.Fprintf(os.Stderr, "%sError parsing manifest v2: %v%s\n", colorRed, err, colorReset)
			// Try to provide more specific JSON error
			if syntaxErr, ok := err.(*json.SyntaxError); ok {
				line, col := getLineCol(data, syntaxErr.Offset)
				fmt.Fprintf(os.Stderr, "  at line %d, column %d\n", line, col)
			}
			return ExitFileError
		}
		fmt.Printf("%s[V2 Manifest] %s%s\n", colorCyan, filepath.Base(pluginDir), colorReset)
		validator.ValidateV2(&m)

	case "1", "":
		var m ManifestV1
		if err := json.Unmarshal(data, &m); err != nil {
			fmt.Fprintf(os.Stderr, "%sError parsing manifest v1: %v%s\n", colorRed, err, colorReset)
			return ExitFileError
		}
		fmt.Printf("%s[V1 Manifest] %s%s\n", colorCyan, filepath.Base(pluginDir), colorReset)
		if versionCheck.ManifestVersion == "" {
			fmt.Printf("%s  Warning: manifestVersion not specified, assuming v1%s\n", colorYellow, colorReset)
		}
		validator.ValidateV1(&m)

	default:
		fmt.Fprintf(os.Stderr, "%sError: unsupported manifestVersion: %s%s\n", colorRed, versionCheck.ManifestVersion, colorReset)
		return ExitValidationError
	}

	// Print results
	if validator.HasErrors() {
		fmt.Printf("\n%sValidation FAILED with %d error(s):%s\n\n", colorRed, len(validator.Errors()), colorReset)
		for i, err := range validator.Errors() {
			fmt.Printf("  %s%d. %s%s\n", colorRed, i+1, err.String(), colorReset)
		}
		fmt.Println()
		return ExitValidationError
	}

	fmt.Printf("\n%s✓ Validation PASSED%s\n", colorGreen, colorReset)
	return ExitOK
}

func cmdCheckVersion(pluginDir string) int {
	manifestPath := filepath.Join(pluginDir, "manifest.json")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		manifestPath = filepath.Join(pluginDir, "plugin.json")
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "not-found")
			return ExitFileError
		}
	}

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read-error")
		return ExitFileError
	}

	var versionCheck struct {
		ManifestVersion string `json:"manifestVersion"`
	}
	if err := json.Unmarshal(data, &versionCheck); err != nil {
		fmt.Fprintln(os.Stderr, "parse-error")
		return ExitFileError
	}

	switch versionCheck.ManifestVersion {
	case "2":
		fmt.Println("2")
	case "1", "":
		fmt.Println("1")
	default:
		fmt.Println(versionCheck.ManifestVersion)
	}
	return ExitOK
}

// getLineCol calculates line and column from byte offset
func getLineCol(data []byte, offset int64) (line, col int) {
	line = 1
	col = 1
	for i := int64(0); i < offset && i < int64(len(data)); i++ {
		if data[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return
}
