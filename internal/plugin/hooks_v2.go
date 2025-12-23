// Package plugin provides hooks handling.
// In v2, hooks are NOT EXECUTED by the core. They are only displayed
// as manual setup instructions to the user.
//
// This file provides helpers to format hooks as human-readable instructions.
package plugin

import (
	"fmt"
	"strings"

	"github.com/AnalyseDeCircuit/opskernel/internal/plugin/registry"
)

// HookInstruction represents a manual setup step for the user
type HookInstruction struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Command     string `json:"command,omitempty"`
	Warning     string `json:"warning,omitempty"`
}

// LegacyInstallHook represents an install hook from old plugin.json format
// This is only used to parse and display as instructions
type LegacyInstallHook struct {
	Type      string `json:"type"`
	User      string `json:"user,omitempty"`
	Shell     string `json:"shell,omitempty"`
	Algorithm string `json:"algorithm,omitempty"`
	Path      string `json:"path,omitempty"`
	KeyPath   string `json:"keyPath,omitempty"`
	Content   string `json:"content,omitempty"`
	Mode      string `json:"mode,omitempty"`
	Command   string `json:"command,omitempty"`
}

// LegacyInstallConfig from old plugin.json
type LegacyInstallConfig struct {
	RequiresApproval bool                `json:"requiresApproval,omitempty"`
	Hooks            []LegacyInstallHook `json:"hooks,omitempty"`
}

// LegacyUninstallConfig from old plugin.json
type LegacyUninstallConfig struct {
	Hooks []LegacyInstallHook `json:"hooks,omitempty"`
}

// FormatHooksAsInstructions converts legacy hooks to human-readable instructions
// Returns instructions that the user must perform manually
func FormatHooksAsInstructions(hooks []LegacyInstallHook, pluginName string) []HookInstruction {
	var instructions []HookInstruction

	for _, hook := range hooks {
		inst := formatSingleHook(hook, pluginName)
		if inst != nil {
			instructions = append(instructions, *inst)
		}
	}

	return instructions
}

func formatSingleHook(hook LegacyInstallHook, pluginName string) *HookInstruction {
	switch hook.Type {
	case "ensure-user":
		shell := hook.Shell
		if shell == "" {
			shell = "/bin/bash"
		}
		return &HookInstruction{
			Type:        hook.Type,
			Description: fmt.Sprintf("Create system user '%s' for plugin %s", hook.User, pluginName),
			Command:     fmt.Sprintf("sudo useradd --system --shell %s --home /home/%s --create-home %s", shell, hook.User, hook.User),
			Warning:     "This creates a system user on the host. Only run if you trust this plugin.",
		}

	case "generate-ssh-key":
		algo := hook.Algorithm
		if algo == "" {
			algo = "ed25519"
		}
		return &HookInstruction{
			Type:        hook.Type,
			Description: fmt.Sprintf("Generate SSH key for plugin %s", pluginName),
			Command:     fmt.Sprintf("sudo ssh-keygen -t %s -f %s -N '' -C 'opskernel-plugin-%s'", algo, hook.KeyPath, pluginName),
			Warning:     "This generates SSH keys on the host.",
		}

	case "authorize-key":
		return &HookInstruction{
			Type:        hook.Type,
			Description: fmt.Sprintf("Add plugin's public key to user '%s' authorized_keys", hook.User),
			Command:     fmt.Sprintf("sudo cat %s >> /home/%s/.ssh/authorized_keys", hook.KeyPath, hook.User),
			Warning:     "This allows the plugin to SSH as this user.",
		}

	case "remove-authorized-key":
		return &HookInstruction{
			Type:        hook.Type,
			Description: fmt.Sprintf("Remove plugin's public key from user '%s' authorized_keys", hook.User),
			Command:     fmt.Sprintf("# Edit /home/%s/.ssh/authorized_keys and remove the line containing 'opskernel-plugin-%s'", hook.User, pluginName),
		}

	case "create-directory":
		mode := hook.Mode
		if mode == "" {
			mode = "755"
		}
		return &HookInstruction{
			Type:        hook.Type,
			Description: fmt.Sprintf("Create directory %s", hook.Path),
			Command:     fmt.Sprintf("sudo mkdir -p -m %s %s", mode, hook.Path),
		}

	case "write-config":
		return &HookInstruction{
			Type:        hook.Type,
			Description: fmt.Sprintf("Write configuration to %s", hook.Path),
			Command:     fmt.Sprintf("# Create/edit %s with the plugin's required configuration", hook.Path),
			Warning:     "Review the configuration content before writing.",
		}

	case "remove-file":
		return &HookInstruction{
			Type:        hook.Type,
			Description: fmt.Sprintf("Remove file %s", hook.Path),
			Command:     fmt.Sprintf("sudo rm -f %s", hook.Path),
			Warning:     "Ensure this file is no longer needed before removing.",
		}

	default:
		return &HookInstruction{
			Type:        hook.Type,
			Description: fmt.Sprintf("Unknown hook type: %s", hook.Type),
			Warning:     "This hook type is not recognized. Manual intervention required.",
		}
	}
}

// GetManualSetupInstructions returns setup instructions for a plugin
// This should be shown to the user before enabling privileged plugins
func GetManualSetupInstructions(manifest *registry.Manifest) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Manual Setup Instructions for %s\n\n", manifest.Name))

	if manifest.IsHighRisk() {
		sb.WriteString("⚠️  WARNING: This plugin has elevated risk level.\n")
		sb.WriteString("Review all instructions carefully before proceeding.\n\n")
	}

	// Note about hooks
	sb.WriteString("## Important Notice\n\n")
	sb.WriteString("OpsKernel does not execute host commands for security reasons.\n")
	sb.WriteString("If this plugin requires host-level setup (like creating users or SSH keys),\n")
	sb.WriteString("you must perform these steps manually.\n\n")

	// Permissions
	if len(manifest.Permissions) > 0 {
		sb.WriteString("## Required Permissions\n\n")
		for _, p := range manifest.Permissions {
			sb.WriteString(fmt.Sprintf("- %s\n", p))
		}
		sb.WriteString("\n")
	}

	// Docker configuration summary
	sb.WriteString("## Container Configuration\n\n")
	sb.WriteString(fmt.Sprintf("- Image: %s\n", manifest.Docker.Image))
	sb.WriteString(fmt.Sprintf("- Internal Port: %d\n", manifest.Docker.Port))
	if manifest.Docker.Network != "" {
		sb.WriteString(fmt.Sprintf("- Network: %s\n", manifest.Docker.Network))
	}
	if len(manifest.Docker.Volumes) > 0 {
		sb.WriteString("- Volumes:\n")
		for _, v := range manifest.Docker.Volumes {
			mode := "rw"
			if v.ReadOnly {
				mode = "ro"
			}
			sb.WriteString(fmt.Sprintf("  - %s -> %s (%s)\n", v.Source, v.Target, mode))
		}
	}
	sb.WriteString("\n")

	// Security warnings
	if manifest.Docker.Security != nil {
		sb.WriteString("## Security Notes\n\n")
		if manifest.Docker.Security.Privileged {
			sb.WriteString("⚠️  This container runs in PRIVILEGED mode with full host access.\n")
		}
		if len(manifest.Docker.Security.CapAdd) > 0 {
			sb.WriteString(fmt.Sprintf("- Added capabilities: %v\n", manifest.Docker.Security.CapAdd))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// DEPRECATED: ValidateHooksV2 is no longer used.
// Hooks are not executed by OpsKernel v2.
func ValidateHooksV2(manifest interface{}) error {
	// No-op - hooks are displayed as instructions, not executed
	return nil
}
