# SyncWatch 进度账本（v0.6 Android 语音）

> 本文件是当前工作线的唯一状态账本（契约见 [`.agents/skills/session-handover/SKILL.md`](.agents/skills/session-handover/SKILL.md)）。
> 接力顺序：§0 → [`docs/adr/0001-android-voice-chat.md`](docs/adr/0001-android-voice-chat.md) → `docs/02-同步协议.md` → §7 当前任务。
> 本文件只记「现在在哪、下一步做什么、哪些还没验证」；设计理由在 ADR，实现事实在 `docs/`。
> 状态机：⬜ 未开始 → 🚧 实现中 → ⏸ 阻塞(等待 X) → 🔶 待验收 → ✅ 已验收。

## 0. 会话入口

> 路由器：只存指针与唯一值；动态值标〔流水 12.N〕。落后于最新流水时以流水为准，只报告不擅改。

| 项 | 值 |
|---|---|
| 最后更新 | 2026-11-10 |
| 工作线 | v0.6 Android 内置语音（`feat/android-voice`，fork 扩展） |
| 分支 / 基线 | `feat/android-voice`（文档头 `32ff2fe`；`fork/main` 已含 ADR 与交接账本；开工前 `git merge main`） |
| 状态 | fork 语音扩展先行；ADR 仍 `Proposed`；12.1–12.6 已验收；T1–T5 未开始〔流水 12.6〕 |
| 当前占用 | Windows 开发机 `D:\Study\Project\syncwatch` |
| 最近刷新 | 流水 12.6（2026-11-10） |
| 下一步 | 完成开工前文档/设计口径修正 → T1；上游 PR 待 T1–T4 验证后由主项目维护者确认〔流水 12.6〕 |
| 开放决策 | 见 §9 待定表（LiveKit 版本、TLS、流量阈值、机型清单等） |
| 已验证 | 源码通读（v0.5.1）、服务器只读检查、ADR/交接账本随本次提交入 `fork/main` |
| 未验证 | 未编译、未上真机、未部署 LiveKit |
| 导航 | 本表 → ADR → `docs/02` → §7 当前任务行；历史见 §12 |

## 1. 目标

给 fork 的 Android 客户端加内置语音（fork 目标 v0.6；上游版本号待定）：两人看同一部片时全双工对话，语音房间直接复用 SyncWatch 的 6 位房间码，不额外装语音软件。本扩展不改变上游核心设计。

## 2. 边界

第一版只保证：两人、Android、戴耳机、全双工。

不做：Windows 内置语音、视频/屏幕传输、客户端 P2P、云端录像、5 人混音、语音转文字、语音消息。

## 3. Fork 已定决策（ADR 仍 Proposed，上游待确认）

| # | 决策 | 理由（详见 ADR） |
|---|---|---|
| 1 | 语音是 **fork 的可选扩展层**，独立容器，不进 `syncwatch` 核心进程 | 隔离故障域；不装 LiveKit 时 fork 行为与上游一致 |
| 2 | Android 用原生 LiveKit SDK，不用 WebView | 后台音频和麦克风生命周期可控 |
| 3 | LiveKit room = 6 位房间码；identity = 昵称 + 会话后缀；token 只给音频收发 | 复用现有房间模型，避免重连/同名冲突，权限最小化 |
| 4 | MVP 由 `hello` 下发 `voiceUrl` + 6 小时 token；仅两人私有使用 | 少一个接口；URL 与 token 同生同灭；6h 覆盖一次观看 |
| 5 | 只开 7880/tcp、7881/tcp、7882/udp，3478/udp 仅 TURN | 避免整段 UDP 暴露 |
| 6 | 切后台关麦、继续收听；无耳机不许开麦 | 隐私与回授风险 |
| 7 | 不做主动 ducking；按量流量设告警，超限停语音 | 不干扰观影；控制账单 |

**需要主项目维护者单独点头的一条**：fork 启用语音后，从「单二进制、纯信令、零媒体流量」变成「核心信令 + 可选媒体面」；上游项目定位不变，合并 PR 时单独说明。

## 4. 进度（静态背景）

截至 2026-11-06：ADR 0001 草案（状态 Proposed）与本交接文档已完成；功能代码未开始。此后进度只写 §0 入口与 §12 流水，本节不再更新。

### 基线变更（2026-11-07，上游 `940adf8`）

设计基于 v0.5.1（`f0e4a34`）做出，上游随后提交了 `940adf8`「Android 现代化 UI 与播放手势重做（v0.5.2-ui）」，版本号已到 `0.5.2-ui`（versionCode 6）。对本方案的影响：

- 设计决策全部不变：信令/媒体分离、LiveKit、token、后台与耳机策略都与 UI 无关。
- Android 接入点变了：`MainActivity.kt` 重写（约 1300 行改动），新增 `PlayerControlOverlay.kt`、`PlayerGestureController.kt`、`SeekPreviewCalculator.kt`。语音按钮和状态应挂在新版播放控制层，不要按 v0.5.1 的布局写。
- `SyncEngine.kt` 有小改（+34/-1），`hello` 解析位置从第 133 行移到第 139 行。
- 服务端 `server/main.go` 未变，`Msg` 结构体仍在第 38 行。

结论：`feat/android-voice` 已包含 `main` 的文档提交；开始写 T3 前先 `git merge main` 到最新代码，再按新结构接入。

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
git pull --ff-only

# 落后 main 时
git merge main
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

开工前按 [`.agents/skills/session-handover/SKILL.md`](.agents/skills/session-handover/SKILL.md) 的启动协议读 §0。然后按 T1 → T2 → T3 → T4 → T5 顺序做，不要跳。

## 7. 任务清单

任务状态总表（收尾时翻转对应行；详细完成标准见下方）：

| 任务 | 状态 | 完成标准（摘要） | 证据 |
|---|---|---|---|
| T1 LiveKit 独立部署 | ⬜ 未开始 | `docker compose up -d` 后能进房并听到回声测试音 | 待补 |
| T2 服务器下发 token | ⬜ 未开始 | `hello` 含 `voiceUrl` 与 6h 音频 token，权限与房间/昵称匹配 | 待补 |
| T3 Android 接入 | ⬜ 未开始 | 单机能进语音房并听到远端；拔耳机立即关麦 | 待补 |
| T4 双机联调 | ⬜ 未开始 | ADR 验收标准逐条通过并记录机型/网络 | 待补 |
| T5 文档 | ⬜ 未开始 | fork 文档目录补齐语音说明；上游 docs/README 原文不动 | 待补 |

### T1 LiveKit 独立部署

- 新增 `voice/docker-compose.yml` + `voice/livekit.yaml`，镜像固定 tag，不写 `latest`
- 只开 7880/tcp、7881/tcp、7882/udp；3478/udp 按需
- **完成标准**：服务器上 `docker compose up -d` 后，用 LiveKit 官方 CLI/示例签发一个临时 token，能进房并听到自己的回声测试音
- 注意：这一步不改 syncwatch 任何代码

### T2 服务器下发 token

- `server/main.go` 的 `Msg` 结构体（第 38 行起）增加 `VoiceUrl` 与 `VoiceToken` 字段，JSON tag 用 `json:"voiceUrl,omitempty"` / `json:"voiceToken,omitempty"`
- 仅在配置了 `LIVEKIT_URL` / `LIVEKIT_API_KEY` / `LIVEKIT_API_SECRET` 时生成；缺失时两个字段都不下发，客户端隐藏语音入口
- 在 `hello` 消息（第 446 行）里带上；`voiceUrl` 原样取 `LIVEKIT_URL`
- **完成标准**：模拟 join 时 `hello` 含 `voiceUrl` 与 token；解码后 room = 房间码、identity = 昵称 + 会话后缀、有效期 6h、只含音频收发权限

### T3 Android 接入

- Manifest：`RECORD_AUDIO`、`POST_NOTIFICATIONS`、`FOREGROUND_SERVICE_MICROPHONE`（API 34）
- 前台服务 + 常驻通知；切后台 `setMicrophoneEnabled(false)`、保持订阅
- 耳机输出检测：没有耳机时禁止开麦、但可收听
- **完成标准**：单机能进语音房、能听到远端；拔耳机立即关麦

### T4 双机联调

- 按 ADR「验收标准」逐条测：延迟 p50 ≤ 300 ms / p95 ≤ 600 ms、5% 丢包可懂、Wi-Fi→4G 重连、30 分钟不中断、播放同步不漂移
- **完成标准**：结果写回本文件或 PR，注明机型、系统版本、网络

### T5 文档

- 在 `docs/branches/android-voice/` 补语音协议、部署、Android 说明
- `docs/branches/android-voice/02-voice-protocol.md` 记录 `voiceUrl` 与 `voiceToken` 字段
- 上游 `README.md`、`docs/01`、`docs/02`、`docs/03`、`docs/05` 原文不动；PR 被接受后再回填

## 8. 兼容性结论（已核对源码）

`hello` 增加 `voiceUrl` / `voiceToken` 字段不会破坏旧客户端：

- 服务端是单个扁平 `Msg` 结构体（`server/main.go:38`），新增字段只影响新字段
- Android 用 `org.json` + `optString` 解析（`android/app/src/main/java/com/syncwatch/app/SyncEngine.kt:139`），未知字段本来就会被忽略
- Windows 端同理，T2 时再确认一次即可

因此语音 PR 只改 `server/`、`android/`、`docs/`，不动 Windows 客户端。这不违反 README「协议速查」的「改协议必须五处同步」——那条针对**语义变更**，这次是**新增可选字段**。PR 里要写明这个区别。

## 9. 待定

| 问题 | 现状 / 建议 |
|---|---|
| 现有 Android 切后台会暂停播放，且没有前台服务（见 `docs/05` 已知限制）。语音要后台继续收听，是否同时保持后台播放？ | 建议只保语音、视频照旧暂停，避免扩大改动；真机验证后再定 |
| 域名与 TLS | 暂不对外；实验期用 `ws://`；对外前必须加 TLS，并重估 TTL/房间复用/token 接口 |
| LiveKit 镜像固定到哪个版本 | T1 时选一个 release tag 并记录 |
| 流量告警阈值 | 按量计费，T1 部署后按实际码率估算 |
| 需要覆盖的机型/厂商 | T4 前列清单 |
| 本文件合并前是否删除 | 建议保留到 v0.6 发布 |
| 仓库级 `AGENTS.md` | 已建立（2026-11-10）；项目红线、文档权威链与会话交接条款见根目录 |

## 10. 多机协作规则

- 一条分支同一时间只在一台机器上活跃；换机前 commit + push，并更新 §0「当前占用」
- 开工前 `git fetch && git pull --ff-only`（共享分支不改写历史；落后 `main` 时用 `git merge main`）
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
| `docs/03-服务端部署.md` | Docker / systemd 部署、端口、运维与排障 |
| `docs/04-Windows客户端.md` | Windows 控制台版（排障用） |
| `docs/05-Android客户端.md` | Android 构建、配置、已知限制 |
| `docs/06-Windows客户端GUI设计.md` | Windows 图形版设计与实施记录 |
| `docs/07-Android客户端现代化UI设计.md` | Android UI 设计目标与规范 |
| `docs/08-Android客户端现代化UI实施.md` | Android UI 实施边界、阶段与验收矩阵 |
| `docs/adr/0001-android-voice-chat.md` | 语音方案的全部决策与验收标准 |
| `AGENTS.md` | 仓库级 agent 规则：红线、权威链、会话交接、代码变更纪律、验证约定 |
| `CONTEXT.md` | 术语表：上游、fork、语音扩展 |
| `docs/branches/android-voice/README.md` | fork 语音扩展的范围、文档索引与上游关系 |
| `docs/branches/android-voice/02-voice-protocol.md` | `hello` 的 `voiceUrl` / `voiceToken` 扩展 |
| `.agents/skills/session-handover/SKILL.md` | 会话级交接流程（本项目特化） |
| `.agents/skills/ledger-rotation/SKILL.md` | 里程碑级账本轮转与保留 |
| `PROGRESS.md` | 本文件：当前进度与交接 |

## 12. 交接流水

> 每次收尾追加一条 `### 12.N — 标题`；N 连续、只增，轮转不改号。证据写在本条内：commit / PR / 构建产物 / 真机或服务器记录。
> 历史索引（轮转后维护）：暂无。

### 12.1 — 2026-11-10 建立会话交接账本

- 类型：流程 / 文档
- 状态：✅ 已验收（维护者，2026-11-10）
- 内容：新增 §0 入口节、§7 任务状态总表、§12 流水；安装 `session-handover` 与 `ledger-rotation`；新增仓库级 `AGENTS.md`，并按 `karpathy-guidelines` 补「代码变更纪律」。
- 证据：本条所在收尾 commit（`git log -1 -- PROGRESS.md`）；ADR、`AGENTS.md`、`.agents/skills` 一并入库。
- 决策：账本固定为 `PROGRESS.md`；流水前缀 `### 12.`；轮转阈值 40 条或 60 KB，保留最近约 20 条。

### 12.2 — 2026-11-10 明确 fork 边界与上游合并策略

- 类型：范围 / 流程
- 状态：✅ 已验收（维护者，2026-11-10）
- 内容：语音仅作为 fork 扩展；不改变上游 README 红线、`docs/01` 范围与版本路线图；未配置 LiveKit 时行为与上游一致；ADR 保持 `Proposed`，上游合并前由主项目维护者确认。
- 证据：`docs/adr/0001-android-voice-chat.md` 标题/状态/适用范围/决策 1；`PROGRESS.md` §0、§1、§3。
- 决策：T1–T4 在 fork 验证；验证后从 fork 向 `origin/main` 提 PR；当前无上游写权限，合并由主项目维护者执行；PR 只带语音代码与必要文档，不带 `AGENTS.md`、`.agents/`、`PROGRESS.md`。

### 12.3 — 2026-11-10 建立 fork 语音文档目录

- 类型：文档
- 状态：✅ 已验收（维护者，2026-11-10）
- 内容：新增 `CONTEXT.md` 术语表与 `docs/branches/android-voice/README.md`；语音文档只写 fork 目录；上游 `README.md`、`docs/01`、`docs/02`、`docs/03`、`docs/05` 原文不动；T5 改为补 fork 文档。
- 证据：`CONTEXT.md`；`docs/branches/android-voice/README.md`；`PROGRESS.md` §0、§7、§11。
- 决策：PR 被上游接受前不回填上游文档。

### 12.4 — 2026-11-10 明确 hello 下发 voiceUrl 与 voiceToken

- 类型：设计 / 协议
- 状态：✅ 已验收（维护者，2026-11-10）
- 内容：`hello` 同时下发 `voiceUrl` 与 `voiceToken`；两者仅在 LiveKit 三个环境变量齐全时出现，缺一不下发；URL 非秘密，token 为 bearer 凭证。
- 证据：`docs/adr/0001-android-voice-chat.md` 决策 4；`docs/branches/android-voice/02-voice-protocol.md`；`PROGRESS.md` §3、§7、§8。
- 决策：客户端用 `voiceUrl` 连接 LiveKit，不再从信令地址硬编码推导。

### 12.5 — 2026-11-10 语音 identity 改为昵称 + 会话后缀

- 类型：设计
- 状态：✅ 已验收（维护者，2026-11-10）
- 内容：LiveKit identity 不再直接使用昵称，改为「昵称 + 每次 join 生成的短后缀」；界面仍显示昵称。避免断线重连或昵称释放后与旧 LiveKit 连接冲突。
- 证据：`docs/adr/0001-android-voice-chat.md` 决策 3；`docs/branches/android-voice/02-voice-protocol.md` token 约束；`PROGRESS.md` §3、T2 完成标准。
- 决策：后缀只影响 LiveKit 内部身份，不改变用户可见昵称；每次 join 重新生成。

### 12.6 — 2026-11-10 确认仅两人私有使用与 6 小时 token

- 类型：范围 / 安全
- 状态：✅ 已验收（维护者，2026-11-10）
- 内容：暂不对外服务，仅两人私有使用；6 小时 token 覆盖一两集电视剧或一部电影；接受泄露后最长 6 小时被冒用的风险。
- 证据：`docs/adr/0001-android-voice-chat.md` 产品约束、决策 4、风险表；`docs/branches/android-voice/02-voice-protocol.md` 安全边界；`PROGRESS.md` §3、§9。
- 决策：对外服务前必须加 TLS，并重新评估 TTL、房间码复用与独立 token 接口。
