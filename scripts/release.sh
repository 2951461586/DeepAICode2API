#!/bin/sh
# 打包发布：交叉编译各平台二进制，并按需创建 GitHub Release。
#
# 用法：
#   sh scripts/release.sh                 # 用 git describe 作为版本，只构建
#   sh scripts/release.sh v0.1.1          # 指定版本
#   sh scripts/release.sh v0.1.1 --publish  # 构建后用 gh 创建 / 更新 Release
#
# 产物在 dist/（已 gitignore）。CGO 关闭，因此都是静态单文件。
set -eu

cd "$(dirname "$0")/.."

VER=${1:-$(git describe --tags --always 2>/dev/null || echo dev)}
PUBLISH=0
if [ "${2:-}" = "--publish" ]; then
	PUBLISH=1
fi

case "$VER" in
v*) ;;
*) VER="v$VER" ;;
esac

# 版本号规范：一律 vX.X.X（三段数字），release **标题就是版本号本身**，
# 不接任何描述后缀 —— 描述写进 release notes（dist/release-notes-<ver>.md）
# 与 docs/changes.md。历史上出现过「v0.1.17 — 修复……分配代理」这种标题，
# 列表里参差不齐，也无法机器解析。
if ! printf '%s' "$VER" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
	echo "版本号必须是 vX.X.X（例：v0.1.23），实际：$VER" >&2
	exit 1
fi

echo "版本：$VER"
mkdir -p dist

# gh release upload 是 `dist/*`：不清掉上一版产物，旧版本二进制会被一起传上去
# （v0.1.3 就踩过一次）。只清理本项目自己的产物，不动 dist/ 里其它文件。
rm -f dist/deepaicode2api_* dist/checksums.txt

# os/arch 组合：桌面端与服务器最常用的五种。
for target in \
	linux/amd64 \
	linux/arm64 \
	windows/amd64 \
	darwin/amd64 \
	darwin/arm64
do
	os=${target%/*}
	arch=${target#*/}
	out="dist/deepaicode2api_${VER}_${os}_${arch}"
	[ "$os" = "windows" ] && out="$out.exe"
	echo "  构建 $os/$arch → $out"
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch \
		go build -trimpath -ldflags "-s -w -X main.version=$VER" -o "$out" .
done

# 附带校验和，便于用户核对下载物。
(
	cd dist
	sha256sum ./*deepaicode2api_* >checksums.txt 2>/dev/null ||
		shasum -a 256 ./*deepaicode2api_* >checksums.txt
)
echo "已生成 dist/checksums.txt"

if [ "$PUBLISH" -eq 1 ]; then
	if ! command -v gh >/dev/null 2>&1; then
		echo "未安装 gh，跳过发布（二进制已在 dist/）"
		exit 1
	fi
	NOTES="dist/release-notes-$VER.md"
	[ -f "$NOTES" ] || NOTES=""
	# 只上传本版资产：dist/ 里还躺着历史版本的 release notes（已 gitignore、长期累积），
	# 用 `dist/*` 会把它们一起挂到每个 Release 上（v0.1.26 之前每个 Release 都带了
	# 十几份旧 notes）。只挑本版二进制 + checksums + 本版 notes。
	ASSETS=""
	for f in dist/deepaicode2api_${VER}_* dist/checksums.txt; do
		[ -e "$f" ] && ASSETS="$ASSETS $f"
	done
	[ -n "$NOTES" ] && ASSETS="$ASSETS $NOTES"
	if gh release view "$VER" >/dev/null 2>&1; then
		echo "Release $VER 已存在：覆盖资产并统一标题"
		gh release upload "$VER" $ASSETS --clobber
		gh release edit "$VER" --title "$VER"
		if [ -n "$NOTES" ]; then
			gh release edit "$VER" --notes-file "$NOTES"
		fi
	elif [ -n "$NOTES" ]; then
		gh release create "$VER" $ASSETS --title "$VER" --notes-file "$NOTES"
	else
		gh release create "$VER" $ASSETS \
			--title "$VER" \
			--notes "DeepAICode2API $VER（详见 docs/changes.md）"
	fi
	echo "已发布：$(gh release view "$VER" --json url -q .url)"
fi
