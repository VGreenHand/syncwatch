# Android 客户端

Kotlin + Media3(ExoPlayer) + OkHttp WebSocket，minSdk 24（Android 7.0+），仅 APK 侧载分发。

## 安装

1. 把 `dist\一起看-android-v0.1.apk` 传到手机（微信文件传输 / 数据线 / 网盘均可）
2. 点击安装，首次提示"未知来源应用"时允许
3. 系统要求：Android 7.0 及以上

## 使用

1. 打开 App，依次填写：
   - **服务器地址**：默认来自构建配置（见下方"配置"），换服务器时直接在输入框改，App 会记住上次输入
   - **昵称**：房间内唯一
   - **房间码**：创建房间**留空**（服务器生成 6 位码）；加入朋友房间填对方房间码
   - **房间密码**：可选，创建者与加入者需一致
2. 点「选择本地视频文件」，从手机存储选中片子（走系统文件选择器，不需要存储权限）
3. 点「进入房间」，顶部出现"已加入房间 XXXXXX"
4. 创建者把房间码发给朋友

## 观影中

- **顶部成员栏**：实时显示每位成员及状态（播放中 / 暂停 / 缓冲中）
- **播放、暂停、拖进度**：操作会同步全房间，并弹提示归因（如"小明 暂停了"）
- **断线自动重连**：5s 间隔重试最多 30 次，重连后自动追到房间当前进度
- **返回键**：退出房间回到加入页
- **屏幕常亮**：播放期间自动保持

## 与电脑端互通

同一套协议、同一台服务器，**可以混用**：一台 Windows + 一部手机 + 另一台电脑同房，上限 5 人。

## 配置（构建时注入默认服务器地址）

默认服务器地址不在源码里，而是构建时从 `android/local.properties` 读取，这样仓库里不含私人 IP：

```properties
# android/local.properties（不入库，从 local.properties.example 复制）
sdk.dir=C\:\\Android\\Sdk
syncwatch.server=你的服务器IP:8995
```

`app/build.gradle` 会把它写成 `BuildConfig.DEFAULT_SERVER`，`MainActivity` 首次启动时填入输入框。
改完地址**重新编译**才生效；只是临时换服务器的话，直接在 App 界面里改即可（会持久化）。

## 构建

### 环境要求

| 组件 | 版本 |
|------|------|
| JDK | 17 |
| Gradle | 8.7 |
| Android SDK | platform 34 + build-tools 34.0.0 |

### 一键构建

```bat
cd android
build-apk.bat
```

脚本做了这些事：

1. 按 `JAVA_HOME` → `F:\android-tools\jdk17*` 顺序自动找 JDK 17；
2. 按 `ANDROID_HOME` → `F:\android-tools\sdk` 找 SDK，并写入临时 `local.properties`；
3. **把工程复制到纯 ASCII 路径**（`%TEMP%\syncwatch-android-build`）再编译 —— AGP 不接受含中文的项目路径，这是踩过的坑；
4. 产物拷到 `dist\一起看-android-v0.1.apk`。

### 手动构建

仓库自带 Gradle Wrapper，无需预装 Gradle（首次会从腾讯云镜像下载 8.7）：

```bash
cd android
echo "sdk.dir=$ANDROID_HOME" > local.properties   # 或直接复制 local.properties.example
./gradlew assembleDebug        # Windows 用 gradlew.bat assembleDebug
```

> 项目路径含中文时 AGP 会直接报错，这正是 `build-apk.bat` 要先复制到 ASCII 路径的原因。手动构建请确保路径全英文。

首次构建会从阿里云 Maven 镜像拉依赖（`settings.gradle` 已配好），约 2~3 分钟。

### 换成自己的包名 / 应用名

- 包名：`android/app/build.gradle` 的 `namespace` 与 `applicationId`（两处要一起改）
- 应用名：`android/app/src/main/res/values/strings.xml` 的 `app_name`

## 已知限制（v0.1）

- 依赖系统解码器：MP4/MKV 的 H.264/H.265 + AAC 全覆盖；**DTS 等特殊音轨会无声**（未集成 ffmpeg 扩展）
- 切到后台会暂停（未做前台 Service，观影场景本身需要亮屏）
- 片源需自己准备，App 不做在线播放
- 仅侧载，不上架应用市场（避免软著与审核周期）
