# Android 客户端

Kotlin + Media3(ExoPlayer) + OkHttp WebSocket，minSdk 24（Android 7.0+），仅 APK 侧载分发。

> 当前界面为 **0.5.2-ui 本地迭代版**：深色沉浸主题、自定义播放控制层、横滑快进快退、平板双栏。
> 服务端与同步协议未变，可以和 v0.5.1 的 Windows 客户端直接互通。

## 安装

1. 把 `dist\一起看-android-v0.5.2-ui.apk` 传到手机（微信文件传输 / 数据线 / 网盘均可）
2. 点击安装，首次提示"未知来源应用"时允许
3. 系统要求：Android 7.0 及以上

## 使用

1. 打开 App，在首页顶部选择 **创建房间** 或 **加入房间**：
   - **昵称**：房间内显示的名字，会记住上次输入
   - **本地视频**：从手机存储选中片子，走系统文件选择器，不需要存储权限
   - **房间码**：加入朋友房间时填写；创建房间时不需要填
   - **房间密码**：可选，创建者与加入者需一致
   - **服务器地址**：位于首页底部「高级设置」，分发包通常已内置，无需修改
2. 创建房间会直接生成 6 位房间码；加入房间则校验输入的码。
3. 创建者点房间码旁的复制按钮，把码发给朋友。
4. 如果加入时房间码不存在，会弹出阻断式提示，避免误留在服务器新建的同码空房里。

## 观影中

### 竖屏

- 视频下方显示片名、房间码、连接状态与在线成员
- 点房间码或复制按钮可一键复制
- 点成员摘要展开播放/暂停/缓冲状态与片源时长；时长不一致会提示
- **全屏观看**进入横屏沉浸模式

### 横屏与手势

| 操作 | 效果 |
|------|------|
| 左右滑动画面 | 预览目标时间，松手后跳转并向全房间同步一次 |
| 双击画面 | 播放 / 暂停 |
| 单击画面 | 显示 / 隐藏控制层 |
| 左侧上下滑 | 调整本机亮度 |
| 右侧上下滑 | 调整本机音量 |
| 底部快退 / 快进 | 各跳 10 秒 |
| 锁图标 | 锁定播放界面，仅保留解锁入口；远端同步仍然生效 |

- 完整横滑约对应 90 秒；进度限制在片头与片尾之间
- 手势只在松手时真正 seek，避免逐帧拖动造成卡顿与刷屏
- 横滑预览期间如果朋友操作了进度，远端位置优先，本地未提交的预览会取消
- 倍速在播放中调整会同步全房间；暂停时调整只保存在本端，继续播放时随 play 消息带上
- 横屏按返回：先退出全屏；竖屏按返回：确认后才离开房间

### 其他

- **断线自动重连**：5s 间隔重试最多 30 次，重连后自动追到房间当前进度
- **屏幕常亮**：播放期间自动保持
- 亮度、音量只影响本机，不会同步给其他成员

### 平板

可用宽度达到 600dp 后，播放页自动变为“左侧视频 + 右侧房间信息”双栏；首页卡片限宽居中，不会把手机界面简单拉伸。

## 与电脑端互通

同一套协议、同一台服务器，**可以混用**：一台 Windows + 一部手机 + 另一台电脑同房，上限 5 人。本次 UI 改版不涉及协议字段或同步阈值。

## 配置（构建时注入默认服务器地址）

默认服务器地址不在源码里，而是构建时从 `android/local.properties` 读取，这样仓库里不含私人 IP：

```properties
# android/local.properties（不入库，从 local.properties.example 复制）
sdk.dir=C\:\\Android\\Sdk
syncwatch.server=你的服务器IP:8995
```

`app/build.gradle` 会把它写成 `BuildConfig.DEFAULT_SERVER`，`MainActivity` 首次启动时填入高级设置。
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
4. 产物拷到 `dist\一起看-android-v0.5.2-ui.apk`。

### 单元测试

```bash
cd android
./gradlew testDebugUnitTest
```

当前覆盖 `SeekPreviewCalculator` 的横滑位移换算与片头/片尾边界。若在本机中文路径下运行失败，请使用 `build-apk.bat` 相同的 ASCII 临时目录方式。

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

## 已知限制

- 依赖系统解码器：MP4/MKV 的 H.264/H.265 + AAC 全覆盖；**DTS 等特殊音轨会无声**（未集成 ffmpeg 扩展）
- 切到后台会暂停（未做前台 Service，观影场景本身需要亮屏）
- 片源需自己准备，App 不做在线播放
- 仅侧载，不上架应用市场（避免软著与审核周期）
- 横滑预览暂不显示视频缩略图
- 竖屏固定方向；横屏只通过全屏按钮进入，不随设备自动旋转
