# Build stage (offline-friendly, uses vendored deps)
FROM golang:1.23-alpine AS builder
WORKDIR /app
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories
ENV CGO_ENABLED=0 GO111MODULE=on GOOS=linux GOARCH=amd64
COPY go.mod ./
COPY vendor ./vendor
COPY . .
RUN go build -mod=vendor -ldflags="-s -w" -trimpath -o server ./cmd/server

# Runtime stage
FROM alpine:latest
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates tzdata pciutils && \
    rm -rf /var/cache/apk/* /var/lib/apk/*
WORKDIR /app
COPY --from=builder /app/server .
COPY templates/ ./templates/
EXPOSE 8000
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:8000/api/info || exit 1
CMD ["./server"]
