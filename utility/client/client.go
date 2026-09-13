// Package client 负责创建并初始化业务数据库的 Milvus 客户端。
//
// 本包只保证数据库（db_name）就绪并返回业务库客户端；集合 schema、向量索引与
// 加载由 internal/ai 下的 eino indexer 首次写入时创建，避免两处重复定义集合结构。
// 初始化流程与配置项详见 dev-docs/milvus.md。
package client

import (
	"context"
	"fmt"

	"SuperBizAgent/internal/config"

	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

// NewMilvusClient 创建 Milvus 客户端，并确保业务数据库已就绪。
// 连接参数从 manifest/config/config.yaml 读取（支持环境变量覆盖）。
func NewMilvusClient(ctx context.Context) (*milvusclient.Client, error) {
	cfg, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("加载 Milvus 配置失败: %w", err)
	}
	milvusCfg := cfg.Milvus

	// 1. 先连接 default 数据库，用于初始化业务数据库
	defaultClient, err := milvusclient.New(ctx, &milvusclient.ClientConfig{
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
		_ = defaultClient.Close(ctx) // 关闭失败不应覆盖主错误
		return nil, err
	}

	// 3. 关闭默认数据库连接，避免连接泄漏
	if err := defaultClient.Close(ctx); err != nil {
		return nil, fmt.Errorf("关闭 Milvus 默认数据库连接失败: %w", err)
	}

	// 4. 创建连接到业务数据库的客户端
	bizClient, err := milvusclient.New(ctx, &milvusclient.ClientConfig{
		Address:  milvusCfg.Address,
		DBName:   milvusCfg.DBName,
		Username: milvusCfg.Username,
		Password: milvusCfg.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("连接 Milvus 业务数据库 %s 失败: %w", milvusCfg.DBName, err)
	}

	return bizClient, nil
}

// ensureDatabase 确保指定数据库存在，不存在则创建。
func ensureDatabase(ctx context.Context, c *milvusclient.Client, dbName string) error {
	databases, err := c.ListDatabase(ctx, milvusclient.NewListDatabaseOption())
	if err != nil {
		return fmt.Errorf("获取 Milvus 数据库列表失败: %w", err)
	}

	for _, db := range databases {
		if db == dbName {
			return nil
		}
	}

	if err := c.CreateDatabase(ctx, milvusclient.NewCreateDatabaseOption(dbName)); err != nil {
		return fmt.Errorf("创建 Milvus 数据库 %s 失败: %w", dbName, err)
	}
	return nil
}
