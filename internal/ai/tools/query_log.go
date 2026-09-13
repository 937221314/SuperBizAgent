package tools

import (
	"context"
	"fmt"

	"SuperBizAgent/internal/config"

	mcpp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// GetLogMcpTool 从配置的 SSE MCP 服务获取日志类工具，返回的客户端由调用方负责生命周期管理。
func GetLogMcpTool(ctx context.Context) ([]tool.BaseTool, error) {
	cfg, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}

	cli, err := client.NewSSEMCPClient(cfg.McpURL)
	if err != nil {
		return nil, fmt.Errorf("创建 SSE 客户端失败: %v", err)
	}

	err = cli.Start(ctx)
	if err != nil {
		return nil, fmt.Errorf("SSE 客户端启动失败: %v", err)
	}

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "example-client",
		Version: "1.0.0",
	}

	if _, err := cli.Initialize(ctx, initRequest); err != nil {
		return nil, fmt.Errorf("SSE 客户端初始化失败: %v", err)
	}

	mcpTools, err := mcpp.GetTools(ctx, &mcpp.Config{Cli: cli})
	if err != nil {
		return nil, fmt.Errorf("获取 MCP 工具失败: %v", err)
	}
	return mcpTools, nil
}
