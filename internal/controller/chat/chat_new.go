package chat

import "SuperBizAgent/api/chat"

// ControllerV1 聊天模块的 HTTP 控制器。
type ControllerV1 struct{}

// NewV1 创建聊天控制器。
//
// ChatStream/FileUpload/AIOps 接口尚未实现，暂返回 nil。
func NewV1() chat.IChatV1 {
	return nil
}
