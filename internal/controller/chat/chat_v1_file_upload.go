package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"context"
	"errors"
)

// FileUpload 处理文件上传请求，功能尚未实现。
func (c *ControllerV1) FileUpload(ctx context.Context, req *v1.FileUploadReq) (res *v1.FileUploadRes, err error) {
	return nil, errors.New("文件上传功能尚未实现")
}
