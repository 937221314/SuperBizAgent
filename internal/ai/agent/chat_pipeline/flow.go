package chat_pipeline

import (
	"context"

	"SuperBizAgent/internal/ai/tools"
	"SuperBizAgent/internal/ai/tools/currenttime"
	"SuperBizAgent/internal/ai/tools/docsearch"
	"SuperBizAgent/internal/ai/tools/mysql"
	"SuperBizAgent/internal/ai/tools/prometheus"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/gogf/gf/v2/frame/g"
)

// newReactAgentLambda component initialization function of node 'ReactAgent' in graph 'EinoAgent'
func newReactAgentLambda(ctx context.Context) (lba *compose.Lambda, err error) {
	chatModelIns, err := newChatModel(ctx)
	if err != nil {
		return nil, err
	}

	toolList, err := newAgentTools(ctx)
	if err != nil {
		return nil, err
	}

	ins, err := react.NewAgent(ctx, &react.AgentConfig{
		MaxStep:            25,
		ToolReturnDirectly: map[string]struct{}{},
		ToolCallingModel:   chatModelIns,
		ToolsConfig:        compose.ToolsNodeConfig{Tools: toolList},
	})
	if err != nil {
		return nil, err
	}

	return compose.AnyLambda(ins.Generate, ins.Stream, nil, nil)
}

// newAgentTools 组装 ReAct Agent 可用的工具集。
// 日志 MCP 工具是可选依赖：MCP 服务不可用时仅记录日志并跳过，不阻塞对话；
// 其余工具（mysql/prometheus/currenttime/docsearch）构造失败则返回错误。
func newAgentTools(ctx context.Context) ([]tool.BaseTool, error) {
	toolList := make([]tool.BaseTool, 0, 6)

	if mcpTools, err := tools.GetLogMcpTool(ctx); err != nil {
		g.Log().Warningf(ctx, "[chat_pipeline] 日志 MCP 工具不可用，已跳过: %v", err)
	} else {
		toolList = append(toolList, mcpTools...)
	}

	constructors := []func(context.Context) (tool.InvokableTool, error){
		mysql.NewMysqlQueryTool,
		mysql.NewMysqlExecTool,
		prometheus.NewPrometheusAlertsQueryTool,
		currenttime.NewGetCurrentTimeTool,
		docsearch.NewQueryInternalDocsTool,
	}
	for _, newTool := range constructors {
		t, err := newTool(ctx)
		if err != nil {
			return nil, err
		}
		toolList = append(toolList, t)
	}

	return toolList, nil
}
