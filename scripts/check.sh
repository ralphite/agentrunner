#!/usr/bin/env bash
# Single gate for "a step is done": formatting, vet, lint, tests.
# Live-tagged tests (-tags live) are intentionally excluded; they run at
# stage-exit human checkpoints (PLAN 0.5 rule 4).
#
# 结构(2026-07-18 提速改造,覆盖面与串行版逐项相同):秒级前置检查串行,
# 之后六条互相独立的腿并行——串行版一次全量 8 分钟的墙钟主要耗在
# "前端 vitest+build 恒定 ~66s 不缓存"与"Go 冷缓存 lint/test 各数分钟"
# 排队上。standalone `go vet` 已删:golangci-lint 的 standard 预设本身
# 含 govet,单独跑是第二遍全量类型检查。
set -euo pipefail
cd "$(dirname "$0")/.."

# ---- 前置(秒级,串行) ----
scripts/check-go-toolchain.sh

# gofmt only what the repo tracks — runtime/ workspaces hold agent-written
# .go files from QA sessions that must never fail the gate.
unformatted=$(git ls-files '*.go' | xargs gofmt -l)
if [[ -n "$unformatted" ]]; then
  echo "gofmt: files need formatting:" >&2
  echo "$unformatted" >&2
  exit 1
fi

# 登记簿真实性:SPEC 锚判据/幻影锚/GAPS 重号(PROCESS §五,G29 复盘)。
scripts/lint-docs.sh
scripts/lint-product-terms.sh

# Node 版本与 node_modules 就绪须先于并行腿(vitest 与 build 都要)。
node_ok=$(node -p 'const [a,b]=process.versions.node.split(".").map(Number); Number((a===20&&b>=19)||(a===22&&b>=12)||a>22)')
if (( ! node_ok )); then
  echo "webui: Node.js ^20.19 or >=22.12 required (found $(node --version))" >&2
  exit 1
fi
if [[ ! -d webui/frontend/node_modules ]]; then
  (cd webui/frontend && npm ci)
fi

# QA sessions write throwaway Go files under gitignored runtime/ workspaces;
# ./... walks in regardless of .gitignore and a broken demo package would
# fail the gate — scope the toolchain to the repo's real packages.
packages=$(go list ./... 2>/dev/null | grep -v "/runtime/")

# ---- 并行腿 ----
# 各腿独立:失败互不掩盖,日志各自落盘,谁红打谁的全量输出。
# go build cache / golangci cache 均为并发安全,共享无碍。
logdir=$(mktemp -d)
trap 'rm -rf "$logdir"' EXIT

declare -A pids legnames
run_leg() { # run_leg <name> <cmd...>
  local name="$1"; shift
  ( "$@" ) > "$logdir/$name.log" 2>&1 &
  pids[$name]=$!
}

run_leg lint golangci-lint run                      # 含 govet(standard 预设)
run_leg wiring scripts/lint-wiring.sh               # 接线审计:deadcode vs 基线(PROCESS §五)
run_leg gotest go test $packages
run_leg fe-test bash -c 'cd webui/frontend && npm run test'
# webui Go tests embed the SPA — build must precede them, inside one leg.
run_leg webui bash -c 'cd webui/frontend && npm run build && cd .. && go vet ./... && go test ./...'
run_leg install scripts/test-install.sh             # 安装器孪生(离线,INC-63 gate A)

fail=0
for name in lint wiring gotest fe-test webui install; do
  if wait "${pids[$name]}"; then
    echo "check.sh: $name ok"
  else
    fail=1
    echo "check.sh: $name FAILED — full log:" >&2
    cat "$logdir/$name.log" >&2
  fi
done
if (( fail )); then
  echo "check.sh: RED" >&2
  exit 1
fi
echo "check.sh: all green"
