import type { ApiEnvelope } from '../types/api';

// API_BASE_URL 后端地址，与 main.go 的 s.SetPort(6872) 及 /api 分组保持一致。
export const API_BASE_URL = 'http://localhost:6872/api';

// ApiError 表示后端以 {code,message} 返回的业务错误，或 HTTP 层错误。
export class ApiError extends Error {
  readonly code: number;

  constructor(code: number, message: string) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
  }
}

// requestJson 统一请求后端并拆解 {code,message,data} 信封。
// 后端业务错误同样返回 HTTP 200，故只以 code === 0 判定成功，不能依赖 HTTP 状态码。
export async function requestJson<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${path}`, init);

  let envelope: ApiEnvelope<T>;
  try {
    envelope = (await response.json()) as ApiEnvelope<T>;
  } catch {
    throw new ApiError(response.status, `HTTP 错误: ${response.status}`);
  }

  if (envelope.code !== 0) {
    throw new ApiError(envelope.code, envelope.message || '未知错误');
  }

  return envelope.data;
}
