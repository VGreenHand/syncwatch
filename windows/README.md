# Windows 客户端（Go + mpv）

## 三步跑起来

```bat
build.bat                  :: 编译（有 Go 用 Go，没 Go 用 Docker）
powershell -File fetch-mpv.ps1   :: 下载 mpv 到 .\mpv\
echo 1.2.3.4:8995> server.txt    :: 填你的服务器地址
启动观影.bat
```

## 代码结构（`main.go`，约 550 行）

| 区块 | 作用 |
|------|------|
| `MpvClient` / `dialMpv` | 通过命名管道 `\\.\pipe\syncwatch-mpv` 与 mpv 通信（JSON IPC） |
| `handlePropertyChange` | 监听 mpv 的 `pause` / `paused-for-cache`，转成上行指令 |
| `applySync` | 收到服务器 `sync` 后执行 mpv 命令（暂停、seek、倍速） |
| `wsReadLoop` / `sendMsg` | WebSocket 收发，断线后 `reconnect()` 追进度 |
| `defaultServerAddr` | 服务器地址：环境变量 → `server.txt` → 内置默认值 |
| `suppressEvents` | 抑制"程序自己触发的"播放器事件，避免回音式广播 |

## 调参提示

- 心跳间隔：`HEARTBEAT_EVERY`（5s）
- 抑制窗口：`suppressEvents(ms)` 调用处，默认几百毫秒

## 打包

`打包.bat` → `dist\一起看-windows-v0.4.zip`（含 mpv，免安装）
