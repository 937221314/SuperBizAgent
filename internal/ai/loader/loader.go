// Package loader 提供知识库文档的加载器。
//
// 加载器只负责读取本地文件并转换为 eino 文档，向量化与写入由 indexer 完成。
// 详见 dev-docs/milvus.md。
package loader

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino-ext/components/document/loader/file"
	"github.com/cloudwego/eino-ext/components/document/parser/docx"
	"github.com/cloudwego/eino-ext/components/document/parser/pdf"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/document/parser"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

// NewFileLoader 创建本地文件加载器，使用文件名作为文档 ID。
func NewFileLoader(ctx context.Context) (ldr document.Loader, err error) {
	p, err := newParser(ctx)
	if err != nil {
		return nil, err
	}
	cfg := file.FileLoaderConfig{
		UseNameAsID: true, // 使用文件名作为文档 ID
		Parser:      p,
	}
	ldr, err = file.NewFileLoader(ctx, &cfg)
	if err != nil {
		return nil, fmt.Errorf("创建文件加载器失败: %w", err)
	}
	return ldr, nil
}

// newExtParser 创建按扩展名分派的解析器：
// .pdf / .docx 使用 eino-ext 专用解析器，.doc 明确报错，其余按纯文本处理
// （纯文本会先做编码探测，见 charsetTextParser）。
func newExtParser(ctx context.Context) (*parser.ExtParser, error) {
	pdfParser, err := pdf.NewPDFParser(ctx, &pdf.Config{ToPages: true})
	if err != nil {
		return nil, fmt.Errorf("创建 PDF 解析器失败: %w", err)
	}
	docxParser, err := docx.NewDocxParser(ctx, &docx.Config{
		ToSections:    true,
		IncludeTables: true,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 DOCX 解析器失败: %w", err)
	}

	extParser, err := parser.NewExtParser(ctx, &parser.ExtParserConfig{
		Parsers: map[string]parser.Parser{
			".pdf":  pdfParser,
			".docx": docxParser,
			".doc":  unsupportedParser{ext: ".doc"},
		},
		FallbackParser: charsetTextParser{fallback: parser.TextParser{}},
	})
	if err != nil {
		return nil, fmt.Errorf("创建扩展名解析器失败: %w", err)
	}
	return extParser, nil
}

// newParser 创建大小写不敏感的文档解析器。
func newParser(ctx context.Context) (parser.Parser, error) {
	extParser, err := newExtParser(ctx)
	if err != nil {
		return nil, err
	}
	return caseInsensitiveParser{inner: extParser}, nil
}

// caseInsensitiveParser 委派前将扩展名统一为小写，使 .Pdf / .DOCX 等
// 混合大小写也能命中专用解析器（ExtParser 内部按 filepath.Ext 精确匹配）。
type caseInsensitiveParser struct {
	inner parser.Parser
}

// Parse 实现 parser.Parser：把 URI 的扩展名小写化后交给内层解析器。
// opts 中的 _source 等元数据来自 ExtraMeta，不受 URI 改写影响。
func (p caseInsensitiveParser) Parse(ctx context.Context, reader io.Reader, opts ...parser.Option) ([]*schema.Document, error) {
	uri := parser.GetCommonOptions(nil, opts...).URI
	if ext := filepath.Ext(uri); ext != "" {
		normalized := make([]parser.Option, 0, len(opts)+1)
		normalized = append(normalized, opts...)
		normalized = append(normalized, parser.WithURI(uri[:len(uri)-len(ext)]+strings.ToLower(ext)))
		opts = normalized
	}
	return p.inner.Parse(ctx, reader, opts...)
}

// supportedDocExts 知识库允许索引的扩展名（小写）。
var supportedDocExts = map[string]bool{
	".md": true, ".markdown": true, ".txt": true, ".pdf": true, ".docx": true,
}

// IsSupportedDoc 判断文件是否属于知识库支持的格式；.doc 暂不支持，返回 false。
func IsSupportedDoc(path string) bool {
	return supportedDocExts[strings.ToLower(filepath.Ext(path))]
}

// unsupportedParser 对暂不支持的格式直接报错，避免被兜底逻辑当作纯文本读入。
type unsupportedParser struct {
	ext string
}

// Parse 始终返回错误。
func (p unsupportedParser) Parse(ctx context.Context, _ io.Reader, _ ...parser.Option) ([]*schema.Document, error) {
	g.Log().Warningf(ctx, "跳过不支持的文档格式: %s", p.ext)
	return nil, fmt.Errorf("暂不支持 %s 格式，请另存为 .docx 后重新上传", p.ext)
}
