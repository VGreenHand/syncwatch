@echo off
chcp 65001 >nul
setlocal enabledelayedexpansion
cd /d "%~dp0"

rem ============================================================
rem  一键构建 Android APK
rem  产物：..\dist\一起看-android-v0.5.1.apk
rem
rem  说明：AGP 不接受含中文的项目路径，所以先把源码复制到
rem        %TEMP%\syncwatch-android-build 再编译。
rem ============================================================

set "VER=v0.5.1"
set "BUILD=%TEMP%\syncwatch-android-build"

rem ---------- 0. 载入本机工具链路径（可选） ----------
if exist "%~dp0..\local-env.bat" call "%~dp0..\local-env.bat"

rem ---------- 1. 查找 JDK 17 ----------
if defined JAVA_HOME if exist "%JAVA_HOME%\bin\java.exe" goto java_ok
for /d %%d in ("C:\Program Files\Java\jdk-17*") do set "JAVA_HOME=%%d"
for /d %%d in ("C:\Program Files\Eclipse Adoptium\jdk-17*") do set "JAVA_HOME=%%d"
for /d %%d in ("F:\android-tools\jdk17*") do set "JAVA_HOME=%%d"

:java_ok
if not defined JAVA_HOME goto no_java
if not exist "%JAVA_HOME%\bin\java.exe" goto no_java
set "PATH=%JAVA_HOME%\bin;%PATH%"

rem ---------- 2. 查找 Android SDK ----------
if defined ANDROID_HOME if exist "%ANDROID_HOME%\platform-tools" goto sdk_ok
if exist "F:\android-tools\sdk\platform-tools" set "ANDROID_HOME=F:\android-tools\sdk"
if exist "%LOCALAPPDATA%\Android\Sdk\platform-tools" set "ANDROID_HOME=%LOCALAPPDATA%\Android\Sdk"

:sdk_ok
if not defined ANDROID_HOME goto no_sdk
if not exist "%ANDROID_HOME%\platform-tools" goto no_sdk

echo JDK    : %JAVA_HOME%
echo SDK    : %ANDROID_HOME%
echo.

rem ---------- 3. 复制到纯 ASCII 路径 ----------
echo [1/4] 同步源码到 %BUILD%
if exist "%BUILD%" rmdir /s /q "%BUILD%"
robocopy "%~dp0." "%BUILD%" /E /XD .gradle build .idea /NFL /NDL /NJH /NJS /NP >nul
if errorlevel 8 (
    echo ❌ 复制源码失败（robocopy 错误码 %errorlevel%）
    exit /b 1
)

rem 确保 local.properties 存在（sdk 路径用正斜杠，Windows 上的 Gradle 也认）
if not exist "%BUILD%\local.properties" (
    set "SDKFWD=!ANDROID_HOME:\=/!"
    echo sdk.dir=!SDKFWD!> "%BUILD%\local.properties"
)

rem ---------- 4. 选定 Gradle：local-env → PATH → 仓库自带 wrapper ----------
if not defined GRADLE_BIN (
    where gradle >nul 2>nul
    if not errorlevel 1 set "GRADLE_BIN=gradle"
)
if not defined GRADLE_BIN if exist "F:\android-tools\gradle-tmp\gradle-8.7\bin\gradle.bat" set "GRADLE_BIN=F:\android-tools\gradle-tmp\gradle-8.7\bin\gradle.bat"
if not defined GRADLE_BIN if exist "%BUILD%\gradlew.bat" set "GRADLE_BIN=%BUILD%\gradlew.bat"
if not defined GRADLE_BIN goto no_gradle
echo Gradle : %GRADLE_BIN%
echo.

rem ---------- 5. 编译 ----------
echo [2/4] 编译中（首次约 2~3 分钟，会从阿里云 Maven 镜像拉依赖）...
call "%GRADLE_BIN%" -p "%BUILD%" assembleDebug --console=plain
if errorlevel 1 goto fail

rem ---------- 6. 回收产物 ----------
set "APK=%BUILD%\app\build\outputs\apk\debug\app-debug.apk"
if not exist "%APK%" (
    echo ❌ 编译结束但未找到 APK：%APK%
    goto fail
)
if not exist "%~dp0..\dist" mkdir "%~dp0..\dist"
set "OUT=%~dp0..\dist\一起看-android-%VER%.apk"
echo [3/4] 拷贝 APK...
copy /y "%APK%" "%OUT%" >nul

echo [4/4] 完成
for %%f in ("%OUT%") do echo.
for %%f in ("%OUT%") do echo ✅ 产物：%%~ff  (%%~zf 字节)
echo.
echo 传到手机安装即可（Android 7.0+，允许"未知来源"）。
exit /b 0

rem ============================================================
:no_java
echo ❌ 未找到 JDK 17。请安装 JDK 17 并在 local-env.bat 里设置 JAVA_HOME。
echo    下载：https://mirrors.tuna.tsinghua.edu.cn/Adoptium/17/jdk/x64/windows/
exit /b 1

:no_sdk
echo ❌ 未找到 Android SDK。请安装 SDK 并在 local-env.bat 里设置 ANDROID_HOME。
echo    需要组件：platform-tools、platforms;android-34、build-tools;34.0.0
exit /b 1

:no_gradle
echo ❌ 未找到 Gradle。请安装 Gradle 8.7 并在 local-env.bat 里设置 GRADLE_BIN。
echo    下载：https://mirrors.cloud.tencent.com/gradle/gradle-8.7-bin.zip
exit /b 1

:fail
echo.
echo ❌ 构建失败，请查看上方 Gradle 输出
exit /b 1
