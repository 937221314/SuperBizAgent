// Package sse 提供基于 GoFrame HTTP 服务的 SSE（Server-Sent Events）推送能力。
//
// 约定：Create 与 Run 必须在同一个 handler goroutine 中依次调用；响应写入只发生在
// Run 循环中（单写者），messageChan 永不关闭。详见 dev-docs/sse.md。
package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/container/gmap"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/guid"
)

const (
	// contentTypeSSE 需与 utility/middleware 的流式响应白名单保持一致。
	contentTypeSSE = "text/event-stream; charset=utf-8"

	eventConnected   = "connected"
	heartbeatComment = "ping"

	// defaultHeartbeatInterval 必须小于反向代理/网关的空闲超时。
	defaultHeartbeatInterval = 20 * time.Second

	defaultMessageChanSize = 100
	defaultRetryInterval   = 3 * time.Second
	clientIDMaxLen         = 64
)

// clientIDPattern 限定客户端自带 ID，避免超长与特殊字符。
var clientIDPattern = regexp.MustCompile(fmt.Sprintf(`^[A-Za-z0-9_-]{1,%d}$`, clientIDMaxLen))

// 投递错误，可用 errors.Is 判断。
var (
	// ErrClientNotFound 表示客户端不存在（未连接或已注销）。
	ErrClientNotFound = gerror.New("sse 客户端不存在")
	// ErrClientClosed 表示客户端连接已关闭。
	ErrClientClosed = gerror.New("sse 客户端连接已关闭")
	// ErrBufferFull 表示缓冲区已满，消息被丢弃。
	ErrBufferFull = gerror.New("sse 客户端消息缓冲区已满，消息被丢弃")
)

// Message 表示一条待推送的 SSE 事件。
type Message struct {
	// ID 写入 "id:" 字段，为空时不写（空 id 会重置客户端的 Last-Event-ID）。
	ID string

	// Event 事件名，写入 "event:" 字段，为空时按默认 message 事件处理。
	Event string

	// Data 事件数据，换行会被拆成多个 "data:" 字段。
	Data string
}

// Client 表示一条已建立的 SSE 连接，字段不导出以避免绕过单写者约定。
type Client struct {
	id          string
	request     *ghttp.Request
	messageChan chan Message
	done        chan struct{}
	closeOnce   sync.Once
}

// ID 返回客户端 ID。
func (c *Client) ID() string {
	return c.id
}

// close 广播连接结束信号，幂等；不关闭 messageChan。
func (c *Client) close() {
	c.closeOnce.Do(func() {
		close(c.done)
	})
}

// writeRaw 写入并立即 Flush：gf 的 Response 带缓冲，不 Flush 不会下发。
// Write/Flush 都不返回 error，写失败由 request context 取消体现。
func (c *Client) writeRaw(raw string) {
	c.request.Response.Write(raw)
	c.request.Response.Flush()
}

// writeEvent 写入一个事件帧，只能由 Run 循环调用（单写者）。
func (c *Client) writeEvent(msg Message) {
	c.writeRaw(buildEvent(msg))
}

// writeComment 写入注释帧（": text"），用于心跳。
func (c *Client) writeComment(text string) {
	c.writeRaw(": " + text + "\n\n")
}

// Server SSE 服务，管理客户端注册表与消息投递。
type Server struct {
	// clients key 为客户端 ID，gmap 的 safe 模式保证并发安全。
	clients *gmap.StrAnyMap

	heartbeatInterval time.Duration
	retryInterval     time.Duration
	messageChanSize   int
}

// Option 用于自定义 Server 的可选参数。
type Option func(*Server)

// WithHeartbeatInterval 设置心跳间隔，小于等于 0 时忽略。
func WithHeartbeatInterval(d time.Duration) Option {
	return func(s *Server) {
		if d > 0 {
			s.heartbeatInterval = d
		}
	}
}

// WithRetryInterval 设置下发给客户端的重连间隔，小于等于 0 时忽略。
func WithRetryInterval(d time.Duration) Option {
	return func(s *Server) {
		if d > 0 {
			s.retryInterval = d
		}
	}
}

// WithMessageChanSize 设置单客户端的消息缓冲容量，小于等于 0 时忽略。
func WithMessageChanSize(n int) Option {
	return func(s *Server) {
		if n > 0 {
			s.messageChanSize = n
		}
	}
}

// New 创建 SSE 服务。
func New(opts ...Option) *Server {
	s := &Server{
		clients:           gmap.NewStrAnyMap(true),
		heartbeatInterval: defaultHeartbeatInterval,
		retryInterval:     defaultRetryInterval,
		messageChanSize:   defaultMessageChanSize,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ClientCount 返回在线客户端数量。
func (s *Server) ClientCount() int {
	return s.clients.Size()
}

// Create 完成 SSE 握手：校验客户端 ID、注册客户端并下发 retry 与 connected 事件。
// 它不阻塞，返回的 Client 必须立即交给 Run 消费消息。CORS 头由中间件统一处理。
func (s *Server) Create(ctx context.Context, r *ghttp.Request) (*Client, error) {
	// 不支持 Flush 时数据会滞留在缓冲区，握手阶段直接失败。
	if _, ok := r.Response.RawWriter().(http.Flusher); !ok {
		return nil, gerror.New("sse 当前响应不支持 Flush，无法建立流式连接")
	}

	clientID, err := resolveClientID(r)
	if err != nil {
		return nil, err
	}

	client := &Client{
		id:          clientID,
		request:     r,
		messageChan: make(chan Message, s.messageChanSize),
		done:        make(chan struct{}),
	}
	// 原子查重，避免同名连接互相覆盖。
	if !s.clients.SetIfNotExist(clientID, client) {
		return nil, gerror.Newf("sse 客户端 ID 已被占用: %s", clientID)
	}

	// 响应头必须在任何写入之前设置。
	r.Response.Header().Set("Content-Type", contentTypeSSE)
	r.Response.Header().Set("Cache-Control", "no-cache")
	// 关闭反向代理的响应缓冲，否则事件会被攒够一批才下发。
	r.Response.Header().Set("X-Accel-Buffering", "no")

	// 此时 Run 还没开始消费消息队列，业务消息必然排在握手帧之后。
	client.writeRaw(buildRetry(s.retryInterval))
	client.writeEvent(Message{
		ID:    clientID,
		Event: eventConnected,
		Data:  connectedData(clientID),
	})

	g.Log().Debugf(ctx, "sse 客户端已连接: %s", clientID)
	return client, nil
}

// Run 阻塞消费消息并写入响应，直到连接断开、ctx 取消或被 RemoveClient 关闭，
// 返回时自动注销客户端。必须在 Create 成功后的同一个 handler goroutine 中调用。
func (s *Server) Run(ctx context.Context, client *Client) {
	if client == nil {
		return
	}
	// 统一在此注销：close(done) 幂等，按实例删除不会误删同 ID 新连接。
	defer func() {
		s.removeClient(client)
		g.Log().Debugf(ctx, "sse 客户端已断开: %s", client.ID())
	}()

	heartbeat := time.NewTicker(s.heartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-client.done:
			return
		case msg := <-client.messageChan:
			client.writeEvent(msg)
		case <-heartbeat.C:
			client.writeComment(heartbeatComment)
		}
	}
}

// Send 非阻塞地向指定客户端投递消息：客户端不存在、已关闭或缓冲区已满时立即返回错误。
func (s *Server) Send(clientID string, msg Message) error {
	client, err := s.clientOf(clientID)
	if err != nil {
		return err
	}
	// 先单独确认一次关闭状态，避免 select 随机选中已关闭客户端仍可写的 channel。
	select {
	case <-client.done:
		return gerror.Wrapf(ErrClientClosed, "client_id=%s", clientID)
	default:
	}
	select {
	case <-client.done:
		return gerror.Wrapf(ErrClientClosed, "client_id=%s", clientID)
	case client.messageChan <- msg:
		return nil
	default:
		return gerror.Wrapf(ErrBufferFull, "client_id=%s", clientID)
	}
}

// Broadcast 向所有在线客户端投递同一条消息，返回成功数量；单个客户端失败不影响其它客户端。
func (s *Server) Broadcast(ctx context.Context, msg Message) int {
	sent := 0
	for _, clientID := range s.clients.Keys() {
		if err := s.Send(clientID, msg); err != nil {
			g.Log().Debugf(ctx, "sse 广播失败 client_id=%s: %v", clientID, err)
			continue
		}
		sent++
	}
	return sent
}

// RemoveClient 关闭并注销指定 ID 的客户端，返回是否存在；Run 会随之退出。
func (s *Server) RemoveClient(clientID string) bool {
	value, ok := s.clients.Search(clientID)
	if !ok {
		return false
	}
	if client, ok := value.(*Client); ok {
		client.close()
	}
	s.clients.Remove(clientID)
	return true
}

// clientOf 按 ID 取客户端，不存在时返回 ErrClientNotFound。
func (s *Server) clientOf(clientID string) (*Client, error) {
	value, ok := s.clients.Search(clientID)
	if !ok {
		return nil, gerror.Wrapf(ErrClientNotFound, "client_id=%s", clientID)
	}
	client, ok := value.(*Client)
	if !ok {
		// 不可达：注册表只写入 *Client。
		return nil, gerror.Wrapf(ErrClientNotFound, "client_id=%s", clientID)
	}
	return client, nil
}

// removeClient 按实例注销：只有注册表中仍是该实例时才删除，避免误删同 ID 的新连接。
func (s *Server) removeClient(client *Client) {
	client.close()
	value, ok := s.clients.Search(client.id)
	if !ok {
		return
	}
	if registered, ok := value.(*Client); ok && registered == client {
		s.clients.Remove(client.id)
	}
}

// resolveClientID 未指定时生成 ID，指定时按 clientIDPattern 校验。
func resolveClientID(r *ghttp.Request) (string, error) {
	clientID := strings.TrimSpace(r.Get("client_id").String())
	if clientID == "" {
		return guid.S(), nil
	}
	if !clientIDPattern.MatchString(clientID) {
		return "", gerror.Newf(
			"sse 客户端 ID 非法（仅允许字母、数字、-、_，长度 1~%d）: %s",
			clientIDMaxLen, clientID,
		)
	}
	return clientID, nil
}

// connectedPayload 是 connected 事件的数据体。
type connectedPayload struct {
	Status   string `json:"status"`
	ClientID string `json:"client_id"`
}

// connectedData 生成 connected 事件数据，转义交由 json.Marshal 处理。
func connectedData(clientID string) string {
	b, err := json.Marshal(connectedPayload{Status: "connected", ClientID: clientID})
	if err != nil {
		// 纯字符串结构体不会失败，仅兜底。
		return `{"status":"connected"}`
	}
	return string(b)
}

// buildRetry 生成 retry 帧（单位毫秒）。
func buildRetry(d time.Duration) string {
	return fmt.Sprintf("retry: %d\n\n", d.Milliseconds())
}

// buildEvent 按 SSE 规范把消息序列化为一个事件帧：
// 字段各占一行，事件之间以空行分隔。
func buildEvent(msg Message) string {
	var b strings.Builder
	if msg.ID != "" {
		writeSSEField(&b, "id", sanitizeField(msg.ID))
	}
	if msg.Event != "" {
		writeSSEField(&b, "event", sanitizeField(msg.Event))
	}
	// Split("") 得到 [""]，空数据也保证有一个 data 字段。
	for _, line := range strings.Split(normalizeNewline(msg.Data), "\n") {
		writeSSEField(&b, "data", line)
	}
	b.WriteByte('\n') // 空行：事件结束标志
	return b.String()
}

// writeSSEField 写入一行 "name: value"。
func writeSSEField(b *strings.Builder, name, value string) {
	b.WriteString(name)
	b.WriteString(": ")
	b.WriteString(value)
	b.WriteByte('\n')
}

// sanitizeField 去掉字段值中的换行，防止破坏帧结构（帧注入）。
func sanitizeField(s string) string {
	if !strings.ContainsAny(s, "\r\n") {
		return s
	}
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

// normalizeNewline 把 CRLF/CR 统一为 LF，便于按行拆分。
func normalizeNewline(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}
