// chat.ts 定义前端会话消息与历史记录的类型。

// Role 消息角色。
export type Role = 'user' | 'assistant';

// Mode 对话模式：快速（请求-响应）或流式。
export type Mode = 'quick' | 'stream';

// Message 单条会话消息。
export interface Message {
  type: Role;
  content: string;
  timestamp: string;
}

// ChatHistory 一次完整对话。
export interface ChatHistory {
  id: string;
  title: string;
  messages: Message[];
  createdAt: string;
  updatedAt: string;
}
