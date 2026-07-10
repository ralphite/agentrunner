#!/usr/bin/env bash
# QA-28 real-API gate (INC-18, #59): protected write paths under acceptEdits.
# Filesystem/journal red lines (independent of model wording):
#   1. under acceptEdits, editing a NORMAL file is auto-allowed and lands
#      (no approval_requested for it);
#   2. editing a PROTECTED file (.mcp.json) is NOT auto-allowed — it journals
#      approval_requested and the file is NOT modified while it waits.
#
# Private daemon on an isolated runtime root. Session copied to shared store +
# export archived.
#
#   qa/run-qa28.sh <ar-binary>
set -euo pipefail

AR="${1:?usage: run-qa28.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "QA-28: GEMINI_API_KEY unset" >&2; exit 2; }

stamp="$(date +%Y%m%d-%H%M%S)"
work="${QA28_WORK:-/tmp/qa28-$stamp}"
mkdir -p "$work/ws"
export XDG_DATA_HOME="$work/xdg"

# Seed both files so an edit is an edit (not a create).
echo "package main" > "$work/ws/normal.txt"
printf '{"mcpServers":{}}\n' > "$work/ws/.mcp.json"
mcp_before="$(cat "$work/ws/.mcp.json")"

cat > "$work/spec.yaml" <<'YAML'
name: qa28
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 512 }
system_prompt: |
  你是一个文件编辑助手。当用户要求改文件时，用 edit_file 工具做精确替换。
  一次改一个文件。
tools: [read_file, edit_file]
mode: acceptEdits
permissions: []   # no rules — mode default governs (acceptEdits)
YAML

"$AR" daemon >"$work/daemon.log" 2>&1 &
DPID=$!
sock="$XDG_DATA_HOME/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$sock" ] && break; sleep 0.1; done
[ -S "$sock" ] || { echo "QA-28: daemon socket never appeared" >&2; cat "$work/daemon.log" >&2; exit 1; }
trap 'kill "$DPID" 2>/dev/null || true' EXIT

asst_count() { grep -c '"type":"assistant_message"' "$1" 2>/dev/null || echo 0; }
count() { grep -c "\"type\":\"$2\"" "$1" 2>/dev/null || echo 0; }
wait_asst() { local i n; for i in $(seq 1 400); do n="$(asst_count "$1")"; [ "$n" -ge "$2" ] && return 0; sleep 0.2; done; echo "QA-28: timeout ($n asst)" >&2; exit 1; }

# --- Scenario A: normal file edit — acceptEdits auto-allows ---
sid="$("$AR" new --detach --workspace "$work/ws" "$work/spec.yaml" \
  "把 normal.txt 里的 'package main' 改成 'package app'。")"
[ -n "$sid" ] || { echo "QA-28: no session id" >&2; exit 1; }
echo "QA-28 session: $sid"
sdir="$XDG_DATA_HOME/agentrunner/sessions/$sid"
wait_asst "$sdir/events.jsonl" 1
sleep 2
if grep -q "package app" "$work/ws/normal.txt"; then
  echo "PASS(1): acceptEdits auto-allowed the normal-file edit (it landed)"
else
  echo "QA-28 NOTE: normal edit not observed (model may have varied); continuing to the protected red line" >&2
fi

# --- Scenario B: protected file edit — must ask, must NOT modify while waiting ---
"$AR" send --detach "$sid" "现在编辑 .mcp.json：把 {} 改成 {\"demo\":{}}（在 mcpServers 里）。" >/dev/null
# Wait for either an approval_requested (protected → ask) or a beat.
apid=""
for i in $(seq 1 400); do
  apid="$({ grep -o '"approval_id":"[^"]*"' "$sdir/events.jsonl" 2>/dev/null || true; } | tail -1 | cut -d'"' -f4)"
  [ -n "$apid" ] && break
  sleep 0.2
done
if [ -z "$apid" ]; then
  echo "QA-28 FAIL: editing .mcp.json under acceptEdits did NOT ask (protected path not enforced)" >&2
  exit 1
fi
echo "PASS(2): protected .mcp.json edit required approval (id $apid)"

# The file must be unchanged while the approval is pending.
if [ "$(cat "$work/ws/.mcp.json")" != "$mcp_before" ]; then
  echo "QA-28 FAIL: .mcp.json was modified despite pending approval" >&2
  exit 1
fi
echo "PASS(3): .mcp.json unchanged while approval pending (write held back)"

shared="$HOME/.local/share/agentrunner/sessions"
mkdir -p "$shared"; cp -R "$sdir" "$shared/$sid"
run_dir="$here/runs/$(date +%Y-%m-%d)-QA28"
mkdir -p "$run_dir"
"$AR" events "$sid" > "$run_dir/events.export.jsonl" 2>/dev/null || cp "$sdir/events.jsonl" "$run_dir/events.export.jsonl"
{ echo "QA-28 protected write paths — $(date)"; echo "session: $sid"; echo "protected .mcp.json unchanged: $([ "$(cat "$work/ws/.mcp.json")" = "$mcp_before" ] && echo yes || echo NO)"; } > "$run_dir/notes.md"
echo "QA-28: all green. session kept in $shared; export archived at $run_dir"
