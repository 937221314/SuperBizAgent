import type { ChatRequest } from '../types/api';
import { API_BASE_URL, ApiError } from './http';

// openChatStream 建立流式对话连接，返回原始 Response 交由 SSE 解析器消费。
// 流式响应不包信封，业务错误以 SSE error 事件下发。
export async function openChatStream(req: ChatRequest): Promise<Response> {
  const response = await fetch(`${API_BASE_URL}/chat_stream`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  });

  if (!response.ok || !response.body) {
    throw new ApiError(response.status, `HTTP 错误: ${response.status}`);
  }

  return response;
}
