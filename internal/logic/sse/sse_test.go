package sse

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestBuildEvent(t *testing.T) {
	cases := []struct {
		name string
		msg  Message
		want string
	}{
		{
			name: "完整字段",
			msg:  Message{ID: "id-1", Event: "chat", Data: "hello"},
			want: "id: id-1\nevent: chat\ndata: hello\n\n",
		},
		{
			name: "省略 id 与 event",
			msg:  Message{Data: "hello"},
			want: "data: hello\n\n",
		},
		{
			name: "多行数据按行拆成多个 data 字段",
			msg:  Message{Data: "line1\nline2\r\nline3"},
			want: "data: line1\ndata: line2\ndata: line3\n\n",
		},
		{
			name: "空数据仍保留一个 data 字段",
			msg:  Message{},
			want: "data: \n\n",
		},
		{
			name: "id 与 event 中的换行被清除，避免帧注入",
			msg:  Message{ID: "a\nb", Event: "c\r\nd", Data: "x"},
			want: "id: ab\nevent: cd\ndata: x\n\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := buildEvent(c.msg); got != c.want {
				t.Fatalf("buildEvent() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestBuildRetry(t *testing.T) {
	if got, want := buildRetry(1500*time.Millisecond), "retry: 1500\n\n"; got != want {
		t.Fatalf("buildRetry() = %q, want %q", got, want)
	}
}

func TestConnectedDataEscapesClientID(t *testing.T) {
	// 客户端 ID 来自外部输入，必须经 JSON 转义，否则会破坏数据体结构。
	// 注意：Client 的 ID 已由 clientIDPattern 校验，这里是连接数据的独立兜底测试。
	raw := connectedData(`a"b\c`)

	var payload struct {
		Status   string `json:"status"`
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("connectedData() 不是合法 JSON: %v, raw=%s", err, raw)
	}
	if payload.Status != "connected" {
		t.Fatalf("status = %q, want connected", payload.Status)
	}
	if payload.ClientID != `a"b\c` {
		t.Fatalf("client_id = %q, want %q", payload.ClientID, `a"b\c`)
	}
}

func TestClientIDPattern(t *testing.T) {
	valid := []string{
		"a",
		"ABC-123_x",
		strings.Repeat("a", clientIDMaxLen),
		guidLikeID,
	}
	for _, id := range valid {
		if !clientIDPattern.MatchString(id) {
			t.Errorf("clientIDPattern 应接受 %q", id)
		}
	}

	invalid := []string{
		"",
		"a b",
		"a\"b",
		"a/b",
		"a\nb",
		"中文",
		strings.Repeat("a", clientIDMaxLen+1),
	}
	for _, id := range invalid {
		if clientIDPattern.MatchString(id) {
			t.Errorf("clientIDPattern 应拒绝 %q", id)
		}
	}
}

func TestSend(t *testing.T) {
	s := New()
	client := newFakeClient("c1", 1)
	s.clients.Set(client.id, client)

	// 目标不存在
	if err := s.Send("missing", Message{Data: "x"}); !errors.Is(err, ErrClientNotFound) {
		t.Fatalf("Send(未注册客户端) = %v, want ErrClientNotFound", err)
	}

	// 正常投递
	if err := s.Send("c1", Message{Data: "x"}); err != nil {
		t.Fatalf("Send() 首次投递失败: %v", err)
	}
	select {
	case msg := <-client.messageChan:
		if msg.Data != "x" {
			t.Fatalf("投递内容 = %q, want x", msg.Data)
		}
	default:
		t.Fatal("消息未进入客户端缓冲区")
	}

	// 缓冲区已满：非阻塞丢弃
	if err := s.Send("c1", Message{Data: "first"}); err != nil {
		t.Fatalf("Send() 填满缓冲区失败: %v", err)
	}
	if err := s.Send("c1", Message{Data: "second"}); !errors.Is(err, ErrBufferFull) {
		t.Fatalf("Send(缓冲区已满) = %v, want ErrBufferFull", err)
	}

	// 连接已关闭但尚未注销（例如 Run 正在退出）
	client.close()
	if err := s.Send("c1", Message{Data: "third"}); !errors.Is(err, ErrClientClosed) {
		t.Fatalf("Send(已关闭连接) = %v, want ErrClientClosed", err)
	}

	// 注销后变为「不存在」，且重复注销幂等
	if !s.RemoveClient("c1") {
		t.Fatal("RemoveClient() 应返回 true")
	}
	if s.RemoveClient("c1") {
		t.Fatal("重复 RemoveClient() 应返回 false")
	}
	if s.ClientCount() != 0 {
		t.Fatalf("ClientCount() = %d, want 0", s.ClientCount())
	}
}

func TestBroadcast(t *testing.T) {
	s := New()
	okClient := newFakeClient("ok", 1)
	fullClient := newFakeClient("full", 1)
	fullClient.messageChan <- Message{Data: "占满缓冲区"}
	s.clients.Set(okClient.id, okClient)
	s.clients.Set(fullClient.id, fullClient)

	// 只有 ok 客户端能收到，full 客户端失败不影响其它客户端
	if got := s.Broadcast(context.Background(), Message{Event: "notice", Data: "broadcast"}); got != 1 {
		t.Fatalf("Broadcast() = %d, want 1", got)
	}
	select {
	case msg := <-okClient.messageChan:
		if msg.Event != "notice" || msg.Data != "broadcast" {
			t.Fatalf("广播内容 = %+v", msg)
		}
	default:
		t.Fatal("广播消息未进入客户端缓冲区")
	}
}

func TestRunExitsOnRemoveClient(t *testing.T) {
	s := New()
	client := newFakeClient("c1", 1)
	s.clients.Set(client.id, client)

	exited := make(chan struct{})
	go func() {
		defer close(exited)
		s.Run(context.Background(), client)
	}()

	if !s.RemoveClient("c1") {
		t.Fatal("RemoveClient() 应返回 true")
	}
	select {
	case <-exited:
	case <-time.After(2 * time.Second):
		t.Fatal("Run 未随 RemoveClient 退出")
	}
	if s.ClientCount() != 0 {
		t.Fatalf("ClientCount() = %d, want 0", s.ClientCount())
	}
}

func TestRunNilClient(t *testing.T) {
	// 不应 panic。
	New().Run(context.Background(), nil)
}

// guidLikeID 模拟 guid.S() 的形态（32 位小写字母与数字），用于确认服务端
// 生成的 ID 同样满足 clientIDPattern。
const guidLikeID = "0f8a1b2c3d4e5f60718293a4b5c6d7e8"

// newFakeClient 构造一个不依赖 HTTP 连接的白盒客户端，用于测试投递与退出路径。
//
// request 保持为 nil：这些用例不会触发响应写入（写入只发生在 Run 消费消息或
// 心跳时，而用例都在此之前退出）。
func newFakeClient(id string, chanSize int) *Client {
	return &Client{
		id:          id,
		messageChan: make(chan Message, chanSize),
		done:        make(chan struct{}),
	}
}
