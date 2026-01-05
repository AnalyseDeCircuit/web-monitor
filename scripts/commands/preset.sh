#!/bin/bash
# ============================================================================
# OpsKernel Preset Commands: preset {minimal|server|no-docker}
# ============================================================================

cmd_preset() {
    local mode="${1:-}"
    shift 2>/dev/null || true
    
    case "$mode" in
        minimal)    _preset_minimal "$@" ;;
        server)     _preset_server "$@" ;;
        no-docker)  _preset_no_docker "$@" ;;
        list)       _preset_list ;;
        *)          _preset_usage ;;
    esac
}

_preset_minimal() {
    apply_preset "MINIMAL"
    run_cmd "Starting minimal mode (CPU/Mem/Disk/Net only)" \
        docker compose -f "$OPSKERNEL_ROOT/$COMPOSE_FILE" up -d opskernel
    msg_success "Started in minimal mode"
}

_preset_server() {
    apply_preset "SERVER"
    run_cmd "Starting server mode (no GPU/Power)" \
        docker compose -f "$OPSKERNEL_ROOT/$COMPOSE_FILE" up -d opskernel docker-socket-proxy
    msg_success "Started in server mode"
}

_preset_no_docker() {
    apply_preset "NO_DOCKER"
    run_cmd "Starting without Docker management" \
        docker compose -f "$OPSKERNEL_ROOT/$COMPOSE_FILE" up -d opskernel
    msg_success "Started without Docker management"
}

_preset_list() {
    cat <<EOF
Available presets:

  minimal     CPU/Mem/Disk/Net only
              Disables: Docker, GPU, Cron, SSH, Systemd, Sensors, Power, System

  server      Server mode (no GPU/Power)
              Disables: GPU, Power

  no-docker   Without Docker management
              Disables: Docker module only
EOF
}

_preset_usage() {
    cat <<EOF
Usage: opskernel preset <mode>

Modes:
  minimal     Start with minimal features (CPU/Mem/Disk/Net only)
  server      Server mode without GPU/Power
  no-docker   Disable Docker management
  list        Show available presets

Examples:
  opskernel preset minimal
  opskernel preset server
EOF
}
