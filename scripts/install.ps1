# gox 一键安装脚本（Windows PowerShell）
#
# 用法：
#   irm https://raw.githubusercontent.com/robertpanvip/gox-core/main/scripts/install.ps1 | iex
#   $env:GOX_VERSION="v0.1.0"; $env:GOX_INSTALL_DIR="$HOME\bin"; .\install.ps1
#
# 环境变量：
#   GOX_VERSION      指定版本（如 v0.1.0），默认取最新 Release
#   GOX_INSTALL_DIR  安装目录，默认 $HOME\.local\bin
#   GOX_REPO         GitHub 仓库（默认 robertpanvip/gox-core）

$ErrorActionPreference = 'Stop'
$Repo = if ($env:GOX_REPO) { $env:GOX_REPO } else { 'robertpanvip/gox-core' }
$ApiBase = "https://api.github.com/repos/$Repo"
$ReleaseBase = "https://github.com/$Repo/releases"
$InstallDir = if ($env:GOX_INSTALL_DIR) { $env:GOX_INSTALL_DIR } else { Join-Path $HOME '.local\bin' }
$Version = $env:GOX_VERSION

function Info($msg) { Write-Host "gox-install: $msg" }
function Fail($msg) { throw "gox-install: $msg" }

# 1. 平台探测（gox 目前只发 amd64 的 Windows 包）
$arch = $env:PROCESSOR_ARCHITECTURE
if ($arch -ne 'AMD64') { Fail "不支持的 CPU 架构: $arch（当前仅提供 windows_amd64）" }
$target = 'windows_amd64'
Info "目标平台: $target"

# 2. 解析版本
if (-not $Version) {
    Info '查询最新 Release ...'
    try {
        $release = Invoke-RestMethod -Uri "$ApiBase/releases/latest" -Headers @{ 'User-Agent' = 'gox-installer' }
        $Version = $release.tag_name
    } catch { Fail "查询最新版本失败: $_" }
}
if (-not $Version) { Fail '无法解析最新版本号' }
Info "版本: $Version"

$archive = "gox_${Version}_${target}.zip"
$url = "$ReleaseBase/download/$Version/$archive"
$checksumsUrl = "$ReleaseBase/download/$Version/checksums.txt"

# 3. 下载到临时目录
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("gox-install-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tmp -Force | Out-Null
try {
    $archivePath = Join-Path $tmp $archive
    $checksumsPath = Join-Path $tmp 'checksums.txt'
    Info "下载 $archive ..."
    try {
        Invoke-WebRequest -Uri $url -OutFile $archivePath -Headers @{ 'User-Agent' = 'gox-installer' }
        Invoke-WebRequest -Uri $checksumsUrl -OutFile $checksumsPath -Headers @{ 'User-Agent' = 'gox-installer' }
    } catch { Fail "下载失败: $_" }

    # 4. 校验 SHA256
    $expected = (Get-Content $checksumsPath | Where-Object { $_ -match "\s+$([regex]::Escape($archive))$" } | ForEach-Object { ($_ -split '\s+')[0] } | Select-Object -First 1)
    if (-not $expected) { Fail "checksums.txt 中没有 $archive 的条目" }
    $actual = (Get-FileHash -Algorithm SHA256 -Path $archivePath).Hash.ToLowerInvariant()
    if ($actual -ne $expected.ToLowerInvariant()) { Fail "SHA256 校验失败（期望 $expected，实际 $actual）" }
    Info 'SHA256 校验通过'

    # 5. 解压并安装
    $unpack = Join-Path $tmp 'unpack'
    Expand-Archive -Path $archivePath -DestinationPath $unpack -Force
    $bin = Join-Path $unpack 'gox.exe'
    if (-not (Test-Path $bin)) { Fail '压缩包内未找到 gox.exe' }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item -Path $bin -Destination (Join-Path $InstallDir 'gox.exe') -Force
    Info "已安装: $(Join-Path $InstallDir 'gox.exe')"

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ($userPath -notlike "*$InstallDir*") {
        Info "请把 $InstallDir 加入 PATH（当前会话可临时执行）："
        Info "  `$env:Path += ';$InstallDir'"
    } else {
        Info '现在可以直接运行: gox init myapp; cd myapp; gox run main.gx'
    }
} finally {
    Remove-Item -Path $tmp -Recurse -Force -ErrorAction SilentlyContinue
}
