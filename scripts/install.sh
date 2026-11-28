#!/usr/bin/env bash
# gox 一键安装脚本（Linux / macOS）
#
# 用法：
#   curl -fsSL https://raw.githubusercontent.com/robertpanvip/gox-core/main/scripts/install.sh | bash
#   GOX_VERSION=v0.1.0 GOX_INSTALL_DIR=/usr/local/bin bash install.sh
#
# 环境变量：
#   GOX_VERSION      指定版本（如 v0.1.0），默认取最新 Release
#   GOX_INSTALL_DIR  安装目录，默认 $HOME/.local/bin
#   GOX_REPO         GitHub 仓库（默认 robertpanvip/gox-core，便于 fork/离线镜像）
set -euo pipefail

REPO="${GOX_REPO:-robertpanvip/gox-core}"
API_BASE="https://api.github.com/repos/${REPO}"
RELEASE_BASE="https://github.com/${REPO}/releases"
INSTALL_DIR="${GOX_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${GOX_VERSION:-}"

die() { printf 'gox-install: %s\n' "$*" >&2; exit 1; }
info() { printf 'gox-install: %s\n' "$*"; }

need() { command -v "$1" >/dev/null 2>&1 || die "缺少必需命令: $1（请先安装）"; }
need curl
need tar

# --- 1. 平台探测 ---
os=$(uname -s)
arch=$(uname -m)
case "$os" in
  Linux)  os_part=linux ;;
  Darwin) os_part=darwin ;;
  *) die "不支持的操作系统: $os（Windows 请用 install.ps1）" ;;
esac
case "$arch" in
  x86_64|amd64) arch_part=amd64 ;;
  arm64|aarch64) arch_part=arm64 ;;
  *) die "不支持的 CPU 架构: $arch" ;;
esac
target="${os_part}_${arch_part}"
info "目标平台: $target"

# --- 2. 解析版本 ---
if [ -z "$VERSION" ]; then
  info "查询最新 Release ..."
  VERSION=$(curl -fsSL "${API_BASE}/releases/latest" \
    | grep -m1 '"tag_name"' \
    | sed -E 's/.*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/')
  [ -n "$VERSION" ] || die "无法解析最新版本号（检查网络或仓库是否有 Release）"
fi
info "版本: $VERSION"

archive="gox_${VERSION}_${target}.tar.gz"
url="${RELEASE_BASE}/download/${VERSION}/${archive}"
checksums_url="${RELEASE_BASE}/download/${VERSION}/checksums.txt"

# --- 3. 下载到临时目录（失败自动清理） ---
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
info "下载 $archive ..."
curl -fSL --retry 3 -o "$tmp/$archive" "$url" || die "下载失败: $url"
curl -fSL --retry 3 -o "$tmp/checksums.txt" "$checksums_url" || die "下载校验和失败: $checksums_url"

# --- 4. 校验 SHA256 ---
sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    die "缺少 sha256sum 或 shasum，无法校验下载内容"
  fi
}
expected=$(grep -E "[[:space:]]${archive}\$" "$tmp/checksums.txt" | awk '{print $1}')
[ -n "$expected" ] || die "checksums.txt 中没有 $archive 的条目"
actual=$(sha256 "$tmp/$archive")
[ "$actual" = "$expected" ] || die "SHA256 校验失败（期望 $expected，实际 $actual）"
info "SHA256 校验通过"

# --- 5. 解压并安装 ---
mkdir -p "$tmp/unpack"
tar -xzf "$tmp/$archive" -C "$tmp/unpack" || die "解压失败"
bin="$tmp/unpack/gox"
[ -f "$bin" ] || die "压缩包内未找到 gox 二进制"
chmod +x "$bin"
mkdir -p "$INSTALL_DIR"
install -m 0755 "$bin" "$INSTALL_DIR/gox" || die "安装失败（检查目录权限: $INSTALL_DIR）"

info "已安装: $INSTALL_DIR/gox"
"$INSTALL_DIR/gox" version 2>/dev/null || true

case ":$PATH:" in
  *":$INSTALL_DIR:"*)
    info "现在可以直接运行: gox init myapp && cd myapp && gox run main.gx"
    ;;
  *)
    info "请把 $INSTALL_DIR 加入 PATH："
    info "  export PATH=\"\$PATH:$INSTALL_DIR\""
    ;;
esac
