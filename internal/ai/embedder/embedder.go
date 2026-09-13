// Package embedder 提供知识库所需的文本向量模型。
//
// 模型参数（api_key/base_url/model）与向量维度均来自 utility/config 加载的 yaml 配置，
// 维度必须与集合向量字段维度一致。详见 dev-docs/milvus.md、dev-docs/configuration.md。
package embedder

import (
	"context"
	"fmt"
	"time"

	"SuperBizAgent/utility/config"

	"github.com/cloudwego/eino-ext/components/embedding/openai"
	"github.com/cloudwego/eino/components/embedding"
)

// DashscopeEmbedding 基于 text-embedding 配置创建 DashScope 文本向量模型。
func DashscopeEmbedding(ctx context.Context) (emb embedding.Embedder, err error) {
	cfg, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}

	embCfg := cfg.TextEmbedding
	if embCfg.APIKey == "" || embCfg.BaseURL == "" || embCfg.Model == "" {
		return nil, fmt.Errorf("文本模型配置不完整，请检查 text-embedding.api_key/base_url/model")
	}

	// 与集合向量字段维度保持同源，避免维度不一致导致写入失败
	dim := int(cfg.Milvus.VectorDim)

	emb, err = openai.NewEmbedder(ctx, &openai.EmbeddingConfig{
		APIKey:     embCfg.APIKey,
		BaseURL:    embCfg.BaseURL,
		Model:      embCfg.Model,
		Timeout:    30 * time.Second,
		Dimensions: &dim,
	})
	if err != nil {
		return nil, fmt.Errorf("创建文本向量模型失败: %w", err)
	}
	return emb, nil
}
