# SuperBizAgent

基于 Go 构建的智能业务代理（Business Agent）项目。

## 简介

SuperBizAgent 是一个使用 Go 语言开发的业务代理服务。当前已具备：

- **SSE 流式推送**：`internal/logic/sse` 提供客户端生命周期管理与消息投递；
- **Milvus 向量能力**：`internal/ai` 下提供文档加载、文本向量化、向量索引与检索组件；
- **统一配置加载**：`internal/config` 按「环境变量 > 配置文件 > 代码默认值」合并配置；
- **AI 工具集**：`internal/ai/tools` 下提供 MySQL 读写、内部文档检索、当前时间与 Prometheus 告警查询工具；
  工具已具备构造与单测，尚未挂载到 ToolsNode（见 `dev-docs/todo.md`）；
- **HTTP 中间件与日志回调**：`utility/middleware`、`utility/logcallback`。

## 环境要求

- Go 1.27.1 或更高版本

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

## 常用命令

```bash
make check   # fmt + vet + lint + test，提交前执行
make build   # 构建全部包
make vet     # go vet ./...
make lint    # golangci-lint run ./...
make test    # go test -race ./...
make fmt     # gofmt -w .
```

## 目录结构

```
SuperBizAgent/
├── api/                # API 接口定义
│   └── chat/           # 聊天相关接口
├── dev-docs/           # 开发文档（本地维护，不纳入版本控制）
├── docs/               # 知识库文档目录（运行期数据，不纳入版本控制）
├── hack/               # 构建、脚本等工具
├── internal/           # 内部实现（不对外暴露）
│   ├── ai/             # AI 组件
│   │   ├── embedder/   # 文本向量模型
│   │   ├── indexer/    # Milvus 索引器
│   │   ├── loader/     # 文档加载器
│   │   ├── retriever/  # Milvus 检索器
│   │   └── tools/      # AI 工具（每个工具各自成子包）
│   │       ├── currenttime/  # get_current_time：当前时间
│   │       ├── docsearch/    # query_internal_docs：内部文档检索
│   │       ├── mysql/        # mysql_query / mysql_exec：MySQL 读写
│   │       ├── prometheus/   # query_prometheus_alerts：Prometheus 活跃告警
│   │       └── query_log.go  # 日志类 MCP 工具获取
│   ├── config/         # 配置加载与默认值
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
- `manifest/config/config.yaml` 含明文密码，已加入 `.gitignore`，不要提交。

## 开发约定

- 本项目**优先使用中文交流**，详见 [AGENTS.md](./AGENTS.md)。
- 提交前确保 `go build ./...` 通过，并执行 `make check`（fmt/vet/lint/test）。
- 提交信息使用语义化前缀（`feat:`、`fix:`、`chore:`、`docs:` 等）。
- `dev-docs/` 存放开发文档，`docs/` 被程序用作知识库文档目录，两者内容均不纳入版本控制
  （`docs/` 仅保留 `.gitkeep` 占位）。

## 许可证

待补充。
