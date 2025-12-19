# Build stage (offline-friendly, uses vendored deps)
FROM golang:1.24-bookworm AS builder
WORKDIR /app
# Use Aliyun mirror for Debian
RUN sed -i 's/deb.debian.org/mirrors.aliyun.com/g' /etc/apt/sources.list.d/debian.sources
ENV CGO_ENABLED=1 GO111MODULE=on GOOS=linux GOARCH=amd64
COPY go.mod ./
COPY vendor ./vendor
COPY . .
RUN go build -mod=vendor -ldflags="-s -w" -trimpath -o server ./cmd/server

# Plugins are now managed as separate Docker containers (docker-managed mode).
# We do NOT build exec-mode plugins into the main image anymore.
# The plugins/src directory is kept for reference but not built here.

# Runtime stage
FROM debian:bookworm-slim
RUN sed -i 's/deb.debian.org/mirrors.aliyun.com/g' /etc/apt/sources.list.d/debian.sources \
    && apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata pciutils net-tools iproute2 curl \
    && update-pciids \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /app/server .
# Copy plugins directory (docs only, no binaries)
RUN mkdir -p ./plugins
COPY templates/ ./templates/
COPY static/ ./static/
EXPOSE 8000
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8000/api/info || exit 1
CMD ["./server"]
