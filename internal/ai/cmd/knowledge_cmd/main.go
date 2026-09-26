package main

import (
	"SuperBizAgent/internal/ai/agent/knowledge_index_pipeline"
	loader2 "SuperBizAgent/internal/ai/loader"
	"SuperBizAgent/utility/client"
	"SuperBizAgent/utility/common"
	"SuperBizAgent/utility/logcallback"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/compose"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/joho/godotenv"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

func main() {
	fmt.Println("知识库操作:")
	if err := godotenv.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "加载 .env 失败: %v\n", err)
	}
	ctx := context.Background()
	r, err := knowledge_index_pipeline.BuildKnowledgeIndexing(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "构建知识库索引失败: %v\n", err)
		os.Exit(1)
	}
	if err := filepath.WalkDir("./docs", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("读取目录失败: %w", err)
		}
		// 判断是否为目录
		if d.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".md") {
			fmt.Printf("[skip] 不是一个 markdonw 文件: %s\n", path)
			return nil
		}

		fmt.Printf("[start] 索引文件: %s\n", path)
		// 删除 biz 数据 metadata 中 _source 一样的数据
		loader, err := loader2.NewFileLoader(ctx)
		if err != nil {
			return fmt.Errorf("创建加载器失败: %w", err)
		}

		docs, err := loader.Load(ctx, document.Source{URI: path})
		if err != nil {
			return fmt.Errorf("加载文件失败: %w", err)
		}
		cli, err := client.NewMilvusClient(ctx)
		if err != nil {
			return fmt.Errorf("创建 Milvus 客户端失败: %w", err)
		}
		// 查询所有 metadata 中 _source 一样的数据并删除
		expr := fmt.Sprintf(`metadata["_source"] == %q`, docs[0].MetaData["_source"])

		queryResult, err := cli.Query(ctx, milvusclient.NewQueryOption(common.MilvusCollectionName).WithFilter(expr).WithOutputFields("id"))
		if err != nil {
			return fmt.Errorf("向量查询失败: %w", err)
		}
		if queryResult.ResultCount > 0 {
			// 提取需要删除的id
			var idsToDelete []string
			for _, column := range queryResult.Fields {
				if column.Name() == "id" {
					for i := 0; i < column.Len(); i++ {
						id, err := column.GetAsString(i)
						if err != nil {
							return fmt.Errorf("读取向量 id 失败: %w", err)
						}
						idsToDelete = append(idsToDelete, id)
					}
				}
			}

			// 删除这些数据
			if len(idsToDelete) > 0 {
				deleteExpr := fmt.Sprintf(`id in ["%s"]`, strings.Join(idsToDelete, `","`))
				_, err = cli.Delete(ctx, milvusclient.NewDeleteOption(common.MilvusCollectionName).WithExpr(deleteExpr))
				if err != nil {
					return fmt.Errorf("向量删除失败: %w", err)
				}
				g.Log().Infof(ctx, "[info] 删除 %d 条来自 %s 的记录\n", len(idsToDelete), docs[0].MetaData["_source"])
			}

		}
		// 重新构建
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
