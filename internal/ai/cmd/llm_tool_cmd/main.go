package main

import (
	"SuperBizAgent/internal/ai/models"
	glocaltools "SuperBizAgent/internal/ai/tools"
	"SuperBizAgent/internal/ai/tools/currenttime"
	"SuperBizAgent/internal/ai/tools/mysql"
	"context"
	"fmt"
	"os"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/joho/godotenv"
)

// fail 统一打印错误并以非 0 退出码结束，便于脚本判断本次测试是否成功。
func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// LLM 工具测试：校验工具 schema 能否成功绑定到模型，并观察模型的输出。
// 注意本入口不含 ToolsNode，只验证「模型知道有哪些工具」，不执行工具调用。
func main() {
	_ = godotenv.Load() // 自动读取当前目录下的 .env 文件
	ctx := context.Background()
	cm, err := models.OpenAIForQwenFlash(ctx)
	if err != nil {
		fail("创建模型实例失败: %v", err)
	}
	// 获取工具信息
	toolList, err := glocaltools.GetLogMcpTool(ctx)
	if err != nil {
		fail("获取MCP日志工具失败: %v", err)
	}
	currentTimeTool, err := currenttime.NewGetCurrentTimeTool(ctx)
	if err != nil {
		fail("获取当前时间工具失败: %v", err)
	}
	mysqlQueryTool, err := mysql.NewMysqlQueryTool(ctx)
	if err != nil {
		fail("创建MySQL检索工具失败: %v", err)
	}
	toolList = append(toolList, currentTimeTool, mysqlQueryTool)
	toolInfos := make([]*schema.ToolInfo, 0, len(toolList))
	for _, t := range toolList {
		info, err := t.Info(ctx)
		if err != nil {
			// 任一工具元信息失败都终止：残余工具集会掩盖失败，测试结论不可信。
			fail("获取工具信息失败: %v", err)
		}
		toolInfos = append(toolInfos, info)
	}
	// 将 tools 绑定到 语言模型
	if err = cm.BindTools(toolInfos); err != nil {
		fail("工具信息绑定到模型失败: %v", err)
	}
	// 创建编排
	chain := compose.NewChain[[]*schema.Message, *schema.Message]()
	chain.AppendChatModel(cm, compose.WithNodeName("chat_model"))

	// 编译并运行
	agent, err := chain.Compile(ctx)
	if err != nil {
		fail("编排编译失败: %v", err)
	}
	fmt.Println("人：告诉我,你有哪些工具可以使用？")
	// 运行示例
	result, err := agent.Invoke(ctx, []*schema.Message{
		schema.SystemMessage("你是一个模型助手，可以回答用户的问题，回复结果简单高效，简洁明了，对于不知道的问题直接回答不知道，不用伪造结果。"),
		schema.UserMessage("告诉我,你有哪些工具可以使用？"),
	})
	if err != nil {
		fail("运行智能体失败: %v", err)
	}
	// 输出结果：链上没有 ToolsNode，模型若返回工具调用，Content 会为空，需显式区分。
	if len(result.ToolCalls) > 0 {
		fmt.Println("LLM 请求调用工具（本入口无 ToolsNode，无法执行）：", result.ToolCalls)
		return
	}
	fmt.Println("LLM：", result.Content)
}
