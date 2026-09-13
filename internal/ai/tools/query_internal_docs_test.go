package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	einoRetriever "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

// fakeRetriever 用于替换 Milvus 检索器，按需返回固定结果或错误。
type fakeRetriever struct {
	docs []*schema.Document
	err  error

	// gotQuery 记录实际透传给检索器的查询串。
	gotQuery string
}

// Retrieve 实现 eino 的 retriever.Retriever 接口。
func (f *fakeRetriever) Retrieve(_ context.Context, query string, _ ...einoRetriever.Option) ([]*schema.Document, error) {
	f.gotQuery = query
	if f.err != nil {
		return nil, f.err
	}
	return f.docs, nil
}

// TestQueryInternalDocsToolRun 校验入参透传与结果序列化。
func TestQueryInternalDocsToolRun(t *testing.T) {
	fake := &fakeRetriever{docs: []*schema.Document{
		{ID: "doc-1", Content: "步骤一：打开配置", MetaData: map[string]any{"source": "guide.md"}},
		nil, // 检索器理论上不返回 nil，仍需跳过而不是 panic
	}}

	tl, err := newQueryInternalDocsTool(fake)
	if err != nil {
		t.Fatalf("创建工具失败: %v", err)
	}

	out, err := tl.InvokableRun(context.Background(), `{"query":"如何配置"}`)
	if err != nil {
		t.Fatalf("执行工具失败: %v", err)
	}
	if fake.gotQuery != "如何配置" {
		t.Errorf("查询串透传错误，期望 %q，实际 %q", "如何配置", fake.gotQuery)
	}

	var got []docSnippet
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("输出不是合法 JSON: %v，原文 %s", err, out)
	}
	if len(got) != 1 {
		t.Fatalf("期望跳过 nil 文档后剩 1 条，实际 %d 条", len(got))
	}
	if got[0].ID != "doc-1" || got[0].Content != "步骤一：打开配置" {
		t.Errorf("片段内容不符: %+v", got[0])
	}
	if got[0].Metadata["source"] != "guide.md" {
		t.Errorf("元数据丢失: %+v", got[0].Metadata)
	}
}

// TestQueryInternalDocsToolRetrieveError 校验检索失败时错误向上透传。
func TestQueryInternalDocsToolRetrieveError(t *testing.T) {
	sentinel := errors.New("milvus 不可用")
	tl, err := newQueryInternalDocsTool(&fakeRetriever{err: sentinel})
	if err != nil {
		t.Fatalf("创建工具失败: %v", err)
	}

	_, err = tl.InvokableRun(context.Background(), `{"query":"任意"}`)
	if !errors.Is(err, sentinel) {
		t.Fatalf("期望包装并保留原错误，实际: %v", err)
	}
}

// TestQueryInternalDocsInputSchema 校验工具 schema 暴露的是 query 字段且为必填。
func TestQueryInternalDocsInputSchema(t *testing.T) {
	info, err := utils.GoStruct2ToolInfo[*QueryInternalDocsInput]("query_internal_docs", "test")
	if err != nil {
		t.Fatalf("推导 schema 失败: %v", err)
	}

	js, err := info.ToJSONSchema()
	if err != nil {
		t.Fatalf("转换 schema 失败: %v", err)
	}
	raw, err := json.Marshal(js)
	if err != nil {
		t.Fatalf("序列化 schema 失败: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, `"query"`) {
		t.Errorf("schema 缺少 query 字段: %s", s)
	}
	if !strings.Contains(s, `"required"`) {
		t.Errorf("schema 未把 query 标记为必填: %s", s)
	}
	if strings.Contains(s, `"quer"`) {
		t.Errorf("schema 仍残留拼错的 quer 字段: %s", s)
	}
}
