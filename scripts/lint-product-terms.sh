#!/usr/bin/env bash
# Prevent the removed product entity from returning through code, UI, QA, or
# canonical live documentation. Historical archives and external reference
# captures are evidence, not product requirements, so they are intentionally
# outside this gate.
set -euo pipefail
cd "$(dirname "$0")/.."

paths=(
  internal
  cmd
  webui
  qa
  docs/JOURNEYS.md
  docs/SPEC.md
  docs/DESIGN.md
  docs/QA.md
  docs/GAPS.md
  docs/PROCESS.md
  docs/increments
)

common_globs=(
  --glob '!**/frontend/package-lock.json'
  --glob '!**/frontend/dist/**'
  --glob '!**/frontend/storybook-static/**'
  --glob '!**/frontend/playwright-report/**'
  --glob '!**/frontend/test-results/**'
  --glob '!**/runs/**'
  --glob '!**/codex-reference/**'
)

matches=""
matches+=$(rg -n -i '\btask(s)?\b|task[-_]|[-_]task' "${paths[@]}" "${common_globs[@]}" || true)
camel=$(rg -n 'Task[A-Z]|[a-z]Task|task[A-Z]|Task_|_Task' "${paths[@]}" "${common_globs[@]}" || true)
if [[ -n "$matches" && -n "$camel" ]]; then
  matches+=$'\n'
fi
matches+="$camel"

if [[ -n "$matches" ]]; then
  echo "product terminology: removed entity reintroduced:" >&2
  echo "$matches" >&2
  exit 1
fi
