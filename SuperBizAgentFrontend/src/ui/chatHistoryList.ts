import { escapeHtml } from '../core/markdown';
import type { ChatHistory } from '../types/chat';

export interface ChatHistoryHandlers {
  onSelect: (id: string) => void;
  onDelete: (id: string) => void;
}

// ChatHistoryList 渲染左侧近期对话列表。
export class ChatHistoryList {
  constructor(
    private readonly container: HTMLElement,
    private readonly handlers: ChatHistoryHandlers,
  ) {}

  // render 全量重绘历史列表。
  render(histories: ChatHistory[]): void {
    this.container.innerHTML = '';

    for (const history of histories) {
      const item = document.createElement('div');
      item.className = 'history-item';
      item.dataset.historyId = history.id;
      item.innerHTML = `
        <div class="history-item-content">
          <span class="history-item-title">${escapeHtml(history.title)}</span>
        </div>
        <button class="history-item-delete" data-history-id="${history.id}" title="删除">
          <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M18 6L6 18M6 6L18 18" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
          </svg>
        </button>
      `;

      item.addEventListener('click', (event) => {
        const target = event.target;
        if (target instanceof Element && target.closest('.history-item-delete')) {
          return;
        }
        this.handlers.onSelect(history.id);
      });

      const deleteBtn = item.querySelector('.history-item-delete');
      deleteBtn?.addEventListener('click', (event) => {
        event.stopPropagation();
        this.handlers.onDelete(history.id);
      });

      this.container.appendChild(item);
    }
  }
}
