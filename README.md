# SuperBizAgent

基于 Go 构建的智能业务代理（Business Agent）项目。

## 简介

SuperBizAgent 是一个使用 Go 语言开发的业务代理服务。当前已具备：

- **SSE 流式推送**：`internal/logic/sse` 提供客户端生命周期管理与消息投递；
- **Milvus 向量能力**：`internal/ai` 下提供文档加载、文本向量化、向量索引与检索组件；
  知识库入库支持 `.md`/`.txt`/`.pdf`/`.docx`（`.doc` 暂不支持，需先转 `.docx`），
  入库流程与切分参数见 `dev-docs/knowledge-index.md`；
- **统一配置加载**：`internal/config` 按「环境变量 > 配置文件 > 代码默认值」合并配置；
- **AI 工具集**：`internal/ai/tools` 下提供 MySQL 读写、内部文档检索、当前时间与 Prometheus 告警查询工具；
  工具已具备构造与单测，尚未挂载到 ToolsNode（见 `dev-docs/todo.md`）；
- **聊天 HTTP 接口**：`api/chat` 与 `internal/controller/chat` 定义对话、SSE 流式、文件上传与 AI 运维接口；
  `main.go` 已装配 HTTP 服务并监听 `:6872`，响应统一由 `utility/middleware` 包裹为 `{code,message,data}`；
- **前端界面**：`SuperBizAgentFrontend/` 提供基于 Vite + TypeScript 的对话界面（快速/流式对话、历史记录、文件上传、AI Ops）；
- **HTTP 中间件与日志回调**：`utility/middleware`、`utility/logcallback`。

## 技术栈

### 后端

| 类别 | 选型 |
| --- | --- |
| 语言 | Go 1.27.1 |
| Web 框架 | GoFrame v2（`ghttp`，统一响应与 CORS 中间件） |
| 智能体编排 | CloudWeGo Eino（`compose` / `adk`：RAG、ReAct、plan-execute-replan） |
| 模型接入 | OpenAI 兼容协议（`eino-ext` embedding / model，DashScope） |
| 向量库 | Milvus（`milvus-io/milvus/client/v2`） |
| 关系库 | MySQL（GORM） |
| 外部集成 | Prometheus 告警查询、MCP（`mark3labs/mcp-go`） |
| 实时推送 | SSE（`internal/logic/sse`） |
| 配置加载 | YAML + 环境变量（`yaml.v3`、`joho/godotenv`） |

### 前端

| 类别 | 选型 |
| --- | --- |
| 构建工具 | Vite |
| 语言 | TypeScript（`strict`） |
| 渲染方式 | 原生 DOM（未引入前端框架） |
| Markdown | marked + highlight.js + DOMPurify |
| 数据持久化 | 浏览器 localStorage |

## 环境要求

- Go 1.27.1 或更高版本
- Node.js 20+（仅前端开发与构建需要）

## 快速开始

```bash
# 克隆仓库
git clone <repository-url>
cd SuperBizAgent

# 制备本地配置（含明文密钥，已在 .gitignore 中忽略）
cp manifest/config/config.example.yaml manifest/config/config.yaml
# 按实际环境修改 manifest/config/config.yaml

# 运行
go run .

# 构建
go build -o SuperBizAgent .
```

## 前端

对话界面位于 `SuperBizAgentFrontend/`，技术栈为 Vite + TypeScript（详见其 [README](./SuperBizAgentFrontend/README.md)）。

```bash
cd SuperBizAgentFrontend
npm install        # 首次安装依赖
npm run dev        # 开发服务器（默认 http://localhost:5173）
npm run typecheck  # 类型检查
npm run build      # 构建，产出 dist/
```

- 后端默认地址 `http://localhost:6872/api`，在 `SuperBizAgentFrontend/src/api/http.ts` 中配置。
- 前端为独立静态工程，由静态服务器托管 `dist/`；Go 后端不托管前端静态资源。

## 常用命令

```bash
make check              # fmt + vet + lint + test，提交前执行
make build              # 构建全部包
make vet                # go vet ./...
make lint               # golangci-lint run ./...
make test               # go test -race ./...
make fmt                # gofmt -w .
make frontend-install   # 前端安装依赖（首次）
make frontend-typecheck # 前端类型检查
make frontend-build     # 前端构建（产出 SuperBizAgentFrontend/dist/）
```

## 目录结构

```
SuperBizAgent/
├── api/                # API 接口定义
│   └── chat/           # 聊天相关接口
│       └── v1/         # 请求 / 响应结构定义
├── dev-docs/           # 开发文档（本地维护，不纳入版本控制）
├── docs/               # 知识库文档目录（运行期数据，不纳入版本控制）
├── hack/               # 构建、脚本等工具
├── internal/           # 内部实现（不对外暴露）
│   ├── ai/             # AI 组件
│   │   ├── agent/      # 多智能体与流水线编排
│   │   │   ├── chat_pipeline/           # 对话流水线（RAG + ReAct）
│   │   │   ├── knowledge_index_pipeline/ # 知识库入库流水线
│   │   │   └── plan_execute_replan/     # plan-execute-replan 多智能体
│   │   ├── cmd/        # 命令行调试入口
│   │   │   ├── chat_cmd/      # 对话调试命令
│   │   │   └── knowledge_cmd/ # 知识库入库命令
│   │   ├── embedder/   # 文本向量模型
│   │   ├── indexer/    # Milvus 索引器
│   │   ├── loader/     # 文档加载器
│   │   ├── models/     # 模型构造器（OpenAI 兼容协议）
│   │   ├── retriever/  # Milvus 检索器
│   │   └── tools/      # AI 工具（每个工具各自成子包）
│   │       ├── currenttime/  # get_current_time：当前时间
│   │       ├── docsearch/    # query_internal_docs：内部文档检索
│   │       ├── mysql/        # mysql_query / mysql_exec：MySQL 读写
│   │       ├── prometheus/   # query_prometheus_alerts：Prometheus 活跃告警
│   │       └── query_log.go  # 日志类 MCP 工具获取
│   ├── config/         # 配置加载与默认值
│   ├── controller/     # HTTP 控制器
│   │   └── chat/       # 聊天接口（对话 / 流式 / 上传 / AIOps）
│   └── logic/          # 业务逻辑
│       └── sse/        # SSE 流式处理
├── manifest/           # 配置与清单
│   └── config/         # 配置示例与本地配置
├── utility/            # 通用工具
│   ├── client/         # Milvus 客户端与数据库初始化
│   ├── common/         # 公共常量与路径
│   ├── logcallback/    # eino 日志回调
│   ├── mem/            # 会话记忆
│   └── middleware/     # HTTP 中间件
├── SuperBizAgentFrontend/  # 前端对话界面（Vite + TypeScript）
├── main.go             # 程序入口
├── go.mod              # Go 模块定义
├── Makefile            # 常用构建与检查命令
├── .golangci.yml       # golangci-lint 配置
├── .gitignore          # Git 忽略规则
└── AGENTS.md           # 项目协作约定
```

## 配置

- 配置由 `internal/config` 加载，默认读取 `manifest/config/config.yaml`，可用 `CONFIG_PATH` 指定其它路径。
- 优先级：**环境变量 > 配置文件 > 代码默认值**；配置文件不存在时使用内置默认值。
- 示例见 `manifest/config/config.example.yaml`，环境变量与配置项清单详见 `dev-docs/configuration.md`。
- 工具相关配置项：`mysql.dangerous_statement_mode`（`mysql_exec` 遇到 DROP/TRUNCATE 的行为）、
  `prometheus.base_url`（告警查询地址）、`mcp_url`（日志类 MCP 服务地址）。
- 知识库切分：`knowledge_chunk.chunk_size`（分片最大字符数）、`knowledge_chunk.overlap_size`
  （相邻分片重叠字符数），仅 Markdown 之外格式的递归切分使用。
- `manifest/config/config.yaml` 含明文密码，已加入 `.gitignore`，不要提交。

## 开发约定

- 本项目**优先使用中文交流**，详见 [AGENTS.md](./AGENTS.md)。
- 提交前确保 `go build ./...` 通过，并执行 `make check`（fmt/vet/lint/test）。
  改动前端时另需 `make frontend-typecheck`（必要时 `make frontend-build`）。
- 提交信息使用语义化前缀（`feat:`、`fix:`、`chore:`、`docs:` 等）。
- `dev-docs/` 存放开发文档，`docs/` 被程序用作知识库文档目录，两者内容均不纳入版本控制
  （`docs/` 仅保留 `.gitkeep` 占位）。

## 许可证

待补充。
