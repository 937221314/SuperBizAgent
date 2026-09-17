package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"SuperBizAgent/internal/config"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/gogf/gf/v2/frame/g"
)

// prometheusAlertsResult 是 /api/v1/alerts 的原始响应。
type prometheusAlertsResult struct {
	Status    string     `json:"status"`
	Data      alertsData `json:"data"`
	Error     string     `json:"error"`
	ErrorType string     `json:"errorType"`
}

// alertsData 是原始响应的 data 段。
type alertsData struct {
	Alerts []rawAlert `json:"alerts"`
}

// rawAlert 是 Prometheus 返回的单条告警。
// labels 与 annotations 的值类型在 JSON 里不确定（字符串、数字、数组都可能），
// 取值时必须做带 ok 的断言，不能直接 `.(string)`。
type rawAlert struct {
	Labels      map[string]any `json:"labels"`
	Annotations map[string]any `json:"annotations"`
	State       string         `json:"state"`
	ActiveAt    string         `json:"active_at"`
	Value       string         `json:"value"`
}

// SimplifiedAlert 是回传给模型的一条告警，只保留模型需要的字段。
type SimplifiedAlert struct {
	AlertName   string `json:"alert_name" jsonschema:"description=告警名称，从 Prometheus 告警的 labels.alertname 字段提取"`
	Description string `json:"description" jsonschema:"description=告警描述信息，从 Prometheus 告警的 annotations.description 字段提取，缺失时为空串"`
	State       string `json:"state" jsonschema:"description=告警状态，通常为 'firing'（触发中）或 'pending'（待触发）"`
	ActiveAt    string `json:"active_at" jsonschema:"description=告警激活时间，RFC3339 格式的时间戳，例如 '2025-10-29T08:48:42.496134755Z'"`
	Duration    string `json:"duration" jsonschema:"description=告警持续时间，从激活时间到当前时间的时长，格式如 '2h30m15s'、'30m15s' 或 '15s'；激活时间无法解析时为空串"`
}

// AlertsOutput 是 query_prometheus_alerts 工具的输出。
type AlertsOutput struct {
	// Success 查询是否成功。失败时 Alerts 为空，原因见 Error。
	Success bool `json:"success" jsonschema:"description=查询是否成功"`
	// Alerts 活动告警列表，相同 alertname 只保留第一条。
	Alerts []SimplifiedAlert `json:"alerts,omitempty" jsonschema:"description=活动告警列表，每个告警包含名称、描述、状态、激活时间和持续时间。相同 alertname 的告警只保留第一个"`
	// Message 操作结果的状态消息。
	Message string `json:"message,omitempty" jsonschema:"description=操作结果的状态消息"`
	// Error 查询失败时的原因，成功时为空。
	Error string `json:"error,omitempty" jsonschema:"description=如果查询失败，包含错误信息"`
}

// queryAlerts 调用 Prometheus 的 /api/v1/alerts 并返回原始告警列表。
//
// 返回的 error 表示「这次查询没拿到数据」，由调用方写进输出而不是当成工具调用失败。
func queryAlerts(ctx context.Context, baseURL string, hc *http.Client) ([]rawAlert, error) {
	apiURL := strings.TrimRight(baseURL, "/") + "/api/v1/alerts"

	g.Log().Printf(ctx, "查询 Prometheus 告警: %s", apiURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("构造 Prometheus 告警请求失败: %w", err)
	}

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 Prometheus 告警失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 多读一个字节用于判断是否超限，避免把超限的响应当成截断后的合法 JSON 解析。
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("读取 Prometheus 告警响应失败: %w", err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("查询 Prometheus 告警的响应超过 %d 字节上限", maxResponseBytes)
	}

	// 非 2xx 时响应体通常是 Prometheus 的错误 JSON，截断后带上原文便于排查。
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("查询 Prometheus 告警接口返回 %s: %s", resp.Status, truncateErrorMessage(string(body)))
	}

	var result prometheusAlertsResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析 Prometheus 告警响应失败: %w", err)
	}
	// status 非 success 时 Prometheus 用 error/errorType 说明原因，必须读出来，
	// 否则查询被拒会被静默当成「0 条告警」。
	if result.Status != "success" {
		return nil, fmt.Errorf("查询 Prometheus 告警失败（status=%s, errorType=%s）: %s",
			result.Status, result.ErrorType, truncateErrorMessage(result.Error))
	}

	return result.Data.Alerts, nil
}

// truncateErrorMessage 截断要拼进错误信息的原文，避免占满模型上下文。
func truncateErrorMessage(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxErrorMessageBytes {
		return s
	}
	return s[:maxErrorMessageBytes] + "..."
}

// simplifyAlerts 把原始告警转成模型视角的列表，相同 alertname 只保留第一条，
// 返回跳过的条数（alertname 缺失或非字符串）。
//
// 跳过而不是用空串当 key：否则所有配置异常的告警会被去重成一条。
func simplifyAlerts(alerts []rawAlert, now time.Time) (simplified []SimplifiedAlert, skipped int) {
	seen := make(map[string]bool, len(alerts))
	simplified = make([]SimplifiedAlert, 0, len(alerts))

	for _, alert := range alerts {
		alertName, ok := alert.Labels["alertname"].(string)
		if !ok || alertName == "" {
			skipped++
			continue
		}
		if seen[alertName] {
			continue
		}
		seen[alertName] = true

		// 缺失或类型不符时留空串：description 只是给人看的说明，不值得让整条告警消失。
		description, _ := alert.Annotations["description"].(string)

		simplified = append(simplified, SimplifiedAlert{
			AlertName:   alertName,
			Description: description,
			State:       alert.State,
			ActiveAt:    alert.ActiveAt,
			Duration:    formatDuration(alert.ActiveAt, now),
		})
	}

	return simplified, skipped
}

// formatDuration 把 active_at 到 now 的时长格式化成 2h30m15s / 30m15s / 15s。
// active_at 无法解析时返回空串，让模型看到「时长未知」而不是一个可疑数值。
func formatDuration(activeAt string, now time.Time) string {
	start, err := time.Parse(time.RFC3339, activeAt)
	if err != nil {
		return ""
	}

	duration := now.Sub(start)
	if duration < 0 {
		// 两端时钟不同步或时钟回拨时不要给出负时长。
		duration = 0
	}
	duration = duration.Truncate(time.Second)

	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60

	switch {
	case hours > 0:
		return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
	case minutes > 0:
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	default:
		return fmt.Sprintf("%ds", seconds)
	}
}

// NewPrometheusAlertsQueryTool 创建查询 Prometheus 活跃告警的工具（query_prometheus_alerts）。
// 地址取自配置项 prometheus.base_url；ctx 只用于与同包其它构造函数保持一致的调用形状，
// 工具本身不依赖构建期上下文。
func NewPrometheusAlertsQueryTool(ctx context.Context) (tool.InvokableTool, error) {
	cfg, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}
	return newPrometheusAlertsQueryTool(cfg.Prometheus.BaseURL, &http.Client{Timeout: requestTimeout}, time.Now)
}

// newPrometheusAlertsQueryTool 基于注入的地址、HTTP 客户端与时钟构建工具，便于测试替换掉真实 Prometheus。
func newPrometheusAlertsQueryTool(baseURL string, hc *http.Client, now func() time.Time) (tool.InvokableTool, error) {
	handler := func(ctx context.Context, _ *struct{}) (AlertsOutput, error) {
		g.Log().Printf(ctx, "查询 Prometheus 活跃告警")

		alerts, err := queryAlerts(ctx, baseURL, hc)
		if err != nil {
			// 查询失败不算工具调用失败：原因写进输出，模型可以据此告知用户或换个问题。
			g.Log().Printf(ctx, "查询 Prometheus 告警失败: %v", err)
			return AlertsOutput{
				Success: false,
				Error:   err.Error(),
				Message: "无法查询 Prometheus 告警",
			}, nil
		}

		// 时间是整条响应的共同基准：只在入口取一次，避免各条告警的持续时长互相错位。
		simplified, skipped := simplifyAlerts(alerts, now())

		message := fmt.Sprintf("已成功检索 %d 条活跃告警", len(simplified))
		if skipped > 0 {
			message += fmt.Sprintf("，另有 %d 条因缺少 alertname 被跳过", skipped)
		}

		g.Log().Printf(ctx, "查询 Prometheus 告警完毕，返回 %d 条告警", len(simplified))

		return AlertsOutput{
			Success: true,
			Alerts:  simplified,
			Message: message,
		}, nil
	}

	t, err := utils.InferTool("query_prometheus_alerts", toolDescQueryPrometheusAlerts, handler)
	if err != nil {
		return nil, fmt.Errorf("创建 query_prometheus_alerts 工具失败: %w", err)
	}
	return t, nil
}
