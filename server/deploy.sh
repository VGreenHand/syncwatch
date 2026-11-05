#!/usr/bin/env bash
# 一键部署：把 server/ 目录推到远端主机并重建容器
#
# 用法：
#   cp deploy.env.example deploy.env   # 第一次：填 host/port/user/目录
#   ./deploy.sh
#
# 前置：本机已能免密 ssh 登录远端（推荐用公钥，避免密码进历史记录）
set -euo pipefail
cd "$(dirname "$0")"

if [ ! -f deploy.env ]; then
  echo "缺少 deploy.env，请先执行：cp deploy.env.example deploy.env 并填写主机信息"
  exit 1
fi
# shellcheck disable=SC1091
source deploy.env

: "${SERVER_HOST:?deploy.env 缺少 SERVER_HOST}"
: "${SERVER_USER:?deploy.env 缺少 SERVER_USER}"
SERVER_PORT="${SERVER_PORT:-22}"
REMOTE_DIR="${REMOTE_DIR:-~/syncwatch/server}"
SSH_KEY="${SSH_KEY:-}"

[ -f .env ] || cp .env.example .env

SSH_OPTS=(-p "$SERVER_PORT" -o BatchMode=yes -o StrictHostKeyChecking=accept-new)
[ -n "$SSH_KEY" ] && [ -f "$SSH_KEY" ] && SSH_OPTS+=(-i "$SSH_KEY")

TARGET="$SERVER_USER@$SERVER_HOST"
echo "==> 目标：$TARGET:$SERVER_PORT  ($REMOTE_DIR)"

echo "==> 检查远端 Docker"
if ! ssh "${SSH_OPTS[@]}" "$TARGET" 'docker --version && docker compose version' >/dev/null 2>&1; then
  echo "❌ 远端 SSH 或 docker/compose 不可用，请先按 docs/03-服务端部署.md 准备环境"
  exit 1
fi

echo "==> 上传源码"
tar czf - main.go go.mod go.sum Dockerfile docker-compose.yml .env \
  | ssh "${SSH_OPTS[@]}" "$TARGET" "mkdir -p '$REMOTE_DIR' && tar xzf - -C '$REMOTE_DIR'"

echo "==> 重建并启动容器"
ssh "${SSH_OPTS[@]}" "$TARGET" "cd '$REMOTE_DIR' && (docker rm -f syncwatch >/dev/null 2>&1 || true) && docker compose up -d --build"

echo "==> 健康检查"
PORT_LOCAL="${PORT:-8995}"
if ssh "${SSH_OPTS[@]}" "$TARGET" "sleep 2; curl -s --max-time 5 http://127.0.0.1:8995/health"; then
  echo
  echo "✅ 部署完成：ws://$SERVER_HOST:$PORT_LOCAL/ws"
else
  echo "⚠️ 健康检查失败，查看日志：ssh -p $SERVER_PORT $TARGET \"cd '$REMOTE_DIR' && docker compose logs --tail 50\""
  exit 1
fi
