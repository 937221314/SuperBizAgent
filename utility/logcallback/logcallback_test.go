package logcallback

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/embedding"
)

// TestEmbeddingInputSummary 校验 Embedding 入参摘要只含关键信息、不含文本内容。
func TestEmbeddingInputSummary(t *testing.T) {
	in := &embedding.CallbackInput{
		Texts:  []string{"你好", "世界和平"},
		Config: &embedding.Config{Model: "m1"},
	}
	got := embeddingInputSummary(in)
	for _, want := range []string{"texts=2", "chars=6", "model=m1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("摘要缺少 %q: %s", want, got)
		}
	}
	if strings.Contains(got, "你好") || strings.Contains(got, "世界和平") {
		t.Fatalf("摘要不应包含文本内容: %s", got)
	}
}

// TestEmbeddingInputSummaryFromStrings 校验 []string 形式入参也能汇总。
func TestEmbeddingInputSummaryFromStrings(t *testing.T) {
	got := embeddingInputSummary([]string{"abc"})
	if !strings.Contains(got, "texts=1") || !strings.Contains(got, "chars=3") {
		t.Fatalf("摘要不正确: %s", got)
	}
}

// TestEmbeddingInputSummaryUnknown 校验无法解析的入参不 panic 且有提示。
func TestEmbeddingInputSummaryUnknown(t *testing.T) {
	if got := embeddingInputSummary(123); !strings.Contains(got, "无法解析") {
		t.Fatalf("期望提示无法解析，实际: %s", got)
	}
}

// TestEmbeddingOutputSummary 校验 Embedding 出参摘要只含条数/维度/token，不含向量值。
func TestEmbeddingOutputSummary(t *testing.T) {
	out := &embedding.CallbackOutput{
		Embeddings: [][]float64{make([]float64, 1024), make([]float64, 1024)},
		Config:     &embedding.Config{Model: "m1"},
		TokenUsage: &embedding.TokenUsage{TotalTokens: 57},
	}
	got := embeddingOutputSummary(out)
	for _, want := range []string{"vectors=2", "dim=1024", "model=m1", "tokens=57"} {
		if !strings.Contains(got, want) {
			t.Fatalf("摘要缺少 %q: %s", want, got)
		}
	}
	if strings.Contains(got, "[0") {
		t.Fatalf("摘要不应包含向量值: %s", got)
	}
	if len(got) > 120 {
		t.Fatalf("摘要过长，可能泄漏向量数据（%d 字节）: %s", len(got), got)
	}
}

// TestDimsOf 校验维度提取：同维返回单值，混维返回说明，空返回 0。
func TestDimsOf(t *testing.T) {
	if got := dimsOf([][]float64{{1, 2, 3}, {4, 5, 6}}); got != "3" {
		t.Fatalf("dimsOf 同维 = %q, want 3", got)
	}
	if got := dimsOf([][]float64{{1, 2, 3}, {1, 2}}); got != "mixed(2 vectors)" {
		t.Fatalf("dimsOf 混维 = %q", got)
	}
	if got := dimsOf(nil); got != "0" {
		t.Fatalf("dimsOf(nil) = %q, want 0", got)
	}
}
