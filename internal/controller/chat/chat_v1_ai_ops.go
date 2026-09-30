package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/agent/plan_execute_replan"
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino-examples/adk/common/prints"
	"github.com/cloudwego/eino/adk"
	"github.com/gogf/gf/v2/errors/gerror"
)

// system prompt
var query = `
"1. 你是一个智能的服务告警分析助手,首先调用工具query_prometheus_alerts获取所有活跃的告警。"
"2. 分别根据告警的名称调用工具query_internal_docs，获取告警名对应的处理方案。"
"3. 完全遵循内部文档的内容进行查询和分析,不允许使用文档外的任何信息。"
"4. 涉及到时间的参数都需要先通过工具get_current_time获取当前时间,再结合工具的时间要求进行传参。"
"5. 涉及到日志的查询,需要先通过日志工具获取相关日志信息，参数必须携带地域和日志主题。"
"6. 分别将告警对应查询到的信息进行总结分析,最后生成告警运维分析报告，格式如下：
告警分析报告
---
# 告警处理详情
## 活跃告警清单
## 告警根因分析N(第N个告警)
## 处理方案执行N(第N个告警)
## 结论`

// AIOps 处理 AI 运维请求，功能尚未实现。
func (c *ControllerV1) AIOps(ctx context.Context, req *v1.AIOpsReq) (res *v1.AIOpsRes, err error) {
	planExecuteAgent, err := plan_execute_replan.BuildPlanAgent(ctx)
	if err != nil {
		return nil, gerror.Wrapf(err, "构建智能运维智能体失败")
	}

	r := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent: planExecuteAgent,
	})

	iter := r.Query(ctx, query)
	var lastMessage adk.Message
	var detail []string
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		fmt.Println("=== event ===")
		prints.Event(event) // 输出调试信息
		if event.Err != nil {
			return nil, gerror.Wrapf(event.Err, "智能运维智能体执行失败")
		}
		if event.Output != nil {
			var msg adk.Message
			msg, _, err = adk.GetMessage(event)
			if err != nil {
				return nil, gerror.Wrapf(err, "获取消息失败")
			}
			if msg == nil {
				continue
			}
			lastMessage = msg
			detail = append(detail, msg.String())
		}
	}
	if lastMessage == nil || lastMessage.Content == "" {
		return nil, gerror.New("内部错误")
	}
	res = &v1.AIOpsRes{
		Result: extractResult(lastMessage.Content),
		Detail: detail,
	}

	return res, nil
}

// respondOutput replanner 的 respond 工具输出结构。
type respondOutput struct {
	Response string `json:"response"`
}

// extractResult 从 replanner 的 respond 工具参数 JSON 中提取最终回答文本，
// 解析失败或字段为空时返回原文。
func extractResult(content string) string {
	var resp respondOutput
	if err := json.Unmarshal([]byte(content), &resp); err == nil && resp.Response != "" {
		return resp.Response
	}
	return content
}
