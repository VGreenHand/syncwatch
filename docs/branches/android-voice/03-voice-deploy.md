# 语音服务部署（fork）

> 上游 `docs/03-服务端部署.md` 未修改。本文件只描述 fork 的 LiveKit 伴生服务；不配置 LiveKit 时，`syncwatch` 行为与上游一致。

## 拓扑

- `syncwatch` 与 LiveKit 部署在同一台阿里云服务器，但使用独立容器与独立 Compose 文件。
- 沿用现有公网 IP、Docker 与安全组；不把 LiveKit 塞进 `syncwatch` 核心进程。
- LiveKit 故障或停服不影响播放同步。

## 端口

| 协议 | 端口 | 用途 | 默认 |
|---|---|---|---|
| TCP | 7880 | LiveKit 信令 | 开 |
| TCP | 7881 | WebRTC over TCP 回退 | 开 |
| UDP | 7882 | WebRTC 媒体单端口 mux | 开 |
| UDP | 3478 | TURN 兜底 | 关；直连失败再开 |

安全组只放行上述端口，尽量限制来源 IP；不开放 `50000-60000` 整段 UDP。

## 配置要点

- 新增 `voice/docker-compose.yml` 与 `voice/livekit.yaml`；镜像固定为 `livekit/livekit-server:v1.13.11`，不使用 `latest`。
- LiveKit 的 `rtc.tcp_port` 用 7881，`rtc.udp_port` 用 7882，`rtc.use_external_ip: true`。
- TURN 先关闭；需要时再启用 3478/udp。
- Docker 网络优先使用 host 模式，避免 WebRTC 端口再经过一层 NAT；若用 bridge，必须显式映射 7880/tcp、7881/tcp、7882/udp。

## 环境变量

- `LIVEKIT_URL`：客户端连接地址，例如 `ws://<公网IP>:7880`。
- `LIVEKIT_API_KEY` / `LIVEKIT_API_SECRET`：只放服务器 `.env` 或部署环境，不入库、不写日志。
- `syncwatch` 服务端读取同一组变量，T2 时用于签发 token。

## 验证

1. `docker compose up -d` 后 `docker compose logs` 无端口/证书错误。
2. 用 LiveKit 官方示例或 CLI 进房，能听到回声测试音。
3. 从公网检查 7880/tcp、7881/tcp、7882/udp 可达，3478/udp 未开放。
4. 停止 LiveKit，确认 `syncwatch` 的加入房间与播放同步仍正常。

## 未定

- Compose 使用 host 还是 bridge 网络，T1 实测后定。
- 对外服务前的 TLS 方案；当前仅两人私有使用。

## 升级策略

- 固定 `livekit/livekit-server:v1.13.11`；升级时手动改 tag，并在 T1 回声测试 + T4 双机回归通过后再保留新版本。
