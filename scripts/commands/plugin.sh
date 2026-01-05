#!/bin/bash
# ============================================================================
# OpsKernel Plugin Commands: plugin {build|up|down|rm|logs|rebuild} <name>
# ============================================================================

cmd_plugin() {
    local action="${1:-}"
    local name="${2:-}"
    shift 2 2>/dev/null || true
    
    [[ -z "$action" ]] && { _plugin_usage; return 1; }
    
    case "$action" in
        build)   _plugin_build "$name" "$@" ;;
        create)  _plugin_create "$name" "$@" ;;
        up|start)     _plugin_up "$name" "$@" ;;
        down|stop)    _plugin_down "$name" "$@" ;;
        rm|remove)    _plugin_rm "$name" "$@" ;;
        logs)         _plugin_logs "$name" "$@" ;;
        rebuild)      _plugin_rebuild "$name" "$@" ;;
        list)         _plugin_list ;;
        *)            _plugin_usage ;;
    esac
}

_require_name() {
    if [[ -z "$1" ]]; then
        die "Plugin name required. Use 'opskernel plugin list' to see available plugins."
    fi
}

_plugin_build() {
    local name="$1"
    _require_name "$name"
    
    run_cmd "Building plugin: $name" plugin_compose build "plugin-$name"
    msg_success "Plugin $name built"
}

_plugin_create() {
    local name="$1"
    _require_name "$name"
    require_docker
    
    local st=$(service_status plugin "plugin-$name")
    if [[ "$st" != "not created" ]]; then
        msg_info "Plugin container already exists: $name ($st)"
        return 0
    fi
    
    run_cmd "Creating container: $name" plugin_compose up -d --no-start "plugin-$name"
    msg_success "Container for $name created (not started)"
}

_plugin_up() {
    local name="$1"
    _require_name "$name"
    require_docker
    
    local st=$(service_status plugin "plugin-$name")
    if [[ "$st" == "running" ]]; then
        msg_info "Plugin already running: $name"
        return 0
    fi
    
    # Use 'up -d' to create container if it doesn't exist
    run_cmd "Starting plugin: $name" plugin_compose up -d "plugin-$name"
    msg_success "Plugin $name started"
}

_plugin_down() {
    local name="$1"
    _require_name "$name"
    require_docker
    
    local st=$(service_status plugin "plugin-$name")
    if [[ "$st" == "not created" ]]; then
        msg_info "Plugin container does not exist: $name"
        return 0
    fi
    if [[ "$st" != "running" ]]; then
        msg_info "Plugin not running: $name ($st)"
        return 0
    fi
    
    run_cmd "Stopping plugin: $name" plugin_compose stop "plugin-$name"
    msg_success "Plugin $name stopped"
}

_plugin_rm() {
    local name="$1"
    _require_name "$name"
    require_docker
    
    local st=$(service_status plugin "plugin-$name")
    if [[ "$st" == "not created" ]]; then
        msg_info "Plugin container does not exist: $name"
        return 0
    fi
    
    run_cmd "Removing plugin: $name" plugin_compose rm -f "plugin-$name"
    msg_success "Plugin $name removed"
}

_plugin_logs() {
    local name="$1"
    shift 2>/dev/null || true
    _require_name "$name"
    
    local follow=""
    [[ "$1" == "-f" || "$1" == "--follow" ]] && follow="-f"
    plugin_compose logs $follow "plugin-$name"
}

_plugin_rebuild() {
    local name="$1"
    _require_name "$name"
    require_docker
    
    run_cmd "Building plugin: $name" plugin_compose build "plugin-$name"
    plugin_compose rm -f "plugin-$name" 2>/dev/null || true
    run_cmd "Creating container: $name" plugin_compose up -d --no-start "plugin-$name"
    msg_success "Plugin $name rebuilt"
}

_plugin_list() {
    echo "Available plugins:"
    for p in $(get_plugins); do
        local st=$(service_status plugin "plugin-$p")
        echo "  - $p: $st"
    done
}

_plugin_usage() {
    cat <<EOF
Usage: opskernel plugin <command> <name>

Commands:
  list              List available plugins with status
  build   <name>    Build plugin Docker image
  create  <name>    Create container (not start)
  up      <name>    Start plugin
  down    <name>    Stop plugin
  rm      <name>    Remove plugin container
  logs    <name> [-f]  View plugin logs
  rebuild <name>    Build + rm + create

Available plugins: $(get_plugins | tr ' ' ', ')

Examples:
  opskernel plugin list
  opskernel plugin up webshell
  opskernel plugin logs webshell -f
  opskernel plugin rebuild filemanager
EOF
}
