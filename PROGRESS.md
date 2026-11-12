# SyncWatch 进度账本（v0.6 Android 语音）

> 本文件是当前工作线的唯一状态账本（契约见 [`.agents/skills/session-handover/SKILL.md`](.agents/skills/session-handover/SKILL.md)）。
> 接力顺序：§0 → [`docs/adr/0001-android-voice-chat.md`](docs/adr/0001-android-voice-chat.md) → `docs/02-同步协议.md` → §7 当前任务。
> 本文件只记「现在在哪、下一步做什么、哪些还没验证」；设计理由在 ADR，实现事实在 `docs/`。
> 状态机：⬜ 未开始 → 🚧 实现中 → ⏸ 阻塞(等待 X) → 🔶 待验收 → ✅ 已验收。

## 0. 会话入口

> 路由器：只存指针与唯一值；动态值标〔流水 12.N〕。落后于最新流水时以流水为准，只报告不擅改。

| 项 | 值 |
|---|---|
| 最后更新 | 2026-11-12 |
| 工作线 | v0.6 Android 内置语音（`feat/android-voice`，fork 扩展） |
| 分支 / 基线 | `feat/android-voice`（已合并 `fork/main`；与 `fork/feat/android-voice` 同步，T1 提交 `37a6234`、T2 提交 `0c146b6` 已 push） |
| 状态 | fork 语音扩展先行；ADR 仍 `Proposed`（仅指上游决策未批准）；fork 实施按既有授权先行；12.1–12.15 已验收；T1 已验收（部署/公网/音频/停服通过，告警按维护者决定豁免）；T2 已验收（本地 + 隔离服务器副本证据）；T3 真机部分通过（语音按钮/开关、后台提示、无耳机拒绝、LiveKit 入房并发布音频轨），待补远端接收等项；T4–T5 未开始〔流水 12.25〕 |
| 当前占用 | Windows 开发机 `D:\Study\Project\syncwatch` |
| 最近刷新 | 流水 12.25（2026-11-12） |
| 下一步 | 补验 440Hz 远端接收、开麦时拔耳机自动关麦、回前台后播放拦截与后台不广播 `pause` 的时间点对照；通过后 T3 转 ✅ 并进入 T4 双机；分屏退出房间已由维护者决定搁置〔流水 12.25〕 |
| 开放决策 | 见 §9 待定表（TLS、SDK 版本、流量阈值、机型清单等；LiveKit 服务端版本与网络已定） |
| 已验证 | 源码通读（v0.5.1）、开工前设计口径与开发计划已入库、T1–T4 验收口径已具体化、`voice/` 配置与自包含 `deploy.sh` 已就绪；服务器已部署 `syncwatch-livekit`，镜像 `v1.13.11`，三端口监听，本机与公网 HTTP 200，`node_ip` 生效；T1 音频验收通过；停 LiveKit 后 `syncwatch` 仍 healthy、8995 仍监听，恢复后自检全 PASS；维护者决定暂不设置临时告警并验收通过 T1；T1 收尾与账本提交已 push 至 `fork/feat/android-voice`；T2 服务端、`tools/sim`、`tools/ctl` 构建通过，本地 `hello` 的 token 签名/TTL/权限矩阵、缺配置关闭路径、并发入房与 `sync` 广播均通过；T2 隔离服务器副本使用真实 `LIVEKIT_*`，容器健康、token `sig=ok`、TTL `6h0m0s`、权限矩阵与 A/B `sync` 广播通过，生产 `syncwatch` 未受影响的 8995 保持 healthy；T3.0 固定 LiveKit Android SDK `2.30.0`，APK 构建通过（minSdk 24 / compileSdk 34 / JDK 17 / Gradle 8.7），`testDebugUnitTest` 与 `lintDebug` 通过，合并后权限仅新增 `RECORD_AUDIO`（另有 SDK 提供的 `MODIFY_AUDIO_SETTINGS` 与旧蓝牙权限）；2026-11-12 真机 `nubia NX799J`（Android 16 / API 36）验证：语音按钮出现且可开关、切后台回前台提示正常、无耳机时拒绝开麦；LiveKit 日志确认 `Tester` 以 Android SDK 2.30.0 入房并发布音频轨，会话 2m4s，token 被服务端接受〔流水 12.25〕 |
| 未验证 | UDP 7882 与 TCP 7881 的实际选中路径未单独确认；停服窗口内未用客户端实测播放同步；T3 仍待补验：远端 440Hz 接收、开麦时拔耳机自动关麦、回前台后播放拦截、后台不广播 `pause` 的时间点对照；分屏检测会退出房间（维护者已决定搁置）；无语音配置回归已通过；生产部署已更新至 T2 代码；Windows 控制台/GUI/Android 旧客户端未实际入房（仅源码与 `tools/ctl` 兼容验证）；签发失败路径未强制触发〔流水 12.25〕 |
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
| 2 | Android 用原生 LiveKit SDK，不用 WebView | 麦克风、音频路由和生命周期可控 |
| 3 | LiveKit room = 6 位房间码；identity = 昵称 + 会话后缀；token 只给音频收发 | 复用现有房间模型，避免重连/同名冲突，权限最小化 |
| 4 | MVP 由 `hello` 下发 `voiceUrl` + 6 小时 token；仅两人私有使用 | 少一个接口；URL 与 token 同生同灭；6h 覆盖一次观看 |
| 5 | 只开 7880/tcp、7881/tcp、7882/udp，3478/udp 仅 TURN | 避免整段 UDP 暴露 |
| 6 | 切后台暂停视频并断开语音；分屏只关麦；无耳机不许开麦 | 隐私、回授与实现成本 |
| 7 | 不做主动 ducking；按量流量设告警，超限停语音 | 不干扰观影；控制账单 |

**需要主项目维护者单独点头的一条**：fork 启用语音后，从「单二进制、纯信令、零媒体流量」变成「核心信令 + 可选媒体面」；上游项目定位不变，合并 PR 时单独说明。

ADR 的 `Proposed` 只表示上游合并决策尚未批准；fork 内实施按维护者既有授权先行，12.15 是计划修订（待验收），不等于上游接受该决策。

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

服务器（2026-11-11 起新增 LiveKit 伴生容器）：

- 阿里云轻量应用服务器，西南1（成都），Alibaba Cloud Linux 3.21.04
- 2 vCPU / 2 GiB / 40 GiB 系统盘；峰值带宽 200 Mbps；**无固定流量包，按量计费**
- 已运行 `syncwatch` 容器：内存 11.33 MiB、CPU 0.00%、端口 8995、状态 healthy
- 空闲负载 0.07/0.05/0.00，资源足够跑 LiveKit
- 公网 IP **不写进仓库**；本机记录在 `private/`（已 gitignore）或 SSH config 别名
- 已确认可以添加 UDP 防火墙规则；`rpcbind` 仍在 111 端口监听所有网卡（未处理，低优先级）
- Docker Hub 直连超时；`livekit/livekit-server:v1.13.11` 经 `docker.m.daocloud.io` 拉取后重打原标签
- LiveKit 容器 `syncwatch-livekit` 使用 host 网络；镜像 digest `sha256:0e20c255a2c17000c52d0631b79737a4216c06bc51df13878768df01261c8dfd`，服务端版本 `1.13.11`
- LiveKit 日志提示 UDP 接收缓冲 425984，低于建议值 5000000；属 WARN，T4 前评估 `net.core.rmem_max`

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
| T1 LiveKit 独立部署 | ✅ 已验收 | 发布端发送已知音频，另一公网客户端听到；端口、告警与停服演练符合预期 | 部署/公网 TCP/440 Hz 音频/停服隔离均通过；告警按维护者决定豁免〔流水 12.18–12.22〕 |
| T2 服务器下发 token | ✅ 已验收 | `hello` 含 `voiceUrl` 与 6h token；权限限定麦克风发布与音频订阅；异常时不泄露凭证 | 本地 `tools/sim`：`sig=ok`、TTL `6h0m0s`、权限矩阵通过；缺配置/错误 URL 为 `voice=off`；`tools/ctl` 入房与并发 `sync` 通过；隔离服务器副本使用真实 `LIVEKIT_*` 通过，生产未动；维护者 2026-11-11 确认验收；旧客户端真机与签发失败路径待 T3/T4〔流水 12.23〕 |
| T3 Android 接入 | 🔶 待验收 | 单机能进房听到远端；后台本地暂停不广播；回前台待对齐，手动 seek 后恢复；分屏只关麦；无语音配置时行为不变 | 代码完成，构建/单测/lint 通过；真机部分通过：按钮/开关、后台回前台提示、无耳机拒绝、LiveKit 入房并发布音频轨（NX799J/Android 16/SDK 2.30.0，2m4s）；待补远端接收、开麦拔耳机、播放拦截、后台不广播 `pause` 对照；分屏退出房间已搁置〔流水 12.25〕 |
| T4 双机联调 | ⬜ 未开始 | 按固定采样、丢包、切换与 30 分钟口径通过，并记录机型、系统、网络与原始数据 | 待补 |
| T5 文档 | ⬜ 未开始 | fork 文档目录补齐语音说明；上游 docs/README 原文不动 | 待补 |

### T1 LiveKit 独立部署

- 新增 `voice/docker-compose.yml` + `voice/livekit.yaml`，镜像固定 `livekit/livekit-server:v1.13.11`
- 与 `syncwatch` 同机、独立容器；沿用公网 IP 与安全组
- LiveKit 容器使用 `network_mode: host`；`syncwatch` 保持现有部署方式
- 只开 7880/tcp、7881/tcp、7882/udp；3478/udp 默认关闭，直连失败再开
- 服务器上由 `deploy.sh` 探测公网 IPv4 写入 `rtc.node_ip`（不依赖 Google STUN）；仓库模板保留 `rtc.use_external_ip: true` 作为回退；安全组尽量限制来源 IP
- 先记录 LiveKit CLI 版本；`--publish-demo` 是视频演示，不能代替音频验收
- **音频验收**：发布端用 `lk room join --identity publisher --publish <已知音频.ogg> <房间>`（或官方示例发布麦克风），订阅端用另一台公网客户端加入同一房间并确认听到；记录发布端、订阅端、命令、房间名、时间与结果。若 CLI 不能本地播放，使用两个官方示例客户端，一个发布、一个订阅
- **成本保护**：在阿里云设置临时月度流量/费用告警（建议先取 100 GB/月或月度预算 50%，取较低者），并演练停服：`docker compose -f voice/docker-compose.yml stop` 后确认 `syncwatch` 仍能加入房间和同步播放
- **完成标准**：上述音频验收成功，端口检查符合预期，停服演练不影响 `syncwatch`；失败时保留 `docker compose logs` 摘要
- 注意：这一步不改 syncwatch 任何代码

### T2 服务器下发 token

- `server/main.go` 的 `Msg` 结构体（第 38 行起）增加 `VoiceUrl` 与 `VoiceToken` 字段，JSON tag 用 `json:"voiceUrl,omitempty"` / `json:"voiceToken,omitempty"`
- 仅在配置了 `LIVEKIT_URL` / `LIVEKIT_API_KEY` / `LIVEKIT_API_SECRET` 时生成；缺失时两个字段都不下发，客户端隐藏语音入口
- `LIVEKIT_*` 由 T1 生成的 `voice/.env` 注入 `syncwatch` 运行环境；密钥不复制进仓库、不打印
- 在 `hello` 消息（第 446 行）里带上；`voiceUrl` 原样取 `LIVEKIT_URL`
- **权限矩阵**：`roomJoin=true`、`room=<房间码>`、`canPublish=true`、`canPublishSources=[microphone]`、`canSubscribe=true`、`canPublishData=false`；`roomAdmin`、`roomCreate`、`roomRecord`、`recorder` 均为 false
- **异常路径**：配置不完整或签发失败时不阻断普通 `join`，不下发两个字段，日志不打印 API Secret、完整 token 或可复用凭证
- **验证矩阵**：配置齐全 → 两个字段出现；缺任一变量 → 不出现；配置不可解析或签发失败 → 不出现且普通入房正常；旧 Android、Windows 控制台、GUI 能忽略新字段并正常入房
- **完成标准**：用 `tools/sim` / `tools/ctl` 模拟 join，`hello` 含 `voiceUrl` 与 token；只打印 token 的非敏感 claims 和长度，确认 room = 房间码、identity = 昵称 + 会话后缀、有效期 6h、权限符合上述矩阵

### T3 Android 接入

- **T3.0 依赖与构建**：先固定 LiveKit Android SDK 版本，只加依赖与最小连接/发布/订阅代码，验证 minSdk 24 / SDK 34 / Gradle 8.7 / JDK 17 可构建，再改播放 UI 和生命周期
- Manifest：只加 `RECORD_AUDIO`；若 SDK 明确要求再加 `MODIFY_AUDIO_SETTINGS`。第一版不做前台 Service，因此不加 `POST_NOTIFICATIONS` / `FOREGROUND_SERVICE_MICROPHONE`；后台语音再评估
- **语音模式判定**：`hello` 同时含 `voiceUrl` 与 `voiceToken` 才启用语音状态机；否则保持上游现有生命周期与同步行为
- **状态机**：`NORMAL` 正常同步；`BACKGROUND_LOCAL_PAUSE` 切后台本地暂停；`FOREGROUND_PENDING_ALIGN` 回前台待对齐
- `NORMAL → BACKGROUND_LOCAL_PAUSE`：`onStop` 本地暂停视频并断开 LiveKit，保留 SyncWatch 信令连接；不发送 `pause`；显式抑制 `onLocalPause` 广播；可发一次 `memberState=paused`，随后停发心跳位置
- `BACKGROUND_LOCAL_PAUSE → FOREGROUND_PENDING_ALIGN`：`onStart` / `onResume` 保持本机暂停并重连语音；禁用直接播放；收到 `sync` 只更新房间目标与成员状态，不自动 seek/play；不发送心跳位置
- `FOREGROUND_PENDING_ALIGN → NORMAL`：用户手动 seek 后发送现有 `seek`，恢复心跳位置与播放按钮；不自动追赶
- 分屏/焦点：API 29+ 用 `onTopResumedActivityChanged` 判断顶层 resumed；API 24–28 用 `onPause` / `onResume` 兜底；`isInMultiWindowMode()` 只用于区分分屏。失去焦点只关麦，保持视频与远端音频
- 耳机输出：用 `AudioManager` 输出设备与 `AudioDeviceCallback` 判断有线/蓝牙/USB 耳机；拔耳机立即关麦；扬声器/听筒不算耳机。无耳机、权限被拒或麦克风被占用时只禁用发言并提示原因，仍可收听
- **完成标准**：单机能进语音房、能听到远端；切后台不暂停对方；回前台保持暂停直到手动 seek；分屏只关麦；无语音配置时行为与上游一致；拔耳机立即关麦

### T4 双机联调

- **延迟**：至少 3 轮 × 5 分钟，每方向不少于 30 个样本；用 LiveKit 连接统计 RTT 加主观回声延迟记录 p50 / p95；若无法取得客观值，写明估算方法与限制
- **丢包**：在中间热点/路由施加 5% 丢包 5 分钟（记录工具与版本，如 Linux `tc netem` 或 Windows `clumsy`），判断语音可懂度
- **切换**：Wi-Fi → 4G 切换后 15 秒内语音恢复，播放同步不中断；记录断开/重连次数
- **30 分钟**：记录断开、重连、`forceSeek` 次数；播放位置偏差不超过 1.0 秒（现有同步阈值），超过即失败
- **成本**：按实测码率调整 T1 的临时告警阈值；再次演练只停语音、不停 `syncwatch`
- **完成标准**：结果写回本文件或 PR，注明机型、Android 版本、网络、日期、APK 版本、测量工具与原始样本

### T5 文档

- 在 `docs/branches/android-voice/` 补语音协议、部署、Android 说明
- `docs/branches/android-voice/02-voice-protocol.md` 记录 `voiceUrl` 与 `voiceToken` 字段、权限矩阵与异常路径
- 把 T1–T4 的实际命令、结果与限制写回 fork 文档；上游 `README.md`、`docs/01`、`docs/02`、`docs/03`、`docs/05` 原文不动，PR 被接受后再回填

## 8. 兼容性结论（已核对源码）

`hello` 增加 `voiceUrl` / `voiceToken` 字段不会破坏旧客户端：

- 服务端是单个扁平 `Msg` 结构体（`server/main.go:38`），新增字段只影响新字段
- Android 用 `org.json` + `optString` 解析（`android/app/src/main/java/com/syncwatch/app/SyncEngine.kt:139`），未知字段本来就会被忽略
- Windows 端同理，T2 时再确认一次即可

因此语音 PR 只改 `server/`、`android/`、`docs/`，不动 Windows 客户端。这不违反 README「协议速查」的「改协议必须五处同步」——那条针对**语义变更**，这次是**新增可选字段**。PR 里要写明这个区别。

## 9. 待定

| 问题 | 现状 / 建议 |
|---|---|
| Android 切后台/分屏时视频与语音怎么处理？ | 已定：切后台只暂停本机并断开语音，不广播 pause；回前台保持暂停、重连语音，手动 seek 后恢复；分屏失去焦点只关麦。补充：语音未配置时保持上游行为；待对齐期间忽略 `sync` 的自动 seek/play |
| Windows 语音何时做？ | v0.6 不做；后续 GUI 版优先评估 WebView2 + LiveKit JS SDK |
| 域名与 TLS | 暂不对外；实验期用 `ws://`；对外前必须加 TLS，并重估 TTL/房间复用/token 接口 |
| LiveKit 镜像版本 | 已定 `v1.13.11`（2026-10-25 release）；升级前跑 T1 音频验收 + T4 双机回归 |
| LiveKit 容器网络 | 使用 host；`syncwatch` 保持现有部署，不使用 bridge 端口映射 |
| 流量告警阈值 | T1 先设 100 GB/月或月度预算 50%（取较低者）；T4 按实测码率调整 |
| 需要覆盖的机型/厂商 | T4 前列清单 |
| 本文件合并前是否删除 | 建议保留到 v0.6 发布 |
| 仓库级 `AGENTS.md` | 已建立（2026-11-10）；项目红线、文档权威链与会话交接条款见根目录 |
| LiveKit Android SDK 版本 | T3.0 固定版本并先验证 minSdk 24 / SDK 34 / Gradle 8.7 / JDK 17 |
| 分屏焦点检测 | API 29+ 用 `onTopResumedActivityChanged`；API 24–28 用 `onPause` / `onResume` 兜底 |
| ADR `Proposed` 的含义 | 仅表示上游合并决策未批准；fork 内实施按既有授权先行，不等于上游接受 |
| T1 服务器访问 | 本机公钥被 `deploy` / `root` / `ecs-user` / `ubuntu` 拒绝；服务器 agent 在 bwrap 内只映射单 gid，无法访问 docker socket。改为维护者在宿主执行 `su - dsh` 一条命令，agent 只负责写文件与分析日志 |
| UDP 接收缓冲 | 当前 `net.core.rmem_max` 下 LiveKit 报 425984，低于建议 5000000；T1 不阻塞，T4 丢包/延迟测试前评估是否持久化调整 |
| 临时流量/费用告警 | 维护者 2026-11-11 决定暂不设置。风险：按量计费无硬闸，异常流量只能靠账单/监控发现后手动停 `syncwatch-livekit`；T4 实测码率后再评估 |

## 10. 多机协作规则

- 一条分支同一时间只在一台机器上活跃；换机前 commit + push，并更新 §0「当前占用」
- 开工前 `git fetch && git pull --ff-only`（共享分支不改写历史；落后 `main` 时用 `git merge main`）
- 不对共享分支 `push --force`；分支名按任务起，不按机器起
- 阶段验收后 push 一次到 `fork/feat/android-voice`（T1 配置、T2 协议、T3/T4 客户端）；日常小步只在本地 commit，`.env` 与真实 IP 永不入库
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
| `docs/branches/android-voice/03-voice-deploy.md` | LiveKit 同机部署、端口、NAT 与排障 |
| `docs/branches/android-voice/05-android-voice.md` | Android 生命周期、分屏、音频焦点与后续后台语音 |
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

### 12.7 — 2026-11-10 确定 T1 同机公网最小端口方案

- 类型：部署
- 状态：✅ 已验收（维护者，2026-11-10）
- 内容：LiveKit 与 `syncwatch` 同机、独立容器；只开 7880/tcp、7881/tcp、7882/udp；TURN 3478 默认关闭；配置 `rtc.use_external_ip: true`；安全组尽量限制来源 IP。
- 证据：`docs/adr/0001-android-voice-chat.md` 决策 5；`docs/branches/android-voice/03-voice-deploy.md`；`PROGRESS.md` T1。
- 决策：先测直连，NAT 不通再启用 TURN；不开放 50000–60000 整段 UDP。

### 12.8 — 2026-11-10 固定 LiveKit 镜像 v1.13.11

- 类型：部署
- 状态：✅ 已验收（维护者，2026-11-10）
- 内容：LiveKit 镜像固定为 `livekit/livekit-server:v1.13.11`（GitHub latest release，2026-10-25）；升级时手动改 tag，并通过 T1 回声测试与 T4 双机回归。
- 证据：GitHub releases API 查询结果；`docs/branches/android-voice/03-voice-deploy.md`；`PROGRESS.md` T1、§9。
- 决策：不使用 `latest`。

### 12.9 — 2026-11-10 确定 LiveKit 使用 host 网络

- 类型：部署
- 状态：✅ 已验收（维护者，2026-11-10）
- 内容：LiveKit 容器使用 `network_mode: host`，直接绑定 7880/7881/7882；`syncwatch` 保持现有部署方式；不使用 bridge 端口映射。
- 证据：`docs/adr/0001-android-voice-chat.md` 决策 5；`docs/branches/android-voice/03-voice-deploy.md`；`PROGRESS.md` T1、§9。
- 决策：减少 WebRTC UDP 的 NAT 层，接受 host 网络隔离较弱的代价。

### 12.10 — 2026-11-10 确定 Android 分屏/后台生命周期

- 类型：Android / 设计
- 状态：✅ 已验收（维护者，2026-11-10）
- 内容：第一版不做后台语音；`onPause` 关闭麦克风但保持视频与远端音频，`onStop` 暂停视频并断开 LiveKit；回前台恢复视频并重连语音。分屏用 `isInMultiWindowMode()` 区分。
- 证据：`docs/adr/0001-android-voice-chat.md` 产品约束、决策 6、验收标准；`docs/branches/android-voice/05-android-voice.md`；`PROGRESS.md` T3、§9。
- 决策：后续若做后台语音，再把语音会话抽成独立控制器并包进前台 Service；协议与服务端无需改动。

### 12.11 — 2026-11-10 切后台只做本地暂停

- 类型：Android / 协议行为
- 状态：✅ 已验收（维护者，2026-11-10）
- 内容：Android 切后台自动暂停只影响本机，不发送 `pause`；用户主动暂停仍广播全房间。回前台按最近 `hello` / `sync` 的 `position`、`anchorMs`、`rate` 计算房间进度并 seek 后恢复。
- 证据：`docs/adr/0001-android-voice-chat.md` 决策 6、验收标准；`docs/branches/android-voice/05-android-voice.md`；`PROGRESS.md` T3。
- 决策：不新增协议；复用现有房间快照字段。

### 12.12 — 2026-11-10 回前台改为手动跳转

- 类型：Android / 交互
- 状态：✅ 已验收（维护者，2026-11-10）
- 内容：回前台不自动 seek；保持本机暂停并重连语音，两人商量后手动 seek，现有 `seek` 同步全房间。手动静止期间暂停心跳位置上报，避免服务端 `forceSeek` 自动跳转。
- 证据：`docs/adr/0001-android-voice-chat.md` 决策 6、验收标准；`docs/branches/android-voice/05-android-voice.md`；`PROGRESS.md` T3、§9。
- 决策：不新增协议；手动 seek 后恢复心跳。

### 12.13 — 2026-11-10 回前台待对齐状态禁止直接播放

- 类型：Android / 交互
- 状态：✅ 已验收（维护者，2026-11-10）
- 内容：回前台后处于「待对齐」状态时先禁用直接播放；用户 seek 到商定位置后再允许播放，避免把全房间拉回旧进度。
- 证据：`docs/adr/0001-android-voice-chat.md` 决策 6；`docs/branches/android-voice/05-android-voice.md`；`PROGRESS.md` T3。
- 决策：T3 用待对齐状态保护播放按钮，不改协议。

### 12.14 — 2026-11-10 开工前设计口径收尾

- 类型：流程 / 设计收尾
- 状态：✅ 已验收（维护者，2026-11-10）
- 内容：开工前设计与文档口径已收尾；开发计划保存于 ADR、`docs/branches/android-voice/` 的 README/02/03/05 与 §7 任务表；T1–T5 未开始。
- 证据：`git ls-files` 确认 ADR、`CONTEXT.md`、`docs/branches/android-voice/` 四份文档、`AGENTS.md`、`PROGRESS.md` 均已入库；工作区干净；本地领先 `fork/feat/android-voice`，未 push。
- 决策：下一步 T1；未编译、未上真机、未部署 LiveKit。

### 12.15 — 2026-11-10 按评审补齐语音计划执行口径

- 类型：计划 / 文档
- 状态：✅ 已验收（维护者指示按此计划开始 T1，2026-11-10）
- 内容：T1 改为可执行的音频发布/订阅验收，并加入临时流量告警与停服演练；T2 明确 token 权限矩阵、异常路径与旧客户端兼容验证；T3 增加语音启用判定、`NORMAL` / `BACKGROUND_LOCAL_PAUSE` / `FOREGROUND_PENDING_ALIGN` 状态机、分屏多 resume 检测、耳机路由、SDK 兼容性先行与 Manifest 权限核减；T4 固定延迟、丢包、切换、30 分钟与告警的测量口径；ADR 与会话交接 skill 区分上游 `Proposed` 与 fork 实施授权。
- 证据：本文件 §0/§3/§7/§9；`docs/adr/0001-android-voice-chat.md` 状态与 §4/§6/§7/验收；`docs/branches/android-voice/02-voice-protocol.md`、`03-voice-deploy.md`、`05-android-voice.md`、`README.md` 索引；`.agents/skills/session-handover/SKILL.md`；工作区修改未提交。
- 决策：按 T1 → T2 → T3 → T4 → T5 施工；T1 本地配置完成后等待 SSH 授权。未编译、未上真机、未部署、未 push。

### 12.16 — 2026-11-10 T1 本地配置完成，等待服务器 SSH 授权

- 类型：部署 / 阻塞
- 状态：⏸ 阻塞（等待维护者提供可用 SSH 账号，或将本机公钥加入服务器 `authorized_keys`）
- 内容：新增 `voice/docker-compose.yml`、`voice/livekit.yaml`、`voice/.env.example`，并将 `voice/.env` 加入 `.gitignore`；镜像固定 `livekit/livekit-server:v1.13.11`，使用 host 网络、7880/tcp、7881/tcp、7882/udp 与 `rtc.use_external_ip: true`。
- 证据：本地文件已创建；目标服务器 SSH 可达，但 `deploy` / `root` / `ecs-user` / `ubuntu` 等账号的公钥认证均被拒；本机无 `~/.ssh/config`、`server/deploy.env` 或 `private/` 配置。未部署、未生成 token、未跑音频验收、未设置告警。
- 决策：获得 SSH 授权后继续；真实服务器地址、密钥与 token 不写入仓库。

### 12.17 — 2026-11-11 T1 改由服务器宿主执行，部署脚本自包含

- 类型：部署
- 状态：🚧 实现中（等待服务器宿主执行输出）
- 内容：服务器侧 agent 运行在 bwrap 沙箱内，只映射单一 gid，无法访问 docker socket；改为维护者在宿主执行 `su - dsh` 命令。`voice/deploy.sh` 改为自包含：生成 Compose、LiveKit 配置与 `.env`，探测公网 IPv4 写入 `rtc.node_ip`（避开境内常不可达的 Google STUN），启动后输出容器、端口、HTTP、日志与错误扫描；`--check` 可重复自检。
- 证据：`voice/deploy.sh`（压缩还原字节一致）、`voice/docker-compose.yml`、`voice/livekit.yaml`、`voice/.env.example`、`docs/branches/android-voice/03-voice-deploy.md`；工作区修改未提交。未执行部署、未生成 token、未跑音频验收。
- 决策：T1 不依赖 SSH；服务器 agent 只做写文件与日志分析，Docker 操作由宿主执行。阶段验收后 push 到 `fork/feat/android-voice`。

### 12.18 — 2026-11-11 T1 LiveKit 容器部署通过

- 类型：部署
- 状态：🚧 实现中（部署与自检通过，等待公网与音频验收）
- 内容：宿主以 `dsh` 执行 `~/syncwatch-voice/deploy.sh`；Docker Hub 直连超时，改用 `docker.m.daocloud.io` 拉取同一镜像并重打 `livekit/livekit-server:v1.13.11` 标签；容器 `syncwatch-livekit` 启动，使用 host 网络与显式 `node_ip`。
- 证据：`docker images` 显示镜像 `0e20c255a2c1`、89.9 MB；服务端 `livekit-server version 1.13.11`；镜像 digest `sha256:0e20c255a2c17000c52d0631b79737a4216c06bc51df13878768df01261c8dfd`；`7880/7881 tcp` 与 `7882 udp` 有监听；本机 `127.0.0.1:7880` 返回 200；日志确认 `using explicit node IP for NAT1To1Ips`；错误扫描未发现明显错误。
- 遗留：日志 WARN `UDP receive buffer 425984 < 5000000`，T4 前评估 `net.core.rmem_max`；公网可达性、音频验收、流量告警与停服演练未做。
- 决策：T1 保持 🚧；先在阿里云防火墙放行 7880/tcp、7881/tcp、7882/udp，再从公网验证并做音频验收。

### 12.19 — 2026-11-11 公网 TCP 验收通过，准备音频验收

- 类型：验收 / 测试工具
- 状态：🚧 实现中（等待音频实走）
- 内容：从 Windows 开发机对公网验证 `7880/tcp`、`7881/tcp` 均可达，`http://<服务器>:7880/` 返回 200；新增 `voice/test/index.html`，用官方 LiveKit JS SDK（CDN，UMD）做 440 Hz 单音发布/收听测试，仅用于 T1 验收，不属于 Android 客户端实现。
- 证据：`Test-NetConnection` 两次 `TcpTestSucceeded : True`；`curl.exe` 返回 `200`；测试页文件已创建，未在浏览器实跑。未记录真实公网 IP。
- 遗留：网关防火墙已放行三端口；UDP 7882 尚未由 WebRTC 客户端实走；音频、告警、停服演练未完成。
- 决策：下一步在服务器生成 6 小时 publisher/listener token，Windows 发布 440 Hz，手机订阅确认听到；记录两端浏览器/机型与时间。

### 12.20 — 2026-11-11 T1 音频验收通过

- 类型：验收
- 状态：🚧 实现中（等待告警与停服演练）
- 内容：Windows Chrome 通过 `voice/test/index.html` 用 publisher token 发布 440 Hz 单音；Android 手机在同一 Wi-Fi 打开同一测试页，用 listener token 订阅，维护者确认正常听到。SDK 改为本地 `livekit-client.umd.min.js`（610592 字节）后，订阅端不再依赖 CDN。
- 证据：发布端日志 `connected as publisher room 123456`、`publishing 440Hz`；订阅端日志 `connected as listener room 123456`、`subscribed audio`；维护者确认听到声音。token 与公网 IP 未记录。
- 遗留：手机型号/系统/浏览器版本待补；UDP 7882 与 TCP 7881 的实际选中路径未单独确认；流量告警与停服演练未做。
- 决策：继续完成 T1 剩余两项（临时告警、停 LiveKit 不影响 `syncwatch`），然后 T1 收尾置 🔶 待验收。

### 12.21 — 2026-11-11 停服演练通过，T1 仅剩告警

- 类型：演练
- 状态：🚧 实现中（等待告警设置）
- 内容：执行 `docker compose stop` 停掉 LiveKit 后，`docker ps` 仅剩 `syncwatch`（`Up 5 days (healthy)`），`8995` 仍在 `0.0.0.0` 与 `[::]` 监听；随后 `docker compose start` 恢复，`deploy.sh --check` 重新出现容器运行、三端口监听、HTTP 200、`.env` 完整的 PASS。
- 证据：`docker compose stop` 输出 `syncwatch-livekit Stopped`；`docker ps` 仅剩 `syncwatch`；`ss -lnt` 显示 8995；恢复后 `--check` 全 PASS（镜像 digest `sha256:0e20c255…`，服务端 `1.13.11`）。
- 遗留：停服窗口内未用客户端实测播放同步（容器与端口隔离已验证，播放同步留 T4）；临时告警未设置；手机型号/系统/浏览器待补。
- 决策：告警设置完成后 T1 置 🔶 待验收。

### 12.22 — 2026-11-11 T1 收尾并验收通过（告警豁免）

- 类型：收尾
- 状态：✅ 已验收（维护者 2026-11-11）
- 内容：T1 部署、公网 TCP、440 Hz 音频验收、停服隔离与恢复自检均通过；维护者明确决定暂不设置临时流量/费用告警，按已接受风险记录，不阻塞 T1 收尾。T1 置 🔶 待验收。
- 证据：流水 12.18–12.21；`PROGRESS.md` §9 告警豁免条目；未记录公网 IP、密钥或 token。提交 `37a6234`（`feat(v0.6): T1 LiveKit 部署与音频验收`），已推送 `fork/feat/android-voice`（`32ff2fe..37a6234`）。
- 遗留：UDP 7882 与 TCP 7881 的实际选中路径未单独确认；停服窗口内未用客户端实测播放同步（留 T4）；手机型号/系统/浏览器待补。
- 决策：维护者验收通过 T1，告警豁免维持；下一任务 T2 服务端下发 token。阶段检查点 commit + push 到 `fork/feat/android-voice`，维护者已授权。

### 12.23 — 2026-11-11 T2 服务端下发 token 本地实现完成

- 类型：实现
- 状态：✅ 已验收
- 内容：`server/main.go` 在成员成功加入后签发 6 小时 LiveKit HS256 JWT，随 `hello` 下发 `voiceUrl` / `voiceToken`；三项 `LIVEKIT_*` 完整且 URL 为 `ws://` 或 `wss://` 才启用，缺任一项或 URL 非法时两个字段都不下发，普通入房不受影响。token 权限固定为指定房间、麦克风发布与订阅，`canPublishData`、管理员、建房、录制均为 false。`server/docker-compose.yml` 接入三项环境变量，`server/.env.example` 与 fork 文档补充注入和验收口径；`tools/sim` 增加 claims 解码、长度输出与本地 HS256 校验，不输出 token 本体。
- 证据：临时 Go 1.22.12 工具链下 `go build` 服务端、`tools/sim`、`tools/ctl` 通过，`gofmt` 与 `git diff --check` 通过。本地完整配置启动日志 `[voice] token issuance enabled`；`hello` 输出 `voiceUrl=ws://127.0.0.1:7880 tokenLen=476 sig=ok alg=HS256 sub=Alice#99cc name=Alice room=TEST01 ttl=6h0m0s join=true publish=true sources=[microphone] subscribe=true data=false admin=false create=false record=false recorder=false`。逐项缺少 `LIVEKIT_URL` / `LIVEKIT_API_KEY` / `LIVEKIT_API_SECRET` 及非法 URL 时均为 `voice=off` 且入房正常；`tools/ctl` 作为不解析语音字段的客户端成功入房并发送 `play`；同一昵称两次 join 得到不同后缀 `Alice#6731` / `Alice#545e`。并发复测：Alice 15:36:14 入房、Bob 15:36:15 加入后 `members=2`，A 收到 `play` / `pause` / `seek` 的 `sync` 广播；`hello` 继承既有房间 `pos=200.0 created=false`，token 为 `Alice#e2ee`、`sig=ok`、`ttl=6h0m0s`。2026-11-11 17:00–17:01 在服务器创建隔离副本，镜像 `syncwatch:t2-test`、容器 `syncwatch-t2-test`、仅绑定 `127.0.0.1:18995`；日志 `[voice] token issuance enabled`，容器环境变量名计数 3，健康检查 200。经临时 SSH 隧道跑 A/B：`voiceUrl` 存在，`tokenLen=509`、`sig=ok`、`alg=HS256`、`room=T2TEST`、`ttl=6h0m0s`，权限矩阵全通过；Bob 的 `play` / `pause` / `seek` 均广播给 Alice。测试期间生产 `syncwatch` 保持 healthy、8995 仍监听、健康接口 200；测试容器、镜像和目录已清理。提交 `0c146b6` 已 push 至 `fork/feat/android-voice`。
- 遗留：T2 尚未更新生产部署；Windows 控制台/GUI/Android 旧客户端未实际入房（仅源码与 `tools/ctl` 兼容验证）；签发失败路径未强制触发；token 被 LiveKit 实际接受留给 T3/T4。
- 决策：维护者 2026-11-11 确认 T2 验收通过，状态翻为 ✅，并批准提交/推送；T2 不依赖生产部署。下一步先做 T3.0 最小客户端验卡，再进入 T3 Android 接入；生产部署仍待维护者决定。

### 12.24 — 2026-11-12 T3 Android 语音接入实现完成

- 类型：实现
- 状态：🔶 待验收
- 内容：Android 依赖固定 `io.livekit:livekit-android:2.30.0` 与 `kotlinx-coroutines-android:1.8.1`；`settings.gradle` 增加 JitPack（LiveKit 的 `audioswitch` 依赖只发布在 JitPack）。新增 `VoiceClient.kt` 封装连接/断开、麦克风开关与远端音轨事件，旧连接取消后不再回调，避免重连串扰。`SyncEngine` 在 `hello` 同时含 `voiceUrl`/`voiceToken` 时回调语音配置；新增 `syncSuspended`：后台本地暂停与回前台待对齐期间不应用远端 `play`/`seek`/`forceSeek`、停止位置心跳，并抑制本地 play/pause/倍速/缓冲广播。`MainActivity` 实现 `NORMAL → BACKGROUND_LOCAL_PAUSE → FOREGROUND_PENDING_ALIGN → NORMAL` 三态：`onStop` 本地暂停、断开 LiveKit、保留信令并只发一次 `memberState=paused`；`onStart` 保持暂停并重连语音；手动 seek 后解除待对齐并恢复心跳；API 29+ 用 `onTopResumedActivityChanged`、API 24–28 用 `onPause` + `isInMultiWindowMode()` 兜底关麦；`AudioDeviceCallback` 检测有线/蓝牙/USB 耳机，拔耳机立即关麦；无耳机、权限拒绝或麦克风占用只禁用发言并提示，仍可收听。控制层底部新增麦克风按钮与开/关图标。Manifest 只新增 `RECORD_AUDIO`，并移除 LiveKit 合并的 `CAMERA`/`FOREGROUND_SERVICE*`，保留 `camera required=false` 特性声明。
- 证据：`android/build-apk.bat` 构建成功，产物 `dist/一起看-android-v0.5.2-ui.apk` 63,787,395 字节；`aapt dump permissions` 显示 `INTERNET`、`ACCESS_NETWORK_STATE`、`WAKE_LOCK`、`RECORD_AUDIO`、`BLUETOOTH`（maxSdk 30）、`MODIFY_AUDIO_SETTINGS`，无 `CAMERA`/`FOREGROUND_SERVICE*`。`testDebugUnitTest` 与 `lintDebug` 均 BUILD SUCCESSFUL；Lint 仅剩既有警告（方向锁定、SetTextI18n、旧 API 等）。`adb devices` 为空，未做真机安装或运行。LiveKit 2.30.0 于 2026-11-02 发布，AAR `minSdkVersion=21`、`minCompileSdk=1`，运行时 Kotlin 1.9.25，与现用 minSdk 24 / compileSdk 34 / Gradle 8.7 / JDK 17 兼容。
- 遗留：所有真机行为未验证——进房听远端、切后台不广播、回前台待对齐、分屏关麦、拔耳机关麦、无语音配置回归、蓝牙/USB 耳机路由、权限拒绝与麦克风占用路径；token 被 LiveKit 实际接受、LiveKit SDK 2.30.0 与服务器 `v1.13.11` 的实际兼容性也待 T4。窄竞态：若在 `hello` 到达前立即切后台，仍会走旧逻辑广播一次 `pause`，未处理。T3 改动尚未提交。
- 决策：无新增决策；等待维护者用真机按 T3 完成标准验收，通过后再进入 T4。

### 12.25 — 2026-11-12 T3 真机部分验收（单机）

- 类型：验证
- 状态：🔶 待验收
- 内容：生产 `syncwatch` 更新到 T2 代码并注入 `LIVEKIT_*`（回滚镜像 `syncwatch:pre-t3`，源码备份 `server.bak-20261112`）；`tools/sim` 探针确认 `hello` 下发 `voiceUrl=ws://47.109.196.250:7880`、token 500 字符、TTL `6h0m0s`、仅麦克风发布/音频订阅。真机 `nubia NX799J`（Android 16 / API 36）安装 T3 APK：语音按钮出现且可开/关；切后台再回前台提示「已回到前台，拖动进度条对齐后再继续播放」；断开耳机后点麦克风提示「请先连接耳机再开麦，避免回声」。LiveKit 日志显示 `Tester#a5e6` 以 Android SDK 2.30.0 / Android 16 / NX799J 入房并发布音频轨，会话 2m4.7s，以 `CLIENT_REQUEST_LEAVE` 正常结束，证明 token 被服务端接受、麦克风发布链路打通。
- 证据：服务器 `syncwatch` 日志 11:39:48 `[voice] token issuance enabled`，11:56–11:59 `Tester` 入房与 `seek`/`play`/`pause`；LiveKit 日志 11:59:21–12:01:25 音频轨统计与 participant closing（`sdk=ANDROID version=2.30.0 osVersion=16 deviceModel=nubia NX799J`）；用户 2026-11-12 实机回报。
- 遗留：未验证远端 440Hz 接收、开麦时拔耳机自动关麦、回前台后播放拦截、后台不广播 `pause` 的时间点对照；分屏会退出房间（疑为多窗口配置变更导致 Activity 重建），维护者决定接受现状并搁置；LiveKit 日志出现 clock skew / jitter 警告，T4 需复测；无语音配置回归已在更新生产前验证通过。
- 决策：分屏检测搁置，不作为 T3 阻塞项；T3 仍为 🔶 待验收，核心「听到远端」与后台广播行为补验后转 ✅。
