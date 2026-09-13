// Package retriever 提供基于 Milvus 的向量检索器。
//
// 客户端与数据库初始化复用 utility/client，集合与向量字段需与 indexer 写入的一致。
// 详见 dev-docs/milvus.md。
package retriever

import (
	"context"
	"fmt"

	"SuperBizAgent/internal/ai/embedder"
	"SuperBizAgent/internal/config"
	"SuperBizAgent/utility/client"

	"github.com/cloudwego/eino-ext/components/retriever/milvus2"
	"github.com/cloudwego/eino-ext/components/retriever/milvus2/search_mode"
	"github.com/cloudwego/eino/components/retriever"
)

// NewMilvusRetriever 创建 Milvus 检索器，客户端复用 utility/client 的业务库连接。
func NewMilvusRetriever(ctx context.Context) (retriever.Retriever, error) {
	cli, err := client.NewMilvusClient(ctx)
	if err != nil {
		return nil, err
	}

	cfg, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("加载 Milvus 配置失败: %w", err)
	}

	// 向量模型
	emb, err := embedder.DashscopeEmbedding(ctx)
	if err != nil {
		return nil, err
	}

	// 创建 retriever
	rtr, err := milvus2.NewRetriever(ctx, &milvus2.RetrieverConfig{
		Client:      cli,
		Collection:  cfg.Milvus.CollectionName,
		VectorField: "vector",
		TopK:        1,
		OutputFields: []string{
			"id",
			"content",
			"metadata",
		},
		SearchMode: search_mode.NewApproximate(milvus2.COSINE),
		Embedding:  emb,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 Milvus 检索器失败: %w", err)
	}
	return rtr, nil
}
