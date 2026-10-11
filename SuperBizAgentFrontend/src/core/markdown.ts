import DOMPurify from 'dompurify';
import hljs from 'highlight.js';
import { marked } from 'marked';

marked.setOptions({ breaks: true, gfm: true });

// escapeHtml 把文本转义为安全 HTML（Markdown 不可用时的降级显示）。
export function escapeHtml(text: string): string {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}

// renderMarkdown 渲染 Markdown 并做 XSS 净化；失败时降级为纯文本转义。
// marked v18 已移除 highlight 配置，代码高亮改由 highlightCodeBlocks 完成后处理。
export function renderMarkdown(content: string): string {
  if (!content) {
    return '';
  }
  try {
    const raw = marked.parse(content, { async: false });
    return DOMPurify.sanitize(raw);
  } catch (err) {
    console.error('Markdown 渲染失败:', err);
    return escapeHtml(content);
  }
}

// highlightCodeBlocks 对容器内未高亮的代码块调用 highlight.js。
export function highlightCodeBlocks(container: HTMLElement | null): void {
  if (!container) {
    return;
  }
  container.querySelectorAll<HTMLElement>('pre code').forEach((block) => {
    if (!block.classList.contains('hljs')) {
      hljs.highlightElement(block);
    }
  });
}
