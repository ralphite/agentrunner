#!/usr/bin/env bash
# QA-33 real-API gate (INC-26 #91, re-scoped by PLAN 5.7): structured output
# via the SPEC's output_schema on a provider WITHOUT native structured output
# (anthropic) — `ar new` engages the internal validate-and-retry fallback
# automatically (the retired --json-schema flag is gone; the spec is the
# single entry). Red lines:
#   1. `ar new` exits 0 (a conforming reply was obtained);
#   2. stdout is a single JSON value that MATCHES the schema (name:string,
#      lines:integer, no extras) — validated independently by python;
#   3. the value is plausible (name mentions the file; lines is the real count).
#
# The fallback orchestration is CLIENT-side (validate + re-send), so any
# daemon works; we still use a private daemon on an isolated root running THIS
# binary for a controlled ANTHROPIC_API_KEY + fresh build, then copy the
# session into the shared store for visibility (mirrors QA-32).
#
#   qa/run-qa33.sh <ar-binary>
set -euo pipefail
QA=QA-33
AR="${1:?usage: run-qa33.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"

[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
if [ -z "${ANTHROPIC_API_KEY:-}" ]; then
  main_root="$(cd "$here/.." && dirname "$(git rev-parse --git-common-dir)")"
  [ -f "$main_root/.env" ] && { set -a; . "$main_root/.env"; set +a; }
fi
[ -n "${ANTHROPIC_API_KEY:-}" ] || { echo "$QA: ANTHROPIC_API_KEY unset" >&2; exit 2; }

stamp="$(date +%Y%m%d-%H%M%S)"
work="${QA33_WORK:-/tmp/qa33-$stamp}"
mkdir -p "$work/ws"
export XDG_DATA_HOME="$work/xdg"

# A workspace file with a KNOWN line count (7 lines).
cat > "$work/ws/sample.txt" <<'TXT'
alpha
bravo
charlie
delta
echo
foxtrot
golf
TXT
WANT_LINES=7

# Schema lives IN the spec (PLAN 5.7 single entry); provider anthropic has
# no native structured output, so the client fallback must engage.
cat > "$work/spec.yaml" <<'YAML'
name: qa33
model: { provider: anthropic, id: claude-haiku-4-5-20251001, max_tokens: 512 }
system_prompt: |
  你可以用 read_file 读取工作区文件。按用户要求返回结果。
tools: [read_file]
permissions:
  - { action: allow }
output_schema:
  type: object
  properties:
    name: { type: string }
    lines: { type: integer }
  required: [name, lines]
  additionalProperties: false
YAML

# Private daemon on the isolated root, running THIS binary.
"$AR" daemon >"$work/daemon.log" 2>&1 &
DPID=$!
sock="$XDG_DATA_HOME/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$sock" ] && break; sleep 0.1; done
[ -S "$sock" ] || { echo "$QA: daemon socket never appeared" >&2; cat "$work/daemon.log" >&2; exit 1; }
trap 'kill "$DPID" 2>/dev/null || true' EXIT

# Foreground run — the spec's output_schema alone must trigger the fallback.
set +e
"$AR" new --workspace "$work/ws" "$work/spec.yaml" \
  "读取 sample.txt,数出它有多少行,然后以 JSON 返回一个对象:name 为文件名字符串、lines 为行数整数。" \
  >"$work/out.json" 2>"$work/run.err"
code=$?
set -e
echo "$QA exit code: $code"
echo "$QA stdout:"; cat "$work/out.json"
echo "$QA stderr tail:"; tail -3 "$work/run.err" 2>/dev/null || true

fail=0
note() { echo "$QA: $*"; }

# Red line 1: the run succeeded (a conforming reply was obtained).
if [ "$code" -eq 0 ]; then note "PASS  ar new (spec output_schema fallback) exited 0"; else note "FAIL  exit=$code (no conforming reply)"; fail=1; fi

# Red line 2: stdout is a JSON value matching the schema (independent check).
py="$(command -v python3 || command -v python || true)"
if [ -n "$py" ]; then
  if "$py" - "$work/out.json" "$WANT_LINES" <<'PY'
import json, sys
raw = open(sys.argv[1]).read().strip()
want = int(sys.argv[2])
obj = json.loads(raw)                     # must parse
assert isinstance(obj, dict), "not an object"
assert set(obj.keys()) == {"name", "lines"}, f"keys={set(obj.keys())}"
assert isinstance(obj["name"], str) and obj["name"], "name not a non-empty string"
assert isinstance(obj["lines"], int) and not isinstance(obj["lines"], bool), "lines not an integer"
print(f"parsed name={obj['name']!r} lines={obj['lines']}")
# Red line 3 (plausibility): name mentions the file and lines is the real count.
assert "sample" in obj["name"], f"name does not mention the file: {obj['name']!r}"
assert obj["lines"] == want, f"lines={obj['lines']} want={want}"
PY
  then note "PASS  stdout matches schema AND is plausible (name~sample, lines=$WANT_LINES)"; else note "FAIL  stdout failed schema/plausibility check"; fail=1; fi
else
  note "NOTE  python not found; skipped independent JSON check"
fi

# Copy the session into the shared store + archive.
sid="$(ls -t "$XDG_DATA_HOME/agentrunner/sessions" 2>/dev/null | head -1)"
if [ -n "${sid:-}" ]; then
  echo "$QA session: $sid"
  shared="$HOME/.local/share/agentrunner/sessions"
  mkdir -p "$shared"; cp -R "$XDG_DATA_HOME/agentrunner/sessions/$sid" "$shared/$sid" 2>/dev/null || true
  run_dir="$here/runs/$(date +%Y-%m-%d)-QA33"
  mkdir -p "$run_dir"
  "$AR" events "$sid" > "$run_dir/events.export.jsonl" 2>/dev/null || cp "$XDG_DATA_HOME/agentrunner/sessions/$sid/events.jsonl" "$run_dir/events.export.jsonl" 2>/dev/null || true
  cp "$work/out.json" "$run_dir/structured-output.json" 2>/dev/null || true
  { echo "$QA structured output (spec output_schema fallback) — $(date)"; echo "session: $sid"; echo "workspace: $work/ws"; } > "$run_dir/notes.md"
fi

if [ "$fail" -eq 0 ]; then note "all green. session copied to shared store; export archived at ${run_dir:-<none>}"; else note "one or more red lines FAILED (kept at $work)"; exit 1; fi
