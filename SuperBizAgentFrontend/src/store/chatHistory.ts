import type { ChatHistory, Message } from '../types/chat';

const STORAGE_KEY = 'chatHistories';
const MAX_HISTORIES = 50;

function isMessage(value: unknown): value is Message {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  const msg = value as Record<string, unknown>;
  return (msg.type === 'user' || msg.type === 'assistant') && typeof msg.content === 'string';
}

function isChatHistory(value: unknown): value is ChatHistory {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  const history = value as Record<string, unknown>;
  return (
    typeof history.id === 'string' &&
    typeof history.title === 'string' &&
    Array.isArray(history.messages) &&
    history.messages.every(isMessage)
  );
}

// loadChatHistories 从 localStorage 读取历史对话，损坏数据按空列表处理。
export function loadChatHistories(): ChatHistory[] {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (!stored) {
      return [];
    }
    const parsed: unknown = JSON.parse(stored);
    if (!Array.isArray(parsed)) {
      return [];
    }
    return parsed.filter(isChatHistory);
  } catch (err) {
    console.error('加载历史对话失败:', err);
    return [];
  }
}

// saveChatHistories 持久化历史对话，最多保留 MAX_HISTORIES 条。
export function saveChatHistories(histories: ChatHistory[]): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(histories.slice(0, MAX_HISTORIES)));
  } catch (err) {
    console.error('保存历史对话失败:', err);
  }
}

// buildHistoryTitle 用首条用户消息生成标题（截断 30 字）。
export function buildHistoryTitle(messages: Message[]): string {
  const firstUser = messages.find((msg) => msg.type === 'user');
  if (!firstUser) {
    return '新对话';
  }
  return firstUser.content.substring(0, 30) + (firstUser.content.length > 30 ? '...' : '');
}

// ChatHistoryStore 管理历史对话列表与持久化。
export class ChatHistoryStore {
  private histories: ChatHistory[];

  constructor() {
    this.histories = loadChatHistories();
  }

  // list 返回当前历史记录列表。
  list(): ChatHistory[] {
    return this.histories;
  }

  // find 按 id 查找历史记录。
  find(id: string): ChatHistory | undefined {
    return this.histories.find((history) => history.id === id);
  }

  // upsert 按 id 新增或更新。
  // 旧实现误在消息数组上按 id 查重，这里改为在历史列表中查重。
  upsert(history: ChatHistory): void {
    const index = this.histories.findIndex((item) => item.id === history.id);
    if (index === -1) {
      this.histories.unshift(history);
    } else {
      const existing = this.histories[index];
      this.histories[index] = { ...history, createdAt: existing?.createdAt ?? history.createdAt };
    }
    if (this.histories.length > MAX_HISTORIES) {
      this.histories.length = MAX_HISTORIES;
    }
    this.persist();
  }

  // remove 删除指定历史记录。
  remove(id: string): void {
    this.histories = this.histories.filter((history) => history.id !== id);
    this.persist();
  }

  // persist 写入 localStorage。
  persist(): void {
    saveChatHistories(this.histories);
  }
}
