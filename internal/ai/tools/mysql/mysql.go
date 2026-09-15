// Package mysql 提供按调用方给定 DSN 操作 MySQL 的两个工具：mysql_query（只读查询）
// 与 mysql_exec（写入与 DDL）。
//
// 连接由每次调用按入参 DSN 建立、返回前关闭；语句关键字判定只是给模型的引导，
// 不构成安全边界。设计取舍见 dev-docs/mysql.md。
package mysql

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openMySQLFunc 按 DSN 建立连接池，release 用于关闭它。抽成函数类型是为了在测试中注入实现。
type openMySQLFunc func(dsn string) (db *gorm.DB, release func(), err error)

// openMySQL 用原生 DSN 建立 gorm 连接；SQL 日志置为静默，避免污染模型上下文。
func openMySQL(dsn string) (*gorm.DB, func(), error) {
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	return db, func() { _ = sqlDB.Close() }, nil
}

// validateInput 校验 dsn 与 sql 两个必填参数。
func validateInput(dsn, statement string) error {
	if dsn == "" {
		return fmt.Errorf("dsn 参数不能为空")
	}
	if statement == "" {
		return fmt.Errorf("sql 参数不能为空")
	}
	return nil
}

// stmtKind 是语句类型判定结果，用于在 mysql_query 与 mysql_exec 之间互相指路。
type stmtKind int

const (
	// stmtUnknown 表示无法识别首个关键字，按最保守的方式拒绝执行。
	stmtUnknown stmtKind = iota
	// stmtRead 表示查询语句，仅 mysql_query 放行。
	stmtRead
	// stmtWrite 表示增删改与 DDL，仅 mysql_exec 放行。
	stmtWrite
	// stmtDestructive 表示 DROP 与 TRUNCATE，破坏力最大，mysql_exec 也需用户二次确认。
	stmtDestructive
)

// 关键字集合只覆盖首个关键字，不解析子句。
var (
	readKeywords        = map[string]bool{"SELECT": true, "SHOW": true, "DESCRIBE": true, "DESC": true, "EXPLAIN": true, "TABLE": true, "VALUES": true}
	writeKeywords       = map[string]bool{"INSERT": true, "UPDATE": true, "DELETE": true, "REPLACE": true, "CREATE": true, "ALTER": true, "RENAME": true, "GRANT": true, "REVOKE": true, "CALL": true, "SET": true, "USE": true, "ANALYZE": true, "OPTIMIZE": true, "REPAIR": true, "FLUSH": true, "LOAD": true, "HANDLER": true, "DO": true}
	destructiveKeywords = map[string]bool{"DROP": true, "TRUNCATE": true}
)

// classifyStatement 取首个关键字判定语句类型，跳过前导空白与注释。
//
// 需要留意两点：判定不展开子句，所以 WITH ... DELETE 这类会被判为 stmtUnknown 而拒绝；
// 判定只是给模型的引导，不能当作安全边界——`DELETE` 一旦放行就会真的执行，
// 真正的只读约束要靠只读事务或只读账号（见 dev-docs/mysql.md）。
func classifyStatement(sqlText string) stmtKind {
	keyword := firstKeyword(sqlText)
	switch {
	case keyword == "":
		return stmtUnknown
	case destructiveKeywords[keyword]:
		return stmtDestructive
	case readKeywords[keyword]:
		return stmtRead
	case writeKeywords[keyword]:
		return stmtWrite
	default:
		return stmtUnknown
	}
}

// firstKeyword 返回 SQL 的首个关键字（大写），跳过前导空白与注释；无法识别时返回空串。
func firstKeyword(sqlText string) string {
	rest := sqlText
	for {
		rest = strings.TrimLeft(rest, " \t\r\n")
		switch {
		case strings.HasPrefix(rest, "--"), strings.HasPrefix(rest, "#"):
			idx := strings.IndexAny(rest, "\r\n")
			if idx < 0 {
				return ""
			}
			rest = rest[idx+1:]
			continue
		case strings.HasPrefix(rest, "/*"):
			idx := strings.Index(rest, "*/")
			if idx < 0 {
				return ""
			}
			rest = rest[idx+2:]
			continue
		}
		break
	}

	end := 0
	for end < len(rest) && (isASCIILetter(rest[end]) || rest[end] == '_') {
		end++
	}
	if end == 0 {
		return ""
	}
	return strings.ToUpper(rest[:end])
}

// isASCIILetter 判断字节是否为 ASCII 字母。
func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// scanRowsToJSON 执行查询并把结果集序列化为 JSON 数组，空结果集返回 []。
func scanRowsToJSON(ctx context.Context, db *gorm.DB, query string) (string, error) {
	rows, err := db.WithContext(ctx).Raw(query).Rows()
	if err != nil {
		return "", fmt.Errorf("执行查询失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	columns, err := rows.Columns()
	if err != nil {
		return "", fmt.Errorf("获取列名失败: %w", err)
	}

	results := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return "", fmt.Errorf("扫描行数据失败: %w", err)
		}

		row := make(map[string]any, len(columns))
		for i, column := range columns {
			// 驱动把变长字段给成 []byte，转成 string 才不会在 JSON 里变成 base64。
			if b, ok := values[i].([]byte); ok {
				row[column] = string(b)
			} else {
				row[column] = values[i]
			}
		}
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("遍历结果集失败: %w", err)
	}

	return marshalJSON(results)
}

// execSQL 执行写入或 DDL 语句，返回受影响行数。
func execSQL(ctx context.Context, db *gorm.DB, statement string) (string, error) {
	result := db.WithContext(ctx).Exec(statement)
	if result.Error != nil {
		return "", fmt.Errorf("执行 SQL 失败: %w", result.Error)
	}

	return marshalJSON(map[string]any{"rows_affected": result.RowsAffected})
}

// marshalJSON 序列化工具返回值。
func marshalJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("序列化结果失败: %w", err)
	}
	return string(b), nil
}
