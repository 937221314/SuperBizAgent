// Package sse 提供基于 GoFrame HTTP 服务的 SSE（Server-Sent Events）推送能力。
//
// 典型用法（必须在 handler 内、同一个 goroutine 中依次调用 Create 与 Run）：
//
//	client, err := srv.Create(r.Context(), r)
//	if err != nil {
//	    return err
//	}
//	srv.Run(r.Context(), client) // 阻塞直到连接断开
//	return nil
//
// 并发约定（修改本文件前请先读完）：
//   - 响应写入只发生在 Run 的循环中（单写者），Send/Broadcast 仅把消息投递到
//     channel，因此事件帧不会被并发写乱；
//   - messageChan 永不关闭，连接结束信号由 done channel 广播，避免向已关闭
//     channel 发送导致的 panic；
//   - 客户端注册表基于 gmap.StrAnyMap（safe 模式），可以并发读写。
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
	// contentTypeSSE 是 SSE 响应的标准 Content-Type。
	// 注意与 utility/middleware 的流式响应白名单保持一致（按媒体类型匹配，
	// charset 参数不影响匹配结果）。
	contentTypeSSE = "text/event-stream; charset=utf-8"

	// eventConnected 是握手成功事件名。
	eventConnected = "connected"

	// heartbeatComment 是心跳注释行的内容。
	heartbeatComment = "ping"

	// defaultMessageChanSize 是单个客户端的消息缓冲容量。
	defaultMessageChanSize = 100

	// defaultHeartbeatInterval 是心跳发送间隔。
	// 必须小于反向代理/网关的空闲超时，否则连接可能被中途回收。
	defaultHeartbeatInterval = 20 * time.Second

	// defaultRetryInterval 是下发给客户端的重连间隔（EventSource 的 retry 字段）。
	defaultRetryInterval = 3 * time.Second

	// clientIDMaxLen 是客户端自带 ID 的最大长度。
	clientIDMaxLen = 64
)

// clientIDPattern 限定客户端自带 ID 的字符集与长度，避免超长或特殊字符造成
// 内存放大、日志注入，或与内部约定混淆。服务端生成的 ID 同样满足该规则。
var clientIDPattern = regexp.MustCompile(fmt.Sprintf(`^[A-Za-z0-9_-]{1,%d}$`, clientIDMaxLen))

// 投递类错误，供调用方用 errors.Is 判断。
var (
	// ErrClientNotFound 表示目标客户端不存在（未连接或已注销）。
	ErrClientNotFound = gerror.New("sse 客户端不存在")
	// ErrClientClosed 表示目标客户端连接已关闭。
	ErrClientClosed = gerror.New("sse 客户端连接已关闭")
	// ErrBufferFull 表示客户端消息缓冲区已满，消息被丢弃。
	ErrBufferFull = gerror.New("sse 客户端消息缓冲区已满，消息被丢弃")
)

// Message 表示一条待推送的 SSE 事件。
type Message struct {
	// ID 事件 ID，写入 "id:" 字段；为空时不写该字段（空值会把客户端的
	// Last-Event-ID 重置为空，因此这里选择不写）。
	ID string

	// Event 事件名，写入 "event:" 字段；为空时客户端按默认的 message 事件处理。
	Event string

	// Data 事件数据，写入 "data:" 字段；为空时写一个空的 data 字段，
	// 内容中的换行会被拆成多个 data 字段。
	Data string
}

// Client 表示一条已建立的 SSE 连接。
//
// 字段不导出，避免业务层直接操作响应对象而绕过单写者约定。
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

// close 广播连接结束信号，幂等；不会关闭 messageChan。
func (c *Client) close() {
	c.closeOnce.Do(func() {
		close(c.done)
	})
}

// writeRaw 写入原始内容并立即 Flush。
//
// gf 的 Response 内部带缓冲区，不调用 Flush 内容不会下发。
// Write 与 Flush 都不返回 error（写失败无法在这一层感知），
// 连接断开统一由 request context 取消体现。
func (c *Client) writeRaw(raw string) {
	c.request.Response.Write(raw)
	c.request.Response.Flush()
}

// writeEvent 写入一个完整的事件帧。只能由 Run 的循环调用。
func (c *Client) writeEvent(msg Message) {
	c.writeRaw(buildEvent(msg))
}

// writeComment 写入一个注释帧（": text"），用于心跳保活。text 必须是单行内部常量。
func (c *Client) writeComment(text string) {
	c.writeRaw(": " + text + "\n\n")
}

// Server SSE 服务，负责客户端注册表与消息投递。
type Server struct {
	// clients 客户端注册表，key 为客户端 ID；gmap 的 safe 模式保证并发安全。
	clients *gmap.StrAnyMap

	heartbeatInterval time.Duration
	retryInterval     time.Duration
	messageChanSize   int
}

// Option 用于自定义 Server 的可选参数。
type Option func(*Server)

// WithHeartbeatInterval 设置心跳间隔；小于等于 0 时忽略，保留默认值。
func WithHeartbeatInterval(d time.Duration) Option {
	return func(s *Server) {
		if d > 0 {
			s.heartbeatInterval = d
		}
	}
}

// WithRetryInterval 设置下发给客户端的重连间隔；小于等于 0 时忽略，保留默认值。
func WithRetryInterval(d time.Duration) Option {
	return func(s *Server) {
		if d > 0 {
			s.retryInterval = d
		}
	}
}

// WithMessageChanSize 设置单个客户端的消息缓冲容量；小于等于 0 时忽略，保留默认值。
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

// ClientCount 返回当前已连接的客户端数量。
func (s *Server) ClientCount() int {
	return s.clients.Size()
}

// Create 完成 SSE 握手：校验能力与客户端 ID、注册客户端，并下发 retry 与
// connected 事件。Create 本身不阻塞，返回的 Client 必须立即交给 Run 消费消息。
//
// 客户端未携带 client_id 时由服务端生成；格式非法或 ID 已被占用时返回错误。
// CORS 响应头由 utility/middleware 的 CORSMiddleware 统一处理，此处不重复设置。
func (s *Server) Create(ctx context.Context, r *ghttp.Request) (*Client, error) {
	// SSE 依赖分块传输，要求底层 ResponseWriter 支持 Flush；
	// 不支持时数据会滞留在缓冲区，与其静默失效不如在握手阶段直接失败。
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
	// SetIfNotExist 保证「查重 + 注册」的原子性，避免并发同名连接互相覆盖。
	if !s.clients.SetIfNotExist(clientID, client) {
		return nil, gerror.Newf("sse 客户端 ID 已被占用: %s", clientID)
	}

	// 以下响应头必须在任何写入之前设置。
	r.Response.Header().Set("Content-Type", contentTypeSSE)
	r.Response.Header().Set("Cache-Control", "no-cache")
	// 关闭 Nginx 等反向代理的响应缓冲，否则事件会被攒够一批才下发。
	r.Response.Header().Set("X-Accel-Buffering", "no")

	// 先写 retry，再写 connected：此时 Run 尚未开始消费消息队列，
	// 因此投递进来的业务消息一定排在握手帧之后。
	client.writeRaw(buildRetry(s.retryInterval))
	client.writeEvent(Message{
		ID:    clientID,
		Event: eventConnected,
		Data:  connectedData(clientID),
	})

	g.Log().Debugf(ctx, "sse 客户端已连接: %s", clientID)
	return client, nil
}

// Run 阻塞消费客户端消息并写入响应，直到连接断开、ctx 取消或被 RemoveClient 关闭。
//
// 必须在 Create 成功后的同一个 handler goroutine 中调用，返回时会自动注销客户端。
// 连接断开（客户端关闭页面、网络中断）通过 ctx.Done 感知，因此不做错误返回。
func (s *Server) Run(ctx context.Context, client *Client) {
	if client == nil {
		return
	}
	// 退出路径统一在这里注销：close(done) 幂等，注册表按实例删除，不会误删同 ID 新连接。
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
			// 心跳用于保活与让前端确认连接存活；写失败无法在这一层感知。
			client.writeComment(heartbeatComment)
		}
	}
}

// Send 向指定客户端投递一条消息。
//
// 投递是非阻塞的：客户端不存在、已关闭或缓冲区已满时立即返回错误，
// 不做阻塞等待，避免慢客户端拖垮业务发送方。
func (s *Server) Send(clientID string, msg Message) error {
	client, err := s.clientOf(clientID)
	if err != nil {
		return err
	}
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

// Broadcast 向所有在线客户端投递同一条消息，返回成功投递的客户端数量。
//
// 单个客户端失败（已断开、缓冲区已满）不影响其它客户端，失败原因记入日志。
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

// RemoveClient 关闭并注销指定 ID 的客户端，返回注册表中是否存在该客户端。
//
// Run 会因 done 被关闭而退出，因此该方法是幂等的「踢下线」入口。
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

// clientOf 按 ID 取出客户端，不存在时返回 ErrClientNotFound。
func (s *Server) clientOf(clientID string) (*Client, error) {
	value, ok := s.clients.Search(clientID)
	if !ok {
		return nil, gerror.Wrapf(ErrClientNotFound, "client_id=%s", clientID)
	}
	client, ok := value.(*Client)
	if !ok {
		// 不可达：注册表只写入 *Client，这里只是避免类型断言 panic。
		return nil, gerror.Wrapf(ErrClientNotFound, "client_id=%s", clientID)
	}
	return client, nil
}

// removeClient 注销 client 自身：广播关闭信号，并且只有当注册表中仍是该实例时才删除。
//
// 与 RemoveClient 的区别：本方法按实例删除，用于 Run 退出时的清理，
// 避免「旧连接清理」与「同 ID 新连接注册」竞争时误删新连接。
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

// resolveClientID 解析客户端 ID：未指定时由服务端生成，指定时按 clientIDPattern 校验。
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

// connectedData 生成 connected 事件的数据。使用 json.Marshal 转义，
// 避免客户端 ID 中的引号等字符破坏 JSON 结构。
func connectedData(clientID string) string {
	b, err := json.Marshal(connectedPayload{Status: "connected", ClientID: clientID})
	if err != nil {
		// 对纯字符串结构体不会失败，这里只是兜底。
		return `{"status":"connected"}`
	}
	return string(b)
}

// buildRetry 生成 retry 字段帧（单位毫秒），指示 EventSource 的重连间隔。
func buildRetry(d time.Duration) string {
	return fmt.Sprintf("retry: %d\n\n", d.Milliseconds())
}

// buildEvent 把消息序列化为一个符合 SSE 规范的事件帧。
//
// 规范要求每个字段独占一行、以 "\n" 结尾，事件之间以空行分隔；
// data 中的换行必须拆成多个 data 字段，否则整帧会被解析失败。
func buildEvent(msg Message) string {
	var b strings.Builder
	if msg.ID != "" {
		writeSSEField(&b, "id", sanitizeField(msg.ID))
	}
	if msg.Event != "" {
		writeSSEField(&b, "event", sanitizeField(msg.Event))
	}
	// Split("") 返回 [""]，因此空数据也会写一个 "data: " 字段，保证帧结构完整。
	for _, line := range strings.Split(normalizeNewline(msg.Data), "\n") {
		writeSSEField(&b, "data", line)
	}
	b.WriteByte('\n') // 空行：事件结束标志
	return b.String()
}

// writeSSEField 写入一行 "name: value\n" 字段。
func writeSSEField(b *strings.Builder, name, value string) {
	b.WriteString(name)
	b.WriteString(": ")
	b.WriteString(value)
	b.WriteByte('\n')
}

// sanitizeField 去掉字段值中的换行。
//
// id/event 字段含换行会破坏帧结构，并可能被用于注入额外字段（SSE 帧注入）。
func sanitizeField(s string) string {
	if !strings.ContainsAny(s, "\r\n") {
		return s
	}
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

// normalizeNewline 把 CRLF 与 CR 统一成 LF，便于按行拆分。
func normalizeNewline(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}
