package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// fakeDriverName 是测试驱动名，全局只注册一次。
const fakeDriverName = "tools_mysql_fake"

// fakeFixture 描述一次测试期望的驱动行为，并记录实际下发的语句，
// 以便断言被拒绝的语句根本没有到达数据库。
type fakeFixture struct {
	columns      []string
	rows         [][]driver.Value
	queryErr     error
	execErr      error
	rowsAffected int64

	mu       sync.Mutex
	queried  []string
	executed []string

	readOnlyTx bool

	// blockOnCtx 让查询/写入挂到 ctx 结束，用于验证超时与截止时间透传。
	blockOnCtx bool
	// ctxErrors 记录驱动看到的 ctx 错误。
	ctxErrors []error
	// deadlineLeft 记录调用驱动时距离 ctx 截止还剩多久。
	deadlineLeft time.Duration
}

// recordQuery 记录一次查询。
func (f *fakeFixture) recordQuery(query string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queried = append(f.queried, query)
}

// recordExec 记录一次写入或 DDL。
func (f *fakeFixture) recordExec(statement string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.executed = append(f.executed, statement)
}

// queriedSQL 返回已下发查询的副本。
func (f *fakeFixture) queriedSQL() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.queried...)
}

// executedSQL 返回已下发写入的副本。
func (f *fakeFixture) executedSQL() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.executed...)
}

// markReadOnlyTx 记录一次只读事务开启。
func (f *fakeFixture) markReadOnlyTx() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readOnlyTx = true
}

// usedReadOnlyTx 返回是否开过只读事务。
func (f *fakeFixture) usedReadOnlyTx() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.readOnlyTx
}

// recordCtx 记录驱动看到的 ctx 截止时间（无截止时间时记 0）。
func (f *fakeFixture) recordCtx(ctx context.Context) {
	f.mu.Lock()
	defer f.mu.Unlock()

	deadline, ok := ctx.Deadline()
	if !ok {
		f.deadlineLeft = 0
		return
	}
	f.deadlineLeft = time.Until(deadline)
}

// waitCtx 在 blockOnCtx 时挂到 ctx 结束，模拟慢查询；否则立即返回。
func (f *fakeFixture) waitCtx(ctx context.Context) error {
	f.recordCtx(ctx)
	if !f.blockOnCtx {
		return nil
	}

	<-ctx.Done()
	err := ctx.Err()

	f.mu.Lock()
	defer f.mu.Unlock()
	f.ctxErrors = append(f.ctxErrors, err)
	return err
}

// deadlineLeftOf 返回驱动侧看到的截止时间剩余量（0 表示没有截止时间）。
func (f *fakeFixture) deadlineLeftOf() time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deadlineLeft
}

var (
	// fakeRegistry 把 DSN 映射到 fixture，使 fakeDriver.Open 能取到当前用例的期望行为。
	fakeRegistry sync.Map

	// fakeDriverOnce 保证驱动只注册一次，重复 sql.Register 会 panic。
	fakeDriverOnce sync.Once
)

// fakeDriver 是最小 database/sql 驱动，避免为测试引入额外依赖。
type fakeDriver struct{}

// Open 按 DSN 取回用例注册的 fixture。
func (fakeDriver) Open(dsn string) (driver.Conn, error) {
	v, ok := fakeRegistry.Load(dsn)
	if !ok {
		return nil, fmt.Errorf("fake driver: dsn %q 未注册 fixture", dsn)
	}
	return &fakeConn{fx: v.(*fakeFixture)}, nil
}

// fakeConn 只支持无参数路径：工具里的 Raw(...).Rows() 与 Exec(statement) 都不会经过 Prepare。
type fakeConn struct{ fx *fakeFixture }

// Prepare 不会被调用，返回 ErrSkip 即可。
func (c *fakeConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }

// Close 无需释放资源。
func (c *fakeConn) Close() error { return nil }

// Begin 不会被调用（gorm 总是传 TxOptions，走 BeginTx）。
func (c *fakeConn) Begin() (driver.Tx, error) { return nil, driver.ErrSkip }

// BeginTx 支持只读事务，便于断言 mysql_query 走的确实是只读事务。
func (c *fakeConn) BeginTx(_ context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if opts.ReadOnly {
		c.fx.markReadOnlyTx()
	}
	return fakeTx{}, nil
}

// fakeTx 是空实现的事务：回滚与提交对 fake 驱动没有区别。
type fakeTx struct{}

// Commit 提交事务。
func (fakeTx) Commit() error { return nil }

// Rollback 回滚事务。
func (fakeTx) Rollback() error { return nil }

// QueryContext 记录并返回 fixture 声明的结果集。
func (c *fakeConn) QueryContext(ctx context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.fx.recordQuery(query)
	if err := c.fx.waitCtx(ctx); err != nil {
		return nil, err
	}
	if c.fx.queryErr != nil {
		return nil, c.fx.queryErr
	}
	return &fakeRows{columns: c.fx.columns, rows: c.fx.rows}, nil
}

// ExecContext 记录并返回 fixture 声明的受影响行数。
func (c *fakeConn) ExecContext(ctx context.Context, statement string, _ []driver.NamedValue) (driver.Result, error) {
	c.fx.recordExec(statement)
	if err := c.fx.waitCtx(ctx); err != nil {
		return nil, err
	}
	if c.fx.execErr != nil {
		return nil, c.fx.execErr
	}
	return driver.RowsAffected(c.fx.rowsAffected), nil
}

// fakeRows 按 fixture 声明的列与行逐个吐出数据。
type fakeRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

// Columns 返回 fixture 声明的列名。
func (r *fakeRows) Columns() []string { return r.columns }

// Close 无需释放资源。
func (r *fakeRows) Close() error { return nil }

// Next 逐行填充 dest，行末返回 io.EOF。
func (r *fakeRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}

	row := r.rows[r.index]
	r.index++
	if len(row) != len(dest) {
		return fmt.Errorf("fake rows: 第 %d 行列数 %d 与声明的 %d 不一致", r.index, len(row), len(dest))
	}
	copy(dest, row)
	return nil
}

// ensureFakeDriver 注册 fake 驱动，全局只注册一次。
func ensureFakeDriver() {
	fakeDriverOnce.Do(func() { sql.Register(fakeDriverName, fakeDriver{}) })
}

// openFakeDB 用 fake 驱动建一个 gorm 连接，同时返回底层 *sql.DB 供调用方关闭。
func openFakeDB(dsn string) (*gorm.DB, *sql.DB, error) {
	sqlDB, err := sql.Open(fakeDriverName, dsn)
	if err != nil {
		return nil, nil, err
	}

	// SkipInitializeWithVersion 必须为 true，否则 gorm 会去查 SELECT VERSION()。
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = sqlDB.Close()
		return nil, nil, err
	}
	return db, sqlDB, nil
}

// newFakeOpen 注册 fixture 并返回注入 fake 连接的 openMySQLFunc，用例结束自动清理。
// 同一 DSN 只建一个连接，与生产的连接池缓存语义一致。
func newFakeOpen(t *testing.T, dsn string, fx *fakeFixture) openMySQLFunc {
	t.Helper()

	ensureFakeDriver()
	fakeRegistry.Store(dsn, fx)

	var (
		once    sync.Once
		db      *gorm.DB
		sqlDB   *sql.DB
		openErr error
	)
	t.Cleanup(func() {
		fakeRegistry.Delete(dsn)
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	return func(gotDSN string) (*gorm.DB, error) {
		if gotDSN != dsn {
			return nil, fmt.Errorf("fake open: 意外 dsn %q", gotDSN)
		}
		once.Do(func() { db, sqlDB, openErr = openFakeDB(dsn) })
		return db, openErr
	}
}
