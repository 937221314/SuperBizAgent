package mysql

import (
	"context"
	"fmt"

	"SuperBizAgent/internal/config"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// toolDescMysqlExec 是 mysql_exec 的工具描述。模型只能靠描述在两个工具间选对，
// 因此这里显式指向 mysql_query，并说明危险语句的确认语义。
const toolDescMysqlExec = "对 MySQL 执行写入或 DDL 语句（INSERT/UPDATE/DELETE/CREATE/ALTER 等），返回受影响行数。" +
	"查询语句请改用 mysql_query。DROP 与 TRUNCATE 属于危险操作，需用户确认后才会执行；" +
	"返回结果中 canceled 为 true 表示未获确认、语句未执行，此时不要重试。"

// ExecInput 是 mysql_exec 工具的入参。
type ExecInput struct {
	// DSN 用于连接目标 MySQL，包含用户名、密码、主机、端口与库名。
	DSN string `json:"dsn" jsonschema:"required" jsonschema_description:"用于连接 MySQL 的数据源名称（DSN），包含用户名、密码、主机、端口和数据库名"`
	// SQL 接受写入与 DDL 语句，查询语句会被拒绝并提示改用 mysql_query。
	SQL string `json:"sql" jsonschema:"required" jsonschema_description:"要执行的写入或 DDL 语句，例如 INSERT/UPDATE/DELETE/CREATE/ALTER；查询请改用 mysql_query"`
}

// DangerousStatementMode 决定 mysql_exec 遇到 DROP/TRUNCATE 时的处理方式。
type DangerousStatementMode string

const (
	// ModeDangerousDeny 直接拒绝危险语句，既不执行也不挂起。
	ModeDangerousDeny DangerousStatementMode = "deny"
	// ModeDangerousInterrupt 挂起调用，等用户确认后再执行。
	//
	// 它要求编排层支持 checkpoint 与恢复（ToolsNode + CheckPointStore + 恢复入口）；
	// 三者缺一时调用方只会拿到一个无人处理的打断信号，因此默认用 ModeDangerousDeny。
	ModeDangerousInterrupt DangerousStatementMode = "interrupt"
)

// deniedResult 是危险语句未执行时返回给模型的结果。
//
// 这里返回正常结果而不是 error，是因为 compose 的 ToolsNode 会把工具返回的 error
// 升级为整图失败，模型拿不到任何解释；返回结果才能让模型向用户说明「未执行」。
type deniedResult struct {
	Canceled bool   `json:"canceled"`
	Message  string `json:"message"`
}

// destructiveInterruptInfo 是挂起时交给编排层、最终展示给用户的信息。
// 只带语句本身、不带 DSN：DSN 含密码，不应出现在用户确认界面或日志里。
type destructiveInterruptInfo struct {
	Tool      string `json:"tool"`
	Statement string `json:"statement"`
	Message   string `json:"message"`
}

// NewMysqlExecTool 创建写入工具 mysql_exec，危险语句的处理方式取自配置。
func NewMysqlExecTool(ctx context.Context) (tool.InvokableTool, error) {
	cfg, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}
	return newMysqlExecTool(openMySQL, parseDangerousMode(cfg.Mysql.DangerousStatementMode))
}

// parseDangerousMode 解析配置值；无法识别时退回 deny，宁可拒绝也不误挂起。
func parseDangerousMode(mode string) DangerousStatementMode {
	if DangerousStatementMode(mode) == ModeDangerousInterrupt {
		return ModeDangerousInterrupt
	}
	return ModeDangerousDeny
}

// newMysqlExecTool 基于注入的连接构造函数与危险语句模式建工具，便于测试替换掉真实 MySQL。
func newMysqlExecTool(open openMySQLFunc, mode DangerousStatementMode) (tool.InvokableTool, error) {
	handler := func(ctx context.Context, input *ExecInput) (string, error) {
		if err := validateInput(input.DSN, input.SQL); err != nil {
			return "", err
		}

		switch classifyStatement(input.SQL) {
		case stmtWrite:
			return runStatement(ctx, open, input)
		case stmtDestructive:
			return confirmDestructive(ctx, open, mode, input)
		case stmtRead:
			return "", fmt.Errorf("mysql_exec 用于写入或 DDL，查询语句请改用 mysql_query")
		default:
			return "", fmt.Errorf("无法识别的 SQL 语句：mysql_exec 只接受 INSERT/UPDATE/DELETE/CREATE/ALTER 等写入或 DDL 语句")
		}
	}

	t, err := utils.InferTool("mysql_exec", toolDescMysqlExec, handler)
	if err != nil {
		return nil, fmt.Errorf("创建 mysql_exec 工具失败: %w", err)
	}
	return t, nil
}

// runStatement 建立连接并执行语句。
func runStatement(ctx context.Context, open openMySQLFunc, input *ExecInput) (string, error) {
	db, release, err := open(input.DSN)
	if err != nil {
		return "", err
	}
	defer release()

	return execSQL(ctx, db, input.SQL)
}

// confirmDestructive 处理 DROP/TRUNCATE：deny 模式直接拒绝，interrupt 模式挂起等用户确认。
//
// 用不带 state 的 tool.Interrupt：待确认的 sql 与 dsn 本来就在 tool call 参数里随
// checkpoint 持久化，再用 state 存一遍只会让含密码的 DSN 多落一处。
func confirmDestructive(ctx context.Context, open openMySQLFunc, mode DangerousStatementMode, input *ExecInput) (string, error) {
	keyword := firstKeyword(input.SQL)
	if mode != ModeDangerousInterrupt {
		return denyResult(fmt.Sprintf("%s 属于危险操作，未执行；如需继续，请先与用户确认后重试", keyword))
	}

	info := destructiveInterruptInfo{
		Tool:      "mysql_exec",
		Statement: input.SQL,
		Message:   fmt.Sprintf("即将执行危险语句 %s，需用户确认后才会执行", keyword),
	}

	// 首次执行：挂起并请编排层打 checkpoint。
	if wasInterrupted, _, _ := tool.GetInterruptState[any](ctx); !wasInterrupted {
		return "", tool.Interrupt(ctx, info)
	}

	// 恢复执行：只有被显式指定为恢复目标才继续，否则重新挂起，
	// 避免同批次其它工具被恢复时误执行这条危险语句。
	isTarget, hasData, confirmed := tool.GetResumeContext[bool](ctx)
	if !isTarget {
		return "", tool.Interrupt(ctx, info)
	}
	if !hasData || !confirmed {
		return denyResult(fmt.Sprintf("用户未确认，已取消执行 %s", keyword))
	}

	return runStatement(ctx, open, input)
}

// denyResult 生成「未执行」结果。
func denyResult(message string) (string, error) {
	return marshalJSON(deniedResult{Canceled: true, Message: message})
}
