# Android 语音扩展（fork 工作线）

> 本目录只描述 fork 的语音扩展，不改变上游项目的设计、范围或版本路线图。
> 上游文档（`README.md`、`docs/01`、`docs/02`、`docs/03`、`docs/05`）保持原文；语音相关事实先写在这里，PR 被上游接受后再回填。

## 范围

- 只做 Android 双人全双工语音；不做 Windows 语音、视频/屏幕传输、客户端 P2P、云端录像。
- LiveKit 是独立可选语音层；未配置时 fork 行为与上游一致。
- 施工分支：`feat/android-voice`；账本：`PROGRESS.md`；决策：`docs/adr/0001-android-voice-chat.md`。

## 文档索引

- `02-voice-protocol.md`（待建）：`voiceToken` 与 `hello` 扩展。
- `03-voice-deploy.md`（待建）：LiveKit 部署、端口、NAT 与排障。
- `05-android-voice.md`（待建）：权限、前台服务、耳机检测与音频路由。

## 与上游的关系

- 当前无上游写权限；T1–T4 在 fork 验证后，从 fork 向 `origin/main` 提 PR。
- 合并由主项目维护者执行；PR 只带语音代码与必要文档，不带 `AGENTS.md`、`.agents/`、`PROGRESS.md`。
