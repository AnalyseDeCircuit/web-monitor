#!/bin/bash
# ============================================================================
# OpsKernel - Interactive Management Script (whiptail TUI)
# Equivalent to the original Makefile commands
# ============================================================================

set -e

# Colors for non-whiptail output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
COMPOSE_FILE="docker/docker-compose.yml"
PLUGIN_COMPOSE_FILE="docker/docker-compose.plugins.yml"
ENV_FILE=".env"
AVAILABLE_PLUGINS=("webshell" "filemanager" "db-explorer" "perf-report")

CORE_SERVICES=("opskernel" "docker-socket-proxy")
PLUGIN_SERVICES=("plugin-webshell" "plugin-filemanager" "plugin-db-explorer" "plugin-perf-report")

UI_MODE=0

# ============================================================================
# Helper Functions
# ============================================================================

check_whiptail() {
    if ! command -v whiptail &> /dev/null; then
        echo -e "${RED}Error: whiptail is not installed.${NC}"
        echo "Install it with: sudo apt install whiptail"
        exit 1
    fi
}

check_docker() {
    if ! command -v docker &> /dev/null; then
        echo -e "${RED}Error: docker is not installed.${NC}"
        exit 1
    fi
}

ui_success() {
    if [[ "${UI_MODE}" -eq 1 ]] && command -v whiptail &> /dev/null; then
        whiptail --title "Success" --msgbox "$1" 8 72
    else
        echo -e "${GREEN}$1${NC}"
    fi
}

ui_error() {
    if [[ "${UI_MODE}" -eq 1 ]] && command -v whiptail &> /dev/null; then
        whiptail --title "Error" --scrolltext --msgbox "$1" 18 80
    else
        echo -e "${RED}$1${NC}" >&2
    fi
}

ui_info() {
    if [[ "${UI_MODE}" -eq 1 ]] && command -v whiptail &> /dev/null; then
        whiptail --title "Info" --scrolltext --msgbox "$1" 18 80
    else
        echo -e "${YELLOW}$1${NC}"
    fi
}

core_compose() {
    if [[ -f "$ENV_FILE" ]]; then
        docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" "$@"
    else
        docker compose -f "$COMPOSE_FILE" "$@"
    fi
}

plugin_compose() {
    docker compose -f "$PLUGIN_COMPOSE_FILE" "$@"
}

run_or_report() {
    local action="$1"; shift
    local output

    if output=$("$@" 2>&1); then
        return 0
    else
        local rc=$?
        ui_error "$action failed (exit $rc)\n\nCommand: $*\n\n$output"
        return $rc
    fi
}

have_timeout() {
    command -v timeout &> /dev/null
}

docker_reachable() {
    if have_timeout; then
        timeout 2 docker version &> /dev/null
    else
        docker version &> /dev/null
    fi
}

container_id_for_service() {
    local compose_kind="$1" # core|plugin
    local service="$2"

    if [[ "$compose_kind" == "core" ]]; then
        core_compose ps -q "$service" 2>/dev/null || true
    else
        plugin_compose ps -q "$service" 2>/dev/null || true
    fi
}

container_status_for_id() {
    local id="$1"
    [[ -z "$id" ]] && { echo "not_created"; return 0; }
    docker inspect -f '{{.State.Status}}' "$id" 2>/dev/null || echo "unknown"
}

container_exit_code_for_id() {
    local id="$1"
    [[ -z "$id" ]] && { echo ""; return 0; }
    docker inspect -f '{{.State.ExitCode}}' "$id" 2>/dev/null || echo ""
}

core_status_compact() {
    if ! docker_reachable; then
        echo "Docker: unavailable | Core: unknown | Plugins: ?/4"
        return 0
    fi

    local ops_id ops_status
    ops_id=$(container_id_for_service core "opskernel")
    ops_status=$(container_status_for_id "$ops_id")

    local running_count=0
    for svc in "${PLUGIN_SERVICES[@]}"; do
        local pid pstatus
        pid=$(container_id_for_service plugin "$svc")
        pstatus=$(container_status_for_id "$pid")
        [[ "$pstatus" == "running" ]] && running_count=$((running_count+1))
    done

    local core_state
    case "$ops_status" in
        running) core_state="running" ;;
        exited|dead) core_state="stopped" ;;
        not_created) core_state="not_created" ;;
        *) core_state="$ops_status" ;;
    esac

    echo "Docker: ok | Core: ${core_state} | Plugins: ${running_count}/4 running"
}

show_status() {
    if ! docker_reachable; then
        ui_error "Docker daemon is not reachable.\n\n- Is Docker running?\n- Do you have permission (docker group/root)?\n\nTry: docker info"
        return 1
    fi

    local msg
    msg="DOCKER\n  Status: reachable\n\nCORE\n"

    for svc in "${CORE_SERVICES[@]}"; do
        local id st exitc
        id=$(container_id_for_service core "$svc")
        st=$(container_status_for_id "$id")
        exitc=$(container_exit_code_for_id "$id")
        if [[ "$st" == "not_created" ]]; then
            msg+="  - ${svc}: not created\n"
        elif [[ "$st" == "running" ]]; then
            msg+="  - ${svc}: running\n"
        elif [[ "$st" == "exited" || "$st" == "dead" ]]; then
            if [[ -n "$exitc" && "$exitc" != "0" ]]; then
                msg+="  - ${svc}: exited (crashed, exit=${exitc})\n"
            else
                msg+="  - ${svc}: exited (exit=${exitc})\n"
            fi
        else
            msg+="  - ${svc}: ${st}\n"
        fi
    done

    msg+="\nPLUGINS\n"
    local running_count=0
    local created_count=0
    for svc in "${PLUGIN_SERVICES[@]}"; do
        local id st exitc
        id=$(container_id_for_service plugin "$svc")
        st=$(container_status_for_id "$id")
        exitc=$(container_exit_code_for_id "$id")
        [[ "$st" != "not_created" ]] && created_count=$((created_count+1))
        [[ "$st" == "running" ]] && running_count=$((running_count+1))

        if [[ "$st" == "not_created" ]]; then
            msg+="  - ${svc}: not created\n"
        elif [[ "$st" == "running" ]]; then
            msg+="  - ${svc}: running\n"
        elif [[ "$st" == "exited" || "$st" == "dead" ]]; then
            if [[ -n "$exitc" && "$exitc" != "0" ]]; then
                msg+="  - ${svc}: stopped/crashed (exit=${exitc})\n"
            else
                msg+="  - ${svc}: stopped (exit=${exitc})\n"
            fi
        else
            msg+="  - ${svc}: ${st}\n"
        fi
    done

    msg+="\nSummary: Plugins ${running_count}/4 running (${created_count}/4 created)"
    ui_info "$msg"
}

# ============================================================================
# Core Services
# ============================================================================

core_up() {
    if ! docker_reachable; then
        ui_error "Docker daemon is not reachable.\n\nCannot start core services."
        return 1
    fi

    local ops_id ops_status
    ops_id=$(container_id_for_service core "opskernel")
    ops_status=$(container_status_for_id "$ops_id")
    if [[ "$ops_status" == "running" ]]; then
        ui_info "Core is already running.\n\n$(core_status_compact)"
        return 0
    fi

    echo "Starting core services..."
    run_or_report "Start core services" core_compose up -d opskernel docker-socket-proxy
    ui_success "Core services started successfully!\n\n$(core_status_compact)"
}

core_down() {
    if ! docker_reachable; then
        ui_error "Docker daemon is not reachable.\n\nCannot stop services cleanly."
        return 1
    fi

    echo "Stopping all services..."
    run_or_report "Stop core services" core_compose down
    ui_success "All services stopped and removed."
}

core_restart() {
    if ! docker_reachable; then
        ui_error "Docker daemon is not reachable.\n\nCannot restart core services."
        return 1
    fi

    echo "Restarting core services..."
    core_compose stop opskernel docker-socket-proxy 2>/dev/null || true
    run_or_report "Restart core services" core_compose up -d opskernel docker-socket-proxy
    ui_success "Core services restarted!\n\n$(core_status_compact)"
}

core_logs() {
    clear
    echo -e "${GREEN}Viewing core logs (Ctrl+C to exit)...${NC}"
    core_compose logs -f opskernel
}

core_stats() {
    clear
    echo -e "${GREEN}Container stats (Ctrl+C to exit)...${NC}"
    core_compose stats
}

# ============================================================================
# Preset Modes
# ============================================================================

preset_menu() {
    local choice
    choice=$(whiptail --title "Preset Modes" --menu "Select a preset mode:" 15 60 5 \
        "1" "Minimal (CPU/Mem/Disk/Net only)" \
        "2" "Server (no GPU/Power)" \
        "3" "No Docker (disable Docker management)" \
        "4" "Back to Main Menu" \
        3>&1 1>&2 2>&3)
    
    case $choice in
        1) up_minimal ;;
        2) up_server ;;
        3) up_no_docker ;;
        4) return ;;
    esac
}

up_minimal() {
    echo "Starting in minimal mode..."
    run_or_report "Start minimal mode" env \
        ENABLE_DOCKER=false ENABLE_GPU=false ENABLE_CRON=false ENABLE_SSH=false \
        ENABLE_SYSTEMD=false ENABLE_SENSORS=false ENABLE_POWER=false ENABLE_SYSTEM=false \
        docker compose -f "$COMPOSE_FILE" up -d opskernel
    ui_success "Started in minimal mode (CPU/Mem/Disk/Net only)"
}

up_server() {
    echo "Starting in server mode..."
    run_or_report "Start server mode" env \
        ENABLE_GPU=false ENABLE_POWER=false \
        docker compose -f "$COMPOSE_FILE" up -d opskernel docker-socket-proxy
    ui_success "Started in server mode (no GPU/Power)"
}

up_no_docker() {
    echo "Starting without Docker management..."
    run_or_report "Start without Docker management" env \
        ENABLE_DOCKER=false \
        docker compose -f "$COMPOSE_FILE" up -d opskernel
    ui_success "Started without Docker management"
}

# ============================================================================
# Single Plugin Operations
# ============================================================================

select_plugin() {
    local choice
    choice=$(whiptail --title "Select Plugin" --menu "Choose a plugin:" 15 60 6 \
        "webshell" "Terminal access via SSH" \
        "filemanager" "File management via SFTP" \
        "db-explorer" "Database browser (read-only)" \
        "perf-report" "Performance reports" \
        "back" "Back to previous menu" \
        3>&1 1>&2 2>&3)
    
    echo "$choice"
}

plugin_menu() {
    while true; do
        local choice
        local status
        status=$(core_status_compact)
        choice=$(whiptail --title "Single Plugin Operations" --menu "${status}\n\nSelect operation:" 20 72 9 \
            "1" "Build plugin image" \
            "2" "Create container (not start)" \
            "3" "Start plugin" \
            "4" "Stop plugin" \
            "5" "Remove plugin container" \
            "6" "View plugin logs" \
            "7" "Rebuild plugin (build + rm + create)" \
            "8" "Back to Main Menu" \
            3>&1 1>&2 2>&3)
        
        [[ -z "$choice" ]] && return
        [[ "$choice" == "8" ]] && return
        
        local plugin
        plugin=$(select_plugin)
        [[ -z "$plugin" || "$plugin" == "back" ]] && continue
        
        case $choice in
            1) plugin_build "$plugin" ;;
            2) plugin_create "$plugin" ;;
            3) plugin_up "$plugin" ;;
            4) plugin_down "$plugin" ;;
            5) plugin_rm "$plugin" ;;
            6) plugin_logs "$plugin" ;;
            7) plugin_rebuild "$plugin" ;;
        esac
    done
}

plugin_build() {
    local plugin="$1"
    echo "Building plugin: $plugin..."
    run_or_report "Build plugin $plugin" plugin_compose build "plugin-$plugin"
    ui_success "Plugin $plugin built successfully!"
}

plugin_create() {
    local plugin="$1"
    if ! docker_reachable; then
        ui_error "Docker daemon is not reachable.\n\nCannot create plugin container."
        return 1
    fi

    echo "Creating container for plugin: $plugin..."
    local id st
    id=$(container_id_for_service plugin "plugin-$plugin")
    st=$(container_status_for_id "$id")
    if [[ "$st" != "not_created" ]]; then
        ui_info "Plugin container already exists: $plugin ($st)."
        return 0
    fi

    run_or_report "Create plugin container $plugin" plugin_compose up -d --no-start "plugin-$plugin"
    ui_success "Container for $plugin created (not started)."
}

plugin_up() {
    local plugin="$1"
    if ! docker_reachable; then
        ui_error "Docker daemon is not reachable.\n\nCannot start plugin."
        return 1
    fi

    echo "Starting plugin: $plugin..."
    local id st
    id=$(container_id_for_service plugin "plugin-$plugin")
    st=$(container_status_for_id "$id")
    if [[ "$st" == "not_created" ]]; then
        ui_error "Plugin container is not created: $plugin\n\nRun: plugin-create $plugin"
        return 1
    fi
    if [[ "$st" == "running" ]]; then
        ui_info "Plugin is already running: $plugin"
        return 0
    fi

    run_or_report "Start plugin $plugin" plugin_compose start "plugin-$plugin"
    ui_success "Plugin $plugin started!"
}

plugin_down() {
    local plugin="$1"
    if ! docker_reachable; then
        ui_error "Docker daemon is not reachable.\n\nCannot stop plugin."
        return 1
    fi

    echo "Stopping plugin: $plugin..."
    local id st
    id=$(container_id_for_service plugin "plugin-$plugin")
    st=$(container_status_for_id "$id")
    if [[ "$st" == "not_created" ]]; then
        ui_info "Plugin container does not exist: $plugin"
        return 0
    fi
    if [[ "$st" != "running" ]]; then
        ui_info "Plugin is not running: $plugin ($st)"
        return 0
    fi

    run_or_report "Stop plugin $plugin" plugin_compose stop "plugin-$plugin"
    ui_success "Plugin $plugin stopped."
}

plugin_rm() {
    local plugin="$1"
    if ! docker_reachable; then
        ui_error "Docker daemon is not reachable.\n\nCannot remove plugin container."
        return 1
    fi

    local id st
    id=$(container_id_for_service plugin "plugin-$plugin")
    st=$(container_status_for_id "$id")
    if [[ "$st" == "not_created" ]]; then
        ui_info "Plugin container does not exist: $plugin"
        return 0
    fi

    if whiptail --title "Confirm" --yesno "Remove container for plugin $plugin?" 8 60; then
        echo "Removing plugin container: $plugin..."
        run_or_report "Remove plugin container $plugin" plugin_compose rm -f "plugin-$plugin"
        ui_success "Plugin container $plugin removed."
    fi
}

plugin_logs() {
    local plugin="$1"
    clear
    echo -e "${GREEN}Viewing logs for $plugin (Ctrl+C to exit)...${NC}"
    plugin_compose logs -f "plugin-$plugin"
}

plugin_rebuild() {
    local plugin="$1"
    if ! docker_reachable; then
        ui_error "Docker daemon is not reachable.\n\nCannot rebuild plugin."
        return 1
    fi

    echo "Rebuilding plugin: $plugin..."
    run_or_report "Build plugin $plugin" plugin_compose build "plugin-$plugin"
    plugin_compose rm -f "plugin-$plugin" 2>/dev/null || true
    run_or_report "Create plugin container $plugin" plugin_compose up -d --no-start "plugin-$plugin"
    ui_success "Plugin $plugin rebuilt successfully!"
}

# ============================================================================
# All Plugins Operations
# ============================================================================

all_plugins_menu() {
    local PLUGINS="plugin-webshell plugin-filemanager plugin-db-explorer plugin-perf-report"
    local status
    status=$(core_status_compact)
    
    local choice
    choice=$(whiptail --title "All Plugins Operations" --menu "${status}\n\nSelect operation for ALL plugins:" 18 72 8 \
        "1" "Build all plugin images" \
        "2" "Create all containers (not start)" \
        "3" "Start all plugins" \
        "4" "Stop all plugins" \
        "5" "Remove all plugin containers" \
        "6" "Rebuild all plugins" \
        "7" "Back to Main Menu" \
        3>&1 1>&2 2>&3)
    
    case $choice in
        1)
            echo "Building all plugins..."
            run_or_report "Build all plugins" plugin_compose build $PLUGINS
            ui_success "All plugins built!"
            ;;
        2)
            echo "Creating all plugin containers..."
            run_or_report "Create all plugin containers" plugin_compose up -d --no-start $PLUGINS
            ui_success "All plugin containers created!"
            ;;
        3)
            echo "Starting all plugins..."
            run_or_report "Start all plugins" plugin_compose start $PLUGINS
            ui_success "All plugins started!"
            ;;
        4)
            echo "Stopping all plugins..."
            run_or_report "Stop all plugins" plugin_compose stop $PLUGINS
            ui_success "All plugins stopped!"
            ;;
        5)
            if whiptail --title "Confirm" --yesno "Remove ALL plugin containers?" 8 60; then
                echo "Removing all plugin containers..."
                run_or_report "Remove all plugin containers" plugin_compose rm -f $PLUGINS
                ui_success "All plugin containers removed!"
            fi
            ;;
        6)
            echo "Rebuilding all plugins..."
            run_or_report "Build all plugins" plugin_compose build $PLUGINS
            plugin_compose rm -f $PLUGINS 2>/dev/null || true
            run_or_report "Create all plugin containers" plugin_compose up -d --no-start $PLUGINS
            ui_success "All plugins rebuilt!"
            ;;
        7) return ;;
    esac
}

# ============================================================================
# Build & Development
# ============================================================================

build_menu() {
    local choice
    choice=$(whiptail --title "Build & Development" --menu "Select operation:" 16 60 7 \
        "1" "Build core images" \
        "2" "Build ALL images (core + plugins)" \
        "3" "Rebuild (down + build + up)" \
        "4" "Run locally (Go, no Docker)" \
        "5" "Clean build artifacts" \
        "6" "Quick start (core + all plugins)" \
        "7" "Back to Main Menu" \
        3>&1 1>&2 2>&3)
    
    case $choice in
        1)
            echo "Building core images..."
            run_or_report "Build core images" core_compose build opskernel docker-socket-proxy
            ui_success "Core images built!"
            ;;
        2)
            echo "Building ALL images..."
            run_or_report "Build all images" core_compose build
            ui_success "All images built!"
            ;;
        3)
            echo "Rebuilding (down + build + up)..."
            run_or_report "Down core services" core_compose down
            run_or_report "Build core images" core_compose build opskernel docker-socket-proxy
            run_or_report "Up core services" core_compose up -d opskernel docker-socket-proxy
            ui_success "Rebuild complete!"
            ;;
        4)
            clear
            echo -e "${GREEN}Running locally with Go (Ctrl+C to exit)...${NC}"
            HOST_FS="" HOST_PROC="/proc" HOST_SYS="/sys" HOST_ETC="/etc" HOST_VAR="/var" HOST_RUN="/run" \
            go run cmd/server/main.go
            ;;
        5)
            echo "Cleaning build artifacts..."
            rm -f server cmd/server/server
            ui_success "Build artifacts cleaned!"
            ;;
        6)
            quick_start
            ;;
        7) return ;;
    esac
}

quick_start() {
    local PLUGINS="plugin-webshell plugin-filemanager plugin-db-explorer plugin-perf-report"
    
    echo "Quick starting core + all plugins..."
    run_or_report "Start core services" core_compose up -d opskernel docker-socket-proxy
    plugin_compose start $PLUGINS 2>/dev/null || true
    
    ui_success "Core services and all plugins started!\n\nAccess the dashboard at http://localhost:38080\n\n$(core_status_compact)"
}

# ============================================================================
# Main Menu
# ============================================================================

main_menu() {
    while true; do
        local choice
        local status
        status=$(core_status_compact)
        choice=$(whiptail --title "OpsKernel Management" --menu "${status}\n\nSelect a category:" 20 72 10 \
            "0" "View Status" \
            "1" "Core Services (up/down/restart/logs)" \
            "2" "Preset Modes (minimal/server/no-docker)" \
            "3" "Single Plugin Operations" \
            "4" "All Plugins Operations" \
            "5" "Build & Development" \
            "6" "View Container Stats" \
            "7" "Quick Start (core + all plugins)" \
            "8" "Help" \
            "9" "Exit" \
            3>&1 1>&2 2>&3)
        
        case $choice in
            0) show_status ;;
            1) core_menu ;;
            2) preset_menu ;;
            3) plugin_menu ;;
            4) all_plugins_menu ;;
            5) build_menu ;;
            6) core_stats ;;
            7) quick_start ;;
            8) show_help ;;
            9|"") exit 0 ;;
        esac
    done
}

core_menu() {
    local choice
    local status
    status=$(core_status_compact)
    choice=$(whiptail --title "Core Services" --menu "${status}\n\nSelect operation:" 16 72 6 \
        "0" "View Status" \
        "1" "Start core services" \
        "2" "Stop and remove all containers" \
        "3" "Restart core services" \
        "4" "View logs" \
        "5" "Back to Main Menu" \
        3>&1 1>&2 2>&3)
    
    case $choice in
        0) show_status ;;
        1) core_up ;;
        2) core_down ;;
        3) core_restart ;;
        4) core_logs ;;
        5) return ;;
    esac
}

show_help() {
    whiptail --title "OpsKernel Help" --scrolltext --msgbox "
╔══════════════════════════════════════════════════════════════════╗
║              OpsKernel - Management Commands                     ║
╚══════════════════════════════════════════════════════════════════╝

CORE SERVICES
─────────────────────────────────────────────────────────────────
  Start       - Start core services (opskernel + docker-proxy)
  Stop        - Stop and remove ALL containers
  Restart     - Restart core services (keeps plugins)
  Logs        - View core service logs (Ctrl+C to exit)
  Stats       - View container resource usage

PRESET MODES
─────────────────────────────────────────────────────────────────
  Minimal     - CPU/Mem/Disk/Net only
  Server      - No GPU/Power modules
  No Docker   - Without Docker management

PLUGINS
─────────────────────────────────────────────────────────────────
  Available: webshell, filemanager, db-explorer, perf-report
  
  Single plugin operations require selecting a plugin first.
  All plugins operations affect all 4 plugins at once.

BUILD & DEVELOPMENT
─────────────────────────────────────────────────────────────────
  Build       - Rebuild core Docker images
  Build All   - Build all images (core + plugins)
  Rebuild     - down + build + up
  Dev         - Run locally with Go (no Docker)
  Clean       - Remove build artifacts
  Quick Start - Start core + all plugins in one step

COMMAND LINE USAGE
─────────────────────────────────────────────────────────────────
  ./opskernel.sh                  Interactive menu
    ./opskernel.sh status            Show status summary
  ./opskernel.sh up               Start core services
  ./opskernel.sh down             Stop all services
  ./opskernel.sh restart          Restart core
  ./opskernel.sh logs             View logs
  ./opskernel.sh plugin-up NAME   Start a plugin
  ./opskernel.sh plugins-up       Start all plugins
  ./opskernel.sh all              Quick start everything
" 30 75
}

# ============================================================================
# CLI Mode (non-interactive)
# ============================================================================

cli_mode() {
    local cmd="$1"
    local arg="$2"
    
    case "$cmd" in
        status)
            show_status
            ;;
        up)
            core_compose up -d opskernel docker-socket-proxy
            echo -e "${GREEN}✓ Core services started${NC}"
            ;;
        down)
            core_compose down
            echo -e "${GREEN}✓ All services stopped${NC}"
            ;;
        restart)
            core_compose stop opskernel docker-socket-proxy 2>/dev/null || true
            core_compose up -d opskernel docker-socket-proxy
            echo -e "${GREEN}✓ Core services restarted${NC}"
            ;;
        logs)
            core_compose logs -f opskernel
            ;;
        stats)
            core_compose stats
            ;;
        up-minimal)
            ENABLE_DOCKER=false ENABLE_GPU=false ENABLE_CRON=false ENABLE_SSH=false \
            ENABLE_SYSTEMD=false ENABLE_SENSORS=false ENABLE_POWER=false ENABLE_SYSTEM=false \
            docker compose -f "$COMPOSE_FILE" up -d opskernel
            echo -e "${GREEN}✓ Started in minimal mode${NC}"
            ;;
        up-server)
            ENABLE_GPU=false ENABLE_POWER=false \
            docker compose -f "$COMPOSE_FILE" up -d opskernel docker-socket-proxy
            echo -e "${GREEN}✓ Started in server mode${NC}"
            ;;
        up-no-docker)
            ENABLE_DOCKER=false docker compose -f "$COMPOSE_FILE" up -d opskernel
            echo -e "${GREEN}✓ Started without Docker management${NC}"
            ;;
        plugin-build)
            [[ -z "$arg" ]] && { echo -e "${RED}Usage: $0 plugin-build <name>${NC}"; exit 1; }
            plugin_compose build "plugin-$arg"
            echo -e "${GREEN}✓ Plugin $arg built${NC}"
            ;;
        plugin-create)
            [[ -z "$arg" ]] && { echo -e "${RED}Usage: $0 plugin-create <name>${NC}"; exit 1; }
            plugin_compose up -d --no-start "plugin-$arg"
            echo -e "${GREEN}✓ Plugin $arg container created${NC}"
            ;;
        plugin-up)
            [[ -z "$arg" ]] && { echo -e "${RED}Usage: $0 plugin-up <name>${NC}"; exit 1; }
            plugin_compose start "plugin-$arg"
            echo -e "${GREEN}✓ Plugin $arg started${NC}"
            ;;
        plugin-down)
            [[ -z "$arg" ]] && { echo -e "${RED}Usage: $0 plugin-down <name>${NC}"; exit 1; }
            plugin_compose stop "plugin-$arg"
            echo -e "${GREEN}✓ Plugin $arg stopped${NC}"
            ;;
        plugin-rm)
            [[ -z "$arg" ]] && { echo -e "${RED}Usage: $0 plugin-rm <name>${NC}"; exit 1; }
            plugin_compose rm -f "plugin-$arg"
            echo -e "${GREEN}✓ Plugin $arg removed${NC}"
            ;;
        plugin-logs)
            [[ -z "$arg" ]] && { echo -e "${RED}Usage: $0 plugin-logs <name>${NC}"; exit 1; }
            plugin_compose logs -f "plugin-$arg"
            ;;
        plugin-rebuild)
            [[ -z "$arg" ]] && { echo -e "${RED}Usage: $0 plugin-rebuild <name>${NC}"; exit 1; }
            plugin_compose build "plugin-$arg"
            plugin_compose rm -f "plugin-$arg" 2>/dev/null || true
            plugin_compose up -d --no-start "plugin-$arg"
            echo -e "${GREEN}✓ Plugin $arg rebuilt${NC}"
            ;;
        plugins-build)
            plugin_compose build plugin-webshell plugin-filemanager plugin-db-explorer plugin-perf-report
            echo -e "${GREEN}✓ All plugins built${NC}"
            ;;
        plugins-create)
            plugin_compose up -d --no-start plugin-webshell plugin-filemanager plugin-db-explorer plugin-perf-report
            echo -e "${GREEN}✓ All plugin containers created${NC}"
            ;;
        plugins-up)
            plugin_compose start plugin-webshell plugin-filemanager plugin-db-explorer plugin-perf-report
            echo -e "${GREEN}✓ All plugins started${NC}"
            ;;
        plugins-down)
            plugin_compose stop plugin-webshell plugin-filemanager plugin-db-explorer plugin-perf-report
            echo -e "${GREEN}✓ All plugins stopped${NC}"
            ;;
        plugins-rm)
            plugin_compose rm -f plugin-webshell plugin-filemanager plugin-db-explorer plugin-perf-report
            echo -e "${GREEN}✓ All plugin containers removed${NC}"
            ;;
        plugins-rebuild)
            plugin_compose build plugin-webshell plugin-filemanager plugin-db-explorer plugin-perf-report
            plugin_compose rm -f plugin-webshell plugin-filemanager plugin-db-explorer plugin-perf-report 2>/dev/null || true
            plugin_compose up -d --no-start plugin-webshell plugin-filemanager plugin-db-explorer plugin-perf-report
            echo -e "${GREEN}✓ All plugins rebuilt${NC}"
            ;;
        build)
            core_compose build opskernel docker-socket-proxy
            echo -e "${GREEN}✓ Core images built${NC}"
            ;;
        build-all)
            core_compose build
            echo -e "${GREEN}✓ All images built${NC}"
            ;;
        rebuild)
            core_compose down
            core_compose build opskernel docker-socket-proxy
            core_compose up -d opskernel docker-socket-proxy
            echo -e "${GREEN}✓ Rebuild complete${NC}"
            ;;
        dev)
            HOST_FS="" HOST_PROC="/proc" HOST_SYS="/sys" HOST_ETC="/etc" HOST_VAR="/var" HOST_RUN="/run" \
            go run cmd/server/main.go
            ;;
        clean)
            rm -f server cmd/server/server
            echo -e "${GREEN}✓ Build artifacts cleaned${NC}"
            ;;
        all)
            core_compose up -d opskernel docker-socket-proxy
            plugin_compose start plugin-webshell plugin-filemanager plugin-db-explorer plugin-perf-report 2>/dev/null || true
            echo -e "${GREEN}✓ Core services and all plugins started!${NC}"
            ;;
        help|--help|-h)
            cat << 'EOF'
OpsKernel Management Script

Usage: ./opskernel.sh [command] [args]

Commands:
  (no args)       Launch interactive menu (requires whiptail)
    status          Show Docker/core/plugins status
  
  Core Services:
    up            Start core services
    down          Stop and remove all containers
    restart       Restart core services
    logs          View core logs
    stats         View container stats
  
  Preset Modes:
    up-minimal    Minimal mode (CPU/Mem/Disk/Net only)
    up-server     Server mode (no GPU/Power)
    up-no-docker  Without Docker management
  
  Single Plugin:
    plugin-build   <name>   Build plugin image
    plugin-create  <name>   Create container (not start)
    plugin-up      <name>   Start plugin
    plugin-down    <name>   Stop plugin
    plugin-rm      <name>   Remove plugin container
    plugin-logs    <name>   View plugin logs
    plugin-rebuild <name>   Rebuild plugin
  
  All Plugins:
    plugins-build           Build all plugins
    plugins-create          Create all containers
    plugins-up              Start all plugins
    plugins-down            Stop all plugins
    plugins-rm              Remove all containers
    plugins-rebuild         Rebuild all plugins
  
  Build:
    build         Build core images
    build-all     Build all images
    rebuild       down + build + up
    dev           Run locally with Go
    clean         Clean build artifacts
  
  Quick:
    all           Start core + all plugins

Available plugins: webshell, filemanager, db-explorer, perf-report
EOF
            ;;
        *)
            echo -e "${RED}Unknown command: $cmd${NC}"
            echo "Run '$0 help' for usage information"
            exit 1
            ;;
    esac
}

# ============================================================================
# Entry Point
# ============================================================================

cd "$(dirname "$0")"

check_docker

if [[ $# -gt 0 ]]; then
    # CLI mode
    cli_mode "$1" "$2"
else
    # Interactive mode
    UI_MODE=1
    check_whiptail
    main_menu
fi
