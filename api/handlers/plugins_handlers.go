// Package handlers provides HTTP handlers for plugin management.
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/AnalyseDeCircuit/opskernel/internal/logs"
	"github.com/AnalyseDeCircuit/opskernel/internal/plugin"
	"github.com/AnalyseDeCircuit/opskernel/internal/plugin/runtime"
	"github.com/AnalyseDeCircuit/opskernel/internal/plugin/store"
)

// ============================================================================
// Standardized API Error Response
// ============================================================================

// PluginAPIError represents a structured error response for plugin operations.
// This provides machine-readable error information for clients.
type PluginAPIError struct {
	// Error message (human-readable)
	Error string `json:"error"`

	// Machine-readable error code
	// Values: not_found, conflict, forbidden, bad_request, docker_unavailable,
	//         docker_timeout, policy_denied, internal_error
	Code string `json:"code"`

	// Whether the operation can be retried
	Retryable bool `json:"retryable"`

	// Additional details (optional)
	Details map[string]interface{} `json:"details,omitempty"`
}

// writePluginError writes a structured error response
func writePluginError(w http.ResponseWriter, status int, code, message string, retryable bool, details map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := PluginAPIError{
		Error:     message,
		Code:      code,
		Retryable: retryable,
		Details:   details,
	}
	json.NewEncoder(w).Encode(resp)
}

// mapPluginError maps internal errors to appropriate HTTP status and error codes
func mapPluginError(err error) (status int, code string, retryable bool) {
	errMsg := err.Error()

	// Check for Docker typed errors
	if de, ok := err.(*runtime.DockerError); ok {
		switch de.Kind {
		case runtime.ErrKindNotFound:
			return http.StatusNotFound, "not_found", false
		case runtime.ErrKindConflict:
			return http.StatusConflict, "conflict", false
		case runtime.ErrKindTimeout:
			return http.StatusGatewayTimeout, "docker_timeout", true
		case runtime.ErrKindUnreachable, runtime.ErrKindTemporary:
			return http.StatusServiceUnavailable, "docker_unavailable", true
		case runtime.ErrKindForbidden:
			return http.StatusForbidden, "forbidden", false
		case runtime.ErrKindReadOnly:
			return http.StatusForbidden, "docker_readonly", false
		case runtime.ErrKindBadRequest:
			return http.StatusBadRequest, "bad_request", false
		}
	}

	// Fallback: check error message patterns
	switch {
	case strings.Contains(errMsg, "not found"):
		return http.StatusNotFound, "not_found", false
	case strings.Contains(errMsg, "policy denied"):
		return http.StatusForbidden, "policy_denied", false
	case strings.Contains(errMsg, "409") || strings.Contains(errMsg, "Conflict"):
		return http.StatusConflict, "conflict", false
	case strings.Contains(errMsg, "timeout"):
		return http.StatusGatewayTimeout, "docker_timeout", true
	case strings.Contains(errMsg, "connection refused") || strings.Contains(errMsg, "no such host"):
		return http.StatusServiceUnavailable, "docker_unavailable", true
	default:
		return http.StatusBadRequest, "bad_request", false
	}
}

// ============================================================================
// Plugin Handlers
// ============================================================================

// PluginHandlers provides HTTP handlers for plugin operations
type PluginHandlers struct {
	manager *plugin.ManagerV2
}

// NewPluginHandlers creates new plugin handlers
func NewPluginHandlers(manager *plugin.ManagerV2) *PluginHandlers {
	return &PluginHandlers{manager: manager}
}

// HandleList returns the list of plugins (GET /api/plugins/list)
func (h *PluginHandlers) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Auth check
	_, role, ok := requireAuth(w, r)
	if !ok {
		return
	}

	plugins := h.manager.ListPluginsForRole(role)

	// Convert to response format
	type pluginResponse struct {
		Name               string   `json:"name"`
		Version            string   `json:"version"`
		Description        string   `json:"description,omitempty"`
		Type               string   `json:"type"` // For legacy compatibility
		Risk               string   `json:"risk"`
		AdminOnly          bool     `json:"adminOnly"`
		State              string   `json:"state"`
		Enabled            bool     `json:"enabled"`
		Running            bool     `json:"running"`
		ContainerName      string   `json:"containerName,omitempty"`
		BaseURL            string   `json:"baseUrl,omitempty"`
		Error              string   `json:"error,omitempty"`
		Mode               string   `json:"mode"` // Always "docker" in v2
		Icon               string   `json:"icon,omitempty"`
		NavTitle           string   `json:"navTitle,omitempty"`
		DeprecationWarning string   `json:"deprecationWarning,omitempty"`
		Confirmed          bool     `json:"confirmed"`
		ProxyURL           string   `json:"proxyUrl,omitempty"`
		Permissions        []string `json:"permissions,omitempty"`
	}

	response := make([]pluginResponse, 0, len(plugins))
	for _, p := range plugins {
		perms := make([]string, 0, len(p.Permissions))
		for _, perm := range p.Permissions {
			perms = append(perms, string(perm))
		}

		resp := pluginResponse{
			Name:               p.Name,
			Version:            p.Version,
			Description:        p.Description,
			Type:               "normal", // Default for legacy compat
			Risk:               string(p.Risk),
			AdminOnly:          p.AdminOnly,
			State:              string(p.State),
			Enabled:            p.Enabled,
			Running:            p.Running,
			Error:              p.Error,
			Mode:               "docker",
			Icon:               p.Icon,
			NavTitle:           p.NavTitle,
			DeprecationWarning: p.DeprecationWarning,
			Confirmed:          p.Confirmed,
			ProxyURL:           p.ProxyURL,
			Permissions:        perms,
		}

		// Set type based on risk for legacy compat
		if p.Risk == "high" || p.Risk == "critical" {
			resp.Type = "privileged"
		}

		response = append(response, resp)
	}

	writeJSON(w, http.StatusOK, response)
}

// HandleAction handles plugin enable/disable (POST /api/plugins/action)
// Uses structured error responses for better client handling
func (h *PluginHandlers) HandleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Auth check (Admin only)
	username, role, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if role != "admin" {
		writePluginError(w, http.StatusForbidden, "forbidden", "Admin access required", false, nil)
		return
	}

	var req struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		return
	}

	if err := h.manager.TogglePlugin(req.Name, req.Enabled); err != nil {
		status, code, retryable := mapPluginError(err)
		writePluginError(w, status, code, err.Error(), retryable, map[string]interface{}{
			"plugin": req.Name,
			"action": boolToAction(req.Enabled),
		})
		return
	}

	logs.LogOperation(username, "plugin_toggle", req.Name+" enabled="+boolToString(req.Enabled), clientIP(r))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleEnable handles plugin enable with confirmation (POST /api/plugins/enable)
// Uses structured error responses for better client handling
func (h *PluginHandlers) HandleEnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username, role, ok := requireAuth(w, r)
	if !ok {
		return
	}

	var req struct {
		Name string `json:"name"`

		// Confirmation fields
		AcknowledgedRisk         string   `json:"acknowledgedRisk,omitempty"`
		AcknowledgedPermissions  []string `json:"acknowledgedPermissions,omitempty"`
		AcknowledgedDockerParams []string `json:"acknowledgedDockerParams,omitempty"`
		ExplicitApproval         bool     `json:"explicitApproval,omitempty"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		return
	}

	enableReq := &plugin.EnableRequest{
		Name:     req.Name,
		Username: username,
		Role:     role,
	}

	// Add confirmation if provided
	if req.ExplicitApproval {
		enableReq.Confirmation = &store.Confirmation{
			PluginName:               req.Name,
			Username:                 username,
			AcknowledgedRisk:         req.AcknowledgedRisk,
			AcknowledgedPermissions:  req.AcknowledgedPermissions,
			AcknowledgedDockerParams: req.AcknowledgedDockerParams,
			ExplicitApproval:         true,
			Timestamp:                time.Now(),
		}
	}

	result, err := h.manager.EnablePlugin(r.Context(), enableReq)
	if err != nil {
		status, code, retryable := mapPluginError(err)
		writePluginError(w, status, code, err.Error(), retryable, map[string]interface{}{
			"plugin": req.Name,
			"action": "enable",
		})
		return
	}

	if result.Success {
		logs.LogOperation(username, "plugin_enable", req.Name, clientIP(r))
	}

	writeJSON(w, http.StatusOK, result)
}

// HandleDisable handles plugin disable (POST /api/plugins/disable)
// Uses structured error responses for better client handling
func (h *PluginHandlers) HandleDisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username, role, ok := requireAuth(w, r)
	if !ok {
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		return
	}

	if err := h.manager.DisablePlugin(r.Context(), req.Name, username, role); err != nil {
		status, code, retryable := mapPluginError(err)
		writePluginError(w, status, code, err.Error(), retryable, map[string]interface{}{
			"plugin": req.Name,
			"action": "disable",
		})
		return
	}

	logs.LogOperation(username, "plugin_disable", req.Name, clientIP(r))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleInstall handles plugin installation (POST /api/plugins/install)
// In v2, this is a no-op as containers are created on enable
func (h *PluginHandlers) HandleInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username, role, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if role != "admin" {
		writePluginError(w, http.StatusForbidden, "forbidden", "Admin access required", false, nil)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		return
	}

	result, err := h.manager.InstallPlugin(req.Name)
	if err != nil {
		status, code, retryable := mapPluginError(err)
		writePluginError(w, status, code, err.Error(), retryable, map[string]interface{}{
			"plugin": req.Name,
			"action": "install",
		})
		return
	}

	logs.LogOperation(username, "plugin_install", req.Name, clientIP(r))
	writeJSON(w, http.StatusOK, result)
}

// HandleUninstall handles plugin uninstallation (POST /api/plugins/uninstall)
// Uses structured error responses for better client handling
func (h *PluginHandlers) HandleUninstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username, role, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if role != "admin" {
		writePluginError(w, http.StatusForbidden, "forbidden", "Admin access required", false, nil)
		return
	}

	var req struct {
		Name       string `json:"name"`
		RemoveData bool   `json:"removeData,omitempty"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		return
	}

	result, err := h.manager.UninstallPlugin(req.Name, req.RemoveData)
	if err != nil {
		status, code, retryable := mapPluginError(err)
		writePluginError(w, status, code, err.Error(), retryable, map[string]interface{}{
			"plugin":     req.Name,
			"action":     "uninstall",
			"removeData": req.RemoveData,
		})
		return
	}

	logs.LogOperation(username, "plugin_uninstall", req.Name, clientIP(r))
	writeJSON(w, http.StatusOK, result)
}

// HandleManifest returns plugin manifest (GET /api/plugins/manifest?name=xxx)
func (h *PluginHandlers) HandleManifest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	_, role, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if role != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		writeJSONError(w, http.StatusBadRequest, "name parameter required")
		return
	}

	manifest, exists := h.manager.GetManifest(name)
	if !exists {
		writeJSONError(w, http.StatusNotFound, "manifest not found")
		return
	}

	writeJSON(w, http.StatusOK, manifest)
}

// HandleSecuritySummary returns security summary for confirmation UI (GET /api/plugins/security?name=xxx)
func (h *PluginHandlers) HandleSecuritySummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	_, _, ok := requireAuth(w, r)
	if !ok {
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		writeJSONError(w, http.StatusBadRequest, "name parameter required")
		return
	}

	manifest, exists := h.manager.GetManifest(name)
	if !exists {
		writeJSONError(w, http.StatusNotFound, "plugin not found")
		return
	}

	summary := manifest.GetSecuritySummary()
	writeJSON(w, http.StatusOK, summary)
}

// HandleProxy proxies requests to plugin containers
// Supports both /plugins/{name}/... and /api/plugins/{name}/... for backward compat
func (h *PluginHandlers) HandleProxy(w http.ResponseWriter, r *http.Request) {
	// Extract plugin name from path
	var pluginName string
	var isNewPath bool

	if strings.HasPrefix(r.URL.Path, "/plugins/") {
		// New path: /plugins/{name}/...
		path := strings.TrimPrefix(r.URL.Path, "/plugins/")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) == 0 || parts[0] == "" {
			http.NotFound(w, r)
			return
		}
		pluginName = parts[0]
		isNewPath = true
	} else if strings.HasPrefix(r.URL.Path, "/api/plugins/") {
		// Legacy path: /api/plugins/{name}/...
		path := strings.TrimPrefix(r.URL.Path, "/api/plugins/")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) == 0 || parts[0] == "" {
			http.NotFound(w, r)
			return
		}
		pluginName = parts[0]
		isNewPath = false
	} else {
		http.NotFound(w, r)
		return
	}

	// Skip API endpoints
	apiEndpoints := []string{"list", "action", "enable", "disable", "install", "uninstall", "manifest", "security"}
	for _, ep := range apiEndpoints {
		if pluginName == ep {
			http.NotFound(w, r)
			return
		}
	}

	// Auth check
	username, role, ok := requireAuth(w, r)
	if !ok {
		return
	}

	// Check plugin permissions
	info, exists := h.manager.GetPlugin(pluginName)
	if !exists {
		http.NotFound(w, r)
		return
	}

	if info.AdminOnly && role != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Audit logging for sensitive plugins
	if pluginName == "webshell" {
		if strings.HasSuffix(strings.TrimRight(r.URL.Path, "/"), "/ws") {
			logs.LogOperation(username, "webshell_connect", "websocket connect", clientIP(r))
		}
	}

	// Log new path usage
	if isNewPath {
		// Using new unified path /plugins/
	}

	h.manager.ServeHTTP(w, r, pluginName)
}

// decodeJSONBodyPlugin decodes JSON request body
func decodeJSONBodyPlugin(w http.ResponseWriter, r *http.Request, v interface{}) error {
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(v); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return err
	}
	return nil
}

// boolToAction converts a boolean to an action string
func boolToAction(enabled bool) string {
	if enabled {
		return "enable"
	}
	return "disable"
}
