// Package indexer 提供基于 Milvus 的文档索引器。
//
// 集合 schema、向量索引与加载由 eino 的 milvus2 indexer 在首次写入时创建；
// Milvus 客户端与数据库初始化复用 utility/client。详见 dev-docs/milvus.md。
package indexer

import (
	"context"
	"fmt"

	"SuperBizAgent/internal/ai/embedder"
	"SuperBizAgent/internal/config"
	"SuperBizAgent/utility/client"

	"github.com/cloudwego/eino-ext/components/indexer/milvus2"
	"github.com/cloudwego/eino/components/indexer"
)

// NewMilvusIndexer 创建 Milvus 索引器，客户端复用 utility/client 的业务库连接。
// 集合名与向量维度取自 milvus.collection_name 与 milvus.vector_dim 配置。
func NewMilvusIndexer(ctx context.Context) (indexer.Indexer, error) {
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

	idx, err := milvus2.NewIndexer(ctx, &milvus2.IndexerConfig{
		Client:     cli,
		Collection: cfg.Milvus.CollectionName,
		Vector: &milvus2.VectorConfig{
			Dimension:    cfg.Milvus.VectorDim,
			MetricType:   milvus2.COSINE,
			IndexBuilder: milvus2.NewHNSWIndexBuilder().WithM(16).WithEfConstruction(200),
		},
		Embedding: emb,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 Milvus 索引器失败: %w", err)
	}
	return idx, nil
}
