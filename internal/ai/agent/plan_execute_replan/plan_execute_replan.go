package plan_execute_replan

import (
	"SuperBizAgent/internal/ai/models"
	"context"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
)

// BuildPlanAgent 构建规划执行智能体
func BuildPlanAgent(ctx context.Context) (adk.Agent, error) {
	// 创建模型实例
	cm, err := models.OpenAIForQwenFlash(ctx)
	if err != nil {
		return nil, err
	}
	// 规划
	planAgent, err := newplan(ctx, cm)
	if err != nil {
		return nil, err
	}

	// 执行
	executeAgent, err := newExecutor(ctx, cm)
	if err != nil {
		return nil, err
	}

	// 评估
	replanAgent, err := newReplan(ctx, cm)
	if err != nil {
		return nil, err
	}

	// 构建 多智能体协调
	planExecuteAgent, err := planexecute.New(ctx, &planexecute.Config{
		Planner:       planAgent,
		Executor:      executeAgent,
		Replanner:     replanAgent,
		MaxIterations: 20,
	})
	if err != nil {
		return nil, fmt.Errorf("构建 PlanExecute 智能体失败: %v", err)
	}

	return planExecuteAgent, nil
}
