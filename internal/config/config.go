// Package config 负责加载并缓存应用配置。
//
// 优先级：环境变量 > 配置文件 > 代码默认值。默认读取 manifest/config/config.yaml，
// 可用 CONFIG_PATH 指定其它路径；配置文件由 GoFrame 的 gcfg 读取，文件变更后经
// gfsnotify 触发热更新。配置项与环境变量清单详见 dev-docs/configuration.md。
package config

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"SuperBizAgent/utility/common"
	"SuperBizAgent/utility/mem"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/util/gconv"
)

// DefaultConfigPath 默认配置文件路径（相对于程序运行目录）。
const DefaultConfigPath = "manifest/config/config.yaml"

// Milvus 配置相关的环境变量名。
const (
	EnvMilvusAddress        = "MILVUS_ADDRESS"
	EnvMilvusUsername       = "MILVUS_USERNAME"
	EnvMilvusPassword       = "MILVUS_PASSWORD"
	EnvMilvusDBName         = "MILVUS_DB_NAME"
	EnvMilvusCollectionName = "MILVUS_COLLECTION_NAME"
)

// 文本模型配置相关的环境变量名。
const (
	EnvTextEmbeddingAPIKey  = "TEXT_EMBEDDING_API_KEY"
	EnvTextEmbeddingBaseURL = "TEXT_EMBEDDING_BASE_URL"
	EnvTextEmbeddingModel   = "TEXT_EMBEDDING_MODEL"
)

// MCP 服务相关的环境变量名。
const (
	EnvMcpURL = "MCP_URL"
)

// Prometheus 告警工具相关的环境变量名。
const (
	EnvPrometheusBaseURL = "PROMETHEUS_BASE_URL"
)

// MySQL 工具相关的环境变量名。
const (
	EnvMysqlDangerousStatementMode = "MYSQL_DANGEROUS_STATEMENT_MODE"
)

// 知识库文档目录相关的环境变量名。
const (
	EnvFileDir = "FILE_DIR"
)

// Milvus 默认库、向量维度与索引相关的环境变量名。
const (
	EnvMilvusDefaultDBName            = "MILVUS_DEFAULT_DB_NAME"
	EnvMilvusVectorDim                = "MILVUS_VECTOR_DIM"
	EnvMilvusIndexM                   = "MILVUS_INDEX_M"
	EnvMilvusIndexEfConstruction      = "MILVUS_INDEX_EF_CONSTRUCTION"
	EnvMilvusRecreateOnSchemaMismatch = "MILVUS_RECREATE_ON_SCHEMA_MISMATCH"
)

// 知识库切分相关的环境变量名。
const (
	EnvKnowledgeChunkSize        = "KNOWLEDGE_CHUNK_SIZE"
	EnvKnowledgeChunkOverlapSize = "KNOWLEDGE_CHUNK_OVERLAP_SIZE"
)

// 会话记忆相关的环境变量名。
const (
	EnvMemorySessionTTL      = "MEMORY_SESSION_TTL"
	EnvMemoryCleanupInterval = "MEMORY_CLEANUP_INTERVAL"
	EnvMemoryMaxWindowSize   = "MEMORY_MAX_WINDOW_SIZE"
)

// SSE 推送相关的环境变量名。
const (
	EnvSSESlowClientPolicy = "SSE_SLOW_CLIENT_POLICY"
)

// Config 应用总配置。
type Config struct {
	Milvus         MilvusConfig         `json:"milvus"`
	TextEmbedding  TextEmbeddingConfig  `json:"text-embedding"`
	Mysql          MysqlConfig          `json:"mysql"`
	Prometheus     PrometheusConfig     `json:"prometheus"`
	KnowledgeChunk KnowledgeChunkConfig `json:"knowledge_chunk"`
	Memory         MemoryConfig         `json:"memory"`
	SSE            SSEConfig            `json:"sse"`
	McpURL         string               `json:"mcp_url"`
	FileDir        string               `json:"file_dir"`
}

// MemoryConfig 会话记忆配置，对应配置文件中的 memory 段。
type MemoryConfig struct {
	// SessionTTL 会话空闲多久后被后台回收
	SessionTTL time.Duration `json:"session_ttl"`
	// CleanupInterval 后台清理会话的周期
	CleanupInterval time.Duration `json:"cleanup_interval"`
	// MaxWindowSize 会话记忆窗口大小（消息条数），约等于对话轮数 × 2
	MaxWindowSize int `json:"max_window_size"`
}

// KnowledgeChunkConfig 知识库文档切分参数，对应配置文件中的 knowledge_chunk 段。
type KnowledgeChunkConfig struct {
	// ChunkSize 每个分片的最大字符数（按 rune 计），必须大于 0
	ChunkSize int `json:"chunk_size"`
	// OverlapSize 相邻分片的重叠字符数，需大于等于 0 且小于 ChunkSize
	OverlapSize int `json:"overlap_size"`
}

// PrometheusConfig Prometheus 告警工具配置，对应配置文件中的 prometheus 段。
type PrometheusConfig struct {
	// BaseURL Prometheus 服务地址，例如 http://127.0.0.1:9090
	BaseURL string `json:"base_url"`
}

// SSEConfig SSE 推送相关配置，对应配置文件中的 sse 段。
type SSEConfig struct {
	// SlowClientPolicy 慢客户端（缓冲区满）处置策略：
	// drop 丢弃消息（默认）；disconnect 断开该客户端连接。
	SlowClientPolicy string `json:"slow_client_policy"`
}

// MysqlConfig MySQL 工具配置，对应配置文件中的 mysql 段。
type MysqlConfig struct {
	// DangerousStatementMode 决定 mysql_exec 遇到 DROP/TRUNCATE 时的行为：
	// deny 直接拒绝，interrupt 挂起等用户确认。取值需与 internal/ai/tools/mysql 的
	// ModeDangerousDeny / ModeDangerousInterrupt 保持一致。
	DangerousStatementMode string `json:"dangerous_statement_mode"`
}

// TextEmbeddingConfig 文本向量模型配置，对应配置文件中的 text-embedding 段。
type TextEmbeddingConfig struct {
	// APIKey 模型服务的 API Key，建议通过环境变量注入
	APIKey string `json:"api_key"`
	// BaseURL 模型服务地址
	BaseURL string `json:"base_url"`
	// Model 模型名称
	Model string `json:"model"`
}

// MilvusConfig Milvus 客户端配置。
type MilvusConfig struct {
	// Address 服务地址，格式 host:port
	Address string `json:"address"`
	// Username 认证用户名
	Username string `json:"username"`
	// Password 认证密码，建议通过环境变量注入
	Password string `json:"password"`
	// DefaultDBName 初始化数据库时使用的默认库
	DefaultDBName string `json:"default_db_name"`
	// DBName 业务数据库名称
	DBName string `json:"db_name"`
	// CollectionName 业务集合名称
	CollectionName string `json:"collection_name"`
	// VectorDim 向量维度
	VectorDim int64 `json:"vector_dim"`
	// Index 向量索引参数，对应配置文件中的 milvus.index 段
	Index MilvusIndexConfig `json:"index"`
	// RecreateOnSchemaMismatch 集合已存在但 schema 与代码期望不一致时，是否删除重建。
	// 默认 false（直接报错）；开启会丢失集合内全部数据。
	RecreateOnSchemaMismatch bool `json:"recreate_on_schema_mismatch"`
}

// MilvusIndexConfig 向量索引参数，对应配置文件中的 milvus.index 段。
// 当前仅支持 HNSW（由 eino milvus2 indexer 创建）。
type MilvusIndexConfig struct {
	// M HNSW 每个节点的最大连接数，取值范围 4-64
	M int `json:"m"`
	// EfConstruction HNSW 建索引时的搜索宽度，取值范围 8-512
	EfConstruction int `json:"ef_construction"`
}

var (
	initOnce sync.Once
	initErr  error
	cfgPtr   atomic.Pointer[Config]
)

// defaultConfig 返回内置默认配置，避免配置文件缺失时无法启动。
func defaultConfig() *Config {
	return &Config{
		Milvus: MilvusConfig{
			Address:        "127.0.0.1:19530",
			Username:       "root",
			Password:       "",
			DefaultDBName:  "default",
			DBName:         common.MilvusDBName,
			CollectionName: common.MilvusCollectionName,
			VectorDim:      1024,
			Index: MilvusIndexConfig{
				M:              16,
				EfConstruction: 200,
			},
		},
		TextEmbedding: TextEmbeddingConfig{
			Model: "qwen3.7-text-embedding",
		},
		Mysql: MysqlConfig{
			// 默认直接拒绝：interrupt 需要编排层支持 checkpoint 与恢复，通路未就绪时
			// 挂起会变成无人处理的打断信号。
			DangerousStatementMode: "deny",
		},
		McpURL:  "http://localhost:3000/sse",
		FileDir: common.FileDir,
		KnowledgeChunk: KnowledgeChunkConfig{
			ChunkSize:   1000,
			OverlapSize: 200,
		},
		Memory: MemoryConfig{
			SessionTTL:      mem.DefaultSessionTTL,
			CleanupInterval: mem.DefaultCleanupInterval,
			MaxWindowSize:   mem.DefaultMaxWindowSize,
		},
		SSE: SSEConfig{
			SlowClientPolicy: "drop",
		},
		Prometheus: PrometheusConfig{
			BaseURL: "http://127.0.0.1:9090",
		},
	}
}

// EnvConfigPath 是指定配置文件路径的环境变量名。
const EnvConfigPath = "CONFIG_PATH"

// Path 返回当前生效的配置文件路径：CONFIG_PATH 优先，否则取 DefaultConfigPath。
func Path() string {
	if p := os.Getenv(EnvConfigPath); p != "" {
		return p
	}
	return DefaultConfigPath
}

// loader 返回本次加载使用的 gcfg 实例。
//
// 默认复用 gf 全局实例，与 logger、server、dashscope 等读同一份配置文件；
// 设置 CONFIG_PATH 时改用独立实例，避免影响 gf 自身的配置读取。
func loader() (*gcfg.Config, error) {
	if os.Getenv(EnvConfigPath) == "" {
		return g.Cfg(), nil
	}
	path := Path()
	adapter, err := gcfg.NewAdapterFile(path)
	if err != nil {
		return nil, fmt.Errorf("初始化配置适配器 %s 失败: %w", path, err)
	}
	return gcfg.NewWithAdapter(adapter), nil
}

// cfgPath 返回 cfgs 实际读取的配置文件路径，仅用于错误提示。
func cfgPath(cfgs *gcfg.Config) string {
	if adapter, ok := cfgs.GetAdapter().(*gcfg.AdapterFile); ok {
		if p, err := adapter.GetFilePath(); err == nil && p != "" {
			return p
		}
	}
	return Path()
}

// bind 读取配置文件并转换为 Config，顺序为：
// 代码默认值 → 配置文件 → 环境变量 → 补默认值 → 校验。
// 配置文件不存在时不报错，返回默认配置。
func bind(ctx context.Context, cfgs *gcfg.Config) (*Config, error) {
	data, err := cfgs.Data(ctx)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件 %s 失败: %w", cfgPath(cfgs), err)
	}

	c := defaultConfig()
	if len(data) > 0 {
		if err := gconv.Struct(data, c); err != nil {
			return nil, fmt.Errorf("解析配置文件 %s 失败: %w", cfgPath(cfgs), err)
		}
	}

	if err := applyEnv(c); err != nil {
		return nil, err
	}
	fillDefaults(c)

	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// Get 返回全局配置，仅在首次调用时加载（并发安全）；
// 同时注册配置文件监听，文件变更后自动热更新（见 Watch）。
func Get() (*Config, error) {
	initOnce.Do(func() {
		cfgs, err := loader()
		if err != nil {
			initErr = err
			return
		}
		c, err := bind(context.Background(), cfgs)
		if err != nil {
			initErr = err
			return
		}
		// 先注册监听再写入缓存，避免极端情况下丢掉首个变更事件。
		if err := watch(cfgs, cfgPtr.Load, cfgPtr.Store); err != nil {
			initErr = err
			return
		}
		cfgPtr.Store(c)
	})
	if initErr != nil {
		return nil, initErr
	}
	return cfgPtr.Load(), nil
}

// watcherName 是本包注册到 gcfg 文件适配器上的监听器名。
const watcherName = "SuperBizAgent"

var (
	watchMu    sync.RWMutex
	watchFuncs []func(*Config)
)

// Watch 注册配置热更新回调，可注册多个；回调触发时新配置已写入缓存，Get 可见。
// 回调由 gcfg 在独立 goroutine 中执行，与其他重载互斥、不会重入，
// 但仍可能与其他请求并发，需自行保证并发安全，且应尽快返回。
func Watch(fn func(*Config)) {
	if fn == nil {
		return
	}
	watchMu.Lock()
	watchFuncs = append(watchFuncs, fn)
	watchMu.Unlock()
}

// watch 在 cfgs 的文件适配器上注册监听：文件变更后重新加载，与 last() 取到的当前
// 配置不同才存入 store 并通知已注册回调，重载失败只告警并沿用旧配置。
//
// 适配器对一次保存可能投递多个事件，因此重载全程串行：内容未变的事件会被去重，
// 同一个失败原因也只告警一次，避免日志刷屏。
func watch(cfgs *gcfg.Config, last func() *Config, store func(*Config)) error {
	adapter, ok := cfgs.GetAdapter().(*gcfg.AdapterFile)
	if !ok {
		return fmt.Errorf("配置适配器 %T 不支持文件监听，无法热更新", cfgs.GetAdapter())
	}
	var (
		mu      sync.Mutex
		lastErr string
	)
	adapter.AddWatcher(watcherName, func(ctx context.Context) {
		mu.Lock()
		defer mu.Unlock()

		c, err := bind(ctx, cfgs)
		if err != nil {
			if msg := err.Error(); msg != lastErr {
				g.Log().Warningf(ctx, "[config] 热更新失败，继续使用旧配置: %v", err)
				lastErr = msg
			}
			return
		}
		lastErr = ""
		if cur := last(); cur != nil && reflect.DeepEqual(cur, c) {
			return
		}
		store(c)
		g.Log().Infof(ctx, "[config] 配置已热更新")
		watchMu.RLock()
		funcs := make([]func(*Config), len(watchFuncs))
		copy(funcs, watchFuncs)
		watchMu.RUnlock()
		for _, fn := range funcs {
			fn(c)
		}
	})
	return nil
}

// applyEnv 使用环境变量覆盖配置项，空值忽略；非空但非法的值返回错误。
func applyEnv(c *Config) error {
	overrides := map[string]*string{
		EnvMilvusAddress:        &c.Milvus.Address,
		EnvMilvusUsername:       &c.Milvus.Username,
		EnvMilvusPassword:       &c.Milvus.Password,
		EnvMilvusDefaultDBName:  &c.Milvus.DefaultDBName,
		EnvMilvusDBName:         &c.Milvus.DBName,
		EnvMilvusCollectionName: &c.Milvus.CollectionName,

		EnvTextEmbeddingAPIKey:  &c.TextEmbedding.APIKey,
		EnvTextEmbeddingBaseURL: &c.TextEmbedding.BaseURL,
		EnvTextEmbeddingModel:   &c.TextEmbedding.Model,
		EnvMcpURL:               &c.McpURL,

		EnvMysqlDangerousStatementMode: &c.Mysql.DangerousStatementMode,
		EnvPrometheusBaseURL:           &c.Prometheus.BaseURL,
		EnvSSESlowClientPolicy:         &c.SSE.SlowClientPolicy,
		EnvFileDir:                     &c.FileDir,
	}
	for key, target := range overrides {
		if v := os.Getenv(key); v != "" {
			*target = v
		}
	}

	int64Targets := map[string]*int64{
		EnvMilvusVectorDim: &c.Milvus.VectorDim,
	}
	for key, target := range int64Targets {
		if err := setInt64Env(key, target); err != nil {
			return err
		}
	}

	intTargets := map[string]*int{
		EnvMilvusIndexM:              &c.Milvus.Index.M,
		EnvMilvusIndexEfConstruction: &c.Milvus.Index.EfConstruction,
		EnvKnowledgeChunkSize:        &c.KnowledgeChunk.ChunkSize,
		EnvKnowledgeChunkOverlapSize: &c.KnowledgeChunk.OverlapSize,
		EnvMemoryMaxWindowSize:       &c.Memory.MaxWindowSize,
	}
	for key, target := range intTargets {
		if err := setIntEnv(key, target); err != nil {
			return err
		}
	}

	durationTargets := map[string]*time.Duration{
		EnvMemorySessionTTL:      &c.Memory.SessionTTL,
		EnvMemoryCleanupInterval: &c.Memory.CleanupInterval,
	}
	for key, target := range durationTargets {
		if err := setDurationEnv(key, target); err != nil {
			return err
		}
	}

	boolTargets := map[string]*bool{
		EnvMilvusRecreateOnSchemaMismatch: &c.Milvus.RecreateOnSchemaMismatch,
	}
	for key, target := range boolTargets {
		if err := setBoolEnv(key, target); err != nil {
			return err
		}
	}

	return nil
}

// setIntEnv 用环境变量覆盖 int 配置项；空值忽略，非法值报错。
func setIntEnv(key string, target *int) error {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("环境变量 %s 不是合法整数: %w", key, err)
	}
	*target = n
	return nil
}

// setInt64Env 用环境变量覆盖 int64 配置项；空值忽略，非法值报错。
func setInt64Env(key string, target *int64) error {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fmt.Errorf("环境变量 %s 不是合法整数: %w", key, err)
	}
	*target = n
	return nil
}

// setDurationEnv 用环境变量覆盖时长配置项；空值忽略，非法值报错。
func setDurationEnv(key string, target *time.Duration) error {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fmt.Errorf("环境变量 %s 不是合法时长: %w", key, err)
	}
	*target = d
	return nil
}

// setBoolEnv 用环境变量覆盖 bool 配置项；空值忽略，非法值报错。
func setBoolEnv(key string, target *bool) error {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fmt.Errorf("环境变量 %s 不是合法布尔值: %w", key, err)
	}
	*target = b
	return nil
}

// fillDefaults 为未配置的字段补齐默认值。
func fillDefaults(c *Config) {
	def := defaultConfig()
	if c.Milvus.DefaultDBName == "" {
		c.Milvus.DefaultDBName = def.Milvus.DefaultDBName
	}
	if c.Milvus.DBName == "" {
		c.Milvus.DBName = def.Milvus.DBName
	}
	if c.Milvus.CollectionName == "" {
		c.Milvus.CollectionName = def.Milvus.CollectionName
	}
	if c.Milvus.VectorDim <= 0 {
		c.Milvus.VectorDim = def.Milvus.VectorDim
	}
	if c.Milvus.Index.M <= 0 {
		c.Milvus.Index.M = def.Milvus.Index.M
	}
	if c.Milvus.Index.EfConstruction <= 0 {
		c.Milvus.Index.EfConstruction = def.Milvus.Index.EfConstruction
	}
	if c.TextEmbedding.Model == "" {
		c.TextEmbedding.Model = def.TextEmbedding.Model
	}
	if c.Mysql.DangerousStatementMode == "" {
		c.Mysql.DangerousStatementMode = def.Mysql.DangerousStatementMode
	}
	if c.McpURL == "" {
		c.McpURL = def.McpURL
	}
	if c.Prometheus.BaseURL == "" {
		c.Prometheus.BaseURL = def.Prometheus.BaseURL
	}
	if c.FileDir == "" {
		c.FileDir = def.FileDir
	}
	if c.KnowledgeChunk.ChunkSize <= 0 {
		c.KnowledgeChunk.ChunkSize = def.KnowledgeChunk.ChunkSize
	}
	if c.KnowledgeChunk.OverlapSize < 0 {
		c.KnowledgeChunk.OverlapSize = def.KnowledgeChunk.OverlapSize
	}
	if c.Memory.SessionTTL <= 0 {
		c.Memory.SessionTTL = def.Memory.SessionTTL
	}
	if c.Memory.CleanupInterval <= 0 {
		c.Memory.CleanupInterval = def.Memory.CleanupInterval
	}
	if c.Memory.MaxWindowSize <= 0 {
		c.Memory.MaxWindowSize = def.Memory.MaxWindowSize
	}
	if c.SSE.SlowClientPolicy == "" {
		c.SSE.SlowClientPolicy = def.SSE.SlowClientPolicy
	}
}

// validate 校验必填项。
// 文本模型配置不在此校验：缺省时仍可启动，由真正使用模型的组件惰性报错。
func (c *Config) validate() error {
	if c.Milvus.Address == "" {
		return fmt.Errorf("配置项 milvus.address 不能为空")
	}
	if c.Milvus.VectorDim <= 0 {
		return fmt.Errorf("配置项 milvus.vector_dim 必须大于 0，当前为 %d", c.Milvus.VectorDim)
	}
	if c.Milvus.Index.M < 4 || c.Milvus.Index.M > 64 {
		return fmt.Errorf("配置项 milvus.index.m 取值范围为 4-64，当前为 %d", c.Milvus.Index.M)
	}
	if c.Milvus.Index.EfConstruction < 8 || c.Milvus.Index.EfConstruction > 512 {
		return fmt.Errorf("配置项 milvus.index.ef_construction 取值范围为 8-512，当前为 %d", c.Milvus.Index.EfConstruction)
	}
	switch c.Mysql.DangerousStatementMode {
	case "deny", "interrupt":
	default:
		return fmt.Errorf("配置项 mysql.dangerous_statement_mode 只能是 deny 或 interrupt，当前为 %q", c.Mysql.DangerousStatementMode)
	}
	switch c.SSE.SlowClientPolicy {
	case "drop", "disconnect":
	default:
		return fmt.Errorf("配置项 sse.slow_client_policy 只能是 drop 或 disconnect，当前为 %q", c.SSE.SlowClientPolicy)
	}
	if c.KnowledgeChunk.ChunkSize <= 0 {
		return fmt.Errorf("配置项 knowledge_chunk.chunk_size 必须大于 0，当前为 %d", c.KnowledgeChunk.ChunkSize)
	}
	if c.KnowledgeChunk.OverlapSize < 0 || c.KnowledgeChunk.OverlapSize >= c.KnowledgeChunk.ChunkSize {
		return fmt.Errorf("配置项 knowledge_chunk.overlap_size 必须大于等于 0 且小于 chunk_size，当前为 %d", c.KnowledgeChunk.OverlapSize)
	}
	return nil
}
