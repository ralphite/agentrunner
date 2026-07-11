#!/usr/bin/env bash
# QA-44 real-API gate (INC-42, G29): runtime permission-mode switch
# default↔acceptEdits via `ar mode`. Journal/filesystem red lines
# (independent of model wording):
#   1. default: an edit ASKS (approval_requested) and the file stays
#      unchanged while pending; the ask is denied to close the turn.
#   2. `ar mode <sid> acceptEdits` at idle journals mode_changed{cause:user}.
#   3. the SAME kind of edit now lands with NO new approval_requested.
#   4. protected .mcp.json still asks under acceptEdits (INC-18 unbent).
#   5. `ar mode <sid> bypass` is rejected client-side, no journal change.
#   6. `ar mode <sid> default` journals mode_changed; the next edit asks again.
#
# Private daemon on an isolated runtime root (new-binary daemon-path rule).
# Session copied to shared store + export archived. The daemon is left
# RUNNING (print DPID) so the webui leg can drive the same stack; kill it
# manually when done.
#
#   qa/run-qa44.sh <ar-binary>
set -euo pipefail

AR="${1:?usage: run-qa44.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "QA-44: GEMINI_API_KEY unset" >&2; exit 2; }

stamp="$(date +%Y%m%d-%H%M%S)"
work="${QA44_WORK:-/tmp/qa44-$stamp}"
mkdir -p "$work/ws"
export XDG_DATA_HOME="$work/xdg"

echo "alpha beta" > "$work/ws/normal.txt"
printf '{"mcpServers":{}}\n' > "$work/ws/.mcp.json"
mcp_before="$(cat "$work/ws/.mcp.json")"

cat > "$work/spec.yaml" <<'YAML'
name: qa44
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 512 }
system_prompt: |
  你是一个文件编辑助手。当用户要求改文件时，用 edit_file 工具做精确替换。
  一次只改一个文件。如果某次编辑被拒绝(denied)，不要重试，直接简短确认收到。
tools: [read_file, edit_file]
permissions: []   # no rules — mode default governs (default: edit → ask)
YAML

"$AR" daemon >"$work/daemon.log" 2>&1 &
DPID=$!
sock="$XDG_DATA_HOME/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$sock" ] && break; sleep 0.1; done
[ -S "$sock" ] || { echo "QA-44: daemon socket never appeared" >&2; cat "$work/daemon.log" >&2; exit 1; }

count() { local n; n="$(grep -c "\"type\":\"$2\"" "$1" 2>/dev/null)" || n=0; printf '%s' "${n:-0}"; }
wait_count() { # file type want [ticks]
  local f="$1" t="$2" want="$3" ticks="${4:-400}" i n
  for i in $(seq 1 "$ticks"); do n="$(count "$f" "$t")"; [ "$n" -ge "$want" ] && return 0; sleep 0.2; done
  echo "QA-44: timeout waiting for $want $t (have ${n:-0})" >&2; return 1
}
last_apid() { { grep -o '"approval_id":"[^"]*"' "$1" 2>/dev/null || true; } | tail -1 | cut -d'"' -f4; }

# --- 1. default: edit asks, file held, deny to close the turn ---
sid="$("$AR" new --detach --workspace "$work/ws" "$work/spec.yaml" \
  "把 normal.txt 里的 'alpha' 改成 'gamma'。")"
[ -n "$sid" ] || { echo "QA-44: no session id" >&2; exit 1; }
echo "QA-44 session: $sid  (work=$work daemon pid=$DPID)"
ev="$XDG_DATA_HOME/agentrunner/sessions/$sid/events.jsonl"

wait_count "$ev" approval_requested 1
grep -q "alpha beta" "$work/ws/normal.txt" || { echo "QA-44 FAIL(1): file changed while ask pending" >&2; exit 1; }
echo "PASS(1): default-mode edit asked; file held back while pending"
"$AR" approve "$sid" "$(last_apid "$ev")" deny >/dev/null
wait_count "$ev" assistant_message 2   # turn 1 closes after the denial renders

# --- 2. switch to acceptEdits at idle ---
"$AR" mode "$sid" acceptEdits >/dev/null
wait_count "$ev" mode_changed 1
grep -q '"to":"acceptEdits"' "$ev" && grep -q '"cause":"user"' "$ev" \
  || { echo "QA-44 FAIL(2): mode_changed payload wrong" >&2; exit 1; }
echo "PASS(2): ar mode → mode_changed{to:acceptEdits, cause:user} journaled"

# --- 3. same edit now lands without a new ask ---
asks_before="$(count "$ev" approval_requested)"
"$AR" send --detach "$sid" "现在把 normal.txt 里的 'beta' 改成 'delta'。" >/dev/null
for i in $(seq 1 400); do grep -q "delta" "$work/ws/normal.txt" && break; sleep 0.2; done
grep -q "delta" "$work/ws/normal.txt" || { echo "QA-44 FAIL(3): acceptEdits edit did not land" >&2; exit 1; }
[ "$(count "$ev" approval_requested)" -eq "$asks_before" ] \
  || { echo "QA-44 FAIL(3): acceptEdits edit still asked" >&2; exit 1; }
echo "PASS(3): acceptEdits edit landed with no new ask"
wait_count "$ev" assistant_message 3

# --- 4. protected path still asks under acceptEdits ---
"$AR" send --detach "$sid" "编辑 .mcp.json：把 mcpServers 的 {} 改成 {\"demo\":{}}。" >/dev/null
wait_count "$ev" approval_requested "$((asks_before + 1))"
[ "$(cat "$work/ws/.mcp.json")" = "$mcp_before" ] \
  || { echo "QA-44 FAIL(4): protected file modified while pending" >&2; exit 1; }
echo "PASS(4): protected .mcp.json still asked under acceptEdits; file held"
"$AR" approve "$sid" "$(last_apid "$ev")" deny >/dev/null
wait_count "$ev" assistant_message 4

# --- 5. bypass is not a runtime target ---
if "$AR" mode "$sid" bypass >/dev/null 2>&1; then
  echo "QA-44 FAIL(5): ar mode bypass was accepted" >&2; exit 1
fi
[ "$(count "$ev" mode_changed)" -eq 1 ] || { echo "QA-44 FAIL(5): journal changed" >&2; exit 1; }
echo "PASS(5): bypass rejected client-side; no journal change"

# --- 6. switch back to default: the ask returns ---
"$AR" mode "$sid" default >/dev/null
wait_count "$ev" mode_changed 2
grep -q '"to":"default"' "$ev" || { echo "QA-44 FAIL(6): no default mode_changed" >&2; exit 1; }
asks_now="$(count "$ev" approval_requested)"
"$AR" send --detach "$sid" "把 normal.txt 里的 'delta' 改成 'omega'。" >/dev/null
wait_count "$ev" approval_requested "$((asks_now + 1))"
grep -q "delta" "$work/ws/normal.txt" || { echo "QA-44 FAIL(6): file changed while pending" >&2; exit 1; }
echo "PASS(6): back on default, the same edit asks again; file held"
"$AR" approve "$sid" "$(last_apid "$ev")" deny >/dev/null
wait_count "$ev" assistant_message 5

# --- retention: copy to shared store + export archive ---
shared="$HOME/.local/share/agentrunner/sessions"
mkdir -p "$shared"; cp -R "$XDG_DATA_HOME/agentrunner/sessions/$sid" "$shared/$sid"
run_dir="$here/runs/$(date +%Y-%m-%d)-QA44"
mkdir -p "$run_dir"
"$AR" events "$sid" > "$run_dir/events.export.jsonl" 2>/dev/null || cp "$ev" "$run_dir/events.export.jsonl"
{
  echo "QA-44 runtime mode switch — $(date)"
  echo "session: $sid"
  echo "work: $work (daemon pid $DPID left running for the webui leg)"
  echo "red lines: 6/6 PASS"
} > "$run_dir/notes.md"
echo "QA-44: all six red lines green. session kept in $shared; export archived at $run_dir"
echo "QA-44: daemon still running (pid $DPID, XDG_DATA_HOME=$XDG_DATA_HOME) for the webui leg"
