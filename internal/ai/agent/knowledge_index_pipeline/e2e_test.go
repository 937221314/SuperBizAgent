package knowledge_index_pipeline

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	aiLoader "SuperBizAgent/internal/ai/loader"

	"github.com/cloudwego/eino-ext/components/document/loader/file"
	"github.com/cloudwego/eino/components/document"
)

// TestEndToEndLoadAndSplit 用真实样例跑通「加载 → 切分」，
// 覆盖 PDF、DOCX、UTF-8 TXT、GBK TXT 四种格式（不含向量化与入库）。
func TestEndToEndLoadAndSplit(t *testing.T) {
	ctx := context.Background()

	ldr, err := aiLoader.NewFileLoader(ctx)
	if err != nil {
		t.Fatalf("创建加载器失败: %v", err)
	}
	tfr, err := newDocumentTransformer(ctx)
	if err != nil {
		t.Fatalf("创建切分器失败: %v", err)
	}

	cases := []struct {
		file   string
		needle string // 期望正文包含的片段；为空则只要求正文非空
	}{
		{"test_pdf.pdf", ""},
		{"test_docx.docx", ""},
		{"test_utf8.txt", "知识库文本样例"},
		{"test_gbk.txt", "知识库编码探测样例"},
	}

	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			src := document.Source{URI: filepath.Join("testdata", tc.file)}
			docs, err := ldr.Load(ctx, src)
			if err != nil {
				t.Fatalf("加载失败: %v", err)
			}
			if len(docs) == 0 {
				t.Fatal("加载未产出文档")
			}

			var content strings.Builder
			for _, doc := range docs {
				if doc.MetaData[file.MetaKeySource] != src.URI {
					t.Fatalf("_source 元数据不是原路径: %v", doc.MetaData)
				}
				content.WriteString(doc.Content)
			}
			if strings.TrimSpace(content.String()) == "" {
				t.Fatal("加载内容为空")
			}
			if tc.needle != "" && !strings.Contains(content.String(), tc.needle) {
				t.Fatalf("内容未包含 %q，实际前 200 字: %.200s", tc.needle, content.String())
			}

			chunks, err := tfr.Transform(ctx, docs)
			if err != nil {
				t.Fatalf("切分失败: %v", err)
			}
			if len(chunks) == 0 {
				t.Fatal("切分未产出分片")
			}

			seen := make(map[string]bool, len(chunks))
			var total int
			for _, chunk := range chunks {
				if chunk.ID == "" {
					t.Fatal("分片 ID 为空")
				}
				if seen[chunk.ID] {
					t.Fatalf("分片 ID 重复: %s", chunk.ID)
				}
				seen[chunk.ID] = true
				total += len([]rune(chunk.Content))
			}
			t.Logf("docs=%d chunks=%d runes=%d", len(docs), len(chunks), total)
		})
	}
}
