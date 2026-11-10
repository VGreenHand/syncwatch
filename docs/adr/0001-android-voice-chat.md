# 0001. Android 内置语音：独立 LiveKit 媒体面（fork 扩展）

- 状态：提议（Proposed；fork 先行实施，上游合并前需主项目维护者确认）
- 日期：2026-11-06
- 决策者：SyncWatch 维护者、语音功能贡献者
- 相关文档：`README.md`、`docs/01-方案设计.md`、`docs/02-同步协议.md`、`docs/05-Android客户端.md`

> 适用范围：本 ADR 只约束 fork 的语音扩展（`feat/android-voice`），不改变上游 `origin` 的项目定位、README 红线、`docs/01` 范围与版本路线图。上游是否接受本决策，由主项目维护者在合并 PR 时确认；未配置 LiveKit 时，fork 的行为与上游一致。

## 背景

SyncWatch v0.5.2-ui 的架构是「Go 信令服务器 + Windows mpv 客户端 + Android Media3 客户端」。服务器只转发播放、暂停、拖动和心跳等 JSON 信令，不接触音视频数据；项目卖点是单二进制、低带宽、无视频分发。

本 fork 要加入 Android 内置语音，让两人在看片时全双工对话。这个需求和现有架构有一个根本差异：**实时语音是媒体流，不是信令**。它不能复用现有 8 KB 文本帧的 WebSocket 通道；启用语音后，fork 不能再宣称「纯信令、零媒体流量」，但上游未启用语音时不受影响。

本文档的决策基于 v0.5.1（`f0e4a34`）做出，与之后的 UI 重做（`940adf8`，v0.5.2-ui）不冲突：UI 变化只影响语音入口挂在哪里，不影响信令/媒体分离、token 与音频生命周期这些决策。

已确认的产品约束：

- 第一版只做 Android，不做 Windows 内置语音。
- 两人全双工，先不追求 5 人混音。
- 必须戴耳机才能开麦；没有耳机时仍可听对方说话。
- 切后台立即关闭麦克风，但继续收听；回到前台恢复发言。
- 不主动降低影片音量；系统或音频框架的强制行为以真机测试为准。
- 自建阿里云轻量服务器（成都，2 vCPU / 2 GiB，200 Mbps 峰值，无固定流量包）。
- 暂不对外提供服务，仅两人私有使用；日常观看一两集电视剧或一部电影。
- 暂无域名，实验阶段用公网 IP；项目会开源到 GitHub。
- 目标是给项目贡献 v0.6 语音能力，同时学习 Android/RTC 开发。

## 决策

### 1. 语音是 fork 的可选扩展层，不进入核心信令进程

LiveKit 作为独立 Docker Compose 服务运行，与现有 `syncwatch` 容器分开。核心 `syncwatch-server` 继续只做房间和播放同步；不部署 LiveKit 或未配置语音环境变量时，fork 的行为与上游一致，上游项目本身的设计不变。

这样做的目的不是让用户多启动一个软件，而是隔离故障域：语音服务崩溃、升级或占满资源时，播放同步不受影响；不需要语音的部署可以完全不安装 LiveKit。

### 2. Android 使用原生 LiveKit SDK

第一版只改 Android 客户端，使用 LiveKit Android SDK（底层为 WebRTC），不采用 WebView 方案。原因是我们要求切后台继续收听，WebView 的后台音频和麦克风生命周期不可靠；原生 SDK 可以配合前台服务、音频路由和麦克风状态管理。

Windows 端暂不加入语音；未来如果要做，可复用同一套 LiveKit 房间和 token 模型，但客户端代码不能直接复用 Android 实现。

### 3. 语音房间与 SyncWatch 房间绑定

- LiveKit room name 直接使用 SyncWatch 的 6 位房间码。
- LiveKit participant identity 使用「昵称 + 会话后缀」（如 `小明#a3f9`），保证同一房间内唯一；界面仍显示昵称。
- 后缀在每次 `join` 时生成，重连换新；不复用旧连接的身份。
- 只有成功加入 SyncWatch 房间的成员才应获得语音 token。
- 语音 token 的权限限定为：加入指定房间、发布音频、订阅音频；不授予视频、录制或管理员权限。

### 4. MVP 由 `hello` 下发连接地址与短期 token

SyncWatch 服务器在成员加入成功时，随 `hello` 返回 `voiceUrl` 与 `voiceToken`。`voiceUrl` 来自服务器配置的 `LIVEKIT_URL`，告诉客户端连哪台 LiveKit；`voiceToken` 是使用 LiveKit API Secret 签名的 JWT，包含昵称、房间码、有效期和音频收发权限。

- API Secret 只保存在服务器，绝不写入 APK 或仓库。
- 只有 `LIVEKIT_URL`、`LIVEKIT_API_KEY`、`LIVEKIT_API_SECRET` 全部配置时才下发两个字段；缺任一项时都不下发，客户端隐藏语音入口。
- `voiceUrl` 是连接地址，不是秘密；`voiceToken` 是 bearer 凭证，payload 可被解码，不得写入日志或放进 URL。
- token 有效期设为 6 小时，覆盖一两集电视剧或一部电影；过期或重连时重新加入 SyncWatch 获取新 token。本阶段仅两人私有使用，暂不对外服务。
- MVP 阶段使用 `ws://`，token 和房间密码一样可能被链路窃听；在对外提供服务前必须加 TLS，并重新评估 token 有效期、房间码复用风险与独立 `/voice-token` 接口。

### 5. 端口与网络

LiveKit 与 `syncwatch` 部署在同一台服务器，但是独立容器；沿用主机、公网 IP 与安全组，不合并 Compose 文件。实验阶段只开放必要端口，避免开放 `50000-60000` 整段 UDP：

| 协议 | 端口 | 用途 |
|------|------|------|
| TCP | 7880 | LiveKit 信令 |
| TCP | 7881 | WebRTC over TCP 回退 |
| UDP | 7882 | WebRTC 媒体（单端口 UDP mux） |
| UDP | 3478 | TURN 兜底，默认关闭；仅在直连失败时启用 |

云端 NAT 后需要开启 `rtc.use_external_ip: true`，让 LiveKit 对外宣告公网地址。安全组只放行上述端口，尽量限制来源 IP。如果 LiveKit 配置无法使用单端口 UDP，再回退到官方端口范围，并单独记录原因。

### 6. 后台与音频路由

- 前台：麦克风和扬声器都可用，但只有检测到耳机输出时才允许开麦。
- 切后台：立即 `setMicrophoneEnabled(false)`，保持远端音频播放；显示常驻通知说明「语音收听中，麦克风已关闭」。
- 回前台：重新检测耳机和音频路由，再恢复麦克风。
- 麦克风不可用（权限被拒、被其他应用占用、蓝牙路由异常）时只禁用发言，不阻断收听，也不影响播放同步；界面提示具体原因。
- 第一版不主动做 ducking（压低影片音量）；如果 Android 音频焦点或 WebRTC 通信模式强制压低/暂停 ExoPlayer，以双机原型实测结果为准，并记录为已知限制。

### 7. 成本与滥用防护

- 服务器按流量计费，必须配置月度流量/费用告警；超过阈值时暂停语音服务，不影响播放同步。
- 语音 token 短期有效，房间仍需密码；LiveKit 不对公网匿名开放。
- 两人使用不需要大带宽；200 Mbps 峰值不是瓶颈，账单和端口暴露才是风险。

## 非目标

- 不做 Windows 内置语音（第一版）。
- 不做视频传输、屏幕共享或云端录像。
- 不做客户端之间的 P2P 音频直连；媒体统一经 LiveKit SFU 中转。
- 不做 5 人混音、语音消息、弹幕或语音转文字。
- 不在没有 TLS 的情况下把语音服务作为公开长期服务。

## 备选方案

| 方案 | 结论 | 原因 |
|------|------|------|
| Discord / QQ 语音 | 不采用 | 两人使用最便宜、最可靠，但目标是给项目加 v0.6、实现一个 App / 一个房间码的集成体验 |
| WebView + 浏览器 WebRTC | 不采用为最终方案 | 可快速验证，但后台音频/麦克风生命周期不可靠 |
| Kraken（VideoTogether 同款 SFU） | 仅参考 | 纯 Go、Apache-2.0、音频 SFU，但没有现成 Android SDK，客户端和信令要自己写 |
| 托管 RTC（声网/TRTC 等） | 暂缓 | 运维少，但有服务商依赖、账号与计费；现有服务器可先自建验证 |
| P2P WebRTC mesh | 不采用 | NAT/TURN 复杂，且与项目「不做客户端互传」的边界冲突 |

## 后果

### 正面

- 核心信令服务保持轻量，语音故障不会拖垮播放同步。
- LiveKit 负责 Opus 编解码、抖动缓冲、丢包隐藏、回声消除、重连和 TURN，避免从零写音频栈。
- Android SDK 降低原生 RTC 接入成本；未来 5 人扩展有现成路径。
- 语音服务可选部署，开源用户不强制安装。

### 负面

- 启用语音的 fork 从「单二进制、纯信令」变成「核心 + 可选媒体面」，部署和文档复杂度上升；上游项目不受影响。
- 服务器多一个容器、一组端口和流量账单；需要监控和滥用防护。
- 核心服务器需要持有 LiveKit API Secret，并与 LiveKit 版本/协议保持兼容。
- Android 前台服务、音频焦点、耳机检测和厂商后台策略会带来真机适配工作。
- 无域名/TLS 阶段只能作为实验环境，不能承诺公网安全。

### 风险与缓解

| 风险 | 缓解 |
|------|------|
| Android 厂商杀后台 | 前台服务 + 常驻通知；明确「尽力保持」，不承诺所有机型 |
| 音频焦点导致影片被压低/暂停 | 双机原型先测；必要时调整 ExoPlayer 音频焦点处理，并记录限制 |
| 蓝牙耳机路由/麦克风不可用 | 以实际输出路由为准；路由不满足时禁用开麦并提示 |
| token 在明文链路被窃听 | 仅两人私有使用，暂不对外；对外前加 TLS，并重新评估 TTL、房间复用与 `/voice-token` |
| 端口暴露被滥用 | 房间密码、短期 token、限流、流量告警、必要时暂停语音 |
| LiveKit 升级破坏兼容 | 固定镜像版本；升级前在测试环境跑两人回归 |

## 验收标准（第一版）

- 两台 Android 真机（Android 7.0+）加入同一 SyncWatch 房间，戴耳机可全双工对话。
- 目标延迟：局域网/国内宽带 p50 ≤ 300 ms、p95 ≤ 600 ms；以实测记录为准。
- 切后台后麦克风关闭、对方语音继续播放；回前台恢复发言。
- 无耳机时不能开麦，但仍能听到对方。
- 拒绝麦克风权限或麦克风被其他应用占用时，仍能听到对方，播放同步正常。
- 5% 丢包下语音仍可懂；Wi-Fi 切换 4G 后能重连。
- 连续看片 30 分钟，语音不中断、不导致播放同步明显漂移。
- 流量告警可触发，语音服务可独立停止而不影响 `syncwatch`。

## 后续工作

1. 安装 Android Studio / SDK 34，确认 LiveKit Android SDK 版本与 minSdk 24 兼容。
2. 在 `server/main.go` 增加可选的 LiveKit token 生成，随 `hello` 下发；配置缺失时语音关闭。
3. 增加 Android 权限：`RECORD_AUDIO`、`POST_NOTIFICATIONS`、前台服务类型；实现耳机检测与后台静音。
4. 增加 `voice/` Docker Compose、LiveKit 配置、端口说明、流量告警和部署文档。
5. 用两台真机做双机原型，先验证音频焦点、回声、后台和耳机路由，再定 UI。
6. 补 `docs/branches/android-voice/` 的协议补充、部署与 Android 说明；上游 `docs/02`、`docs/05`、`README.md` 原文不动，PR 被接受后再回填。

## 待定问题

- 正式对外时是否购买域名并启用 TLS；token 是否迁移到 `/voice-token`。
- LiveKit 镜像已固定为 `v1.13.11`（2026-10-25 release）；升级前跑两人回归。
- 阿里云按量流量的实际单价和月度告警阈值。
- 需要覆盖的 Android 机型/厂商列表。
- 未来 5 人时是否仍用 SFU 转发，还是引入混音。

## 参考

- LiveKit 端口与防火墙：<https://docs.livekit.io/transport/self-hosting/ports-firewall/>
- LiveKit 自建虚拟机部署：<https://docs.livekit.io/transport/self-hosting/vm/>
- VideoTogether 浏览器语音实现：<https://github.com/VideoTogether/VideoTogether/blob/main/source/extension/vt.js>
- Kraken 音频 SFU：<https://github.com/MixinNetwork/kraken>
