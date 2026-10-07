import type { SseEvent, SseEventType } from '../types/sse';

// consumeSseStream 按 SSE 规范逐帧解析响应流，并对每个事件回调 onEvent。
//
// 后端帧格式见 internal/logic/sse/sse.go 的 buildEvent：字段各占一行、空行结束；
// data 字段可重复出现（多行内容），此处用换行还原；未知字段（如 retry）忽略。
export async function consumeSseStream(
  response: Response,
  onEvent: (event: SseEvent) => void,
): Promise<void> {
  if (!response.body) {
    throw new Error('响应不含可读流');
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let currentEvent: SseEventType = 'message';
  let currentData = '';
  let hasData = false;

  const flush = (): void => {
    // 空消息事件（无 data 且为默认 message 类型）不回调。
    if (hasData || currentEvent !== 'message') {
      onEvent({ type: currentEvent, data: currentData });
    }
    currentEvent = 'message';
    currentData = '';
    hasData = false;
  };

  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) {
        break;
      }

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop() ?? '';

      for (const rawLine of lines) {
        const line = rawLine.endsWith('\r') ? rawLine.slice(0, -1) : rawLine;

        if (line === '') {
          flush();
          continue;
        }
        if (line.startsWith('id:')) {
          continue;
        }
        if (line.startsWith('event:')) {
          currentEvent = line.slice(6).trim() as SseEventType;
          continue;
        }
        if (line.startsWith('data:')) {
          const chunk = line.slice(5).replace(/^ /, '');
          currentData = hasData ? `${currentData}\n${chunk}` : chunk;
          hasData = true;
        }
      }
    }

    // 流意外结束且未收到结束空行时，补发残留事件。
    flush();
  } finally {
    reader.releaseLock();
  }
}
