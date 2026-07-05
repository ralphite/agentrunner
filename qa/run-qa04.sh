#!/usr/bin/env bash
# QA-04 real-API gate (v2 M3 exit): parallel sub-agents. The parent launches
# 3 background sub-agents in one turn (non-blocking), each investigates a
# topic and reports, and the parent consumes each result. Structural asserts.
#
#   qa/run-qa04.sh <ar-binary>
set -euo pipefail
QA=QA-04
AR="${1:?usage: run-qa04.sh <ar-binary>}"
. "$(cd "$(dirname "$0")" && pwd)/lib.sh"

qa_setup gin
# Parent orchestrator + worker sub-agent (sibling spec, resolved by name).
cat > "$WORK/base.yaml" <<'YAML'
name: lead
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 2048 }
system_prompt: |
  你是编排者。当用户要求并行调查时,用 spawn_agent 工具、参数
  background 设为 true、agent 设为 "worker",严格按要求的数量和分工
  启动子 agent。启动后不要等待,继续等它们的结果消息。
tools: [read_file, spawn_agent]
agents: [worker]
permissions:
  - { action: allow }
YAML
cat > "$WORK/worker.yaml" <<'YAML'
name: worker
description: 调查一个指定目录/主题并用一句话汇报
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 1024 }
system_prompt: 你是调查员。读相关文件,用一句话汇报你的发现。
tools: [read_file]
YAML
qa_daemon

sid="$(qa_new "并行启动恰好 3 个 worker 子 agent(background=true),分别调查:A=render 目录的职责,B=binding 目录的职责,C=middleware 是什么。全部启动后等它们返回。")"
SDIR="$(qa_sdir "$sid")"
echo "session $sid"

# Wait until 3 sub-agents have completed (or timeout).
for i in $(seq 1 400); do
  n="$(count_type subagent_completed "$SDIR/events.jsonl")"
  [ "$n" -ge 3 ] && break
  sleep 0.3
done
"$AR" close "$sid" >/dev/null 2>&1 || true
for i in $(seq 1 100); do tail -c 200 "$SDIR/events.jsonl" | grep -q '"type":"run_ended"' && break; sleep 0.1; done

# ---- Assertions ----
fail=0
spawns="$(count_type spawn_requested "$SDIR/events.jsonl")"
subs="$(count_type subagent_completed "$SDIR/events.jsonl")"
bg="$(grep -c '"background":true' "$SDIR/events.jsonl" 2>/dev/null || echo 0)"; bg="${bg%%$'\n'*}"
[ "$spawns" -ge 3 ] || { echo "$QA: FAIL spawn_requested = $spawns, want >= 3" >&2; fail=1; }
[ "$subs" -ge 3 ]   || { echo "$QA: FAIL subagent_completed = $subs, want >= 3" >&2; fail=1; }

# All spawns launched in the FIRST parent turn (non-blocking parallel): every
# spawn_requested precedes the second generation_started.
t2_line="$(grep -n '"type":"generation_started"' "$SDIR/events.jsonl" | sed -n '2p' | cut -d: -f1)"
last_spawn_line="$(grep -n '"type":"spawn_requested"' "$SDIR/events.jsonl" | tail -1 | cut -d: -f1)"
if [ -n "$t2_line" ] && [ -n "$last_spawn_line" ] && [ "$last_spawn_line" -gt "$t2_line" ]; then
  echo "$QA: WARN a spawn landed after turn 2 (parent may have spawned across turns)" >&2
fi

# Child journals exist for each spawned sub-agent.
nchild="$(ls -d "$SDIR"/sub/*/ 2>/dev/null | wc -l | tr -d ' ')"
[ "$nchild" -ge 3 ] || { echo "$QA: FAIL child journals = $nchild, want >= 3" >&2; fail=1; }

# The parent had at least one turn AFTER the spawns (consumed results).
turns="$(count_type generation_started "$SDIR/events.jsonl")"
[ "$turns" -ge 2 ] || { echo "$QA: FAIL parent turns = $turns, want >= 2 (spawn + consume)" >&2; fail=1; }

if [ "$fail" = 0 ]; then
  echo "$QA PASS: $spawns parallel sub-agents launched, $subs completed, $nchild child journals, $turns parent turns"
fi
exit $fail
