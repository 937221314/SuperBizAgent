package main

import (
	"SuperBizAgent/internal/ai/agent/plan_execute_replan"
	"context"
	"fmt"
	"os"

	"github.com/cloudwego/eino/adk"
)

var query = `
"1. 你是一个智能的服务告警运维分析助手,首先调用工具query_prometheus_alerts获取所有活跃的告警。"
"2. 分别根据告警的名称调用工具query_internal_docs，获取告警名对应的处理方案。"
"3. 完全遵循内部文档的内容进行查询和分析,不允许使用文档外的任何信息。"
"4. 涉及到时间的参数都需要先通过工具get_current_time获取当前时间,再结合用户的时间要求进行传参。"
"5. 涉及到日志的查询,需要先通过日志工具获取相关日志信息，参数必须携带地域和日志主题。"
"6. 分别将告警对应查询到的信息进行总结分析,最后汇总所有告警和总结。"`

// 运维监测智能体测试
func main() {
	ctx := context.Background()
	planExecuteAgent, err := plan_execute_replan.BuildPlanAgent(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "构建运维监测智能体失败: %v\n", err)
		os.Exit(1)
	}

	r := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent: planExecuteAgent,
	})

	fmt.Println("=== 运维监测智能体开始执行 ===")
	iter := r.Query(ctx, query)
	var lastMessage adk.Message
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			fmt.Fprintf(os.Stderr, "智能体执行失败: %v\n", event.Err)
			os.Exit(1)
		}
		if event.Output != nil {
			msg, _, err := adk.GetMessage(event)
			if err != nil {
				fmt.Fprintf(os.Stderr, "获取消息失败: %v\n", err)
				os.Exit(1)
			}
			if msg == nil {
				continue
			}
			lastMessage = msg
			fmt.Println(msg.String())
		}
	}

	if lastMessage == nil || lastMessage.Content == "" {
		fmt.Fprintln(os.Stderr, "内部错误: 未获取到有效输出")
		os.Exit(1)
	}
	fmt.Println("=== 最终回答 ===")
	fmt.Println(lastMessage.Content)
}
