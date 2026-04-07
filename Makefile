.PHONY: help build run test clean docker-build docker-run docker-push docker-compose-up docker-compose-down lint coverage

# 默认目标
help:
	@echo "可用的命令:"
	@echo "  make build              - 编译 Go 二进制文件"
	@echo "  make run                - 运行服务"
	@echo "  make test               - 运行所有测试"
	@echo "  make test-verbose       - 运行测试（详细输出）"
	@echo "  make test-race          - 运行竞态检测"
	@echo "  make coverage           - 生成测试覆盖率报告"
	@echo "  make lint               - 运行代码检查"
	@echo "  make clean              - 清理构建产物"
	@echo ""
	@echo "Docker 命令:"
	@echo "  make docker-build       - 构建 Docker 镜像"
	@echo "  make docker-run         - 运行 Docker 容器"
	@echo "  make docker-push        - 推送镜像到仓库"
	@echo "  make docker-stop        - 停止并删除容器"
	@echo ""
	@echo "Docker Compose 命令:"
	@echo "  make up                 - 启动服务（docker-compose）"
	@echo "  make down               - 停止服务"
	@echo "  make logs               - 查看日志"
	@echo "  make restart            - 重启服务"
	@echo "  make ps                 - 查看容器状态"

# Go 构建配置
BINARY_NAME=goloop
MAIN_PATH=./cmd/server
BUILD_FLAGS=-ldflags="-w -s"
GO=go

# Docker 配置
IMAGE_NAME=goloop
IMAGE_TAG=latest
DOCKER_REGISTRY ?= ghcr.io/$(shell git remote get-url origin | sed -E 's|.*github\.com[:/](.*)(\.git)?$$|\1|' | tr '[:upper:]' '[:lower:]')

# 编译
build:
	@echo "编译 $(BINARY_NAME)..."
	$(GO) build $(BUILD_FLAGS) -o bin/$(BINARY_NAME) $(MAIN_PATH)
	@echo "编译完成: bin/$(BINARY_NAME)"

# 加载 .env 文件（如果存在）
-include .env
export

# 运行服务
run:
	@echo "启动服务..."
	$(GO) run $(MAIN_PATH)

# 测试
test:
	@echo "运行测试..."
	$(GO) test ./... -timeout 60s

test-verbose:
	@echo "运行测试（详细输出）..."
	$(GO) test ./... -v -timeout 60s

test-race:
	@echo "运行竞态检测..."
	$(GO) test ./... -race -timeout 60s

# 测试覆盖率
coverage:
	@echo "生成测试覆盖率报告..."
	$(GO) test ./... -coverprofile=coverage.out -covermode=atomic
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "覆盖率报告已生成: coverage.html"

# 代码检查
lint:
	@echo "运行代码检查..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint 未安装，请运行: brew install golangci-lint"; \
	fi

# 清理
clean:
	@echo "清理构建产物..."
	rm -rf bin/
	rm -f coverage.out coverage.html
	rm -rf /tmp/images/*
	$(GO) clean -cache -testcache -modcache
	@echo "清理完成"

# Docker 构建
docker-build:
	@echo "构建 Docker 镜像..."
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .
	@echo "镜像构建完成: $(IMAGE_NAME):$(IMAGE_TAG)"

# Docker 多平台构建
docker-build-multi:
	@echo "构建多平台 Docker 镜像..."
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		-t $(IMAGE_NAME):$(IMAGE_TAG) \
		--load \
		.

# Docker 运行
docker-run:
	@echo "启动 Docker 容器..."
	docker run -d \
		--name $(BINARY_NAME) \
		-p 8080:8080 \
		-v $(PWD)/config/config.yaml:/app/config/config.yaml:ro \
		-v goloop-images:/tmp/images \
		--restart unless-stopped \
		$(IMAGE_NAME):$(IMAGE_TAG)
	@echo "容器已启动，访问 http://localhost:8080"

# Docker 停止
docker-stop:
	@echo "停止并删除容器..."
	docker stop $(BINARY_NAME) || true
	docker rm $(BINARY_NAME) || true

# Docker 推送
docker-push:
	@echo "推送镜像到 $(DOCKER_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)..."
	docker tag $(IMAGE_NAME):$(IMAGE_TAG) $(DOCKER_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)
	docker push $(DOCKER_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

# Docker Compose 命令
up:
	@echo "启动 docker-compose 服务..."
	docker-compose up -d
	@echo "服务已启动，访问 http://localhost:8080"

down:
	@echo "停止 docker-compose 服务..."
	docker-compose down

logs:
	@echo "查看服务日志..."
	docker-compose logs -f

restart:
	@echo "重启服务..."
	docker-compose restart

ps:
	@echo "查看容器状态..."
	docker-compose ps

# 开发环境
dev: build
	@echo "启动开发模式..."
	./bin/$(BINARY_NAME)

# 安装开发依赖
install-tools:
	@echo "安装开发工具..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "工具安装完成"

# 版本发布
release:
	@echo "请创建版本标签以触发自动构建:"
	@echo "  git tag v1.0.0"
	@echo "  git push origin v1.0.0"
