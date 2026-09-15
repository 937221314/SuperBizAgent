package currenttime

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// TestNewGetCurrentTimeTool 用固定时钟断言工具输出，避免依赖真实时间与运行机器的时区。
func TestNewGetCurrentTimeTool(t *testing.T) {
	// 用 FixedZone 而非 time.Local：断言结果必须与运行环境无关。
	loc := time.FixedZone("UTC+8", 8*60*60)
	fixed := time.Date(2024, 5, 6, 7, 8, 9, 123456000, loc)

	tl, err := newGetCurrentTimeTool(func() time.Time { return fixed })
	if err != nil {
		t.Fatalf("创建工具失败: %v", err)
	}

	// 模型调用无参工具时传的是空对象。
	got, err := tl.InvokableRun(context.Background(), "{}")
	if err != nil {
		t.Fatalf("调用工具失败: %v", err)
	}

	var out GetCurrentTimeOutput
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("输出不是合法 JSON（%s）: %v", got, err)
	}

	tests := []struct {
		name string
		got  int64
		want int64
	}{
		{"seconds", out.Seconds, fixed.Unix()},
		{"milliseconds", out.Milliseconds, fixed.UnixMilli()},
		{"microseconds", out.Microseconds, fixed.UnixMicro()},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}

	// 三个字段同源于一次 now()，单位换算后必须自洽。
	if out.Milliseconds/1000 != out.Seconds {
		t.Errorf("毫秒与秒不自洽: %d vs %d", out.Milliseconds, out.Seconds)
	}
	if out.Microseconds/1000 != out.Milliseconds {
		t.Errorf("微秒与毫秒不自洽: %d vs %d", out.Microseconds, out.Milliseconds)
	}

	parsed, err := time.ParseInLocation(dateFormatStr, out.Timestamp, loc)
	if err != nil {
		t.Fatalf("timestamp 无法按 %s 解析: %v", dateFormatStr, err)
	}
	if !parsed.Equal(fixed) {
		t.Errorf("timestamp = %s, want %s", parsed.Format(dateFormatStr), fixed.Format(dateFormatStr))
	}
}

// TestNewGetCurrentTimeToolInfo 覆盖 InferTool 对空入参结构体的反射结果：
// 接线后模型只能从 Info 拿到参数 schema，schema 退化会让无参调用直接失败。
func TestNewGetCurrentTimeToolInfo(t *testing.T) {
	tl, err := NewGetCurrentTimeTool(context.Background())
	if err != nil {
		t.Fatalf("创建工具失败: %v", err)
	}

	info, err := tl.Info(context.Background())
	if err != nil {
		t.Fatalf("获取工具元信息失败: %v", err)
	}
	if info.Name != "get_current_time" {
		t.Errorf("工具名 = %q, want %q", info.Name, "get_current_time")
	}
	if info.Desc == "" {
		t.Error("工具描述为空，模型无法判断调用时机")
	}
	if info.ParamsOneOf == nil {
		t.Fatal("ParamsOneOf 为 nil，模型拿不到参数定义")
	}

	js, err := info.ToJSONSchema()
	if err != nil {
		t.Fatalf("参数 schema 解析失败: %v", err)
	}
	if js == nil {
		t.Fatal("参数 schema 为 nil")
	}
	if js.Type != "object" {
		t.Errorf("参数 schema type = %q, want %q", js.Type, "object")
	}
	if js.Properties == nil || js.Properties.Len() != 0 {
		t.Errorf("参数 schema 应为无属性的 object，实际 properties = %v", js.Properties)
	}
}
