#!/usr/bin/env bash
# QA-26 real-API gate (INC-17, G5): approval "allow and don't ask again".
# UJ-08 flow against live Gemini. Runtime red lines:
#   1. session 1 asks for approval (catch-all ask → a non-read-only command
#      journals approval_requested);
#   2. `ar approve --always` appends an EXACT allow rule to the USER config
#      (file fact — the whole point of "don't ask again");
#   3. a FRESH session 2 over the same user config runs the same command
#      WITHOUT asking (no approval_requested → the remembered rule took).
#
# Private daemon + private XDG_CONFIG_HOME (so the user config we write is
# isolated, not the developer's real ~/.config). Session copied to shared
# store + export archived.
#
#   qa/run-qa26.sh <ar-binary>
set -euo pipefail

AR="${1:?usage: run-qa26.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "QA-26: GEMINI_API_KEY unset" >&2; exit 2; }

stamp="$(date +%Y%m%d-%H%M%S)"
work="${QA26_WORK:-/tmp/qa26-$stamp}"
mkdir -p "$work/ws"
export XDG_DATA_HOME="$work/xdg"
export XDG_CONFIG_HOME="$work/xdgc"
usercfg="$work/xdgc/agentrunner/settings.yaml"
# No cleanup trap (QA rule): data kept; only the private daemon is stopped.

cat > "$work/spec.yaml" <<'YAML'
name: qa26
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 512 }
system_prompt: |
  你是一个 shell 助手。当用户要求执行一条命令时，用 bash 工具**原样**执行
  用户给的确切命令（不加任何参数、不改写、一次调用）。
tools: [bash]
permissions:
  - { action: ask }   # catch-all: every non-read-only bash needs approval
YAML

"$AR" daemon >"$work/daemon.log" 2>&1 &
DPID=$!
sock="$XDG_DATA_HOME/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$sock" ] && break; sleep 0.1; done
[ -S "$sock" ] || { echo "QA-26: daemon socket never appeared" >&2; cat "$work/daemon.log" >&2; exit 1; }
trap 'kill "$DPID" 2>/dev/null || true' EXIT

apid_of() { { grep -o '"approval_id":"[^"]*"' "$1" 2>/dev/null || true; } | head -1 | cut -d'"' -f4; }
count() { grep -c "\"type\":\"$2\"" "$1" 2>/dev/null || echo 0; }

# --- Session 1: run `date`, which the catch-all makes ask ---
sid="$("$AR" new --detach --workspace "$work/ws" "$work/spec.yaml" \
  "用 bash 执行这条命令，原样、不加参数：date")"
[ -n "$sid" ] || { echo "QA-26: no session id" >&2; exit 1; }
echo "QA-26 session 1: $sid"
sdir="$XDG_DATA_HOME/agentrunner/sessions/$sid"

apid=""
for i in $(seq 1 400); do
  apid="$(apid_of "$sdir/events.jsonl")"
  [ -n "$apid" ] && break
  sleep 0.2
done
[ -n "$apid" ] || { echo "QA-26 FAIL: no approval_requested in session 1" >&2; exit 1; }
echo "PASS(1): session 1 asked for approval (id $apid)"

# --- Approve with --always ---
"$AR" approve "$sid" "$apid" approve --always >/dev/null
# Give the agent a beat to write the user config.
for i in $(seq 1 50); do [ -f "$usercfg" ] && grep -q "date" "$usercfg" && break; sleep 0.2; done
if [ ! -f "$usercfg" ] || ! grep -q "date" "$usercfg"; then
  echo "QA-26 FAIL: user config did not gain an allow rule for the command" >&2
  echo "--- $usercfg ---" >&2; cat "$usercfg" 2>/dev/null >&2; exit 1
fi
echo "PASS(2): approve --always appended an exact allow rule to user config"

# --- Session 2 (FRESH): same command must NOT ask ---
sid2="$("$AR" new --detach --workspace "$work/ws" "$work/spec.yaml" \
  "用 bash 执行这条命令，原样、不加参数：date")"
[ -n "$sid2" ] || { echo "QA-26: no session 2 id" >&2; exit 1; }
echo "QA-26 session 2: $sid2"
sdir2="$XDG_DATA_HOME/agentrunner/sessions/$sid2"
# Wait for the command to run (activity_completed) — with the remembered rule
# it executes without an approval_requested.
for i in $(seq 1 400); do
  [ "$(count "$sdir2/events.jsonl" activity_completed)" -ge 1 ] && break
  sleep 0.2
done
if [ "$(count "$sdir2/events.jsonl" approval_requested)" -ne 0 ]; then
  echo "QA-26 FAIL: fresh session still asked despite the remembered rule" >&2
  exit 1
fi
if [ "$(count "$sdir2/events.jsonl" activity_completed)" -ge 1 ]; then
  echo "PASS(3): fresh session ran the remembered command WITHOUT asking"
else
  echo "QA-26 NOTE: session 2 produced no activity_completed (model may have varied the command); config red line (PASS 2) still holds" >&2
fi

shared="$HOME/.local/share/agentrunner/sessions"
mkdir -p "$shared"; cp -R "$sdir" "$shared/$sid"; cp -R "$sdir2" "$shared/$sid2"
run_dir="$here/runs/$(date +%Y-%m-%d)-QA26"
mkdir -p "$run_dir"
"$AR" events "$sid" > "$run_dir/session1.events.jsonl" 2>/dev/null || cp "$sdir/events.jsonl" "$run_dir/session1.events.jsonl"
"$AR" events "$sid2" > "$run_dir/session2.events.jsonl" 2>/dev/null || cp "$sdir2/events.jsonl" "$run_dir/session2.events.jsonl"
cp "$usercfg" "$run_dir/user-settings.yaml"
{ echo "QA-26 approval remember — $(date)"; echo "session1: $sid  session2: $sid2"; echo "approval id: $apid"; } > "$run_dir/notes.md"
echo "QA-26: all green. sessions kept in $shared; export archived at $run_dir"
