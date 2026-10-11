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
	mem.SetDefaultWindowSize(cfg.Memory.MaxWindowSize)
	mem.StartSessionGC(ctx, cfg.Memory.SessionTTL, cfg.Memory.CleanupInterval)

	// 配置热更新：设置 CONFIG_RELOAD_INTERVAL（如 30s）后按周期重载配置文件。
	reloadInterval, err := config.ReloadInterval()
	if err != nil {
		g.Log().Warningf(ctx, "解析 %s 失败，已关闭配置热更新: %v", config.EnvConfigReloadInterval, err)
	}
	config.StartAutoReload(ctx, reloadInterval, func(c *config.Config) {
		// 会话窗口在启动时设定，重载后重新应用；GC 周期固定，需重启。
		mem.SetDefaultWindowSize(c.Memory.MaxWindowSize)
	})

	s := g.Server()
	s.Group("/api", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.CORSMiddleware)
		group.Middleware(middleware.ResponseMiddleware)
		group.Bind(chat.NewV1())
	})
	s.SetPort(6872)
	s.Run()
}
