package main

import (
	"SuperBizAgent/internal/ai/retriever"
	"context"
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// fail 统一打印错误并以非 0 退出码结束，便于脚本判断本次测试是否成功。
func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func main() {
	_ = godotenv.Load()
	ctx := context.Background()
	r, err := retriever.NewMilvusRetriever(ctx)
	if err != nil {
		fail("构建检索器失败: %v", err)
	}

	query := "服务下线的原因"
	docs, err := r.Retrieve(ctx, query)
	if err != nil {
		fail("检索文档失败: %v", err)
	}

	fmt.Println("Q: ", query)
	for _, doc := range docs {
		fmt.Println("A: ", doc.Content)
	}
	fmt.Println("Done", len(docs))
}
