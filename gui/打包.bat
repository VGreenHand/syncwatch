@echo off
setlocal
cd /d "%~dp0"

rem ============================================================
rem  打包成可分发压缩包：dist\一起看-windows-v0.5.zip
rem  内含 GUI 主程序 + mpv（从 windows\mpv 复用，不重复下载）
rem  解压后双击 开始一起看.bat 即可，无需安装任何东西
rem ============================================================

set "VER=v0.5"
set "DIST=%~dp0..\dist"
set "MPV_SRC=%~dp0..\windows\mpv"
set "STAGE=%TEMP%\syncwatch-gui-pkg"

if not exist "build\bin\gui.exe" (
    echo 未找到 build\bin\gui.exe，先编译...
    call build.bat
)
if not exist "build\bin\gui.exe" (
    echo 编译失败，无法打包
    pause
    exit /b 1
)
if not exist "%MPV_SRC%\mpv.exe" (
    echo 未找到 mpv：%MPV_SRC%\mpv.exe
    echo 请先运行 windows\fetch-mpv.ps1 下载播放内核
    pause
    exit /b 1
)

if not exist "%DIST%" mkdir "%DIST%"
set "ZIP=%DIST%\一起看-windows-%VER%.zip"
if exist "%ZIP%" del /q "%ZIP%"

echo [1/3] 收集文件...
if exist "%STAGE%" rmdir /s /q "%STAGE%"
mkdir "%STAGE%"
copy /y "build\bin\gui.exe" "%STAGE%\一起看.exe" >nul
copy /y "开始一起看.bat" "%STAGE%" >nul
copy /y "使用说明.txt" "%STAGE%" >nul
if exist "server.txt" (copy /y "server.txt" "%STAGE%" >nul) else (echo 提示: 未找到 server.txt，分发包将要求用户手填服务器地址)
xcopy "%MPV_SRC%" "%STAGE%\mpv\" /E /I /Y /Q >nul

echo [2/3] 压缩（含 mpv，约 50MB，需要十几秒）...
powershell -NoProfile -Command "Compress-Archive -Path '%STAGE%\*' -DestinationPath '%ZIP%' -Force"
if errorlevel 1 (
    echo 压缩失败
    pause
    exit /b 1
)
rmdir /s /q "%STAGE%"

echo [3/3] 完成
echo.
for %%f in ("%ZIP%") do echo 打包完成：%%~ff  (%%~zf 字节)
echo.
echo 把这个 zip 发给朋友，解压后双击「开始一起看.bat」。
echo 若已放 server.txt 则对方可直连；否则需在设置里填写。
pause
