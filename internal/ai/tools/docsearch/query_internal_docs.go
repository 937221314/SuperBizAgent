// Package docsearch 提供内部文档检索工具 query_internal_docs。
//
// 工具产出 eino 的 tool.InvokableTool；检索复用 internal/ai/retriever 的 Milvus 检索器，
// 检索器惰性构建并全局复用。详见 dev-docs/milvus.md。
package docsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"SuperBizAgent/internal/ai/retriever"

	einoRetriever "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// toolDescQueryInternalDocs 是工具描述，模型依赖它判断是否调用，需写清适用场景。
const toolDescQueryInternalDocs = "搜索内部文档和知识库，获取相关信息和处理步骤（基于知识库的 RAG 检索）。" +
	"当你需要了解公司文档中存储的内部流程、最佳实践或分步指南时，使用它。" +
	"结果为空表示未找到相关内容。"

var (
	// internalDocsMu 保护 internalDocs 的惰性构建，避免并发首次调用重复建连。
	internalDocsMu sync.Mutex

	// internalDocs 全局复用的检索器：构建一次即可，不要每次调用工具都重建
	// Milvus 客户端与 embedding 客户端。
	internalDocs einoRetriever.Retriever
)

// QueryInternalDocsInput 是 query_internal_docs 工具的入参。
type QueryInternalDocsInput struct {
	// Query 检索用的自然语言问题或关键词。
	Query string `json:"query" jsonschema:"required,description=The query string to search the internal documentation for relevant information and processing steps"`
}

// docSnippet 是回传给模型的检索片段，只保留引用所需字段，避免整段 Document 占用上下文。
type docSnippet struct {
	ID       string         `json:"id"`
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewQueryInternalDocsTool 创建内部文档检索工具（query_internal_docs）。
// 检索器在首次调用时惰性构建并全局复用。
func NewQueryInternalDocsTool(ctx context.Context) (tool.InvokableTool, error) {
	r, err := internalDocsRetriever(ctx)
	if err != nil {
		return nil, fmt.Errorf("创建内部文档检索器失败: %w", err)
	}
	return newQueryInternalDocsTool(r)
}

// newQueryInternalDocsTool 基于指定检索器构建工具，检索器由调用方持有并复用。
func newQueryInternalDocsTool(r einoRetriever.Retriever) (tool.InvokableTool, error) {
	handler := func(ctx context.Context, input *QueryInternalDocsInput) (string, error) {
		docs, err := r.Retrieve(ctx, input.Query)
		if err != nil {
			return "", fmt.Errorf("检索内部文档失败: %w", err)
		}

		snippets := make([]docSnippet, 0, len(docs))
		for _, doc := range docs {
			if doc == nil {
				continue
			}
			snippets = append(snippets, docSnippet{
				ID:       doc.ID,
				Content:  doc.Content,
				Metadata: doc.MetaData,
			})
		}

		b, err := json.Marshal(snippets)
		if err != nil {
			return "", fmt.Errorf("检索结果序列化失败: %w", err)
		}
		return string(b), nil
	}

	t, err := utils.InferTool("query_internal_docs", toolDescQueryInternalDocs, handler)
	if err != nil {
		return nil, fmt.Errorf("创建 query_internal_docs 工具失败: %w", err)
	}
	return t, nil
}

// internalDocsRetriever 返回全局复用的检索器；构建失败不缓存，下次调用可重试。
func internalDocsRetriever(ctx context.Context) (einoRetriever.Retriever, error) {
	internalDocsMu.Lock()
	defer internalDocsMu.Unlock()

	if internalDocs != nil {
		return internalDocs, nil
	}

	r, err := retriever.NewMilvusRetriever(ctx)
	if err != nil {
		return nil, err
	}
	internalDocs = r
	return internalDocs, nil
}
