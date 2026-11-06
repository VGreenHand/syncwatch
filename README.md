# 一起看 · SyncWatch

> 2~5 人异地同步观看**各自本地已有**的视频：播放、暂停、拖动进度实时对齐，谁操作的会立刻提示。
> **视频数据一字节都不上传**，服务器只转发几十字节的同步指令。

<p>
<a href="https://github.com/yangnicejin/syncwatch/actions/workflows/build.yml"><img alt="build" src="https://github.com/yangnicejin/syncwatch/actions/workflows/build.yml/badge.svg"></a>
<img alt="server" src="https://img.shields.io/badge/server-Go%201.21-00ADD8">
<img alt="windows" src="https://img.shields.io/badge/Windows-mpv%20IPC-0078D4">
<img alt="android" src="https://img.shields.io/badge/Android-Kotlin%20%2B%20Media3-3DDC84">
<img alt="license" src="https://img.shields.io/badge/license-MIT-green">
</p>

---

## 为什么不做成"在线一起看"

常见的同步观影方案要把视频推给每个人：流量大、成本高，家庭宽带上行还容易被运营商判定为 PCDN。

本项目反过来做——**每个人磁盘上本来就有同一个片子**，网络只需要对齐"按了播放 / 暂停 / 拖到 12:34"这几条指令。

| 特性 | 说明 |
|------|------|
| 零视频流量 | 网络上只有 JSON 信令，单房间实测 < 2 KB/s；看一部两小时的片子，全房间总流量不到 1 MB |
| 无 PCDN 风险 | 架构上不存在"家庭宽带上行分发视频"的形态，家宽侧没有视频流特征 |
| 全格式播放 | Windows 用 mpv（ffmpeg 系），RMVB / MKV / HEVC / 外挂字幕都能放 |
| 跨端互通 | Windows 与 Android 同一套协议，可以混在同一个房间里 |
| 服务端权威 | 房间状态由服务器持有，暂停竞态、进度漂移由服务器裁决，不靠客户端互相同步 |
| 成员可见 | 谁在线、谁在缓冲、谁暂停了，界面实时显示并标注是谁操作的 |
| 极轻部署 | 1 核 1G 云主机足够，服务端是单文件 Go 程序，也能直接跑二进制不起 Docker |

## 架构

```
        ┌──────────────────────────────────────────────┐
        │         ECS / 任意云主机（1 核 1G 够用）           │
        │  ┌────────────────────────────────────────┐  │
        │  │  syncwatch-server（Go，单二进制）        │  │
        │  │  房间管理 · 权威状态机 · 防抖 · 漂移纠偏  │  │
        │  └────────────────────────────────────────┘  │
        └───────▲───────────────────────▲──────────────┘
                │ ws:// 信令 < 2KB/s      │
        ┌───────┴────────┐      ┌────────┴─────────┐
        │ Windows 客户端   │      │ Android 客户端    │
        │ mpv + JSON IPC  │      │ Media3 ExoPlayer │
        └───────┬────────┘      └────────┬─────────┘
                │                        │
          本地视频文件               本地视频文件
        （不传输、不上传、不外发、不互传）
```

**红线**：永远不要加入客户端之间传视频、或服务器分发文件的逻辑。那会让整个项目失去合规前提。

## 三分钟上手

### 0. 你需要准备

- 一台有公网 IP 的服务器（云主机即可，Ubuntu / Debian 都行）
- 参与的人各自持有**同一个版本**的视频文件（时长必须一致）
- Windows：用分发包时无需安装任何东西（mpv 随包）；要从源码编译则需 Go 1.22+ / Node 18+ / Wails CLI
- Android 7.0 以上

### 1. 起服务端

**方式 A：Docker（推荐）**

```bash
git clone https://github.com/yangnicejin/syncwatch.git
cd syncwatch/server
cp .env.example .env          # 国内服务器把镜像源换成 daocloud，见文件内注释
docker compose up -d --build
curl http://127.0.0.1:8995/health    # {"status":"ok","rooms":0}
```

**方式 B：直接跑二进制**（不想装 Docker）

```bash
cd server && go build -o syncwatch-server . && PORT=8995 ./syncwatch-server
```

或者从 [Releases](https://github.com/yangnicejin/syncwatch/releases) 下载 `syncwatch-server-linux-amd64`，`chmod +x` 后直接运行，不需要 Go 环境。

> **记得在云服务器安全组放行 TCP 8995**（或你自定义的端口）。没放行的话，客户端会一直重连。

部署细节（镜像源、systemd 常驻、升级、迁移、限流加固）见 [docs/03-服务端部署.md](docs/03-服务端部署.md)。

### 2. Windows 端

同一套协议有两个 Windows 客户端，**日常用图形版，排障用控制台版**：

| | 图形版 `gui/`（默认） | 控制台版 `windows/` |
|---|---|---|
| 形态 | 单窗口，mpv 画面内嵌在界面里 | 命令行四步问答 + 独立 mpv 窗口 |
| 房间码 | 顶部显示，点一下复制 | 控制台彩色横幅 + 自动复制 |
| 成员 / 片源 | 右侧成员面板，标出谁在缓冲、时长是否一致 | 控制台文字列表 |
| 分发包 | `syncwatch-windows-gui-<版本>.zip`（Releases 里下载，或本地 `一起看-windows-v<版本>.zip`） | `syncwatch-windows-portable.zip` |
| 谁构建 | 本地打包后手动附到 Release | CI 打 tag 时自动构建 |

#### 图形版（推荐）

需要 Go 1.22+、Node 18+、Wails CLI：

```bat
go install github.com/wailsapp/wails/v2/cmd/wails@latest
cd gui
powershell -ExecutionPolicy Bypass -File ..\windows\fetch-mpv.ps1   :: 准备播放内核（与命令行版共用同一份 mpv）
echo 你的IP:8995 > server.txt                                       :: 预置地址，打包时会带上
build.bat                                                           :: 等价 wails build -platform windows/amd64
打包.bat                                                             :: 产出 dist\一起看-windows-v0.5.1.zip
```

解压后双击 `开始一起看.bat`。包内若带 `server.txt` 会自动读取；不带则首次打开点右上角齿轮填 `1.2.3.4:8995`，填完可点「测试连接」确认。

> **注意**：Releases 里的 `syncwatch-windows-portable.zip` 是**控制台版**，自 v0.5 起只作排障兜底。图形版是 `syncwatch-windows-gui-<版本>.zip`，由维护者本地构建后手动附到 Release——CI 只装了 Go 和 Android SDK，没有 Node / Wails。公开的图形版包**不含** `server.txt`，解压后首次启动要自己填服务器地址；需要 CI 也产图形版，得在 workflow 的 windows job 里加 Node + Wails CLI 两步。

界面流程：

```
首页填昵称 → 创建房间 / 加入房间 → 选择本地视频 → 进播放页
播放页：顶部  片名 + 房间码（点击复制）+ 连接状态 + 服务器地址 + 时间
        中间  视频（内嵌在同一个窗口里，不是独立窗口）
        底部  播放/暂停 · 进度 · 倍速 · 音量 · 离开
        右侧  成员面板（谁在播放、谁在缓冲、片源时长是否一致）
```

播放页的快捷操作（v0.5.1）：

| 操作 | 效果 |
|---|---|
| 单击画面 | 暂停 / 播放（双击画面 = 全屏） |
| 空格 | 暂停 / 播放 |
| ← / → | 快退 / 快进 5 秒，按住 Shift 为 30 秒；连按会连着走，松手才同步一次 |
| ↑ / ↓ | 音量 ±5（只影响自己） |
| F / M / Esc | 全屏 / 静音 / 收起成员面板或退出全屏 |

> 设计与实施记录见 [docs/06-Windows客户端GUI设计.md](docs/06-Windows客户端GUI设计.md)（第 12 章是技术验证结论与踩坑清单）。

#### 控制台版（排障用）

`windows/` 是 v0.1 起就有的命令行客户端，日志直白，定位"连不上 / 不同步"比图形版更快。

```bat
cd windows
build.bat        :: 会把 server.txt 里的地址编译进 exe
启动观影.bat      :: 四步问答：昵称 → 视频路径 → 房间码 → 密码
```

启动后依次输入：

```
[1/4] 你的昵称              → 房间里显示的名字
[2/4] 视频文件完整路径       → 直接把文件拖进窗口
[3/4] 房间码                → 有朋友的码就填；直接回车 = 自己创建一个新房间
[4/4] 房间密码              → 房间没设密码就直接回车
```

> 从源码自己编译时，`build.bat` 会读取 `server.txt` 并用 `-ldflags -X main.builtinServer=` 把地址**编译进 exe**，之后把 exe 单独发给别人也能直连。CI 产物故意不这么做——公开仓库里不该出现私人地址。

只有控制台版支持命令行参数，方便做快捷方式：

| 参数 | 说明 |
|------|------|
| `-server host:port` | 服务器地址（默认 `server.txt` → `SYNCWATCH_SERVER` 环境变量 → 内置值） |
| `-name 昵称` | 昵称，留空则交互输入 |
| `-room 房间码` | 留空则由服务器创建新房间 |
| `-password 密码` | 房间密码 |
| `-file 路径` | 视频文件，留空则交互输入 |
| `-mpv 路径` | 指定 mpv.exe（默认自动查找 `mpv\mpv.exe`，再退回 PATH） |

图形版没有命令行参数，地址、昵称都在界面上填。需要无人值守起播（调试 / 自动化）时用环境变量：`SYNCWATCH_AUTO_VIDEO`、`SYNCWATCH_AUTO_ROOM`（`new` = 建房）、`SYNCWATCH_AUTO_NAME`。

### 3. Android 端

从 [Releases](https://github.com/yangnicejin/syncwatch/releases) 下载 `syncwatch-android-*.apk` 侧载安装（Android 7.0+，需要允许"安装未知来源应用"）。

自己编译：

```bat
cd android
build-apk.bat            :: 产出 dist\一起看-android-v0.4.apk
```

需要 JDK 17 + Android SDK（compileSdk 34）。构建脚本会自动处理中文路径问题（把工程复制到临时目录再编译），也支持用 `local.properties` 里的 `syncwatch.server` 把默认服务器地址写进 APK。

界面流程：填服务器地址 → 填昵称 → 选视频 → 填房间码（留空则建房）→ 加入。房间码会显示在播放页顶部，点一下即可复制发给别人。

### 4. 一起看（关键一步）

**一个人建房，其他人填同一个房间码。** 这一步搞错就会"各看各的"：

```
你（房主）                                朋友
点「创建房间」→ 选片 → 开始             点「加入房间」→ 填 6ZJ46H → 选同一份片
（控制台版：房间码那一步直接回车）
  ↓                                       ↓
顶部显示：房间码 6ZJ46H [复制]  ──→       加入成功，双方成员列表互相可见
（房间码已自动复制到剪贴板）
```

如果填了不存在的房间码，**服务器会新建一个房间**而不是报错——为了避免你误以为进了朋友的房间，两端都会明确提示：

```
[注意] 你输入的房间码 XXX 不存在或已解散，
       所以新建了一个房间，并没有进到朋友的房间！
```

## 目录结构

```
syncwatch/
├── README.md                  ← 你正在看的文件
├── docs/                      ← 全部文档
│   ├── 01-方案设计.md          需求、架构选型、PCDN 风险分析、里程碑
│   ├── 02-同步协议.md          消息格式、状态机、漂移算法（改协议先看这个）
│   ├── 03-服务端部署.md        部署、加固、升级、迁移、排障
│   ├── 04-Windows客户端.md     控制台版使用与构建（排障用）
│   ├── 05-Android客户端.md     使用与构建
│   └── 06-Windows客户端GUI设计.md  图形版设计稿 + 实施记录 + 踩坑清单
├── server/                    ← Go WebSocket 信令服务器（全部逻辑在 main.go）
│   ├── main.go                 房间管理、权威状态机、心跳、限流
│   ├── Dockerfile
│   ├── docker-compose.yml
│   ├── deploy.sh               一键部署到远程主机（rsync + docker compose）
│   └── .env.example            端口、基础镜像、GOPROXY
├── gui/                       ← Windows 图形客户端（Wails + React + 内嵌 mpv，v0.5 起为默认）
│   ├── main.go                 Wails 启动配置（透明 WebView2 才能叠控制条）
│   ├── app.go                  暴露给前端的方法：建房/加入/播放控制/配置
│   ├── syncclient.go           同步协议（与服务端、Android 完全一致）
│   ├── player.go               mpv 启停与 JSON IPC
│   ├── win32.go                mpv 宿主子窗口 + 专用 UI 线程
│   ├── build.bat / 打包.bat     编译与打分发 zip
│   └── frontend/               React + TypeScript 界面
├── windows/                   ← Windows 命令行客户端（保留用于排障）
│   ├── main.go                 连接、同步逻辑、mpv 事件处理
│   ├── ui.go                   控制台配色、房间码横幅、剪贴板、窗口标题
│   ├── build.bat               编译（注入内置服务器地址）
│   ├── 启动观影.bat             双击入口
│   ├── 打包.bat                 产出可分发 zip
│   └── fetch-mpv.ps1           下载便携 mpv
├── android/                   ← Android 客户端（Kotlin + Media3）
│   └── app/src/main/java/com/syncwatch/app/
│       ├── MainActivity.kt     界面与播放器
│       ├── SyncEngine.kt       WebSocket 状态机、回声抑制、心跳
│       └── Protocol.kt         消息构造
├── tools/                     ← 联调工具（不是产品代码）
│   ├── sim/                    模拟多个成员进房，做协议回归
│   └── ctl/                    对指定房间发单条 play/pause/seek
├── dist/                      ← 构建产物（不入库，包内含内置地址）
└── .github/workflows/build.yml 打 tag 自动编译并发布 Release
```

## 仓库边界

这个仓库刻意不含任何私人信息，clone 下来可以直接公开再分叉：

| 不入库的东西 | 原因 | 怎么获得 |
|---|---|---|
| `private/` | 本地开发笔记：真实 IP、SSH、线上状态、待办、踩坑记录 | 只在作者本机，`.gitignore` 整目录排除 |
| `windows/mpv/`、`gui/build/`、`gui/frontend/node_modules/` | 100 MB+ 播放内核与构建产物 | 跑 `windows/fetch-mpv.ps1`；`wails build` 自动装前端依赖 |
| `windows/server.txt`、`gui/server.txt` | 一行真实服务器地址 | `echo 你的IP:8995 > server.txt` |
| `dist/`、`*.exe`、`*.zip`、`*.apk` | 分发包里可能内置了地址 | 本地构建 |
| `server/.env`、`server/deploy.env` | 部署目标与凭据 | 从对应 `.example` 复制 |
| `android/local.properties`、`local-env.bat` | 本机 SDK / JDK 绝对路径 | 从对应 `.example` 复制 |

所有 `.example` 只有占位值，照着填即可。提交前自查一遍（排除回环地址、示例地址和 Win32 版本号这些合法命中，剩下的才需要看）：

```bash
git add -A
git grep --cached -nE '([0-9]{1,3}\.){3}[0-9]{1,3}|C:\\Users' \
  | grep -vE '127\.0\.0\.1|0\.0\.0\.0|1\.2\.3\.4|255\.255\.255\.255|6\.0\.0\.0' \
  || echo "干净"
```

## 同步是怎么做到的

服务端只维护**一个**房间级状态：`state`（playing / paused）、`position`（锚点时刻的进度）、`anchor`（锚点时间戳）、`rate`（倍速）。任意时刻的权威进度是：

```
current = position + (now - anchor) * rate      // 仅在 playing 时累加
```

客户端每 5 秒上报一次心跳位置，服务端据此判断漂移：

| 偏差 | 动作 |
|------|------|
| < 0.3 s | 忽略，不做任何处理 |
| 0.3 ~ 1 s | 记录，不打断播放 |
| > 1 s，且连续两次 | 只给**漂移的那个人**发 `forceSeek` |

几个关键取舍，都是踩过坑才定下来的：

- **不回显给发起者**：谁按的播放，谁本地已经执行了，服务器只广播给其他人。否则会形成 A→B→A 的回声。
- **seek 防抖 120ms**：拖动进度条会连发几十条事件，只广播最后一次。
- **收到 `hello` 前不上报本地事件**：mpv 启动瞬间的初始暂停会误广播，把别人的播放打断。
- **空房间保留 15 分钟**：手机切后台、WiFi 抖动导致的断线重连，回来时房间和进度都还在。
- **限制人数 5 人**：防滥用，也够家庭/朋友场景。

完整协议（消息格式、状态机、加入流程）见 [docs/02-同步协议.md](docs/02-同步协议.md)。

## 协议速查

传输层是明文 `ws://host:port/ws` 上的 UTF-8 JSON 文本帧，单条上限 8 KB，每连接限流 40 条/秒。

| 方向 | type | 载荷 | 说明 |
|------|------|------|------|
| C→S | `join` | `room`(空=建房), `name`, `password` | 必须是首条消息 |
| C→S | `play` / `pause` | `position`, `rate?` | 请求播放 / 暂停 |
| C→S | `seek` | `position` | 请求跳转 |
| C→S | `heartbeat` | `position`, `rate` | 每 5 秒，用于漂移检测 |
| C→S | `memberState` | `state` | 上报自己的播放器状态（含 buffering） |
| C→S | `fileInfo` | `file`, `size`, `duration` | 上报片源信息，用于一致性提示 |
| S→C | `hello` | 全量状态 + `created` | 加入成功；`created=true` 表示新建了房间 |
| S→C | `sync` | `state`, `position`, `anchorMs`, `rate`, `operator` | 状态变更广播（不回显发起者） |
| S→C | `forceSeek` | `position`, `operator` | 漂移纠偏，`operator` 固定为 `server(drift)` |
| S→C | `members` / `join` / `leave` | 成员信息 | 成员变化 |
| S→C | `joinError` / `error` | `error` | 加入失败（随后断开）/ 非致命错误 |

改协议必须**五处同步改**：`server/main.go`、`windows/main.go`（控制台版）、`gui/syncclient.go`（图形版，与前者同源）、`android/.../Protocol.kt`、`docs/02-同步协议.md`。漏一处不会报错，只会"没反应"——这是最难查的一类问题。

## 配置项

| 位置 | 作用 | 是否入库 |
|------|------|----------|
| `server/.env` | 端口、基础镜像、GOPROXY | 否（有 `.env.example`） |
| `server/deploy.env` | 部署目标的 host / port / user | 否（有 `.example`） |
| `windows/server.txt` | 控制台版默认服务器地址（一行 `host:port`，编译时注入 exe） | 否 |
| `gui/server.txt` | 图形版默认服务器地址（打包时打进 zip） | 否 |
| `windows/mpv/` | mpv 播放器（100MB+），两个 Windows 客户端共用 | 否，`fetch-mpv.ps1` 下载 |
| `gui/frontend/node_modules/`、`gui/build/` | 前端依赖与 Wails 构建产物 | 否 |
| `android/local.properties` | `sdk.dir` 与 `syncwatch.server` | 否（有 `.example`） |
| `local-env.bat` | 本机 Go / JDK / SDK 路径，构建脚本自动读取 | 否（有 `.example`） |
| `SYNCWATCH_SERVER` 环境变量 | 临时覆盖客户端服务器地址（调试用） | — |

## 技术栈

| 组件 | 版本 | 备注 |
|------|------|------|
| Go | 1.21+ | 服务端与 Windows 客户端共用 |
| gorilla/websocket | v1.5.3 | WebSocket 实现 |
| Docker | 20+ / compose v2 | 服务端部署 |
| JDK / Gradle | 17 / 8.7 | 构建 Android |
| Android SDK | compileSdk 34 / minSdk 24 | Android 7.0+ |
| Kotlin | 1.9.24 | |
| Media3 | 1.3.1 | ExoPlayer |
| mpv | 任意较新版本 | Windows 播放内核，`fetch-mpv.ps1` 自动取最新 |
| Wails | v2.17 | Windows 图形壳（WebView2） |
| React / Vite | 18 / 7 | 图形界面 |

## 构建与发布

| 产物 | 命令 | 谁产出 |
|---|---|---|
| 服务端二进制 | `cd server && go build -o syncwatch-server .` | 本地 / CI（linux amd64 + arm64） |
| 图形版 exe | `gui\build.bat`（= `wails build -platform windows/amd64`） | **仅本地** |
| 图形版 zip | `gui\打包.bat` → `dist\一起看-windows-v0.5.1.zip` | 本地构建；公开版手动附到 Release |
| 控制台版 zip | `windows\build.bat` + `windows\打包.bat` | 本地 / CI |
| Android APK | `android\build-apk.bat` → `dist\一起看-android-v0.5.1.apk` | 本地 / CI |
| 线上部署 | `server\deploy.sh`（rsync + docker compose） | 仅本地 |

**打 tag 自动发布**：推 `v0.5.1` 这类 tag 后，GitHub Actions 编译服务端（amd64 / arm64）、控制台版 Windows 包、Android APK，并在 Releases 页面建版本。**图形版不在 CI 范围内**（workflow 里没有 Node + Wails 步骤），维护者本地打包成 `syncwatch-windows-gui-<版本>.zip` 后手动附上去——这份包不含 `server.txt`。

发版时版本号散在 7 处，漏一处就会出现"关于里显示 v0.4、包名却是 v0.5"：

1. `android/app/build.gradle` 的 `versionCode` / `versionName`
2. `android/build-apk.bat` 的 `set "VER="`
3. `windows/打包.bat` 的 zip 文件名
4. `windows/ui.go` 的 `const AppVersion`
5. `gui/打包.bat` 的 `set "VER="`
6. `gui/app.go` 的 `const AppVersion`（v0.5.1 起显示在设置面板底部）
7. `server/main.go` 根路径返回的版本串

## 联调与测试

```bash
# 起一个本地服务端
cd server && PORT=8995 go run .

# 两个模拟成员进同一个房间，观察广播与防抖
cd tools/sim && go run . -room TEST01

# 向房间发单条指令，观察客户端反应
cd tools/ctl && go run . -room TEST01 -name remote -cmd seek -pos 300
```

`tools/` 里这两个小程序是协议层的"遥控器"，改完服务端拿它们回归最快，不用开播放器。

Android 调试构建还留了一个自动化入口（仅 `BuildConfig.DEBUG` 生效，正式包不会执行），可以跳过系统文件选择器直接进房：

```bash
adb shell am start -n com.syncwatch.app/.MainActivity \
  --es video test.mp4 --es server 127.0.0.1:8995 \
  --es name Tablet --es room TEST01 --ez autojoin true --ez autoplay true
```

## 常见问题

<details>
<summary><b>两个人都显示「已连接」，却互相看不到，操作也传不过去</b></summary>

先看**播放页顶栏的服务器地址**：两个人的那一串必须完全一致。

最常见的原因就是不一致——各自连到了不同的服务器。典型场景：一方按老习惯在设置里填了 `127.0.0.1:8995`（或 `localhost`），那只有他自己那台电脑能连；另一边填的是公网地址。两边都能连上、都能建房，房间码甚至还长得一样，但两个房间在两台不同的服务器上，所以成员列表里只有自己，拖动进度也不会影响对方。

排查顺序：

1. 播放页顶栏、或成员面板底部，读出双方的服务器地址，必须一致；
2. 地址是 `127.0.0.1` / `localhost` / `0.0.0.0` 时，客户端会用黄字标出「仅本机」——这种地址不可能和朋友一起看，改成服务器的公网地址；
3. 地址一致仍不行，去服务器上看 `docker logs syncwatch`，正常应有两条 `<昵称> joined`。

v0.5.1 起：分发包自带 `server.txt` 时，若本地配置里残留了回环地址，启动会自动改用 `server.txt` 的公网地址；服务器地址也会直接显示在播放页顶栏，不用再靠猜。
</details>

<details>
<summary><b>两个人都点了「创建房间」，各看各的</b></summary>

正确流程是——一个人创建，把播放页顶部显示的 6 位房间码（点一下即复制）发给另一个人，后者走「加入房间」填进去。

v0.5.1 起，如果填的房间码在服务器上不存在（打错、或空房超过 15 分钟被回收），会弹出阻断式提示「没有加入朋友的房间」——因为服务器会替你新建一个**同码的空房**，此时两边看着都是"已连接"，实际是两个房间。点「返回重新输入」核对房间码即可。Android 端同理，房间码显示在播放页顶部，点一下可复制。
</details>

<details>
<summary><b>拖进度条之后，两边跳回片头</b></summary>

已修复（v0.4）。根因是 mpv 的 `seek` 事件在跳转完成**之前**就发出，事件里带的位置还是旧值 0，被当成用户操作上报，服务器照单全收就把房间位置改成 0。现在改为等 `seeking` 属性变 false 后再读真实位置。

如果你还遇到，先确认两端都是 v0.4 及以上；仍有问题就看日志里有没有 `你跳转到了 0.0s`——控制台版直接看窗口输出，图形版在 `%TEMP%\syncwatch-gui.log`（界面右上角「文档」图标可一键打开）。
</details>

<details>
<summary><b>远端点了播放，Windows 卡在暂停不动</b></summary>

已修复（v0.4）。根因是 `mpv` 的 `pause` 是布尔值，代码却用读浮点数的方式去解析，失败后静默返回 0，于是 Windows 永远认为自己"没在暂停"，"播放"分支从不执行。

同样是升级到 v0.4 即可。升级后若仍有问题，检查 mpv 是否被别的进程占用（Windows 端一台机器只能跑一个实例；图形版日志见 `%TEMP%\syncwatch-gui.log`）。
</details>

<details>
<summary><b>两个人画面越看越歪</b></summary>

先确认片源时长是否一致（成员列表会显示各自时长）。若时长一致仍在漂移，说明解码速度有差异，服务器会在偏差 > 1 秒且连续两次时强制纠偏，日志里能看到 `forceSeek`。偶尔一次纠偏是正常的，频繁纠偏通常是片源版本不同。
</details>

<details>
<summary><b>连不上服务器 / 一直在重连</b></summary>

按顺序排查：① 安全组是否放行端口 ② 服务器上 `docker ps` 容器是否 Up ③ `curl http://IP:端口/health` 是否有响应 ④ 客户端 `server.txt` 是否含多余空格或换行 ⑤ 客户端填的是不是公网 IP（不是 `127.0.0.1`，那是服务器自己）。
</details>

<details>
<summary><b>Windows 提示找不到 mpv</b></summary>

两个客户端都按 `mpv\mpv.exe`（exe 同目录下的 mpv 文件夹）找播放内核。跑 `windows/fetch-mpv.ps1` 自动下载，或手动解压一份到该目录；控制台版也可以用 `-mpv "C:\path\mpv.exe"` 指定。图形版日志在 `%TEMP%\syncwatch-gui.log`，搜 `mpv` 能看到实际查找路径。
</details>

<details>
<summary><b>Android 某些片子没声音</b></summary>

系统解码器不支持 DTS 等特殊音轨。MP4/MKV 的 H.264/H.265 + AAC 覆盖良好，遇到无声换片源或转码。
</details>

<details>
<summary><b>会被运营商判定 PCDN 吗</b></summary>

本架构不产生任何家庭宽带上行视频流量，只有几十字节的同步指令，家宽侧没有视频分发特征。**红线：永远不要加客户端间 P2P 传视频的功能**，详见 [docs/01-方案设计.md](docs/01-方案设计.md) 第二章。
</details>

## 已知限制

- Windows 一台机器只能跑一个实例（mpv 命名管道固定，第二个抢不到）；图形版和控制台版混开也算两个
- Android 切后台会暂停（未做前台 Service；观影场景本身需要亮屏）
- 不做视频分发，所有人须自备同一片源
- 服务端无持久化：房间存在内存里，进程重启即清空；空房间 15 分钟后回收
- 明文 `ws://`，无加密。信令里只有进度和昵称，不含任何隐私内容；需要更高安全性可以自行套 TLS 换成 `wss://`，协议不用改

## 版本演进

- `v0.1` 服务端 + Windows + Android 三端跑通，协议联调与端到端冒烟测试
- `v0.2` Windows 加入房间入口（此前只能靠命令行 `-room`，双击启动必然各建各的房间）；房间码彩色横幅 + 自动复制；双端「房间不存在」提示；Android 界面改版、修复输入框白字压浅底
- `v0.3` 房间跨断线保留（15 分钟）；漂移纠偏只发给漂移者；Android 缓冲状态不再被误判成暂停；服务端位置参数校验
- `v0.4` 双端实机联调：修复 mpv 布尔属性解析错误导致的"远端放音本端不动"、seek 事件取错位置导致的"跳回片头"；成员状态实时上报；纠偏阈值分档
- `v0.5` Windows 图形客户端：单窗口一体化（视频内嵌、房间码可复制、成员面板、全屏控制条自动隐藏、片源时长提示）；**协议与服务端零改动**。分发包目前需本地打包，CI 仍只产控制台版
- `v0.5.1` 修「两台电脑都显示已连接却互相看不到」：根因是图形版的自动化联调入口会把 `127.0.0.1` 写进本地配置，而配置优先级高于分发包自带的 `server.txt`，于是房主连本机、朋友连公网，两边各建了一个同码房间。同时修：WS 并发写崩溃、断线不重连；加入不存在的房间码改为阻断式提示，回环地址与服务器地址在界面上明示；新增单击画面暂停/播放、方向键控进度与音量。已发布，Releases 里可下载图形版包 `syncwatch-windows-gui-v0.5.1.zip`
- `v0.6`（计划）房主踢人、房间锁、Android 前台 Service
- `v0.7`（计划）语音 / 文字聊天（纯信令，成本极低）
- `v1.0`（计划）一体化安装包、自动更新

## 许可与免责

MIT License，见 [LICENSE](LICENSE)。

本项目仅同步播放控制指令，不提供、不分发任何视频内容。请确保你观看的片源来源合法。
