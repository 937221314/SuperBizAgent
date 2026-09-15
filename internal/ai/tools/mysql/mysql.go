// Package mysql 提供按调用方给定 DSN 操作 MySQL 的两个工具：mysql_query（只读查询）
// 与 mysql_exec（写入与 DDL）。
//
// 连接按 DSN 复用（见 pool.go）；语句关键字判定只是给模型的引导，真正的只读约束靠
// mysql_query 的只读事务。设计取舍见 dev-docs/mysql.md。
package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// 单次调用的资源上限。
//
// statementTimeout 是上层没有 deadline 时的兜底；maxResultRows 与 maxResultBytes 防止
// 大表全量结果撑爆模型上下文。字节数是估算值，只用于在读取阶段提前截断。
const (
	statementTimeout = 30 * time.Second
	maxResultRows    = 200
	maxResultBytes   = 256 * 1024
)

// openMySQLFunc 按 DSN 取得连接池；缓存的建连与淘汰由实现自己负责，调用方不需要归还。
type openMySQLFunc func(dsn string) (*gorm.DB, error)

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
	// stmtContextual 表示首关键字无法定性，由工具按自身语义决定是否放行。
	// 目前只有 WITH：既可能是只读的 CTE 查询，也可能是 WITH ... DELETE。
	stmtContextual
)

// 关键字集合只覆盖首个关键字，不解析子句。
var (
	readKeywords        = map[string]bool{"SELECT": true, "SHOW": true, "DESCRIBE": true, "DESC": true, "EXPLAIN": true, "TABLE": true, "VALUES": true}
	writeKeywords       = map[string]bool{"INSERT": true, "UPDATE": true, "DELETE": true, "REPLACE": true, "CREATE": true, "ALTER": true, "RENAME": true, "GRANT": true, "REVOKE": true, "CALL": true, "SET": true, "USE": true, "ANALYZE": true, "OPTIMIZE": true, "REPAIR": true, "FLUSH": true, "LOAD": true, "HANDLER": true, "DO": true}
	destructiveKeywords = map[string]bool{"DROP": true, "TRUNCATE": true}
)

// classifyStatement 取首个关键字判定语句类型，跳过前导空白与注释。
//
// 判定不展开子句，所以 WITH 开头的语句归为 stmtContextual：既不解析 CTE 内部到底
// 是查询还是写入，也不给一个错误的确切结论，交由各家工具自己决定（见 stmtContextual）。
// 除 WITH 之外的判定都只是给模型的引导，不能当作安全边界——`DELETE` 一旦放行就会
// 真的执行，真正的只读约束要靠 mysql_query 的只读事务（见 dev-docs/mysql.md）。
func classifyStatement(sqlText string) stmtKind {
	keyword := firstKeyword(sqlText)
	switch {
	case keyword == "":
		return stmtUnknown
	case keyword == "WITH":
		return stmtContextual
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

// queryOutput 是 mysql_query 的结果封装。除 Rows 外带上条数与截断标记，
// 让模型能区分「数据就这么多」与「只看到了一部分」。
type queryOutput struct {
	Rows      []map[string]any `json:"rows"`
	RowCount  int              `json:"row_count"`
	Truncated bool             `json:"truncated"`
}

// scanRowsToJSON 在只读事务中执行查询并序列化结果集，空结果集返回 []。
//
// 只读事务是关键字白名单之外的第二道闸：即便某条写语句绕过了白名单，MySQL 也会在
// 服务端拒绝它（Cannot execute statement in a READ ONLY transaction）。文档把临时表
// 明确列为例外，所以白名单不能因此去掉。
func scanRowsToJSON(ctx context.Context, db *gorm.DB, query string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, statementTimeout)
	defer cancel()

	var out string
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		out, err = scanRowsInTx(tx, query)
		return err
	}, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		// 只读事务被服务端拒绝时补一句提示，让模型知道该换工具而不是以为 SQL 写错了。
		// 按错误文本判断是为了不引入驱动依赖，属于 best-effort。
		if strings.Contains(strings.ToUpper(err.Error()), "READ ONLY") {
			return "", fmt.Errorf("%w（mysql_query 只读，写操作请改用 mysql_exec）", err)
		}
		return "", err
	}
	return out, nil
}

// scanRowsInTx 读取结果集，达到行数或字节上限就截断并标记。
func scanRowsInTx(tx *gorm.DB, query string) (string, error) {
	rows, err := tx.Raw(query).Rows()
	if err != nil {
		return "", fmt.Errorf("执行查询失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	columns, err := rows.Columns()
	if err != nil {
		return "", fmt.Errorf("获取列名失败: %w", err)
	}

	results := make([]map[string]any, 0)
	totalBytes := 0
	truncated := false
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

		rowBytes := estimateRowBytes(row)
		if len(results) >= maxResultRows || totalBytes+rowBytes > maxResultBytes {
			truncated = true
			break
		}
		totalBytes += rowBytes
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("遍历结果集失败: %w", err)
	}

	return marshalJSON(queryOutput{Rows: results, RowCount: len(results), Truncated: truncated})
}

// estimateRowBytes 估算一行序列化后的大小。
// 值在扫描时已统一成 string；非 string 标量（数字、time.Time、nil 等）按 time.Time 的
// 序列化长度估算，宁可估大：估小会让真实输出超出 maxResultBytes。
func estimateRowBytes(row map[string]any) int {
	size := 2 // {}
	for key, value := range row {
		size += len(key) + 4 // 键的引号与冒号
		if s, ok := value.(string); ok {
			size += len(s) + 2
			continue
		}
		size += 26
	}
	return size
}

// execSQL 执行写入或 DDL 语句，返回受影响行数。
func execSQL(ctx context.Context, db *gorm.DB, statement string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, statementTimeout)
	defer cancel()

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
