package prometheus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/tool"
)

// fixedNow 与 fixedActiveAt 相差 2h30m15s，用于精确断言 duration。
var fixedNow = func() time.Time { return time.Date(2025, 10, 29, 11, 18, 57, 0, time.UTC) }

const fixedActiveAt = "2025-10-29T08:48:42Z"

// newTestTool 用 httptest 服务替换真实 Prometheus，避免测试依赖外部进程。
func newTestTool(t *testing.T, handler http.HandlerFunc) tool.InvokableTool {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	tl, err := newPrometheusAlertsQueryTool(srv.URL, srv.Client(), fixedNow)
	if err != nil {
		t.Fatalf("创建工具失败: %v", err)
	}
	return tl
}

// invoke 调用工具并把输出解析成结构体；同时断言工具本身不返回 error。
func invoke(t *testing.T, tl tool.InvokableTool) AlertsOutput {
	t.Helper()

	// 模型调用无参工具时传的是空对象。
	raw, err := tl.InvokableRun(context.Background(), "{}")
	if err != nil {
		t.Fatalf("工具调用不应返回 error（失败原因要写进输出），实际: %v", err)
	}

	var out AlertsOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("输出不是合法 JSON（%s）: %v", raw, err)
	}
	return out
}

// writeAlerts 让测试用例只关心 payload，响应状态码固定 200。
func writeAlerts(t *testing.T, payload string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}
}

// alertsPayload 拼出 Prometheus /api/v1/alerts 的响应体。
func alertsPayload(t *testing.T, alerts ...map[string]any) string {
	t.Helper()

	b, err := json.Marshal(map[string]any{
		"status": "success",
		"data":   map[string]any{"alerts": alerts},
	})
	if err != nil {
		t.Fatalf("构造响应体失败: %v", err)
	}
	return string(b)
}

// TestQueryPrometheusAlertsToolDeduplicates 校验相同 alertname 只保留第一条，且时长按注入时钟计算。
func TestQueryPrometheusAlertsToolDeduplicates(t *testing.T) {
	payload := alertsPayload(t,
		map[string]any{
			"labels":      map[string]any{"alertname": "HighCPU"},
			"annotations": map[string]any{"description": "CPU 持续高于 90%"},
			"state":       "firing",
			"active_at":   fixedActiveAt,
		},
		map[string]any{
			"labels":      map[string]any{"alertname": "HighCPU", "instance": "10.0.0.2"},
			"annotations": map[string]any{"description": "重复的同类告警，应被丢弃"},
			"state":       "firing",
			"active_at":   fixedActiveAt,
		},
		map[string]any{
			"labels":      map[string]any{"alertname": "DiskFull"},
			"annotations": map[string]any{"description": "磁盘使用率高于 95%"},
			"state":       "pending",
			"active_at":   "2025-10-29T11:15:57Z",
		},
	)

	tl := newTestTool(t, writeAlerts(t, payload))
	out := invoke(t, tl)

	if !out.Success {
		t.Fatalf("期望成功，实际 error=%q", out.Error)
	}
	if len(out.Alerts) != 2 {
		t.Fatalf("期望去重后 2 条告警，实际 %d 条: %+v", len(out.Alerts), out.Alerts)
	}

	first := out.Alerts[0]
	if first.AlertName != "HighCPU" {
		t.Errorf("首条告警名 = %q, want %q", first.AlertName, "HighCPU")
	}
	if first.Description != "CPU 持续高于 90%" {
		t.Errorf("应保留第一条的描述，实际 %q", first.Description)
	}
	if first.Duration != "2h30m15s" {
		t.Errorf("duration = %q, want %q", first.Duration, "2h30m15s")
	}
	if got := out.Alerts[1].Duration; got != "3m0s" {
		t.Errorf("第二条 duration = %q, want %q", got, "3m0s")
	}
	if !strings.Contains(out.Message, "2 条") {
		t.Errorf("message 未包含条数: %q", out.Message)
	}
}

// TestQueryPrometheusAlertsToolSkipsInvalidAlertName 校验 alertname 缺失或非字符串时跳过而不是 panic，
// 也不会因空 alertname 把所有异常告警去重成一条。
func TestQueryPrometheusAlertsToolSkipsInvalidAlertName(t *testing.T) {
	payload := alertsPayload(t,
		map[string]any{
			"labels":      map[string]any{"instance": "10.0.0.1"},
			"annotations": map[string]any{"description": "没有 alertname"},
			"state":       "firing",
			"active_at":   fixedActiveAt,
		},
		map[string]any{
			"labels":      map[string]any{"alertname": 42},
			"annotations": map[string]any{"description": "alertname 类型不对"},
			"state":       "firing",
			"active_at":   fixedActiveAt,
		},
		map[string]any{
			"labels":      map[string]any{"alertname": ""},
			"annotations": map[string]any{"description": "alertname 为空串"},
			"state":       "firing",
			"active_at":   fixedActiveAt,
		},
		map[string]any{
			"labels":      map[string]any{"alertname": "NodeDown"},
			"annotations": map[string]any{"description": "节点不可达"},
			"state":       "firing",
			"active_at":   fixedActiveAt,
		},
	)

	tl := newTestTool(t, writeAlerts(t, payload))
	out := invoke(t, tl)

	if !out.Success {
		t.Fatalf("期望成功，实际 error=%q", out.Error)
	}
	if len(out.Alerts) != 1 || out.Alerts[0].AlertName != "NodeDown" {
		t.Fatalf("期望只返回 NodeDown，实际 %+v", out.Alerts)
	}
	if !strings.Contains(out.Message, "3 条") {
		t.Errorf("message 未说明跳过条数: %q", out.Message)
	}
}

// TestQueryPrometheusAlertsToolToleratesMissingFields 校验 description 缺失与 active_at 非法时的降级形态。
func TestQueryPrometheusAlertsToolToleratesMissingFields(t *testing.T) {
	payload := alertsPayload(t,
		map[string]any{
			"labels":    map[string]any{"alertname": "NoDescription"},
			"state":     "firing",
			"active_at": fixedActiveAt,
		},
		map[string]any{
			"labels":      map[string]any{"alertname": "BadTime"},
			"annotations": map[string]any{"description": "激活时间格式不对"},
			"state":       "firing",
			"active_at":   "not-a-time",
		},
	)

	tl := newTestTool(t, writeAlerts(t, payload))
	out := invoke(t, tl)

	if !out.Success {
		t.Fatalf("期望成功，实际 error=%q", out.Error)
	}
	if len(out.Alerts) != 2 {
		t.Fatalf("期望 2 条告警，实际 %+v", out.Alerts)
	}
	if out.Alerts[0].Description != "" {
		t.Errorf("缺 description 时期望空串，实际 %q", out.Alerts[0].Description)
	}
	if out.Alerts[1].Duration != "" {
		t.Errorf("非法 active_at 时期望空 duration，实际 %q", out.Alerts[1].Duration)
	}
}

// TestQueryPrometheusAlertsToolStatusError 校验 status=error 时读出 error/errorType，而不是当成 0 条告警。
func TestQueryPrometheusAlertsToolStatusError(t *testing.T) {
	payload := `{"status":"error","errorType":"bad_data","error":"invalid parameter"}`

	tl := newTestTool(t, writeAlerts(t, payload))
	out := invoke(t, tl)

	if out.Success {
		t.Fatal("status=error 时不应返回成功")
	}
	if !strings.Contains(out.Error, "bad_data") || !strings.Contains(out.Error, "invalid parameter") {
		t.Errorf("error 未带上 errorType 与原因: %q", out.Error)
	}
	if len(out.Alerts) != 0 {
		t.Errorf("失败时不应返回告警: %+v", out.Alerts)
	}
}

// TestQueryPrometheusAlertsToolHTTPError 校验非 2xx 时给出状态码与响应片段。
func TestQueryPrometheusAlertsToolHTTPError(t *testing.T) {
	tl := newTestTool(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	out := invoke(t, tl)

	if out.Success {
		t.Fatal("HTTP 500 时不应返回成功")
	}
	if !strings.Contains(out.Error, "500") || !strings.Contains(out.Error, "boom") {
		t.Errorf("error 未带上状态码与响应片段: %q", out.Error)
	}
}

// TestQueryPrometheusAlertsToolInvalidJSON 校验解析失败走失败输出而不是 error。
func TestQueryPrometheusAlertsToolInvalidJSON(t *testing.T) {
	tl := newTestTool(t, writeAlerts(t, "{not json"))
	out := invoke(t, tl)

	if out.Success {
		t.Fatal("响应不是合法 JSON 时不应返回成功")
	}
	if !strings.Contains(out.Error, "解析") {
		t.Errorf("error 未说明解析失败: %q", out.Error)
	}
}

// TestQueryPrometheusAlertsToolResponseTooLarge 校验超大响应被上限拦截，不会整体读进内存后当成合法 JSON。
func TestQueryPrometheusAlertsToolResponseTooLarge(t *testing.T) {
	huge := `{"status":"success","data":{"alerts":[]},"padding":"` +
		strings.Repeat("a", maxResponseBytes) + `"}`

	tl := newTestTool(t, writeAlerts(t, huge))
	out := invoke(t, tl)

	if out.Success {
		t.Fatal("超过上限的响应不应返回成功")
	}
	if !strings.Contains(out.Error, "上限") {
		t.Errorf("error 未说明超限: %q", out.Error)
	}
}

// TestQueryPrometheusAlertsToolContextCanceled 校验调用方 ctx 会中止请求（依赖 NewRequestWithContext）。
func TestQueryPrometheusAlertsToolContextCanceled(t *testing.T) {
	tl := newTestTool(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	raw, err := tl.InvokableRun(ctx, "{}")
	if err != nil {
		t.Fatalf("工具调用不应返回 error，实际: %v", err)
	}

	var out AlertsOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("输出不是合法 JSON（%s）: %v", raw, err)
	}
	if out.Success {
		t.Fatal("ctx 已取消时不应返回成功")
	}
	if !strings.Contains(out.Error, "context canceled") {
		t.Errorf("error 未体现 ctx 取消: %q", out.Error)
	}
}

// TestFormatDuration 覆盖时长格式化的各档位与边界。
func TestFormatDuration(t *testing.T) {
	base := time.Date(2025, 10, 29, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		activeAt string
		now      time.Time
		want     string
	}{
		{"小时档", "2025-10-29T09:29:45Z", base, "2h30m15s"},
		{"分钟档", "2025-10-29T11:29:45Z", base, "30m15s"},
		{"秒档", "2025-10-29T11:59:45Z", base, "15s"},
		{"不足一秒", "2025-10-29T11:59:59.5Z", base, "0s"},
		{"时钟回拨", "2025-10-29T12:00:30Z", base, "0s"},
		{"无法解析", "not-a-time", base, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatDuration(tc.activeAt, tc.now); got != tc.want {
				t.Errorf("formatDuration(%q) = %q, want %q", tc.activeAt, got, tc.want)
			}
		})
	}
}

// TestNewPrometheusAlertsQueryToolInfo 覆盖 InferTool 对空入参结构体的反射结果：
// 接线后模型只能从 Info 拿到参数 schema，schema 退化会让无参调用直接失败。
func TestNewPrometheusAlertsQueryToolInfo(t *testing.T) {
	tl, err := NewPrometheusAlertsQueryTool(context.Background())
	if err != nil {
		t.Fatalf("创建工具失败: %v", err)
	}

	info, err := tl.Info(context.Background())
	if err != nil {
		t.Fatalf("获取工具元信息失败: %v", err)
	}
	if info.Name != "query_prometheus_alerts" {
		t.Errorf("工具名 = %q, want %q", info.Name, "query_prometheus_alerts")
	}
	if info.Desc == "" {
		t.Error("工具描述为空，模型无法判断调用时机")
	}
	if !strings.Contains(info.Desc, "alertname") {
		t.Error("工具描述未说明去重规则")
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
