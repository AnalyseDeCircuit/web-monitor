# Build stage (offline-friendly, uses vendored deps)
FROM golang:1.21-alpine AS builder
WORKDIR /app
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache gcc musl-dev ca-certificates upx
ENV CGO_ENABLED=1 GO111MODULE=on GOOS=linux GOARCH=amd64
COPY go.mod ./
COPY vendor ./vendor
COPY . .
RUN go build -mod=vendor -ldflags="-s -w" -trimpath -o server . && \
    upx --lzma -q --best server || true

# Runtime stage
FROM alpine:latest
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates pciutils && \
    rm -rf /var/cache/apk/* /var/lib/apk/*
WORKDIR /app
COPY --from=builder /app/server .
COPY templates/ ./templates/
EXPOSE 8000
CMD ["./server"]
