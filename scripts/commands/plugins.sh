#!/bin/bash
# ============================================================================
# OpsKernel Plugins (All) Commands: plugins {build|up|down|rm|rebuild}
# ============================================================================

cmd_plugins() {
    local action="${1:-status}"
    shift 2>/dev/null || true
    
    case "$action" in
        build)        _plugins_build "$@" ;;
        create)       _plugins_create "$@" ;;
        up|start)     _plugins_up "$@" ;;
        down|stop)    _plugins_down "$@" ;;
        rm|remove)    _plugins_rm "$@" ;;
        rebuild)      _plugins_rebuild "$@" ;;
        status)       _plugins_status "$@" ;;
        *)            _plugins_usage ;;
    esac
}

_get_services() {
    get_plugin_services
}

_plugins_build() {
    run_cmd "Building all plugins" plugin_compose build $(_get_services)
    msg_success "All plugins built"
}

_plugins_create() {
    require_docker
    run_cmd "Creating all plugin containers" plugin_compose up -d --no-start $(_get_services)
    msg_success "All plugin containers created"
}

_plugins_up() {
    require_docker
    # Use 'up -d' instead of 'start' to create containers if they don't exist
    run_cmd "Starting all plugins" plugin_compose up -d $(_get_services)
    msg_success "All plugins started"
    msg_dim "$(status_compact)"
}

_plugins_down() {
    require_docker
    run_cmd "Stopping all plugins" plugin_compose stop $(_get_services)
    msg_success "All plugins stopped"
}

_plugins_rm() {
    require_docker
    run_cmd "Removing all plugin containers" plugin_compose rm -f $(_get_services)
    msg_success "All plugin containers removed"
}

_plugins_rebuild() {
    require_docker
    run_cmd "Building all plugins" plugin_compose build $(_get_services)
    plugin_compose rm -f $(_get_services) 2>/dev/null || true
    run_cmd "Creating all containers" plugin_compose up -d --no-start $(_get_services)
    msg_success "All plugins rebuilt"
}

_plugins_status() {
    echo "Plugin Status:"
    for svc in $(_get_services); do
        local st=$(service_status plugin "$svc")
        echo "  - $svc: $st"
    done
}

_plugins_usage() {
    cat <<EOF
Usage: opskernel plugins <command>

Commands:
  status      Show status of all plugins (default)
  build       Build all plugin images
  create      Create all containers (not start)
  up          Start all plugins
  down        Stop all plugins
  rm          Remove all plugin containers
  rebuild     Build + rm + create all

Available plugins: $(get_plugins | tr ' ' ', ')

Examples:
  opskernel plugins up
  opskernel plugins rebuild
EOF
}
