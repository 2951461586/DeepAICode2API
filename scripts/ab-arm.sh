#!/usr/bin/env bash
# 起网关（状态副本）→ 建临时 Key → 压测 → 停网关。一轮 = 一个二进制 / 一组参数。
#
# 用法: CONC=28 TOTAL=56 ./scripts/ab-arm.sh ./bin/deepaicode2api
# 说明: 与 docs/performance.md §14.5 的同名片段一致，改动时两边同步。
set -euo pipefail
BIN=${1:?用法: ab-arm.sh <binary>}
WORK=$(mktemp -d)
cp deepaicode2api-data.json "$WORK/state.json"    # 副本：不动真实号池
"$BIN" -data "$WORK/state.json" -port 10087 >"$WORK/gw.log" 2>&1 &
GW=$!
trap 'kill $GW 2>/dev/null || true; rm -rf "$WORK"' EXIT
until curl -sf -m1 -o /dev/null http://127.0.0.1:10087/healthz; do sleep 0.5; done
KEY=$(curl -sS -X POST -H 'Content-Type: application/json' -d '{"name":"ab","group":""}' \
  http://127.0.0.1:10087/admin/api/keys | sed -n 's/.*"key":"\([^"]*\)".*/\1/p')
# 只测 max_retries 时，在这里插一句（不需要重编）：
# curl -sS -X POST -H 'Content-Type: application/json' -d '{"max_retries":16}' \
#   http://127.0.0.1:10087/admin/api/settings >/dev/null
BASE=http://127.0.0.1:10087 KEY="$KEY" CONC=${CONC:-28} TOTAL=${TOTAL:-56} node scripts/ab-load.mjs
