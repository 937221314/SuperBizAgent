// getEl 按 id 获取元素；元素必然存在时使用，缺失视为编码错误直接抛错。
export function getEl<T extends HTMLElement = HTMLElement>(id: string): T {
  const el = document.getElementById(id);
  if (!el) {
    throw new Error(`找不到元素 #${id}`);
  }
  return el as T;
}

// getElOrNull 按 id 获取元素，不存在时返回 null（用于可选元素）。
export function getElOrNull<T extends HTMLElement = HTMLElement>(id: string): T | null {
  return document.getElementById(id) as T | null;
}
