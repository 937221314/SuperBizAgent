# syntax=docker/dockerfile:1
#
# SuperBizAgent 应用镜像。
#
# 与同目录 docker-compose.yml 的分工：compose 只负责 etcd / minio / milvus / attu 依赖，
# 本文件只构建应用本身；应用运行期需加入 compose 的 `milvus` 网络，并通过
# MILVUS_ADDRESS=standalone:19530 访问向量库（示例配置里的 127.0.0.1 在容器内不可用）。
#
# 构建（build context 必须是仓库根目录，不是本目录）：
#   docker build -f manifest/docker/Dockerfile -t superbizagent:dev .
#
# 国内环境换源：--build-arg GOPROXY=https://goproxy.cn,direct
# 当前 main.go 是占位实现，入口包就绪后改：--build-arg PKG=./internal/ai/cmd/xxx_cmd

# ---------- 构建阶段 ----------
FROM golang:1.27.1-alpine AS builder

ARG GOPROXY=https://proxy.golang.org,direct
# CGO_ENABLED=0 才能落到 alpine 运行阶段（musl 下无 C 依赖，二进制静态链接）
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOPROXY=${GOPROXY}

WORKDIR /src

# 先只拷依赖清单：源码变更不会让依赖下载层缓存失效。
# milvus / eino 依赖体积大，go mod download 单独成层收益明显。
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# PKG 为入口包路径，默认根包；产物统一命名为 app，避免 ARG 无法在 ENTRYPOINT 中展开
ARG PKG=.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/app ${PKG}

# ---------- 运行阶段 ----------
FROM alpine:3.24

# ca-certificates：访问模型 / MCP / Prometheus 的 HTTPS 端点必需
# tzdata：日志与时间工具依赖本地时区，构建期拷 tzdata 比装 tzdata 包更小
RUN apk add --no-cache ca-certificates \
 && adduser -D -u 10001 app

WORKDIR /app

COPY --from=builder /out/app /usr/local/bin/SuperBizAgent

# 镜像内只放示例配置：config.example.yaml 已在版本控制中，不含真实密钥。
# 真实取值由 compose 的 environment（MILVUS_ADDRESS / MILVUS_PASSWORD /
# TEXT_EMBEDDING_API_KEY ...）覆盖，优先级高于配置文件；也可用 volumes 挂载宿主配置。
COPY manifest/config/config.example.yaml /app/manifest/config/config.yaml

# 知识库目录：utility/common.FileDir 固定为 ./docs/，相对 WORKDIR 解析
COPY --chown=app:app docs/ /app/docs/

USER app

# 对应 config.example.yaml 的 server.address；HTTP 服务与这里、compose 的 ports 需同步
EXPOSE 8080

# CONFIG_PATH 显式指定，避免将来调整 WORKDIR 后配置静默丢失（缺文件时程序用默认值不报错）
ENV CONFIG_PATH=/app/manifest/config/config.yaml \
    TZ=Asia/Shanghai

ENTRYPOINT ["/usr/local/bin/SuperBizAgent"]
