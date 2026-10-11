package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeConfigFile 写一份临时配置文件并返回其路径。
func writeConfigFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入配置文件失败: %v", err)
	}
	return path
}

// TestLoadMilvusIndex 校验 milvus.index 的默认值、显式取值与越界拦截。
func TestLoadMilvusIndex(t *testing.T) {
	cases := []struct {
		name        string
		content     string
		wantM       int
		wantEf      int
		wantLoadErr bool
	}{
		{
			name:    "未配置时取默认值",
			content: "",
			wantM:   16,
			wantEf:  200,
		},
		{
			name:    "显式配置",
			content: "milvus:\n  index:\n    m: 32\n    ef_construction: 400\n",
			wantM:   32,
			wantEf:  400,
		},
		{
			name:        "m 越界报错",
			content:     "milvus:\n  index:\n    m: 2\n",
			wantLoadErr: true,
		},
		{
			name:        "ef_construction 越界报错",
			content:     "milvus:\n  index:\n    ef_construction: 1000\n",
			wantLoadErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Load(writeConfigFile(t, c.content))
			if c.wantLoadErr {
				if err == nil {
					t.Fatalf("期望加载失败，实际 m=%d ef=%d", got.Milvus.Index.M, got.Milvus.Index.EfConstruction)
				}
				return
			}
			if err != nil {
				t.Fatalf("加载配置失败: %v", err)
			}
			if got.Milvus.Index.M != c.wantM || got.Milvus.Index.EfConstruction != c.wantEf {
				t.Errorf("index = {%d, %d}，期望 {%d, %d}",
					got.Milvus.Index.M, got.Milvus.Index.EfConstruction, c.wantM, c.wantEf)
			}
		})
	}
}

// TestLoadDangerousStatementMode 校验 mysql.dangerous_statement_mode 的默认值、
// 显式取值、环境变量覆盖与非法取值拦截。
func TestLoadDangerousStatementMode(t *testing.T) {
	cases := []struct {
		name        string
		content     string
		env         string
		want        string
		wantLoadErr bool
	}{
		{
			name:    "未配置时取默认 deny",
			content: "",
			env:     "",
			want:    "deny",
		},
		{
			name:    "显式配置 interrupt",
			content: "mysql:\n  dangerous_statement_mode: \"interrupt\"\n",
			env:     "",
			want:    "interrupt",
		},
		{
			name:    "环境变量覆盖配置文件",
			content: "mysql:\n  dangerous_statement_mode: \"deny\"\n",
			env:     "interrupt",
			want:    "interrupt",
		},
		{
			name:        "非法取值在加载期报错",
			content:     "mysql:\n  dangerous_statement_mode: \"drop\"\n",
			env:         "",
			wantLoadErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// 固定环境变量，避免开发者本机环境影响用例。
			t.Setenv(EnvMysqlDangerousStatementMode, c.env)

			got, err := Load(writeConfigFile(t, c.content))
			if c.wantLoadErr {
				if err == nil {
					t.Fatalf("期望加载失败，实际成功且取值为 %q", got.Mysql.DangerousStatementMode)
				}
				return
			}
			if err != nil {
				t.Fatalf("加载配置失败: %v", err)
			}
			if got.Mysql.DangerousStatementMode != c.want {
				t.Errorf("dangerous_statement_mode = %q，期望 %q", got.Mysql.DangerousStatementMode, c.want)
			}
		})
	}
}
