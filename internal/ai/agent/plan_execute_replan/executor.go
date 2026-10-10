package plan_execute_replan

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
	"github.com/cloudwego/eino/compose"
)

// newExecutor 创建一个执行者
func newExecutor(ctx context.Context, cm *openai.ChatModel) (adk.Agent, error) {
	// 获取工具集
	baseTool, err := getBaseTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取 MCP 工具集失败: %v", err)
	}

	return planexecute.NewExecutor(ctx, &planexecute.ExecutorConfig{
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: baseTool,
			},
		},
		Model:         cm,
		MaxIterations: 99999,
	})
}
