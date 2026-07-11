#!/usr/bin/env bash
# QA-56 real gate (INC-53, HANDA #24): project overlay + launcher. The
# security red lines of the new OS-exec surface (POST /api/open) are the
# focus — actual `open -a` is NOT exercised here (it launches real apps; the
# argv construction is covered by the Go twins TestLaunchArgvWhitelist /
# TestOpenLaunchesKnownWorkspace). Real HTTP against a live arwebui:
#   1. POST /api/open with an off-whitelist app → 400, launch NOT attempted;
#   2. POST /api/open with an unknown/arbitrary workspace → 400 (fail-closed);
#   3. POST /api/projects overlay (display name) persists to webui-meta.json
#      and GET /api/projects surfaces it.
#
#   qa/run-qa56.sh <arwebui-binary> <ar-binary>
set -euo pipefail

WEBUI="${1:?usage: run-qa56.sh <arwebui-binary> <ar-binary>}"
AR="${2:?usage: run-qa56.sh <arwebui-binary> <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"

port=8799
rt="$(mktemp -d /tmp/qa54-XXXX)"
"$WEBUI" --ar "$AR" --addr "127.0.0.1:$port" --no-daemon --runtime "$rt" >"$rt/webui.log" 2>&1 &
WPID=$!
trap 'kill "$WPID" 2>/dev/null || true' EXIT
for i in $(seq 1 100); do curl -sf "http://127.0.0.1:$port/api/health" >/dev/null 2>&1 && break; sleep 0.1; done
curl -sf "http://127.0.0.1:$port/api/health" >/dev/null || { echo "QA-56: arwebui never came up" >&2; cat "$rt/webui.log" >&2; exit 1; }

# A real, known workspace from the shared store (fail-closed validation checks
# membership in `ar sessions list --json`).
WS="$("$AR" sessions list --json 2>/dev/null | python3 -c "
import sys,json,os
rows=json.load(sys.stdin)
for r in rows:
    w=r.get('workspace')
    if w and os.path.isdir(w): print(w); break
")"
[ -n "$WS" ] || { echo "QA-56: no real workspace in store" >&2; exit 1; }
echo "known workspace: $WS"

code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }

# Red line 1: off-whitelist app rejected.
c1="$(code -X POST "http://127.0.0.1:$port/api/open" -H 'Content-Type: application/json' \
  -d "{\"app\":\"/bin/sh\",\"workspace\":\"$WS\"}")"
[ "$c1" = "400" ] || { echo "QA-56 FAIL: off-whitelist app → $c1, want 400" >&2; exit 1; }
echo "PASS(1): off-whitelist app rejected (400)"

# Red line 2: unknown workspace rejected (fail-closed).
c2="$(code -X POST "http://127.0.0.1:$port/api/open" -H 'Content-Type: application/json' \
  -d '{"app":"finder","workspace":"/etc"}')"
[ "$c2" = "400" ] || { echo "QA-56 FAIL: unknown workspace → $c2, want 400" >&2; exit 1; }
echo "PASS(2): arbitrary/unknown workspace rejected (400, fail-closed)"

# Red line 3: overlay display name persists + surfaces.
c3="$(code -X POST "http://127.0.0.1:$port/api/projects" -H 'Content-Type: application/json' \
  -d "{\"workspace\":\"$WS\",\"displayName\":\"QA54 Project\"}")"
[ "$c3" = "200" ] || { echo "QA-56 FAIL: project overlay update → $c3, want 200" >&2; exit 1; }
got="$(curl -s "http://127.0.0.1:$port/api/projects" | python3 -c "
import sys,json
d=json.load(sys.stdin)
rows=d if isinstance(d,list) else d.get('projects',d)
import json as j
print(j.dumps(rows,ensure_ascii=False))
")"
echo "$got" | grep -q "QA54 Project" || { echo "QA-56 FAIL: overlay name not surfaced: $got" >&2; exit 1; }
meta="$rt/webui-meta.json"
[ -f "$meta" ] && grep -q "QA54 Project" "$meta" \
  || { echo "QA-56 FAIL: overlay not persisted to webui-meta.json" >&2; ls "$rt" >&2; exit 1; }
echo "PASS(3): overlay display name persisted to webui-meta.json + surfaced"

run_dir="$here/runs/$(date +%Y-%m-%d)-INC53"
mkdir -p "$run_dir"
cp "$meta" "$run_dir/webui-meta.json" 2>/dev/null || true
{ echo "QA-56 project launcher — $(date)"; echo "reject off-whitelist app + unknown workspace; overlay persist OK"; \
  echo "note: real open -a not exercised (side-effectful); argv covered by Go twins"; } > "$run_dir/notes.md"
echo "QA-56: all green (security reject paths + overlay persist). archived at $run_dir"
