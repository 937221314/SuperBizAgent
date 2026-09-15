package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// mysql crud 工具描述
const toolMysqlCrudDescription = "对 MySQL 数据库执行 SQL 查询，并以 JSON 格式返回结果。当你需要查询、插入、更新或删除数据库中的数据时，请使用此工具。结果将格式化为 JSON，以便于解析。"

// MysqlCrudInput 是 mysql crud 工具的入参。
type MysqlCrudInput struct {
	DSN         string `json:"dsn" jsonschema:"description=用于连接 MySQL 数据库的数据源名称（DSN），包括用户名、密码、主机、端口和数据库名"`
	SQL         string `json:"sql" jsonschema:"description=要针对 MySQL 数据库执行的 SQL 查询"`
	OperateType string `json:"operate_type" jsonschema:"description=要执行的 SQL 操作类型：查询（query）、插入（insert）、更新（update）或删除（delete）"`
}

// mysql crud 工具实现
type mysqlCrudTool struct {
	info *schema.ToolInfo
}

// Info 返回工具的元信息
func (t *mysqlCrudTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return t.info, nil
}

// 执行工具
func (t *mysqlCrudTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	// 1. 解析输入参数
	var input MysqlCrudInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return "", fmt.Errorf("解析参数失败: %w", err)
	}

	// 2. 校验必填参数
	if input.DSN == "" {
		return "", fmt.Errorf("dsn 参数不能为空")
	}
	if input.SQL == "" {
		return "", fmt.Errorf("sql 参数不能为空")
	}

	// 3. 连接数据库
	db, err := gorm.Open(mysql.Open(input.DSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // 关闭默认的 SQL 日志，避免干扰
	})
	if err != nil {
		return "", fmt.Errorf("连接数据库失败: %w", err)
	}

	// 获取 *sql.DB, 用于关闭数据库连接
	sqlDB, err := db.DB()
	if err != nil {
		return "", fmt.Errorf("获取数据库连接失败: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()

	// 4. 根据操作类型执行
	switch input.OperateType {
	case "query":
		return t.handleQuery(ctx, db, input.SQL)
	case "insert", "update", "delete":
		return t.handleExec(ctx, db, input.SQL)
	default:
		return "", fmt.Errorf("不支持的操作类型: %s", input.OperateType)
	}
}

// handleQuery 处理查询操作
func (t *mysqlCrudTool) handleQuery(ctx context.Context, db *gorm.DB, query string) (string, error) {
	// 使用 Raw + Rows 执行任意 SQL，并扫描为 []map[string]any
	rows, err := db.WithContext(ctx).Raw(query).Rows()
	if err != nil {
		return "", fmt.Errorf("执行查询失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// 获取列名
	colums, err := rows.Columns()
	if err != nil {
		return "", fmt.Errorf("获取列名失败: %w", err)
	}

	// 构建结果集
	var results []map[string]any
	// 列数
	colsNumber := len(colums)
	for rows.Next() {
		// 创建 与列数对应值的切片
		values := make([]any, colsNumber)
		valuePtrs := make([]any, colsNumber)

		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return "", fmt.Errorf("扫描行数据失败: %w", err)
		}

		// 构建行数据
		row := make(map[string]any)
		for i, col := range colums {
			val := values[i]
			// 将 []byte 转换为 string, 便于序列化
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}

		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("遍历结果集失败: %w", err)
	}

	// 序列化
	output, err := json.Marshal(results)
	if err != nil {
		return "", fmt.Errorf("序列化结果失败: %w", err)
	}

	return string(output), nil
}

// handleExec 处理增删改操作
func (t *mysqlCrudTool) handleExec(ctx context.Context, db *gorm.DB, execSQL string) (string, error) {
	// 执行 SQL
	result := db.WithContext(ctx).Exec(execSQL)
	if result.Error != nil {
		return "", fmt.Errorf("执行 SQL 失败: %w", result.Error)
	}

	// 构建返回结果
	output := map[string]any{
		"rows_affected": result.RowsAffected,
	}

	// 序列化
	jsonOutput, err := json.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("序列化结果失败: %w", err)
	}

	return string(jsonOutput), nil
}

// NewMysqlCrudTool 创建 Mysql CRUD 工具
func NewMysqlCrudTool(ctx context.Context) (tool.InvokableTool, error) {
	// 构建工具信息
	info := &schema.ToolInfo{
		Name: "mysql_crud",
		Desc: toolMysqlCrudDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"dsn": {
				Type:     schema.String,
				Desc:     "用于连接 MySQL 数据库的数据源名称（DSN），包括用户名、密码、主机、端口和数据库名",
				Required: true,
			},
			"sql": {
				Type:     schema.String,
				Desc:     "要针对 MySQL 数据库执行的 SQL 查询",
				Required: true,
			},
			"operate_type": {
				Type:     schema.String,
				Desc:     "要执行的 SQL 操作类型：查询（query）、插入（insert）、更新（update）或删除（delete）",
				Required: true,
				Enum:     []string{"query", "insert", "update", "delete"},
			},
		}),
	}

	return &mysqlCrudTool{info: info}, nil
}
