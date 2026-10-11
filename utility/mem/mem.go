// Package mem 提供基于内存的会话记忆实现。
//
// 每个会话维护一份消息切片，并以滑动窗口方式裁剪历史。所有导出方法并发安全。
// 模型与裁剪规则详见 dev-docs/session-memory.md。
package mem

import (
	"context"
	"log"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/schema"
)

const (
	// DefaultMaxWindowSize 是会话默认窗口大小（消息条数）的初始值，约 4 轮对话。
	DefaultMaxWindowSize = 8
	// DefaultSessionTTL 是会话默认的空闲回收时间。
	DefaultSessionTTL = 30 * time.Minute
	// DefaultCleanupInterval 是后台清理会话的默认周期。
	DefaultCleanupInterval = 5 * time.Minute
)

// 全局会话表，由 mu 保护。
var (
	simpleMemoryMap = make(map[string]*SimpleMemory)
	mu              sync.RWMutex
)

// defaultWindowSize 是当前生效的默认窗口大小，可经 SetDefaultWindowSize 覆盖。
var defaultWindowSize atomic.Int64

func init() {
	defaultWindowSize.Store(DefaultMaxWindowSize)
}

// SetDefaultWindowSize 设置新建会话的默认窗口大小（消息条数），n <= 0 时忽略。
// 由 main 在启动时按 memory.max_window_size 调用；并发调用安全。
func SetDefaultWindowSize(n int) {
	if n > 0 {
		defaultWindowSize.Store(int64(n))
	}
}

// DefaultWindowSize 返回当前生效的默认窗口大小。
func DefaultWindowSize() int {
	return int(defaultWindowSize.Load())
}

// SimpleMemory 表示一个会话的滑动窗口记忆，字段非导出以保证并发安全。
type SimpleMemory struct {
	id            string
	messages      []*schema.Message
	maxWindowSize int
	mu            sync.RWMutex
	// lastActive 最近一次使用时间（UnixNano），用原子读写以支持无锁的 GC 扫描。
	lastActive atomic.Int64
}

// NewSimpleMemory 创建指定 ID 的会话记忆。
// maxWindowSize 为窗口可保留的最大消息条数，小于等于 0 时使用当前默认窗口大小（DefaultWindowSize）。
func NewSimpleMemory(id string, maxWindowSize int) *SimpleMemory {
	if maxWindowSize <= 0 {
		maxWindowSize = DefaultWindowSize()
	}
	m := &SimpleMemory{
		id:            id,
		messages:      make([]*schema.Message, 0, maxWindowSize),
		maxWindowSize: maxWindowSize,
	}
	m.touch()
	return m
}

// GetSimpleMemory 按 ID 获取会话记忆，不存在时以默认窗口大小创建。
func GetSimpleMemory(id string) *SimpleMemory {
	mu.RLock()
	m, ok := simpleMemoryMap[id]
	mu.RUnlock()
	if ok {
		m.touch()
		return m
	}

	// 未命中时升级为写锁并二次确认，避免并发下重复创建。
	mu.Lock()
	defer mu.Unlock()
	if m, ok = simpleMemoryMap[id]; ok {
		m.touch()
		return m
	}
	m = NewSimpleMemory(id, DefaultWindowSize())
	simpleMemoryMap[id] = m
	return m
}

// DeleteSimpleMemory 删除指定会话记忆，返回是否删除成功。
func DeleteSimpleMemory(id string) bool {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := simpleMemoryMap[id]; !ok {
		return false
	}
	delete(simpleMemoryMap, id)
	return true
}

// ID 返回会话 ID。
func (c *SimpleMemory) ID() string {
	return c.id
}

// Len 返回当前保存的消息条数。
func (c *SimpleMemory) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.messages)
}

// MaxWindowSize 返回最大窗口大小。
func (c *SimpleMemory) MaxWindowSize() int {
	return c.maxWindowSize
}

// SetMessage 追加一条消息，并按滑动窗口裁剪历史。
func (c *SimpleMemory) SetMessage(msg *schema.Message) {
	c.touch()

	c.mu.Lock()
	defer c.mu.Unlock()

	c.messages = append(c.messages, msg)
	c.messages = trimMessages(c.messages, c.maxWindowSize)
}

// GetMessages 返回当前消息切片的副本，调用方修改不会影响内部状态。
func (c *SimpleMemory) GetMessages() []*schema.Message {
	c.touch()

	c.mu.RLock()
	defer c.mu.RUnlock()
	return slices.Clone(c.messages)
}

// touch 刷新会话的最近使用时间，供空闲回收判定。
func (c *SimpleMemory) touch() {
	c.lastActive.Store(time.Now().UnixNano())
}

// SessionCount 返回当前活跃会话数（仅用于观测与测试）。
func SessionCount() int {
	mu.RLock()
	defer mu.RUnlock()
	return len(simpleMemoryMap)
}

// StartSessionGC 启动后台清理 goroutine：每 interval 回收一次空闲超过 ttl 的会话，
// ctx 取消时退出。ttl 或 interval 非正时不启动。调用方保证只调用一次。
func StartSessionGC(ctx context.Context, ttl, interval time.Duration) {
	if ttl <= 0 || interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n := evictIdle(ttl, time.Now()); n > 0 {
					log.Printf("[mem] 回收空闲会话 %d 个（TTL %s）", n, ttl)
				}
			}
		}
	}()
}

// evictIdle 回收 lastActive 早于 now-ttl 的会话，返回回收数量。
func evictIdle(ttl time.Duration, now time.Time) int {
	if ttl <= 0 {
		return 0
	}
	deadline := now.Add(-ttl).UnixNano()

	mu.Lock()
	defer mu.Unlock()
	evicted := 0
	for id, m := range simpleMemoryMap {
		if m.lastActive.Load() < deadline {
			delete(simpleMemoryMap, id)
			evicted++
		}
	}
	return evicted
}

// trimMessages 按滑动窗口裁剪消息：保留首条 system 消息（占用窗口配额），
// 其余取最近 maxWindowSize 条，并剥离开头的非 user 消息。规则详见 dev-docs/session-memory.md。
func trimMessages(msgs []*schema.Message, maxWindowSize int) []*schema.Message {
	if maxWindowSize <= 0 {
		return nil
	}
	if len(msgs) <= maxWindowSize {
		return msgs
	}

	head := 0
	if msgs[0] != nil && msgs[0].Role == schema.System {
		head = 1
	}
	body := msgs[head:]
	bodyBudget := max(maxWindowSize-head, 0)

	if len(body) > bodyBudget {
		body = body[len(body)-bodyBudget:]
	}
	// 循环剥离而非只剥一条：tool-call 链会产生连续多条 assistant/tool 消息，
	// 只剥一条仍会留下孤立的 tool 消息，破坏对话配对关系。
	for len(body) > 0 && (body[0] == nil || body[0].Role != schema.User) {
		body = body[1:]
	}

	result := make([]*schema.Message, 0, head+len(body))
	result = append(result, msgs[:head]...)
	result = append(result, body...)

	return result
}
