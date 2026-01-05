#!/bin/bash
# ============================================================================
# OpsKernel Common Functions
# ============================================================================

# Colors
readonly C_RED='\033[0;31m'
readonly C_GREEN='\033[0;32m'
readonly C_YELLOW='\033[1;33m'
readonly C_BLUE='\033[0;34m'
readonly C_DIM='\033[2m'
readonly C_BOLD='\033[1m'
readonly C_RESET='\033[0m'

# Output functions (CLI mode - no TUI)
msg_success() { echo -e "${C_GREEN}✓ $*${C_RESET}"; }
msg_error()   { echo -e "${C_RED}✗ $*${C_RESET}" >&2; }
msg_warn()    { echo -e "${C_YELLOW}! $*${C_RESET}"; }
msg_info()    { echo -e "${C_BLUE}→ $*${C_RESET}"; }
msg_dim()     { echo -e "${C_DIM}$*${C_RESET}"; }
msg_step()    { echo -e "${C_BOLD}▸ $*${C_RESET}"; }

# Die with error message
die() {
    msg_error "$@"
    exit 1
}

# Check if command exists
has_cmd() {
    command -v "$1" &>/dev/null
}

# Check whiptail availability
has_whiptail() {
    has_cmd whiptail
}

# Check Docker availability
check_docker() {
    has_cmd docker || die "Docker is not installed"
}

# Check if Docker daemon is reachable
docker_reachable() {
    if has_cmd timeout; then
        timeout 2 docker version &>/dev/null
    else
        docker version &>/dev/null
    fi
}

# Require Docker to be reachable
require_docker() {
    docker_reachable || die "Docker daemon is not reachable. Is Docker running?"
}

# Run command with real-time output (progress visible)
# Usage: run_cmd "description" command args...
run_cmd() {
    local desc="$1"; shift
    msg_step "$desc"
    
    # Run with output visible
    if "$@"; then
        return 0
    else
        local rc=$?
        msg_error "$desc failed (exit $rc)"
        return $rc
    fi
}

# Run command silently, show output only on error
# Usage: run_quiet "description" command args...
run_quiet() {
    local desc="$1"; shift
    local output
    
    if output=$("$@" 2>&1); then
        return 0
    else
        local rc=$?
        msg_error "$desc failed (exit $rc)"
        msg_dim "Command: $*"
        [[ -n "$output" ]] && echo "$output" >&2
        return $rc
    fi
}

# Alias for backward compatibility
run_or_die() {
    run_cmd "$@"
}

# Validate plugin name format
validate_plugin_name() {
    local name="$1"
    if ! [[ "$name" =~ ^[a-z][a-z0-9-]*[a-z0-9]$ ]]; then
        die "Invalid plugin name: $name (must be lowercase, start with letter, use a-z 0-9 - only)"
    fi
}

# Check if plugin exists
plugin_exists() {
    local name="$1"
    [[ -d "$OPSKERNEL_ROOT/plugins/$name" ]] && [[ -f "$OPSKERNEL_ROOT/plugins/$name/manifest.json" ]]
}

# Require plugin to exist
require_plugin() {
    local name="$1"
    plugin_exists "$name" || die "Plugin not found: $name"
}
