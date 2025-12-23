// Package gateway handles HTTP reverse proxy for plugins.
// It provides a unified /plugins/{name}/ endpoint for all plugin UIs.
package gateway

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/AnalyseDeCircuit/opskernel/internal/plugin/registry"
	"github.com/AnalyseDeCircuit/opskernel/internal/plugin/runtime"
	"github.com/AnalyseDeCircuit/opskernel/internal/plugin/store"
)

// Gateway handles plugin HTTP reverse proxy
type Gateway struct {
	registry *registry.Registry
	runtime  *runtime.Runtime
	store    *store.Store
}

// NewGateway creates a new plugin gateway
func NewGateway(reg *registry.Registry, rt *runtime.Runtime, st *store.Store) *Gateway {
	return &Gateway{
		registry: reg,
		runtime:  rt,
		store:    st,
	}
}

// ServeHTTP handles requests to /plugins/{name}/...
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse plugin name from path
	// Path format: /plugins/{name}/... or /plugins/{name}
	path := strings.TrimPrefix(r.URL.Path, "/plugins/")
	if path == "" || path == "/" {
		http.NotFound(w, r)
		return
	}

	parts := strings.SplitN(path, "/", 2)
	pluginName := parts[0]

	// Check if plugin exists
	manifest, exists := g.registry.Get(pluginName)
	if !exists {
		http.NotFound(w, r)
		return
	}

	// Check if plugin is enabled
	state := g.store.Get(pluginName)
	if !state.Enabled {
		http.Error(w, "Plugin is disabled", http.StatusForbidden)
		return
	}

	// Check if plugin is running
	if !g.runtime.IsRunning(pluginName) {
		http.Error(w, "Plugin is not running", http.StatusServiceUnavailable)
		return
	}

	// Get proxy
	proxy := g.runtime.GetProxy(pluginName)
	if proxy == nil {
		http.Error(w, "Plugin proxy not available", http.StatusServiceUnavailable)
		return
	}

	// Determine the path to forward
	// If manifest specifies a UI path, respect it
	forwardPath := "/"
	if len(parts) > 1 {
		forwardPath = "/" + parts[1]
	}
	if manifest.UI != nil && manifest.UI.Path != "" {
		// Prepend the UI base path if specified
		basePath := strings.TrimSuffix(manifest.UI.Path, "/")
		if basePath != "" && basePath != "/" {
			forwardPath = basePath + forwardPath
		}
	}

	// Strip the /plugins/{name} prefix and proxy
	prefix := "/plugins/" + pluginName
	http.StripPrefix(prefix, proxy).ServeHTTP(w, r)
}

// PluginInfo returns info for the gateway to use in responses
type PluginInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Running     bool   `json:"running"`
	Enabled     bool   `json:"enabled"`
	ProxyURL    string `json:"proxyUrl"` // e.g., /plugins/webshell/
}

// GetPluginInfo returns gateway info for a plugin
func (g *Gateway) GetPluginInfo(name string) (*PluginInfo, error) {
	manifest, exists := g.registry.Get(name)
	if !exists {
		return nil, fmt.Errorf("plugin not found: %s", name)
	}

	state := g.store.Get(name)
	running := g.runtime.IsRunning(name)

	return &PluginInfo{
		Name:        manifest.Name,
		Description: manifest.Description,
		Running:     running,
		Enabled:     state.Enabled,
		ProxyURL:    fmt.Sprintf("/plugins/%s/", name),
	}, nil
}

// ListPluginURLs returns the proxy URLs for all enabled plugins
func (g *Gateway) ListPluginURLs() map[string]string {
	urls := make(map[string]string)
	for _, manifest := range g.registry.List() {
		state := g.store.Get(manifest.Name)
		if state.Enabled {
			urls[manifest.Name] = fmt.Sprintf("/plugins/%s/", manifest.Name)
		}
	}
	return urls
}
