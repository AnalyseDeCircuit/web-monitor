# Build stage (offline-friendly, uses vendored deps)
FROM golang:1.21-alpine AS builder
WORKDIR /app
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache gcc musl-dev ca-certificates
ENV CGO_ENABLED=1 GO111MODULE=on
COPY go.mod ./
COPY vendor ./vendor
COPY . .
RUN go build -mod=vendor -o server .

# Runtime stage
FROM alpine:latest
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates pciutils
WORKDIR /app
COPY --from=builder /app/server .
COPY templates/ ./templates/
EXPOSE 8000
CMD ["./server"]
