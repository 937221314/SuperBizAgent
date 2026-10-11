// ToolsMenu 管理输入区“更多选项”菜单（当前仅含上传文件）。
export class ToolsMenu {
  constructor(
    private readonly button: HTMLElement,
    private readonly menu: HTMLElement,
    private readonly onUpload: () => void,
  ) {}

  // bind 绑定按钮与菜单项事件。
  bind(): void {
    this.button.addEventListener('click', (event) => {
      event.stopPropagation();
      this.toggle();
    });

    this.menu.querySelectorAll<HTMLElement>('.tools-menu-item').forEach((item) => {
      item.addEventListener('click', () => {
        this.onUpload();
        this.close();
      });
    });
  }

  // toggle 切换菜单显示状态。
  toggle(): void {
    this.wrapper()?.classList.toggle('active');
  }

  // close 关闭菜单。
  close(): void {
    this.wrapper()?.classList.remove('active');
  }

  // isInside 判断事件目标是否位于菜单内（用于点击外部关闭）。
  isInside(target: EventTarget | null): boolean {
    return target instanceof Node && (this.button.contains(target) || this.menu.contains(target));
  }

  private wrapper(): HTMLElement | null {
    return this.button.closest('.tools-btn-wrapper');
  }
}
