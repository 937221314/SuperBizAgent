package knowledge_index_pipeline

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino-ext/components/document/loader/file"
	"github.com/cloudwego/eino/schema"
)

// TestMultiFormatSplitter 覆盖按扩展名分派的切分行为。
func TestMultiFormatSplitter(t *testing.T) {
	ctx := context.Background()
	tfr, err := newDocumentTransformer(ctx)
	if err != nil {
		t.Fatalf("newDocumentTransformer 失败: %v", err)
	}

	t.Run("markdown 按标题切分", func(t *testing.T) {
		docs := []*schema.Document{{
			Content:  "# 标题\n正文一\n## 小节\n正文二\n",
			MetaData: map[string]any{file.MetaKeyExtension: ".md"},
		}}
		chunks, err := tfr.Transform(ctx, docs)
		if err != nil {
			t.Fatalf("Transform 失败: %v", err)
		}
		if len(chunks) < 2 {
			t.Fatalf("期望至少 2 个分片，实际 %d", len(chunks))
		}
		if got := chunks[0].MetaData["title"]; got != "标题" {
			t.Fatalf("期望 title=标题，实际 %v", got)
		}
	})

	t.Run("长文本递归切分且分片 ID 唯一", func(t *testing.T) {
		// 约 3000 个字符，超过默认 chunk_size(1000)
		content := strings.Repeat("这是一段用于测试切分的中文内容。", 200)
		docs := []*schema.Document{{
			ID:       "same-id",
			Content:  content,
			MetaData: map[string]any{file.MetaKeyExtension: ".txt"},
		}}
		chunks, err := tfr.Transform(ctx, docs)
		if err != nil {
			t.Fatalf("Transform 失败: %v", err)
		}
		if len(chunks) < 2 {
			t.Fatalf("期望切出多个分片，实际 %d", len(chunks))
		}
		seen := make(map[string]bool, len(chunks))
		for _, chunk := range chunks {
			if chunk.ID == "" {
				t.Fatal("分片 ID 为空，会导致 Milvus 主键冲突")
			}
			if seen[chunk.ID] {
				t.Fatalf("分片 ID 重复: %s", chunk.ID)
			}
			seen[chunk.ID] = true
		}
	})

	t.Run("大写扩展名与缺失元数据走文本切分", func(t *testing.T) {
		docs := []*schema.Document{
			{Content: "PDF 内容", MetaData: map[string]any{file.MetaKeyExtension: ".PDF"}},
			{Content: "无元数据内容"},
		}
		chunks, err := tfr.Transform(ctx, docs)
		if err != nil {
			t.Fatalf("Transform 失败: %v", err)
		}
		if len(chunks) != 2 {
			t.Fatalf("期望 2 个分片，实际 %d", len(chunks))
		}
	})

	t.Run("空内容不产出分片", func(t *testing.T) {
		docs := []*schema.Document{{
			Content:  "",
			MetaData: map[string]any{file.MetaKeyExtension: ".txt"},
		}}
		chunks, err := tfr.Transform(ctx, docs)
		if err != nil {
			t.Fatalf("Transform 失败: %v", err)
		}
		if len(chunks) != 0 {
			t.Fatalf("期望 0 个分片，实际 %d", len(chunks))
		}
	})
}
