#!/bin/sh
# 与 CI 一致的本地检查：格式 + vet + 前端语法 + 测试。
set -eu

cd "$(dirname "$0")/.."

files=$(gofmt -l . || true)
if [ -n "$files" ]; then
	echo "以下文件未格式化："
	echo "$files"
	echo "运行 make fmt 修复。"
	exit 1
fi

go vet ./...

# 前端是零构建的 ES module：没有打包器帮忙做语法校验，这里用 node 的
# module 解析器逐文件过一遍。`--input-type=module` 是必须的：不带它时
# node --check 按 CJS 解析，会放过 ESM 的顶层 return 之类错误。
#
# 未安装 node 时跳过（不阻塞只有 Go 工具链的环境）；CI 的 ubuntu-latest 自带 node。
if command -v node >/dev/null 2>&1; then
	for f in internal/server/web/js/*.js internal/server/web/js/render/*.js; do
		if ! node --input-type=module --check <"$f"; then
			echo "JS 语法错误：$f"
			exit 1
		fi
	done
	node scripts/console-smoke.mjs >/dev/null
	echo "前端语法与冒烟：通过"
else
	echo "未安装 node，跳过前端检查"
fi

go test -count=1 ./...
