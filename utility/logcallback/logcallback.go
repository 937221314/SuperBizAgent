package logcallback

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/eino/callbacks"
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
		if config.Detail {
			g.Log().Info(ctx, format(config.Debug, input))
		}
		return ctx
	})
	builder.OnEndFn(func(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
		g.Log().Infof(ctx, "[view end]: [%s:%s:%s]", info.Component, info.Type, info.Name)
		if config.Detail {
			g.Log().Info(ctx, format(config.Debug, output))
		}
		return ctx
	})

	return builder.Build()
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
