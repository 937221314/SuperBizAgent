# SuperBizAgent 前端

智能 OnCall 助手的对话界面，使用 Vite + TypeScript 构建。

## 功能

- **智能对话**：快速（请求-响应）与流式（SSE）两种模式
- **历史对话**：浏览器本地保存（localStorage，最多 50 条）
- **文件上传**：支持 `.txt` / `.md` / `.markdown` / `.pdf` / `.docx`（`.doc` 需先转 `.docx`），
  单文件上限 50MB，上传后写入知识库
- **AI Ops**：一键触发运维分析，展示可折叠的详细步骤

## 技术选型

- 构建：Vite
- 语言：TypeScript（`strict`）
- Markdown：marked + highlight.js + DOMPurify
- 架构：原生 DOM + 模块化拆分（未引入前端框架）

### 为何暂不引入 React/Vue

当前前端为单页、无路由、状态量中等，且以 Go 团队维护为主，因此选择「Vite + 原生 TS」。
出现以下任一情况时，再评估迁移到前端框架：

- 出现多路由 / 多视图页面
- 交互状态显著复杂化（多处联动、并发会话等）
- 出现多名专职前端贡献者
- 需要组件级自动化测试 / Storybook

迁移时 `src/types`、`src/api`、`src/sse`、`src/core`、`src/store` 均为框架无关层，可直接复用，只需替换 `src/ui` 与入口。

## 目录结构

```
src/
├── app.ts        # 编排层：状态与各模块串联
├── main.ts       # 入口
├── api/          # 后端接口封装（统一按 code === 0 判定成败）
├── core/         # Markdown 渲染与 XSS 净化
├── sse/          # SSE 帧解析
├── store/        # 历史对话持久化
├── types/        # 后端契约与业务类型
├── ui/           # DOM 渲染模块
└── utils/        # 通用工具
```

## 开发

前置：Node.js（建议 20+），并确保 Go 后端已启动（默认 `http://localhost:6872`）。

```bash
npm install        # 安装依赖
npm run dev        # 开发服务器（默认 http://localhost:5173）
npm run typecheck  # 类型检查
npm run build      # 构建，产出 dist/
npm run preview    # 预览构建产物
```

- 后端地址在 `src/api/http.ts` 的 `API_BASE_URL` 修改（默认 `http://localhost:6872/api`）。
- 后端已开启 CORS，开发时可直接跨域访问；如需同源，可启用 `vite.config.ts` 中预留的 `server.proxy`。
- 上传字段名固定为 `file`，对应后端 `GetUploadFile("file")`，不可随意改名。

## 部署

`npm run build` 产出 `dist/`，由独立静态服务器托管（当前 Go 后端不托管前端静态资源）。
`dist/`、`node_modules/` 已由 `.gitignore` 忽略，不入库。
