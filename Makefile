.PHONY: help build run docs clean test vendor

# 默认目标
.DEFAULT_GOAL := help

# 项目变量
BINARY_NAME=web-monitor
DOCS_DIR=docs
GO_FILES=$(shell find . -name '*.go' -type f -not -path "./vendor/*" -not -path "./docs/*")

help: ## 显示帮助信息
	@echo "Web Monitor - Makefile命令:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
	@echo ""

build: ## 构建项目
	@echo "构建 $(BINARY_NAME)..."
	@go build -o bin/$(BINARY_NAME) cmd/server/main.go
	@echo "✅ 构建完成: bin/$(BINARY_NAME)"

run: ## 运行项目
	@echo "启动 $(BINARY_NAME)..."
	@go run cmd/server/main.go

docs: ## 生成Swagger API文档
	@echo "生成Swagger文档..."
	@if [ ! -f ~/go/bin/swag ]; then \
		echo "安装swag工具..."; \
		go install github.com/swaggo/swag/cmd/swag@latest; \
	fi
	@GOFLAGS="-mod=mod" ~/go/bin/swag init -g cmd/server/main.go -o $(DOCS_DIR) --parseDependency --parseInternal
	@echo "✅ Swagger文档已生成到 $(DOCS_DIR)/"
	@echo "📖 访问 http://localhost:8000/swagger/index.html 查看文档"

docs-fmt: ## 格式化Swagger注释
	@echo "格式化Swagger注释..."
	@GOFLAGS="-mod=mod" ~/go/bin/swag fmt
	@echo "✅ Swagger注释已格式化"

vendor: ## 同步vendor依赖
	@echo "同步vendor目录..."
	@go mod tidy
	@go mod vendor
	@echo "✅ Vendor已同步"

clean: ## 清理构建文件
	@echo "清理构建文件..."
	@rm -rf bin/
	@rm -rf $(DOCS_DIR)/
	@echo "✅ 清理完成"

test: ## 运行测试
	@echo "运行测试..."
	@go test -v -cover ./...

fmt: ## 格式化代码
	@echo "格式化代码..."
	@go fmt ./...
	@echo "✅ 代码已格式化"

lint: ## 运行代码检查
	@echo "运行代码检查..."
	@if [ ! -f ~/go/bin/golangci-lint ]; then \
		echo "安装golangci-lint..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	fi
	@~/go/bin/golangci-lint run
	@echo "✅ 代码检查完成"

dev: docs ## 开发模式(生成文档并运行)
	@make run

all: clean docs build ## 完整构建(清理+文档+编译)
	@echo "✅ 完整构建完成"

docker-build: ## 构建Docker镜像
	@echo "构建Docker镜像..."
	@docker build -t $(BINARY_NAME):latest .
	@echo "✅ Docker镜像构建完成"

docker-run: ## 运行Docker容器
	@echo "启动Docker容器..."
	@docker compose up -d
	@echo "✅ Docker容器已启动"

docker-stop: ## 停止Docker容器
	@echo "停止Docker容器..."
	@docker compose down
	@echo "✅ Docker容器已停止"

update-deps: ## 更新依赖
	@echo "更新Go依赖..."
	@go get -u ./...
	@go mod tidy
	@go mod vendor
	@echo "✅ 依赖已更新"

install-tools: ## 安装开发工具
	@echo "安装开发工具..."
	@go install github.com/swaggo/swag/cmd/swag@latest
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "✅ 开发工具已安装"
