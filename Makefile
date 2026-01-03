.PHONY: build clean test run release

# 变量定义
BINARY_NAME=BotTokenAuth
MAIN_FILE=main.go
BUILD_DIR=bin
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-s -w -X main.Version=$(VERSION)

# 默认任务
all: clean build

# 构建应用
build:
	@echo "Building application..."
	@mkdir -p $(BUILD_DIR)
	@go build -trimpath -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_FILE)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# 构建多平台版本
build-all: clean
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	@echo "Building Linux AMD64..."
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_FILE)
	@echo "Building Linux ARM64..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_FILE)
	@echo "Building Windows AMD64..."
	@GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_FILE)
	@echo "Building macOS Intel..."
	@GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_FILE)
	@echo "Building macOS Apple Silicon..."
	@GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_FILE)
	@echo "Multi-platform build complete"

# 打包发布版本
release: build-all
	@echo "Creating release packages..."
	@mkdir -p $(BUILD_DIR)/release
	@cd $(BUILD_DIR) && \
		tar -czf release/$(BINARY_NAME)-linux-amd64.tar.gz $(BINARY_NAME)-linux-amd64 ../config.toml.example && \
		tar -czf release/$(BINARY_NAME)-linux-arm64.tar.gz $(BINARY_NAME)-linux-arm64 ../config.toml.example && \
		tar -czf release/$(BINARY_NAME)-darwin-amd64.tar.gz $(BINARY_NAME)-darwin-amd64 ../config.toml.example && \
		tar -czf release/$(BINARY_NAME)-darwin-arm64.tar.gz $(BINARY_NAME)-darwin-arm64 ../config.toml.example && \
		zip -q release/$(BINARY_NAME)-windows-amd64.zip $(BINARY_NAME)-windows-amd64.exe ../config.toml.example
	@cd $(BUILD_DIR)/release && \
		for file in *; do \
			shasum -a 256 $$file > $$file.sha256; \
		done
	@echo "Release packages created in $(BUILD_DIR)/release/"
	@ls -lh $(BUILD_DIR)/release/

# 运行应用
run:
	@echo "Running application..."
	@go run $(MAIN_FILE)

# 测试应用
test:
	@echo "Running tests..."
	@go test -v ./...

# 清理构建产物
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean complete"

# 安装依赖
deps:
	@echo "Installing dependencies..."
	@go mod download
	@echo "Dependencies installed"

# 检查代码格式
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@echo "Formatting complete"

# 检查代码质量
lint:
	@echo "Linting code..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# 显示帮助信息
help:
	@echo "Available commands:"
	@echo "  make build      - Build the application"
	@echo "  make build-all  - Build for all platforms"
	@echo "  make release    - Create release packages with checksums"
	@echo "  make run        - Run the application"
	@echo "  make test       - Run tests"
	@echo "  make clean      - Remove build artifacts"
	@echo "  make deps       - Install dependencies"
	@echo "  make fmt        - Format code"
	@echo "  make lint       - Run linter" 