// Package store handles plugin state persistence.
// This includes enabled state, confirmation records, and error tracking.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultStatePath = "/data/plugins-state.json"
)

// PluginState represents the persisted state of a plugin
type PluginState struct {
	// Whether the plugin is enabled
	Enabled bool `json:"enabled"`

	// Whether the user has confirmed the risk/permissions
	Confirmed bool `json:"confirmed"`

	// Timestamp of confirmation
	ConfirmedAt *time.Time `json:"confirmedAt,omitempty"`

	// User who confirmed
	ConfirmedBy string `json:"confirmedBy,omitempty"`

	// Hash of the manifest at confirmation time (to detect changes)
	ManifestHash string `json:"manifestHash,omitempty"`

	// Last error encountered
	LastError string `json:"lastError,omitempty"`

	// Last error timestamp
	LastErrorAt *time.Time `json:"lastErrorAt,omitempty"`

	// Last successful start timestamp
	LastStartedAt *time.Time `json:"lastStartedAt,omitempty"`

	// Runtime state (not persisted, set after container inspection)
	Running bool `json:"-"`
}

// Confirmation represents the user's explicit approval of a plugin
type Confirmation struct {
	PluginName string `json:"pluginName"`
	Username   string `json:"username"`

	// Acknowledged items
	AcknowledgedRisk         string   `json:"acknowledgedRisk"`
	AcknowledgedPermissions  []string `json:"acknowledgedPermissions"`
	AcknowledgedDockerParams []string `json:"acknowledgedDockerParams,omitempty"`

	// Explicit acknowledgment flag
	ExplicitApproval bool `json:"explicitApproval"`

	// Timestamp
	Timestamp time.Time `json:"timestamp"`
}

// Store manages plugin state persistence
type Store struct {
	statePath string
	states    map[string]*PluginState
	mu        sync.RWMutex
}

// NewStore creates a new plugin state store
func NewStore(statePath string) *Store {
	if statePath == "" {
		statePath = defaultStatePath
	}
	return &Store{
		statePath: statePath,
		states:    make(map[string]*PluginState),
	}
}

// Load reads state from disk
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			// No state file yet, start fresh
			s.states = make(map[string]*PluginState)
			return nil
		}
		return fmt.Errorf("failed to read state file: %w", err)
	}

	var states map[string]*PluginState
	if err := json.Unmarshal(data, &states); err != nil {
		return fmt.Errorf("failed to parse state file: %w", err)
	}

	s.states = states
	return nil
}

// Save writes state to disk
func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.MarshalIndent(s.states, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(s.statePath), 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	if err := os.WriteFile(s.statePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

// Get returns the state for a plugin
func (s *Store) Get(name string) *PluginState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if state, ok := s.states[name]; ok {
		// Return a copy
		cp := *state
		return &cp
	}

	// Return default state
	return &PluginState{
		Enabled:   false,
		Confirmed: false,
	}
}

// Set updates the state for a plugin
func (s *Store) Set(name string, state *PluginState) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.states[name] = state
}

// SetEnabled updates the enabled state and saves
func (s *Store) SetEnabled(name string, enabled bool) error {
	s.mu.Lock()
	state, ok := s.states[name]
	if !ok {
		state = &PluginState{}
		s.states[name] = state
	}
	state.Enabled = enabled
	if enabled {
		now := time.Now()
		state.LastStartedAt = &now
	}
	s.mu.Unlock()

	return s.Save()
}

// SetConfirmed records user confirmation
func (s *Store) SetConfirmed(name string, confirmation *Confirmation) error {
	s.mu.Lock()
	state, ok := s.states[name]
	if !ok {
		state = &PluginState{}
		s.states[name] = state
	}
	state.Confirmed = true
	state.ConfirmedAt = &confirmation.Timestamp
	state.ConfirmedBy = confirmation.Username
	s.mu.Unlock()

	return s.Save()
}

// SetError records an error for a plugin
func (s *Store) SetError(name string, errMsg string) error {
	s.mu.Lock()
	state, ok := s.states[name]
	if !ok {
		state = &PluginState{}
		s.states[name] = state
	}
	state.LastError = errMsg
	now := time.Now()
	state.LastErrorAt = &now
	s.mu.Unlock()

	return s.Save()
}

// ClearError clears the error for a plugin
func (s *Store) ClearError(name string) error {
	s.mu.Lock()
	if state, ok := s.states[name]; ok {
		state.LastError = ""
		state.LastErrorAt = nil
	}
	s.mu.Unlock()

	return s.Save()
}

// IsEnabled returns whether a plugin is enabled
func (s *Store) IsEnabled(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if state, ok := s.states[name]; ok {
		return state.Enabled
	}
	return false
}

// IsConfirmed returns whether a plugin has been confirmed
func (s *Store) IsConfirmed(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if state, ok := s.states[name]; ok {
		return state.Confirmed
	}
	return false
}

// GetEnabledPlugins returns names of all enabled plugins
func (s *Store) GetEnabledPlugins() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var enabled []string
	for name, state := range s.states {
		if state.Enabled {
			enabled = append(enabled, name)
		}
	}
	return enabled
}

// GetAllStates returns all plugin states (for status API)
func (s *Store) GetAllStates() map[string]*PluginState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*PluginState, len(s.states))
	for name, state := range s.states {
		cp := *state
		result[name] = &cp
	}
	return result
}

// MigrateFromLegacy migrates from the old enabled.json format
func (s *Store) MigrateFromLegacy(legacyPath string) error {
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Nothing to migrate
		}
		return err
	}

	var legacyState map[string]bool
	if err := json.Unmarshal(data, &legacyState); err != nil {
		return err
	}

	s.mu.Lock()
	for name, enabled := range legacyState {
		if _, ok := s.states[name]; !ok {
			s.states[name] = &PluginState{}
		}
		s.states[name].Enabled = enabled
		// Mark as confirmed since it was previously enabled
		if enabled {
			s.states[name].Confirmed = true
		}
	}
	s.mu.Unlock()

	// Save new format
	if err := s.Save(); err != nil {
		return err
	}

	// Rename old file to mark as migrated
	return os.Rename(legacyPath, legacyPath+".migrated")
}

// ============================================================================
// Reconcile Support - State Drift Detection
// ============================================================================

// StateDrift represents a mismatch between expected and actual state
type StateDrift struct {
	PluginName    string `json:"pluginName"`
	ExpectedState string `json:"expectedState"` // "running" or "stopped"
	ActualState   string `json:"actualState"`
	Action        string `json:"action"` // "start" or "stop" or "none"
}

// GetExpectedRunningPlugins returns names of plugins that should be running
// (i.e., enabled plugins with no persistent error)
func (s *Store) GetExpectedRunningPlugins() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var expected []string
	for name, state := range s.states {
		if state.Enabled {
			expected = append(expected, name)
		}
	}
	return expected
}

// RecordReconcileError records an error encountered during reconcile
func (s *Store) RecordReconcileError(name string, err error) {
	s.mu.Lock()
	state, ok := s.states[name]
	if !ok {
		state = &PluginState{}
		s.states[name] = state
	}
	state.LastError = fmt.Sprintf("reconcile: %v", err)
	now := time.Now()
	state.LastErrorAt = &now
	s.mu.Unlock()

	// Best-effort save (don't fail reconcile if save fails)
	_ = s.Save()
}

// RecordReconcileSuccess clears reconcile errors after successful reconcile
func (s *Store) RecordReconcileSuccess(name string) {
	s.mu.Lock()
	if state, ok := s.states[name]; ok {
		// Only clear if it was a reconcile error
		if strings.HasPrefix(state.LastError, "reconcile:") {
			state.LastError = ""
			state.LastErrorAt = nil
		}
	}
	s.mu.Unlock()

	_ = s.Save()
}
