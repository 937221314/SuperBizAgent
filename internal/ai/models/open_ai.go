package models

import (
	"context"
	"os"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/gogf/gf/v2/frame/g"
)

// OpenAIForQwenFlash 基于 dashscope_chat_flash 配置创建兼容 OpenAI 协议的对话模型。
func OpenAIForQwenFlash(ctx context.Context) (cm *openai.ChatModel, err error) {
	model, err := g.Cfg().Get(ctx, "dashscope_chat_flash.model")
	if err != nil {
		return nil, err
	}
	key, err := g.Cfg().Get(ctx, "dashscope_chat_flash.api_key")
	if err != nil {
		return nil, err
	}
	apiKey := key.String()
	base, err := g.Cfg().Get(ctx, "dashscope_chat_flash.base_url")
	if err != nil {
		return nil, err
	}

	if apiKey == "" || apiKey == "DASHSCOPE_API_KEY" {
		apiKey = os.Getenv("DASHSCOPE_API_KEY")
	}
	config := &openai.ChatModelConfig{
		Model:       model.String(),
		APIKey:      apiKey,
		BaseURL:     base.String(),
		Temperature: new(float32(0.7)), // 默认温度
		MaxTokens:   new(2048),         // 默认最大 Token 数
		TopP:        new(float32(0.9)), // 默认 Top-P
		Timeout:     120 * time.Second, // 请求超时时间
	}
	cm, err = openai.NewChatModel(ctx, config)
	if err != nil {
		return nil, err
	}
	return cm, nil
}

// OpenAIForQwenThink 基于 dashscope_chat_plus 配置创建兼容 OpenAI 协议的对话模型。
func OpenAIForQwenThink(ctx context.Context) (cm *openai.ChatModel, err error) {
	model, err := g.Cfg().Get(ctx, "dashscope_chat_plus.model")
	if err != nil {
		return nil, err
	}
	key, err := g.Cfg().Get(ctx, "dashscope_chat_plus.api_key")
	if err != nil {
		return nil, err
	}
	apiKey := key.String()
	base, err := g.Cfg().Get(ctx, "dashscope_chat_plus.base_url")
	if err != nil {
		return nil, err
	}

	if apiKey == "" || apiKey == "DASHSCOPE_API_KEY" {
		apiKey = os.Getenv("DASHSCOPE_API_KEY")
	}
	config := &openai.ChatModelConfig{
		Model:       model.String(),
		APIKey:      apiKey,
		BaseURL:     base.String(),
		Temperature: new(float32(0.7)), // 默认温度
		MaxTokens:   new(2048),         // 默认最大 Token 数
		TopP:        new(float32(0.9)), // 默认 Top-P
		Timeout:     120 * time.Second, // 请求超时时间
	}
	cm, err = openai.NewChatModel(ctx, config)
	if err != nil {
		return nil, err
	}
	return cm, nil
}
