# 联调工具（不是产品代码）

改完服务端或协议，用这两个小程序做回归，比开播放器快得多。

## sim —— 模拟成员

```bash
cd tools/sim
go run . -role a -room TEST01          # 成员 A：建房间 + 待在房里打印收到的广播
go run . -role b -room TEST01          # 成员 B：依次发 play/pause/seek
```

| 参数 | 默认 | 说明 |
|------|------|------|
| `-role` | b | `a`=创建者等待并打印；`b`=加入并发指令 |
| `-addr` | 127.0.0.1:8995 | 服务器地址 |
| `-room` | TEST01 | 房间码 |

## ctl —— 单条指令遥控器

```bash
cd tools/ctl
go run . -room TEST01 -name remote -cmd play  -pos 30
go run . -room TEST01 -name remote -cmd pause -pos 45
go run . -room TEST01 -name remote -cmd seek  -pos 120
```

| 参数 | 默认 | 说明 |
|------|------|------|
| `-room` | 必填 | 房间码 |
| `-name` | remote | 以什么昵称加入 |
| `-cmd` | play | `play` / `pause` / `seek` |
| `-pos` | 0 | 目标进度（秒） |
| `-addr` | 127.0.0.1:8995 | 服务器地址 |

典型用法：手机上开着 App 进房，然后用 `ctl` 在电脑上发指令，观察手机是否跟随。
