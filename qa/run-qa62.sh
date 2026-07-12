#!/usr/bin/env bash
# QA-62 real-API gate (INC-62, G35, UJ-08/18): standing approval — the
# "always allow" answer takes effect IN THE SAME SESSION, and spawn_agent
# is written back for the next session. Red lines against live Gemini:
#   1. session 1 (mode default, no rules) asks for the FIRST spawn_agent;
#   2. `ar approve --always` once → the remaining spawns of the SAME
#      session proceed with ZERO further approval_requested, and at least
#      one effect_resolved carries the "standing approval" verdict;
#   3. the USER config gains a tool-level spawn_agent allow rule;
#   4. a FRESH session 2 spawns WITHOUT asking (the written rule took).
#
# Private daemon + private XDG_DATA_HOME/XDG_CONFIG_HOME: the scenario
# WRITES the user config, so it must not touch the developer's real
# ~/.config (same isolation rationale as QA-26); on a CI runner this is
# scratch anyway. Journals archived to qa/runs/<date>-QA62.
#
#   qa/run-qa62.sh <ar-binary>
set -euo pipefail

AR="${1:?usage: run-qa62.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "QA-62: GEMINI_API_KEY unset" >&2; exit 2; }

stamp="$(date +%Y%m%d-%H%M%S)"
work="${QA62_WORK:-/tmp/qa62-$stamp}"
mkdir -p "$work/ws"
export XDG_DATA_HOME="$work/xdg"
export XDG_CONFIG_HOME="$work/xdgc"
usercfg="$work/xdgc/agentrunner/settings.yaml"
# No cleanup trap (QA rule): data kept; only the private daemon is stopped.

# Mode default, NO permissions block: spawn_agent is execute-class, so the
# mode default alone makes it ask — exactly the G35 user scenario.
cat > "$work/spec.yaml" <<'YAML'
name: qa62
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 2048 }
system_prompt: |
  你是编排者。当用户要求启动子 agent 时,用 spawn_agent 工具、参数
  background 设为 true、agent 设为 "worker",严格按要求的数量启动。
  逐个启动,每次一个调用。全部启动后不要等待,直接说"已全部启动"。
tools: [spawn_agent]
agents: [worker]
YAML
cat > "$work/worker.yaml" <<'YAML'
name: worker
description: 把指定的一个数字原样复述一句话
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 256 }
system_prompt: 你收到一个数字,用一句话原样复述它,不做任何其他事。
tools: [read_file]
YAML

"$AR" daemon >"$work/daemon.log" 2>&1 &
DPID=$!
sock="$XDG_DATA_HOME/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$sock" ] && break; sleep 0.1; done
[ -S "$sock" ] || { echo "QA-62: daemon socket never appeared" >&2; cat "$work/daemon.log" >&2; exit 1; }
trap 'kill "$DPID" 2>/dev/null || true' EXIT

apid_of() { { grep -o '"approval_id":"[^"]*"' "$1" 2>/dev/null || true; } | head -1 | cut -d'"' -f4; }
count() { grep -c "\"type\":\"$2\"" "$1" 2>/dev/null || echo 0; }

# --- Session 1: three spawns, ONE question ---
sid="$("$AR" new --detach --workspace "$work/ws" "$work/spec.yaml" \
  "启动恰好 3 个 worker 子 agent(background=true),任务分别是:复述数字 1;复述数字 2;复述数字 3。")"
[ -n "$sid" ] || { echo "QA-62: no session id" >&2; exit 1; }
echo "QA-62 session 1: $sid"
sdir="$XDG_DATA_HOME/agentrunner/sessions/$sid"

apid=""
for i in $(seq 1 600); do
  apid="$(apid_of "$sdir/events.jsonl")"
  [ -n "$apid" ] && break
  sleep 0.2
done
[ -n "$apid" ] || { echo "QA-62 FAIL: no approval_requested for the first spawn" >&2; exit 1; }
echo "PASS(1): session 1 asked for the first spawn (id $apid)"

"$AR" approve "$sid" "$apid" approve --always >/dev/null
echo "approved --always; waiting for the remaining spawns…"

spawns=0
for i in $(seq 1 900); do
  spawns="$(count "$sdir/events.jsonl" spawn_requested)"
  [ "$spawns" -ge 3 ] && break
  sleep 0.2
done
asks="$(count "$sdir/events.jsonl" approval_requested)"
if [ "$spawns" -lt 3 ]; then
  echo "QA-62 FAIL: only $spawns spawn_requested (want 3); asks=$asks" >&2
  tail -5 "$sdir/events.jsonl" >&2; exit 1
fi
if [ "$asks" -ne 1 ]; then
  echo "QA-62 FAIL: $asks approval_requested for 3 spawns (want exactly 1 — G35 regression)" >&2
  exit 1
fi
echo "PASS(2): 3 spawns, exactly 1 ask — standing approval silenced the rest"

if ! grep -q "standing approval" "$sdir/events.jsonl"; then
  echo "QA-62 FAIL: no effect_resolved carries the standing-approval verdict" >&2
  exit 1
fi
echo "PASS(3): audit trail names the standing answer in effect_resolved"

for i in $(seq 1 50); do [ -f "$usercfg" ] && grep -q "spawn_agent" "$usercfg" && break; sleep 0.2; done
if [ ! -f "$usercfg" ] || ! grep -q "spawn_agent" "$usercfg"; then
  echo "QA-62 FAIL: user config did not gain the spawn_agent allow rule" >&2
  echo "--- $usercfg ---" >&2; cat "$usercfg" 2>/dev/null >&2; exit 1
fi
echo "PASS(4): user config gained a tool-level spawn_agent allow rule"

# --- Session 2 (FRESH): spawning must NOT ask ---
sid2="$("$AR" new --detach --workspace "$work/ws" "$work/spec.yaml" \
  "启动恰好 1 个 worker 子 agent(background=true),任务是:复述数字 7。")"
[ -n "$sid2" ] || { echo "QA-62: no session 2 id" >&2; exit 1; }
echo "QA-62 session 2: $sid2"
sdir2="$XDG_DATA_HOME/agentrunner/sessions/$sid2"
for i in $(seq 1 600); do
  [ "$(count "$sdir2/events.jsonl" spawn_requested)" -ge 1 ] && break
  sleep 0.2
done
if [ "$(count "$sdir2/events.jsonl" spawn_requested)" -lt 1 ]; then
  echo "QA-62 FAIL: session 2 never spawned" >&2
  tail -5 "$sdir2/events.jsonl" >&2; exit 1
fi
if [ "$(count "$sdir2/events.jsonl" approval_requested)" -ne 0 ]; then
  echo "QA-62 FAIL: fresh session still asked despite the written rule" >&2
  exit 1
fi
echo "PASS(5): fresh session spawned WITHOUT asking (next-session rule took)"

run_dir="$here/runs/$(date +%Y-%m-%d)-QA62"
mkdir -p "$run_dir"
"$AR" events "$sid" > "$run_dir/session1.events.jsonl" 2>/dev/null || cp "$sdir/events.jsonl" "$run_dir/session1.events.jsonl"
"$AR" events "$sid2" > "$run_dir/session2.events.jsonl" 2>/dev/null || cp "$sdir2/events.jsonl" "$run_dir/session2.events.jsonl"
cp "$usercfg" "$run_dir/user-settings.yaml" 2>/dev/null || true
{ echo "QA-62 standing approval — $(date)"; echo "session1: $sid  session2: $sid2"; echo "approval id: $apid"; echo "spawns=$spawns asks=$asks"; } > "$run_dir/notes.md"
echo "QA-62: all green. export archived at $run_dir"
