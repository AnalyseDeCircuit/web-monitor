#!/bin/bash
# ============================================================================
# OpsKernel Build Commands: build {core|all|rebuild|dev|clean}
# ============================================================================

cmd_build() {
    local action="${1:-core}"
    shift 2>/dev/null || true
    
    case "$action" in
        core)     _build_core "$@" ;;
        all)      _build_all "$@" ;;
        rebuild)  _build_rebuild "$@" ;;
        dev)      _build_dev "$@" ;;
        clean)    _build_clean "$@" ;;
        *)        _build_usage ;;
    esac
}

_build_core() {
    run_cmd "Building core images" core_compose build "${CORE_SERVICES[@]}"
    msg_success "Core images built"
}

_build_all() {
    # Build core
    run_cmd "Building core images" core_compose build "${CORE_SERVICES[@]}"
    
    # Build plugins
    local services=$(get_plugin_services)
    if [[ -n "$services" ]]; then
        run_cmd "Building plugin images" plugin_compose build $services
    fi
    
    # Start all
    run_cmd "Starting core services" core_compose up -d "${CORE_SERVICES[@]}"
    if [[ -n "$services" ]]; then
        run_cmd "Starting all plugins" plugin_compose up -d $services
    fi
    
    msg_success "Build & start complete"
    msg_dim "$(status_compact)"
}

_build_rebuild() {
    run_cmd "Stopping services" core_compose down
    run_cmd "Building core images" core_compose build "${CORE_SERVICES[@]}"
    run_cmd "Starting services" core_compose up -d "${CORE_SERVICES[@]}"
    msg_success "Rebuild complete"
    msg_dim "$(status_compact)"
}

_build_dev() {
    msg_step "Running locally with Go"
    cd "$OPSKERNEL_ROOT"
    HOST_FS="" HOST_PROC="/proc" HOST_SYS="/sys" HOST_ETC="/etc" HOST_VAR="/var" HOST_RUN="/run" \
        go run cmd/server/main.go
}

_build_clean() {
    msg_step "Cleaning build artifacts"
    rm -f "$OPSKERNEL_ROOT/server" "$OPSKERNEL_ROOT/cmd/server/server"
    msg_success "Build artifacts cleaned"
}

_build_usage() {
    cat <<EOF
Usage: opskernel build <command>

Commands:
  core      Build core Docker images (default)
  all       Build all images (core + plugins)
  rebuild   down + build + up
  dev       Run locally with Go (no Docker)
  clean     Remove build artifacts

Examples:
  opskernel build
  opskernel build all
  opskernel build dev
EOF
}
