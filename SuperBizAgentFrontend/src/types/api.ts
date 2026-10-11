// api.ts 定义后端 HTTP 接口的响应结构，字段与 api/chat/v1/chat.go 保持一致（JSON 均为小写）。

// ApiEnvelope 是 utility/middleware.ResponseMiddleware 统一的响应信封，
// 所有接口都返回该结构，成败由 code 是否为 0 判定。
export interface ApiEnvelope<T> {
  code: number;
  message: string;
  data: T;
}

// ChatRequest 非流式/流式对话的请求体。
export interface ChatRequest {
  id: string;
  question: string;
}

// ChatRes 非流式对话响应。
export interface ChatRes {
  answer: string;
}

// AIOpsRes AI 运维响应。
export interface AIOpsRes {
  result: string;
  detail: string[];
}

// UploadRes 文件上传响应。
export interface UploadRes {
  fileName: string;
  filePath: string;
  fileSize: number;
}
