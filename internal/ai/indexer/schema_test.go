package indexer

import (
	"strings"
	"testing"

	"github.com/milvus-io/milvus/client/v2/entity"
)

// expectedSpecs 返回与 ensureCollectionSchema 一致的期望字段（向量维度取 1024）。
func expectedSpecs() []fieldSpec {
	return []fieldSpec{
		{name: schemaIDField, dataType: entity.FieldTypeVarChar, maxLength: schemaMaxIDLen, primaryKey: true},
		{name: schemaContentField, dataType: entity.FieldTypeVarChar, maxLength: schemaMaxContentLen},
		{name: schemaMetadataField, dataType: entity.FieldTypeJSON},
		{name: schemaVectorField, dataType: entity.FieldTypeFloatVector, dim: 1024},
	}
}

func newTestSchema(fields ...*entity.Field) *entity.Schema {
	s := entity.NewSchema()
	for _, f := range fields {
		s.WithField(f)
	}
	return s
}

func validFields() []*entity.Field {
	return []*entity.Field{
		entity.NewField().WithName("id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(512).WithIsPrimaryKey(true),
		entity.NewField().WithName("content").WithDataType(entity.FieldTypeVarChar).WithMaxLength(65535),
		entity.NewField().WithName("metadata").WithDataType(entity.FieldTypeJSON),
		entity.NewField().WithName("vector").WithDataType(entity.FieldTypeFloatVector).WithDim(1024),
	}
}

// TestDiffSchema 覆盖集合 schema 比对的各类差异。
func TestDiffSchema(t *testing.T) {
	t.Run("一致时无差异", func(t *testing.T) {
		if diffs := diffSchema(newTestSchema(validFields()...), expectedSpecs()); len(diffs) != 0 {
			t.Fatalf("期望无差异，实际 %v", diffs)
		}
	})

	t.Run("字段顺序无关", func(t *testing.T) {
		fs := validFields()
		fs[0], fs[3] = fs[3], fs[0]
		if diffs := diffSchema(newTestSchema(fs...), expectedSpecs()); len(diffs) != 0 {
			t.Fatalf("期望无差异，实际 %v", diffs)
		}
	})

	t.Run("维度变化", func(t *testing.T) {
		fs := validFields()
		fs[3] = entity.NewField().WithName("vector").WithDataType(entity.FieldTypeFloatVector).WithDim(768)
		assertHasDiff(t, diffSchema(newTestSchema(fs...), expectedSpecs()), "dim")
	})

	t.Run("缺少字段", func(t *testing.T) {
		assertHasDiff(t, diffSchema(newTestSchema(validFields()[:3]...), expectedSpecs()), "缺少字段 vector")
	})

	t.Run("多余字段", func(t *testing.T) {
		fs := append(validFields(),
			entity.NewField().WithName("extra").WithDataType(entity.FieldTypeVarChar).WithMaxLength(10))
		assertHasDiff(t, diffSchema(newTestSchema(fs...), expectedSpecs()), "多余字段 extra")
	})

	t.Run("类型变化", func(t *testing.T) {
		fs := validFields()
		fs[2] = entity.NewField().WithName("metadata").WithDataType(entity.FieldTypeVarChar).WithMaxLength(65535)
		assertHasDiff(t, diffSchema(newTestSchema(fs...), expectedSpecs()), "类型")
	})

	t.Run("主键属性变化", func(t *testing.T) {
		fs := validFields()
		fs[0] = entity.NewField().WithName("id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(512)
		assertHasDiff(t, diffSchema(newTestSchema(fs...), expectedSpecs()), "主键")
	})

	t.Run("动态字段忽略", func(t *testing.T) {
		dynamic := entity.NewField().WithName("$meta").WithDataType(entity.FieldTypeJSON)
		dynamic.IsDynamic = true
		fs := append(validFields(), dynamic)
		if diffs := diffSchema(newTestSchema(fs...), expectedSpecs()); len(diffs) != 0 {
			t.Fatalf("动态字段应被忽略，实际差异 %v", diffs)
		}
	})

	t.Run("schema 为空", func(t *testing.T) {
		assertHasDiff(t, diffSchema(nil, expectedSpecs()), "现有 schema 为空")
	})
}

func assertHasDiff(t *testing.T, diffs []string, want string) {
	t.Helper()
	for _, d := range diffs {
		if strings.Contains(d, want) {
			return
		}
	}
	t.Fatalf("期望差异包含 %q，实际 %v", want, diffs)
}
