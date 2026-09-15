package mysql

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

// poolFixture 记录 fake 建连次数，并保留返回的 *sql.DB 以便断言是否被回收。
type poolFixture struct {
	dialed  []string
	sqlDBs  map[string]*sql.DB
	dialErr error
}

// dial 是注入到 mysqlPool 的建连实现。
func (f *poolFixture) dial(dsn string) (*gorm.DB, *sql.DB, error) {
	f.dialed = append(f.dialed, dsn)
	if f.dialErr != nil {
		return nil, nil, f.dialErr
	}

	fakeRegistry.Store(dsn, &fakeFixture{columns: []string{"id"}})
	db, sqlDB, err := openFakeDB(dsn)
	if err != nil {
		return nil, nil, err
	}
	f.sqlDBs[dsn] = sqlDB
	return db, sqlDB, nil
}

// newPoolFixture 创建 fake 建连实现并注册驱动，用例结束清理 fixture 注册项。
func newPoolFixture(t *testing.T) *poolFixture {
	t.Helper()

	ensureFakeDriver()
	return &poolFixture{sqlDBs: make(map[string]*sql.DB)}
}

// newTestPool 创建注入 fake 建连与可控时钟的连接池。
func newTestPool(fx *poolFixture, now func() time.Time) *mysqlPool {
	p := newMysqlPool(fx.dial)
	p.now = now
	return p
}

// isClosed 判断连接池底层 *sql.DB 是否已关闭。
func isClosed(t *testing.T, sqlDB *sql.DB) bool {
	t.Helper()

	err := sqlDB.Ping()
	return err != nil && strings.Contains(err.Error(), "closed")
}

// TestPoolReusesConnection 校验同一 DSN 只建一次连接池。
func TestPoolReusesConnection(t *testing.T) {
	fx := newPoolFixture(t)
	pool := newTestPool(fx, time.Now)

	first, err := pool.acquire("dsn-a")
	if err != nil {
		t.Fatalf("首次取连接失败: %v", err)
	}
	second, err := pool.acquire("dsn-a")
	if err != nil {
		t.Fatalf("再次取连接失败: %v", err)
	}

	if first != second {
		t.Errorf("同一 DSN 应复用同一个 gorm.DB")
	}
	if len(fx.dialed) != 1 {
		t.Errorf("期望只建连 1 次，实际 %d 次: %v", len(fx.dialed), fx.dialed)
	}
}

// TestPoolEvictsLeastRecentlyUsed 校验缓存满时淘汰最久未使用的连接池并关闭它。
func TestPoolEvictsLeastRecentlyUsed(t *testing.T) {
	fx := newPoolFixture(t)
	pool := newTestPool(fx, time.Now)
	pool.maxPools = 2

	for _, dsn := range []string{"dsn-a", "dsn-b"} {
		if _, err := pool.acquire(dsn); err != nil {
			t.Fatalf("取连接失败: %v", err)
		}
	}
	// 再取一次 dsn-a，让它成为最近使用，dsn-b 变成最久未使用。
	if _, err := pool.acquire("dsn-a"); err != nil {
		t.Fatalf("取连接失败: %v", err)
	}
	if _, err := pool.acquire("dsn-c"); err != nil {
		t.Fatalf("取连接失败: %v", err)
	}

	if !isClosed(t, fx.sqlDBs["dsn-b"]) {
		t.Errorf("dsn-b 应被淘汰并关闭")
	}
	if isClosed(t, fx.sqlDBs["dsn-a"]) {
		t.Errorf("dsn-a 刚被使用，不应被淘汰")
	}
	if isClosed(t, fx.sqlDBs["dsn-c"]) {
		t.Errorf("dsn-c 刚建连，不应被淘汰")
	}
}

// TestPoolEvictsIdle 校验长期空闲的连接池会被回收。
func TestPoolEvictsIdle(t *testing.T) {
	fx := newPoolFixture(t)

	current := time.Now()
	pool := newTestPool(fx, func() time.Time { return current })
	pool.maxIdle = time.Minute

	if _, err := pool.acquire("dsn-a"); err != nil {
		t.Fatalf("取连接失败: %v", err)
	}

	current = current.Add(2 * time.Minute)
	if _, err := pool.acquire("dsn-b"); err != nil {
		t.Fatalf("取连接失败: %v", err)
	}

	if !isClosed(t, fx.sqlDBs["dsn-a"]) {
		t.Errorf("空闲超时的 dsn-a 应被回收")
	}
	if isClosed(t, fx.sqlDBs["dsn-b"]) {
		t.Errorf("dsn-b 刚建连，不应被回收")
	}
}

// TestPoolDialErrorNotCached 校验建连失败不写入缓存，下次调用可重试。
func TestPoolDialErrorNotCached(t *testing.T) {
	fx := newPoolFixture(t)
	sentinel := errors.New("连不上")
	fx.dialErr = sentinel

	pool := newTestPool(fx, time.Now)
	if _, err := pool.acquire("dsn-a"); !errors.Is(err, sentinel) {
		t.Fatalf("期望原错误上抛，实际: %v", err)
	}

	fx.dialErr = nil
	if _, err := pool.acquire("dsn-a"); err != nil {
		t.Fatalf("建连失败后应可重试，实际: %v", err)
	}
	if len(fx.dialed) != 2 {
		t.Errorf("期望重试时再次建连，实际建连 %d 次", len(fx.dialed))
	}
}
