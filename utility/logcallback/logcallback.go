package logcallback

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/gogf/gf/v2/frame/g"
)

// Config 日志回调器配置。
type Config struct {
	Detail bool // 是否打印回调入参/出参明细
	Debug  bool // 明细是否格式化（缩进）输出
}

// New 创建日志回调器；config 为 nil 时默认开启明细。
func New(config *Config) callbacks.Handler {
	if config == nil {
		config = &Config{Detail: true}
	}

	builder := callbacks.NewHandlerBuilder()
	builder.OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
		g.Log().Infof(ctx, "[view start]: [%s:%s:%s]", info.Component, info.Type, info.Name)
		if !config.Detail {
			return ctx
		}
		// Embedding 的入参是待向量化文本、出参是稠密向量，整段序列化会严重污染日志，
		// 因此只输出条数/规模等关键信息。
		if info.Component == components.ComponentOfEmbedding {
			g.Log().Info(ctx, embeddingInputSummary(input))
			return ctx
		}
		g.Log().Info(ctx, format(config.Debug, input))
		return ctx
	})
	builder.OnEndFn(func(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
		g.Log().Infof(ctx, "[view end]: [%s:%s:%s]", info.Component, info.Type, info.Name)
		if !config.Detail {
			return ctx
		}
		if info.Component == components.ComponentOfEmbedding {
			g.Log().Info(ctx, embeddingOutputSummary(output))
			return ctx
		}
		g.Log().Info(ctx, format(config.Debug, output))
		return ctx
	})

	return builder.Build()
}

// embeddingInputSummary 汇总 Embedding 入参：条数、总字符数与模型，不打印文本内容。
func embeddingInputSummary(input callbacks.CallbackInput) string {
	in := embedding.ConvCallbackInput(input)
	if in == nil {
		return fmt.Sprintf("[Embedding 输入] 无法解析的入参类型: %T", input)
	}
	total := 0
	for _, text := range in.Texts {
		total += utf8.RuneCountInString(text)
	}
	return fmt.Sprintf("[Embedding 输入] texts=%d, chars=%d, model=%s",
		len(in.Texts), total, modelOf(in.Config))
}

// embeddingOutputSummary 汇总 Embedding 出参：向量条数、维度、模型与 token，不打印向量值。
func embeddingOutputSummary(output callbacks.CallbackOutput) string {
	out := embedding.ConvCallbackOutput(output)
	if out == nil {
		return fmt.Sprintf("[Embedding 输出] 无法解析的出参类型: %T", output)
	}
	return fmt.Sprintf("[Embedding 输出] vectors=%d, dim=%s, model=%s, tokens=%s",
		len(out.Embeddings), dimsOf(out.Embeddings), modelOf(out.Config), tokensOf(out.TokenUsage))
}

// dimsOf 返回向量维度：所有向量同维时返回单个数字，否则返回混合维度的说明。
func dimsOf(vectors [][]float64) string {
	if len(vectors) == 0 {
		return "0"
	}
	dim := len(vectors[0])
	for _, v := range vectors {
		if len(v) != dim {
			return fmt.Sprintf("mixed(%d vectors)", len(vectors))
		}
	}
	return strconv.Itoa(dim)
}

// modelOf 返回模型名，缺省时用 "-" 占位。
func modelOf(cfg *embedding.Config) string {
	if cfg == nil || cfg.Model == "" {
		return "-"
	}
	return cfg.Model
}

// tokensOf 返回总 token 数，缺省时用 "-" 占位。
func tokensOf(usage *embedding.TokenUsage) string {
	if usage == nil {
		return "-"
	}
	return strconv.Itoa(usage.TotalTokens)
}

// format 序列化回调数据；序列化失败时返回错误描述，避免静默输出空内容。
func format(debug bool, v any) string {
	var (
		b   []byte
		err error
	)
	if debug {
		b, err = json.MarshalIndent(v, "", "  ")
	} else {
		b, err = json.Marshal(v)
	}
	if err != nil {
		return "<marshal error: " + err.Error() + ">"
	}
	return string(b)
}
