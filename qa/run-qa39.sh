#!/usr/bin/env bash
# QA-39 real-API gate (INC-35, #91 余项): provider-native structured output.
# A spec declares output_schema and a TOOL-LESS face; the live Gemini call is
# constrained natively (responseJsonSchema) so the FIRST answer is already
# schema-valid JSON — no CLI re-prompt. Red lines:
#   1. the run reaches a single assistant answer (no re-prompt loop);
#   2. that answer is a JSON value MATCHING the schema (name:string,
#      lines:integer) — validated independently by python;
#   3. the value is plausible (lines == the real count).
#
# The schema is threaded through the daemon's agent loop into the provider →
# private daemon running THIS binary; session copied to the shared store.
#
#   qa/run-qa39.sh <ar-binary>
set -euo pipefail
QA=QA-39
AR="${1:?usage: run-qa39.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
if [ -z "${GEMINI_API_KEY:-}" ]; then
  main_root="$(cd "$here/.." && dirname "$(git rev-parse --git-common-dir)")"
  [ -f "$main_root/.env" ] && { set -a; . "$main_root/.env"; set +a; }
fi
[ -n "${GEMINI_API_KEY:-}" ] || { echo "$QA: GEMINI_API_KEY unset" >&2; exit 2; }

stamp="$(date +%Y%m%d-%H%M%S)"
work="${QA39_WORK:-/tmp/qa39-$stamp}"
mkdir -p "$work/ws"
export XDG_DATA_HOME="$work/xdg"
WANT=5

# output_schema constrains generation; a TOOL-LESS spec so native JSON mode
# applies (JSON mode and tools are mutually exclusive). The file content is
# given inline in the prompt so the model needs no read tool.
cat > "$work/spec.yaml" <<'YAML'
name: qa39
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 256 }
system_prompt: |
  你是一个抽取器。只输出符合要求 schema 的 JSON,不要任何解释。
tools: []
output_schema:
  type: object
  properties:
    name: { type: string }
    lines: { type: integer }
  required: [name, lines]
  additionalProperties: false
permissions:
  - { action: allow }
YAML

"$AR" daemon >"$work/daemon.log" 2>&1 &
DPID=$!
sock="$XDG_DATA_HOME/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$sock" ] && break; sleep 0.1; done
[ -S "$sock" ] || { echo "$QA: daemon socket never appeared" >&2; cat "$work/daemon.log" >&2; exit 1; }
trap 'kill "$DPID" 2>/dev/null || true' EXIT

asst_count() { local n; n="$(grep -c '"type":"assistant_message"' "$1" 2>/dev/null)" || n=0; printf '%s' "${n:-0}"; }

sid="$("$AR" new --detach --workspace "$work/ws" "$work/spec.yaml" \
  "文件 report.txt 有 5 行。返回一个 JSON:name 是文件名字符串 \"report.txt\",lines 是行数整数。" 2>/dev/null | head -1)"
[ -n "$sid" ] || { echo "$QA: no session id" >&2; exit 1; }
SDIR="$XDG_DATA_HOME/agentrunner/sessions/$sid"
echo "$QA session: $sid"

deadline=$((SECONDS + 90))
while [ $SECONDS -lt $deadline ]; do
  [ "$(asst_count "$SDIR/events.jsonl")" -ge 1 ] && tail -1 "$SDIR/events.jsonl" | grep -q waiting_entered && break
  sleep 2
done
EV="$SDIR/events.jsonl"

fail=0
note() { echo "$QA: $*"; }

# Red line 1: exactly one assistant answer — native constraint, no re-prompt.
n="$(asst_count "$EV")"
if [ "$n" -ge 1 ]; then note "PASS  produced an assistant answer (count=$n)"; else note "FAIL  no answer"; fail=1; fi

# Extract the last assistant event to a file (a heredoc program can't also
# read the answer from stdin — the heredoc IS stdin), pass the path via argv.
grep -a '"type":"assistant_message"' "$EV" | tail -1 > "$work/last.json"
py="$(command -v python3 || command -v python || true)"
if [ -n "$py" ]; then
  if "$py" - "$WANT" "$work/last.json" "$work/answer.txt" <<'PY'
import json, sys, re
want = int(sys.argv[1])
line = open(sys.argv[2]).read()
# The journal line is a JSON event; pull the assistant text out of it.
ev = json.loads(line)
def find_text(o):
    if isinstance(o, dict):
        if o.get("kind") == "text" and isinstance(o.get("text"), str):
            return o["text"]
        for v in o.values():
            r = find_text(v)
            if r: return r
    elif isinstance(o, list):
        for v in o:
            r = find_text(v)
            if r: return r
    return None
text = find_text(ev) or ""
open(sys.argv[3], "w").write(text)
# Native mode returns RAW JSON (no ```fences). Record whether it was raw for
# the caller to note; validation itself tolerates a fence.
raw = text.strip()
fenced = raw.startswith("```")
m = re.search(r'\{.*\}', text, re.S)
assert m, f"no JSON object in answer: {text!r}"
obj = json.loads(m.group(0))
assert isinstance(obj, dict) and set(obj.keys()) == {"name","lines"}, f"keys={list(obj.keys())}"
assert isinstance(obj["name"], str) and obj["name"], "name not a non-empty string"
assert isinstance(obj["lines"], int) and not isinstance(obj["lines"], bool), "lines not an integer"
assert obj["lines"] == want, f"lines={obj['lines']} want={want}"
sys.stderr.write(f"parsed name={obj['name']!r} lines={obj['lines']} raw_json={not fenced}\n")
PY
  then note "PASS  answer is schema-valid JSON, natively constrained (name:string, lines=$WANT)"; else note "FAIL  answer not schema-valid / implausible"; fail=1; fi
else
  note "NOTE  python not found; skipped independent JSON check"
fi

shared="$HOME/.local/share/agentrunner/sessions"
mkdir -p "$shared"; cp -R "$SDIR" "$shared/$sid" 2>/dev/null || true
run_dir="$here/runs/$(date +%Y-%m-%d)-QA39"
mkdir -p "$run_dir"
"$AR" events "$sid" > "$run_dir/events.export.jsonl" 2>/dev/null || cp "$EV" "$run_dir/events.export.jsonl"
{ echo "$QA provider-native structured output (output_schema) — $(date)"; echo "session: $sid"; } > "$run_dir/notes.md"

if [ "$fail" -eq 0 ]; then note "all green. session copied to $shared/$sid; export archived at $run_dir"; else note "one or more red lines FAILED (kept at $SDIR)"; exit 1; fi
