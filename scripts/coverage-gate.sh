#!/usr/bin/env bash
# 每包最低覆盖率门禁。
#
# 为什么需要：结构治理把代码搬进新包时，测试很容易留在原地。PR-4 抽走
# internal/poolsvc（号池的全部并发原语）后，它的同包覆盖率一度只有 8.8%，
# 而 go test ./... 照样全绿 —— 覆盖率门禁是唯一能拦住这类「搬走代码、
# 留下测试」的自动检查。
#
# 阈值只加在「有并发 / 有硬契约」的包上，不追求整体数字：
#   - poolsvc：槽位、探测去重、粘性、入池、预热，全是锁与 goroutine；
#   - httpx：gzip 协商与请求体上限是对外契约（且曾被 gzip 头坑过）。
#
# 用法：scripts/coverage-gate.sh   （CI 里在 go test 之后跑）
set -euo pipefail

# 包 → 最低覆盖率（整数百分比）
declare -A MIN=(
  [deepaicode2api/internal/poolsvc]=70
  [deepaicode2api/internal/httpx]=60
)

out=$(go test -count=1 -cover ./internal/... 2>&1) || true
printf '%s\n' "$out" | grep -E '^(ok|FAIL|---)' || true

fail=0
for pkg in "${!MIN[@]}"; do
  pct=$(printf '%s\n' "$out" | awk -v p="$pkg" '
    $1=="ok" && $2==p {
      for (i = 1; i <= NF; i++) {
        if ($i == "coverage:") { gsub("%", "", $(i+1)); print $(i+1); exit }
      }
    }')
  if [ -z "${pct:-}" ]; then
    echo "::error::覆盖率门禁：读不到 $pkg 的覆盖率（该包编译失败或没有测试）"
    fail=1
    continue
  fi
  want=${MIN[$pkg]}
  if awk -v got="$pct" -v want="$want" 'BEGIN { exit !(got + 0 < want + 0) }'; then
    echo "::error::覆盖率门禁：$pkg = ${pct}% < ${want}%"
    fail=1
  else
    echo "覆盖率门禁通过：$pkg = ${pct}%（阈值 ${want}%）"
  fi
done

exit "$fail"
