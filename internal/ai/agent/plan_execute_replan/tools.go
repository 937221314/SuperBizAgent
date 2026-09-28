package plan_execute_replan

import (
	"SuperBizAgent/internal/ai/tools"
	"SuperBizAgent/internal/ai/tools/currenttime"
	"SuperBizAgent/internal/ai/tools/docsearch"
	"SuperBizAgent/internal/ai/tools/mysql"
	"SuperBizAgent/internal/ai/tools/prometheus"
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
)

// getBaseTools 技能工具列表
func getBaseTools(ctx context.Context) ([]tool.BaseTool, error) {
	// 提供工具信息
	mcpTools, err := tools.GetLogMcpTool(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取Mcp工具失败: %v", err)
	}

	currentTimeTool, err := currenttime.NewGetCurrentTimeTool(ctx)
	if err != nil {
		return nil, fmt.Errorf("创建 currentTime 工具失败: %v", err)
	}

	mysqlQueryTool, err := mysql.NewMysqlQueryTool(ctx)
	if err != nil {
		return nil, fmt.Errorf("创建 mysqlQuery 工具失败: %v", err)
	}

	mysqlExecTool, err := mysql.NewMysqlExecTool(ctx)
	if err != nil {
		return nil, fmt.Errorf("创建 mysqlExec 工具失败: %v", err)
	}

	docSearchTool, err := docsearch.NewQueryInternalDocsTool(ctx)
	if err != nil {
		return nil, fmt.Errorf("创建文档检索工具失败: %v", err)
	}

	prometheusTool, err := prometheus.NewPrometheusAlertsQueryTool(ctx)
	if err != nil {
		return nil, fmt.Errorf("创建查询 prometheus 告警工具失败: %v", err)
	}

	mcpTools = append(mcpTools, currentTimeTool)
	mcpTools = append(mcpTools, mysqlQueryTool)
	mcpTools = append(mcpTools, mysqlExecTool)
	mcpTools = append(mcpTools, docSearchTool)
	mcpTools = append(mcpTools, prometheusTool)

	return mcpTools, nil
}
