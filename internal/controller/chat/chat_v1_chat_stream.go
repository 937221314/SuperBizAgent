package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/agent/chat_pipeline"
	"SuperBizAgent/internal/logic/sse"
	"SuperBizAgent/utility/logcallback"
	"SuperBizAgent/utility/mem"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

// ChatStream 处理流式对话请求，经 SSE 把回答推送给前端并写入会话记忆。
func (c *ControllerV1) ChatStream(ctx context.Context, req *v1.ChatStreamReq) (res *v1.ChatStreamRes, err error) {
	id := req.ID
	msg := req.Question

	// 建立 SSE 连接并完成握手；握手失败时返回 error，交由 ResponseMiddleware 包装。
	client, err := c.server.Create(ctx, g.RequestFromCtx(ctx))
	if err != nil {
		return nil, err
	}

	// 流式生成放到独立 goroutine，结果经 server.Send 推送；当前 goroutine 交给 Run
	// 阻塞消费消息并写响应，连接断开后 Run 返回并自动注销客户端。
	go c.streamToClient(ctx, client.ID(), id, msg)

	c.server.Run(ctx, client)
	return &v1.ChatStreamRes{}, nil
}

// streamToClient 执行 LLM 流式生成，并把各阶段事件推送给指定客户端。
func (c *ControllerV1) streamToClient(ctx context.Context, clientID, id, msg string) {
	userMessage := chat_pipeline.UserMessage{
		ID:      id,
		Query:   msg,
		History: mem.GetSimpleMemory(id).GetMessages(),
	}

	runner, err := chat_pipeline.BuildEinoAgent(ctx)
	if err != nil {
		_ = c.server.Send(clientID, sse.Message{Event: "error", Data: fmt.Sprintf("构建智能体失败: %v", err)})
		return
	}

	sr, err := runner.Stream(ctx, &userMessage, compose.WithCallbacks(logcallback.New(nil)))
	if err != nil {
		_ = c.server.Send(clientID, sse.Message{Event: "error", Data: fmt.Sprintf("LLM 回复失败: %v", err)})
		return
	}
	defer sr.Close()

	var fullResponse strings.Builder
	// 无论正常结束还是中途出错，把已生成的完整回答写入会话记忆。
	defer func() {
		answer := fullResponse.String()
		if answer != "" {
			mem.GetSimpleMemory(id).SetMessage(schema.UserMessage(msg))
			mem.GetSimpleMemory(id).SetMessage(schema.SystemMessage(answer))
		}
	}()

	for {
		chunk, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			_ = c.server.Send(clientID, sse.Message{Event: "done", Data: "流完毕"})
			return
		}
		if err != nil {
			_ = c.server.Send(clientID, sse.Message{Event: "error", Data: err.Error()})
			return
		}
		fullResponse.WriteString(chunk.Content)
		_ = c.server.Send(clientID, sse.Message{Event: "message", Data: chunk.Content})
	}
}
