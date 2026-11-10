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

## 会话交接（skill `session-handover`）

1. **接力先读入口**：checkout 工作线分支 → 读 `PROGRESS.md` §0 + §7 当前任务行 → 用 `Select-String -Path PROGRESS.md -Pattern '^### 12\.'` 比对最新流水；不通读全文、不读历史流水。
2. **单写者**：只有当前分支的收尾方/维护者能写 `PROGRESS.md`；其他会话发现入口过期只报告。
3. **收尾一次 commit**：追加 `### 12.N` 流水 → 刷 §0 入口 → 翻转 §7 任务状态行；实现方收尾置 `🔶 待验收`。
4. **维护者验收**：补 `✅ 已验收 + 决策`，再刷 §0。
5. **里程碑触发** `ledger-rotation`：阶段切换收尾时检查轮转（阈值：流水 > 40 条或 `PROGRESS.md` > 60 KB）；不在每次收尾检查。
6. **多机纪律**：一条分支同一时间只在一台机器活跃；换机前 commit + push 并更新 §0「当前占用」；共享分支不 `push --force`。

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
