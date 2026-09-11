# SuperBizAgent

基于 Go 构建的智能业务代理（Business Agent）项目。

## 简介

SuperBizAgent 是一个使用 Go 语言开发的业务代理服务，当前处于项目初始化阶段。
项目采用标准的分层目录结构，便于后续扩展业务逻辑、API 接口与配置管理。

## 环境要求

- Go 1.27.1 或更高版本

## 快速开始

```bash
# 克隆仓库
git clone <repository-url>
cd SuperBizAgent

# 运行
go run .

# 构建
go build -o SuperBizAgent .
```

## 目录结构

```
SuperBizAgent/
├── api/                # API 接口定义
│   └── chat/           # 聊天相关接口
├── docs/               # 项目文档
├── hack/               # 构建、脚本等工具
├── internal/           # 内部实现（不对外暴露）
│   └── logic/          # 业务逻辑
│       └── sse/        # SSE 流式处理
├── manifest/           # 配置与清单
│   └── config/         # 配置文件
├── utility/            # 通用工具
│   └── common/         # 公共工具函数
├── main.go             # 程序入口
├── go.mod              # Go 模块定义
├── .gitignore          # Git 忽略规则
└── AGENTS.md           # 项目协作约定
```

## 开发约定

- 本项目**优先使用中文交流**，详见 [AGENTS.md](./AGENTS.md)。
- 提交前确保 `go build ./...` 构建通过。
- 提交信息使用语义化前缀（`feat:`、`fix:`、`chore:`、`docs:` 等）。

## 许可证

待补充。
