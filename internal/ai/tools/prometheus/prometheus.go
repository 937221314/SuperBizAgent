// Package prometheus 提供查询 Prometheus 活跃告警的工具 query_prometheus_alerts。
//
// 工具只读 Prometheus 的 /api/v1/alerts 接口，产出 eino 的 tool.InvokableTool；
// 查询失败也返回结构化结果而不是 error，让模型能自我纠正。工具尚未挂载，
// 见 dev-docs/todo.md 的「工具尚未挂载」。
package prometheus

import "time"

const (
	// toolDescQueryPrometheusAlerts 是工具描述，模型依赖它判断是否调用，需写清适用场景、
	// 去重规则与失败形态。
	toolDescQueryPrometheusAlerts = "从 Prometheus 警报系统查询活动警报。此工具检索所有当前活动/触发的警报，" +
		"包括其名称、描述、状态、激活时间和持续时间。返回结果按 alertname 去重，相同 alertname 只保留第一条；" +
		"返回 success=false 时 error 字段说明失败原因。" +
		"当您需要检查当前触发的警报、调查警报情况或监视警报状态时，请使用此工具。"

	// requestTimeout 是单次查询的兜底超时：调用方没传 deadline 时也要能返回。
	requestTimeout = 10 * time.Second

	// maxResponseBytes 限制读取的响应体大小，避免异常响应撑爆内存。
	maxResponseBytes = 1 << 20

	// maxErrorMessageBytes 限制透传给模型的响应片段长度。
	maxErrorMessageBytes = 200
)
