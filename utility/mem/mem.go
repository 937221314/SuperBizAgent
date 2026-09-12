// Package mem 提供基于内存的会话记忆实现。
//
// 每个会话维护一份消息切片，并以滑动窗口方式裁剪历史。所有导出方法并发安全。
// 模型与裁剪规则详见 dev-docs/session-memory.md。
package mem

import (
	"slices"
	"sync"

	"github.com/cloudwego/eino/schema"
)

// DefaultMaxWindowSize 是会话默认的最大窗口大小（消息条数）。
const DefaultMaxWindowSize = 6

// 全局会话表，由 mu 保护。
var (
	simpleMemoryMap = make(map[string]*SimpleMemory)
	mu              sync.RWMutex
)

// SimpleMemory 表示一个会话的滑动窗口记忆，字段非导出以保证并发安全。
type SimpleMemory struct {
	id            string
	messages      []*schema.Message
	maxWindowSize int
	mu            sync.RWMutex
}

// NewSimpleMemory 创建指定 ID 的会话记忆。
// maxWindowSize 为窗口可保留的最大消息条数，小于等于 0 时使用 DefaultMaxWindowSize。
func NewSimpleMemory(id string, maxWindowSize int) *SimpleMemory {
	if maxWindowSize <= 0 {
		maxWindowSize = DefaultMaxWindowSize
	}
	return &SimpleMemory{
		id:            id,
		messages:      make([]*schema.Message, 0, maxWindowSize),
		maxWindowSize: maxWindowSize,
	}
}

// GetSimpleMemory 按 ID 获取会话记忆，不存在时以默认窗口大小创建。
func GetSimpleMemory(id string) *SimpleMemory {
	mu.RLock()
	m, ok := simpleMemoryMap[id]
	mu.RUnlock()
	if ok {
		return m
	}

	// 未命中时升级为写锁并二次确认，避免并发下重复创建。
	mu.Lock()
	defer mu.Unlock()
	if m, ok = simpleMemoryMap[id]; ok {
		return m
	}
	m = NewSimpleMemory(id, DefaultMaxWindowSize)
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
	c.mu.Lock()
	defer c.mu.Unlock()

	c.messages = append(c.messages, msg)
	c.messages = trimMessages(c.messages, c.maxWindowSize)
}

// GetMessages 返回当前消息切片的副本，调用方修改不会影响内部状态。
func (c *SimpleMemory) GetMessages() []*schema.Message {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return slices.Clone(c.messages)
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
