package chat

import "context"

// IChatV1 定义聊天相关的流式与非流式接口。
type IChatV1 interface {
	Chat(ctx context.Context) (err error)
	ChatStream(ctx context.Context) (err error)
	FileUpload(ctx context.Context) (err error)
	AIOps(ctx context.Context) (err error)
}
