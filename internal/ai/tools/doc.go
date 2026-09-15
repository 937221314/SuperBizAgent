// Package tools 存放 Agent 可挂载到 ToolsNode 的工具，以及各工具子包的公共约定。
//
// 工具统一产出 eino 的 tool.InvokableTool；带测试的工具单独成子包（mysql、docsearch），
// 实现与测试就近组织，本包只保留暂无测试的轻量工具。约定见 dev-docs/README.md。
package tools
