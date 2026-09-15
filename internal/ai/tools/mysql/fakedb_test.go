package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"sync"
	"testing"

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

// Begin 不支持事务。
func (c *fakeConn) Begin() (driver.Tx, error) { return nil, driver.ErrSkip }

// QueryContext 记录并返回 fixture 声明的结果集。
func (c *fakeConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.fx.recordQuery(query)
	if c.fx.queryErr != nil {
		return nil, c.fx.queryErr
	}
	return &fakeRows{columns: c.fx.columns, rows: c.fx.rows}, nil
}

// ExecContext 记录并返回 fixture 声明的受影响行数。
func (c *fakeConn) ExecContext(_ context.Context, statement string, _ []driver.NamedValue) (driver.Result, error) {
	c.fx.recordExec(statement)
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

// newFakeOpen 注册 fixture 并返回注入 fake 连接的 openMySQLFunc，用例结束自动清理。
func newFakeOpen(t *testing.T, dsn string, fx *fakeFixture) openMySQLFunc {
	t.Helper()

	fakeDriverOnce.Do(func() { sql.Register(fakeDriverName, fakeDriver{}) })
	fakeRegistry.Store(dsn, fx)
	t.Cleanup(func() { fakeRegistry.Delete(dsn) })

	return func(gotDSN string) (*gorm.DB, func(), error) {
		if gotDSN != dsn {
			return nil, nil, fmt.Errorf("fake open: 意外 dsn %q", gotDSN)
		}

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

		return db, func() { _ = sqlDB.Close() }, nil
	}
}
