package chat_pipeline

import "github.com/cloudwego/eino/schema"

// UserMessage 是对话 Agent 的输入：用户标识、当前问题与历史消息。
type UserMessage struct {
	ID      string            `json:"id"`
	Query   string            `json:"query"`
	History []*schema.Message `json:"history"`
}
