package loader

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/document/parser"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
)

// TestNewParser 覆盖扩展名分派：.txt 按纯文本、.doc 明确报错、扩展名不区分大小写。
func TestNewParser(t *testing.T) {
	ctx := context.Background()
	p, err := newParser(ctx)
	if err != nil {
		t.Fatalf("newParser 失败: %v", err)
	}

	t.Run("txt 走纯文本解析", func(t *testing.T) {
		docs, err := p.Parse(ctx, strings.NewReader("hello"), parser.WithURI("/tmp/demo.txt"))
		if err != nil {
			t.Fatalf("Parse 失败: %v", err)
		}
		if len(docs) != 1 || docs[0].Content != "hello" {
			t.Fatalf("期望单个纯文本文档，实际 %#v", docs)
		}
	})

	t.Run("doc 明确报错", func(t *testing.T) {
		_, err := p.Parse(ctx, strings.NewReader("x"), parser.WithURI("/tmp/demo.doc"))
		if err == nil {
			t.Fatal("期望 .doc 返回错误")
		}
		if !strings.Contains(err.Error(), "暂不支持") {
			t.Fatalf("期望错误信息包含「暂不支持」，实际 %v", err)
		}
	})

	t.Run("混合大小写扩展名同样命中", func(t *testing.T) {
		for _, uri := range []string{"/tmp/demo.Doc", "/tmp/demo.DOC"} {
			_, err := p.Parse(ctx, strings.NewReader("x"), parser.WithURI(uri))
			if err == nil || !strings.Contains(err.Error(), "暂不支持") {
				t.Fatalf("%s 期望 .doc 报错，实际 %v", uri, err)
			}
		}
	})
}

// TestCharsetTextParser 覆盖纯文本的编码探测：GBK 与带 BOM 的 UTF-16 都能正确转成 UTF-8。
func TestCharsetTextParser(t *testing.T) {
	ctx := context.Background()
	p, err := newParser(ctx)
	if err != nil {
		t.Fatalf("newParser 失败: %v", err)
	}
	const want = "知识库中文内容测试"

	t.Run("GBK", func(t *testing.T) {
		encoded, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(want))
		if err != nil {
			t.Fatalf("GBK 编码失败: %v", err)
		}
		docs, err := p.Parse(ctx, bytes.NewReader(encoded), parser.WithURI("/tmp/demo.txt"))
		if err != nil {
			t.Fatalf("Parse 失败: %v", err)
		}
		if docs[0].Content != want {
			t.Fatalf("期望 %q，实际 %q", want, docs[0].Content)
		}
	})

	t.Run("UTF-16LE BOM", func(t *testing.T) {
		encoded, err := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder().Bytes([]byte(want))
		if err != nil {
			t.Fatalf("UTF-16 编码失败: %v", err)
		}
		docs, err := p.Parse(ctx, bytes.NewReader(encoded), parser.WithURI("/tmp/demo.txt"))
		if err != nil {
			t.Fatalf("Parse 失败: %v", err)
		}
		if docs[0].Content != want {
			t.Fatalf("期望 %q，实际 %q", want, docs[0].Content)
		}
	})
}
