#!/bin/bash
# ============================================================================
# OpsKernel Docker/Compose Operations
# ============================================================================

# Core compose wrapper
core_compose() {
    local args=(-f "$OPSKERNEL_ROOT/$COMPOSE_FILE")
    [[ -f "$OPSKERNEL_ROOT/$ENV_FILE" ]] && args+=(--env-file "$OPSKERNEL_ROOT/$ENV_FILE")
    
    # Handle build command with APT_MIRROR
    if [[ -n "$APT_MIRROR" && "$1" == "build" ]]; then
        shift  # Remove 'build' from args
        docker compose "${args[@]}" build --build-arg "APT_MIRROR=$APT_MIRROR" "$@"
        return $?
    fi
    
    docker compose "${args[@]}" "$@"
}

# Plugin compose wrapper
plugin_compose() {
    local args=(-f "$OPSKERNEL_ROOT/$PLUGIN_COMPOSE_FILE")
    
    # Handle build command with APT_MIRROR
    if [[ -n "$APT_MIRROR" && "$1" == "build" ]]; then
        shift  # Remove 'build' from args
        docker compose "${args[@]}" build --build-arg "APT_MIRROR=$APT_MIRROR" "$@"
        return $?
    fi
    
    docker compose "${args[@]}" "$@"
}

# Get container ID for a service
container_id() {
    local kind="$1"  # core|plugin
    local service="$2"
    
    if [[ "$kind" == "core" ]]; then
        core_compose ps -q "$service" 2>/dev/null || true
    else
        plugin_compose ps -q "$service" 2>/dev/null || true
    fi
}

# Get container status
container_status() {
    local id="$1"
    [[ -z "$id" ]] && { echo "not_created"; return 0; }
    docker inspect -f '{{.State.Status}}' "$id" 2>/dev/null || echo "unknown"
}

# Get container exit code
container_exit_code() {
    local id="$1"
    [[ -z "$id" ]] && { echo ""; return 0; }
    docker inspect -f '{{.State.ExitCode}}' "$id" 2>/dev/null || echo ""
}

# Get service status (human readable)
service_status() {
    local kind="$1"
    local service="$2"
    
    local id=$(container_id "$kind" "$service")
    local status=$(container_status "$id")
    local exit_code=$(container_exit_code "$id")
    
    case "$status" in
        running)     echo "running" ;;
        exited|dead)
            if [[ -n "$exit_code" && "$exit_code" != "0" ]]; then
                echo "crashed (exit=$exit_code)"
            else
                echo "stopped"
            fi
            ;;
        not_created) echo "not created" ;;
        *)           echo "$status" ;;
    esac
}

# Compact status line
status_compact() {
    if ! docker_reachable; then
        echo "Docker: unavailable"
        return 0
    fi
    
    local core_status=$(service_status core "opskernel")
    
    local running=0 total=0
    for svc in $(get_plugin_services); do
        ((total++))
        local st=$(service_status plugin "$svc")
        [[ "$st" == "running" ]] && ((running++))
    done
    
    echo "Core: $core_status | Plugins: $running/$total running"
}

# Detailed status
status_full() {
    if ! docker_reachable; then
        msg_error "Docker daemon is not reachable"
        return 1
    fi
    
    echo "DOCKER"
    echo "  Status: reachable"
    echo ""
    echo "CORE SERVICES"
    for svc in "${CORE_SERVICES[@]}"; do
        local st=$(service_status core "$svc")
        echo "  - $svc: $st"
    done
    echo ""
    echo "PLUGINS"
    local running=0 created=0
    for svc in $(get_plugin_services); do
        local st=$(service_status plugin "$svc")
        echo "  - $svc: $st"
        [[ "$st" == "running" ]] && ((running++))
        [[ "$st" != "not created" ]] && ((created++))
    done
    local total=$(get_plugin_services | wc -w)
    echo ""
    echo "Summary: $running/$total plugins running ($created/$total created)"
}
