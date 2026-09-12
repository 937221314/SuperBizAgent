package mem

import (
	"fmt"
	"sync"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func userMsg(content string) *schema.Message { return schema.UserMessage(content) }
func sysMsg(content string) *schema.Message  { return schema.SystemMessage(content) }
func asstMsg(content string) *schema.Message { return schema.AssistantMessage(content, nil) }
func toolMsg(content string) *schema.Message { return schema.ToolMessage(content, "call-1") }

func contents(msgs []*schema.Message) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		if m == nil {
			out = append(out, "<nil>")
			continue
		}
		out = append(out, string(m.Role)+":"+m.Content)
	}
	return out
}

func assertContents(t *testing.T, got []*schema.Message, want []string) {
	t.Helper()
	gotS := contents(got)
	if len(gotS) != len(want) {
		t.Fatalf("消息数量不符: got %v, want %v", gotS, want)
	}
	for i := range want {
		if gotS[i] != want[i] {
			t.Fatalf("消息内容不符: got %v, want %v", gotS, want)
		}
	}
}

func TestNewSimpleMemoryInvalidWindowSize(t *testing.T) {
	for _, size := range []int{0, -1} {
		m := NewSimpleMemory("s", size)
		if m.MaxWindowSize() != DefaultMaxWindowSize {
			t.Fatalf("窗口大小 %d 应回退为 %d, got %d", size, DefaultMaxWindowSize, m.MaxWindowSize())
		}
	}
}

func TestTrimMessagesExactWindow(t *testing.T) {
	msgs := []*schema.Message{userMsg("1"), asstMsg("2"), userMsg("3")}
	got := trimMessages(msgs, 3)
	assertContents(t, got, []string{"user:1", "assistant:2", "user:3"})
}

func TestTrimMessagesKeepsSystem(t *testing.T) {
	msgs := []*schema.Message{
		sysMsg("sys"),
		userMsg("1"), asstMsg("2"),
		userMsg("3"), asstMsg("4"),
		userMsg("5"),
	}
	got := trimMessages(msgs, 4)
	// system 占 1 个配额，其余保留最近 3 条
	assertContents(t, got, []string{"system:sys", "user:3", "assistant:4", "user:5"})
}

func TestTrimMessagesStartsWithUser(t *testing.T) {
	// 裁剪后首条为 assistant，应继续丢弃直到以 user 开头
	msgs := []*schema.Message{
		userMsg("0"), asstMsg("1"), userMsg("2"), asstMsg("3"),
	}
	got := trimMessages(msgs, 3)
	assertContents(t, got, []string{"user:2", "assistant:3"})
}

func TestTrimMessagesStripsConsecutiveNonUser(t *testing.T) {
	// 开头的连续 assistant/tool 消息应被“全部”剥离，而非只剥一条
	msgs := []*schema.Message{
		asstMsg("a1"), asstMsg("a2"), toolMsg("t1"), userMsg("u1"), asstMsg("a3"),
	}
	got := trimMessages(msgs, 4)
	assertContents(t, got, []string{"user:u1", "assistant:a3"})
}

func TestTrimMessagesStripsLeadingNil(t *testing.T) {
	// 开头的 nil 消息同样应被剥离，结果首条必须是真实 user 消息
	msgs := []*schema.Message{
		nil, nil, userMsg("u1"), asstMsg("a2"), userMsg("u2"),
	}
	got := trimMessages(msgs, 4)
	assertContents(t, got, []string{"user:u1", "assistant:a2", "user:u2"})
}

func TestTrimMessagesAllNonUser(t *testing.T) {
	// 全部为非 user 消息时应被剥空，仅保留 system
	msgs := []*schema.Message{
		sysMsg("sys"), asstMsg("a1"), toolMsg("t1"), asstMsg("a2"),
	}
	got := trimMessages(msgs, 3)
	assertContents(t, got, []string{"system:sys"})
}

func TestTrimMessagesZeroWindow(t *testing.T) {
	if got := trimMessages([]*schema.Message{userMsg("1")}, 0); got != nil {
		t.Fatalf("窗口为 0 应返回 nil, got %v", contents(got))
	}
}

func TestSetMessageSlidingWindow(t *testing.T) {
	m := NewSimpleMemory("s1", 4)
	m.SetMessage(sysMsg("sys"))
	m.SetMessage(userMsg("1"))
	m.SetMessage(asstMsg("2"))
	m.SetMessage(userMsg("3"))
	m.SetMessage(asstMsg("4"))
	m.SetMessage(userMsg("5"))

	got := m.GetMessages()
	if len(got) > 4 {
		t.Fatalf("窗口溢出: len=%d", len(got))
	}
	if got[0].Role != schema.System {
		t.Fatalf("system 消息应被保留, got %v", contents(got))
	}
	if got[len(got)-1].Content != "5" {
		t.Fatalf("最新消息应在末尾, got %v", contents(got))
	}
}

func TestGetMessagesReturnsCopy(t *testing.T) {
	m := NewSimpleMemory("s2", 4)
	m.SetMessage(userMsg("1"))

	got := m.GetMessages()
	got[0] = asstMsg("hacked")

	fresh := m.GetMessages()
	if fresh[0].Content != "1" {
		t.Fatalf("GetMessages 应返回副本, 内部状态被外部修改: %v", contents(fresh))
	}
}

func TestGetSimpleMemoryIdempotent(t *testing.T) {
	const id = "idempotent"
	DeleteSimpleMemory(id)
	defer DeleteSimpleMemory(id)

	a := GetSimpleMemory(id)
	b := GetSimpleMemory(id)
	if a != b {
		t.Fatal("相同 ID 应返回同一实例")
	}
	if !DeleteSimpleMemory(id) {
		t.Fatal("删除已存在的会话应返回 true")
	}
	if DeleteSimpleMemory(id) {
		t.Fatal("删除不存在的会话应返回 false")
	}
}

func TestConcurrentAccess(t *testing.T) {
	m := NewSimpleMemory("concurrent", 8)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				m.SetMessage(userMsg(fmt.Sprintf("u-%d-%d", n, j)))
				m.SetMessage(asstMsg(fmt.Sprintf("a-%d-%d", n, j)))
				_ = m.GetMessages()
				_ = m.Len()
			}
		}(i)
	}
	wg.Wait()

	if m.Len() > m.MaxWindowSize() {
		t.Fatalf("并发写入后窗口溢出: len=%d", m.Len())
	}
}
