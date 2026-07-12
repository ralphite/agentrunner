#!/usr/bin/env bash
# smoke-release.sh — real-launch a packaged tarball (INC-63, release CI leg).
#
# Extracts the given agentrunner-<ver>-<target>.tar.gz, then proves the
# artifact actually runs on this machine: `ar --version`, `ar init`, and a
# real `arwebui` server answering /api/health. Only makes sense for a
# tarball matching the host platform.
#
# Usage: scripts/smoke-release.sh dist/agentrunner-<ver>-linux-x86_64.tar.gz
set -euo pipefail

tarball="${1:?usage: smoke-release.sh <tarball>}"

work="$(mktemp -d)"
webui_pid=""
cleanup() {
  [[ -n "$webui_pid" ]] && kill "$webui_pid" 2>/dev/null || true
  rm -rf "$work"
}
trap cleanup EXIT

tar -xzf "$tarball" -C "$work"
[[ -x "$work/ar" && -x "$work/arwebui" ]] || { echo "smoke: tarball missing ar/arwebui" >&2; exit 1; }

echo "==> ar --version"
"$work/ar" --version

echo "==> ar init (sample spec generation)"
(cd "$work" && ./ar init >/dev/null && test -f spec.yaml)

echo "==> arwebui serves /api/health"
addr="127.0.0.1:8899"
mkdir -p "$work/runtime"
# --no-daemon: health of the web binary is what this leg proves; daemon
# lifecycles have their own acceptance coverage.
"$work/arwebui" -ar "$work/ar" -addr "$addr" -runtime "$work/runtime" -no-daemon \
  >"$work/webui.log" 2>&1 &
webui_pid=$!

for _ in $(seq 1 50); do
  if curl -fsS "http://$addr/api/health" >/dev/null 2>&1; then
    curl -fsS "http://$addr/api/health"; echo
    echo "smoke: OK"
    exit 0
  fi
  kill -0 "$webui_pid" 2>/dev/null || { echo "smoke: arwebui exited early:" >&2; cat "$work/webui.log" >&2; exit 1; }
  sleep 0.2
done
echo "smoke: /api/health never came up; log:" >&2
cat "$work/webui.log" >&2
exit 1
