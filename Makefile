.PHONY: build vet test lint fmt check frontend-install frontend-typecheck frontend-build

# 构建全部包
build:
	go build ./...

# 官方静态检查
vet:
	go vet ./...

# 单元测试（含竞态检测）
test:
	go test -race ./...

# golangci-lint 静态检查（需 v2.14.0 及以上：更低版本内置的 x/tools < v0.50.0 读不了
# go1.27 的 export data V5，会报 typecheck 错误）
lint:
	golangci-lint run ./...

# 格式化
fmt:
	gofmt -w .

# 提交前完整检查
check: fmt vet lint test

# --- 前端（SuperBizAgentFrontend）---
# 首次使用需先执行 frontend-install
frontend-install:
	cd SuperBizAgentFrontend && npm install

frontend-typecheck:
	cd SuperBizAgentFrontend && npm run typecheck

frontend-build:
	cd SuperBizAgentFrontend && npm run build
