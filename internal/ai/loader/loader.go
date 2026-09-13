// Package loader 提供知识库文档的加载器。
//
// 加载器只负责读取本地文件并转换为 eino 文档，向量化与写入由 indexer 完成。
// 详见 dev-docs/milvus.md。
package loader

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/document/loader/file"
	"github.com/cloudwego/eino/components/document"
)

// NewFileLoader 创建本地文件加载器，使用文件名作为文档 ID。
func NewFileLoader(ctx context.Context) (ldr document.Loader, err error) {
	cfg := file.FileLoaderConfig{
		UseNameAsID: true, // 使用文件名作为文档 ID
	}
	ldr, err = file.NewFileLoader(ctx, &cfg)
	if err != nil {
		return nil, fmt.Errorf("创建文件加载器失败: %w", err)
	}
	return ldr, nil
}
