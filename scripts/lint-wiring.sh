#!/usr/bin/env bash
# 接线审计(PROCESS §五;2026-07-10 G29 复盘落地):deadcode 从两个 main
# (cmd/agentrunner 与 webui)做 whole-program 可达性,对比基线。抓"数据表/
# 事件底座写了、测试也绿、但没有任何生产调用方"的静默缺口——有测试调用
# ≠ 已接线(事故:pipeline.ValidTransition 的 default↔acceptEdits 边有表
# 有测试,入口从未接线,SPEC 却记 ✅,见 GAPS G29)。
# 基线变化必须显式:新增不可达 = 拒绝提交;已接线/已删 = 从基线移除;
# 有意的测试基建留基线并行内 # 注明理由(增行须 LOG 记档)。
set -euo pipefail
cd "$(dirname "$0")/.."

DEADCODE_VERSION=v0.48.0
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

go run "golang.org/x/tools/cmd/deadcode@${DEADCODE_VERSION}" ./cmd/... \
  | sed -E 's|:[0-9]+:[0-9]+: unreachable func: | |' >"$tmp/cur"
(cd webui && go run "golang.org/x/tools/cmd/deadcode@${DEADCODE_VERSION}" . \
  | sed -E 's|:[0-9]+:[0-9]+: unreachable func: | |' | sed 's|^|webui/|') >>"$tmp/cur"
sort -o "$tmp/cur" "$tmp/cur"

sed -E 's/[[:space:]]*#.*$//' scripts/deadcode-baseline.txt \
  | grep -v '^[[:space:]]*$' | sort >"$tmp/base"

fail=0
if comm -23 "$tmp/cur" "$tmp/base" | grep .; then
  echo "lint-wiring: 新增 main 不可达导出(上列)——要么接线/删除,要么在 scripts/deadcode-baseline.txt 登记并给理由(LOG 记档,PROCESS §五)" >&2
  fail=1
fi
if comm -13 "$tmp/cur" "$tmp/base" | grep .; then
  echo "lint-wiring: 基线过时(上列已接线或已删)——从 scripts/deadcode-baseline.txt 移除" >&2
  fail=1
fi
exit $fail
