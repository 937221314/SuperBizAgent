package knowledge_index_pipeline

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"SuperBizAgent/internal/config"

	"github.com/cloudwego/eino-ext/components/document/loader/file"
	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/markdown"
	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/recursive"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
)

// newDocumentTransformer component initialization function of node 'MarkdownSplitter' in graph 'KnowledgeIndexing'
// 按文件扩展名分派：Markdown 走标题切分，其余（.pdf/.docx/.txt 等）走递归切分。
func newDocumentTransformer(ctx context.Context) (tfr document.Transformer, err error) {
	cfg, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("加载切分配置失败: %w", err)
	}

	// 分片 ID 统一重新生成，避免沿用 loader 的原始 ID 导致主键冲突
	// （PDF 分页文档与无 ID 文档尤其如此）。
	newID := func(ctx context.Context, originalID string, splitIndex int) string {
		return uuid.New().String()
	}

	mdSplitter, err := markdown.NewHeaderSplitter(ctx, &markdown.HeaderConfig{
		Headers: map[string]string{
			"#":   "title",
			"##":  "section",
			"###": "subsection",
		},
		TrimHeaders: false,
		IDGenerator: newID,
	})
	if err != nil {
		return nil, err
	}

	textSplitter, err := recursive.NewSplitter(ctx, &recursive.Config{
		ChunkSize:   cfg.KnowledgeChunk.ChunkSize,
		OverlapSize: cfg.KnowledgeChunk.OverlapSize,
		// 按 rune 计数，避免中文被按字节过早切断。
		LenFunc: utf8.RuneCountInString,
		// 补充中文常见分隔符，否则整段中文（无换行/英文标点）无法切分。
		Separators:  []string{"\n", "。", "！", "？", "；", ".", "?", "!"},
		IDGenerator: newID,
	})
	if err != nil {
		return nil, err
	}

	return &multiFormatSplitter{markdown: mdSplitter, text: textSplitter}, nil
}

// multiFormatSplitter 依据文档元数据中的文件扩展名选择切分器。
type multiFormatSplitter struct {
	markdown document.Transformer
	text     document.Transformer
}

// Transform 实现 document.Transformer：.md/.markdown 走 Markdown 标题切分，
// 其余扩展名（含未知或缺失元数据）走递归切分。
func (s *multiFormatSplitter) Transform(ctx context.Context, docs []*schema.Document, opts ...document.TransformerOption) ([]*schema.Document, error) {
	var mdDocs, textDocs []*schema.Document
	for _, doc := range docs {
		if isMarkdownDoc(doc) {
			mdDocs = append(mdDocs, doc)
		} else {
			textDocs = append(textDocs, doc)
		}
	}

	result := make([]*schema.Document, 0, len(docs))
	if len(mdDocs) > 0 {
		chunks, err := s.markdown.Transform(ctx, mdDocs, opts...)
		if err != nil {
			return nil, fmt.Errorf("Markdown 切分失败: %w", err)
		}
		result = append(result, chunks...)
	}
	if len(textDocs) > 0 {
		chunks, err := s.text.Transform(ctx, textDocs, opts...)
		if err != nil {
			return nil, fmt.Errorf("文本切分失败: %w", err)
		}
		result = append(result, chunks...)
	}
	return result, nil
}

// GetType 返回组件类型标识，供 eino 回调与日志使用。
func (s *multiFormatSplitter) GetType() string {
	return "MultiFormatSplitter"
}

// isMarkdownDoc 判断文档是否为 Markdown：读取 FileLoader 写入的 _extension 元数据。
func isMarkdownDoc(doc *schema.Document) bool {
	if doc.MetaData == nil {
		return false
	}
	ext, ok := doc.MetaData[file.MetaKeyExtension].(string)
	if !ok {
		return false
	}
	switch strings.ToLower(ext) {
	case ".md", ".markdown":
		return true
	default:
		return false
	}
}
