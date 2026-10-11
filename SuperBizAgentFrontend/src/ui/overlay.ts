// overlay.ts 管理 index.html 中的 #loadingOverlay 全屏遮罩。

// showOverlay 显示遮罩并更新文案，同时锁定页面滚动。
export function showOverlay(text: string, subtext: string): void {
  const overlay = document.getElementById('loadingOverlay');
  if (!overlay) {
    return;
  }
  const textEl = overlay.querySelector('.loading-text');
  const subtextEl = overlay.querySelector('.loading-subtext');
  if (textEl) {
    textEl.textContent = text;
  }
  if (subtextEl) {
    subtextEl.textContent = subtext;
  }
  overlay.style.display = 'flex';
  document.body.style.overflow = 'hidden';
}

// hideOverlay 隐藏遮罩并恢复页面滚动。
export function hideOverlay(): void {
  const overlay = document.getElementById('loadingOverlay');
  if (overlay) {
    overlay.style.display = 'none';
  }
  document.body.style.overflow = '';
}
