#!/bin/bash
# ============================================================================
# OpsKernel Configuration - Decoupled from hardcoded values
# ============================================================================

# Paths (relative to project root)
OPSKERNEL_ROOT="${OPSKERNEL_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
COMPOSE_FILE="${COMPOSE_FILE:-docker/docker-compose.yml}"
PLUGIN_COMPOSE_FILE="${PLUGIN_COMPOSE_FILE:-docker/docker-compose.plugins.yml}"
ENV_FILE="${ENV_FILE:-.env}"
PLUGINCTL_BIN="${PLUGINCTL_BIN:-./bin/pluginctl}"
PLUGIN_TEMPLATE_DIR="${PLUGIN_TEMPLATE_DIR:-plugins/_template/go}"

# APT Mirror configuration
# Usage: APT_MIRROR=aliyun ./opskernel build core
# Supported aliases: aliyun, tuna, tencent, ustc, 163, huawei
# Or use custom URL: APT_MIRROR=mirrors.example.com
APT_MIRROR="${APT_MIRROR:-}"

# Resolve APT mirror alias to URL
resolve_apt_mirror() {
    local mirror="$1"
    case "$mirror" in
        aliyun)  echo "mirrors.aliyun.com" ;;
        tuna)    echo "mirrors.tuna.tsinghua.edu.cn" ;;
        tencent) echo "mirrors.cloud.tencent.com" ;;
        ustc)    echo "mirrors.ustc.edu.cn" ;;
        163)     echo "mirrors.163.com" ;;
        huawei)  echo "repo.huaweicloud.com" ;;
        "")      echo "" ;;
        *)       echo "$mirror" ;;  # Custom URL
    esac
}

# Export build args for docker compose
get_build_args() {
    local args=""
    if [[ -n "$APT_MIRROR" ]]; then
        args="--build-arg APT_MIRROR=$APT_MIRROR"
    fi
    echo "$args"
}

# Core services
CORE_SERVICES=(
    "opskernel"
    "docker-socket-proxy"
)

# Plugins - dynamically discovered or configured
_discover_plugins() {
    local plugins=()
    local plugins_dir="$OPSKERNEL_ROOT/plugins"
    
    if [[ -d "$plugins_dir" ]]; then
        for dir in "$plugins_dir"/*/; do
            [[ -d "$dir" ]] || continue
            local name=$(basename "$dir")
            # Skip templates and hidden dirs
            [[ "$name" == _* || "$name" == .* ]] && continue
            # Must have manifest.json
            [[ -f "$dir/manifest.json" ]] && plugins+=("$name")
        done
    fi
    
    # Fallback to defaults if none found
    if [[ ${#plugins[@]} -eq 0 ]]; then
        plugins=("webshell" "filemanager" "db-explorer" "perf-report")
    fi
    
    echo "${plugins[@]}"
}

# Initialize plugins list (lazy)
_init_plugins() {
    if [[ -z "${AVAILABLE_PLUGINS+x}" ]]; then
        read -ra AVAILABLE_PLUGINS <<< "$(_discover_plugins)"
    fi
}

# Get available plugins
get_plugins() {
    _init_plugins
    echo "${AVAILABLE_PLUGINS[@]}"
}

# Get plugin service names (for docker-compose)
get_plugin_services() {
    _init_plugins
    local services=()
    for p in "${AVAILABLE_PLUGINS[@]}"; do
        services+=("plugin-$p")
    done
    echo "${services[@]}"
}

# Preset configurations
declare -A PRESET_MINIMAL=(
    [ENABLE_DOCKER]="false"
    [ENABLE_GPU]="false"
    [ENABLE_CRON]="false"
    [ENABLE_SSH]="false"
    [ENABLE_SYSTEMD]="false"
    [ENABLE_SENSORS]="false"
    [ENABLE_POWER]="false"
    [ENABLE_SYSTEM]="false"
)

declare -A PRESET_SERVER=(
    [ENABLE_GPU]="false"
    [ENABLE_POWER]="false"
)

declare -A PRESET_NO_DOCKER=(
    [ENABLE_DOCKER]="false"
)

# Export preset as env vars
apply_preset() {
    local preset_name="$1"
    local -n preset_ref="PRESET_${preset_name^^}"
    
    for key in "${!preset_ref[@]}"; do
        export "$key"="${preset_ref[$key]}"
    done
}
