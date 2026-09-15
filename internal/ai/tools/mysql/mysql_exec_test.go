package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool/utils"
)

// TestMysqlExecRowsAffected 校验写入语句的结果格式。
func TestMysqlExecRowsAffected(t *testing.T) {
	fx := &fakeFixture{rowsAffected: 3}

	tl, err := newMysqlExecTool(newFakeOpen(t, fakeDSN, fx), ModeDangerousDeny)
	if err != nil {
		t.Fatalf("创建工具失败: %v", err)
	}

	const statement = "UPDATE t SET c = 1 WHERE id = 2"
	out, err := tl.InvokableRun(context.Background(), `{"dsn":"`+fakeDSN+`","sql":"`+statement+`"}`)
	if err != nil {
		t.Fatalf("执行工具失败: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("输出不是合法 JSON: %v，原文 %s", err, out)
	}
	if got["rows_affected"] != float64(3) {
		t.Errorf("rows_affected 应为 3，实际 %#v", got["rows_affected"])
	}

	sent := fx.executedSQL()
	if len(sent) != 1 || sent[0] != statement {
		t.Errorf("下发语句不符: %v", sent)
	}
}

// TestMysqlExecAcceptsWriteAndDDL 校验增删改与 DDL 均被放行。
func TestMysqlExecAcceptsWriteAndDDL(t *testing.T) {
	statements := []string{
		"INSERT INTO t (id) VALUES (1)",
		"UPDATE t SET c = 1",
		"DELETE FROM t WHERE id = 1",
		"CREATE TABLE t (id int)",
		"ALTER TABLE t ADD COLUMN c int",
	}

	for _, statement := range statements {
		t.Run(statement, func(t *testing.T) {
			fx := &fakeFixture{}

			tl, err := newMysqlExecTool(newFakeOpen(t, fakeDSN, fx), ModeDangerousDeny)
			if err != nil {
				t.Fatalf("创建工具失败: %v", err)
			}

			if _, err := tl.InvokableRun(context.Background(), `{"dsn":"`+fakeDSN+`","sql":"`+statement+`"}`); err != nil {
				t.Fatalf("期望放行，实际报错: %v", err)
			}
			if sent := fx.executedSQL(); len(sent) != 1 {
				t.Errorf("期望下发 1 条语句，实际 %v", sent)
			}
		})
	}
}

// TestMysqlExecRejectsRead 校验查询语句被拒并指向 mysql_query。
func TestMysqlExecRejectsRead(t *testing.T) {
	fx := &fakeFixture{}

	tl, err := newMysqlExecTool(newFakeOpen(t, fakeDSN, fx), ModeDangerousDeny)
	if err != nil {
		t.Fatalf("创建工具失败: %v", err)
	}

	_, err = tl.InvokableRun(context.Background(), `{"dsn":"`+fakeDSN+`","sql":"SELECT 1"}`)
	if err == nil {
		t.Fatalf("期望被拒绝，实际执行成功")
	}
	if !strings.Contains(err.Error(), "mysql_query") {
		t.Errorf("错误信息应指向 mysql_query，实际: %v", err)
	}
	if sent := fx.executedSQL(); len(sent) != 0 {
		t.Errorf("被拒语句不应下发，实际下发: %v", sent)
	}
}

// TestMysqlExecRejectsDestructive 校验 deny 模式（默认）下 DROP 与 TRUNCATE 不执行，
// 并向模型返回 canceled 结果。
func TestMysqlExecRejectsDestructive(t *testing.T) {
	cases := []struct {
		sql     string
		keyword string
	}{
		{"DROP TABLE t", "DROP"},
		{"DROP DATABASE d", "DROP"},
		{"TRUNCATE TABLE t", "TRUNCATE"},
		{"  /* x */ truncate t", "TRUNCATE"},
	}

	for _, c := range cases {
		t.Run(c.sql, func(t *testing.T) {
			fx := &fakeFixture{}

			tl, err := newMysqlExecTool(newFakeOpen(t, fakeDSN, fx), ModeDangerousDeny)
			if err != nil {
				t.Fatalf("创建工具失败: %v", err)
			}

			out, err := tl.InvokableRun(context.Background(), `{"dsn":"`+fakeDSN+`","sql":"`+c.sql+`"}`)
			if err != nil {
				t.Fatalf("deny 模式不应返回 error（error 会让整图失败），实际: %v", err)
			}

			var got deniedResult
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("输出不是合法 JSON: %v，原文 %s", err, out)
			}
			if !got.Canceled {
				t.Errorf("canceled 应为 true，实际 %s", out)
			}
			if !strings.Contains(got.Message, "危险操作") || !strings.Contains(got.Message, c.keyword) {
				t.Errorf("提示应含 %s 与「危险操作」，实际: %s", c.keyword, got.Message)
			}
			if sent := fx.executedSQL(); len(sent) != 0 {
				t.Errorf("被拒语句不应下发，实际下发: %v", sent)
			}
		})
	}
}

// TestMysqlExecRejectsUnknown 校验无法识别的语句按保守方式拒绝。
func TestMysqlExecRejectsUnknown(t *testing.T) {
	fx := &fakeFixture{}

	tl, err := newMysqlExecTool(newFakeOpen(t, fakeDSN, fx), ModeDangerousDeny)
	if err != nil {
		t.Fatalf("创建工具失败: %v", err)
	}

	_, err = tl.InvokableRun(context.Background(), `{"dsn":"`+fakeDSN+`","sql":"VACUUM t"}`)
	if err == nil {
		t.Fatalf("期望被拒绝，实际执行成功")
	}
	if !strings.Contains(err.Error(), "无法识别") {
		t.Errorf("错误信息应说明无法识别，实际: %v", err)
	}
}

// TestMysqlExecError 校验执行失败时保留原错误。
func TestMysqlExecError(t *testing.T) {
	sentinel := errors.New("写入炸了")
	fx := &fakeFixture{execErr: sentinel}

	tl, err := newMysqlExecTool(newFakeOpen(t, fakeDSN, fx), ModeDangerousDeny)
	if err != nil {
		t.Fatalf("创建工具失败: %v", err)
	}

	_, err = tl.InvokableRun(context.Background(), `{"dsn":"`+fakeDSN+`","sql":"UPDATE t SET c = 1"}`)
	if !errors.Is(err, sentinel) {
		t.Fatalf("期望包装并保留原错误，实际: %v", err)
	}
}

// TestMysqlExecInputRequired 校验 dsn 与 sql 为空时的报错。
func TestMysqlExecInputRequired(t *testing.T) {
	fx := &fakeFixture{}

	tl, err := newMysqlExecTool(newFakeOpen(t, fakeDSN, fx), ModeDangerousDeny)
	if err != nil {
		t.Fatalf("创建工具失败: %v", err)
	}

	cases := []struct {
		name            string
		args            string
		wantErrContains string
	}{
		{"缺 dsn", `{"sql":"UPDATE t SET c = 1"}`, "dsn"},
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

// TestMysqlExecSchema 校验暴露给模型的参数 schema。
func TestMysqlExecSchema(t *testing.T) {
	info, err := utils.GoStruct2ToolInfo[*ExecInput]("mysql_exec", "test")
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
