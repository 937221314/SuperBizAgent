import type { ChatRequest, ChatRes } from '../types/api';
import { requestJson } from './http';

// sendChat 发送非流式对话请求。
export function sendChat(req: ChatRequest): Promise<ChatRes> {
  return requestJson<ChatRes>('/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  });
}
