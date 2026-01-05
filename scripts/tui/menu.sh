#!/bin/bash
# ============================================================================
# OpsKernel TUI - Whiptail Interactive Menu
# Standalone module - requires whiptail
# ============================================================================

# Ensure we have required functions
[[ -z "$OPSKERNEL_ROOT" ]] && { echo "Error: OPSKERNEL_ROOT not set"; exit 1; }

# TUI output wrappers (override CLI versions)
tui_success() {
    whiptail --title "Success" --msgbox "$1" 8 72
}

tui_error() {
    whiptail --title "Error" --scrolltext --msgbox "$1" 18 80
}

tui_info() {
    whiptail --title "Info" --scrolltext --msgbox "$1" 18 80
}

tui_confirm() {
    whiptail --title "Confirm" --yesno "$1" 8 60
}

# ============================================================================
# Menus
# ============================================================================

tui_main_menu() {
    while true; do
        local status=$(status_compact)
        local choice
        choice=$(whiptail --title "OpsKernel Management" --menu "$status\n\nSelect:" 20 72 10 \
            "status"  "View detailed status" \
            "core"    "Core services →" \
            "preset"  "Preset modes →" \
            "plugin"  "Single plugin →" \
            "plugins" "All plugins →" \
            "build"   "Build & dev →" \
            "stats"   "Container stats" \
            "quick"   "Quick start (core + plugins)" \
            "help"    "Help" \
            "exit"    "Exit" \
            3>&1 1>&2 2>&3) || return 0
        
        case "$choice" in
            status)  status_full | tui_info ;;
            core)    tui_core_menu ;;
            preset)  tui_preset_menu ;;
            plugin)  tui_plugin_menu ;;
            plugins) tui_plugins_menu ;;
            build)   tui_build_menu ;;
            stats)   clear; core_compose stats ;;
            quick)   _tui_quick_start ;;
            help)    _tui_help ;;
            exit)    return 0 ;;
        esac
    done
}

tui_core_menu() {
    local status=$(status_compact)
    local choice
    choice=$(whiptail --title "Core Services" --menu "$status\n\nSelect:" 16 72 6 \
        "status"  "View status" \
        "up"      "Start core services" \
        "down"    "Stop all services" \
        "restart" "Restart core" \
        "logs"    "View logs" \
        "back"    "← Back" \
        3>&1 1>&2 2>&3) || return 0
    
    case "$choice" in
        status)  status_full | tui_info ;;
        up)      _core_up && tui_success "Core services started!\n\n$(status_compact)" ;;
        down)    _core_down && tui_success "All services stopped" ;;
        restart) _core_restart && tui_success "Core restarted!\n\n$(status_compact)" ;;
        logs)    clear; _core_logs -f ;;
        back)    return 0 ;;
    esac
}

tui_preset_menu() {
    local choice
    choice=$(whiptail --title "Preset Modes" --menu "Select a preset:" 15 60 4 \
        "minimal"   "CPU/Mem/Disk/Net only" \
        "server"    "No GPU/Power" \
        "no-docker" "Without Docker management" \
        "back"      "← Back" \
        3>&1 1>&2 2>&3) || return 0
    
    case "$choice" in
        minimal)   _preset_minimal && tui_success "Started in minimal mode" ;;
        server)    _preset_server && tui_success "Started in server mode" ;;
        no-docker) _preset_no_docker && tui_success "Started without Docker" ;;
        back)      return 0 ;;
    esac
}

tui_plugin_menu() {
    while true; do
        local status=$(status_compact)
        local choice
        choice=$(whiptail --title "Single Plugin" --menu "$status\n\nSelect operation:" 18 72 8 \
            "list"    "List plugins" \
            "build"   "Build image" \
            "create"  "Create container" \
            "up"      "Start" \
            "down"    "Stop" \
            "rm"      "Remove container" \
            "logs"    "View logs" \
            "rebuild" "Rebuild" \
            "back"    "← Back" \
            3>&1 1>&2 2>&3) || return 0
        
        [[ "$choice" == "back" ]] && return 0
        [[ "$choice" == "list" ]] && { _plugin_list | tui_info; continue; }
        
        local plugin=$(_tui_select_plugin)
        [[ -z "$plugin" || "$plugin" == "back" ]] && continue
        
        case "$choice" in
            build)   _plugin_build "$plugin" && tui_success "Plugin $plugin built" ;;
            create)  _plugin_create "$plugin" && tui_success "Container created" ;;
            up)      _plugin_up "$plugin" && tui_success "Plugin $plugin started" ;;
            down)    _plugin_down "$plugin" && tui_success "Plugin $plugin stopped" ;;
            rm)      tui_confirm "Remove container for $plugin?" && _plugin_rm "$plugin" && tui_success "Removed" ;;
            logs)    clear; _plugin_logs "$plugin" -f ;;
            rebuild) _plugin_rebuild "$plugin" && tui_success "Plugin $plugin rebuilt" ;;
        esac
    done
}

_tui_select_plugin() {
    local plugins=($(get_plugins))
    local menu_args=()
    
    for p in "${plugins[@]}"; do
        local st=$(service_status plugin "plugin-$p")
        menu_args+=("$p" "$st")
    done
    menu_args+=("back" "← Cancel")
    
    whiptail --title "Select Plugin" --menu "Choose:" 15 60 $((${#plugins[@]} + 1)) \
        "${menu_args[@]}" 3>&1 1>&2 2>&3
}

tui_plugins_menu() {
    local status=$(status_compact)
    local choice
    choice=$(whiptail --title "All Plugins" --menu "$status\n\nOperation for ALL plugins:" 16 72 7 \
        "status"  "Status" \
        "build"   "Build all" \
        "create"  "Create all containers" \
        "up"      "Start all" \
        "down"    "Stop all" \
        "rm"      "Remove all" \
        "rebuild" "Rebuild all" \
        "back"    "← Back" \
        3>&1 1>&2 2>&3) || return 0
    
    case "$choice" in
        status)  _plugins_status | tui_info ;;
        build)   _plugins_build && tui_success "All plugins built" ;;
        create)  _plugins_create && tui_success "All containers created" ;;
        up)      _plugins_up && tui_success "All plugins started" ;;
        down)    _plugins_down && tui_success "All plugins stopped" ;;
        rm)      tui_confirm "Remove ALL containers?" && _plugins_rm && tui_success "All removed" ;;
        rebuild) _plugins_rebuild && tui_success "All plugins rebuilt" ;;
        back)    return 0 ;;
    esac
}

tui_build_menu() {
    local choice
    choice=$(whiptail --title "Build & Development" --menu "Select:" 16 60 6 \
        "core"    "Build core images" \
        "all"     "Build all images" \
        "rebuild" "Rebuild (down+build+up)" \
        "dev"     "Run locally (Go)" \
        "clean"   "Clean artifacts" \
        "back"    "← Back" \
        3>&1 1>&2 2>&3) || return 0
    
    case "$choice" in
        core)    _build_core && tui_success "Core images built" ;;
        all)     _build_all && tui_success "All images built" ;;
        rebuild) _build_rebuild && tui_success "Rebuild complete" ;;
        dev)     clear; _build_dev ;;
        clean)   _build_clean && tui_success "Cleaned" ;;
        back)    return 0 ;;
    esac
}

_tui_quick_start() {
    msg_info "Quick starting..."
    core_compose up -d "${CORE_SERVICES[@]}"
    plugin_compose start $(get_plugin_services) 2>/dev/null || true
    tui_success "Core + all plugins started!\n\nAccess: http://localhost:38080\n\n$(status_compact)"
}

_tui_help() {
    whiptail --title "OpsKernel Help" --scrolltext --msgbox "
OpsKernel Management

CLI COMMANDS
────────────────────────────────────
  opskernel                    TUI menu (if whiptail available)
  opskernel core up            Start core services
  opskernel core logs -f       Follow core logs
  opskernel plugin up NAME     Start a plugin
  opskernel plugins up         Start all plugins
  opskernel preset minimal     Minimal mode
  opskernel build dev          Run locally with Go
  opskernel dev validate NAME  Validate manifest

CORE COMMANDS
────────────────────────────────────
  core up/down/restart/logs/stats

PLUGIN COMMANDS
────────────────────────────────────
  plugin list/build/create/up/down/rm/logs/rebuild <name>
  plugins build/create/up/down/rm/rebuild

PRESETS
────────────────────────────────────
  preset minimal   - CPU/Mem/Disk/Net only
  preset server    - No GPU/Power
  preset no-docker - Without Docker management

BUILD
────────────────────────────────────
  build core/all/rebuild/dev/clean

DEV
────────────────────────────────────
  dev validate <name>  - Validate manifest
  dev init <name>      - Create from template
" 28 72
}

# ============================================================================
# Entry
# ============================================================================

run_tui() {
    has_whiptail || die "whiptail not installed. Use CLI: opskernel help"
    tui_main_menu
}
