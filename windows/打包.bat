@echo off
chcp 65001 >nul
setlocal
cd /d "%~dp0"

rem ============================================================
rem  打包成可分发压缩包：dist\一起看-windows-v0.3.zip
rem  对方解压后双击「启动观影.bat」即可，无需安装任何东西
rem ============================================================

if not exist "syncwatch-win.exe" (
    echo 未找到 syncwatch-win.exe，先编译...
    call build.bat
)
if not exist "syncwatch-win.exe" (
    echo ❌ 编译失败，无法打包
    pause
    exit /b 1
)
if not exist "mpv\mpv.exe" (
    echo ❌ 未找到 mpv，先运行 fetch-mpv.ps1
    pause
    exit /b 1
)

set "DIST=%~dp0..\dist"
if not exist "%DIST%" mkdir "%DIST%"
set "ZIP=%DIST%\一起看-windows-v0.3.zip"
if exist "%ZIP%" del /q "%ZIP%"

echo [1/2] 收集文件到临时目录...
set "STAGE=%TEMP%\syncwatch-pkg"
if exist "%STAGE%" rmdir /s /q "%STAGE%"
mkdir "%STAGE%"
copy /y "syncwatch-win.exe" "%STAGE%" >nul
copy /y "启动观影.bat" "%STAGE%" >nul
copy /y "使用说明.txt" "%STAGE%" >nul
if exist "server.txt" (copy /y "server.txt" "%STAGE%" >nul) else (echo 警告：未找到 server.txt，分发包将使用内置地址)
xcopy "mpv" "%STAGE%\mpv\" /E /I /Y /Q >nul

echo [2/2] 压缩...
powershell -NoProfile -Command "Compress-Archive -Path '%STAGE%\*' -DestinationPath '%ZIP%' -Force"
if errorlevel 1 (
    echo ❌ 压缩失败
    pause
    exit /b 1
)
rmdir /s /q "%STAGE%"

echo.
for %%f in ("%ZIP%") do echo ✅ 打包完成：%%~ff  (%%~zf 字节)
echo.
echo 把该 zip 发给朋友，解压双击「启动观影.bat」即可。
pause
