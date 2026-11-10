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
- 权限只含加入指定房间、发布音频、订阅音频；不授予视频、录制或管理员权限。

## 安全边界

- 仅两人私有使用，暂不对外服务；实验期信令和 LiveKit 都走 `ws://`，token 可能在链路被窃听。
- 6 小时 TTL 覆盖一两集电视剧或一部电影；接受泄露后最长 6 小时被冒用的风险。
- JWT payload 只是 Base64 编码，不是加密；签名只防篡改。
- 对外提供服务前必须加 TLS；届时再评估独立 `/voice-token` 接口、更短有效期与房间复用防护。
