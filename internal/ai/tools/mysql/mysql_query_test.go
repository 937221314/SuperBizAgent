package mysql

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool/utils"
)

// fakeDSN 是测试用 DSN，只作为 fake 驱动的注册键，不会被真正解析。
const fakeDSN = "user:pass@tcp(127.0.0.1:3306)/demo"

// TestClassifyStatement 校验首关键字判定的边界，尤其是注释前缀与 WITH 这类危险写法。
func TestClassifyStatement(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want stmtKind
	}{
		{"select", "SELECT id FROM t", stmtRead},
		{"前导空白与小写", "  \n\tselect 1", stmtRead},
		{"行注释", "-- 注释\nSELECT 1", stmtRead},
		{"井号注释", "# 注释\nSHOW TABLES", stmtRead},
		{"块注释", "/* 注释 */ EXPLAIN SELECT 1", stmtRead},
		{"show", "SHOW TABLES", stmtRead},
		{"describe", "DESCRIBE t", stmtRead},
		{"insert", "INSERT INTO t VALUES (1)", stmtWrite},
		{"create", "CREATE TABLE t (id int)", stmtWrite},
		{"alter", "ALTER TABLE t ADD COLUMN c int", stmtWrite},
		{"update", "UPDATE t SET c = 1", stmtWrite},
		{"delete", "DELETE FROM t", stmtWrite},
		{"drop", "DROP TABLE t", stmtDestructive},
		{"注释后小写 drop", "  /* x */ drop database d", stmtDestructive},
		{"truncate", "TRUNCATE TABLE t", stmtDestructive},
		{"with 不算只读", "WITH c AS (SELECT 1) DELETE FROM t", stmtUnknown},
		{"空串", "", stmtUnknown},
		{"只有注释", "-- 注释", stmtUnknown},
		{"未闭合块注释", "/* 未闭合 SELECT 1", stmtUnknown},
		{"非字母开头", "(SELECT 1)", stmtUnknown},
		{"未知关键字", "VACUUM t", stmtUnknown},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyStatement(c.sql); got != c.want {
				t.Errorf("classifyStatement(%q) = %d，期望 %d", c.sql, got, c.want)
			}
		})
	}
}

// TestMysqlQueryScan 校验结果集扫描、[]byte 转换与 JSON 序列化。
func TestMysqlQueryScan(t *testing.T) {
	fx := &fakeFixture{
		columns: []string{"id", "name", "payload"},
		rows: [][]driver.Value{
			{int64(1), "alice", []byte("blob-1")},
			{int64(2), nil, nil},
		},
	}

	tl, err := newMysqlQueryTool(newFakeOpen(t, fakeDSN, fx))
	if err != nil {
		t.Fatalf("创建工具失败: %v", err)
	}

	const statement = "SELECT id, name, payload FROM t"
	out, err := tl.InvokableRun(context.Background(), `{"dsn":"`+fakeDSN+`","sql":"`+statement+`"}`)
	if err != nil {
		t.Fatalf("执行工具失败: %v", err)
	}

	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("输出不是合法 JSON: %v，原文 %s", err, out)
	}
	if len(rows) != 2 {
		t.Fatalf("期望 2 行，实际 %d 行：%s", len(rows), out)
	}
	if rows[0]["name"] != "alice" {
		t.Errorf("第 1 行 name 不符: %#v", rows[0]["name"])
	}
	if rows[0]["payload"] != "blob-1" {
		t.Errorf("[]byte 未转成 string，实际 %#v", rows[0]["payload"])
	}
	if rows[1]["name"] != nil {
		t.Errorf("NULL 应为 nil，实际 %#v", rows[1]["name"])
	}

	sent := fx.queriedSQL()
	if len(sent) != 1 || sent[0] != statement {
		t.Errorf("下发语句不符: %v", sent)
	}
}

// TestMysqlQueryEmptyResult 校验空结果集返回 []，而不是 null。
func TestMysqlQueryEmptyResult(t *testing.T) {
	fx := &fakeFixture{columns: []string{"id"}}

	tl, err := newMysqlQueryTool(newFakeOpen(t, fakeDSN, fx))
	if err != nil {
		t.Fatalf("创建工具失败: %v", err)
	}

	out, err := tl.InvokableRun(context.Background(), `{"dsn":"`+fakeDSN+`","sql":"SELECT id FROM t WHERE 1 = 0"}`)
	if err != nil {
		t.Fatalf("执行工具失败: %v", err)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("空结果集应返回 []，实际 %s", out)
	}
}

// TestMysqlQueryRejectsNonRead 校验非查询语句被拒，且没有下发到数据库。
func TestMysqlQueryRejectsNonRead(t *testing.T) {
	cases := []struct {
		name            string
		sql             string
		wantErrContains string
	}{
		{"delete", "DELETE FROM t", "mysql_exec"},
		{"drop", "DROP TABLE t", "mysql_exec"},
		{"truncate", "TRUNCATE TABLE t", "mysql_exec"},
		{"insert", "INSERT INTO t VALUES (1)", "mysql_exec"},
		{"with 写语句", "WITH c AS (SELECT 1) DELETE FROM t", "无法识别"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fx := &fakeFixture{columns: []string{"id"}}

			tl, err := newMysqlQueryTool(newFakeOpen(t, fakeDSN, fx))
			if err != nil {
				t.Fatalf("创建工具失败: %v", err)
			}

			_, err = tl.InvokableRun(context.Background(), `{"dsn":"`+fakeDSN+`","sql":"`+c.sql+`"}`)
			if err == nil {
				t.Fatalf("期望被拒绝，实际执行成功")
			}
			if !strings.Contains(err.Error(), c.wantErrContains) {
				t.Errorf("错误信息应包含 %q，实际: %v", c.wantErrContains, err)
			}
			if sent := fx.queriedSQL(); len(sent) != 0 {
				t.Errorf("被拒语句不应下发，实际下发: %v", sent)
			}
			if sent := fx.executedSQL(); len(sent) != 0 {
				t.Errorf("被拒语句不应下发，实际下发: %v", sent)
			}
		})
	}
}

// TestMysqlQueryError 校验查询失败时保留原错误。
func TestMysqlQueryError(t *testing.T) {
	sentinel := errors.New("查询炸了")
	fx := &fakeFixture{queryErr: sentinel}

	tl, err := newMysqlQueryTool(newFakeOpen(t, fakeDSN, fx))
	if err != nil {
		t.Fatalf("创建工具失败: %v", err)
	}

	_, err = tl.InvokableRun(context.Background(), `{"dsn":"`+fakeDSN+`","sql":"SELECT 1"}`)
	if !errors.Is(err, sentinel) {
		t.Fatalf("期望包装并保留原错误，实际: %v", err)
	}
}

// TestMysqlQueryInputRequired 校验 dsn 与 sql 为空时的报错。
func TestMysqlQueryInputRequired(t *testing.T) {
	fx := &fakeFixture{columns: []string{"id"}}

	tl, err := newMysqlQueryTool(newFakeOpen(t, fakeDSN, fx))
	if err != nil {
		t.Fatalf("创建工具失败: %v", err)
	}

	cases := []struct {
		name            string
		args            string
		wantErrContains string
	}{
		{"缺 dsn", `{"sql":"SELECT 1"}`, "dsn"},
		{"缺 sql", `{"dsn":"` + fakeDSN + `"}`, "sql"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := tl.InvokableRun(context.Background(), c.args)
			if err == nil {
				t.Fatalf("期望参数校验失败")
			}
			if !strings.Contains(err.Error(), c.wantErrContains) {
				t.Errorf("错误信息应包含 %q，实际: %v", c.wantErrContains, err)
			}
		})
	}
}

// TestMysqlQuerySchema 校验暴露给模型的参数 schema。
func TestMysqlQuerySchema(t *testing.T) {
	info, err := utils.GoStruct2ToolInfo[*QueryInput]("mysql_query", "test")
	if err != nil {
		t.Fatalf("推导 schema 失败: %v", err)
	}

	js, err := info.ToJSONSchema()
	if err != nil {
		t.Fatalf("转换 schema 失败: %v", err)
	}
	raw, err := json.Marshal(js)
	if err != nil {
		t.Fatalf("序列化 schema 失败: %v", err)
	}

	s := string(raw)
	for _, want := range []string{`"dsn"`, `"sql"`, `"required"`} {
		if !strings.Contains(s, want) {
			t.Errorf("schema 缺少 %s: %s", want, s)
		}
	}
	if strings.Contains(s, "operate_type") {
		t.Errorf("schema 仍残留 operate_type: %s", s)
	}
}
