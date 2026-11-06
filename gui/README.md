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
| 分发包 | `dist\一起看-windows-v0.5.zip` | `dist\一起看-windows-v0.4.zip` |

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
打包.bat                                                             :: 产出 dist\一起看-windows-v0.5.zip
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
build\bin\gui.exe
```

日志：`%TEMP%\syncwatch-gui.log`（界面右上角「文档」图标可直接看）。
前缀含义：`[ui]` 前端打点，`[mpv]` 播放器输出，`recv` / `send` 协议收发。

## 改代码前必读的六条约束

这些全部是实测踩出来的，改回去会立刻出可见 bug（详见 [../docs/06-Windows客户端GUI设计.md](../docs/06-Windows客户端GUI设计.md) 第 12 章）：

1. **Win32 窗口操作必须走 `runOnUI()`** —— 窗口归属创建它的线程，跨线程 `ShowWindow` 会无限期阻塞。
2. **`.app` / `.stage` 不能设 `background`** —— 只要有底色就会盖住 mpv 画面，表现为视频区一片纯色。
3. **mpv 必须 `--pause=yes` 启动** —— 否则初始的 `pause=false` 事件会被当成用户点了播放，把房间状态改成 playing。
4. **`observe_property` 的 id 用整数** —— 字符串 id 的事件里没有 `name` 字段，解析不到。
5. **`LoadVideo` 要能早于播放页** —— 选片在首页发生，此时先建 1×1 隐藏宿主把 mpv 拉起来，进播放页再由 `SetVideoRect` 摆正。
6. **底部不要放独立状态栏** —— 那 30px 会被 mpv 的 D3D 表面吃掉，信息并入顶部浮层。

批处理编码在两个目录里不同：`gui/*.bat` 是 **GBK + CRLF**，`windows/*.bat` 是 **UTF-8（首行 `chcp 65001`）+ CRLF**。
改脚本时用原编码写回，别混用——编码错了 cmd 会把中文里的 `&` 当命令分隔符，直接报"此时不应有 do"。
