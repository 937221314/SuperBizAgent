package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/agent/knowledge_index_pipeline"
	loader2 "SuperBizAgent/internal/ai/loader"
	"SuperBizAgent/internal/config"
	"SuperBizAgent/utility/client"
	"SuperBizAgent/utility/common"
	"SuperBizAgent/utility/logcallback"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/compose"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

// FileUpload 处理文件上传请求，保存文件并构建知识库索引。
func (c *ControllerV1) FileUpload(ctx context.Context, req *v1.FileUploadReq) (res *v1.FileUploadRes, err error) {
	// 从请求中获取文件参数
	r := g.RequestFromCtx(ctx)
	uploadFile := r.GetUploadFile("file")
	if uploadFile == nil {
		return nil, gerror.New("请上传文件")
	}
	// 从配置读取知识库文档存放目录
	cfg, err := config.Get()
	if err != nil {
		return nil, gerror.Wrapf(err, "加载配置失败")
	}
	fileDir := cfg.FileDir

	// 确保上传目录存在
	if !gfile.Exists(fileDir) {
		if err := gfile.Mkdir(fileDir); err != nil {
			return nil, gerror.Wrapf(err, "创建目录失败: %s", fileDir)
		}
	}

	// 获取原始文件名
	oldFileName := uploadFile.Filename

	// 保存文件；上传文件的 Save 方法返回的是文件名（basename），需与目录拼成完整路径
	savedName, err := uploadFile.Save(fileDir, false)
	if err != nil {
		return nil, gerror.Wrapf(err, "保存文件失败")
	}
	savedFilePath := filepath.Join(fileDir, savedName)

	// 获取文件信息
	fileInfo, err := os.Stat(savedFilePath)
	if err != nil {
		return nil, gerror.Wrapf(err, "获取文件信息失败")
	}

	res = &v1.FileUploadRes{
		FileName: oldFileName,
		FilePath: savedFilePath,
		FileSize: fileInfo.Size(),
	}

	err = buildIntoIndex(ctx, savedFilePath)
	if err != nil {
		return nil, gerror.Wrapf(err, "构建知识库失败")
	}

	return res, nil
}

// buildIntoIndex 构建索引
func buildIntoIndex(ctx context.Context, path string) error {
	r, err := knowledge_index_pipeline.BuildKnowledgeIndexing(ctx)
	if err != nil {
		return err
	}
	// 删除 biz 集合数据 metadata 中 _source 相同的数据。
	// _source 由 FileLoader 写入，值即文件路径本身，无需先加载文件（避免 PDF/DOCX 被解析两遍）。
	if !loader2.IsSupportedDoc(path) {
		return fmt.Errorf("不支持的文档格式: %s", filepath.Ext(path))
	}

	cli, err := client.NewMilvusClient(ctx)
	if err != nil {
		return fmt.Errorf("创建 Milvus 客户端失败: %w", err)
	}

	// 查询所有 metadata 中 _source 相同的数据并删除
	expr := fmt.Sprintf(`metadata["_source"] == %q`, path)
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

		// 删除数据
		if len(idsToDelete) > 0 {
			deleteExpr := fmt.Sprintf(`id in ["%s"]`, strings.Join(idsToDelete, `","`))
			_, err := cli.Delete(ctx, milvusclient.NewDeleteOption(common.MilvusCollectionName).WithExpr(deleteExpr))
			if err != nil {
				return fmt.Errorf("向量删除失败: %w", err)
			}
			g.Log().Infof(ctx, "[info] 删除 %d 条来自 %s 的记录\n", len(idsToDelete), path)
		}
	}

	// 重新构建
	ids, err := r.Invoke(ctx, document.Source{URI: path}, compose.WithCallbacks(logcallback.New(nil)))
	if err != nil {
		return fmt.Errorf("执行 agent 失败: %w", err)
	}
	g.Log().Infof(ctx, "[info] 索引文件路径: %s, 文件: %d 个, ids: %s\n", path, len(ids), ids)

	return nil
}
