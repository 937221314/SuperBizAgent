// Package currenttime 提供获取当前系统时间的工具 get_current_time（无入参）。
//
// 三个 Unix 时间戳与可读时间同源于一次 time.Now()，单位换算后必须自洽；
// 工具尚未挂载，见 dev-docs/todo.md 的「工具尚未挂载」。
package currenttime

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/gogf/gf/v2/frame/g"
)

const (
	// dateFormatStr 精确到微秒：秒级可读时间在连续调用中会重复，微秒便于区分先后。
	dateFormatStr = "2006-01-02 15:04:05.000000"

	// toolDescGetCurrentTime 是工具描述，模型依赖它判断是否调用，需写清适用场景。
	toolDescGetCurrentTime = "获取多种格式的当前系统时间，返回 Unix 时间戳（秒、毫秒、微秒）与本地时区的可读时间。" +
		"当你需要获取当前系统时间用于日志记录、计时操作或为事件打时间戳时，请使用此工具。"
)

// GetCurrentTimeInput 是 get_current_time 工具的入参。
// 该工具无参数，空结构体经 InferTool 反射后得到的就是空参数 schema。
type GetCurrentTimeInput struct{}

// GetCurrentTimeOutput 是 get_current_time 工具的输出。
type GetCurrentTimeOutput struct {
	// Seconds 当前 Unix 时间戳（秒），自纪元（1970-01-01 00:00:00 UTC）起。
	Seconds int64 `json:"seconds"`
	// Milliseconds 当前 Unix 时间戳（毫秒），自纪元（1970-01-01 00:00:00 UTC）起。
	Milliseconds int64 `json:"milliseconds"`
	// Microseconds 当前 Unix 时间戳（微秒），自纪元（1970-01-01 00:00:00 UTC）起。
	Microseconds int64 `json:"microseconds"`
	// Timestamp 系统本地时区的人类可读时间，格式为 YYYY-MM-DD HH:MM:SS.microseconds。
	Timestamp string `json:"timestamp"`
}

// NewGetCurrentTimeTool 创建获取当前时间的工具（get_current_time）。
// ctx 只用于与同包其它构造函数保持一致的调用形状，工具本身不依赖上下文。
func NewGetCurrentTimeTool(ctx context.Context) (tool.InvokableTool, error) {
	return newGetCurrentTimeTool(time.Now)
}

// newGetCurrentTimeTool 基于注入的时钟构建工具，便于测试用固定时间断言输出。
func newGetCurrentTimeTool(now func() time.Time) (tool.InvokableTool, error) {
	handler := func(ctx context.Context, _ *GetCurrentTimeInput) (GetCurrentTimeOutput, error) {
		current := now()
		output := GetCurrentTimeOutput{
			Seconds:      current.Unix(),
			Milliseconds: current.UnixMilli(),
			Microseconds: current.UnixMicro(),
			Timestamp:    current.Format(dateFormatStr),
		}

		g.Log().Printf(ctx, "获取当前时间: %s", output.Timestamp)
		g.Log().Printf(ctx, "GetCurrentTimeOutput: %v", output)
		return output, nil
	}

	t, err := utils.InferTool("get_current_time", toolDescGetCurrentTime, handler)
	if err != nil {
		return nil, fmt.Errorf("创建 get_current_time 工具失败: %w", err)
	}
	return t, nil
}
