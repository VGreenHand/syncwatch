@echo off
chcp 65001 >nul
setlocal enabledelayedexpansion
cd /d "%~dp0"

rem ============================================================
rem  Build the Android APK (release variant, real signing key)
rem  Output: ..\dist\<app>-android-<VER>.apk
rem
rem  NOTE: AGP rejects non-ASCII project paths, so sources are
rem        copied to %TEMP%\syncwatch-android-build first.
rem
rem  Why assembleRelease instead of assembleDebug:
rem    a debug APK carries android:debuggable="true" and is signed
rem    with CN=Android Debug, which makes phone security suites
rem    (Tencent/Huawei/Xiaomi managers) flag it as risky or
rem    virus-like. Release is non-debuggable and signed with the
rem    key described in keystore.properties.
rem
rem  Keep this file ASCII-only: cmd.exe mis-parses UTF-8 text in
rem  rem/echo lines and silently truncates the surrounding tokens.
rem ============================================================

set "VER=v0.5.2-ui"
set "BUILD=%TEMP%\syncwatch-android-build"

rem ---------- 0. local toolchain paths (optional) ----------
if exist "%~dp0..\local-env.bat" call "%~dp0..\local-env.bat"

rem ---------- 1. locate JDK 17 ----------
if defined JAVA_HOME if exist "%JAVA_HOME%\bin\java.exe" goto java_ok
for /d %%d in ("C:\Program Files\Java\jdk-17*") do if exist "%%~d\bin\java.exe" set "JAVA_HOME=%%d"
for /d %%d in ("C:\Program Files\Eclipse Adoptium\jdk-17*") do if exist "%%~d\bin\java.exe" set "JAVA_HOME=%%d"
for /d %%d in ("F:\android-tools\jdk17*") do if exist "%%~d\bin\java.exe" set "JAVA_HOME=%%d"

:java_ok
if not defined JAVA_HOME goto no_java
if not exist "%JAVA_HOME%\bin\java.exe" goto no_java
set "PATH=%JAVA_HOME%\bin;%PATH%"

rem ---------- 2. locate Android SDK ----------
if defined ANDROID_HOME if exist "%ANDROID_HOME%\platform-tools" goto sdk_ok
if exist "F:\android-tools\sdk\platform-tools" set "ANDROID_HOME=F:\android-tools\sdk"
if exist "%LOCALAPPDATA%\Android\Sdk\platform-tools" set "ANDROID_HOME=%LOCALAPPDATA%\Android\Sdk"

:sdk_ok
if not defined ANDROID_HOME goto no_sdk
if not exist "%ANDROID_HOME%\platform-tools" goto no_sdk

rem ---------- 3. signing material must exist ----------
if not exist "keystore.properties" goto no_keystore
if not exist "keystore\syncwatch-release.jks" goto no_keystore

echo JDK    : %JAVA_HOME%
echo SDK    : %ANDROID_HOME%
echo KEY    : keystore\syncwatch-release.jks
echo.

rem ---------- 4. copy sources to an ASCII-only path ----------
echo [1/5] Syncing sources to %BUILD%
if exist "%BUILD%" rmdir /s /q "%BUILD%"
robocopy "%~dp0." "%BUILD%" /E /XD .gradle build .idea /NFL /NDL /NJH /NJS /NP >nul
if errorlevel 8 (
    echo [FAIL] robocopy error %errorlevel%
    exit /b 1
)

if not exist "%BUILD%\local.properties" (
    set "SDKFWD=!ANDROID_HOME:\=/!"
    echo sdk.dir=!SDKFWD!> "%BUILD%\local.properties"
)

rem ---------- 5. pick a Gradle: local-env -> PATH -> wrapper ----------
if not defined GRADLE_BIN (
    where gradle >nul 2>nul
    if not errorlevel 1 set "GRADLE_BIN=gradle"
)
if not defined GRADLE_BIN if exist "F:\android-tools\gradle-tmp\gradle-8.7\bin\gradle.bat" set "GRADLE_BIN=F:\android-tools\gradle-tmp\gradle-8.7\bin\gradle.bat"
if not defined GRADLE_BIN if exist "%BUILD%\gradlew.bat" set "GRADLE_BIN=%BUILD%\gradlew.bat"
if not defined GRADLE_BIN goto no_gradle
echo Gradle : %GRADLE_BIN%
echo.

rem ---------- 6. build release ----------
echo [2/5] Building release (first run takes 2-3 min, deps from aliyun mirror)...
call "%GRADLE_BIN%" -p "%BUILD%" assembleRelease --console=plain
if errorlevel 1 goto fail

rem ---------- 7. collect artifact ----------
set "APK=%BUILD%\app\build\outputs\apk\release\app-release.apk"
if not exist "%APK%" (
    echo [FAIL] build finished but no APK at %APK%
    goto fail
)
if not exist "%~dp0..\dist" mkdir "%~dp0..\dist"

set "OUT=%~dp0..\dist\一起看-android-%VER%.apk"
echo [3/5] Copying APK...
copy /y "%APK%" "%OUT%" >nul

rem ---------- 8. verify the signature is not a debug key ----------
echo [4/5] Verifying signature...
set "APKSIGNER=%ANDROID_HOME%\build-tools\34.0.0\apksigner.bat"
if not exist "%APKSIGNER%" (
    for /d %%d in ("%ANDROID_HOME%\build-tools\*") do if exist "%%~d\apksigner.bat" set "APKSIGNER=%%~d\apksigner.bat"
)
if exist "%APKSIGNER%" (
    call "%APKSIGNER%" verify --print-certs "%OUT%" | findstr /i "Android Debug" >nul
    if not errorlevel 1 (
        echo [FAIL] artifact is still signed with the debug key
        goto fail
    )
    echo     OK: not a debug certificate
) else (
    echo     WARN: apksigner not found, skipped certificate check
)

rem ---------- 9. done ----------
echo [5/5] Done
for %%f in ("%OUT%") do echo.
for %%f in ("%OUT%") do echo [OK] %%~ff  (%%~zf bytes)
echo.
echo Uninstall any previous build first: the key changed, so the new
echo APK cannot upgrade over the old debug-signed install.
echo Sideloading always keeps the "unknown source" prompt; the virus
echo warning should be gone once the package is a proper release build.
exit /b 0

rem ============================================================
:no_keystore
echo [FAIL] Missing signing material.
echo   Need android\keystore.properties and android\keystore\syncwatch-release.jks
echo   (both git-ignored; carry them when switching machines).
exit /b 1

:no_java
echo [FAIL] JDK 17 not found. Install it and set JAVA_HOME in local-env.bat.
echo   https://mirrors.tuna.tsinghua.edu.cn/Adoptium/17/jdk/x64/windows/
exit /b 1

:no_sdk
echo [FAIL] Android SDK not found. Set ANDROID_HOME in local-env.bat.
echo   Needed: platform-tools, platforms;android-34, build-tools;34.0.0
exit /b 1

:no_gradle
echo [FAIL] Gradle not found. Install Gradle 8.7 and set GRADLE_BIN.
echo   https://mirrors.cloud.tencent.com/gradle/gradle-8.7-bin.zip
exit /b 1

:fail
echo.
echo [FAIL] Build failed, see the Gradle output above.
exit /b 1
