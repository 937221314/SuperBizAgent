package chat_pipeline

import (
	"SuperBizAgent/internal/ai/tools"
	"SuperBizAgent/internal/ai/tools/currenttime"
	"SuperBizAgent/internal/ai/tools/docsearch"
	"SuperBizAgent/internal/ai/tools/mysql"
	"SuperBizAgent/internal/ai/tools/prometheus"
	"context"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
)

// newReactAgentLambda component initialization function of node 'ReactAgent' in graph 'EinoAgent'
func newReactAgentLambda(ctx context.Context) (lba *compose.Lambda, err error) {
	// TODO Modify component configuration here.
	config := &react.AgentConfig{
		MaxStep:            25,
		ToolReturnDirectly: map[string]struct{}{}}
	chatModelIns11, err := newChatModel(ctx)
	if err != nil {
		return nil, err
	}
	config.ToolCallingModel = chatModelIns11
	toolIns20, err := tools.GetLogMcpTool(ctx)
	if err != nil {
		return nil, err
	}
	// toolIns21, err := newTool(ctx)
	toolIns21, err := mysql.NewMysqlQueryTool(ctx) // mysqlCrudQueryTool
	if err != nil {
		return nil, err
	}
	// toolIns22, err := newTool1(ctx)
	toolIns22, err := mysql.NewMysqlExecTool(ctx) // mysqlCrudExecTool
	if err != nil {
		return nil, err
	}
	// toolIns23, err := newTool2(ctx)
	toolIns23, err := prometheus.NewPrometheusAlertsQueryTool(ctx) // prometheusAlertsQueryTool
	if err != nil {
		return nil, err
	}
	// toolIns24, err := newTool3(ctx)
	toolIns24, err := currenttime.NewGetCurrentTimeTool(ctx) // getCurrenttimeTool
	if err != nil {
		return nil, err
	}
	toolIns25, err := docsearch.NewQueryInternalDocsTool(ctx) //queryInternalDocsTool
	if err != nil {
		return nil, err
	}
	config.ToolsConfig.Tools = []tool.BaseTool{toolIns20[0], toolIns21, toolIns22, toolIns23, toolIns24, toolIns25}
	ins, err := react.NewAgent(ctx, config)
	if err != nil {
		return nil, err
	}
	lba, err = compose.AnyLambda(ins.Generate, ins.Stream, nil, nil)
	if err != nil {
		return nil, err
	}
	return lba, nil
}
