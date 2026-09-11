// Package middleware 提供 HTTP 服务的通用中间件。
//
// 中间件注册顺序约定（注意顺序会影响结果）：
//
//	s.Group("/", func(group *ghttp.RouterGroup) {
//	    group.Middleware(
//	        middleware.CORSMiddleware,      // 1. 最先执行：为所有响应（含错误响应）补上跨域头
//	        middleware.ResponseMiddleware,  // 2. 统一包装业务响应
//	    )
//	    group.Bind(controller...)           // 3. 业务路由
//	})
//
// 之所以让 CORS 在前：跨域响应头必须在响应体写出之前设置，
// 否则浏览器拿到的错误响应会缺失跨域头，前端只能看到 "CORS error" 而看不到真实错误。
package middleware

import (
	"mime"
	"net/http"

	"github.com/gogf/gf/v2/errors/gcode"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

// streamContentTypes 流式响应的 Content-Type 列表。
//
// 这些类型的响应体是"边生成边推送"的持续流（SSE、二进制下载、长轮询），
// 不能再用 JSON 信封去包装，否则会破坏原有协议、导致前端无法解析。
var streamContentTypes = []string{
	"text/event-stream",         // SSE：大模型流式输出、实时推送
	"application/octet-stream",  // 二进制流：文件下载
	"multipart/x-mixed-replace", // 连续替换的多段流：视频/图像推流
}

// CORSMiddleware 处理 CORS 跨域请求。
//
// 使用 CORSDefault 时会返回 Access-Control-Allow-Origin: *，即允许任意来源访问。
// 这在开发/公开接口场景下够用，但存在两点需要注意：
//  1. 若前端需要携带 Cookie 或鉴权凭证（withCredentials），浏览器禁止
//     Allow-Origin 为 * 与 Allow-Credentials 同时生效，此时必须改为白名单来源；
//  2. 生产环境建议按域名白名单收敛，避免任意站点直接调用你的接口。
//
// 需要在有明确白名单需求时，替换为自定义实现，例如：
//
//	if origin := r.Header.Get("Origin"); inWhitelist(origin) {
//	    r.Response.Header().Set("Access-Control-Allow-Origin", origin)
//	    r.Response.Header().Set("Access-Control-Allow-Credentials", "true")
//	}
func CORSMiddleware(r *ghttp.Request) {
	// CORSDefault 会自动处理 OPTIONS 预检请求并写入标准跨域响应头。
	r.Response.CORSDefault()

	// 放行，继续执行后续中间件与业务 handler。
	r.Middleware.Next()
}

// ResponseMiddleware 统一响应中间件。
//
// 作用：把业务 handler 的返回值与 error 统一包装成 Response 结构，
// 让前端始终拿到一致的数据格式：{code, message, data}。
//
// 执行模型说明：gf 的中间件是"洋葱模型"，
// r.Middleware.Next() 之前是请求进入阶段，之后是响应返回阶段。
// 本中间件只需在 Next() 之后处理结果。
//
// 注册位置要求：必须位于业务 handler 之前、且建议位于 CORSMiddleware 之后。
func ResponseMiddleware(r *ghttp.Request) {
	// 先执行后续中间件与业务 handler，等它把结果/错误准备好。
	r.Middleware.Next()

	// ---------------------------------------------------------------------
	// 保护 1：handler 已经自行输出过内容，则不再包装。
	//
	// BufferLength() > 0 表示响应缓冲里已有内容（例如 handler 调用了 Write/WriteJson）；
	// BytesWritten() > 0  表示内容已经实际写回客户端（例如用了 WriteExit、静态文件服务）。
	//
	// 若不判断，这里会再追加一段 JSON，客户端收到形如
	//   {"foo":"bar"}{"code":0,"message":"OK","data":null}
	// 的脏数据，导致 JSON 解析失败。
	// 这与 GoFrame 官方 MiddlewareHandlerResponse 的处理保持一致。
	// ---------------------------------------------------------------------
	if r.Response.BufferLength() > 0 || r.Response.BytesWritten() > 0 {
		return
	}

	// ---------------------------------------------------------------------
	// 保护 2：流式响应（SSE / 二进制流）不包装。
	//
	// 对 SSE 这类流，响应头 Content-Type 已是 text/event-stream，
	// 包装成 JSON 会彻底破坏协议（前端 EventSource 直接报错）。
	// ---------------------------------------------------------------------
	mediaType, _, _ := mime.ParseMediaType(r.Response.Header().Get("Content-Type"))
	for _, contentType := range streamContentTypes {
		if mediaType == contentType {
			return
		}
	}

	var (
		// code 必须显式声明为接口类型 gcode.Code：
		// gerror.Code(err) 返回的就是 gcode.Code 接口，
		// 若用 := 推导会得到 gcode 内部的具体实现类型，后续赋值会编译失败。
		code gcode.Code     = gcode.CodeOK           // 业务状态码，默认成功
		msg                 = gcode.CodeOK.Message() // 提示信息，默认取成功码的文案
		data any                                     // 业务数据，仅在成功时返回
		err  = r.GetError()                          // 本次请求链路上被设置的错误
	)

	if err != nil {
		// -----------------------------------------------------------------
		// 分支 A：业务/系统出错。
		//
		// 1) 取错误码；若错误未附带错误码（CodeNil），视为内部错误。
		// 2) 对外只返回错误码对应的通用文案，绝不返回 err.Error()。
		//    原因：err.Error() 可能包含 SQL 语句、文件绝对路径、数据库连接串、
		//    第三方服务地址等内部信息，直接透传会造成信息泄露。
		// 3) 详细错误写入日志，供开发/运维排查。
		// -----------------------------------------------------------------
		code = gerror.Code(err)
		if code == gcode.CodeNil {
			code = gcode.CodeInternalError
		}
		g.Log().Errorf(r.Context(), "request error: %+v", err)
		msg = code.Message()

		// 注意：出错时不返回 data，避免把半成品/敏感数据带给前端。
	} else if r.Response.Status > 0 && r.Response.Status != http.StatusOK {
		// -----------------------------------------------------------------
		// 分支 B：handler 没有返回 error，但显式设置了非 200 的 HTTP 状态码。
		//
		// 典型场景：handler 里写了 r.Response.WriteStatus(404) 但忘了 return error。
		// 此时若还返回 "OK" 会自相矛盾，所以把 HTTP 状态映射为业务错误码。
		// -----------------------------------------------------------------
		switch r.Response.Status {
		case http.StatusNotFound:
			code = gcode.CodeNotFound
		case http.StatusForbidden:
			code = gcode.CodeNotAuthorized
		default:
			code = gcode.CodeUnknown
		}
		msg = code.Message()
	} else {
		// -----------------------------------------------------------------
		// 分支 C：成功。
		//
		// GetHandlerResponse() 取的是 handler 的第一个返回值。
		// 约定业务 handler 统一返回形如 (result any, err error)，
		// 或使用 gf 的 g.Map / 自定义结构体作为响应数据。
		// -----------------------------------------------------------------
		data = r.GetHandlerResponse()
	}

	// 输出统一结构。WriteJson 会设置 Content-Type: application/json 并序列化内容。
	// 注意：WriteJson 内部使用 json.Marshal，失败会 panic，
	// 因此不要往 data 中放入 chan、func 等不可序列化的类型。
	r.Response.WriteJson(Response{
		Code:    code.Code(),
		Message: msg,
		Data:    data,
	})
}

// Response 统一响应结构体。
//
// 约定返回给前端的 JSON 固定为：
//
//	{
//	  "code": 0,
//	  "message": "OK",
//	  "data": { ... }
//	}
//
// 前端可依据 code 是否为 0 判断请求是否成功，而无需依赖 HTTP 状态码
// （很多场景下 HTTP 状态码固定为 200，业务错误通过 code 表达）。
type Response struct {
	// Code 业务状态码，0 表示成功，非 0 表示各类业务/系统错误，
	// 具体取值来源见 gcode 包（如 CodeNotFound、CodeInternalError）。
	Code int `json:"code"    dc:"业务状态码，0 表示成功"`

	// Message 消息提示，成功时为 "OK"，失败时为状态码对应的通用文案。
	// 注意：不会包含底层错误的原始信息，避免信息泄露。
	Message string `json:"message" dc:"消息提示"`

	// Data 业务返回数据，仅在成功时返回；出错时恒为 null。
	Data any `json:"data"    dc:"执行结果"`
}
