@echo off
setlocal
cd /d "%~dp0"

rem ============================================================
rem  构建 GUI 客户端：wails build -> build\bin\gui.exe
rem  需要 Go 1.22+ 与 Node 18+；Wails 会自动调用 npm 打包前端
rem ============================================================

where wails >nul 2>nul
if errorlevel 1 (
    echo 未找到 wails 命令，先安装：
    echo   go install github.com/wailsapp/wails/v2/cmd/wails@latest
    echo 并把 %%USERPROFILE%%\go\bin 加入 PATH
    pause
    exit /b 1
)

echo [构建] 当前目录：%CD%
wails build -platform windows/amd64
if errorlevel 1 (
    echo 构建失败
    pause
    exit /b 1
)

echo.
if exist "build\bin\gui.exe" (
    for %%f in ("build\bin\gui.exe") do echo 构建完成：%%~ff  (%%~zf 字节)
) else (
    echo 未找到产物 build\bin\gui.exe
)
pause
