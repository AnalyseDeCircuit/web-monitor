// Package registry handles plugin discovery and registration.
package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Registry manages plugin discovery and manifest storage
type Registry struct {
	pluginsDir string
	manifests  map[string]*Manifest
	sources    map[string]string // name -> manifest path (for deprecation tracking)
	mu         sync.RWMutex
}

// NewRegistry creates a new plugin registry
func NewRegistry(pluginsDir string) *Registry {
	return &Registry{
		pluginsDir: pluginsDir,
		manifests:  make(map[string]*Manifest),
		sources:    make(map[string]string),
	}
}

// Discover scans the plugins directory and loads all manifests
func (r *Registry) Discover() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Clear existing
	r.manifests = make(map[string]*Manifest)
	r.sources = make(map[string]string)

	if r.pluginsDir == "" {
		return nil
	}

	entries, err := os.ReadDir(r.pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read plugins directory: %w", err)
	}

	var deprecationWarnings []string

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginPath := filepath.Join(r.pluginsDir, entry.Name())
		manifest, source, err := LoadManifestWithMigration(pluginPath)
		if err != nil {
			fmt.Printf("  - %s: skipped (%v)\n", entry.Name(), err)
			continue
		}

		// Validate manifest
		if err := manifest.Validate(); err != nil {
			fmt.Printf("  - %s: invalid manifest (%v)\n", entry.Name(), err)
			continue
		}

		// Track deprecation
		if filepath.Base(source) != "manifest.json" {
			deprecationWarnings = append(deprecationWarnings,
				fmt.Sprintf("Plugin '%s' uses deprecated plugin.json. Please migrate to manifest.json", manifest.Name))
		}

		r.manifests[manifest.Name] = manifest
		r.sources[manifest.Name] = source

		fmt.Printf("  + Registered plugin: %s (v%s, risk=%s)\n", manifest.Name, manifest.Version, manifest.Risk)
	}

	// Print deprecation warnings
	for _, w := range deprecationWarnings {
		fmt.Printf("  [DEPRECATION WARNING] %s\n", w)
	}

	return nil
}

// Get returns a manifest by name
func (r *Registry) Get(name string) (*Manifest, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.manifests[name]
	return m, ok
}

// List returns all registered manifests
func (r *Registry) List() []*Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*Manifest, 0, len(r.manifests))
	for _, m := range r.manifests {
		list = append(list, m)
	}
	return list
}

// GetSource returns the source path for a manifest (for deprecation tracking)
func (r *Registry) GetSource(name string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sources[name]
}

// IsDeprecatedFormat returns true if the plugin uses the old plugin.json format
func (r *Registry) IsDeprecatedFormat(name string) bool {
	source := r.GetSource(name)
	return source != "" && filepath.Base(source) != "manifest.json"
}

// Count returns the number of registered plugins
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.manifests)
}
