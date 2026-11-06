# Windows 客户端（控制台版）

> **v0.5 起默认客户端是图形版**（`gui/`，视频内嵌单窗口），构建与使用见 [06-Windows客户端GUI设计.md](06-Windows客户端GUI设计.md) 和根目录 [README](../README.md)。
> 本文对应 `windows/` 目录下的命令行客户端：它**保留下来专门用于排障**——日志直白，同步逻辑与图形版同源，定位"连不上 / 不同步"比图形版快。
> 两者协议完全一致，可以混进同一个房间（但同一台 Windows 只能开一个，mpv 命名管道会冲突）。

播放内核用 **mpv**（ffmpeg 系，格式覆盖最全），本程序通过 mpv 的 JSON IPC 控制它：不自己解码、不碰视频数据。

## 目录结构

```
windows/
├── main.go               客户端源码（WebSocket ⇄ mpv IPC 桥接）
├── go.mod
├── build.bat             编译（有 Go 用 Go，有 Docker 用 Docker）
├── fetch-mpv.ps1         下载 mpv 到 .\mpv\（约 33MB）
├── 启动观影.bat           双击运行
├── 打包.bat               打成 dist\一起看-windows-v0.4.zip 分发
├── server.txt            你的服务器地址（不入库，需自己创建）
├── mpv/                  播放内核（不入库）
└── syncwatch-win.exe     编译产物（不入库）
```

## 首次准备（两步）

**1. 拿 mpv**

```powershell
cd windows
powershell -ExecutionPolicy Bypass -File fetch-mpv.ps1
```

脚本从 GitHub Releases 下载便携版 mpv 并解压到 `windows\mpv\`，不写注册表、不装系统。

**2. 写服务器地址**

```bat
echo 你的服务器IP:8995> server.txt
```

也可以不建这个文件，改用 `启动观影.bat` 前设置环境变量 `SYNCWATCH_SERVER`，或运行时加 `-server` 参数。

## 使用

双击 `启动观影.bat`，按提示依次输入：

1. **昵称**：房间里显示的名字
2. **视频文件路径**：直接把文件拖进命令行窗口即可自动填入
3. **房间码**：想创建房间就**直接回车**（服务器生成 6 位房间码）；想加入朋友的房间就输入对方给的 6 位码（小写会自动转大写）
4. **房间密码**：房间没设密码就直接回车

进入后控制台会醒目打印房间码，并**自动复制到剪贴板**，直接 Ctrl+V 发给朋友即可。

> **两个人不在同一个房间？** 99% 是各自都直接回车、各建了一个房间。必须有一个人把房间码发给另一个，后者在启动时填进去。
>
> 如果看到 **"你输入的房间码 XXX 不存在或已解散，所以新建了一个房间"**，说明你没进成朋友的房间。核对房间码（注意别多复制了空格）后重新启动。

> 所有人要用**同一个视频文件**（版本尽量一致）。时长不同会在成员列表里直接看出来。

命令行参数（跳过交互）：

```bat
syncwatch-win.exe -server 1.2.3.4:8995 -name 小明 -room 7M8QW6 -file "D:\movie.mkv"
```

| 参数 | 说明 |
|------|------|
| `-server` | 服务器 `host:port`，默认读 环境变量 `SYNCWATCH_SERVER` → `server.txt` → 编译期内置地址 |
| `-name` | 昵称，留空则交互输入 |
| `-room` | 房间码，留空则交互询问（再回车才创建新房间） |
| `-password` | 房间密码（创建者设定，加入者需一致） |
| `-file` | 视频文件路径，留空则交互输入 |
| `-mpv` | 指定 mpv.exe 路径（自动查找：PATH → `mpv.exe` → `mpv\mpv.exe`） |

## 观影中的行为

| 操作 | 效果 |
|------|------|
| 空格暂停/播放 | 全房间同步，其他人看到"某某 暂停了"的归因提示 |
| 拖动进度条 | 全房间跳转（服务端 120ms 防抖，只广播终点） |
| 网络波动 | 每 5s 心跳对时，偏差 > 1s 服务器强制纠偏 |
| 有人掉线 | 成员列表实时更新，对方重连后自动追到当前进度 |
| 视频缓冲 | 上报 `buffering`，别人能看到你在卡 |

关闭 mpv 窗口或 `Ctrl+C` 退出房间。

## 构建

```bat
cd windows
build.bat
```

`build.bat` 会**自动选择编译方式**：

1. 本机有 Go（1.21+）→ `go build` 直接编译；
2. 没有 Go 但有 Docker → 挂载目录用 `golang:1.22-alpine` 交叉编译（国内自动加 `GOPROXY=https://goproxy.cn`）；
3. 都没有 → 提示安装其中一个。

手动编译等价于：

```bash
go mod tidy
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o syncwatch-win.exe .
```

## 打包分发

```bat
cd windows
打包.bat
```

产出 `dist\一起看-windows-v0.4.zip`（exe + mpv + 启动脚本 + 说明），对方解压双击即可，无需装任何东西。

## 常见问题

- **防火墙弹窗**：首次运行 Windows 可能询问网络权限，选择"允许"
- **提示找不到文件**：路径含空格没关系，程序会去掉引号；确认盘符存在
- **提示无法启动 mpv**：`mpv\mpv.exe` 不存在，跑 `fetch-mpv.ps1` 或 `-mpv` 指定路径
- **一台电脑开两个客户端**：不支持，mpv 命名管道名固定，第二个实例会接管失败
- **视频是 HEVC/10bit**：mpv 自带解码，正常播放；若显卡不支持硬解，会回落到软解（CPU 占用高）
