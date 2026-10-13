package models

import (
	"context"
	"os"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/gogf/gf/v2/frame/g"
)

// enableThinkingField 是 dashscope 兼容 OpenAI 协议的思考模式开关字段，随请求体下发。
const enableThinkingField = "enable_thinking"

// OpenAIForQwenFlash 基于 dashscope_chat_flash 配置创建兼容 OpenAI 协议的对话模型。
func OpenAIForQwenFlash(ctx context.Context) (cm *openai.ChatModel, err error) {
	return newQwenChatModel(ctx, "dashscope_chat_flash", nil)
}

// OpenAIForQwenThink 基于 dashscope_chat_plus 配置创建兼容 OpenAI 协议的对话模型。
func OpenAIForQwenThink(ctx context.Context) (cm *openai.ChatModel, err error) {
	return newQwenChatModel(ctx, "dashscope_chat_plus", nil)
}

// OpenAIForQwenAgent 基于 dashscope_chat_flash 配置创建 plan-execute 智能体使用的模型。
//
// eino 的 planexecute 在规划与再规划时强制 tool_choice=required
// （eino/adk/prebuilt/planexecute/plan_execute.go 的 NewPlanner、NewReplanner），
// 而 dashscope 的思考模式拒绝该取值，请求会返回 400：
// "The tool_choice parameter does not support being set to required or object in thinking mode"，
// 因此这里必须显式关闭思考；对话场景不强制工具选择，仍用 OpenAIForQwenFlash 保留思考。
func OpenAIForQwenAgent(ctx context.Context) (cm *openai.ChatModel, err error) {
	return newQwenChatModel(ctx, "dashscope_chat_flash", map[string]any{enableThinkingField: false})
}

// newQwenChatModel 按 section 读取 dashscope 配置并创建模型。
// extraFields 会合并进每次请求体，用于传递 dashscope 专有参数。
func newQwenChatModel(ctx context.Context, section string, extraFields map[string]any) (*openai.ChatModel, error) {
	config, err := qwenModelConfig(ctx, section, extraFields)
	if err != nil {
		return nil, err
	}
	return openai.NewChatModel(ctx, config)
}

// qwenModelConfig 组装模型参数；api_key 为占位值时回退到环境变量 DASHSCOPE_API_KEY。
func qwenModelConfig(ctx context.Context, section string, extraFields map[string]any) (*openai.ChatModelConfig, error) {
	model, err := g.Cfg().Get(ctx, section+".model")
	if err != nil {
		return nil, err
	}
	key, err := g.Cfg().Get(ctx, section+".api_key")
	if err != nil {
		return nil, err
	}
	apiKey := key.String()
	base, err := g.Cfg().Get(ctx, section+".base_url")
	if err != nil {
		return nil, err
	}

	if apiKey == "" || apiKey == "DASHSCOPE_API_KEY" {
		apiKey = os.Getenv("DASHSCOPE_API_KEY")
	}
	return &openai.ChatModelConfig{
		Model:       model.String(),
		APIKey:      apiKey,
		BaseURL:     base.String(),
		Temperature: new(float32(0.7)), // 默认温度
		MaxTokens:   new(2048),         // 默认最大 Token 数
		TopP:        new(float32(0.9)), // 默认 Top-P
		Timeout:     120 * time.Second, // 请求超时时间
		ExtraFields: extraFields,
	}, nil
}
