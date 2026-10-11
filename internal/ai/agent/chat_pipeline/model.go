package chat_pipeline

import (
	"SuperBizAgent/internal/ai/models"
	"context"

	"github.com/cloudwego/eino/components/model"
)

func newChatModel(ctx context.Context) (cm model.ToolCallingChatModel, err error) {
	// TODO Modify component configuration here.
	// config := &openai.ChatModelConfig{}
	// cm, err = openai.NewChatModel(ctx, config)
	cm, err = models.OpenAIForQwenFlash(ctx)
	if err != nil {
		return nil, err
	}
	// 测试通过，问题不后续环节
	// result, err := cm.Generate(ctx, []*schema.Message{
	// 	schema.SystemMessage("你是一个Go 语言助手，可以回答用户的问题，回答简介高效，直接给出结果。"),
	// 	schema.UserMessage("简单解释 go channal"),
	// })
	// if err != nil {
	// 	fmt.Printf("LLM err: %w", err)
	// }
	// fmt.Printf("LLM: %s\n", result.Content)
	return cm, nil
}
