package chat

import (
	"SuperBizAgent/api/chat"
	"SuperBizAgent/internal/config"
	"SuperBizAgent/internal/logic/sse"
	"time"
)

// ControllerV1 聊天模块的 HTTP 控制器。
type ControllerV1 struct {
	server *sse.Server
}

// NewV1 创建聊天控制器，并注入 SSE 服务。
func NewV1() chat.IChatV1 {
	// 慢客户端策略取自配置；配置异常或取值非法时 WithSlowClientPolicy 会忽略，保持默认 drop。
	var policy sse.SlowClientPolicy
	if cfg, err := config.Get(); err == nil {
		policy = sse.SlowClientPolicy(cfg.SSE.SlowClientPolicy)
	}

	ser := sse.New(
		sse.WithRetryInterval(1500*time.Millisecond),
		sse.WithSlowClientPolicy(policy),
	)
	return &ControllerV1{
		server: ser,
	}
}
