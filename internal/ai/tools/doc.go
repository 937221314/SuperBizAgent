// Package tools 存放 Agent 可挂载到 ToolsNode 的工具，以及各工具子包的公共约定。
//
// 工具统一产出 eino 的 tool.InvokableTool；实现与测试就近组织，每个工具各自成子包
// （mysql、docsearch、currenttime），本包只保留公共约定与 MCP 工具获取（query_log.go）。
// 约定见 dev-docs/README.md。
package tools
