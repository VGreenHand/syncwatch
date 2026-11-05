# 一起看 · SyncWatch

> 2~5 人异地同步观看**各自本地已有**的视频文件：播放、暂停、拖动进度全同步，谁操作的会实时提示。
> 视频数据完全不经过网络，服务器只转发几十字节的同步指令。

<p>
<img alt="server" src="https://img.shields.io/badge/server-Go%201.21-00ADD8">
<img alt="windows" src="https://img.shields.io/badge/Windows-mpv%20IPC-0078D4">
<img alt="android" src="https://img.shields.io/badge/Android-Kotlin%20%2B%20Media3-3DDC84">
<img alt="license" src="https://img.shields.io/badge/license-MIT-green">
</p>

---

## 为什么不是"一起看视频网站"

常见的"同步观影"方案要把视频推给每个人，流量大、成本高，家庭宽带上行还容易被运营商判定为 PCDN。
本项目反过来：**每人自己磁盘上已经有同一个片子**，只需要把"按了播放 / 暂停 / 拖到 12:34"这几条指令对齐。

| 特性 | 说明 |
|------|------|
| 零视频流量 | 网络上只有 JSON 信令，单房间 < 2 KB/s，月流量几百 MB |
| 规避 PCDN 风险 | 架构上不存在"家庭宽带上行分发视频"的形态，家宽侧无视频流量特征 |
| 全格式播放 | Windows 端用 mpv（ffmpeg 系），RMVB / MKV / HEVC / 外挂字幕都能放 |
| 跨端互通 | Windows 与 Android 使用同一套协议，可混用同一房间 |
| 服务端权威 | 房间状态由服务器持有，暂停竞态、进度漂移由服务器裁决 |
| 成员状态 | 谁在线、谁在缓冲、谁暂停了，界面实时显示并归因 |
| 5 人上限 | 一个房间最多 5 人，防滥用 |

## 架构

```
        ┌──────────────────────────────────────────┐
        │         ECS / 任意云主机（1 核 1G 够用）      │
        │  ┌────────────────────────────────────┐  │
        │  │  syncwatch-server (Go, 单二进制)      │  │
        │  │  房间管理 · 权威状态机 · 防抖 · 漂移纠偏 │  │
        │  └────────────────────────────────────┘  │
        └───────▲───────────────────────▲──────────┘
                │ ws:// 信令 < 2KB/s      │
        ┌───────┴────────┐      ┌────────┴─────────┐
        │ Windows 客户端   │      │ Android 客户端     │
        │ mpv + JSON IPC  │      │ Media3 ExoPlayer │
        └───────┬────────┘      └────────┬─────────┘
                │                        │
          本地视频文件               本地视频文件
        （不传输、不上传、不外发）
```

## 快速开始

### 1. 起一个服务端（3 分钟）

```bash
git clone <你的仓库地址> && cd syncwatch/server
cp .env.example .env          # 按需改端口；国内服务器记得用镜像源变量
docker compose up -d --build
curl http://127.0.0.1:8995/health   # {"status":"ok","rooms":0}
```

云服务器记得在**安全组放行对应 TCP 端口**，详细步骤（含备案、加固、升级、迁移）见 [docs/03-服务端部署.md](docs/03-服务端部署.md)。

### 2. Windows 端

```bat
cd windows
fetch-mpv.ps1            :: 第一次：下载 mpv 到 windows\mpv\
build.bat                :: 编译 syncwatch-win.exe（需要 Go 1.21+）
echo 你的服务器IP:8995> server.txt
启动观影.bat              :: 双击，按提示输入昵称、拖入视频、填房间码
```

不装 Go 也可以直接从 Releases 下载打包好的 `一起看-windows-v0.2.zip`，解压双击即可。

### 3. Android 端

```bat
cd android
build-apk.bat            :: 产出 dist\一起看-android-v0.2.apk
```

APK 传到手机侧载安装（Android 7.0+），或在 Releases 直接下载。详见 [docs/05-Android客户端.md](docs/05-Android客户端.md)。

> **同一部片**：所有人必须各自持有同一版本的视频文件（时长一致）。进房后成员列表会显示各自时长，不一致会立刻看得出来。

## 目录结构

```
syncwatch/
├── README.md                  ← 你正在看的文件
├── docs/                      ← 全部文档
│   ├── 01-方案设计.md          需求、架构选型、PCDN 风险分析、里程碑
│   ├── 02-同步协议.md          消息格式、状态机、漂移算法（改协议先看这个）
│   ├── 03-服务端部署.md        部署、加固、升级、迁移、排障
│   ├── 04-Windows客户端.md     使用与构建
│   └── 05-Android客户端.md     使用与构建
├── server/                    ← Go WebSocket 信令服务器（全部逻辑在 main.go）
│   ├── main.go
│   ├── Dockerfile
│   ├── docker-compose.yml
│   └── deploy.sh              一键部署到远程主机
├── windows/                   ← Windows 客户端（Go + mpv IPC）
│   ├── main.go
│   ├── build.bat / 启动观影.bat / 打包.bat
│   └── fetch-mpv.ps1
├── android/                   ← Android 客户端（Kotlin + Media3）
│   └── app/src/main/java/com/syncwatch/app/
├── tools/                     ← 联调工具（不是产品代码）
│   ├── sim/                   模拟多个成员进房，做协议回归测试
│   └── ctl/                   对指定房间发送单条 play/pause/seek 指令
├── dist/                      ← 构建产物（不入库）
└── private/                   ← 本地迭代备忘（不入库：含服务器地址、待办、排障手册）
```

## 代码地图

想改东西时按这张表找位置：

| 我想改… | 去哪 |
|---------|------|
| 人数上限 / 纠偏阈值 / 防抖窗口 / 心跳超时 | `server/main.go` 顶部 `const` 块 |
| 房间码规则、密码校验、重名规则 | `server/main.go` `handleJoin()` |
| 房间内状态机（播放/暂停/seek/心跳） | `server/main.go` `readLoop()` 的 switch |
| 协议字段 | `server/main.go` `Msg` 结构体 + 两端 `Msg`/`Protocol`（三处要同步改） |
| Windows 播放器行为（mpv 命令） | `windows/main.go` `handlePropertyChange()` / `applySync()` |
| Android 播放器行为 | `android/.../SyncEngine.kt` + `Protocol.kt` |
| 界面布局 | `android/app/src/main/res/layout/activity_main.xml` |
| 服务器地址默认值 | `windows/server.txt`、`android/local.properties`（都不入库） |

## 配置项

| 位置 | 作用 | 是否入库 |
|------|------|----------|
| `local-env.bat` | 本机 Go / JDK / Android SDK / Gradle 路径，构建脚本自动读取 | 否（有 `local-env.bat.example`） |
| `server/.env` | 端口、镜像源、GOPROXY | 否（有 `.env.example`） |
| `server/deploy.env` | 部署目标的 host/port/user | 否（有示例） |
| `windows/server.txt` | Windows 客户端默认服务器地址（一行 `host:port`） | 否 |
| `windows/mpv/` | mpv 播放器（100MB+） | 否，`fetch-mpv.ps1` 下载 |
| `android/local.properties` | `sdk.dir` 与 `syncwatch.server` | 否（有 `.example`） |
| 环境变量 `SYNCWATCH_SERVER` | 覆盖客户端服务器地址（调试用） | — |

## 技术栈与版本

| 组件 | 版本 | 备注 |
|------|------|------|
| Go | 1.21+ | 服务端与 Windows 客户端共用 |
| gorilla/websocket | v1.5.3 | WebSocket |
| Docker | 20+ / compose v2 | 服务端部署 |
| JDK | 17 | 构建 Android |
| Gradle | 8.7 | 构建 Android |
| Android SDK | compileSdk 34 / minSdk 24 | Android 7.0+ |
| Kotlin | 1.9.24 | |
| Media3 | 1.3.1 | ExoPlayer |
| mpv | 任意较新版本 | Windows 播放内核 |

## 联调与测试

```bash
# 服务器上跑两个模拟成员，验证广播与防抖
cd tools/sim && go run . -room TEST01

# 向房间发一条指令，观察客户端反应
cd tools/ctl && go run . -room TEST01 -name remote -cmd play -pos 30
```

`tools/` 里两个小程序是协议层的"遥控器"，改完服务端拿它们回归最快，不用开播放器。

## 常见问题

<details>
<summary><b>两个人不在同一个房间，各看各的</b></summary>

最常见原因：**两个人都直接回车**，于是各自创建了一个房间。正确流程是——一个人先创建，把控制台醒目打印的 6 位房间码（已自动复制到剪贴板）发给另一个人，后者启动时填进「房间码」这一步。

如果看到红字「你输入的房间码 XXX 不存在或已解散」，说明那个房间已解散或码打错了，你其实新建了一个空房间。核对后重启即可。Android 端同理，房间码显示在播放页顶部，点一下可复制。
</details>

<details>
<summary><b>两个人画面越看越歪</b></summary>

先确认片源时长是否一致（成员列表会显示）。若时长一致仍在漂移，说明客户端解码速度差异，服务器会在偏差 > 1s 时强制纠偏，检查日志里有没有 `forceSeek`。
</details>

<details>
<summary><b>连不上服务器 / 一直重连</b></summary>

按顺序排查：① 安全组是否放行端口 ② 服务器 `docker ps` 容器是否 Up ③ `curl http://IP:端口/health` 是否有响应 ④ 客户端 `server.txt` 地址是否含多余空格。
</details>

<details>
<summary><b>Windows 提示找不到 mpv</b></summary>

`windows/mpv/mpv.exe` 不存在。跑 `windows/fetch-mpv.ps1`，或手动把 mpv 解压到 `windows/mpv/`，也可用 `-mpv "C:\path\mpv.exe"` 指定。
</details>

<details>
<summary><b>Android 某些片子没声音</b></summary>

系统解码器不支持 DTS 等特殊音轨。MP4/MKV 的 H.264/H.265 + AAC 覆盖良好，遇到无声换片源或转码。
</details>

<details>
<summary><b>会被运营商判定 PCDN 吗</b></summary>

本架构不产生任何家庭宽带上行视频流量，只有几十字节的同步指令。**红线：永远不要加客户端间 P2P 传视频的功能**，详见 [docs/01-方案设计.md](docs/01-方案设计.md) 第二章。
</details>

## 已知限制

- Windows 端一台机器只能跑一个实例（mpv 命名管道被占用）
- Android 端切后台会暂停（未做前台 Service，观影场景本身需要亮屏）
- 不做视频分发，所有人须自备同一片源
- 服务端无持久化，房间随进程内存存在（重启即清空，符合使用场景）

## Roadmap

- [x] v0.2 Windows 进房入口（此前只能靠命令行 `-room`）、房间码彩色横幅 + 自动复制、双端「房间不存在」提示、Android 界面改版与输入框可读性修复
- [ ] v0.3 房主踢人、房间锁、Android 前台 Service（后台不掉线）
- [ ] v0.4 语音/文字聊天（纯信令，成本极低）
- [ ] v1.0 一体化安装包、自动更新

## 许可与免责

MIT License，见 [LICENSE](LICENSE)。

本项目仅同步播放控制指令，不提供、不分发任何视频内容。请确保你观看的片源来源合法。
