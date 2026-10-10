import { highlightCodeBlocks, renderMarkdown } from '../core/markdown';
import type { Role } from '../types/chat';

const ASSISTANT_AVATAR = `
  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
    <path d="M12 2L15.09 8.26L22 9.27L17 14.14L18.18 21.02L12 17.77L5.82 21.02L7 14.14L2 9.27L8.91 8.26L12 2Z" fill="white"/>
  </svg>
`;

const LOADING_ICON = `
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 18c-4.41 0-8-3.59-8-8s3.59-8 8-8 8 3.59 8 8-3.59 8-8 8z" fill="currentColor" opacity="0.2"/>
    <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10c1.54 0 3-.36 4.28-1l-1.5-2.6C13.64 19.62 12.84 20 12 20c-4.41 0-8-3.59-8-8s3.59-8 8-8c.84 0 1.64.38 2.18 1l1.5-2.6C13 2.36 12.54 2 12 2z" fill="currentColor"/>
  </svg>
`;

// MessageList 负责消息列表渲染、居中切换与滚动，不持有会话历史状态。
export class MessageList {
  constructor(
    private readonly container: HTMLElement,
    private readonly chatContainer: HTMLElement | null,
  ) {}

  // clear 清空消息列表。
  clear(): void {
    this.container.innerHTML = '';
  }

  // count 返回当前消息数量。
  count(): number {
    return this.container.querySelectorAll('.message').length;
  }

  // add 渲染一条消息并返回其元素；isStreaming 时以纯文本占位，结束时再渲染 Markdown。
  add(type: Role, content: string, isStreaming = false): HTMLElement {
    const isFirstMessage = this.count() === 0;

    const messageDiv = document.createElement('div');
    messageDiv.className = `message ${type}${isStreaming ? ' streaming' : ''}`;

    if (type === 'assistant') {
      const avatar = document.createElement('div');
      avatar.className = 'message-avatar';
      avatar.innerHTML = ASSISTANT_AVATAR;
      messageDiv.appendChild(avatar);
    }

    const wrapper = document.createElement('div');
    wrapper.className = 'message-content-wrapper';

    const contentEl = document.createElement('div');
    contentEl.className = 'message-content';
    if (type === 'assistant' && !isStreaming) {
      contentEl.innerHTML = renderMarkdown(content);
      highlightCodeBlocks(contentEl);
    } else {
      contentEl.textContent = content;
    }

    wrapper.appendChild(contentEl);
    messageDiv.appendChild(wrapper);
    this.container.appendChild(messageDiv);

    if (isFirstMessage && this.chatContainer) {
      this.chatContainer.classList.remove('centered');
      this.chatContainer.style.transition = 'all 0.5s ease';
    }
    this.scrollToBottom();
    return messageDiv;
  }

  // addLoading 渲染带旋转图标的加载消息。
  addLoading(content: string): HTMLElement {
    const isFirstMessage = this.count() === 0;

    const messageDiv = document.createElement('div');
    messageDiv.className = 'message assistant';

    const avatar = document.createElement('div');
    avatar.className = 'message-avatar';
    avatar.innerHTML = ASSISTANT_AVATAR;
    messageDiv.appendChild(avatar);

    const wrapper = document.createElement('div');
    wrapper.className = 'message-content-wrapper';

    const contentEl = document.createElement('div');
    contentEl.className = 'message-content loading-message-content';

    const textSpan = document.createElement('span');
    textSpan.textContent = content;

    const loadingIcon = document.createElement('span');
    loadingIcon.className = 'loading-spinner-icon';
    loadingIcon.innerHTML = LOADING_ICON;

    contentEl.appendChild(textSpan);
    contentEl.appendChild(loadingIcon);
    wrapper.appendChild(contentEl);
    messageDiv.appendChild(wrapper);
    this.container.appendChild(messageDiv);

    if (isFirstMessage && this.chatContainer) {
      this.chatContainer.classList.remove('centered');
      this.chatContainer.style.transition = 'all 0.5s ease';
    }
    this.scrollToBottom();
    return messageDiv;
  }

  // updateStreaming 更新流式消息的纯文本内容。
  updateStreaming(messageElement: HTMLElement, text: string): void {
    const contentEl = messageElement.querySelector('.message-content');
    if (contentEl) {
      contentEl.textContent = text;
    }
    this.scrollToBottom();
  }

  // finalizeStreaming 结束流式状态并把最终内容渲染为 Markdown。
  finalizeStreaming(messageElement: HTMLElement, content: string): void {
    messageElement.classList.remove('streaming');
    const contentEl = messageElement.querySelector<HTMLElement>('.message-content');
    if (contentEl) {
      contentEl.innerHTML = renderMarkdown(content);
      highlightCodeBlocks(contentEl);
    }
  }

  // failStreaming 结束流式状态并展示失败原因（用于 SSE error 事件）。
  failStreaming(messageElement: HTMLElement, reason: string): void {
    messageElement.classList.remove('streaming');
    const contentEl = messageElement.querySelector('.message-content');
    if (contentEl) {
      contentEl.textContent = `抱歉，本次回答失败：${reason}`;
    }
    this.scrollToBottom();
  }

  // updateCentered 按是否有消息切换居中样式（欢迎语仅在居中态可见）。
  updateCentered(): void {
    if (!this.chatContainer) {
      return;
    }
    if (this.count() > 0) {
      this.chatContainer.classList.remove('centered');
    } else {
      this.chatContainer.classList.add('centered');
    }
  }

  // scrollToBottom 滚动到底部。
  scrollToBottom(): void {
    this.container.scrollTop = this.container.scrollHeight;
  }
}
