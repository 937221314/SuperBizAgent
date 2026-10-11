// sse.ts 定义后端 SSE 推送事件的类型。
// 事件名来源见 internal/controller/chat/chat_v1_chat_stream.go 与 internal/logic/sse/sse.go。

// SseEventType SSE 事件名。
export type SseEventType = 'connected' | 'message' | 'done' | 'error';

// SseEvent 一个已解析的 SSE 事件。
export interface SseEvent {
  type: SseEventType;
  data: string;
}
