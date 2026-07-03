#!/usr/bin/env bash
# Single gate for "a step is done": formatting, vet, lint, tests.
# Live-tagged tests (-tags live) are intentionally excluded; they run at
# stage-exit human checkpoints (PLAN 0.5 rule 4).
set -euo pipefail
cd "$(dirname "$0")/.."

unformatted=$(gofmt -l .)
if [[ -n "$unformatted" ]]; then
  echo "gofmt: files need formatting:" >&2
  echo "$unformatted" >&2
  exit 1
fi

go vet ./...
golangci-lint run
go test ./...

echo "check.sh: all green"
