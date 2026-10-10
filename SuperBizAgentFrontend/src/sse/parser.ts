import type { SseEvent, SseEventType } from '../types/sse';

// consumeSseStream 按 SSE 规范逐帧解析响应流，并对每个事件回调 onEvent。
//
// 后端帧格式见 internal/logic/sse/sse.go 的 buildEvent：字段各占一行、空行结束；
// data 字段可重复出现（多行内容），此处用换行还原；未知字段（如 retry）忽略。
//
// onEvent 返回 false 时停止消费并取消底层读取，从而主动关闭连接（后端的 Run 依赖
// request context 取消退出）；用于 done/error 这类业务级结束信号，避免只能等 EOF。
export async function consumeSseStream(
  response: Response,
  onEvent: (event: SseEvent) => boolean | void,
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
  let stopped = false;

  const flush = (): void => {
    // 空消息事件（无 data 且为默认 message 类型）不回调。
    if (hasData || currentEvent !== 'message') {
      if (onEvent({ type: currentEvent, data: currentData }) === false) {
        stopped = true;
      }
    }
    currentEvent = 'message';
    currentData = '';
    hasData = false;
  };

  try {
    for (;;) {
      if (stopped) {
        break;
      }
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
          if (stopped) {
            break;
          }
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

    // 正常 EOF 且未收到结束空行时补发残留事件；提前停止时不再回调。
    if (!stopped) {
      flush();
    }
  } finally {
    if (stopped) {
      // 主动取消读取以关闭连接；取消失败不影响调用方，忽略即可。
      await reader.cancel().catch(() => undefined);
    }
    reader.releaseLock();
  }
}
