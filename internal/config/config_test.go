package config

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/util/gconv"
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

// loadForTest 用临时配置文件加载配置，不触碰进程级单例，供各用例复用。
func loadForTest(t *testing.T, content string) (*Config, error) {
	t.Helper()

	adapter, err := gcfg.NewAdapterFile(writeConfigFile(t, content))
	if err != nil {
		t.Fatalf("初始化配置适配器失败: %v", err)
	}
	return bind(context.Background(), gcfg.NewWithAdapter(adapter))
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
			got, err := loadForTest(t, c.content)
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
	got, err := loadForTest(t, "")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if got.Memory.SessionTTL != 30*time.Minute || got.Memory.CleanupInterval != 5*time.Minute || got.Memory.MaxWindowSize != 8 {
		t.Errorf("默认 memory = {%s, %s, %d}，期望 {30m, 5m, 8}",
			got.Memory.SessionTTL, got.Memory.CleanupInterval, got.Memory.MaxWindowSize)
	}

	got, err = loadForTest(t, "memory:\n  session_ttl: 1h\n  cleanup_interval: 90s\n  max_window_size: 12\n")
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
	got, err := loadForTest(t, "")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if got.Milvus.RecreateOnSchemaMismatch {
		t.Error("未配置时期望默认为 false")
	}

	got, err = loadForTest(t, "milvus:\n  recreate_on_schema_mismatch: true\n")
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

			got, err := loadForTest(t, c.content)
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

// resetWatchersForTest 清空已注册的热更新回调，仅用于测试隔离。
func resetWatchersForTest(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		watchMu.Lock()
		watchFuncs = nil
		watchMu.Unlock()
	})
}

// TestWatchHotReload 校验文件变更触发热更新：重载成功才回调、同一次写入与原子替换的
// 重复事件被去重、非法配置保留旧配置且不回调。
func TestWatchHotReload(t *testing.T) {
	resetWatchersForTest(t)

	path := writeConfigFile(t, "memory:\n  max_window_size: 4\n")
	adapter, err := gcfg.NewAdapterFile(path)
	if err != nil {
		t.Fatalf("初始化配置适配器失败: %v", err)
	}
	cfgs := gcfg.NewWithAdapter(adapter)

	first, err := bind(context.Background(), cfgs)
	if err != nil {
		t.Fatalf("首次加载失败: %v", err)
	}
	if first.Memory.MaxWindowSize != 4 {
		t.Fatalf("首次加载 max_window_size = %d，期望 4", first.Memory.MaxWindowSize)
	}

	var current atomic.Pointer[Config]
	current.Store(first)
	notified := make(chan int, 8)
	Watch(func(c *Config) {
		select {
		case notified <- c.Memory.MaxWindowSize:
		default:
		}
	})
	if err := watch(cfgs, current.Load, current.Store); err != nil {
		t.Fatalf("注册文件监听失败: %v", err)
	}
	// 临时目录随用例结束被删除，先摘掉监听，避免后续用例收到文件消失的告警。
	t.Cleanup(func() { adapter.RemoveWatcher(watcherName) })

	// 内容变化：热更新并回调。
	if err := os.WriteFile(path, []byte("memory:\n  max_window_size: 12\n"), 0o600); err != nil {
		t.Fatalf("改写配置文件失败: %v", err)
	}
	select {
	case v := <-notified:
		if v != 12 {
			t.Fatalf("回调收到 max_window_size = %d，期望 12", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("文件变更未触发热更新")
	}
	if got := current.Load(); got == nil || got.Memory.MaxWindowSize != 12 {
		t.Fatalf("Get 未看到热更新后的配置: %v", current.Load())
	}

	// 同一次写入通常会产生多个文件事件，内容未变时不应重复回调。
	select {
	case v := <-notified:
		t.Fatalf("重复事件不应再次回调，实际收到 %d", v)
	case <-time.After(300 * time.Millisecond):
	}

	// 原子替换（编辑器与 ConfigMap 的常见写法：写临时文件再 rename）会立刻投递
	// Remove+Create 等多个事件，它们并发触发重载也不能重复回调。
	tmp := filepath.Join(filepath.Dir(path), "config.tmp.yaml")
	if err := os.WriteFile(tmp, []byte("memory:\n  max_window_size: 16\n"), 0o600); err != nil {
		t.Fatalf("写入临时配置文件失败: %v", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatalf("替换配置文件失败: %v", err)
	}
	select {
	case v := <-notified:
		if v != 16 {
			t.Fatalf("原子替换后回调收到 max_window_size = %d，期望 16", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("原子替换未触发热更新")
	}
	select {
	case v := <-notified:
		t.Fatalf("原子替换的重复事件不应重复回调，实际收到 %d", v)
	case <-time.After(500 * time.Millisecond):
	}
	if got := current.Load(); got == nil || got.Memory.MaxWindowSize != 16 {
		t.Fatalf("原子替换后 Get 未看到新配置: %v", current.Load())
	}

	// 非法配置：重载失败，保留旧配置且不回调。
	if err := os.WriteFile(path, []byte("milvus:\n  index:\n    m: 2\n"), 0o600); err != nil {
		t.Fatalf("改写配置文件失败: %v", err)
	}
	select {
	case v := <-notified:
		t.Fatalf("非法配置不应回调，实际收到 %d", v)
	case <-time.After(500 * time.Millisecond):
	}
	if got := current.Load(); got == nil || got.Memory.MaxWindowSize != 16 {
		t.Fatalf("热更新失败后应保留旧配置，实际: %v", current.Load())
	}
}

// TestLoadExampleConfig 校验随仓库分发的配置示例能完整绑定并通过校验，
// 避免示例中的键名与结构体 tag 脱节（配置项静默退回默认值）。
func TestLoadExampleConfig(t *testing.T) {
	for _, key := range []string{
		EnvMilvusCollectionName, EnvTextEmbeddingModel, EnvMcpURL,
		EnvFileDir, EnvMemoryMaxWindowSize, EnvKnowledgeChunkSize,
	} {
		t.Setenv(key, "")
	}

	content, err := os.ReadFile(filepath.Join("..", "..", "manifest", "config", "config.example.yaml"))
	if err != nil {
		t.Fatalf("读取配置示例失败: %v", err)
	}
	got, err := loadForTest(t, string(content))
	if err != nil {
		t.Fatalf("加载配置示例失败: %v", err)
	}

	if got.TextEmbedding.Model != "qwen3.7-text-embedding" {
		t.Errorf("text-embedding.model = %q，期望 qwen3.7-text-embedding", got.TextEmbedding.Model)
	}
	if got.Milvus.CollectionName != "biz" || got.Milvus.Index.M != 16 {
		t.Errorf("milvus = %q/%d，期望 biz/16", got.Milvus.CollectionName, got.Milvus.Index.M)
	}
	if got.McpURL != "http://localhost:3000/sse" || got.FileDir != "./docs" {
		t.Errorf("mcp_url/file_dir = %q/%q", got.McpURL, got.FileDir)
	}
	if got.Memory.MaxWindowSize != 8 || got.KnowledgeChunk.ChunkSize != 1000 {
		t.Errorf("memory/chunk = %d/%d，期望 8/1000", got.Memory.MaxWindowSize, got.KnowledgeChunk.ChunkSize)
	}
}

// TestConfigTagCoverage 遍历 Config 的全部 json tag，用「与默认值不同的探针」逐个绑定，
// 校验每个 tag 都能落到对应字段；新增字段时若 tag 写错或漏写，此用例会失败。
func TestConfigTagCoverage(t *testing.T) {
	base := defaultConfig()
	data := map[string]any{}
	baseValues := map[string]any{}

	walkTaggedFields(reflect.ValueOf(base).Elem(), nil, func(path []string, fv reflect.Value) {
		key := strings.Join(path, ".")
		baseValues[key] = fv.Interface()

		var probe any
		switch fv.Kind() {
		case reflect.String:
			probe = fv.String() + "x"
		case reflect.Int, reflect.Int64:
			probe = fv.Int() + 1
		case reflect.Bool:
			probe = !fv.Bool()
		default:
			t.Fatalf("字段 %s 的类型 %s 未覆盖", key, fv.Kind())
		}
		setNestedValue(data, path, probe)
	})

	got := defaultConfig()
	if err := gconv.Struct(data, got); err != nil {
		t.Fatalf("绑定探针配置失败: %v", err)
	}

	walkTaggedFields(reflect.ValueOf(got).Elem(), nil, func(path []string, fv reflect.Value) {
		key := strings.Join(path, ".")
		if reflect.DeepEqual(fv.Interface(), baseValues[key]) {
			t.Errorf("tag %s 未绑定到字段，取值仍为默认值 %v", key, fv.Interface())
		}
	})
}

// walkTaggedFields 递归遍历结构体中带 json tag 的叶子字段（跳过嵌套结构体本身）。
func walkTaggedFields(v reflect.Value, prefix []string, fn func(path []string, fv reflect.Value)) {
	t := v.Type()
	for i := range t.NumField() {
		tag := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		path := append(append([]string{}, prefix...), tag)
		fv := v.Field(i)
		if fv.Kind() == reflect.Struct {
			walkTaggedFields(fv, path, fn)
			continue
		}
		fn(path, fv)
	}
}

// setNestedValue 按 tag 路径写入嵌套 map。
func setNestedValue(m map[string]any, path []string, value any) {
	for _, key := range path[:len(path)-1] {
		next, ok := m[key].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[key] = next
		}
		m = next
	}
	m[path[len(path)-1]] = value
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

	got, err := loadForTest(t, "")
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
			if _, err := loadForTest(t, ""); err == nil {
				t.Fatalf("非法 %s=%q 应使加载失败", c.key, c.value)
			}
		})
	}
}

// TestLoadSSESlowClientPolicy 校验 sse.slow_client_policy 的默认值、显式值、
// 环境变量覆盖与非法值拦截。
func TestLoadSSESlowClientPolicy(t *testing.T) {
	got, err := loadForTest(t, "")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if got.SSE.SlowClientPolicy != "drop" {
		t.Errorf("默认 slow_client_policy = %q，期望 drop", got.SSE.SlowClientPolicy)
	}

	got, err = loadForTest(t, "sse:\n  slow_client_policy: disconnect\n")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if got.SSE.SlowClientPolicy != "disconnect" {
		t.Errorf("显式 slow_client_policy = %q，期望 disconnect", got.SSE.SlowClientPolicy)
	}

	t.Setenv(EnvSSESlowClientPolicy, "drop")
	got, err = loadForTest(t, "sse:\n  slow_client_policy: disconnect\n")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if got.SSE.SlowClientPolicy != "drop" {
		t.Errorf("环境变量覆盖后 = %q，期望 drop", got.SSE.SlowClientPolicy)
	}

	t.Setenv(EnvSSESlowClientPolicy, "")
	if _, err := loadForTest(t, "sse:\n  slow_client_policy: block\n"); err == nil {
		t.Fatal("非法 slow_client_policy 应使加载失败")
	}
}
