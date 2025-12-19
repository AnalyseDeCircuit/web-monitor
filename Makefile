.PHONY: help up down restart logs stats build rebuild dev clean up-minimal up-server up-no-docker

# Default target: show help
default: help

# Help menu
help:
	@echo "Web Monitor Management Commands:"
	@echo "  make up            - Start CORE services (Full Mode)"
	@echo "  make plugins-create - Create plugin containers (do not start)"
	@echo "  make plugins-build  - Build plugin images"
	@echo "  make build-all      - Build ALL images"
	@echo "  make up-minimal    - Start CORE metrics only (CPU/Mem/Disk/Net)"
	@echo "  make up-server     - Start Server Mode (No GPU/Power)"
	@echo "  make up-no-docker  - Start without Docker management"
	@echo "  make down          - Stop and remove containers"
	@echo "  make restart       - Restart services"
	@echo "  make logs          - View logs"
	@echo "  make stats         - View container resource usage"
	@echo "  make build         - Rebuild core images"
	@echo "  make dev           - Run locally (Go)"
	@echo "  make clean         - Clean up build artifacts"

# --- Start Modes ---

# 1. Full Mode (Default)
up:
	docker compose up -d web-monitor-go docker-socket-proxy

# Create plugin containers but do not start them.
plugins-create:
	docker compose up -d --no-start plugin-webshell plugin-filemanager

plugins-build:
	docker compose build plugin-webshell plugin-filemanager

# 2. Minimal Mode: Core system metrics only.
# Disables: Docker, GPU, Cron, SSH, Systemd, Sensors, Power, System(Processes)
up-minimal:
	ENABLE_DOCKER=false \
	ENABLE_GPU=false \
	ENABLE_CRON=false \
	ENABLE_SSH=false \
	ENABLE_SYSTEMD=false \
	ENABLE_SENSORS=false \
	ENABLE_POWER=false \
	ENABLE_SYSTEM=false \
	docker compose up -d web-monitor-go

# 3. Server Mode: Standard server monitoring.
# Disables: GPU, Power (Battery/Screen)
up-server:
	ENABLE_GPU=false \
	ENABLE_POWER=false \
	docker compose up -d web-monitor-go docker-socket-proxy

# 4. No-Docker Mode: For environments without Docker management needs.
# Disables: Docker
up-no-docker:
	ENABLE_DOCKER=false \
	docker compose up -d web-monitor-go

# --- Operations ---

down:
	docker compose down

restart: down up

logs:
	docker compose logs -f

stats:
	docker compose stats

build:
	docker compose build web-monitor-go docker-socket-proxy

build-all:
	docker compose build

rebuild: down build up

# Local development
# Sets host paths to actual system paths since we are not in a container
dev:
	HOST_FS="" HOST_PROC="/proc" HOST_SYS="/sys" HOST_ETC="/etc" HOST_VAR="/var" HOST_RUN="/run" go run cmd/server/main.go

clean:
	rm -f server
	rm -f cmd/server/server
