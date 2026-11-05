# Android 客户端（Kotlin + Media3）

## 构建

```bat
build-apk.bat        :: 产出 ..\dist\一起看-android-v0.4.apk
```

依赖 JDK 17 + Android SDK（platform 34 / build-tools 34.0.0）+ Gradle 8.7，
路径写在仓库根目录的 `local-env.bat` 里（复制 `local-env.bat.example` 改）。

## 代码结构

```
app/src/main/java/com/syncwatch/app/
├── MainActivity.kt   单 Activity：左侧加入表单 → 右侧 ExoPlayer 播放页
├── SyncEngine.kt     WebSocket 连接、心跳、重连、指令 ⇄ 播放器桥接
└── Protocol.kt       消息结构 JSON ⇄ Kotlin 对象
```

| 想改… | 位置 |
|-------|------|
| 界面布局 | `app/src/main/res/layout/activity_main.xml` |
| 心跳间隔 / 重连次数 | `SyncEngine.kt` 顶部 `HB_INTERVAL` / `RECONNECT_MAX` |
| 漂移静默微调阈值 | `SyncEngine.kt` 的 `SOFT_DRIFT` |
| 默认服务器地址 | `local.properties` 的 `syncwatch.server`（不入库） |
| 包名 | `app/build.gradle` 的 `namespace` + `applicationId` |

## 协议

与服务端、Windows 端共用一套：[../docs/02-同步协议.md](../docs/02-同步协议.md)
