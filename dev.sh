#!/usr/bin/env bash
# 开发启动脚本：后端（dev 模式）+ 前端（Vite）+ 实时日志监听。
#
# 用法：
#   ./dev.sh            # 后端 + 前端 + 日志监听
#   ./dev.sh backend    # 仅后端 + 日志监听
#   ./dev.sh frontend   # 仅前端
#   ./dev.sh help       # 帮助
#
# 端口、日志路径可用环境变量覆盖：BACKEND_PORT / FRONTEND_PORT。
# Ctrl+C 退出时会一并回收本轮启动的进程及其监听端口。

set -uo pipefail

# ---------------- 设置环境变量 ----------------
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRONTEND_DIR="${ROOT_DIR}/SuperBizAgentFrontend"
# 日志目录/文件名与 manifest/config/config.yaml 的 logger.path/logger.file 保持一致。
LOG_DIR="${ROOT_DIR}/logs"
LOG_FILE="${LOG_DIR}/app.log"

# 默认端口与 main.go 的 s.SetPort(6872)、vite.config.ts 的 server.port 一致。
BACKEND_PORT="${BACKEND_PORT:-6872}"
FRONTEND_PORT="${FRONTEND_PORT:-5173}"

# 配置热更新由 gcfg 的文件监听触发：改 manifest/config/config.yaml 后无需重启；
# 环境变量变更仍需重启（热更新只重读配置文件）。

# 后端必须从仓库根目录启动：logger.path（./logs）与配置默认值都是相对进程工作目录的，
# 从别处启动会把日志和知识库目录落到错误位置。
cd "${ROOT_DIR}" || { echo "无法进入项目根目录: ${ROOT_DIR}"; exit 1; }

MODE="${1:-all}"

# 本轮启动的子进程 PID 与开关，供 cleanup 只回收自己拉起的东西。
BACKEND_PID=""
FRONTEND_PID=""
TAIL_PID=""
STARTED_BACKEND=0
STARTED_FRONTEND=0

# ---------------- 打印颜色 ----------------
# 输出被重定向到文件或管道时关闭颜色，避免日志里混入转义序列。
if [[ -t 1 ]]; then
  C_RESET=$'\033[0m'
  C_RED=$'\033[31m'
  C_GREEN=$'\033[32m'
  C_YELLOW=$'\033[33m'
  C_CYAN=$'\033[36m'
else
  C_RESET=""; C_RED=""; C_GREEN=""; C_YELLOW=""; C_CYAN=""
fi

info() { printf '%s[dev]%s %s\n' "${C_CYAN}" "${C_RESET}" "$*"; }
ok()   { printf '%s[ ok]%s %s\n' "${C_GREEN}" "${C_RESET}" "$*"; }
warn() { printf '%s[warn]%s %s\n' "${C_YELLOW}" "${C_RESET}" "$*"; }
fail() { printf '%s[err]%s %s\n' "${C_RED}" "${C_RESET}" "$*" >&2; }

usage() {
  cat <<'EOF'
开发启动脚本

用法: ./dev.sh [all|backend|frontend|help]

  all       启动后端（dev 模式）+ 前端 + 日志监听（默认）
  backend   仅启动后端 + 日志监听
  frontend  仅启动前端

环境变量:
  BACKEND_PORT            后端端口，默认 6872
  FRONTEND_PORT           前端端口，默认 5173
EOF
}

# ---------------- 清理旧程序进程 ----------------
# 按端口回收残留进程：go run 会派生真正的服务二进制、vite 也是 node 子进程，
# 它们不在本脚本的作业表里，只能靠监听端口定位。
kill_by_port() {
  local port="$1" pids
  pids="$(lsof -ti "tcp:${port}" 2>/dev/null)"
  [[ -z "${pids}" ]] && return 0

  warn "端口 ${port} 被占用，回收进程: $(echo "${pids}" | tr '\n' ' ')"
  echo "${pids}" | xargs kill -TERM 2>/dev/null
  local i
  for i in 1 2 3 4 5; do
    sleep 0.4
    lsof -ti "tcp:${port}" >/dev/null 2>&1 || return 0
  done

  pids="$(lsof -ti "tcp:${port}" 2>/dev/null)"
  [[ -n "${pids}" ]] && echo "${pids}" | xargs kill -KILL 2>/dev/null
  return 0
}

# kill_tree 先终止子进程再终止自身：go run / npm 退出不会带走派生的服务进程。
# 末尾的 wait 回收作业状态，否则 shell 会在退出时打印 “Terminated: 15 …” 干扰日志。
kill_tree() {
  local pid="$1"
  [[ -n "${pid}" ]] || return 0
  pkill -TERM -P "${pid}" 2>/dev/null
  kill -TERM "${pid}" 2>/dev/null
  wait "${pid}" 2>/dev/null
  return 0
}

# cleanup 幂等：先摘掉 trap，避免结束时再次被 EXIT 触发。
cleanup() {
  trap - INT TERM EXIT
  printf '\n'
  info "正在停止开发环境…"
  kill_tree "${TAIL_PID}"
  kill_tree "${FRONTEND_PID}"
  kill_tree "${BACKEND_PID}"
  # 兜底按端口回收：kill_tree 可能漏掉脱离进程组的后代进程。
  [[ "${STARTED_FRONTEND}" == "1" ]] && kill_by_port "${FRONTEND_PORT}"
  [[ "${STARTED_BACKEND}" == "1" ]] && kill_by_port "${BACKEND_PORT}"
  ok "已停止"
}

# check_deps 检查所选模式需要的外部命令，缺失时直接退出（避免后台任务静默失败）。
check_deps() {
  local missing=""
  if [[ "${MODE}" != "frontend" ]] && ! command -v go >/dev/null 2>&1; then
    missing="${missing} go"
  fi
  if [[ "${MODE}" != "backend" ]] && ! command -v npm >/dev/null 2>&1; then
    missing="${missing} npm"
  fi
  if [[ -n "${missing}" ]]; then
    fail "缺少命令:${missing}，请先安装后重试"
    exit 1
  fi
  if ! command -v lsof >/dev/null 2>&1; then
    warn "未找到 lsof，跳过端口占用检查与回收"
  fi
}

# ---------------- 启动后端函数 ----------------
# 后端以 `go run .` 启动（dev 模式，改代码后重启脚本即可生效）。
# 它会自行读取 .env（main.go 中 godotenv.Load）并按配置写 logs/app.log。
start_backend() {
  kill_by_port "${BACKEND_PORT}"
  mkdir -p "${LOG_DIR}"

  info "启动后端: go run . → http://localhost:${BACKEND_PORT}/api"
  go run . &
  BACKEND_PID=$!
  STARTED_BACKEND=1
}

# ---------------- 启动前端函数 ----------------
start_frontend() {
  kill_by_port "${FRONTEND_PORT}"

  if [[ ! -d "${FRONTEND_DIR}/node_modules" ]]; then
    info "未检测到 node_modules，先执行 npm install"
    if ! (cd "${FRONTEND_DIR}" && npm install); then
      fail "npm install 失败，已跳过前端启动"
      return 1
    fi
  fi

  info "启动前端: npm run dev → http://localhost:${FRONTEND_PORT}"
  (cd "${FRONTEND_DIR}" && npm run dev) &
  FRONTEND_PID=$!
  STARTED_FRONTEND=1
}

# tail_log 实时监听后端日志文件。
# 用 -F 而不是 -f：GoFrame 按 rotateSize 切割 app.log，inode 会变，-f 会在切割后失联。
tail_log() {
  mkdir -p "${LOG_DIR}"
  touch "${LOG_FILE}"
  info "监听日志: ${LOG_FILE}"
  tail -n 0 -F "${LOG_FILE}" &
  TAIL_PID=$!
}

# ---------------- 主逻辑 ----------------
main() {
  case "${MODE}" in
    all | backend | frontend) ;;
    help | -h | --help)
      usage
      exit 0
      ;;
    *)
      fail "未知参数: ${MODE}"
      usage >&2
      exit 2
      ;;
  esac

  check_deps
  trap cleanup INT TERM EXIT

  if [[ "${MODE}" != "frontend" ]]; then
    start_backend
    tail_log
  fi
  if [[ "${MODE}" != "backend" ]]; then
    start_frontend || warn "前端未能启动，可稍后单独执行 ./dev.sh frontend"
  fi

  ok "开发环境已启动，Ctrl+C 停止"
  # 等待所有后台作业；任一服务退出后由 trap cleanup 统一收尾。
  wait
}

main
