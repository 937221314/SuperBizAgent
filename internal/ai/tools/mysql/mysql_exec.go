package mysql

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// toolDescMysqlExec 是 mysql_exec 的工具描述。模型只能靠描述在两个工具间选对，
// 因此这里显式指向 mysql_query，并说明 DROP/TRUNCATE 会被拒绝。
const toolDescMysqlExec = "对 MySQL 执行写入或 DDL 语句（INSERT/UPDATE/DELETE/CREATE/ALTER 等），返回受影响行数。" +
	"查询语句请改用 mysql_query；DROP 与 TRUNCATE 属于危险操作，会被拒绝并提示与用户确认。"

// ExecInput 是 mysql_exec 工具的入参。
type ExecInput struct {
	// DSN 用于连接目标 MySQL，包含用户名、密码、主机、端口与库名。
	DSN string `json:"dsn" jsonschema:"required" jsonschema_description:"用于连接 MySQL 的数据源名称（DSN），包含用户名、密码、主机、端口和数据库名"`
	// SQL 接受写入与 DDL 语句，查询语句会被拒绝并提示改用 mysql_query。
	SQL string `json:"sql" jsonschema:"required" jsonschema_description:"要执行的写入或 DDL 语句，例如 INSERT/UPDATE/DELETE/CREATE/ALTER；查询请改用 mysql_query"`
}

// NewMysqlExecTool 创建写入工具 mysql_exec。
func NewMysqlExecTool(ctx context.Context) (tool.InvokableTool, error) {
	return newMysqlExecTool(openMySQL)
}

// newMysqlExecTool 基于注入的连接构造函数建工具，便于测试替换掉真实 MySQL。
func newMysqlExecTool(open openMySQLFunc) (tool.InvokableTool, error) {
	handler := func(ctx context.Context, input *ExecInput) (string, error) {
		if err := validateInput(input.DSN, input.SQL); err != nil {
			return "", err
		}

		switch classifyStatement(input.SQL) {
		case stmtWrite:
			// 放行：增删改与 DDL。
		case stmtRead:
			return "", fmt.Errorf("mysql_exec 用于写入或 DDL，查询语句请改用 mysql_query")
		case stmtDestructive:
			return "", fmt.Errorf("%s 属于危险操作，已拒绝执行；如需继续，请先与用户确认后在工具外手动处理", firstKeyword(input.SQL))
		default:
			return "", fmt.Errorf("无法识别的 SQL 语句：mysql_exec 只接受 INSERT/UPDATE/DELETE/CREATE/ALTER 等写入或 DDL 语句")
		}

		db, release, err := open(input.DSN)
		if err != nil {
			return "", err
		}
		defer release()

		return execSQL(ctx, db, input.SQL)
	}

	t, err := utils.InferTool("mysql_exec", toolDescMysqlExec, handler)
	if err != nil {
		return nil, fmt.Errorf("创建 mysql_exec 工具失败: %w", err)
	}
	return t, nil
}
