package main

import (
	"context"

	"SuperBizAgent/internal/config"
	"SuperBizAgent/internal/controller/chat"
	"SuperBizAgent/utility/mem"
	"SuperBizAgent/utility/middleware"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load() // 自动读取当前目录下的 .env 文件

	ctx := context.Background()
	cfg, err := config.Get()
	if err != nil {
		g.Log().Fatalf(ctx, "加载配置失败: %v", err)
	}
	// 后台回收空闲会话，避免长跑进程中内存只增不减。
	mem.StartSessionGC(ctx, cfg.Memory.SessionTTL, cfg.Memory.CleanupInterval)

	s := g.Server()
	s.Group("/api", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.CORSMiddleware)
		group.Middleware(middleware.ResponseMiddleware)
		group.Bind(chat.NewV1())
	})
	s.SetPort(6872)
	s.Run()
}
