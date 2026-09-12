package client

import (
	"SuperBizAgent/utility/config"
	"context"
	"fmt"

	cli "github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

// biz 集合的字段名
const (
	milvusIDFieldName       = "id"       // 主键
	milvusVectorFieldName   = "vector"   // 向量字段
	milvusContentFieldName  = "content"  // 文本分片内容
	milvusMetadataFieldName = "metadata" // 分片元数据（来源文件、页码等）

	// varchar 字段最大长度
	milvusContentMaxLen = 8192
	// 集合分片数
	milvusShardNum = 2
	// HNSW 索引参数
	milvusHNSWM              = 16
	milvusHNSWEfConstruction = 200
)

// NewMilvusClient 创建 Milvus 客户端。
//
// 初始化流程：
//  1. 连接默认数据库，检查业务数据库是否存在，不存在则创建；
//  2. 连接业务数据库；
//  3. 检查业务集合是否存在，不存在则创建并建立向量索引、加载到内存。
//
// 连接参数从 manifest/config/config.yaml 读取（支持环境变量覆盖），不再硬编码。
func NewMilvusClient(ctx context.Context) (cli.Client, error) {
	cfg, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("加载 Milvus 配置失败: %w", err)
	}
	milvusCfg := cfg.Milvus

	// 1. 先连接 default 数据库，用于初始化业务数据库
	defaultClient, err := cli.NewClient(ctx, cli.Config{
		Address:  milvusCfg.Address,
		DBName:   milvusCfg.DefaultDBName,
		Username: milvusCfg.Username,
		Password: milvusCfg.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("连接 Milvus 默认数据库 %s 失败: %w", milvusCfg.DefaultDBName, err)
	}

	// 2. 检查业务数据库是否存在，不存在则创建
	if err := ensureDatabase(ctx, defaultClient, milvusCfg.DBName); err != nil {
		// 关闭失败不应覆盖主错误，显式忽略
		_ = defaultClient.Close()
		return nil, err
	}

	// 关闭默认数据库连接，避免连接泄漏
	if err := defaultClient.Close(); err != nil {
		return nil, fmt.Errorf("关闭 Milvus 默认数据库连接失败: %w", err)
	}

	// 3. 创建连接到业务数据库的客户端
	bizClient, err := cli.NewClient(ctx, cli.Config{
		Address:  milvusCfg.Address,
		DBName:   milvusCfg.DBName,
		Username: milvusCfg.Username,
		Password: milvusCfg.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("连接 Milvus 业务数据库 %s 失败: %w", milvusCfg.DBName, err)
	}

	// 4. 检查业务集合是否存在，不存在则创建
	if err := ensureCollection(ctx, bizClient, milvusCfg.CollectionName, milvusCfg.VectorDim); err != nil {
		// 关闭失败不应覆盖主错误，显式忽略
		_ = bizClient.Close()
		return nil, err
	}

	return bizClient, nil
}

// ensureDatabase 确保指定数据库存在，不存在则创建。
func ensureDatabase(ctx context.Context, c cli.Client, dbName string) error {
	databases, err := c.ListDatabases(ctx)
	if err != nil {
		return fmt.Errorf("获取 Milvus 数据库列表失败: %w", err)
	}

	for _, db := range databases {
		if db.Name == dbName {
			return nil
		}
	}

	if err := c.CreateDatabase(ctx, dbName); err != nil {
		return fmt.Errorf("创建 Milvus 数据库 %s 失败: %w", dbName, err)
	}
	return nil
}

// ensureCollection 确保集合存在；不存在则创建集合并建立向量索引、加载到内存。
func ensureCollection(ctx context.Context, c cli.Client, collName string, dim int64) error {
	exists, err := c.HasCollection(ctx, collName)
	if err != nil {
		return fmt.Errorf("检查 Milvus 集合 %s 是否存在失败: %w", collName, err)
	}
	if exists {
		return nil
	}

	if err := c.CreateCollection(ctx, bizSchema(collName, dim), milvusShardNum); err != nil {
		return fmt.Errorf("创建 Milvus 集合 %s 失败: %w", collName, err)
	}

	index, err := entity.NewIndexHNSW(entity.COSINE, milvusHNSWM, milvusHNSWEfConstruction)
	if err != nil {
		return fmt.Errorf("构建 Milvus 集合 %s 的 HNSW 索引参数失败: %w", collName, err)
	}
	if err := c.CreateIndex(ctx, collName, milvusVectorFieldName, index, false); err != nil {
		return fmt.Errorf("为 Milvus 集合 %s 创建向量索引失败: %w", collName, err)
	}

	if err := c.LoadCollection(ctx, collName, false); err != nil {
		return fmt.Errorf("加载 Milvus 集合 %s 到内存失败: %w", collName, err)
	}
	return nil
}

// bizSchema 返回业务集合的结构定义。
func bizSchema(collName string, dim int64) *entity.Schema {
	return entity.NewSchema().
		WithName(collName).
		WithDescription("业务知识库集合，存储文本分片及其向量").
		WithAutoID(true).
		WithField(entity.NewField().
			WithName(milvusIDFieldName).
			WithDataType(entity.FieldTypeInt64).
			WithIsPrimaryKey(true).
			WithIsAutoID(true)).
		WithField(entity.NewField().
			WithName(milvusVectorFieldName).
			WithDataType(entity.FieldTypeFloatVector).
			WithDim(dim)).
		WithField(entity.NewField().
			WithName(milvusContentFieldName).
			WithDataType(entity.FieldTypeVarChar).
			WithMaxLength(milvusContentMaxLen)).
		WithField(entity.NewField().
			WithName(milvusMetadataFieldName).
			WithDataType(entity.FieldTypeJSON))
}
