export type NotificationType = 'info' | 'success' | 'warning' | 'error';

const COLORS: Record<NotificationType, string> = {
  info: '#1a73e8',
  success: '#34a853',
  warning: '#fbbc04',
  error: '#ea4335',
};

// 通知动画在 JS 注入，避免依赖 styles.css 中是否已定义对应 keyframes。
const style = document.createElement('style');
style.textContent = `
  @keyframes slideIn {
    from { transform: translateX(100%); opacity: 0; }
    to { transform: translateX(0); opacity: 1; }
  }
  @keyframes slideOut {
    from { transform: translateX(0); opacity: 1; }
    to { transform: translateX(100%); opacity: 0; }
  }
`;
document.head.appendChild(style);

// showNotification 弹出右上角提示，3 秒后自动消失。
export function showNotification(message: string, type: NotificationType = 'info'): void {
  const notification = document.createElement('div');
  notification.className = `notification ${type}`;
  notification.textContent = message;
  notification.style.cssText = `
    position: fixed;
    top: 20px;
    right: 20px;
    padding: 15px 20px;
    border-radius: 8px;
    color: white;
    font-weight: 500;
    z-index: 10000;
    animation: slideIn 0.3s ease;
    max-width: 300px;
  `;
  notification.style.backgroundColor = COLORS[type];
  document.body.appendChild(notification);

  window.setTimeout(() => {
    notification.style.animation = 'slideOut 0.3s ease';
    window.setTimeout(() => notification.remove(), 300);
  }, 3000);
}
