// Package policy implements plugin security policy enforcement.
// It determines whether a plugin can be enabled based on risk level,
// permissions, user role, and confirmation status.
package policy

import (
	"fmt"

	"github.com/AnalyseDeCircuit/opskernel/internal/plugin/registry"
	"github.com/AnalyseDeCircuit/opskernel/internal/plugin/store"
)

// Policy enforces security rules for plugin operations
type Policy struct {
	// Whether to require explicit confirmation for all plugins
	RequireConfirmation bool

	// Whether to allow critical-risk plugins at all
	AllowCriticalRisk bool

	// Whether to allow privileged containers
	AllowPrivileged bool
}

// DefaultPolicy returns the default security policy
func DefaultPolicy() *Policy {
	return &Policy{
		RequireConfirmation: true,
		AllowCriticalRisk:   true, // Allow but require admin + confirmation
		AllowPrivileged:     true, // Allow but require admin + confirmation
	}
}

// StrictPolicy returns a stricter security policy
func StrictPolicy() *Policy {
	return &Policy{
		RequireConfirmation: true,
		AllowCriticalRisk:   false,
		AllowPrivileged:     false,
	}
}

// CheckEnableRequest represents a request to enable a plugin
type CheckEnableRequest struct {
	PluginName   string
	UserRole     string
	Username     string
	Confirmation *store.Confirmation
}

// CheckResult contains the result of a policy check
type CheckResult struct {
	Allowed bool
	Reason  string

	// If not allowed but can be with confirmation
	RequiresConfirmation bool
	ConfirmationPrompt   *ConfirmationPrompt
}

// ConfirmationPrompt describes what the user needs to confirm
type ConfirmationPrompt struct {
	Risk         registry.RiskLevel    `json:"risk"`
	Permissions  []registry.Permission `json:"permissions"`
	DockerParams []string              `json:"dockerParams"`
	Warnings     []string              `json:"warnings"`
}

// CheckEnable determines if a plugin can be enabled
func (p *Policy) CheckEnable(manifest *registry.Manifest, state *store.PluginState, req *CheckEnableRequest) CheckResult {
	// Check if plugin requires admin
	if manifest.AdminOnly && req.UserRole != "admin" {
		return CheckResult{
			Allowed: false,
			Reason:  "This plugin requires admin privileges",
		}
	}

	// Check if critical risk is allowed
	if manifest.Risk == registry.RiskCritical && !p.AllowCriticalRisk {
		return CheckResult{
			Allowed: false,
			Reason:  "Critical-risk plugins are not allowed by policy",
		}
	}

	// Check if privileged containers are allowed
	if manifest.Docker.Security != nil && manifest.Docker.Security.Privileged && !p.AllowPrivileged {
		return CheckResult{
			Allowed: false,
			Reason:  "Privileged containers are not allowed by policy",
		}
	}

	// High-risk and critical plugins require admin
	if manifest.IsHighRisk() && req.UserRole != "admin" {
		return CheckResult{
			Allowed: false,
			Reason:  "High-risk plugins require admin privileges",
		}
	}

	// Check confirmation requirement
	if p.RequireConfirmation {
		// If already confirmed and manifest hasn't changed, allow
		if state.Confirmed {
			// TODO: Compare manifest hash to detect changes
			return CheckResult{Allowed: true}
		}

		// Need confirmation
		if req.Confirmation == nil || !req.Confirmation.ExplicitApproval {
			summary := manifest.GetSecuritySummary()
			return CheckResult{
				Allowed:              false,
				Reason:               "Explicit confirmation required to enable this plugin",
				RequiresConfirmation: true,
				ConfirmationPrompt: &ConfirmationPrompt{
					Risk:         manifest.Risk,
					Permissions:  manifest.Permissions,
					DockerParams: summary.DockerParams,
					Warnings:     summary.Warnings,
				},
			}
		}

		// Validate confirmation
		if err := p.validateConfirmation(manifest, req.Confirmation); err != nil {
			return CheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("Invalid confirmation: %v", err),
			}
		}
	}

	return CheckResult{Allowed: true}
}

// validateConfirmation checks if the confirmation matches the manifest requirements
func (p *Policy) validateConfirmation(manifest *registry.Manifest, conf *store.Confirmation) error {
	// Check risk acknowledgment
	if conf.AcknowledgedRisk != string(manifest.Risk) {
		return fmt.Errorf("acknowledged risk '%s' does not match manifest risk '%s'",
			conf.AcknowledgedRisk, manifest.Risk)
	}

	// Check that all permissions are acknowledged
	manifestPerms := make(map[string]bool)
	for _, p := range manifest.Permissions {
		manifestPerms[string(p)] = true
	}

	for _, p := range conf.AcknowledgedPermissions {
		if !manifestPerms[p] {
			// Extra permission acknowledged is fine
		}
	}

	// Check that all manifest permissions are acknowledged
	for _, p := range manifest.Permissions {
		found := false
		for _, ack := range conf.AcknowledgedPermissions {
			if ack == string(p) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("permission '%s' not acknowledged", p)
		}
	}

	return nil
}

// CheckDisable determines if a plugin can be disabled
func (p *Policy) CheckDisable(manifest *registry.Manifest, state *store.PluginState, req *CheckEnableRequest) CheckResult {
	// Admin-only plugins can only be disabled by admin
	if manifest.AdminOnly && req.UserRole != "admin" {
		return CheckResult{
			Allowed: false,
			Reason:  "This plugin requires admin privileges to manage",
		}
	}

	// Generally allow disabling
	return CheckResult{Allowed: true}
}

// CheckInstall determines if a plugin can be installed (container created)
func (p *Policy) CheckInstall(manifest *registry.Manifest, userRole string) CheckResult {
	// Only admin can install plugins
	if userRole != "admin" {
		return CheckResult{
			Allowed: false,
			Reason:  "Only administrators can install plugins",
		}
	}

	// Check if critical risk is allowed
	if manifest.Risk == registry.RiskCritical && !p.AllowCriticalRisk {
		return CheckResult{
			Allowed: false,
			Reason:  "Critical-risk plugins are not allowed by policy",
		}
	}

	return CheckResult{Allowed: true}
}

// CheckUninstall determines if a plugin can be uninstalled
func (p *Policy) CheckUninstall(manifest *registry.Manifest, userRole string) CheckResult {
	// Only admin can uninstall plugins
	if userRole != "admin" {
		return CheckResult{
			Allowed: false,
			Reason:  "Only administrators can uninstall plugins",
		}
	}

	return CheckResult{Allowed: true}
}
