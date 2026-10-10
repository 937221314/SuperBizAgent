package loader

import (
	"bytes"
	"context"
	"io"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/document/parser"
	"github.com/cloudwego/eino/schema"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// charsetTextParser 在纯文本解析前做编码探测，避免 GBK/UTF-16 文本乱码。
// 目前支持：UTF-8（含 BOM）、UTF-16（带 BOM）、GBK；其余按原始字节处理。
type charsetTextParser struct {
	fallback parser.TextParser
}

// Parse 读取全部内容并尽量转换为 UTF-8，再交给兜底解析器。
func (p charsetTextParser) Parse(ctx context.Context, reader io.Reader, opts ...parser.Option) ([]*schema.Document, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if decoded, decodeErr := decodeToUTF8(data); decodeErr == nil {
		data = decoded
	}
	return p.fallback.Parse(ctx, bytes.NewReader(data), opts...)
}

// decodeToUTF8 依次尝试：UTF-8 BOM、UTF-16 BOM、合法 UTF-8、GBK。
// 无法确定编码时返回错误，由调用方按原始字节兜底。
func decodeToUTF8(data []byte) ([]byte, error) {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		return data[3:], nil
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}):
		return transformBytes(data, unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder())
	case bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		return transformBytes(data, unicode.UTF16(unicode.BigEndian, unicode.UseBOM).NewDecoder())
	case utf8.Valid(data):
		return data, nil
	default:
		return transformBytes(data, simplifiedchinese.GBK.NewDecoder())
	}
}

// transformBytes 用给定编码解码器转换字节串。
func transformBytes(data []byte, t transform.Transformer) ([]byte, error) {
	out, _, err := transform.Bytes(t, data)
	if err != nil {
		return nil, err
	}
	return out, nil
}
