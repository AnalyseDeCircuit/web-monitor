#!/bin/bash
# ============================================================================
# OpsKernel Dev Commands: dev {validate|init} <name>
# ============================================================================

cmd_dev() {
    local action="${1:-}"
    local name="${2:-}"
    shift 2 2>/dev/null || true
    
    case "$action" in
        validate)  _dev_validate "$name" "$@" ;;
        init)      _dev_init "$name" "$@" ;;
        *)         _dev_usage ;;
    esac
}

_ensure_pluginctl() {
    if [[ ! -x "$OPSKERNEL_ROOT/$PLUGINCTL_BIN" ]]; then
        msg_info "Building pluginctl..."
        mkdir -p "$OPSKERNEL_ROOT/bin"
        (cd "$OPSKERNEL_ROOT" && go build -o "$PLUGINCTL_BIN" ./cmd/pluginctl) || die "Failed to build pluginctl"
        msg_success "pluginctl built"
    fi
}

_dev_validate() {
    local name="$1"
    [[ -z "$name" ]] && die "Usage: opskernel dev validate <plugin-name>"
    
    _ensure_pluginctl
    
    local plugin_dir="$OPSKERNEL_ROOT/plugins/$name"
    [[ -d "$plugin_dir" ]] || die "Plugin directory not found: $plugin_dir"
    
    "$OPSKERNEL_ROOT/$PLUGINCTL_BIN" validate "$plugin_dir"
}

_dev_init() {
    local name="$1"
    [[ -z "$name" ]] && die "Usage: opskernel dev init <plugin-name>"
    
    validate_plugin_name "$name"
    
    local target_dir="$OPSKERNEL_ROOT/plugins/$name"
    local template_dir="$OPSKERNEL_ROOT/$PLUGIN_TEMPLATE_DIR"
    
    [[ -d "$target_dir" ]] && die "Plugin directory already exists: $target_dir"
    [[ -d "$template_dir" ]] || die "Template directory not found: $template_dir"
    
    msg_info "Creating plugin: $name"
    
    # Copy template
    cp -r "$template_dir" "$target_dir"
    
    # Replace placeholders
    local name_title="${name^}"  # Capitalize first letter
    
    # manifest.json
    sed -i "s/my-plugin/$name/g" "$target_dir/manifest.json"
    sed -i "s/My Plugin/$name_title/g" "$target_dir/manifest.json"
    
    # go.mod
    [[ -f "$target_dir/go.mod" ]] && sed -i "s/plugin-template/plugin-$name/g" "$target_dir/go.mod"
    
    # main.go
    [[ -f "$target_dir/main.go" ]] && {
        sed -i "s/my-plugin/$name/g" "$target_dir/main.go"
        sed -i "s/My Plugin/$name_title/g" "$target_dir/main.go"
    }
    
    msg_success "Plugin initialized: $target_dir"
    echo ""
    echo "Next steps:"
    echo "  1. cd plugins/$name"
    echo "  2. Edit manifest.json (metadata, permissions, etc.)"
    echo "  3. Edit main.go (add your plugin logic)"
    echo "  4. opskernel dev validate $name"
    echo "  5. opskernel plugin build $name"
}

_dev_usage() {
    cat <<EOF
Usage: opskernel dev <command> <name>

Commands:
  validate <name>   Validate plugin manifest (v1 or v2)
  init <name>       Initialize new plugin from template

Examples:
  opskernel dev init my-plugin
  opskernel dev validate webshell
EOF
}
