package mysql

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 连接池容量参数。
const (
	maxOpenConns    = 8
	maxIdleConns    = 4
	connMaxIdleTime = 5 * time.Minute
	connMaxLifetime = 30 * time.Minute
)

// 缓存规模参数。
//
// DSN 由模型提供、每次调用都可能不同，所以缓存必须有界，否则等于给模型一个内存增长入口。
// poolIdleTimeout 必须远大于 statementTimeout：空闲回收会关闭连接池，
// 阈值贴近查询时长就可能把在途查询的连接池关掉。
const (
	maxCachedPools  = 8
	poolIdleTimeout = 5 * time.Minute
)

// dialMySQLFunc 建立连接池，同时返回底层 *sql.DB 供淘汰时关闭。
type dialMySQLFunc func(dsn string) (db *gorm.DB, sqlDB *sql.DB, err error)

// dialMySQL 用原生 DSN 建立 gorm 连接池；SQL 日志置为静默，避免污染模型上下文。
func dialMySQL(dsn string) (*gorm.DB, *sql.DB, error) {
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
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetConnMaxIdleTime(connMaxIdleTime)
	sqlDB.SetConnMaxLifetime(connMaxLifetime)

	return db, sqlDB, nil
}

// poolEntry 是一个已缓存的连接池。
type poolEntry struct {
	db       *gorm.DB
	sqlDB    *sql.DB
	lastUsed time.Time
}

// mysqlPool 是按 DSN 复用连接池的有界缓存。
// dial 与 now 可注入，便于测试替换建连实现与控制时间。
type mysqlPool struct {
	mu       sync.Mutex
	entries  map[string]*poolEntry
	dial     dialMySQLFunc
	maxPools int
	maxIdle  time.Duration
	now      func() time.Time
}

// defaultPool 是工具使用的全局缓存。
var defaultPool = newMysqlPool(dialMySQL)

// newMysqlPool 创建连接池缓存。
func newMysqlPool(dial dialMySQLFunc) *mysqlPool {
	return &mysqlPool{
		entries:  make(map[string]*poolEntry),
		dial:     dial,
		maxPools: maxCachedPools,
		maxIdle:  poolIdleTimeout,
		now:      time.Now,
	}
}

// acquire 取出 DSN 对应的连接池，未命中则建连。
//
// 持有锁期间完成建连：牺牲不同 DSN 的并发建连，换取「同一 DSN 不会重复建池」。
// 缓存淘汰（空闲回收 + LRU）在这里顺带做，不为它单独起后台 goroutine。
func (p *mysqlPool) acquire(dsn string) (*gorm.DB, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.now()
	if entry, ok := p.entries[dsn]; ok {
		entry.lastUsed = now
		return entry.db, nil
	}

	p.evictIdleLocked(now)
	for len(p.entries) > 0 && len(p.entries) >= p.maxPools {
		p.evictOldestLocked()
	}

	db, sqlDB, err := p.dial(dsn)
	if err != nil {
		return nil, err
	}
	p.entries[dsn] = &poolEntry{db: db, sqlDB: sqlDB, lastUsed: now}
	return db, nil
}

// evictIdleLocked 回收长期空闲的连接池。
func (p *mysqlPool) evictIdleLocked(now time.Time) {
	for dsn, entry := range p.entries {
		if now.Sub(entry.lastUsed) > p.maxIdle {
			entry.close()
			delete(p.entries, dsn)
		}
	}
}

// evictOldestLocked 淘汰最久未使用的连接池。
func (p *mysqlPool) evictOldestLocked() {
	var oldestDSN string
	var oldestTime time.Time
	for dsn, entry := range p.entries {
		if oldestDSN == "" || entry.lastUsed.Before(oldestTime) {
			oldestDSN, oldestTime = dsn, entry.lastUsed
		}
	}
	if oldestDSN == "" {
		return
	}

	p.entries[oldestDSN].close()
	delete(p.entries, oldestDSN)
}

// close 关闭连接池；在途连接会在归还时一并关闭。
func (e *poolEntry) close() {
	if e.sqlDB != nil {
		_ = e.sqlDB.Close()
	}
}
