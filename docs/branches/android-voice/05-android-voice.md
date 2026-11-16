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
