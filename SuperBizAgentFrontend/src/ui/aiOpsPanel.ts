import { escapeHtml, highlightCodeBlocks, renderMarkdown } from '../core/markdown';

// buildDetailsContainer 构建可折叠的运维步骤详情区域。
function buildDetailsContainer(details: string[]): HTMLElement {
  const detailsContainer = document.createElement('div');
  detailsContainer.className = 'aiops-details';

  const toggle = document.createElement('div');
  toggle.className = 'details-toggle';
  toggle.innerHTML = `
    <svg class="toggle-icon" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M9 18L15 12L9 6" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
    </svg>
    <span>查看详细步骤 (${details.length}条)</span>
  `;

  const content = document.createElement('div');
  content.className = 'details-content';
  details.forEach((detail, index) => {
    const item = document.createElement('div');
    item.className = 'detail-item';
    item.innerHTML = `<strong>步骤 ${index + 1}:</strong> ${escapeHtml(detail)}`;
    content.appendChild(item);
  });

  toggle.addEventListener('click', () => {
    content.classList.toggle('expanded');
    toggle.classList.toggle('expanded');
  });

  detailsContainer.appendChild(toggle);
  detailsContainer.appendChild(content);
  return detailsContainer;
}

// updateAIOpsMessage 用最终结果替换加载中的消息，并按需补上可折叠详情。
export function updateAIOpsMessage(messageElement: HTMLElement, response: string, details: string[]): void {
  messageElement.classList.add('aiops-message');

  const wrapper = messageElement.querySelector('.message-content-wrapper');
  const contentEl = wrapper?.querySelector<HTMLElement>('.message-content');
  if (!wrapper || !contentEl) {
    return;
  }

  contentEl.classList.remove('loading-message-content');
  contentEl.textContent = '';

  if (details.length > 0) {
    const existing = messageElement.querySelector('.aiops-details');
    if (existing) {
      existing.replaceWith(buildDetailsContainer(details));
    } else {
      wrapper.insertBefore(buildDetailsContainer(details), contentEl);
    }
  }

  contentEl.innerHTML = renderMarkdown(response);
  highlightCodeBlocks(contentEl);
}
