# syncwatch-server

同步观影的信令服务器：管房间、存权威播放状态、转发 play/pause/seek，**不接触任何视频数据**。

- 单文件实现：`main.go`（约 460 行，含全部逻辑）
- 协议：见 [../docs/02-同步协议.md](../docs/02-同步协议.md)
- 部署：见 [../docs/03-服务端部署.md](../docs/03-服务端部署.md)

## 本地跑起来

```bash
go mod download
go run .            # 默认监听 :8995
curl localhost:8995/health
```

环境变量 `PORT` 可改端口。

## 常用调参（main.go 顶部 const）

```go
MAX_MEMBERS   = 5                  // 房间人数上限
SEEK_DEBOUNCE = 120 * time.Millisecond  // seek 合并窗口
DRIFT_HARD    = 1.0                // 秒，超过则强制纠偏
HEARTBEAT_LATE = 30 * time.Second  // 心跳超时
RATE_LIMIT    = 40                 // 每连接每秒最大消息数
ROOM_IDLE_TTL = 2 * time.Hour      // 空房保留时长
```

## 部署

```bash
cp .env.example .env      # 国内服务器记得换镜像源
docker compose up -d --build
```

或改完代码一键推上线（需先填 `deploy.env`）：`./deploy.sh`
