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

echo "版本：$VER"
mkdir -p dist

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
	if gh release view "$VER" >/dev/null 2>&1; then
		echo "Release $VER 已存在，改为上传 / 覆盖资产"
		gh release upload "$VER" dist/* --clobber
	else
		gh release create "$VER" dist/* \
			--title "$VER" \
			--notes "DeepAICode2API $VER（详见 docs/changes.md）"
	fi
	echo "已发布：$(gh release view "$VER" --json url -q .url)"
fi
