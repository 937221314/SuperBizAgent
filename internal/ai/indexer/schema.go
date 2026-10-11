package indexer

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

// 与 eino milvus2 indexer 的默认 schema 保持一致。eino 对应常量未导出，
// 这里按 dev-docs/milvus.md 记录的值复刻；eino 默认值变更时需同步。
const (
	schemaIDField       = "id"
	schemaContentField  = "content"
	schemaMetadataField = "metadata"
	schemaVectorField   = "vector"

	schemaMaxIDLen      = 512
	schemaMaxContentLen = 65535
)

// fieldSpec 期望的字段定义，用于与现有集合 schema 比对。
type fieldSpec struct {
	name       string
	dataType   entity.FieldType
	maxLength  int64 // >0 时校验 VarChar 的 max_length
	dim        int64 // >0 时校验 FloatVector 的 dim
	primaryKey bool
}

// ensureCollectionSchema 校验已存在集合的 schema 是否与代码期望一致。
// 集合不存在时直接返回，交给 eino indexer 创建。
// recreateOnMismatch 为 true 时删除集合重建（会丢失数据），否则返回错误快速失败。
func ensureCollectionSchema(ctx context.Context, cli *milvusclient.Client, collection string, vectorDim int64, recreateOnMismatch bool) error {
	has, err := cli.HasCollection(ctx, milvusclient.NewHasCollectionOption(collection))
	if err != nil {
		return fmt.Errorf("检查集合 %s 是否存在失败: %w", collection, err)
	}
	if !has {
		return nil
	}

	coll, err := cli.DescribeCollection(ctx, milvusclient.NewDescribeCollectionOption(collection))
	if err != nil {
		return fmt.Errorf("读取集合 %s schema 失败: %w", collection, err)
	}

	diffs := diffSchema(coll.Schema, []fieldSpec{
		{name: schemaIDField, dataType: entity.FieldTypeVarChar, maxLength: schemaMaxIDLen, primaryKey: true},
		{name: schemaContentField, dataType: entity.FieldTypeVarChar, maxLength: schemaMaxContentLen},
		{name: schemaMetadataField, dataType: entity.FieldTypeJSON},
		{name: schemaVectorField, dataType: entity.FieldTypeFloatVector, dim: vectorDim},
	})
	if len(diffs) == 0 {
		return nil
	}

	if recreateOnMismatch {
		g.Log().Warningf(ctx, "[indexer] 集合 %s 的 schema 与代码不一致（%s），按配置删除重建",
			collection, strings.Join(diffs, "；"))
		if err := cli.DropCollection(ctx, milvusclient.NewDropCollectionOption(collection)); err != nil {
			return fmt.Errorf("删除集合 %s 失败: %w", collection, err)
		}
		return nil
	}

	return fmt.Errorf("集合 %s 的 schema 与代码期望不一致（%s）；"+
		"请手动重建该集合，或设置 milvus.recreate_on_schema_mismatch: true 自动重建",
		collection, strings.Join(diffs, "；"))
}

// diffSchema 比较现有 schema 与期望字段，返回差异描述；一致时返回 nil。
// 字段顺序无关；动态字段（$meta）不参与比较。
func diffSchema(existing *entity.Schema, expected []fieldSpec) []string {
	if existing == nil {
		return []string{"现有 schema 为空"}
	}

	actual := make(map[string]*entity.Field, len(existing.Fields))
	for _, f := range existing.Fields {
		if f.IsDynamic || f.Name == "$meta" {
			continue
		}
		actual[f.Name] = f
	}

	var diffs []string
	for _, want := range expected {
		got, ok := actual[want.name]
		if !ok {
			diffs = append(diffs, fmt.Sprintf("缺少字段 %s", want.name))
			continue
		}
		delete(actual, want.name)

		if got.DataType != want.dataType {
			diffs = append(diffs, fmt.Sprintf("字段 %s 类型为 %s，期望 %s",
				want.name, got.DataType.Name(), want.dataType.Name()))
			continue
		}
		if got.PrimaryKey != want.primaryKey {
			diffs = append(diffs, fmt.Sprintf("字段 %s 主键属性不一致（实际 %t，期望 %t）",
				want.name, got.PrimaryKey, want.primaryKey))
		}
		if want.maxLength > 0 && got.TypeParams[entity.TypeParamMaxLength] != strconv.FormatInt(want.maxLength, 10) {
			diffs = append(diffs, fmt.Sprintf("字段 %s max_length 为 %q，期望 %d",
				want.name, got.TypeParams[entity.TypeParamMaxLength], want.maxLength))
		}
		if want.dim > 0 && got.TypeParams[entity.TypeParamDim] != strconv.FormatInt(want.dim, 10) {
			diffs = append(diffs, fmt.Sprintf("字段 %s dim 为 %q，期望 %d",
				want.name, got.TypeParams[entity.TypeParamDim], want.dim))
		}
	}

	for name := range actual {
		diffs = append(diffs, fmt.Sprintf("存在多余字段 %s", name))
	}

	sort.Strings(diffs)
	return diffs
}
