// Package config 负责加载并缓存应用配置。
//
// 优先级：环境变量 > 配置文件 > 代码默认值。默认读取 manifest/config/config.yaml，
// 可用 CONFIG_PATH 指定其它路径。配置项与环境变量清单详见 dev-docs/configuration.md。
package config

import (
	"fmt"
	"os"
	"sync"

	"SuperBizAgent/utility/common"

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

// Config 应用总配置。
type Config struct {
	Milvus        MilvusConfig        `yaml:"milvus"`
	TextEmbedding TextEmbeddingConfig `yaml:"text-embedding"`
	Mysql         MysqlConfig         `yaml:"mysql"`
	Prometheus    PrometheusConfig    `yaml:"prometheus"`
	McpURL        string              `yaml:"mcp_url"`
}

// PrometheusConfig Prometheus 告警工具配置，对应配置文件中的 prometheus 段。
type PrometheusConfig struct {
	// BaseURL Prometheus 服务地址，例如 http://127.0.0.1:9090
	BaseURL string `yaml:"base_url"`
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
}

var (
	once    sync.Once
	cfg     *Config
	loadErr error
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
		},
		TextEmbedding: TextEmbeddingConfig{
			Model: "qwen3.7-text-embedding",
		},
		Mysql: MysqlConfig{
			// 默认直接拒绝：interrupt 需要编排层支持 checkpoint 与恢复，通路未就绪时
			// 挂起会变成无人处理的打断信号。
			DangerousStatementMode: "deny",
		},
		McpURL: "http://localhost:3000/sse",
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

	applyEnv(c)
	fillDefaults(c)

	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// Get 返回全局配置，仅在首次调用时加载（并发安全，不支持热更新）。
func Get() (*Config, error) {
	once.Do(func() {
		cfg, loadErr = Load("")
	})
	if loadErr != nil {
		return nil, loadErr
	}
	return cfg, nil
}

// applyEnv 使用环境变量覆盖配置项，空值忽略。
func applyEnv(c *Config) {
	overrides := map[string]*string{
		EnvMilvusAddress:        &c.Milvus.Address,
		EnvMilvusUsername:       &c.Milvus.Username,
		EnvMilvusPassword:       &c.Milvus.Password,
		EnvMilvusDBName:         &c.Milvus.DBName,
		EnvMilvusCollectionName: &c.Milvus.CollectionName,

		EnvTextEmbeddingAPIKey:  &c.TextEmbedding.APIKey,
		EnvTextEmbeddingBaseURL: &c.TextEmbedding.BaseURL,
		EnvTextEmbeddingModel:   &c.TextEmbedding.Model,
		EnvMcpURL:               &c.McpURL,

		EnvMysqlDangerousStatementMode: &c.Mysql.DangerousStatementMode,
		EnvPrometheusBaseURL:           &c.Prometheus.BaseURL,
	}
	for key, target := range overrides {
		if v := os.Getenv(key); v != "" {
			*target = v
		}
	}
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
	switch c.Mysql.DangerousStatementMode {
	case "deny", "interrupt":
	default:
		return fmt.Errorf("配置项 mysql.dangerous_statement_mode 只能是 deny 或 interrupt，当前为 %q", c.Mysql.DangerousStatementMode)
	}
	return nil
}
