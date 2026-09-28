package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"context"
	"errors"
)

// AIOps 处理 AI 运维请求，功能尚未实现。
func (c *ControllerV1) AIOps(ctx context.Context, req *v1.AIOpsReq) (res *v1.AIOpsRes, err error) {
	return nil, errors.New("AI 运维功能尚未实现")
}
