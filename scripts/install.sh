#!/bin/sh
# 今日热榜 CLI 安装脚本（macOS / Linux）
#
#   curl -fsSL https://rebang.today/cli/install.sh | sh
#
# 环境变量：
#   REBANG_INSTALL_DIR  安装目录，默认 ~/.local/bin
#   REBANG_CDN          下载地址，默认 https://rebang.today/cli
#   REBANG_VERSION      指定版本（如 0.1.0），默认最新版
set -eu

CDN="${REBANG_CDN:-https://rebang.today/cli}"
CDN="${CDN%/}"
INSTALL_DIR="${REBANG_INSTALL_DIR:-$HOME/.local/bin}"

say() { printf '%s\n' "$*" >&2; }
die() { say "错误：$*"; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || die "需要 $1"; }

fetch() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL --retry 2 -A "rebang-install" "$1"
	elif command -v wget >/dev/null 2>&1; then
		wget -qO- -U "rebang-install" "$1"
	else
		die "需要 curl 或 wget"
	fi
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		die "需要 sha256sum 或 shasum"
	fi
}

detect_platform() {
	os="$(uname -s)"
	case "$os" in
	Darwin) os=darwin ;;
	Linux) os=linux ;;
	*) die "不支持的系统：$os（Windows 请使用 install.ps1）" ;;
	esac
	arch="$(uname -m)"
	case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) die "不支持的架构：$arch" ;;
	esac
	# Rosetta 下的 shell 会报告 x86_64，优先安装原生 arm64 版本。
	if [ "$os" = darwin ] && [ "$arch" = amd64 ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" = 1 ]; then
		arch=arm64
	fi
	echo "$os/$arch"
}

# 从 manifest.json 中读取指定平台的字段，不依赖 jq。
manifest_field() {
	printf '%s' "$1" | tr -d '\n\r' | sed 's/},/}\
/g' | grep "\"$2\"" | sed -n "s/.*\"$3\": *\"\\([^\"]*\\)\".*/\\1/p" | head -n 1
}

main() {
	need tar
	need uname
	platform="$(detect_platform)"
	os="${platform%/*}"
	arch="${platform#*/}"

	if [ -n "${REBANG_VERSION:-}" ]; then
		ver="${REBANG_VERSION#v}"
		name="rebang_${ver}_${os}_${arch}.tar.gz"
		url="$CDN/v$ver/$name"
		want="$(fetch "$CDN/v$ver/checksums.txt" | awk -v n="$name" '$2 == n {print $1}')"
	else
		manifest="$(fetch "$CDN/manifest.json")" || die "无法获取版本信息：$CDN/manifest.json"
		ver="$(printf '%s' "$manifest" | sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' | head -n 1)"
		url="$(manifest_field "$manifest" "$platform" url)"
		want="$(manifest_field "$manifest" "$platform" sha256)"
	fi
	[ -n "$url" ] || die "没有 $platform 平台的安装包"
	[ -n "$want" ] || die "缺少 $platform 安装包的 sha256"

	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp"' EXIT INT TERM
	say "正在下载 rebang $ver（$platform）…"
	fetch "$url" > "$tmp/rebang.tar.gz" || die "下载失败：$url"
	got="$(sha256 "$tmp/rebang.tar.gz")"
	[ "$got" = "$want" ] || die "sha256 校验失败，已中止安装"
	tar -xzf "$tmp/rebang.tar.gz" -C "$tmp" rebang || die "解压失败"

	mkdir -p "$INSTALL_DIR"
	chmod 755 "$tmp/rebang"
	mv -f "$tmp/rebang" "$INSTALL_DIR/rebang"
	say "已安装到 $INSTALL_DIR/rebang"

	case ":$PATH:" in
	*":$INSTALL_DIR:"*) ;;
	*)
		say ""
		say "$INSTALL_DIR 不在 PATH 中，请把下面这行加入 shell 配置文件（如 ~/.zshrc 或 ~/.bashrc）："
		say "  export PATH=\"$INSTALL_DIR:\$PATH\""
		;;
	esac
	say ""
	say "开始使用："
	say "  rebang               进入交互界面"
	say "  rebang auth login    登录"
	say "  rebang skill install 安装 Agent Skill"
	"$INSTALL_DIR/rebang" version >/dev/null 2>&1 || say "提示：程序已安装，但无法运行，请检查系统与架构。"
}

main "$@"
