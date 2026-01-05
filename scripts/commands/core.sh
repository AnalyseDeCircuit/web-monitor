#!/bin/bash
# ============================================================================
# OpsKernel Core Commands: core {up|down|restart|logs|stats|status}
# ============================================================================

cmd_core() {
    local action="${1:-status}"
    shift 2>/dev/null || true
    
    case "$action" in
        up|start)     _core_up "$@" ;;
        down|stop)    _core_down "$@" ;;
        restart)      _core_restart "$@" ;;
        logs)         _core_logs "$@" ;;
        stats)        _core_stats "$@" ;;
        status)       _core_status "$@" ;;
        *)            _core_usage ;;
    esac
}

_core_up() {
    require_docker
    
    local st=$(service_status core "opskernel")
    if [[ "$st" == "running" ]]; then
        msg_info "Core is already running"
        return 0
    fi
    
    run_cmd "Starting core services" core_compose up -d "${CORE_SERVICES[@]}"
    msg_success "Core services started"
    msg_dim "$(status_compact)"
}

_core_down() {
    require_docker
    run_cmd "Stopping all services" core_compose down
    msg_success "All services stopped"
}

_core_restart() {
    require_docker
    msg_step "Restarting core services"
    core_compose stop "${CORE_SERVICES[@]}" 2>/dev/null || true
    run_cmd "Starting core services" core_compose up -d "${CORE_SERVICES[@]}"
    msg_success "Core services restarted"
    msg_dim "$(status_compact)"
}

_core_logs() {
    local follow=""
    [[ "$1" == "-f" || "$1" == "--follow" ]] && follow="-f"
    core_compose logs $follow opskernel
}

_core_stats() {
    core_compose stats
}

_core_status() {
    status_full
}

_core_usage() {
    cat <<EOF
Usage: opskernel core <command>

Commands:
  up, start     Start core services (opskernel + docker-socket-proxy)
  down, stop    Stop and remove all containers
  restart       Restart core services
  logs [-f]     View core logs (-f to follow)
  stats         View container resource usage
  status        Show detailed status (default)

Examples:
  opskernel core up
  opskernel core logs -f
EOF
}
