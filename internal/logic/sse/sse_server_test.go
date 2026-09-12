package sse

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/guid"
)

const (
	// e2eDialTimeout 建立 TCP 连接的超时。
	e2eDialTimeout = 3 * time.Second
	// e2eReadTimeout 单次读取 SSE 数据的超时。
	e2eReadTimeout = 3 * time.Second
)

// startSSEServer 启动测试用的 gf HTTP 服务，返回监听地址。
//
// handler 内的调用顺序（Create -> Run）就是生产代码的正确用法。
func startSSEServer(t *testing.T, srv *Server) string {
	t.Helper()

	httpServer := g.Server(guid.S())
	httpServer.SetAddr("127.0.0.1:0")
	httpServer.SetDumpRouterMap(false)
	httpServer.BindHandler("/sse", func(r *ghttp.Request) {
		client, err := srv.Create(r.Context(), r)
		if err != nil {
			r.Response.WriteStatus(http.StatusBadRequest, err.Error())
			return
		}
		srv.Run(r.Context(), client)
	})
	if err := httpServer.Start(); err != nil {
		t.Fatalf("启动测试服务失败: %v", err)
	}
	t.Cleanup(func() {
		_ = httpServer.Shutdown()
	})

	return fmt.Sprintf("127.0.0.1:%d", httpServer.GetListenedPort())
}

// dialSSE 建立一条 SSE 连接并读完响应头。clientID 为空时不带 client_id 参数。
func dialSSE(t *testing.T, addr, clientID string) (net.Conn, *http.Response, *bufio.Reader) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", addr, e2eDialTimeout)
	if err != nil {
		t.Fatalf("连接测试服务失败: %v", err)
	}

	path := "/sse"
	if clientID != "" {
		path += "?client_id=" + clientID
	}
	requestLine := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nAccept: text/event-stream\r\n\r\n", path, addr)
	if _, err = fmt.Fprintf(conn, "%s", requestLine); err != nil {
		t.Fatalf("发送请求失败: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, "http://"+addr+path, nil)
	if err != nil {
		t.Fatalf("构造请求对象失败: %v", err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		t.Fatalf("解析响应头失败: %v", err)
	}
	return conn, resp, br
}

// readFrame 读取一个 SSE 帧（读到空行为止），返回字段名到值的映射。
//
// 多行 data 会用 "\n" 拼回原始内容；注释行以 ":comment" 为键保存。
func readFrame(t *testing.T, conn net.Conn, br *bufio.Reader) map[string]string {
	t.Helper()

	fields := make(map[string]string)
	for {
		if err := conn.SetReadDeadline(time.Now().Add(e2eReadTimeout)); err != nil {
			t.Fatalf("设置读超时失败: %v", err)
		}
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("读取 SSE 帧失败: %v（已读到 %v）", err, fields)
		}
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			if len(fields) > 0 {
				return fields
			}
			continue // 忽略帧之间的多余空行
		}
		if strings.HasPrefix(line, ":") {
			fields[":comment"] = strings.TrimSpace(strings.TrimPrefix(line, ":"))
			continue
		}

		name, value, found := strings.Cut(line, ":")
		if !found {
			// 规范允许「无冒号」的字段行，视为字段名为整行、值为空。
			fields[line] = ""
			continue
		}
		value = strings.TrimPrefix(value, " ")
		if name == "data" {
			if prev, ok := fields["data"]; ok {
				fields["data"] = prev + "\n" + value
			} else {
				fields["data"] = value
			}
			continue
		}
		fields[name] = value
	}
}

// waitForClientCount 轮询等待客户端数量达到期望值。
func waitForClientCount(t *testing.T, srv *Server, want int, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if srv.ClientCount() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("等待 ClientCount = %d 超时，当前为 %d", want, srv.ClientCount())
}

func TestServerStreamEndToEnd(t *testing.T) {
	// 心跳保持默认值（20s），避免心跳帧与业务帧在读取顺序上互相干扰。
	srv := New(WithRetryInterval(1500 * time.Millisecond))
	addr := startSSEServer(t, srv)

	conn, resp, br := dialSSE(t, addr, "e2e-1")
	defer func() {
		_ = conn.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("握手状态码 = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream 前缀", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache", got)
	}
	if got := resp.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Fatalf("X-Accel-Buffering = %q, want no", got)
	}

	// 帧 1：retry
	if got := readFrame(t, conn, br)["retry"]; got != "1500" {
		t.Fatalf("retry = %q, want 1500", got)
	}

	// 帧 2：connected（回归校验：id 不应残留 %s 占位符）
	connected := readFrame(t, conn, br)
	if got := connected["event"]; got != eventConnected {
		t.Fatalf("connected 帧 event = %q, want %q", got, eventConnected)
	}
	if got := connected["id"]; got != "e2e-1" {
		t.Fatalf("connected 帧 id = %q, want e2e-1", got)
	}
	var payload struct {
		Status   string `json:"status"`
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal([]byte(connected["data"]), &payload); err != nil {
		t.Fatalf("connected 帧 data 不是合法 JSON: %v, raw=%q", err, connected["data"])
	}
	if payload.Status != "connected" || payload.ClientID != "e2e-1" {
		t.Fatalf("connected 帧 data = %+v", payload)
	}

	if srv.ClientCount() != 1 {
		t.Fatalf("ClientCount() = %d, want 1", srv.ClientCount())
	}

	// 单播：多行 data 应被还原
	if err := srv.Send("e2e-1", Message{ID: "m1", Event: "chat", Data: "你好\n世界"}); err != nil {
		t.Fatalf("Send 失败: %v", err)
	}
	msg := readFrame(t, conn, br)
	if msg["id"] != "m1" || msg["event"] != "chat" || msg["data"] != "你好\n世界" {
		t.Fatalf("单播帧 = %v", msg)
	}

	// 广播
	if got := srv.Broadcast(context.Background(), Message{Event: "notice", Data: "全体消息"}); got != 1 {
		t.Fatalf("Broadcast() = %d, want 1", got)
	}
	notice := readFrame(t, conn, br)
	if notice["event"] != "notice" || notice["data"] != "全体消息" {
		t.Fatalf("广播帧 = %v", notice)
	}

	// 同一 client_id 重复连接应被拒绝，且不影响已有连接
	dupConn, dupResp, _ := dialSSE(t, addr, "e2e-1")
	_ = dupConn.Close()
	if dupResp.StatusCode == http.StatusOK {
		t.Fatalf("重复 client_id 应被拒绝，实际状态码 = %d", dupResp.StatusCode)
	}
	if srv.ClientCount() != 1 {
		t.Fatalf("重复连接后 ClientCount() = %d, want 1", srv.ClientCount())
	}

	// 客户端断开后应自动注销（依赖 request context 取消）
	_ = conn.Close()
	waitForClientCount(t, srv, 0, 3*time.Second)

	// 注销之后的投递应返回「客户端不存在」
	if err := srv.Send("e2e-1", Message{Data: "x"}); !errors.Is(err, ErrClientNotFound) {
		t.Fatalf("Send(已断开客户端) = %v, want ErrClientNotFound", err)
	}
}

func TestServerHeartbeat(t *testing.T) {
	srv := New(WithHeartbeatInterval(50 * time.Millisecond))
	addr := startSSEServer(t, srv)

	conn, resp, br := dialSSE(t, addr, "hb-1")
	defer func() {
		_ = conn.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("握手状态码 = %d, want 200", resp.StatusCode)
	}

	// 握手的两帧：retry 与 connected
	readFrame(t, conn, br)
	readFrame(t, conn, br)

	// 之后第一个帧应为心跳注释行
	if got := readFrame(t, conn, br)[":comment"]; got != heartbeatComment {
		t.Fatalf("心跳帧注释 = %q, want %q", got, heartbeatComment)
	}
}

func TestServerClientID(t *testing.T) {
	srv := New()
	addr := startSSEServer(t, srv)

	// 未携带 client_id 时由服务端生成
	conn, resp, br := dialSSE(t, addr, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("握手状态码 = %d, want 200", resp.StatusCode)
	}
	readFrame(t, conn, br) // retry
	connected := readFrame(t, conn, br)
	generated := connected["id"]
	if !clientIDPattern.MatchString(generated) {
		t.Fatalf("服务端生成的 client_id = %q, 不满足 clientIDPattern", generated)
	}
	if got := connected["id"]; got == "" {
		t.Fatal("connected 帧缺少 id 字段")
	}
	if srv.ClientCount() != 1 {
		t.Fatalf("ClientCount() = %d, want 1", srv.ClientCount())
	}

	// 非法 client_id（超长）应被拒绝，且不产生注册
	invalidConn, invalidResp, _ := dialSSE(t, addr, strings.Repeat("a", clientIDMaxLen+1))
	_ = invalidConn.Close()
	if invalidResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法 client_id 状态码 = %d, want 400", invalidResp.StatusCode)
	}
	if srv.ClientCount() != 1 {
		t.Fatalf("非法请求后 ClientCount() = %d, want 1", srv.ClientCount())
	}

	_ = conn.Close()
	waitForClientCount(t, srv, 0, 3*time.Second)
}
