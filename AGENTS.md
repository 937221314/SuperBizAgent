# AGENTS.md

## 交流语言

本项目**优先使用中文交流**。

- 所有面向用户的回复、说明、总结均使用中文。
- 代码注释、提交信息（commit message）、文档尽量使用中文。
- 变量名、函数名、类型名等标识符仍使用英文，遵循 Go 语言命名规范。

## 项目说明

SuperBizAgent 是一个 Go 项目。

- 模块名：`SuperBizAgent`
- Go 版本：1.27.1
- 入口文件：`main.go`

## 开发约定

- 提交前确保 `go build ./...` 构建通过。
- 提交信息使用语义化前缀（如 `feat:`、`fix:`、`chore:`、`docs:`）。
- 构建产物（二进制、测试缓存等）不纳入版本控制，参见 `.gitignore`。

## Git 工作流约定

`main` 是发布分支，**禁止直接 push**；所有变更必须经 Pull Request 合入。

- 日常开发在 `dev` 分支上进行，较大的独立改动从 `dev` 切出 `feat/xxx` 分支。
- 合入 `main` 一律通过 PR：`gh pr create --base main --head <分支>`，
  再由 PR 合并；不要用 `git push origin main` 代替合并。
- 提交 PR 前确认 `go build ./...` 与 `make lint` 通过。
- `main` 已开启分支保护，直推会被远端拒绝。

## 注释约定

源码注释保持简洁：说明性的背景、设计推导与后续计划统一写到 `dev-docs/`，
同一条信息不要在两处各写一遍。

- 导出符号（类型、函数、方法、常量、变量）必须有注释，且**以符号名开头**
  （如 `// Server SSE 服务…`）；否则 `golangci-lint` 的 `revive: exported` 会报错。
- 注释只写「是什么」和「不看代码就会踩的坑」（并发约定、不变量、为何这样写），
  不复述代码本身，也不抄 `dev-docs/` 的段落。
- 包注释用两三行说明职责，并指向对应的 `dev-docs/*.md`。
- 提交前跑 `make lint`，与注释相关的告警不允许遗留。
