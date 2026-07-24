#!/usr/bin/env bash
# Mandatory frontend gate for INC-99 work. Keep separate from check.sh while
# the repository-wide frontend legs remain paused by explicit user decision.
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
frontend="$repo_root/webui/frontend"
skip_install=false
if [[ "${1:-}" == "--skip-install" ]]; then
  skip_install=true
elif [[ $# -gt 0 ]]; then
  echo "usage: $0 [--skip-install]" >&2
  exit 2
fi

node_ok="$(node -p 'const [a,b]=process.versions.node.split(".").map(Number); Number((a===20&&b>=19)||(a===22&&b>=12)||a>22)' 2>/dev/null || echo 0)"
if [[ "$node_ok" != "1" ]]; then
  echo "check-webui: Node.js ^20.19 or >=22.12 required (found $(node --version 2>/dev/null || echo missing))" >&2
  echo "check-webui: run from a shell using the repository .nvmrc" >&2
  exit 1
fi

cd "$frontend"
if [[ "$skip_install" != "true" ]]; then
  npm ci
fi

npm run baseline:storybook:check
npm run test
npm run build
npm run build-storybook
npm run lint:storybook
npm run test:storybook
npm run test:visual

if rg -q "mockServiceWorker|msw-storybook-addon|ApprovalCard\\.stories|storybook" dist; then
  echo "check-webui: production bundle contains Storybook/MSW development assets" >&2
  exit 1
fi
if [[ ! -f storybook-static/mockServiceWorker.js ]]; then
  echo "check-webui: Storybook static build is missing mockServiceWorker.js" >&2
  exit 1
fi

echo "check-webui: all green"
