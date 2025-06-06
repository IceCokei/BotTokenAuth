.PHONY: build clean test run

# 变量定义
BINARY_NAME=TokenAuth
MAIN_FILE=main.go
BUILD_DIR=bin

# 默认任务
all: clean build

# 构建应用
build:
	@echo "Building application..."
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_FILE)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# 构建多平台版本
build-all: clean
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux $(MAIN_FILE)
	@GOOS=windows GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-windows.exe $(MAIN_FILE)
	@GOOS=darwin GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-mac $(MAIN_FILE)
	@echo "Multi-platform build complete"

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
	@echo "  make build-all  - Build for Linux, Windows, and macOS"
	@echo "  make run        - Run the application"
	@echo "  make test       - Run tests"
	@echo "  make clean      - Remove build artifacts"
	@echo "  make deps       - Install dependencies"
	@echo "  make fmt        - Format code"
	@echo "  make lint       - Run linter" 