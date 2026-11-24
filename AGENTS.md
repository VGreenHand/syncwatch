# AGENTS.md — SyncWatch 仓库规则

SyncWatch（一起看）是「Go 信令服务器 + Windows(mpv) + Android(Media3)」的本地片源同步观看项目。当前工作线是 **v0.6 Android 内置语音**（`feat/android-voice`）。

## 不可越过的边界

- **视频零传输**：不得加入客户端间 P2P 传视频、服务器分发文件的逻辑（README 红线）。
- 服务器不做公开文件服务或代理；密钥只放 `server/.env`、`server/deploy.env`、`private/`，仓库只放 `.example`。
- 公网 IP、SSH 配置、真实服务器地址不入库。
- 改协议语义必须**五处同步**：`server/main.go`、`windows/main.go`、`gui/syncclient.go`、`android/.../Protocol.kt`、`docs/02-同步协议.md`。新增可选字段至少要确认五处兼容（不使用该字段的客户端应忽略未知字段），文档按实际改动同步；不要用「新增字段」绕过语义变更检查。
- 语音是**可选伴生服务**：不得把 LiveKit 塞进 `syncwatch` 核心进程，也不得把媒体流混进 8 KB JSON 信令。

## 文档权威链

```
docs/adr/（永久决策）
  > README 红线 + docs/01（范围与不可逆边界）
  > PROGRESS.md §1–§3 + §7（当前工作线目标、边界、验收条件）
  > docs/02、03、04、05、06、07、08（实现事实）
  > PROGRESS.md §12 流水 + 证据
  > PROGRESS.md §0 入口（投影）
```

- 入口节落后于最新流水时，以最新流水为准；接力会话只口头报告，不擅自改写。
- ADR 的 `Proposed` 不等于已批准；只有维护者能把状态改为 `Accepted`。
- README、docs、任务说明提到当前状态时只放指针，不复制状态值。

## 会话交接配置（skill `session-handover` / `ledger-rotation`）

流程本身写在 `.agents/skills/session-handover/SKILL.md` 与 `.agents/skills/ledger-rotation/SKILL.md`，**这两份文件不含项目值，不要改动**。本节只放本仓库的值。

| 配置键 | 本仓库的值 |
| --- | --- |
| 工作线 | v0.6 Android 内置语音；施工分支 `feat/android-voice`，发布基线 `main`。账本随分支走，只写当前 checkout 的副本 |
| 账本 | `PROGRESS.md` |
| 入口节 | `## 0. 会话入口`（约前 25 行） |
| 流水节 | `### 12.N — 标题`；列候选：`Select-String -Path PROGRESS.md -Pattern '^### 12\.'`（git bash：`grep '^### 12\.' PROGRESS.md`）。**取最大号须用小数点后的 N 按数值比较**，例如 `12.31` 大于 `12.9`；不得只取每行的第一个数字（那会得到 `12`，稳定误报滞后） |
| 任务清单 | `§7 任务清单` 当前任务行 |
| 任务文档路由 | 见本节下方「按任务取文档」表 |
| 状态副本 | 无（README、docs 只放指针） |
| 证据位置 | 流水条内写 commit / 构建产物 / 真机或服务器记录；本项目不另建证据目录 |
| 状态机 | ⬜ 未开始 → 🚧 实现中 → ⏸ 阻塞(等待 X) → 🔶 待验收 → ✅ 已验收 |
| 权威链 | 见上文「文档权威链」 |
| 收尾提交 | 允许收尾提交；前缀 `feat(v0.6):` / `fix:` / `docs:` + 中文描述 |
| 轮转 | 流水 > 40 条 或 `PROGRESS.md` > 60 KB；归档 `docs/archive/PROGRESS-<年季>.md`；保留最近 20 条 |
| 附加纪律 | ① 多机：一条分支同一时间只在一台机器活跃，换机前 commit + push 并更新 §0「当前占用」，共享分支不 `push --force`。② ADR `Proposed` ≠ 已批准，只有维护者能改 `Accepted`。③ 未验证的结论必须标「未验证」。④ 未来新工作线在对应分支 `docs/branches/` 另立账本，不与语音线共用可写账本 |

按任务取文档（读协议第 4 步用；只读当前任务那一行，不读整张表）：

| 任务 | 开工前读 |
| --- | --- |
| T1 LiveKit 部署 | ADR §5/§7 + `docs/03-服务端部署.md` + `voice/` |
| T2 服务器下发 token | ADR §4 + `docs/02-同步协议.md` + `server/main.go` |
| T3 Android 接入 | ADR §6 + `docs/05-Android客户端.md` + `docs/07`/`08` + `android/` 相关源码 |
| T4 双机联调 | ADR「验收标准」+ `docs/05` 的已知限制 |
| T5 文档 | `docs/02`、`docs/05`、`README.md` 的对应章节 |

## 代码变更纪律（karpathy-guidelines）

写代码、审查代码、重构时按 `karpathy-guidelines` 执行；本项目的落地口径：

1. **先想再写**：动手前说明假设、影响面和验收方式。协议字段、ADR 决策、LiveKit 部署方式、Android 音频生命周期有歧义时先停下问维护者，不静默选一种解释；有多种方案时列出取舍。
2. **简单优先**：只实现当前任务需要的最小改动，不为 T1–T5 之外的功能预留抽象、配置或开关；不引入 T1–T5 未要求的新依赖、新服务、新目录。能用现有 `Msg`、`SyncEngine`、`tools/sim` 验证的，不新建框架。
3. **外科手术式修改**：只改与任务直接相关的文件；匹配现有 Go/Kotlin/文档风格；不顺手重构、不清理无关死代码、不改无关格式。自己的改动造成未使用的 import/变量/函数才删；发现相邻问题只报告，不擅自修。
4. **目标驱动**：先写可验证的成功标准，再按「步骤 → 验证」推进：

   ```text
   1. [改什么] → verify: [命令或观察结果]
   2. [改什么] → verify: [命令或观察结果]
   ```

   验证命令与门禁见下方「工程与验证」。没有实际跑过的结论标「未验证」，不写「应该可以」。

5. **按任务大小用判断**：小改动保持小改动；上述流程主要约束有行为、协议、部署或数据影响的改动。

## 工程与验证

- 提交前缀跟随仓库习惯：`feat(v0.6): 中文描述`、`fix: 中文描述`、`docs: 中文描述`。
- 服务端：`cd server && go build ./...`；协议回归用 `tools/sim` + `tools/ctl`。
- Android：JDK 17 / SDK 34 / Gradle 8.7；中文路径用 `android/build-apk.bat`（复制到 ASCII 临时目录再编译）。
- Windows：控制台版 `windows/build.bat`；图形版 `gui/build.bat`。
- 涉及真机或服务器的结论必须写清机型、系统版本、网络与日期；未验证就标「未验证」。
