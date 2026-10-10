// Package embedder 提供知识库所需的文本向量模型。
//
// 模型参数（api_key/base_url/model）与向量维度均来自 internal/config 加载的 yaml 配置，
// 维度必须与集合向量字段维度一致。详见 dev-docs/milvus.md、dev-docs/configuration.md。
package embedder

import (
	"context"
	"fmt"
	"os"
	"time"

	"SuperBizAgent/internal/config"

	"github.com/cloudwego/eino-ext/components/embedding/openai"
	"github.com/cloudwego/eino/components"
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

	if embCfg.APIKey == "DASHSCOPE_API_KEY" {
		embCfg.APIKey = os.Getenv("DASHSCOPE_API_KEY")
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
	return &batchEmbedder{inner: emb, batchSize: dashscopeEmbeddingBatchSize}, nil
}

// dashscopeEmbeddingBatchSize 是 DashScope 文本向量接口单次请求的文本条数上限。
const dashscopeEmbeddingBatchSize = 20

// batchEmbedder 将底层 embedder 按固定批次大小分批调用，规避服务端单次请求上限。
type batchEmbedder struct {
	inner     embedding.Embedder
	batchSize int
}

// EmbedStrings 分批调用底层 embedder 并按原顺序合并结果。
func (b *batchEmbedder) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	results := make([][]float64, 0, len(texts))
	for start := 0; start < len(texts); start += b.batchSize {
		end := start + b.batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch, err := b.inner.EmbedStrings(ctx, texts[start:end], opts...)
		if err != nil {
			return nil, fmt.Errorf("批量嵌入失败(第 %d-%d 条): %w", start, end, err)
		}
		results = append(results, batch...)
	}
	return results, nil
}

// GetType 返回底层 embedder 的类型名，保持回调日志与组件识别一致。
func (b *batchEmbedder) GetType() string {
	if t, ok := components.GetType(b.inner); ok {
		return t
	}
	return "Embedding"
}
