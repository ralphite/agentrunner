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
scripts/test-capture-codex-ui.sh

# 前端预检与两条前端腿暂时停跑(2026-07-18 用户指示:npm/vitest/build
# 是墙钟大头且偶发网络挂起)。改动前端时手跑:
#   cd webui/frontend && npm ci && npm run test && npm run build
#   cd webui && go vet ./... && go test ./...
# node_ok=$(node -p 'const [a,b]=process.versions.node.split(".").map(Number); Number((a===20&&b>=19)||(a===22&&b>=12)||a>22)')
# if (( ! node_ok )); then
#   echo "webui: Node.js ^20.19 or >=22.12 required (found $(node --version))" >&2
#   exit 1
# fi
# if [[ ! -d webui/frontend/node_modules ]] \
#   || ! cmp -s webui/frontend/package-lock.json webui/frontend/node_modules/.check-lock-stamp; then
#   (cd webui/frontend && npm ci && cp package-lock.json node_modules/.check-lock-stamp)
# fi

# QA sessions write throwaway Go files under gitignored runtime/ workspaces;
# ./... walks in regardless of .gitignore and a broken demo package would
# fail the gate — scope the toolchain to the repo's real packages.
packages=$(go list ./... 2>/dev/null | grep -v "/runtime/")

# 预热共享 go build cache(串行,warm ≈0.3s)。不做这步,冷缓存下
# lint/gotest/webui 三条腿各自并发全量编译同一依赖图,在小核机器上
# 互相踩踏——首跑 50s 档能恶化到 10 分钟级。冷时慢会集中显示在这一行
# 的耗时里,而不是黑盒的并行等待。
t0=$SECONDS
# e2e 之类 test-only 包 go build 会报 "no non-test Go files",过滤掉;
# 它们的编译由 gotest 腿覆盖。
go build $(go list -f '{{if .GoFiles}}{{.ImportPath}}{{end}}' ./... 2>/dev/null | grep -v "/runtime/")
if (( SECONDS - t0 > 5 )); then
  echo "check.sh: prewarm $((SECONDS - t0))s (cold build cache — 并行腿已可复用)"
fi

# ---- 并行腿 ----
# 各腿独立:失败互不掩盖,日志各自落盘,谁红打谁的全量输出。
# go build cache / golangci cache 均为并发安全,共享无碍。
logdir=$(mktemp -d)
trap 'rm -rf "$logdir"' EXIT

pids=()
legnames=()
run_leg() { # run_leg <name> <cmd...>
  local name="$1"; shift
  # 腿内自报耗时(慢腿可见,PROCESS 一步纪律的可观测性)。
  ( s=$(date +%s); "$@"; rc=$?; echo $(( $(date +%s) - s )) > "$logdir/$name.time"; exit $rc ) \
    > "$logdir/$name.log" 2>&1 &
  pids+=("$!")
  legnames+=("$name")
}

run_leg lint golangci-lint run                      # 含 govet(standard 预设)
run_leg wiring scripts/lint-wiring.sh               # 接线审计:deadcode vs 基线(PROCESS §五)
run_leg gotest go test $packages
# run_leg fe-test bash -c 'cd webui/frontend && npm run test'   # 暂停(见上)
# run_leg webui bash -c 'cd webui/frontend && npm run build && cd .. && go vet ./... && go test ./...'  # 暂停(见上)
run_leg install scripts/test-install.sh             # 安装器孪生(离线,INC-63 gate A)

fail=0
for i in "${!pids[@]}"; do
  name="${legnames[$i]}"
  if wait "${pids[$i]}"; then
    echo "check.sh: $name ok ($(cat "$logdir/$name.time" 2>/dev/null || echo '?')s)"
  else
    fail=1
    echo "check.sh: $name FAILED — full log:" >&2
    cat "$logdir/$name.log" >&2
    # 瞬时红也要可追:红腿日志留档到 /tmp(logdir 本身随 trap 清理)。
    cp "$logdir/$name.log" "/tmp/check-last-red-$name.log" 2>/dev/null || true
  fi
done
if (( fail )); then
  echo "check.sh: RED" >&2
  exit 1
fi
echo "check.sh: all green"
