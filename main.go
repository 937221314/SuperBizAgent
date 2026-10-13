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

	// 配置热更新：配置文件变更后由 gcfg 的文件监听触发，无需轮询。
	config.Watch(func(c *config.Config) {
		// 会话窗口在启动时设定，重载后重新应用；GC 周期固定，需重启。
		mem.SetDefaultWindowSize(c.Memory.MaxWindowSize)
	})

	s := g.Server()
	// HTTP 访问/错误日志复用应用日志，写入同一文件；须在 Run 之前设置，运行期不可改。
	if err := s.SetConfigWithMap(map[string]any{
		"accessLogPattern": g.Log().GetConfig().File,
		"errorLogPattern":  g.Log().GetConfig().File,
	}); err != nil {
		g.Log().Warningf(ctx, "设置访问/错误日志文件失败，将使用默认 pattern: %v", err)
	}
	s.SetLogger(g.Log())
	s.SetAccessLogEnabled(true)
	s.SetErrorLogEnabled(true)
	s.Group("/api", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.CORSMiddleware)
		group.Middleware(middleware.ResponseMiddleware)
		group.Bind(chat.NewV1())
	})
	s.SetPort(6872)
	s.Run()
}
