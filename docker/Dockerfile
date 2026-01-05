# Build stage (offline-friendly, uses vendored deps)
FROM golang:1.24-bookworm AS builder
WORKDIR /app

# APT mirror configuration (optional)
# Usage: --build-arg APT_MIRROR=aliyun|tuna|tencent|ustc|<custom-url>
# Default: official Debian mirrors (for overseas builds)
ARG APT_MIRROR=""
RUN if [ -n "$APT_MIRROR" ]; then \
        case "$APT_MIRROR" in \
            aliyun)  MIRROR_URL="mirrors.aliyun.com" ;; \
            tuna)    MIRROR_URL="mirrors.tuna.tsinghua.edu.cn" ;; \
            tencent) MIRROR_URL="mirrors.cloud.tencent.com" ;; \
            ustc)    MIRROR_URL="mirrors.ustc.edu.cn" ;; \
            163)     MIRROR_URL="mirrors.163.com" ;; \
            huawei)  MIRROR_URL="repo.huaweicloud.com" ;; \
            *)       MIRROR_URL="$APT_MIRROR" ;; \
        esac; \
        sed -i "s|deb.debian.org|$MIRROR_URL|g" /etc/apt/sources.list.d/debian.sources; \
        echo "APT mirror: $MIRROR_URL"; \
    fi

ENV CGO_ENABLED=1 GO111MODULE=on GOOS=linux GOARCH=amd64
COPY go.mod ./
COPY vendor ./vendor
COPY . .
RUN go build -mod=vendor -ldflags="-s -w" -trimpath -o opskernel ./cmd/server

# Plugins are now managed as separate Docker containers (docker-managed mode).
# We do NOT build exec-mode plugins into the main image anymore.
# The plugins/src directory is kept for reference but not built here.

# Runtime stage
FROM debian:bookworm-slim

ARG APT_MIRROR=""
RUN if [ -n "$APT_MIRROR" ]; then \
        case "$APT_MIRROR" in \
            aliyun)  MIRROR_URL="mirrors.aliyun.com" ;; \
            tuna)    MIRROR_URL="mirrors.tuna.tsinghua.edu.cn" ;; \
            tencent) MIRROR_URL="mirrors.cloud.tencent.com" ;; \
            ustc)    MIRROR_URL="mirrors.ustc.edu.cn" ;; \
            163)     MIRROR_URL="mirrors.163.com" ;; \
            huawei)  MIRROR_URL="repo.huaweicloud.com" ;; \
            *)       MIRROR_URL="$APT_MIRROR" ;; \
        esac; \
        sed -i "s|deb.debian.org|$MIRROR_URL|g" /etc/apt/sources.list.d/debian.sources; \
        echo "APT mirror: $MIRROR_URL"; \
    fi \
    && apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata pciutils net-tools iproute2 curl \
    openssh-client \
    && update-pciids \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /app/opskernel .
# Copy plugins directory with manifests (plugin.json files)
COPY plugins/ ./plugins/
COPY templates/ ./templates/
COPY static/ ./static/
EXPOSE 8000
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8000/api/info || exit 1
CMD ["./opskernel"]
