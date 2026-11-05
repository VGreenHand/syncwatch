@echo off
chcp 65001 >nul
setlocal
cd /d "%~dp0"

rem ============================================================
rem  编译 Windows 客户端：优先用本机 Go，没有则用 Docker 交叉编译
rem  产物：windows\syncwatch-win.exe
rem ============================================================

if exist "%~dp0..\local-env.bat" call "%~dp0..\local-env.bat"
if defined GOROOT set "PATH=%GOROOT%\bin;%PATH%"
if not defined GOPROXY set "GOPROXY=https://goproxy.cn,direct"

set "OUT=syncwatch-win.exe"

where go >nul 2>nul
if not errorlevel 1 goto build_go

where docker >nul 2>nul
if not errorlevel 1 goto build_docker

echo.
echo ❌ 未找到 Go 或 Docker，任选其一安装后重试：
echo    Go     : https://golang.google.cn/dl/   （或 go.dev/dl）
echo    Docker : https://docs.docker.com/desktop/
echo.
echo    安装后若不想改 PATH，可在 local-env.bat 里设置 GOROOT。
exit /b 1

:build_go
echo [构建] 使用本机 Go 编译...
go version
rem ---- 从 server.txt 读取服务器地址，编译进 exe 作为内置默认值 ----
set "DEFSRV="
if exist "server.txt" for /f "usebackq delims=" %%i in ("server.txt") do if not defined DEFSRV set "DEFSRV=%%i"
if not defined DEFSRV set "DEFSRV=127.0.0.1:8995"
echo [构建] 内置服务器地址: %DEFSRV%
set "GOOS=windows"
set "GOARCH=amd64"
set "CGO_ENABLED=0"
go mod tidy
if errorlevel 1 goto fail
go build -trimpath -ldflags="-s -w -X main.builtinServer=%DEFSRV%" -o "%OUT%" .
if errorlevel 1 goto fail
goto ok

:build_docker
echo [构建] 本机无 Go，改用 Docker 交叉编译...
docker run --rm -v "%cd%":/src -w /src ^
  -e GOPROXY=%GOPROXY% -e GOOS=windows -e GOARCH=amd64 -e CGO_ENABLED=0 ^
  golang:1.22-alpine sh -c "go mod tidy && go build -trimpath -ldflags='-s -w -X main.builtinServer=%DEFSRV%' -o %OUT% ."
if errorlevel 1 goto fail
goto ok

:ok
echo.
for %%f in ("%OUT%") do echo ✅ 编译完成：%%~ff  (%%~zf 字节)
exit /b 0

:fail
echo.
echo ❌ 编译失败，请查看上方错误信息
exit /b 1
