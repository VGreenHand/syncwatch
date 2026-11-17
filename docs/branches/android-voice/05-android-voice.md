# Android 语音接入（fork）

> 目标行为，T3 实现后以真机结果为准。

## 生命周期

| 状态 | 触发 | 视频 | 麦克风 | 语音连接 | 同步行为 |
|---|---|---|---|---|---|
| `NORMAL` | 默认 | 正常 | 耳机满足时开 | 连接 | 现有 `hello` / `sync` |
| `BACKGROUND_LOCAL_PAUSE` | `onStop` | 本地暂停，不广播 | 关闭 | 断开 | 停发心跳位置，不发 `pause` |
| `FOREGROUND_PENDING_ALIGN` | `onStart` / `onResume` | 保持本机暂停 | 耳机满足时恢复 | 重连 | 忽略 `sync` 的自动 seek/play；手动 seek 后回 `NORMAL` |

语音模式由 `hello` 是否同时含 `voiceUrl` 与 `voiceToken` 判定；未配置语音时保持上游现有生命周期与同步行为。

## 检测方式

- 可见性用 `onStart` / `onStop`；焦点变化用 `onPause` / `onResume`。
- 分屏焦点在 API 29+ 用 `onTopResumedActivityChanged` 判断顶层 resumed；API 24–28 用 `onPause` / `onResume` 兜底。
- `isInMultiWindowMode()`（API 24+）只用于区分是否处于分屏；需要时监听 `onMultiWindowModeChanged()`。
- 锁屏通常触发 `onStop`；若厂商不触发，用 `ACTION_SCREEN_OFF` 兜底关闭麦克风并断开。
- 进程被系统回收时，保存服务器、房间码、昵称；回前台重新 `join` 获取新 token。
- 分屏失去焦点只关麦，保持视频和远端音频；不要仅凭 `onPause` 判断已经切到后台。

## 耳机与权限

- 用 `AudioManager` 输出设备和 `AudioDeviceCallback` 判断有线/蓝牙/USB 耳机；扬声器和听筒不算耳机。
- 拔耳机立即关麦；无耳机、权限被拒或麦克风被占用时只禁用发言并提示原因，仍可收听。
- Manifest 只加 `RECORD_AUDIO`；若 SDK 明确要求再加 `MODIFY_AUDIO_SETTINGS`。第一版不做前台 Service，不加 `POST_NOTIFICATIONS` / `FOREGROUND_SERVICE_MICROPHONE`。
- T3 先固定 LiveKit Android SDK 版本，用最小连接/发布/订阅代码验证 minSdk 24 / SDK 34 / Gradle 8.7 / JDK 17 可构建，再改播放 UI。

## 本地暂停与主动暂停

- 用户点暂停：仍发送 `pause`，全房间暂停；现有协议语义不变。
- 切后台自动暂停：只暂停本机，保留 SyncWatch 信令连接，不发送 `pause`；显式抑制 `onLocalPause` 广播；可发一次 `memberState=paused`，随后停发心跳位置。
- 回前台不自动 seek；保持本机暂停并重连语音，用户商量后手动 seek；现有 `seek` 会同步全房间。
- 待对齐期间收到 `sync` 只更新房间目标与成员状态，不自动 seek/play；禁用直接播放，手动 seek 后恢复心跳和播放按钮。
- 不自动追赶，避免把全房间拉回旧进度。

## 音频焦点

- 影片播放和语音都受 Android 音频焦点约束；另一个 App 播放媒体时，系统可能压低或暂停其中一个。
- T3 请求音频焦点并处理丢失/恢复；T4 记录分屏与音频焦点冲突的真机结果。

## 后续后台语音

- 当前版本不做后台语音；`onStop` 直接断开，不需要前台 Service。
- 后续若要做后台收听：把语音会话从 Activity 抽成独立控制器，再包进前台 Service；协议与服务端已支持，不需要改 `voiceUrl` / `voiceToken`。

## 诊断日志（2026-11-17）

> 状态：已实现，模拟器导出验证通过；真机双机转发验证待做。目标是让物理距离下的另一台真机能把排障证据转发给开发端。

### 为什么要显式导出

release 包 `debuggable=false`，`adb shell run-as` 读取应用私有目录不可用；只把日志写进手机、不提供导出入口，远端用户无法交付证据。因此后台自动记录负责留住现场，设置页与成员抽屉的「导出诊断日志」负责把文件交出来。

### 记录与保留

- release 与 debug 都记录；只在本地保存，不自动上传。
- 按天写入应用私有目录，保留最近 7 天，总量上限 2 MB；应用启动时清理过期文件。
- 日志保留独立于房间寿命；服务端房间在最后一人离开约 15 分钟后回收，日志不应随之删除。
- 退出房间后继续记录应用级事件；语音与播放连接按既有行为断开，但导出入口仍可在首页高级设置使用。

### 记录内容

- 连接与断开、入房、`hello`（含 `created`，用于识别同码重建）、播放/暂停/seek、后台与前台切换。
- 语音连接/重连/断开、远端音频订阅、麦克风开关、耳机插拔、分屏、音频焦点、权限失败。
- 服务端 `forceSeek`、异常与导出结果。
- 心跳每 30 秒记录一次位置摘要；不逐条记录心跳。
- 记录机型、系统版本、APK 版本、时间、房间码、昵称与完整服务器地址，便于与服务端日志对齐。

### 脱敏边界

- 永不记录：`voiceToken` 正文、房间密码、LiveKit API Key/Secret。
- `join` 消息保留事件，但密码替换为 `***`；token 只记录长度。
- 不记录设备唯一标识（包括 Android ID 及其哈希）。
- 导出只能由用户主动触发；文件含服务器地址，只转发给开发端，禁止进入 PR。

### 导出与失败处理

- `activity_main` 成员抽屉与首页「高级设置」各提供「导出诊断日志」。
- 导出单个 `.txt`，开头写机型、系统、APK 版本、时间与当前/最近房间码。
- 通过系统分享面板发送，不申请存储权限；新增组件仅 `FileProvider`。
- 成功提示「诊断日志已生成，请选择发送方式」；失败时提示原因并保留私有目录文件，便于重试。

### 验收口径

- release 与 debug 均可构建；模拟器中生成日志、导出成功。
- 检查 `join`、`hello`、`voiceToken` 在日志与 `logcat` 中均无密码或 token 正文。
- 验证按天切分、7 天清理与 2 MB 上限。
- 验证退出房间后语音与播放连接断开，且首页仍能导出日志。
- 真机双机测试不强制 30 分钟，但应尽可能延长；至少主测手机保留完整日志，异常时另一台手机补导出。
