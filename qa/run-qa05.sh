#!/usr/bin/env bash
# QA-05 real-API gate (v2 M3 exit): kill a running sub-agent. The parent
# launches 2 background sub-agents that run slow work; the USER kills one by
# handle (ar kill); the killed child settles canceled, the other survives,
# and the session continues. (The model-driven steer→cancel+spawn variant,
# C6, is covered deterministically by the scripted twin
# TestSteerChangesOrchestration.) Structural asserts.
#
#   qa/run-qa05.sh <ar-binary>
set -euo pipefail
QA=QA-05
AR="${1:?usage: run-qa05.sh <ar-binary>}"
. "$(cd "$(dirname "$0")" && pwd)/lib.sh"

qa_setup gin
cat > "$WORK/base.yaml" <<'YAML'
name: lead
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 2048 }
system_prompt: |
  你是编排者。用户要求并行调查时,用 spawn_agent(background=true,
  agent=worker)按要求启动子 agent,不等待,继续等结果。
tools: [read_file, spawn_agent]
agents: [worker]
permissions:
  - { action: allow }
YAML
# The worker runs a genuinely slow step so a kill can land mid-run.
cat > "$WORK/worker.yaml" <<'YAML'
name: worker
description: 调查一个主题,先运行给定的准备命令再汇报
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 1024 }
system_prompt: |
  你是调查员。如果任务要求先运行某个命令(如 sleep),用 bash 运行它,
  然后用一句话汇报。
tools: [read_file, bash]
permissions:
  - { action: allow }
YAML
qa_daemon

sid="$(qa_new "并行启动 2 个 worker 子 agent(background=true):S=先运行 'sleep 30' 再调查 render,F=直接调查 middleware。启动后等结果。")"
SDIR="$(qa_sdir "$sid")"
echo "session $sid"

# Wait for BOTH sub-agents to be launched (2 spawn_requested).
for i in $(seq 1 200); do
  [ "$(count_type spawn_requested "$SDIR/events.jsonl")" -ge 2 ] && break
  sleep 0.2
done
[ "$(count_type spawn_requested "$SDIR/events.jsonl")" -ge 2 ] || {
  echo "$QA: FAIL only $(count_type spawn_requested "$SDIR/events.jsonl") spawns" >&2; cat "$WORK/daemon.log" >&2; exit 1; }

# Find the SLOW child's handle: the spawn_requested whose prompt mentions the
# sleep, and read its call_id (= handle). Grab the child whose prompt has "sleep".
slow_handle="$(grep '"type":"spawn_requested"' "$SDIR/events.jsonl" | grep -i sleep | \
  sed -n 's/.*"call_id":"\([^"]*\)".*/\1/p' | head -1)"
[ -n "$slow_handle" ] || {
  # fall back: first spawn's call_id
  slow_handle="$(grep '"type":"spawn_requested"' "$SDIR/events.jsonl" | \
    sed -n 's/.*"call_id":"\([^"]*\)".*/\1/p' | head -1)"; }
echo "killing handle: $slow_handle"
# Observation surface (QA.md 步骤 4 前置): ar ps lists the in-flight set,
# including the handle we are about to kill.
"$AR" ps "$sid" | grep -q "$slow_handle" || {
  echo "$QA: FAIL ar ps does not list in-flight handle $slow_handle" >&2; exit 1; }

# Wait until the slow child's bash is actually running, then USER-kill it.
subdir="$SDIR/sub/${slow_handle}-a1"
for i in $(seq 1 200); do
  grep -q '"name":"bash"' "$subdir/events.jsonl" 2>/dev/null && break
  sleep 0.2
done
"$AR" kill "$sid" "$slow_handle" >/dev/null

# Wait for both children to settle.
for i in $(seq 1 300); do
  [ "$(count_type subagent_completed "$SDIR/events.jsonl")" -ge 2 ] && break
  sleep 0.3
done
"$AR" close "$sid" >/dev/null 2>&1 || true
for i in $(seq 1 100); do tail -c 200 "$SDIR/events.jsonl" | grep -q '"type":"session_closed"' && break; sleep 0.1; done

# ---- Assertions ----
fail=0
# a) The killed child settled as a cancellation (its subagent_completed reason
#    is NOT "completed"); look at the slow child's SubagentCompleted.
slow_reason="$(grep '"type":"subagent_completed"' "$SDIR/events.jsonl" | grep "\"$slow_handle\"" | \
  sed -n 's/.*"reason":"\([^"]*\)".*/\1/p' | head -1)"
case "$slow_reason" in
  completed|"") echo "$QA: FAIL killed child reason = '$slow_reason', want a cancellation" >&2; fail=1 ;;
esac
# b) The slow child's sleep never finished (no post-sleep output leaked).
#    (Best-effort: the child journal has an activity_cancelled.)
grep -q '"type":"activity_cancelled"' "$subdir/events.jsonl" 2>/dev/null || \
  echo "$QA: WARN no activity_cancelled in the killed child journal" >&2
# c) No orphan sleep process lingers.
if pgrep -f "sleep 30" >/dev/null 2>&1; then
  echo "$QA: FAIL a sleep process survived the kill" >&2; fail=1
fi
# d) The other sub-agent completed, and the session ended cleanly.
subs="$(count_type subagent_completed "$SDIR/events.jsonl")"
[ "$subs" -ge 2 ] || { echo "$QA: FAIL subagent_completed = $subs, want >= 2" >&2; fail=1; }
tail -1 "$SDIR/events.jsonl" | grep -q session_closed || { echo "$QA: FAIL session did not end" >&2; fail=1; }

if [ "$fail" = 0 ]; then
  echo "$QA PASS: user-killed a running sub-agent (reason=$slow_reason), other survived, session ended"
fi
exit $fail
