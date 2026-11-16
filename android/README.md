# Android 客户端（Kotlin + Media3）

## 构建

```bat
build-apk.bat        :: 产出 ..\dist\一起看-android-v0.5.2-ui.apk
```

依赖 JDK 17 + Android SDK（platform 34 / build-tools 34.0.0）+ Gradle 8.7，
路径写在仓库根目录的 `local-env.bat` 里（复制 `local-env.bat.example` 改）。

## 正式签名与「安装提示风险 / 病毒」

`build-apk.bat` 走 **`assembleRelease`**，用仓库外的正式密钥签名。
**不要改回 `assembleDebug`**：debug 包带 `android:debuggable="true"`，证书主体是
`CN=Android Debug`，手机管家 / 系统安全中心会直接判成「风险应用」甚至「病毒」。

签名材料两份，都在 `.gitignore` 里，换电脑要自己带上：

| 文件 | 说明 |
|---|---|
| `android/keystore.properties` | `storeFile` / `storePassword` / `keyAlias` / `keyPassword` |
| `android/keystore/syncwatch-release.jks` | 2026-11-14 生成，RSA 2048，有效期 30 年 |

> **丢了 jks 就再也无法给已装用户升级**（同包名必须同签名）。请离线备份两份
> （U 盘 + 私密网盘），密码另存。

彻底重建密钥（只在密钥丢失、且不在乎老用户升级时做）：

```bat
keytool -genkeypair -v -keystore keystore\syncwatch-release.jks ^
  -alias syncwatch -keyalg RSA -keysize 2048 -validity 10950 ^
  -dname "CN=SyncWatch, OU=Mobile, O=SyncWatch, L=Chengdu, ST=Sichuan, C=CN"
```

注意两点：

- 签名换了以后，手机上已装的旧 debug 版**必须先卸载**，否则报「签名不一致，安装失败」。
- 侧载（不走应用商店）仍会弹「未知来源」提示，这是系统行为消不掉；能消掉的是
  「风险 / 病毒」这个判定。

启动图标：`res/mipmap-anydpi-v26/` 是自适应图标（API 26+），`res/mipmap-*dpi/` 的
PNG 由 `tools/gen-launcher-icons.ps1` 生成（API 24/25 用），改图标后跑一次脚本。

## 代码结构

```
app/src/main/java/com/syncwatch/app/
├── MainActivity.kt             单 Activity：首页、播放器生命周期、同步回调协调
├── PlayerControlOverlay.kt     自定义 Media3 控制层、进度条、自动隐藏
├── PlayerGestureController.kt  横滑 seek / 双击 / 亮度 / 音量 / 锁定
├── SeekPreviewCalculator.kt    横滑位移 → 目标时间（纯函数，有单元测试）
├── SyncEngine.kt               WebSocket 连接、心跳、重连、指令 ⇄ 播放器桥接
└── Protocol.kt                 消息结构 JSON ⇄ Kotlin 对象
```

| 想改… | 位置 |
|-------|------|
| 首页布局 | `app/src/main/res/layout/activity_main.xml` |
| 播放控制层 | `app/src/main/res/layout/view_player_controller.xml` |
| 深色主题 / 色彩 | `res/values/colors.xml`、`themes.xml` |
| 横滑灵敏度 | `SeekPreviewCalculator.FULL_WIDTH_SEEK_MS` |
| 控制层隐藏时间 | `PlayerControlOverlay.AUTO_HIDE_MS` |
| 心跳间隔 / 重连次数 | `SyncEngine.kt` 顶部 `HB_INTERVAL` / `RECONNECT_MAX` |
| 漂移静默微调阈值 | `SyncEngine.kt` 的 `SOFT_DRIFT` |
| 默认服务器地址 | `local.properties` 的 `syncwatch.server`（不入库） |
| 包名 | `app/build.gradle` 的 `namespace` + `applicationId` |

## 界面与手势

- 首页：创建 / 加入分段切换，服务器地址收进高级设置。
- 播放页：竖屏视频 + 房间信息；手动全屏进入横屏沉浸模式。
- 横滑：预览目标时间，松手才 seek 一次并同步；完整横滑约 90 秒。
- 双击：播放 / 暂停；单击：显示 / 隐藏控制层。
- 全屏：左侧上下滑调亮度、右侧调音量；支持播放锁定。
- 平板：可用宽度 ≥600dp 时播放页双栏。

## 协议

与服务端、Windows 端共用一套：[../docs/02-同步协议.md](../docs/02-同步协议.md)

本次 UI 改版没有修改任何协议字段或服务端逻辑。
