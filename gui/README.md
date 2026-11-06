# gui/ · Windows 图形客户端

v0.5 起的默认客户端：**单窗口观影**——视频、房间信息、成员列表、控制条都在一个窗口里，
不再像控制台版那样另开一个 mpv 窗口。

```
Wails v2（Go 后端 + React 前端，WebView2 承载界面）
        └── mpv 用 --wid 嵌进一个 Win32 宿主子窗口，压在 WebView 下面
        └── syncclient.go 走 WebSocket，协议与 server/、android/ 完全一致
```

## 与 windows/ 的关系

| | `gui/`（本目录） | `windows/`（控制台版） |
|---|---|---|
| 定位 | 日常观影 | 排障：日志直白 |
| 协议实现 | `syncclient.go` | `main.go` |
| 播放内核 | 复用 `windows/mpv/` | `windows/mpv/` |
| 分发包 | `dist\一起看-windows-v0.5.1.zip` | `dist\一起看-windows-v0.4.zip` |

两份 Go 协议代码**同源但各自独立**，改协议要一起改（连同 `server/main.go`、`Protocol.kt`、`docs/02-同步协议.md`，共五处）。
同一台 Windows 只能跑一个客户端实例，mpv 的命名管道会抢。

## 文件分工

| 文件 | 管什么 |
|---|---|
| `main.go` | Wails 启动参数：窗口尺寸、`WebviewIsTransparent: true` |
| `app.go` | 暴露给前端的方法（建房/加入/选片/播放控制/配置）与事件推送 |
| `syncclient.go` | WebSocket 协议、心跳、回声抑制 |
| `player.go` | mpv 启停、JSON IPC、属性订阅 |
| `win32.go` | mpv 宿主子窗口 + 专用 UI 线程（最容易踩坑的一块） |
| `frontend/src/App.tsx` | 页面状态机：首页 → 创建/加入 → 播放 |
| `frontend/src/App.css` | 深色影院风样式 |

## 构建

前置：Go 1.22+、Node 18+、Wails CLI（`go install github.com/wailsapp/wails/v2/cmd/wails@latest`）。

```bat
cd gui
powershell -ExecutionPolicy Bypass -File ..\windows\fetch-mpv.ps1   :: 首次：准备播放内核
echo 你的IP:8995 > server.txt                                       :: 预置默认地址（不入库）
build.bat                                                           :: wails build -platform windows/amd64
打包.bat                                                             :: 产出 dist\一起看-windows-v0.5.1.zip
```

`打包.bat` 从 `..\windows\mpv` 复制内核（不重复下载），并在 `server.txt` 存在时一并打进 zip——
对方解压即直连，不需要手填地址。zip 里不带 `server.txt` 时，首次启动会要求用户在设置里填。

只改前端时不必走 Wails：

```bat
cd frontend && npm install && npm run build    :: 验证 TS 与界面打包
```

## 调试

```bat
set SYNCWATCH_AUTO_VIDEO=C:\path\test300.mp4
set SYNCWATCH_AUTO_ROOM=new          & rem new = 建房；填房间码 = 加入
set SYNCWATCH_AUTO_NAME=桌面端
set SYNCWATCH_SERVER=127.0.0.1:8995  & rem 可选，默认 127.0.0.1:18998
set SYNCWATCH_AUTO_SEEK=150          & rem 可选，进房 1.5 秒后跳到 150s
set SYNCWATCH_AUTO_TOGGLE=1          & rem 可选，进房 1.8 秒后播放/暂停一次
build\bin\gui.exe
```

**有 `SYNCWATCH_AUTO_VIDEO` 时程序进入调试模式：完全不读也不写 `%APPDATA%\SyncWatch\config.json`。**
这一条是 v0.5.1 补的——以前联调会把 `127.0.0.1` 写进用户配置，顶掉分发包自带的 `server.txt`，
用户打开就只连本机，表现为"两台电脑互相看不到"（详见 [../docs/06-Windows客户端GUI设计.md](../docs/06-Windows客户端GUI设计.md) 第 13 章）。

日志：`%TEMP%\syncwatch-gui.log`（界面右上角「文档」图标可直接看）。
前缀含义：`[ui]` 前端打点，`[mpv]` 播放器输出，`recv` / `send` 协议收发。

## 快捷键与鼠标

| 操作 | 效果 |
|---|---|
| 单击画面 / 双击画面 | 暂停·播放 / 全屏 |
| 空格 | 暂停 / 播放 |
| ← → | 快退 / 快进 5 秒（Shift 为 30 秒），连按合并成一次上报 |
| ↑ ↓ | 音量 ±5（只影响本地） |
| F / M / Esc | 全屏 / 静音 / 收起成员面板或退出全屏 |

焦点在输入框、或打开了日志/设置抽屉时，快捷键不接管，避免和打字冲突。

## 改代码前必读的八条约束

这些全部是实测踩出来的，改回去会立刻出可见 bug（详见 [../docs/06-Windows客户端GUI设计.md](../docs/06-Windows客户端GUI设计.md) 第 12、13 章）：

1. **Win32 窗口操作必须走 `runOnUI()`** —— 窗口归属创建它的线程，跨线程 `ShowWindow` 会无限期阻塞。
2. **`.app` / `.stage` 不能设 `background`** —— 只要有底色就会盖住 mpv 画面，表现为视频区一片纯色。
3. **mpv 必须 `--pause=yes` 启动** —— 否则初始的 `pause=false` 事件会被当成用户点了播放，把房间状态改成 playing。
4. **`observe_property` 的 id 用整数** —— 字符串 id 的事件里没有 `name` 字段，解析不到。
5. **`LoadVideo` 要能早于播放页** —— 选片在首页发生，此时先建 1×1 隐藏宿主把 mpv 拉起来，进播放页再由 `SetVideoRect` 摆正。
6. **底部不要放独立状态栏** —— 那 30px 会被 mpv 的 D3D 表面吃掉，信息并入顶部浮层。
7. **WS 写操作必须串行** —— gorilla/websocket 检测到并发写会直接 `panic`，心跳线程和用户操作撞上就是整个进程崩。`syncclient.send()` 里的 `writeMu` 不能去掉。
8. **服务器地址的来源优先级是 config > server.txt** —— 所以任何"自动/调试"路径都不许往 config 里写地址，否则分发包自带的地址会被静默顶掉，用户看到的是"都连上了却互相看不到"。

批处理编码在两个目录里不同：`gui/*.bat` 是 **GBK + CRLF**，`windows/*.bat` 是 **UTF-8（首行 `chcp 65001`）+ CRLF**。
改脚本时用原编码写回，别混用——编码错了 cmd 会把中文里的 `&` 当命令分隔符，直接报"此时不应有 do"。
