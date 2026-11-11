# 语音协议补充（fork）

> 上游 `docs/02-同步协议.md` 未修改。本文件只记录 fork 语音扩展新增的可选字段；未配置 LiveKit 时，协议与上游一致。

## `hello` 新增字段

| 字段 | 类型 | 说明 |
|---|---|---|
| `voiceUrl` | string | 客户端连接 LiveKit 的地址，来自服务端 `LIVEKIT_URL`，例如 `ws://1.2.3.4:7880`；不是秘密 |
| `voiceToken` | string | LiveKit JWT，包含房间码、昵称、有效期与音频权限；bearer 凭证，不得写日志或放进 URL |

两个字段都使用 `omitempty`，且**同生同灭**：只有 `LIVEKIT_URL`、`LIVEKIT_API_KEY`、`LIVEKIT_API_SECRET` 全部配置时才出现；缺任一项时客户端隐藏语音入口。

## 兼容性

- 服务端是单个扁平 `Msg` 结构体，新增字段不影响现有字段。
- Android 用 `org.json` + `optString` 解析，未知字段本来就会被忽略。
- Windows 端 Go `json.Unmarshal` 忽略未知字段；T2 时确认一次即可。
- 这是**新增可选字段**，不改变现有 `hello` 语义，不属于「五处同步」针对的语义变更。

## token 约束

- room = SyncWatch 的 6 位房间码。
- identity = 昵称 + 每次 `join` 生成的短后缀（如 `小明#a3f9`）；界面仍显示昵称。
- 有效期 6 小时；过期或重连时重新 `join` 获取新 token。
- 权限只含加入指定房间、发布麦克风音频、订阅房间已发布轨道；不授予视频、录制或管理员权限。

| 权限 | 值 |
|---|---|
| `roomJoin` | `true` |
| `room` | SyncWatch 房间码 |
| `canPublish` | `true` |
| `canPublishSources` | `["microphone"]` |
| `canSubscribe` | `true` |
| `canPublishData` | `false` |
| `roomAdmin` / `roomCreate` / `roomRecord` / `recorder` | `false` |

`canSubscribe` 订阅的是房间内所有已发布轨道；第一版只发布麦克风，因此等价于音频订阅。未来若允许视频，需要重新评估这个权限。

## 异常路径

- 配置不完整或签发失败时不阻断普通 `join`，`voiceUrl` 与 `voiceToken` 都不下发，客户端隐藏语音入口。
- 服务端日志不得打印 API Secret、完整 token 或可复用凭证；排障只记录 token 长度和非敏感 claims。
- 旧 Android、Windows 控制台和 GUI 应忽略新字段并正常入房；T2 用 `tools/sim` / `tools/ctl` 和实际客户端各验证一次。

## 安全边界

- 仅两人私有使用，暂不对外服务；实验期信令和 LiveKit 都走 `ws://`，token 可能在链路被窃听。
- 6 小时 TTL 覆盖一两集电视剧或一部电影；接受泄露后最长 6 小时被冒用的风险。
- JWT payload 只是 Base64 编码，不是加密；签名只防篡改。
- 对外提供服务前必须加 TLS；届时再评估独立 `/voice-token` 接口、更短有效期与房间复用防护。
