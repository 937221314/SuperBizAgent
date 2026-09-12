.PHONY: build vet test lint fmt check

# 构建全部包
build:
	go build ./...

# 官方静态检查
vet:
	go vet ./...

# 单元测试（含竞态检测）
test:
	go test -race ./...

# golangci-lint 静态检查（需 golangci-lint v2，且用 go1.27 构建）
lint:
	golangci-lint run ./...

# 格式化
fmt:
	gofmt -w .

# 提交前完整检查
check: fmt vet lint test
