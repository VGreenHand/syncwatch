#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"
MODE="${1:-deploy}"

say() { printf '%s\n' "$*"; }
fail() { say "[FAIL] $*"; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || fail "缺少 $1"; }
mask_ip() { sed -E 's/([0-9]{1,3}\.){3}[0-9]{1,3}/<ip>/g'; }

need docker
docker info >/dev/null 2>&1 || fail "当前账号无 Docker 权限；确认 dsh 已在 docker 组，并从宿主重新登录后再执行"
if docker compose version >/dev/null 2>&1; then
  DC=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  DC=(docker-compose)
else
  fail "缺少 docker compose"
fi

env_ip() {
  [ -s .env ] || return 0
  sed -n 's|^LIVEKIT_URL=ws://\([0-9.]*\):7880$|\1|p' .env
}

env_ready() {
  [ -s .env ] && grep -q '^LIVEKIT_URL=ws://' .env && grep -q '^LIVEKIT_API_KEY=.' .env && grep -q '^LIVEKIT_API_SECRET=.' .env
}

detect_ip() {
  local ip
  ip="$(env_ip)"
  if [ -z "$ip" ]; then
    ip="${LIVEKIT_PUBLIC_IP:-}"
  fi
  if [ -z "$ip" ]; then
    need curl
    ip="$(curl -fsS --max-time 3 http://100.100.100.200/latest/meta-data/eipv4 || true)"
    [ -n "$ip" ] || ip="$(curl -fsS --max-time 3 http://100.100.100.200/latest/meta-data/public-ipv4 || true)"
    [ -n "$ip" ] || ip="$(curl -4 -fsS --max-time 10 https://api.ipify.org || curl -4 -fsS --max-time 10 https://ifconfig.me || true)"
  fi
  case "$ip" in
    ''|*[!0-9.]*) fail "无法获取公网 IPv4；请用 LIVEKIT_PUBLIC_IP=<公网IP> 重跑" ;;
  esac
  printf '%s' "$ip"
}

write_files() {
  local ip="$1"
  umask 077
  cat > livekit.yaml <<YAML
port: 7880

rtc:
  tcp_port: 7881
  udp_port: 7882
  node_ip: $ip
  use_external_ip: false

logging:
  level: info
YAML
  cat > docker-compose.yml <<'YAML'
services:
  livekit:
    image: livekit/livekit-server:v1.13.11
    container_name: syncwatch-livekit
    restart: unless-stopped
    network_mode: host
    command: --config /etc/livekit.yaml
    environment:
      LIVEKIT_KEYS: "${LIVEKIT_API_KEY:?LIVEKIT_API_KEY missing in .env}: ${LIVEKIT_API_SECRET:?LIVEKIT_API_SECRET missing in .env}"
    volumes:
      - ./livekit.yaml:/etc/livekit.yaml:ro
YAML
}

write_env() {
  local ip="$1" key secret
  if command -v openssl >/dev/null 2>&1; then
    key="$(openssl rand -hex 16)"
    secret="$(openssl rand -hex 32)"
  else
    key="$(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n')"
    secret="$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')"
  fi
  printf 'LIVEKIT_URL=ws://%s:7880\nLIVEKIT_API_KEY=%s\nLIVEKIT_API_SECRET=%s\n' "$ip" "$key" "$secret" > .env
  chmod 600 .env
  say "已生成 .env（IP 与密钥未打印）"
}

check() {
  local rc=0 code state image digest version errs

  say "== 容器状态 =="
  "${DC[@]}" ps || true
  state="$(docker inspect -f '{{.State.Running}}' syncwatch-livekit 2>/dev/null || true)"
  if [ "$state" = "true" ]; then
    say "[PASS] syncwatch-livekit running"
  else
    say "[FAIL] syncwatch-livekit not running"
    rc=1
  fi

  say "== 版本 =="
  image="$(docker inspect -f '{{.Config.Image}}' syncwatch-livekit 2>/dev/null || true)"
  digest="$(docker inspect -f '{{.Image}}' syncwatch-livekit 2>/dev/null || true)"
  version="$(docker exec syncwatch-livekit /livekit-server --version 2>/dev/null | head -1 || true)"
  [ -n "$image" ] && say "[INFO] image=$image"
  [ -n "$digest" ] && say "[INFO] digest=$digest"
  [ -n "$version" ] && say "[INFO] server=$version"

  say "== 端口 =="
  if command -v ss >/dev/null 2>&1; then
    if ss -lnt 2>/dev/null | grep -qE ':(7880|7881)[[:space:]]'; then
      say "[PASS] 7880/7881 tcp 有监听"
    else
      say "[FAIL] 未看到 7880/7881 tcp 监听"
      rc=1
    fi
    if ss -lun 2>/dev/null | grep -qE ':7882[[:space:]]'; then
      say "[PASS] 7882 udp 有监听"
    else
      say "[FAIL] 未看到 7882 udp 监听"
      rc=1
    fi
  else
    say "[WARN] 未安装 ss，跳过端口检查"
  fi

  say "== 本地 HTTP =="
  if command -v curl >/dev/null 2>&1; then
    code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 http://127.0.0.1:7880/ || true)"
    if [ -n "$code" ] && [ "$code" != "000" ]; then
      say "[PASS] 127.0.0.1:7880 响应 $code"
    else
      say "[FAIL] 127.0.0.1:7880 无响应"
      rc=1
    fi
  else
    say "[WARN] 未安装 curl，跳过 HTTP 检查"
  fi

  say "== .env 检查 =="
  if env_ready; then
    say "[PASS] LIVEKIT_URL / LIVEKIT_API_KEY / LIVEKIT_API_SECRET 均已设置"
  else
    say "[FAIL] .env 缺失或不完整"
    rc=1
  fi

  say "== 最近日志 =="
  "${DC[@]}" logs --tail=40 2>&1 | mask_ip | tail -40 || true

  say "== 错误扫描 =="
  errs="$("${DC[@]}" logs --tail=200 2>&1 | mask_ip | grep -iE 'error|fatal|panic|failed|denied' | tail -20 || true)"
  if [ -n "$errs" ]; then
    printf '%s\n' "$errs"
  else
    say "未发现明显错误"
  fi

  return "$rc"
}

if [ "$MODE" = "--check" ] || [ "$MODE" = "check" ]; then
  check
  exit $?
fi

ip="$(detect_ip)"
say "公网 IPv4 已识别（写入 node_ip，不在输出中打印）"
write_files "$ip"
if env_ready; then
  sed -i "s|^LIVEKIT_URL=.*|LIVEKIT_URL=ws://$ip:7880|" .env
  chmod 600 .env
  say "沿用现有 .env 的密钥，已按当前公网 IP 刷新 LIVEKIT_URL"
else
  write_env "$ip"
fi
"${DC[@]}" up -d
say "等待容器启动..."
sleep 3

rc=0
check || rc=$?
say "部署目录: $PWD"
say "下一步：确认阿里云安全组放行 7880/tcp、7881/tcp、7882/udp；3478/udp 保持关闭。"
say "公网可达性与音频链路需在另一网络用客户端验证；本脚本只验证本机监听与容器状态。"
exit "$rc"
