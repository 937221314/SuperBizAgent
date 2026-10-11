package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"SuperBizAgent/internal/ai/agent/knowledge_index_pipeline"
	"SuperBizAgent/internal/ai/loader"
	"SuperBizAgent/internal/config"
	"SuperBizAgent/utility/client"
	"SuperBizAgent/utility/logcallback"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/compose"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/joho/godotenv"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

// main 遍历知识库目录并逐个建立索引。
// 目录默认取配置项 file_dir，可用第一个命令行参数覆盖（如从任意 cwd 指定绝对路径）。
func main() {
	fmt.Println("知识库操作:")
	if err := godotenv.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "加载 .env 失败: %v\n", err)
	}
	ctx := context.Background()

	cfg, err := config.Get()
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}
	root := cfg.FileDir
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	r, err := knowledge_index_pipeline.BuildKnowledgeIndexing(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "构建知识库索引失败: %v\n", err)
		os.Exit(1)
	}

	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("读取目录失败: %w", err)
		}
		if d.IsDir() {
			return nil
		}
		if !loader.IsSupportedDoc(path) {
			fmt.Printf("[skip] 不支持的文档格式: %s\n", path)
			return nil
		}

		fmt.Printf("[start] 索引文件: %s\n", path)

		// 删除同一 _source（即文件路径）的旧数据，无需先加载文件。
		cli, err := client.NewMilvusClient(ctx)
		if err != nil {
			return fmt.Errorf("创建 Milvus 客户端失败: %w", err)
		}
		expr := fmt.Sprintf(`metadata["_source"] == %q`, path)
		queryResult, err := cli.Query(ctx, milvusclient.NewQueryOption(cfg.Milvus.CollectionName).WithFilter(expr).WithOutputFields("id"))
		if err != nil {
			return fmt.Errorf("向量查询失败: %w", err)
		}
		if queryResult.ResultCount > 0 {
			var idsToDelete []string
			for _, column := range queryResult.Fields {
				if column.Name() != "id" {
					continue
				}
				for i := 0; i < column.Len(); i++ {
					id, err := column.GetAsString(i)
					if err != nil {
						return fmt.Errorf("读取向量 id 失败: %w", err)
					}
					idsToDelete = append(idsToDelete, id)
				}
			}
			if len(idsToDelete) > 0 {
				deleteExpr := fmt.Sprintf(`id in ["%s"]`, strings.Join(idsToDelete, `","`))
				if _, err := cli.Delete(ctx, milvusclient.NewDeleteOption(cfg.Milvus.CollectionName).WithExpr(deleteExpr)); err != nil {
					return fmt.Errorf("向量删除失败: %w", err)
				}
				g.Log().Infof(ctx, "[info] 删除 %d 条来自 %s 的记录\n", len(idsToDelete), path)
			}
		}

		ids, err := r.Invoke(ctx, document.Source{URI: path}, compose.WithCallbacks(logcallback.New(nil)))
		if err != nil {
			return fmt.Errorf("执行 agent 失败: %w", err)
		}
		g.Log().Infof(ctx, "[info] 索引文件路径: %s, 文件: %d 个, ids: %s\n", path, len(ids), ids)
		return nil
	}); err != nil {
		fmt.Fprintf(os.Stderr, "遍历知识库目录失败: %v\n", err)
		os.Exit(1)
	}
}
