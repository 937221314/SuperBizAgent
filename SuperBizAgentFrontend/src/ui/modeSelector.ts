import type { Mode } from '../types/chat';

const MODE_NAMES: Record<Mode, string> = {
  quick: '快速',
  stream: '流式',
};

// ModeSelector 管理对话模式下拉菜单。
export class ModeSelector {
  constructor(
    private readonly button: HTMLElement,
    private readonly dropdown: HTMLElement,
    private readonly textEl: HTMLElement,
    private readonly onChange: (mode: Mode) => void,
  ) {}

  // bind 绑定按钮与下拉项事件。
  bind(): void {
    this.button.addEventListener('click', (event) => {
      event.stopPropagation();
      this.toggle();
    });

    this.dropdown.querySelectorAll<HTMLElement>('.dropdown-item').forEach((item) => {
      item.addEventListener('click', () => {
        const mode = item.dataset.mode;
        if (mode === 'quick' || mode === 'stream') {
          this.onChange(mode);
        }
        this.close();
      });
    });
  }

  // toggle 切换下拉菜单显示状态。
  toggle(): void {
    this.wrapper()?.classList.toggle('active');
  }

  // close 关闭下拉菜单。
  close(): void {
    this.wrapper()?.classList.remove('active');
  }

  // isInside 判断事件目标是否位于下拉菜单内（用于点击外部关闭）。
  isInside(target: EventTarget | null): boolean {
    return target instanceof Node && (this.button.contains(target) || this.dropdown.contains(target));
  }

  // setMode 同步当前模式到按钮文案与下拉选中态。
  setMode(mode: Mode): void {
    this.textEl.textContent = MODE_NAMES[mode];
    this.dropdown.querySelectorAll<HTMLElement>('.dropdown-item').forEach((item) => {
      item.classList.toggle('active', item.dataset.mode === mode);
    });
  }

  private wrapper(): HTMLElement | null {
    return this.button.closest('.mode-selector-wrapper');
  }
}
