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
// 集合名、向量维度与 HNSW 索引参数取自 milvus 配置；集合已存在时先校验 schema。
func NewMilvusIndexer(ctx context.Context) (indexer.Indexer, error) {
	cli, err := client.NewMilvusClient(ctx)
	if err != nil {
		return nil, err
	}

	cfg, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("加载 Milvus 配置失败: %w", err)
	}

	// 集合已存在时校验 schema，避免与代码期望不一致却静默沿用。
	if err := ensureCollectionSchema(ctx, cli, cfg.Milvus.CollectionName, cfg.Milvus.VectorDim, cfg.Milvus.RecreateOnSchemaMismatch); err != nil {
		return nil, err
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
			Dimension:  cfg.Milvus.VectorDim,
			MetricType: milvus2.COSINE,
			IndexBuilder: milvus2.NewHNSWIndexBuilder().
				WithM(cfg.Milvus.Index.M).
				WithEfConstruction(cfg.Milvus.Index.EfConstruction),
		},
		Embedding: emb,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 Milvus 索引器失败: %w", err)
	}
	return idx, nil
}
