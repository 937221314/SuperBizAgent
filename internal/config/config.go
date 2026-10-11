// Package config 负责加载并缓存应用配置。
//
// 优先级：环境变量 > 配置文件 > 代码默认值。默认读取 manifest/config/config.yaml，
// 可用 CONFIG_PATH 指定其它路径。配置项与环境变量清单详见 dev-docs/configuration.md。
package config

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"SuperBizAgent/utility/common"
	"SuperBizAgent/utility/mem"

	"github.com/gogf/gf/v2/frame/g"
	"gopkg.in/yaml.v3"
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
	Milvus         MilvusConfig         `yaml:"milvus"`
	TextEmbedding  TextEmbeddingConfig  `yaml:"text-embedding"`
	Mysql          MysqlConfig          `yaml:"mysql"`
	Prometheus     PrometheusConfig     `yaml:"prometheus"`
	KnowledgeChunk KnowledgeChunkConfig `yaml:"knowledge_chunk"`
	Memory         MemoryConfig         `yaml:"memory"`
	SSE            SSEConfig            `yaml:"sse"`
	McpURL         string               `yaml:"mcp_url"`
	FileDir        string               `yaml:"file_dir"`
}

// MemoryConfig 会话记忆配置，对应配置文件中的 memory 段。
type MemoryConfig struct {
	// SessionTTL 会话空闲多久后被后台回收
	SessionTTL time.Duration `yaml:"session_ttl"`
	// CleanupInterval 后台清理会话的周期
	CleanupInterval time.Duration `yaml:"cleanup_interval"`
	// MaxWindowSize 会话记忆窗口大小（消息条数），约等于对话轮数 × 2
	MaxWindowSize int `yaml:"max_window_size"`
}

// KnowledgeChunkConfig 知识库文档切分参数，对应配置文件中的 knowledge_chunk 段。
type KnowledgeChunkConfig struct {
	// ChunkSize 每个分片的最大字符数（按 rune 计），必须大于 0
	ChunkSize int `yaml:"chunk_size"`
	// OverlapSize 相邻分片的重叠字符数，需大于等于 0 且小于 ChunkSize
	OverlapSize int `yaml:"overlap_size"`
}

// PrometheusConfig Prometheus 告警工具配置，对应配置文件中的 prometheus 段。
type PrometheusConfig struct {
	// BaseURL Prometheus 服务地址，例如 http://127.0.0.1:9090
	BaseURL string `yaml:"base_url"`
}

// SSEConfig SSE 推送相关配置，对应配置文件中的 sse 段。
type SSEConfig struct {
	// SlowClientPolicy 慢客户端（缓冲区满）处置策略：
	// drop 丢弃消息（默认）；disconnect 断开该客户端连接。
	SlowClientPolicy string `yaml:"slow_client_policy"`
}

// MysqlConfig MySQL 工具配置，对应配置文件中的 mysql 段。
type MysqlConfig struct {
	// DangerousStatementMode 决定 mysql_exec 遇到 DROP/TRUNCATE 时的行为：
	// deny 直接拒绝，interrupt 挂起等用户确认。取值需与 internal/ai/tools/mysql 的
	// ModeDangerousDeny / ModeDangerousInterrupt 保持一致。
	DangerousStatementMode string `yaml:"dangerous_statement_mode"`
}

// TextEmbeddingConfig 文本向量模型配置，对应配置文件中的 text-embedding 段。
type TextEmbeddingConfig struct {
	// APIKey 模型服务的 API Key，建议通过环境变量注入
	APIKey string `yaml:"api_key"`
	// BaseURL 模型服务地址
	BaseURL string `yaml:"base_url"`
	// Model 模型名称
	Model string `yaml:"model"`
}

// MilvusConfig Milvus 客户端配置。
type MilvusConfig struct {
	// Address 服务地址，格式 host:port
	Address string `yaml:"address"`
	// Username 认证用户名
	Username string `yaml:"username"`
	// Password 认证密码，建议通过环境变量注入
	Password string `yaml:"password"`
	// DefaultDBName 初始化数据库时使用的默认库
	DefaultDBName string `yaml:"default_db_name"`
	// DBName 业务数据库名称
	DBName string `yaml:"db_name"`
	// CollectionName 业务集合名称
	CollectionName string `yaml:"collection_name"`
	// VectorDim 向量维度
	VectorDim int64 `yaml:"vector_dim"`
	// Index 向量索引参数，对应配置文件中的 milvus.index 段
	Index MilvusIndexConfig `yaml:"index"`
	// RecreateOnSchemaMismatch 集合已存在但 schema 与代码期望不一致时，是否删除重建。
	// 默认 false（直接报错）；开启会丢失集合内全部数据。
	RecreateOnSchemaMismatch bool `yaml:"recreate_on_schema_mismatch"`
}

// MilvusIndexConfig 向量索引参数，对应配置文件中的 milvus.index 段。
// 当前仅支持 HNSW（由 eino milvus2 indexer 创建）。
type MilvusIndexConfig struct {
	// M HNSW 每个节点的最大连接数，取值范围 4-64
	M int `yaml:"m"`
	// EfConstruction HNSW 建索引时的搜索宽度，取值范围 8-512
	EfConstruction int `yaml:"ef_construction"`
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

// Path 返回当前生效的配置文件路径。
func Path() string {
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		return p
	}
	return DefaultConfigPath
}

// Load 从指定路径加载配置；path 为空时使用 Path()。
// 配置文件不存在时不报错，返回默认配置。
func Load(path string) (*Config, error) {
	if path == "" {
		path = Path()
	}

	c := defaultConfig()

	content, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := yaml.Unmarshal(content, c); err != nil {
			return nil, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
		}
	case !os.IsNotExist(err):
		return nil, fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
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

// Get 返回全局配置，仅在首次调用时加载（并发安全）。
// 配置在进程内缓存；运行期热更新见 Reload 与 StartAutoReload。
func Get() (*Config, error) {
	initOnce.Do(func() {
		c, err := Load("")
		if err != nil {
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

// Reload 重新读取配置文件并原子替换缓存，供热更新使用。
// 失败时保留原有配置并返回错误，调用方据此告警即可，无需回退。
func Reload() (*Config, error) {
	c, err := Load("")
	if err != nil {
		return nil, err
	}
	cfgPtr.Store(c)
	return c, nil
}

// EnvConfigReloadInterval 是定时热更新周期的环境变量名（Go 时长格式，如 30s）。
const EnvConfigReloadInterval = "CONFIG_RELOAD_INTERVAL"

// ReloadInterval 读取并解析定时热更新周期；未设置或非正时长时返回 0（关闭）。
func ReloadInterval() (time.Duration, error) {
	v := os.Getenv(EnvConfigReloadInterval)
	if v == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("环境变量 %s 不是合法时长: %w", EnvConfigReloadInterval, err)
	}
	if d < 0 {
		return 0, nil
	}
	return d, nil
}

// StartAutoReload 按 interval 周期调用 Reload 热更新配置，ctx 取消时退出；
// interval <= 0 时不启动。onReload 在每次成功重载后调用（可为 nil），
// 供调用方重新应用启动期一次性设置（如会话窗口大小）；
// 固定的后台周期（如会话 GC 间隔）无法在运行中变更，需重启。
// 返回的 channel 在后台 goroutine 退出后关闭，供测试或优雅停机等待。
func StartAutoReload(ctx context.Context, interval time.Duration, onReload func(*Config)) <-chan struct{} {
	done := make(chan struct{})
	if interval <= 0 {
		close(done)
		return done
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c, err := Reload()
				if err != nil {
					g.Log().Warningf(ctx, "[config] 热更新失败，继续使用旧配置: %v", err)
					continue
				}
				g.Log().Infof(ctx, "[config] 配置已热更新")
				if onReload != nil {
					onReload(c)
				}
			}
		}
	}()
	return done
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
