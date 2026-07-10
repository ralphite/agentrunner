#!/usr/bin/env bash
# Single gate for "a step is done": formatting, vet, lint, tests.
# Live-tagged tests (-tags live) are intentionally excluded; they run at
# stage-exit human checkpoints (PLAN 0.5 rule 4).
set -euo pipefail
cd "$(dirname "$0")/.."

# gofmt only what the repo tracks — runtime/ workspaces hold agent-written
# .go files from QA sessions that must never fail the gate.
unformatted=$(git ls-files '*.go' | xargs gofmt -l)
if [[ -n "$unformatted" ]]; then
  echo "gofmt: files need formatting:" >&2
  echo "$unformatted" >&2
  exit 1
fi

go vet ./...
golangci-lint run
go test ./...

(cd webui && go vet ./... && go test ./...)

(
  cd webui/frontend
  node_major=$(node -p 'Number(process.versions.node.split(".")[0])')
  if (( node_major < 18 )); then
    echo "webui: Node.js 18+ required (found $(node --version))" >&2
    exit 1
  fi
  if [[ ! -d node_modules ]]; then
    npm ci
  fi
  npm run test
  npm run build
)

echo "check.sh: all green"
