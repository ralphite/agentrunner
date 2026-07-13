#!/usr/bin/env bash
# Single gate for "a step is done": formatting, vet, lint, tests.
# Live-tagged tests (-tags live) are intentionally excluded; they run at
# stage-exit human checkpoints (PLAN 0.5 rule 4).
set -euo pipefail
cd "$(dirname "$0")/.."

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

# QA sessions write throwaway Go files under gitignored runtime/ workspaces;
# ./... walks in regardless of .gitignore and a broken demo package would
# fail the gate — scope the toolchain to the repo's real packages.
packages=$(go list ./... 2>/dev/null | grep -v "/runtime/")
go vet $packages
golangci-lint run

# 接线审计:deadcode 可达性 vs 基线——有测试调用 ≠ 已接线(PROCESS §五)。
scripts/lint-wiring.sh

go test $packages

# Build the embedded SPA before running the WebUI Go tests: embed_test calls
# staticHandler, whose production contract intentionally panics when dist is
# absent. A clean checkout has no gitignored dist directory.
(
  cd webui/frontend
  node_ok=$(node -p 'const [a,b]=process.versions.node.split(".").map(Number); Number((a===20&&b>=19)||(a===22&&b>=12)||a>22)')
  if (( ! node_ok )); then
    echo "webui: Node.js ^20.19 or >=22.12 required (found $(node --version))" >&2
    exit 1
  fi
  if [[ ! -d node_modules ]]; then
    npm ci
  fi
  npm run test
  npm run build
)

(cd webui && go vet ./... && go test ./...)

# 安装器孪生:真 install.sh 打 file:// stub 产物(离线,INC-63 gate A)。
scripts/test-install.sh

echo "check.sh: all green"
