# 下载便携版 mpv 到 windows\mpv\
# 用法：
#   powershell -ExecutionPolicy Bypass -File fetch-mpv.ps1
#   powershell -ExecutionPolicy Bypass -File fetch-mpv.ps1 -Url "https://.../mpv.7z"   # 手动指定
param(
    [string]$Dest = (Join-Path $PSScriptRoot "mpv"),
    [string]$Url = ""
)

$ErrorActionPreference = "Stop"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

function Get-File($u, $out) {
    Write-Host "  下载: $u"
    Invoke-WebRequest -Uri $u -OutFile $out -UseBasicParsing -TimeoutSec 600
}

if (-not $Url) {
    Write-Host "查询 mpv 最新版本（GitHub）..."
    $rel = Invoke-RestMethod -Uri "https://api.github.com/repos/zhongfly/mpv-winbuild/releases/latest" `
        -Headers @{ "User-Agent" = "syncwatch" }
    $asset = $rel.assets | Where-Object { $_.name -like "*x86_64*.7z" } | Select-Object -First 1
    if (-not $asset) { throw "release 中未找到 x86_64 的 7z 包，请用 -Url 手动指定下载地址" }
    $Url = $asset.browser_download_url
    Write-Host "  版本: $($rel.tag_name)"
}

$tmp = Join-Path $env:TEMP "syncwatch-mpv"
if (Test-Path $tmp) { Remove-Item $tmp -Recurse -Force }
New-Item -ItemType Directory -Path $tmp | Out-Null

$archive = Join-Path $tmp "mpv.7z"
Get-File $Url $archive

$7zr = Join-Path $tmp "7zr.exe"
if (-not (Test-Path $7zr)) { Get-File "https://www.7-zip.org/a/7zr.exe" $7zr }

Write-Host "解压到 $Dest ..."
if (Test-Path $Dest) { Remove-Item $Dest -Recurse -Force }
New-Item -ItemType Directory -Path $Dest | Out-Null
& $7zr x $archive "-o$Dest" -y | Out-Null

$exe = Get-ChildItem $Dest -Filter "mpv.exe" -Recurse | Select-Object -First 1
if (-not $exe) { throw "解压完成但未找到 mpv.exe，请检查压缩包结构" }

Remove-Item $tmp -Recurse -Force
Write-Host ""
Write-Host "OK  mpv 就绪: $($exe.FullName)"
