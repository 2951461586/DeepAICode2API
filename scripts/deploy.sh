#!/bin/sh
# 部署 / 回滚：构建 → 打不可变 tag → 起容器 → 等健康检查 → 打印摘要。
#
# 用法：
#   sh scripts/deploy.sh                  # 按 git describe 构建并部署
#   sh scripts/deploy.sh v0.1.13          # 指定版本
#   DEEPAI2API_IMAGE=deepaicode2api:local-20261122-v0.1.12-abc1234 sh scripts/deploy.sh
#                                          # 不构建，直接回滚到旧镜像
#
# 约定：
#   - 镜像 tag 形如 `<仓库名>:local-<日期>-<版本>-<短 sha>`：不可变、能对回提交；
#   - 机密放 `DEEPAI2API_ENV_FILE`（默认 /etc/deepaicode2api.env，0600），
#     `.env` 只放 compose 插值用的部署参数（VERSION / IMAGE / ADDR）；
#   - 健康检查取容器的 healthcheck 状态（镜像内 wget /healthz），
#     失败时打印最近日志并**保留旧容器**做对照（compose 已经替换时给出回滚命令）。
set -eu

cd "$(dirname "$0")/.."

TAG=${1:-$(git describe --tags --always 2>/dev/null || echo dev)}

# 版本一致性：传了 tag 就必须等于当前 HEAD。
# 2026-11-22 踩过：服务器上 `git fetch --tags` 因 TLS 失败，tag 没拉到，
# 脚本却照常构建，于是打出 `local-<日期>-v0.1.15-<上一版 sha>` 的错标镜像。
if [ -n "${1:-}" ] && git rev-parse --git-dir >/dev/null 2>&1; then
	if ! git rev-parse -q --verify "refs/tags/$1" >/dev/null; then
		echo "!! 本地没有 tag $1（先 git fetch --tags，或确认版本号）" >&2
		exit 1
	fi
	if [ "$(git rev-parse "$1^{commit}")" != "$(git rev-parse HEAD)" ]; then
		echo "!! HEAD ($(git rev-parse --short HEAD)) 与 $1 不一致，拒绝用错版本的代码打标签" >&2
		exit 1
	fi
fi
SHA=$(git rev-parse --short HEAD 2>/dev/null || echo nogit)
STAMP=$(date +%Y%m%d)
IMAGE=${DEEPAI2API_IMAGE:-deepaicode2api:local-${STAMP}-${TAG}-${SHA}}
WAIT=${DEPLOY_WAIT:-90}
ENV_FILE=${DEEPAI2API_ENV_FILE:-/etc/deepaicode2api.env}

if [ ! -f "$ENV_FILE" ]; then
	echo "缺少 $ENV_FILE：机密（账号 / 口令 / 代理 / 机器指纹）必须放这里。" >&2
	echo "模板见 .env.example；权限建议 chmod 600 且 root 所有。" >&2
	exit 1
fi

PREV=$(docker inspect deepaicode2api --format '{{.Config.Image}}' 2>/dev/null || true)

echo "== 构建 $IMAGE（VERSION=$TAG，env_file=$ENV_FILE）"
DEEPAI2API_IMAGE=$IMAGE VERSION=$TAG docker compose build --build-arg VERSION="$TAG"

echo "== 启动（不再触发二次构建）"
DEEPAI2API_IMAGE=$IMAGE VERSION=$TAG docker compose up -d --no-build

echo "== 等待健康检查（最长 ${WAIT}s）"
i=0
state=unknown
while [ "$i" -lt "$WAIT" ]; do
	state=$(docker inspect deepaicode2api --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}nohealth{{end}}' 2>/dev/null || echo missing)
	case "$state" in
	healthy) break ;;
	nohealth)
		# 旧镜像没有 healthcheck 时的兜底：直接问 /healthz。
		if docker exec deepaicode2api wget -q -O - http://127.0.0.1:10087/healthz >/dev/null 2>&1; then
			state=healthy
			break
		fi
		;;
	esac
	i=$((i + 2))
	sleep 2
done

if [ "$state" != "healthy" ]; then
	echo "!! 健康检查未通过（state=$state，等待 ${i}s）" >&2
	echo "--- 最近 40 行日志 ---" >&2
	docker logs --tail 40 deepaicode2api >&2 2>&1 || true
	if [ -n "${PREV:-}" ] && [ "$PREV" != "$IMAGE" ]; then
		echo "--- 回滚命令 ---" >&2
		echo "DEEPAI2API_IMAGE=$PREV docker compose up -d --no-build" >&2
	fi
	exit 1
fi

echo "== 健康检查通过"
docker exec deepaicode2api wget -q -O - http://127.0.0.1:10087/healthz 2>/dev/null | head -c 600 || true
echo
echo "镜像：$IMAGE"
[ -n "${PREV:-}" ] && echo "上一版：$PREV（回滚：DEEPAI2API_IMAGE=$PREV docker compose up -d --no-build）"
