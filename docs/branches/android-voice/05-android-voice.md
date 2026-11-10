# Android 语音接入（fork）

> 目标行为，T3 实现后以真机结果为准。

## 生命周期

| 状态 | 回调 | 视频 | 麦克风 | 语音连接 |
|---|---|---|---|---|
| 分屏失去焦点 | `onPause` | 继续 | 关闭 | 保持，继续收听 |
| 切到后台/锁屏 | `onStop` | 暂停 | 关闭 | 断开 |
| 回前台 | `onStart` / `onResume` | 恢复 | 耳机满足时恢复 | 重连 |

## 检测方式

- 可见性用 `onStart` / `onStop`；焦点变化用 `onPause` / `onResume`。
- 分屏用 `isInMultiWindowMode()`（API 24+）；需要时监听 `onMultiWindowModeChanged()`。
- 锁屏通常触发 `onStop`；若厂商不触发，用 `ACTION_SCREEN_OFF` 兜底关闭麦克风并断开。
- 进程被系统回收时，保存服务器、房间码、昵称；回前台重新 `join` 获取新 token。

## 音频焦点

- 影片播放和语音都受 Android 音频焦点约束；另一个 App 播放媒体时，系统可能压低或暂停其中一个。
- T3 请求音频焦点并处理丢失/恢复；T4 记录分屏与音频焦点冲突的真机结果。

## 后续后台语音

- 当前版本不做后台语音；`onStop` 直接断开，不需要前台 Service。
- 后续若要做后台收听：把语音会话从 Activity 抽成独立控制器，再包进前台 Service；协议与服务端已支持，不需要改 `voiceUrl` / `voiceToken`。
