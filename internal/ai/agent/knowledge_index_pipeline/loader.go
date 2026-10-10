package knowledge_index_pipeline

import (
	"context"

	loader2 "SuperBizAgent/internal/ai/loader"

	"github.com/cloudwego/eino/components/document"
)

// newLoader component initialization function of node 'FileLoader' in graph 'KnowledgeIndexing'
// 复用 internal/ai/loader，保证图内加载与元数据查询使用同一份解析配置。
func newLoader(ctx context.Context) (ldr document.Loader, err error) {
	return loader2.NewFileLoader(ctx)
}
