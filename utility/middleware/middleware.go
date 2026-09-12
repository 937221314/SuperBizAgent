// Package middleware 提供 HTTP 服务的通用中间件。
//
// 注册顺序约定（顺序会影响结果）：CORS 必须在 Response 之前，否则错误响应会缺失跨域头。
// 详见 dev-docs/http-middleware.md。
//
//	s.Group("/", func(group *ghttp.RouterGroup) {
//	    group.Middleware(
//	        middleware.CORSMiddleware,
//	        middleware.ResponseMiddleware,
//	    )
//	    group.Bind(controller...)
//	})
package middleware

import (
	"mime"
	"net/http"

	"github.com/gogf/gf/v2/errors/gcode"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

// streamContentTypes 流式响应的 Content-Type 列表，这类响应不能包装成 JSON。
var streamContentTypes = []string{
	"text/event-stream",         // SSE：大模型流式输出、实时推送
	"application/octet-stream",  // 二进制流：文件下载
	"multipart/x-mixed-replace", // 连续替换的多段流：视频/图像推流
}

// CORSMiddleware 处理 CORS 跨域请求。
//
// CORSDefault 会返回 Access-Control-Allow-Origin: *，开发与公开接口场景够用；
// 需要携带凭证或按域名白名单收敛时需替换为自定义实现，详见 dev-docs/http-middleware.md。
func CORSMiddleware(r *ghttp.Request) {
	// CORSDefault 会自动处理 OPTIONS 预检请求并写入标准跨域响应头。
	r.Response.CORSDefault()

	r.Middleware.Next()
}

// ResponseMiddleware 统一响应中间件，把 handler 的返回值与 error 包装成 Response 结构。
//
// gf 的中间件是洋葱模型，Next() 之前是请求进入阶段，之后是响应返回阶段，
// 本中间件只需在 Next() 之后处理结果。约定与保护规则详见 dev-docs/http-middleware.md。
func ResponseMiddleware(r *ghttp.Request) {
	r.Middleware.Next()

	// 保护 1：handler 已自行输出内容（Write/WriteJson/WriteExit/静态文件）时不再包装，
	// 否则客户端会收到 {"foo":"bar"}{"code":0,...} 这类脏数据导致 JSON 解析失败。
	if r.Response.BufferLength() > 0 || r.Response.BytesWritten() > 0 {
		return
	}

	// 保护 2：流式响应（SSE / 二进制流）不包装，包装成 JSON 会破坏原有协议。
	mediaType, _, _ := mime.ParseMediaType(r.Response.Header().Get("Content-Type"))
	for _, contentType := range streamContentTypes {
		if mediaType == contentType {
			return
		}
	}

	var (
		// code 需显式声明为 gcode.Code 接口类型：gerror.Code(err) 返回的就是该接口，
		// 用 := 推导会得到内部具体实现类型，后续重新赋值会编译失败。
		code gcode.Code = gcode.CodeOK
		msg             = gcode.CodeOK.Message()
		data any
		err  = r.GetError()
	)

	if err != nil {
		// 出错：未附带错误码视为内部错误；只对外返回通用文案，细节写日志。
		code = gerror.Code(err)
		if code == gcode.CodeNil {
			code = gcode.CodeInternalError
		}
		g.Log().Errorf(r.Context(), "request error: %+v", err)
		msg = code.Message()

	} else if r.Response.Status > 0 && r.Response.Status != http.StatusOK {
		// handler 未返回 error 但显式设置了非 200 状态码（如 WriteStatus(404)），
		// 此时返回 "OK" 会自相矛盾，故映射为业务错误码。
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
		// 成功：取 handler 的第一个返回值作为业务数据。
		data = r.GetHandlerResponse()
	}

	// WriteJson 内部使用 json.Marshal，失败会 panic，data 中不可放入 chan、func 等类型。
	r.Response.WriteJson(Response{
		Code:    code.Code(),
		Message: msg,
		Data:    data,
	})
}

// Response 统一响应结构体。
//
// 约定返回给前端的 JSON 固定为 {code, message, data}，前端依据 code 是否为 0 判断成败，
// 而不依赖 HTTP 状态码（很多场景下状态码固定为 200，业务错误通过 code 表达）。
type Response struct {
	// Code 业务状态码，0 表示成功，非 0 取值来源见 gcode 包。
	Code int `json:"code"    dc:"业务状态码，0 表示成功"`

	// Message 消息提示，为状态码对应的通用文案，不含底层错误的原始信息。
	Message string `json:"message" dc:"消息提示"`

	// Data 业务返回数据，仅在成功时返回；出错时恒为 null。
	Data any `json:"data"    dc:"执行结果"`
}
