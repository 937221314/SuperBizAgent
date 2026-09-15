package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

// 时间格式
const (
	dataFormatStr                 = "2006-01-02 15:04:05.000000"
	toolGetCurrentTimeDescription = "获取多种格式的当前系统时间。返回当前时间的秒（Unix 时间戳）、毫秒和微秒。当你需要获取当前系统时间用于日志记录、计时操作或为事件打时间戳时，请使用此工具。"
)

// GetCurrentTimeInput 获取当前时间输入参数
type GetCurrentTimeInput struct {
	// 无需输入参数
}

// GetCurrentTimeOutput 获取当前时间的输出结果
type GetCurrentTimeOutput struct {
	Success      bool   `json:"success" jsonschema:"description=表示时间获取是否成功"`
	Seconds      int64  `json:"seconds" jsonschema:"description=当前 Unix 时间戳（秒），自纪元（1970-01-01 00:00:00 UTC）起"`
	Milliseconds int64  `json:"milliseconds" jsonschema:"description=当前 Unix 时间戳（毫秒），自纪元（1970-01-01 00:00:00 UTC）起"`
	Microseconds int64  `json:"microseconds" jsonschema:"description=当前 Unix 时间戳（微秒），自纪元（1970-01-01 00:00:00 UTC）起"`
	Timestamp    string `json:"timestamp" jsonschema:"description=人类可读的时间戳，格式为 'YYYY-MM-DD HH:MM:SS.microseconds'"`
	Message      string `json:"message" jsonschema:"description=描述操作结果的状态消息"`
}

// 获取当前时间工具结构体
type getCurrentTimeTool struct {
	info *schema.ToolInfo
}

// Info 返回工具的元信息
func (t *getCurrentTimeTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return t.info, nil
}

// 执行方法
func (t *getCurrentTimeTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	// 获取当前时间
	now := time.Now()

	// 计算各种时间格式
	sconds := now.Unix()                   // 秒
	milliseconds := now.UnixMilli()        // 毫秒
	microseconds := now.UnixMicro()        // 微秒
	timestamp := now.Format(dataFormatStr) // 可读格式

	g.Log().Printf(ctx, "获取当前时间: %s", timestamp)

	// 构建输出
	timeOutput := GetCurrentTimeOutput{
		Success:      true,
		Seconds:      sconds,
		Milliseconds: milliseconds,
		Microseconds: microseconds,
		Timestamp:    timestamp,
		Message:      "当前时间检索成功",
	}

	// 转换为 JSON
	jsonBytes, err := json.MarshalIndent(timeOutput, "", " ")
	if err != nil {
		return "", fmt.Errorf("将结果序列化为 JSON 失败: %v", err)
	}

	g.Log().Printf(ctx, "GetCurrentTimeOutput: %v", timeOutput)
	return string(jsonBytes), nil
}

// NewGetCurrentTimeTool 创建获取当前时间的工具
func NewGetCurrentTimeTool(ctx context.Context) (tool.InvokableTool, error) {
	info := &schema.ToolInfo{
		Name: "get_current_time",
		Desc: toolGetCurrentTimeDescription,
	}
	return &getCurrentTimeTool{info: info}, nil
}
