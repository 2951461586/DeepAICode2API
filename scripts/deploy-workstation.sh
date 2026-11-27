#!/bin/sh
# 在 workstation 上换镜像（发布不再传二进制）。
#
# 背景：此前线上靠 `altr ws cp` 传 4 MB 的 gzip 再 bind mount 进容器，而 guest agent
# 一天会失联好几次——每次都要抢那几十秒窗口。现在改成：
#   tag 推送 → GitHub Actions 构建并推 ghcr.io/<owner>/deepaicode2api-gw:<tag>
#   → 服务器上只跑这个脚本（docker pull + 换容器）
# 传输走服务器自己的网络，agent 只需要执行一行极短命令。
#
# 用法（在 workstation 上执行）：
#   sh /opt/deepaicode2api/start.sh                              # 用 :latest
#   DEEPAI2API_IMAGE=ghcr.io/2951461586/deepaicode2api-gw:v0.1.32 sh /opt/deepaicode2api/start.sh
# 回滚：把 DEEPAI2API_IMAGE 指回上一版 tag 即可（镜像都留在 GHCR）。
set -e

IMG=${DEEPAI2API_IMAGE:-ghcr.io/2951461586/deepaicode2api-gw:latest}
DIR=/opt/deepaicode2api

echo "image: $IMG"
docker pull "$IMG"
# 打印本次真正落地的 digest：tag 可以覆盖，digest 不能，出问题时以它为准。
docker image inspect "$IMG" --format 'digest: {{index .RepoDigests 0}}' 2>/dev/null || true

docker rm -f deepaicode2api >/dev/null 2>&1 || true
docker run -d --name deepaicode2api --restart unless-stopped \
  --env-file "$DIR/app.env" \
  -v "$DIR/data:/data" \
  -p 127.0.0.1:10087:10087 \
  "$IMG"

sleep 3
docker ps --filter name=deepaicode2api --format 'status: {{.Status}}'
curl -sS -m 8 http://127.0.0.1:10087/healthz
echo
