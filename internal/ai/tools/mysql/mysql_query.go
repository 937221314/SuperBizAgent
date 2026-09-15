package mysql

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// toolDescMysqlQuery 是 mysql_query 的工具描述。模型只能靠描述在两个工具间选对，
// 因此这里显式指向 mysql_exec。
const toolDescMysqlQuery = "对 MySQL 执行只读查询语句（SELECT/SHOW/DESCRIBE/EXPLAIN 等），结果以 JSON 数组返回。" +
	"这是只读工具：插入、更新、删除或 DDL 请改用 mysql_exec。结果为空数组表示没有匹配数据。"

// QueryInput 是 mysql_query 工具的入参。
type QueryInput struct {
	// DSN 用于连接目标 MySQL，包含用户名、密码、主机、端口与库名。
	DSN string `json:"dsn" jsonschema:"required" jsonschema_description:"用于连接 MySQL 的数据源名称（DSN），包含用户名、密码、主机、端口和数据库名"`
	// SQL 只接受查询语句，写操作会被拒绝并提示改用 mysql_exec。
	SQL string `json:"sql" jsonschema:"required" jsonschema_description:"要执行的只读查询语句，例如 SELECT ...；写操作或 DDL 请改用 mysql_exec"`
}

// NewMysqlQueryTool 创建只读查询工具 mysql_query。
func NewMysqlQueryTool(ctx context.Context) (tool.InvokableTool, error) {
	return newMysqlQueryTool(openMySQL)
}

// newMysqlQueryTool 基于注入的连接构造函数建工具，便于测试替换掉真实 MySQL。
func newMysqlQueryTool(open openMySQLFunc) (tool.InvokableTool, error) {
	handler := func(ctx context.Context, input *QueryInput) (string, error) {
		if err := validateInput(input.DSN, input.SQL); err != nil {
			return "", err
		}

		switch classifyStatement(input.SQL) {
		case stmtRead:
		case stmtWrite, stmtDestructive:
			return "", fmt.Errorf("mysql_query 只接受查询语句，写操作或 DDL 请改用 mysql_exec")
		default:
			return "", fmt.Errorf("无法识别的 SQL 语句：mysql_query 只接受 SELECT/SHOW/DESCRIBE/EXPLAIN 等查询语句")
		}

		db, release, err := open(input.DSN)
		if err != nil {
			return "", err
		}
		defer release()

		return scanRowsToJSON(ctx, db, input.SQL)
	}

	t, err := utils.InferTool("mysql_query", toolDescMysqlQuery, handler)
	if err != nil {
		return nil, fmt.Errorf("创建 mysql_query 工具失败: %w", err)
	}
	return t, nil
}
