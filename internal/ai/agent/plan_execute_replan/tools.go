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
	"github.com/gogf/gf/v2/frame/g"
)

// getBaseTools 技能工具列表。
// 日志 MCP 工具是可选依赖：MCP 服务不可用时仅记录日志并跳过，不阻塞执行者构建。
func getBaseTools(ctx context.Context) ([]tool.BaseTool, error) {
	toolList := make([]tool.BaseTool, 0, 6)

	mcpTools, err := tools.GetLogMcpTool(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "[plan_execute_replan] 日志 MCP 工具不可用，已跳过: %v", err)
	} else {
		toolList = append(toolList, mcpTools...)
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

	toolList = append(toolList, currentTimeTool, mysqlQueryTool, mysqlExecTool, docSearchTool, prometheusTool)
	return toolList, nil
}
