# 语音功能开发交接

> 接手顺序：本文件 → [`docs/adr/0001-android-voice-chat.md`](docs/adr/0001-android-voice-chat.md) → `docs/02-同步协议.md`。
> 本文件只记「现在在哪、下一步做什么、哪些还没验证」，设计理由不在这里重复。

| 项 | 值 |
|---|---|
| 最后更新 | 2026-11-06 |
| 分支 | `feat/android-voice`（基线 `bf3fa8e`，v0.5.2-ui） |
| 状态 | 文档已入 `main`；设计待维护者评审；功能代码 0 行 |
| 当前占用 | Windows 开发机，工作区 `D:\Work\Agent-Codex\work\syncwatch` |
| 已验证 | 源码通读（v0.5.1）、服务器只读检查、文档已推送 `fork/main` |
| 未验证 | 未编译、未上真机、未部署 LiveKit |

## 1. 目标

给 Android 客户端加内置语音（目标 v0.6）：两人看同一部片时全双工对话，语音房间直接复用 SyncWatch 的 6 位房间码，不额外装语音软件。

## 2. 边界

第一版只保证：两人、Android、戴耳机、全双工。

不做：Windows 内置语音、视频/屏幕传输、客户端 P2P、云端录像、5 人混音、语音转文字、语音消息。

## 3. 已定决策

| # | 决策 | 理由（详见 ADR） |
|---|---|---|
| 1 | 语音是**可选伴生服务**，独立容器，不进 `syncwatch` 核心进程 | 隔离故障域；不装 LiveKit 时项目功能不退化 |
| 2 | Android 用原生 LiveKit SDK，不用 WebView | 后台音频和麦克风生命周期可控 |
| 3 | LiveKit room = 6 位房间码；identity = 昵称；token 只给音频收发 | 复用现有房间模型，权限最小化 |
| 4 | MVP 由 `hello` 直接下发 6 小时 token | 少一个接口；过期重连再取 |
| 5 | 只开 7880/tcp、7881/tcp、7882/udp，3478/udp 仅 TURN | 避免整段 UDP 暴露 |
| 6 | 切后台关麦、继续收听；无耳机不许开麦 | 隐私与回授风险 |
| 7 | 不做主动 ducking；按量流量设告警，超限停语音 | 不干扰观影；控制账单 |

**需要维护者单独点头的一条**：项目定位从「单二进制、纯信令、零媒体流量」变成「核心信令 + 可选媒体服务」。这是 ADR 的核心，PR 描述里要单独提。

## 4. 进度

已完成：

- ADR 0001 草案（状态 Proposed）
- 本交接文档

未开始：

- `voice/` Docker Compose 与 LiveKit 配置
- 服务器生成语音 token
- Android 权限、前台服务、耳机检测、LiveKit SDK 接入
- 双机真机验证
- `docs/02`、`docs/05`、`README` 的语音章节

### 基线变更（2026-11-07，上游 `940adf8`）

设计基于 v0.5.1（`f0e4a34`）做出，上游随后提交了 `940adf8`「Android 现代化 UI 与播放手势重做（v0.5.2-ui）」，版本号已到 `0.5.2-ui`（versionCode 6）。对本方案的影响：

- 设计决策全部不变：信令/媒体分离、LiveKit、token、后台与耳机策略都与 UI 无关。
- Android 接入点变了：`MainActivity.kt` 重写（约 1300 行改动），新增 `PlayerControlOverlay.kt`、`PlayerGestureController.kt`、`SeekPreviewCalculator.kt`。语音按钮和状态应挂在新版播放控制层，不要按 v0.5.1 的布局写。
- `SyncEngine.kt` 有小改（+34/-1），`hello` 解析位置从第 133 行移到第 139 行。
- 服务端 `server/main.go` 未变，`Msg` 结构体仍在第 38 行。

结论：`feat/android-voice` 已包含 `main` 的文档提交；开始写 T3 前先 `git merge main`（或 rebase）到最新代码，再按新结构接入。

## 5. 环境事实

服务器（只读检查过，未做任何变更）：

- 阿里云轻量应用服务器，西南1（成都），Alibaba Cloud Linux 3.21.04
- 2 vCPU / 2 GiB / 40 GiB 系统盘；峰值带宽 200 Mbps；**无固定流量包，按量计费**
- 已运行 `syncwatch` 容器：内存 11.33 MiB、CPU 0.00%、端口 8995、状态 healthy
- 空闲负载 0.07/0.05/0.00，资源足够跑 LiveKit
- 公网 IP **不写进仓库**；本机记录在 `private/`（已 gitignore）或 SSH config 别名
- 已确认可以添加 UDP 防火墙规则；`rpcbind` 仍在 111 端口监听所有网卡（未处理，低优先级）

端口计划：

| 协议 | 端口 | 用途 |
|---|---|---|
| TCP | 7880 | LiveKit 信令 |
| TCP | 7881 | WebRTC over TCP 回退 |
| UDP | 7882 | WebRTC 媒体（单端口 mux） |
| UDP | 3478 | TURN 兜底 |

客户端与工具链：

- Kotlin + Media3(ExoPlayer) + OkHttp WebSocket；minSdk 24 / target 34 / compile 34；Gradle 8.7；JDK 17
- 本机已有 JDK 17；**未装 Android SDK、adb、gh CLI**
- `android/build-apk.bat` 写死从 `F:\android-tools\...` 找 JDK/SDK，换机器要改脚本或设 `JAVA_HOME`、`ANDROID_HOME`
- AGP 不接受中文路径；手动构建前把工程放到纯 ASCII 路径（脚本会复制到 `%TEMP%`）
- 仓库已有红线：`android/local.properties`、`server/.env`、`private/` 均不入库

## 6. 接手步骤

```powershell
# 第一次（新机器）
git clone https://github.com/<你的GitHub用户名>/syncwatch.git
cd syncwatch
git switch feat/android-voice

# 以后每次开工
git fetch origin
git pull --rebase
```

如果分支还没推到 GitHub，先在原机器执行（fork 后）：

```powershell
git remote add fork https://github.com/<你的GitHub用户名>/syncwatch.git
git push -u fork feat/android-voice
```

环境自检：

```powershell
java -version          # 需要 17
adb devices            # 需要至少一台真机
docker compose version # 服务器上需要
```

然后按 T1 → T2 → T3 → T4 → T5 顺序做，不要跳。

## 7. 任务清单

### T1 LiveKit 独立部署

- 新增 `voice/docker-compose.yml` + `voice/livekit.yaml`，镜像固定 tag，不写 `latest`
- 只开 7880/tcp、7881/tcp、7882/udp；3478/udp 按需
- **完成标准**：服务器上 `docker compose up -d` 后，用 LiveKit 官方 CLI/示例签发一个临时 token，能进房并听到自己的回声测试音
- 注意：这一步不改 syncwatch 任何代码

### T2 服务器下发 token

- `server/main.go` 的 `Msg` 结构体（第 38 行起）增加 `VoiceToken` 字段，JSON tag 用 `json:"voiceToken,omitempty"`
- 仅在配置了 `LIVEKIT_URL` / `LIVEKIT_API_KEY` / `LIVEKIT_API_SECRET` 时生成；缺失时字段留空，客户端隐藏语音入口
- 在 `hello` 消息（第 446 行）里带上
- **完成标准**：模拟 join 时 `hello` 含 token；解码后 room = 房间码、identity = 昵称、有效期 6h、只含音频收发权限

### T3 Android 接入

- Manifest：`RECORD_AUDIO`、`POST_NOTIFICATIONS`、`FOREGROUND_SERVICE_MICROPHONE`（API 34）
- 前台服务 + 常驻通知；切后台 `setMicrophoneEnabled(false)`、保持订阅
- 耳机输出检测：没有耳机时禁止开麦、但可收听
- **完成标准**：单机能进语音房、能听到远端；拔耳机立即关麦

### T4 双机联调

- 按 ADR「验收标准」逐条测：延迟 p50 ≤ 300 ms / p95 ≤ 600 ms、5% 丢包可懂、Wi-Fi→4G 重连、30 分钟不中断、播放同步不漂移
- **完成标准**：结果写回本文件或 PR，注明机型、系统版本、网络

### T5 文档

- `docs/02-同步协议.md` 加 `voiceToken` 字段
- `docs/05-Android客户端.md` 加语音章节
- `README.md` 加 v0.6 说明

## 8. 兼容性结论（已核对源码）

`hello` 增加字段不会破坏旧客户端：

- 服务端是单个扁平 `Msg` 结构体（`server/main.go:38`），新增字段只影响新字段
- Android 用 `org.json` + `optString` 解析（`android/app/src/main/java/com/syncwatch/app/SyncEngine.kt:139`），未知字段本来就会被忽略
- Windows 端同理，T2 时再确认一次即可

因此语音 PR 只改 `server/`、`android/`、`docs/`，不动 Windows 客户端。这不违反 README 第 318 行「改协议必须五处同步」——那条针对**语义变更**，这次是**新增可选字段**。PR 里要写明这个区别。

## 9. 待定

| 问题 | 现状 / 建议 |
|---|---|
| 现有 Android 切后台会暂停播放，且没有前台服务（见 `docs/05` 已知限制）。语音要后台继续收听，是否同时保持后台播放？ | 建议只保语音、视频照旧暂停，避免扩大改动；真机验证后再定 |
| 域名与 TLS | 实验期用 `ws://`；对外前必须加，token 接口届时可迁到 `/voice-token` |
| LiveKit 镜像固定到哪个版本 | T1 时选一个 release tag 并记录 |
| 流量告警阈值 | 按量计费，T1 部署后按实际码率估算 |
| 需要覆盖的机型/厂商 | T4 前列清单 |
| 本文件合并前是否删除 | 建议保留到 v0.6 发布 |
| 仓库级 `AGENTS.md` | 暂不做，等用户提供模板 |

## 10. 多机协作规则

- 一条分支同一时间只在一台机器上活跃；换机前 commit + push，并更新本文件顶部「当前占用」
- 开工前 `git fetch && git pull --rebase`
- 不对共享分支 `push --force`；分支名按任务起，不按机器起
- 密钥只放服务器 `.env` 或本机 `private/`；仓库里只放 `.example`
- 两台机器统一 JDK 17 / SDK 34 / Gradle 8.7，否则会出现「我这儿能编译」
- 提交风格跟仓库现有习惯：`feat(v0.6): 中文描述`、`docs: 中文描述`

## 11. 文档索引

| 文档 | 内容 |
|---|---|
| `README.md` | 项目总览、构建、协议红线 |
| `docs/01-方案设计.md` | 整体架构与「不做视频分发」的边界 |
| `docs/02-同步协议.md` | WebSocket 消息格式与状态机 |
| `docs/05-Android客户端.md` | Android 构建、配置、已知限制 |
| `docs/adr/0001-android-voice-chat.md` | 语音方案的全部决策与验收标准 |
| `PROGRESS.md` | 本文件：当前进度与交接 |
