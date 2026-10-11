// Package v1 定义聊天模块 HTTP 接口的请求与响应结构。
package v1

import "github.com/gogf/gf/v2/frame/g"

// ChatReq 非流式对话请求。
type ChatReq struct {
	g.Meta   `path:"/chat" method:"post" summary:"对话"`
	ID       string `json:"id" dc:"会话 ID"`
	Question string `json:"question" dc:"用户问题"`
}

// ChatRes 非流式对话响应。
type ChatRes struct {
	Answer string `json:"answer" dc:"模型回答"`
}

// ChatStreamReq 流式对话请求。
type ChatStreamReq struct {
	g.Meta   `path:"/chat_stream" method:"post" summary:"流式对话"`
	ID       string `json:"id" dc:"会话 ID"`
	Question string `json:"question" dc:"用户问题"`
}

// ChatStreamRes 流式对话响应，内容经 SSE 推送，无响应正文。
type ChatStreamRes struct{}

// FileUploadReq 文件上传请求。
type FileUploadReq struct {
	g.Meta `path:"/upload" method:"post" mime:"multipart/form-data" summary:"文件上传"`
}

// FileUploadRes 文件上传响应。
type FileUploadRes struct {
	FileName string `json:"fileName" dc:"保存的文件名"`
	FilePath string `json:"filePath" dc:"文件保存路径"`
	FileSize int64  `json:"fileSize" dc:"文件大小(字节)"`
}

// AIOpsReq AI 运维请求。
type AIOpsReq struct {
	g.Meta `path:"/ai_ops" method:"post" summary:"AI运维"`
}

// AIOpsRes AI 运维响应。
type AIOpsRes struct {
	Result string   `json:"result" dc:"运维结果"`
	Detail []string `json:"detail" dc:"详情列表"`
}
