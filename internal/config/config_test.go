package config

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// writeConfigFile 写一份临时配置文件并返回其路径。
func writeConfigFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入配置文件失败: %v", err)
	}
	return path
}

// TestLoadMilvusIndex 校验 milvus.index 的默认值、显式取值与越界拦截。
func TestLoadMilvusIndex(t *testing.T) {
	cases := []struct {
		name        string
		content     string
		wantM       int
		wantEf      int
		wantLoadErr bool
	}{
		{
			name:    "未配置时取默认值",
			content: "",
			wantM:   16,
			wantEf:  200,
		},
		{
			name:    "显式配置",
			content: "milvus:\n  index:\n    m: 32\n    ef_construction: 400\n",
			wantM:   32,
			wantEf:  400,
		},
		{
			name:        "m 越界报错",
			content:     "milvus:\n  index:\n    m: 2\n",
			wantLoadErr: true,
		},
		{
			name:        "ef_construction 越界报错",
			content:     "milvus:\n  index:\n    ef_construction: 1000\n",
			wantLoadErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Load(writeConfigFile(t, c.content))
			if c.wantLoadErr {
				if err == nil {
					t.Fatalf("期望加载失败，实际 m=%d ef=%d", got.Milvus.Index.M, got.Milvus.Index.EfConstruction)
				}
				return
			}
			if err != nil {
				t.Fatalf("加载配置失败: %v", err)
			}
			if got.Milvus.Index.M != c.wantM || got.Milvus.Index.EfConstruction != c.wantEf {
				t.Errorf("index = {%d, %d}，期望 {%d, %d}",
					got.Milvus.Index.M, got.Milvus.Index.EfConstruction, c.wantM, c.wantEf)
			}
		})
	}
}

// TestLoadMemory 校验 memory 段时长的默认值与显式解析。
func TestLoadMemory(t *testing.T) {
	got, err := Load(writeConfigFile(t, ""))
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if got.Memory.SessionTTL != 30*time.Minute || got.Memory.CleanupInterval != 5*time.Minute || got.Memory.MaxWindowSize != 8 {
		t.Errorf("默认 memory = {%s, %s, %d}，期望 {30m, 5m, 8}",
			got.Memory.SessionTTL, got.Memory.CleanupInterval, got.Memory.MaxWindowSize)
	}

	got, err = Load(writeConfigFile(t, "memory:\n  session_ttl: 1h\n  cleanup_interval: 90s\n  max_window_size: 12\n"))
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if got.Memory.SessionTTL != time.Hour || got.Memory.CleanupInterval != 90*time.Second || got.Memory.MaxWindowSize != 12 {
		t.Errorf("显式 memory = {%s, %s, %d}，期望 {1h, 90s, 12}",
			got.Memory.SessionTTL, got.Memory.CleanupInterval, got.Memory.MaxWindowSize)
	}
}

// TestLoadMilvusRecreateOnSchemaMismatch 校验 milvus.recreate_on_schema_mismatch 的解析。
func TestLoadMilvusRecreateOnSchemaMismatch(t *testing.T) {
	got, err := Load(writeConfigFile(t, ""))
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if got.Milvus.RecreateOnSchemaMismatch {
		t.Error("未配置时期望默认为 false")
	}

	got, err = Load(writeConfigFile(t, "milvus:\n  recreate_on_schema_mismatch: true\n"))
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if !got.Milvus.RecreateOnSchemaMismatch {
		t.Error("显式配置 true 时期望为 true")
	}
}

// TestLoadDangerousStatementMode 校验 mysql.dangerous_statement_mode 的默认值、
// 显式取值、环境变量覆盖与非法取值拦截。
func TestLoadDangerousStatementMode(t *testing.T) {
	cases := []struct {
		name        string
		content     string
		env         string
		want        string
		wantLoadErr bool
	}{
		{
			name:    "未配置时取默认 deny",
			content: "",
			env:     "",
			want:    "deny",
		},
		{
			name:    "显式配置 interrupt",
			content: "mysql:\n  dangerous_statement_mode: \"interrupt\"\n",
			env:     "",
			want:    "interrupt",
		},
		{
			name:    "环境变量覆盖配置文件",
			content: "mysql:\n  dangerous_statement_mode: \"deny\"\n",
			env:     "interrupt",
			want:    "interrupt",
		},
		{
			name:        "非法取值在加载期报错",
			content:     "mysql:\n  dangerous_statement_mode: \"drop\"\n",
			env:         "",
			wantLoadErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// 固定环境变量，避免开发者本机环境影响用例。
			t.Setenv(EnvMysqlDangerousStatementMode, c.env)

			got, err := Load(writeConfigFile(t, c.content))
			if c.wantLoadErr {
				if err == nil {
					t.Fatalf("期望加载失败，实际成功且取值为 %q", got.Mysql.DangerousStatementMode)
				}
				return
			}
			if err != nil {
				t.Fatalf("加载配置失败: %v", err)
			}
			if got.Mysql.DangerousStatementMode != c.want {
				t.Errorf("dangerous_statement_mode = %q，期望 %q", got.Mysql.DangerousStatementMode, c.want)
			}
		})
	}
}

// resetConfigForTest 重置 Get/Reload 的包级缓存，仅用于测试隔离。
func resetConfigForTest() {
	initOnce = sync.Once{}
	initErr = nil
	cfgPtr.Store(nil)
}

// TestReload 校验热更新读取新配置，失败时保留旧配置。
func TestReload(t *testing.T) {
	resetConfigForTest()
	t.Cleanup(resetConfigForTest)

	path := writeConfigFile(t, "memory:\n  max_window_size: 4\n")
	t.Setenv("CONFIG_PATH", path)

	got, err := Get()
	if err != nil {
		t.Fatalf("首次加载失败: %v", err)
	}
	if got.Memory.MaxWindowSize != 4 {
		t.Fatalf("首次加载 max_window_size = %d，期望 4", got.Memory.MaxWindowSize)
	}

	if err := os.WriteFile(path, []byte("memory:\n  max_window_size: 12\n"), 0o600); err != nil {
		t.Fatalf("改写配置文件失败: %v", err)
	}
	updated, err := Reload()
	if err != nil {
		t.Fatalf("Reload 失败: %v", err)
	}
	if updated.Memory.MaxWindowSize != 12 {
		t.Fatalf("Reload 后 max_window_size = %d，期望 12", updated.Memory.MaxWindowSize)
	}
	if again, _ := Get(); again == nil || again.Memory.MaxWindowSize != 12 {
		t.Fatal("Get 未看到热更新后的配置")
	}

	// 非法配置：Reload 失败，且保留旧配置。
	if err := os.WriteFile(path, []byte("milvus:\n  address: \"\"\n"), 0o600); err != nil {
		t.Fatalf("改写配置文件失败: %v", err)
	}
	if _, err := Reload(); err == nil {
		t.Fatal("非法配置应使 Reload 失败")
	}
	if again, _ := Get(); again == nil || again.Memory.MaxWindowSize != 12 {
		t.Fatal("Reload 失败后应保留旧配置")
	}
}

// TestReloadInterval 校验环境变量解析。
func TestReloadInterval(t *testing.T) {
	t.Setenv(EnvConfigReloadInterval, "")
	if d, err := ReloadInterval(); err != nil || d != 0 {
		t.Fatalf("空值应返回 (0, nil)，实际 (%s, %v)", d, err)
	}

	t.Setenv(EnvConfigReloadInterval, "30s")
	if d, err := ReloadInterval(); err != nil || d != 30*time.Second {
		t.Fatalf("\"30s\" 应解析为 30s，实际 (%s, %v)", d, err)
	}

	t.Setenv(EnvConfigReloadInterval, "nope")
	if _, err := ReloadInterval(); err == nil {
		t.Fatal("非法时长应返回错误")
	}
}

// TestStartAutoReload 校验定时重载会调用 onReload，且 ctx 取消后 goroutine 退出。
func TestStartAutoReload(t *testing.T) {
	resetConfigForTest()
	t.Cleanup(resetConfigForTest)

	path := writeConfigFile(t, "memory:\n  max_window_size: 4\n")
	t.Setenv("CONFIG_PATH", path)
	if _, err := Get(); err != nil {
		t.Fatalf("首次加载失败: %v", err)
	}
	if err := os.WriteFile(path, []byte("memory:\n  max_window_size: 12\n"), 0o600); err != nil {
		t.Fatalf("改写配置文件失败: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan int, 1)
	done := StartAutoReload(ctx, 10*time.Millisecond, func(c *Config) {
		select {
		case got <- c.Memory.MaxWindowSize:
		default:
		}
	})

	select {
	case v := <-got:
		if v != 12 {
			t.Fatalf("onReload 收到 max_window_size = %d，期望 12", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("自动重载未在预期时间内触发")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("ctx 取消后自动重载 goroutine 未退出")
	}
}

// TestApplyEnvTypedOverrides 校验 int/int64/duration/bool 类环境变量覆盖。
func TestApplyEnvTypedOverrides(t *testing.T) {
	t.Setenv(EnvMilvusDefaultDBName, "default2")
	t.Setenv(EnvMilvusVectorDim, "512")
	t.Setenv(EnvMilvusIndexM, "32")
	t.Setenv(EnvMilvusIndexEfConstruction, "400")
	t.Setenv(EnvMilvusRecreateOnSchemaMismatch, "true")
	t.Setenv(EnvKnowledgeChunkSize, "1500")
	t.Setenv(EnvKnowledgeChunkOverlapSize, "300")
	t.Setenv(EnvMemorySessionTTL, "2h")
	t.Setenv(EnvMemoryCleanupInterval, "1m")
	t.Setenv(EnvMemoryMaxWindowSize, "16")

	got, err := Load(writeConfigFile(t, ""))
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	if got.Milvus.DefaultDBName != "default2" {
		t.Errorf("default_db_name = %q", got.Milvus.DefaultDBName)
	}
	if got.Milvus.VectorDim != 512 {
		t.Errorf("vector_dim = %d", got.Milvus.VectorDim)
	}
	if got.Milvus.Index.M != 32 || got.Milvus.Index.EfConstruction != 400 {
		t.Errorf("index = {%d, %d}", got.Milvus.Index.M, got.Milvus.Index.EfConstruction)
	}
	if !got.Milvus.RecreateOnSchemaMismatch {
		t.Error("recreate_on_schema_mismatch 应为 true")
	}
	if got.KnowledgeChunk.ChunkSize != 1500 || got.KnowledgeChunk.OverlapSize != 300 {
		t.Errorf("knowledge_chunk = {%d, %d}", got.KnowledgeChunk.ChunkSize, got.KnowledgeChunk.OverlapSize)
	}
	if got.Memory.SessionTTL != 2*time.Hour || got.Memory.CleanupInterval != time.Minute || got.Memory.MaxWindowSize != 16 {
		t.Errorf("memory = {%s, %s, %d}", got.Memory.SessionTTL, got.Memory.CleanupInterval, got.Memory.MaxWindowSize)
	}
}

// TestApplyEnvInvalid 校验非法环境变量使加载失败（fail-fast）。
func TestApplyEnvInvalid(t *testing.T) {
	cases := []struct{ key, value string }{
		{EnvMilvusVectorDim, "abc"},
		{EnvKnowledgeChunkSize, "1.5"},
		{EnvMemorySessionTTL, "30"},
		{EnvMilvusRecreateOnSchemaMismatch, "yes"},
	}
	for _, c := range cases {
		t.Run(c.key, func(t *testing.T) {
			t.Setenv(c.key, c.value)
			if _, err := Load(writeConfigFile(t, "")); err == nil {
				t.Fatalf("非法 %s=%q 应使加载失败", c.key, c.value)
			}
		})
	}
}

// TestLoadSSESlowClientPolicy 校验 sse.slow_client_policy 的默认值、显式值、
// 环境变量覆盖与非法值拦截。
func TestLoadSSESlowClientPolicy(t *testing.T) {
	got, err := Load(writeConfigFile(t, ""))
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if got.SSE.SlowClientPolicy != "drop" {
		t.Errorf("默认 slow_client_policy = %q，期望 drop", got.SSE.SlowClientPolicy)
	}

	got, err = Load(writeConfigFile(t, "sse:\n  slow_client_policy: disconnect\n"))
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if got.SSE.SlowClientPolicy != "disconnect" {
		t.Errorf("显式 slow_client_policy = %q，期望 disconnect", got.SSE.SlowClientPolicy)
	}

	t.Setenv(EnvSSESlowClientPolicy, "drop")
	got, err = Load(writeConfigFile(t, "sse:\n  slow_client_policy: disconnect\n"))
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if got.SSE.SlowClientPolicy != "drop" {
		t.Errorf("环境变量覆盖后 = %q，期望 drop", got.SSE.SlowClientPolicy)
	}

	t.Setenv(EnvSSESlowClientPolicy, "")
	if _, err := Load(writeConfigFile(t, "sse:\n  slow_client_policy: block\n")); err == nil {
		t.Fatal("非法 slow_client_policy 应使加载失败")
	}
}
