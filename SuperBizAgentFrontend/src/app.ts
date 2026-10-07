import { requestAIOps } from './api/aiOps';
import { sendChat } from './api/chat';
import { openChatStream } from './api/chatStream';
import { uploadKnowledgeFile } from './api/upload';
import { consumeSseStream } from './sse/parser';
import { ChatHistoryStore, buildHistoryTitle } from './store/chatHistory';
import { updateAIOpsMessage } from './ui/aiOpsPanel';
import { ChatHistoryList } from './ui/chatHistoryList';
import { MessageList } from './ui/messageList';
import { ModeSelector } from './ui/modeSelector';
import { showNotification } from './ui/notifications';
import { hideOverlay, showOverlay } from './ui/overlay';
import { ToolsMenu } from './ui/toolsMenu';
import type { ChatHistory, Message, Mode, Role } from './types/chat';
import { getEl, getElOrNull } from './utils/dom';

const ALLOWED_EXTENSIONS = ['.txt', '.md', '.markdown'];
const MAX_FILE_SIZE = 50 * 1024 * 1024;

// isAllowedFile 校验上传文件类型。
function isAllowedFile(file: File): boolean {
  const name = file.name.toLowerCase();
  return ALLOWED_EXTENSIONS.some((ext) => name.endsWith(ext));
}

// generateSessionId 生成随机会话 ID。
function generateSessionId(): string {
  return `session_${Math.random().toString(36).substring(2, 9)}_${Date.now()}`;
}

// SuperBizAgentApp 前端应用编排层：持有状态与各 UI 模块，串联接口调用。
export class SuperBizAgentApp {
  private readonly store = new ChatHistoryStore();
  private readonly messageList: MessageList;
  private readonly historyList: ChatHistoryList;
  private readonly modeSelector: ModeSelector;
  private readonly toolsMenu: ToolsMenu;

  private readonly messageInput = getEl<HTMLInputElement>('messageInput');
  private readonly sendButton = getEl<HTMLButtonElement>('sendButton');
  private readonly fileInput = getEl<HTMLInputElement>('fileInput');
  private readonly aiOpsButton = getEl<HTMLButtonElement>('aiOpsSidebarBtn');
  private readonly newChatButton = getEl<HTMLButtonElement>('newChatBtn');
  private readonly chatMessages = getEl<HTMLElement>('chatMessages');
  private readonly chatContainer = getElOrNull<HTMLElement>('chatContainer');

  private mode: Mode = 'quick';
  private sessionId = generateSessionId();
  private isStreaming = false;
  private currentMessages: Message[] = [];
  private fromHistory = false;

  constructor() {
    this.messageList = new MessageList(this.chatMessages, this.chatContainer);
    this.historyList = new ChatHistoryList(getEl('chatHistoryList'), {
      onSelect: (id) => this.loadChatHistory(id),
      onDelete: (id) => this.deleteChatHistory(id),
    });
    this.modeSelector = new ModeSelector(
      getEl('modeSelectorBtn'),
      getEl('modeDropdown'),
      getEl('currentModeText'),
      (mode) => this.selectMode(mode),
    );
    this.toolsMenu = new ToolsMenu(getEl('toolsBtn'), getEl('toolsMenu'), () => this.fileInput.click());

    this.bindEvents();
    this.updateUI();
    this.messageList.updateCentered();
    this.renderHistory();
  }

  // bindEvents 绑定全局与输入区事件。
  private bindEvents(): void {
    this.newChatButton.addEventListener('click', () => this.newChat());
    this.aiOpsButton.addEventListener('click', () => void this.triggerAIOps());
    this.sendButton.addEventListener('click', () => void this.sendMessage());
    this.fileInput.addEventListener('change', (event) => this.handleFileSelect(event));

    this.messageInput.addEventListener('keypress', (event) => {
      if (event.key === 'Enter') {
        event.preventDefault();
        void this.sendMessage();
      }
    });

    this.modeSelector.bind();
    this.toolsMenu.bind();

    document.addEventListener('click', (event) => {
      if (!this.modeSelector.isInside(event.target)) {
        this.modeSelector.close();
      }
      if (!this.toolsMenu.isInside(event.target)) {
        this.toolsMenu.close();
      }
    });
  }

  // updateUI 同步模式、发送按钮与输入框的可用状态。
  private updateUI(): void {
    this.modeSelector.setMode(this.mode);
    this.sendButton.disabled = this.isStreaming;
    this.messageInput.disabled = this.isStreaming;
    this.messageInput.placeholder = '问问智能 OnCall 助手';
  }

  // renderHistory 重绘历史对话列表。
  private renderHistory(): void {
    this.historyList.render(this.store.list());
  }

  // appendMessage 渲染消息，并在非流式占位时写入当前会话历史。
  private appendMessage(type: Role, content: string, streaming = false): HTMLElement {
    if (!streaming && content) {
      this.currentMessages.push({ type, content, timestamp: new Date().toISOString() });
    }
    return this.messageList.add(type, content, streaming);
  }

  // persistCurrentChat 把当前会话写入历史（按 id 新增或更新标题/消息）。
  private persistCurrentChat(): void {
    if (this.currentMessages.length === 0) {
      return;
    }
    const now = new Date().toISOString();
    const existing = this.store.find(this.sessionId);
    const history: ChatHistory = {
      id: this.sessionId,
      title: buildHistoryTitle(this.currentMessages),
      messages: [...this.currentMessages],
      createdAt: existing?.createdAt ?? now,
      updatedAt: now,
    };
    this.store.upsert(history);
  }

  // newChat 新建对话，并保存上一段未落库的会话。
  private newChat(): void {
    if (this.isStreaming) {
      showNotification('请等待当前对话完成后再新建对话', 'warning');
      return;
    }

    this.persistCurrentChat();

    this.isStreaming = false;
    this.messageInput.value = '';
    this.currentMessages = [];
    this.fromHistory = false;
    this.messageList.clear();
    this.sessionId = generateSessionId();
    this.mode = 'quick';
    this.updateUI();
    this.messageList.updateCentered();
    if (this.chatContainer) {
      this.chatContainer.style.transition = 'all 0.5s ease';
    }
    this.renderHistory();
  }

  // loadChatHistory 加载指定历史对话。
  private loadChatHistory(historyId: string): void {
    const history = this.store.find(historyId);
    if (!history) {
      return;
    }

    if (this.currentMessages.length > 0 && this.sessionId !== historyId) {
      this.persistCurrentChat();
    }

    this.sessionId = history.id;
    this.currentMessages = [...history.messages];
    this.fromHistory = true;

    this.messageList.clear();
    for (const message of history.messages) {
      this.messageList.add(message.type, message.content);
    }
    this.messageList.updateCentered();
    this.renderHistory();
  }

  // deleteChatHistory 删除历史对话；若为当前会话则重置为空会话。
  private deleteChatHistory(historyId: string): void {
    this.store.remove(historyId);
    this.renderHistory();

    if (this.sessionId === historyId) {
      this.currentMessages = [];
      this.messageList.clear();
      this.sessionId = generateSessionId();
      this.fromHistory = false;
      this.messageList.updateCentered();
    }
  }

  // selectMode 切换对话模式。
  private selectMode(mode: Mode): void {
    if (this.isStreaming) {
      showNotification('请等待当前对话完成后再切换模式', 'warning');
      return;
    }
    this.mode = mode;
    this.updateUI();
    const names: Record<Mode, string> = { quick: '快速', stream: '流式' };
    showNotification(`已切换到${names[mode]}模式`, 'info');
  }

  // sendMessage 发送当前输入框内容。
  private async sendMessage(): Promise<void> {
    const message = this.messageInput.value.trim();
    if (!message) {
      showNotification('请输入消息内容', 'warning');
      return;
    }
    if (this.isStreaming) {
      showNotification('请等待当前对话完成', 'warning');
      return;
    }

    this.appendMessage('user', message);
    this.messageInput.value = '';
    this.isStreaming = true;
    this.updateUI();

    try {
      if (this.mode === 'quick') {
        await this.sendQuick(message);
      } else {
        await this.sendStream(message);
      }
    } catch (err) {
      console.error('发送消息失败', err);
      const reason = err instanceof Error ? err.message : String(err);
      this.appendMessage('assistant', `抱歉，发送消息时出现错误:${reason}`);
    } finally {
      this.isStreaming = false;
      this.updateUI();
      if (this.fromHistory) {
        this.persistCurrentChat();
        this.renderHistory();
      }
    }
  }

  // sendQuick 非流式对话。
  private async sendQuick(message: string): Promise<void> {
    const res = await sendChat({ id: this.sessionId, question: message });
    this.appendMessage('assistant', res.answer);
  }

  // sendStream 流式对话：逐帧渲染，结束时渲染 Markdown 并落库。
  private async sendStream(message: string): Promise<void> {
    const response = await openChatStream({ id: this.sessionId, question: message });
    const placeholder = this.messageList.add('assistant', '', true);
    const state = { text: '', error: null as string | null };

    await consumeSseStream(response, (event) => {
      switch (event.type) {
        case 'message':
          state.text += event.data;
          this.messageList.updateStreaming(placeholder, state.text);
          break;
        case 'error':
          state.error = event.data;
          break;
        default:
          break;
      }
    });

    if (state.error !== null) {
      this.messageList.failStreaming(placeholder, state.error);
      return;
    }

    this.messageList.finalizeStreaming(placeholder, state.text);
    if (state.text) {
      this.currentMessages.push({ type: 'assistant', content: state.text, timestamp: new Date().toISOString() });
      if (this.fromHistory) {
        this.persistCurrentChat();
        this.renderHistory();
      }
    }
  }

  // handleFileSelect 处理文件选择。
  private handleFileSelect(event: Event): void {
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) {
      return;
    }
    if (!isAllowedFile(file)) {
      showNotification('只支持上传 TXT 或 Markdown (.md) 格式的文件', 'error');
      this.fileInput.value = '';
      return;
    }
    void this.uploadFile(file);
  }

  // uploadFile 上传文件到知识库。
  private async uploadFile(file: File): Promise<void> {
    if (!isAllowedFile(file)) {
      showNotification('只支持上传 TXT 或 Markdown (.md) 格式的文件', 'error');
      return;
    }
    if (file.size > MAX_FILE_SIZE) {
      showNotification('文件大小不能超过50MB', 'error');
      return;
    }

    this.isStreaming = true;
    this.updateUI();
    showOverlay('正在上传文件...', `上传: ${file.name}`);

    try {
      await uploadKnowledgeFile(file);
      this.appendMessage('assistant', `${file.name} 上传到知识库成功`);
    } catch (err) {
      console.error('文件上传失败:', err);
      const reason = err instanceof Error ? err.message : String(err);
      showNotification(`文件上传失败: ${reason}`, 'error');
    } finally {
      this.fileInput.value = '';
      this.isStreaming = false;
      hideOverlay();
      this.updateUI();
    }
  }

  // triggerAIOps 触发 AI 运维分析。
  private async triggerAIOps(): Promise<void> {
    if (this.isStreaming) {
      showNotification('请等待当前操作完成', 'warning');
      return;
    }

    this.newChat();
    const loading = this.messageList.addLoading('分析中...');
    this.isStreaming = true;
    this.updateUI();

    try {
      const data = await requestAIOps();
      updateAIOpsMessage(loading, data.result, data.detail);
      this.currentMessages.push({ type: 'assistant', content: data.result, timestamp: new Date().toISOString() });
    } catch (err) {
      console.error('智能运维分析失败:', err);
      const reason = err instanceof Error ? err.message : String(err);
      const contentEl = loading.querySelector('.message-content');
      if (contentEl) {
        contentEl.textContent = `抱歉，智能运维分析时出现错误：${reason}`;
      }
    } finally {
      this.isStreaming = false;
      this.updateUI();
    }
  }
}
