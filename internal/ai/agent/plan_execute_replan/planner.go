package plan_execute_replan

import (
	"context"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
)

// newplan 规划者
func newplan(ctx context.Context, cm *openai.ChatModel) (adk.Agent, error) {

	return planexecute.NewPlanner(ctx, &planexecute.PlannerConfig{
		ToolCallingChatModel: cm,
	})
}
