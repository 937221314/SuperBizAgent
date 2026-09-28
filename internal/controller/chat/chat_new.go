package chat

import (
	"SuperBizAgent/api/chat"
	"SuperBizAgent/internal/logic/sse"
	"time"
)

// ControllerV1 聊天模块的 HTTP 控制器。
type ControllerV1 struct {
	server *sse.Server
}

// NewV1 创建聊天控制器，并注入 SSE 服务。
func NewV1() chat.IChatV1 {
	ser := sse.New(sse.WithRetryInterval(1500 * time.Millisecond))
	return &ControllerV1{
		server: ser,
	}
}
