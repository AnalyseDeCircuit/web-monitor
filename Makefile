.PHONY: help up down restart logs stats build rebuild dev clean \
        up-minimal up-server up-no-docker \
        plugin-build plugin-create plugin-up plugin-down plugin-rm plugin-logs \
        plugins-build plugins-create plugins-up plugins-down plugins-rm

# ============================================================================
#  HELP
# ============================================================================
default: help

help:
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════════╗"
	@echo "║              Web Monitor - Management Commands                   ║"
	@echo "╚══════════════════════════════════════════════════════════════════╝"
	@echo ""
	@echo "┌─────────────────────────────────────────────────────────────────┐"
	@echo "│  CORE SERVICES                                                   │"
	@echo "├─────────────────────────────────────────────────────────────────┤"
	@echo "│  make up            Start core services (full mode)              │"
	@echo "│  make down          Stop and remove ALL containers               │"
	@echo "│  make restart       Restart core services (keeps plugins)        │"
	@echo "│  make logs          View core service logs                       │"
	@echo "│  make stats         View container resource usage                │"
	@echo "├─────────────────────────────────────────────────────────────────┤"
	@echo "│  PRESET MODES                                                    │"
	@echo "│  make up-minimal    Minimal mode (CPU/Mem/Disk/Net only)         │"
	@echo "│  make up-server     Server mode (no GPU/Power)                   │"
	@echo "│  make up-no-docker  Without Docker management                    │"
	@echo "└─────────────────────────────────────────────────────────────────┘"
	@echo ""
	@echo "┌─────────────────────────────────────────────────────────────────┐"
	@echo "│  PLUGINS - Single Plugin (specify P=name)                        │"
	@echo "├─────────────────────────────────────────────────────────────────┤"
	@echo "│  make plugin-build  P=webshell    Build single plugin            │"
	@echo "│  make plugin-create P=webshell    Create container (not start)   │"
	@echo "│  make plugin-up     P=webshell    Start plugin container         │"
	@echo "│  make plugin-down   P=webshell    Stop plugin container          │"
	@echo "│  make plugin-rm     P=webshell    Remove plugin container        │"
	@echo "│  make plugin-logs   P=webshell    View plugin logs               │"
	@echo "├─────────────────────────────────────────────────────────────────┤"
	@echo "│  PLUGINS - All Plugins                                           │"
	@echo "│  make plugins-build               Build all plugin images        │"
	@echo "│  make plugins-create              Create all plugin containers   │"
	@echo "│  make plugins-up                  Start all plugin containers    │"
	@echo "│  make plugins-down                Stop all plugin containers     │"
	@echo "│  make plugins-rm                  Remove all plugin containers   │"
	@echo "├─────────────────────────────────────────────────────────────────┤"
	@echo "│  PLUGIN SETUP                                                    │"
	@echo "│  Use the web UI (Settings > Plugins) or API:                     │"
	@echo "│    curl -X POST localhost:38080/api/plugins/install              │"
	@echo "│         -d '{\"name\":\"webshell\"}'                                │"
	@echo "└─────────────────────────────────────────────────────────────────┘"
	@echo ""
	@echo "┌─────────────────────────────────────────────────────────────────┐"
	@echo "│  BUILD & DEVELOPMENT                                             │"
	@echo "├─────────────────────────────────────────────────────────────────┤"
	@echo "│  make build         Rebuild core images                          │"
	@echo "│  make build-all     Build ALL images (core + plugins)            │"
	@echo "│  make rebuild       down + build + up                            │"
	@echo "│  make dev           Run locally (Go, no Docker)                  │"
	@echo "│  make clean         Clean up build artifacts                     │"
	@echo "└─────────────────────────────────────────────────────────────────┘"
	@echo ""
	@echo "  Available plugins: webshell, filemanager"
	@echo ""

# ============================================================================
#  CORE SERVICES
# ============================================================================

up:
	docker compose -f docker/docker-compose.yml up -d web-monitor-go docker-socket-proxy

down:
	docker compose -f docker/docker-compose.yml down

restart:
	docker compose -f docker/docker-compose.yml stop web-monitor-go docker-socket-proxy || true
	docker compose -f docker/docker-compose.yml up -d web-monitor-go docker-socket-proxy

logs:
	docker compose -f docker/docker-compose.yml logs -f web-monitor-go

stats:
	docker compose -f docker/docker-compose.yml stats

# ============================================================================
#  PRESET MODES
# ============================================================================

up-minimal:
	ENABLE_DOCKER=false ENABLE_GPU=false ENABLE_CRON=false ENABLE_SSH=false \
	ENABLE_SYSTEMD=false ENABLE_SENSORS=false ENABLE_POWER=false ENABLE_SYSTEM=false \
	docker compose -f docker/docker-compose.yml up -d web-monitor-go

up-server:
	ENABLE_GPU=false ENABLE_POWER=false \
	docker compose -f docker/docker-compose.yml up -d web-monitor-go docker-socket-proxy

up-no-docker:
	ENABLE_DOCKER=false docker compose -f docker/docker-compose.yml up -d web-monitor-go

# ============================================================================
#  SINGLE PLUGIN OPERATIONS (use P=pluginname)
# ============================================================================

# Validate plugin name
_check-plugin:
ifndef P
	$(error Please specify plugin name: make <target> P=webshell)
endif
ifeq ($(filter $(P),webshell filemanager),)
	$(error Unknown plugin '$(P)'. Available: webshell, filemanager)
endif

# Plugins use separate compose file for minimal intrusion
PLUGIN_COMPOSE := docker compose -f docker/docker-compose.plugins.yml

plugin-build: _check-plugin
	$(PLUGIN_COMPOSE) build plugin-$(P)

plugin-create: _check-plugin
	$(PLUGIN_COMPOSE) up -d --no-start plugin-$(P)

plugin-up: _check-plugin
	$(PLUGIN_COMPOSE) start plugin-$(P)

plugin-down: _check-plugin
	$(PLUGIN_COMPOSE) stop plugin-$(P)

plugin-rm: _check-plugin
	$(PLUGIN_COMPOSE) rm -f plugin-$(P)

plugin-logs: _check-plugin
	$(PLUGIN_COMPOSE) logs -f plugin-$(P)

# Quick rebuild single plugin: build + rm old + create new
plugin-rebuild: _check-plugin
	$(PLUGIN_COMPOSE) build plugin-$(P)
	$(PLUGIN_COMPOSE) rm -f plugin-$(P) || true
	$(PLUGIN_COMPOSE) up -d --no-start plugin-$(P)

# ============================================================================
#  ALL PLUGINS OPERATIONS
# ============================================================================

PLUGINS := plugin-webshell plugin-filemanager

plugins-build:
	$(PLUGIN_COMPOSE) build $(PLUGINS)

plugins-create:
	$(PLUGIN_COMPOSE) up -d --no-start $(PLUGINS)

plugins-up:
	$(PLUGIN_COMPOSE) start $(PLUGINS)

plugins-down:
	$(PLUGIN_COMPOSE) stop $(PLUGINS)

plugins-rm:
	$(PLUGIN_COMPOSE) rm -f $(PLUGINS)

# Quick rebuild all plugins
plugins-rebuild:
	$(PLUGIN_COMPOSE) build $(PLUGINS)
	$(PLUGIN_COMPOSE) rm -f $(PLUGINS) || true
	$(PLUGIN_COMPOSE) up -d --no-start $(PLUGINS)

# ============================================================================
#  BUILD & DEVELOPMENT
# ============================================================================

build:
	docker compose -f docker/docker-compose.yml build web-monitor-go docker-socket-proxy

build-all:
	docker compose -f docker/docker-compose.yml build

rebuild: down build up

dev:
	HOST_FS="" HOST_PROC="/proc" HOST_SYS="/sys" HOST_ETC="/etc" HOST_VAR="/var" HOST_RUN="/run" \
	go run cmd/server/main.go

clean:
	rm -f server cmd/server/server
