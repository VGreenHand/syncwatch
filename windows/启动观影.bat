@echo off
chcp 65001 >nul
setlocal enabledelayedexpansion
title 一起看 - 同步观影
cd /d "%~dp0"

rem 1. 检查客户端
if not exist "syncwatch-win.exe" (
    echo 未找到 syncwatch-win.exe，尝试编译...
    call build.bat
    if not exist "syncwatch-win.exe" (
        echo 编译失败，请先运行 build.bat 查看错误。
        pause
        exit /b 1
    )
)

rem 2. 检查 mpv
if exist "mpv\mpv.exe" goto check_server
if exist "mpv\mpv.com" goto check_server
echo 未找到 mpv 播放器，尝试自动下载（约 33MB）...
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0fetch-mpv.ps1"
if not exist "mpv\mpv.exe" if not exist "mpv\mpv.com" (
    echo.
    echo 自动下载失败。请手动把 mpv 解压到：%~dp0mpv\
    echo 下载地址：https://github.com/zhongfly/mpv-winbuild/releases
    pause
    exit /b 1
)

rem 3. 检查服务器地址（第一次运行问一次，之后记住）
:check_server
if exist "server.txt" goto run
echo.
echo 首次运行，请输入服务器地址（形如 1.2.3.4:8995，直接回车使用默认值）
set /p SRV=服务器地址:
if "!SRV!"=="" set "SRV=127.0.0.1:8995"
echo !SRV!> server.txt
echo 已保存到 server.txt，以后可在该文件中修改。
echo.

:run
syncwatch-win.exe
echo.
echo 客户端已退出。
pause
