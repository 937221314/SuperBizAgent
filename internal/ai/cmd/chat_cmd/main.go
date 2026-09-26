package main

import (
	"SuperBizAgent/internal/ai/agent/chat_pipeline"
	"SuperBizAgent/utility/mem"
	"context"
	"fmt"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/joho/godotenv"
)

// 文本模型输出测试
// ../agent/chat_pipeline
func main() {
	fmt.Println("文本模型输出测试")
	ctx := context.Background()
	_ = godotenv.Load() // 自动读取当前目录下的 .env 文件

	id := "111"
	userMessage := &chat_pipeline.UserMessage{
		ID:      id,
		Query:   "你好",
		History: mem.GetSimpleMemory(id).GetMessages(),
	}

	runner, err := chat_pipeline.BuildEinoAgent(ctx)
	if err != nil {
		// panic(err)
		g.Log().Fatalf(ctx, "创建 runner 失败:%v", err)
		return
	}

	// 第一次对话
	// out, err := runner.Invoke(ctx, userMessage, compose.WithCallbacks(logcallback.New(nil)))
	out, err := runner.Invoke(ctx, userMessage)
	if err != nil {
		// panic(err)
		g.Log().Fatalf(ctx, "执行 agent 失败:%v", err)
		return
	}

	answer := out.Content
	fmt.Println("Q: 你好")
	fmt.Println("LLM:", answer)
	mem.GetSimpleMemory(id).SetMessage(schema.UserMessage("你好"))
	mem.GetSimpleMemory(id).SetMessage(schema.SystemMessage(answer))

	// 第二轮对话
	userMessage = &chat_pipeline.UserMessage{
		ID:      id,
		Query:   "现在几点了",
		History: mem.GetSimpleMemory(id).GetMessages(),
	}

	// out, err = runner.Invoke(ctx, userMessage, compose.WithCallbacks(logcallback.New(nil)))
	out, err = runner.Invoke(ctx, userMessage)
	if err != nil {
		// panic(err)
		g.Log().Fatalf(ctx, "执行 agent 失败:%v", err)
		return
	}
	answer = out.Content
	fmt.Println("第二轮对话")
	fmt.Println("Q: 现在几点了")
	fmt.Println("LLM:", answer)
}
