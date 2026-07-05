#!/usr/bin/env bash
# QA-09 real-API gate (v2 收口, C7): the full orchestration — DESIGN §8's
# seven steps in one real session. Image input → 3 parallel sub-agents →
# first-back-first-served → steer (cancel B, spawn D) → all receipts →
# continued chat → kill -9 → revived chat writes SUMMARY.md via write_file.
#
#   v2/qa/run-qa09.sh <ar-binary>
set -euo pipefail
QA=QA-09
AR="${1:?usage: run-qa09.sh <ar-binary>}"
. "$(cd "$(dirname "$0")" && pwd)/lib.sh"

PNG="$qa_here/fixtures/build-error.png"
[ -f "$PNG" ] || { echo "$QA: fixture $PNG missing" >&2; exit 2; }

qa_setup gin
cat > "$WORK/base.yaml" <<'YAML'
name: lead
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 2048 }
system_prompt: |
  你是编排者,简洁作答。用户要求并行调查时,用 spawn_agent
  (background=true, agent=worker)严格按要求的数量和分工启动子 agent,
  启动后不要等待。用户要求取消某个子 agent 时用 task_kill(handle)。
  需要写文件时用 write_file。
tools: [read_file, write_file, spawn_agent]
agents: [worker]
permissions:
  - { action: allow }
YAML
cat > "$WORK/worker.yaml" <<'YAML'
name: worker
description: 调查一个指定主题并用一两句话汇报
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 1024 }
system_prompt: 你是调查员。读相关文件,用一两句话汇报发现。
tools: [read_file]
permissions:
  - { action: allow }
YAML
qa_daemon

crash_restart() {
  kill -9 "$DPID" 2>/dev/null || true
  sleep 0.5
  "$AR" daemon >>"$WORK/daemon.log" 2>&1 &
  DPID=$!
  local i; for i in $(seq 1 100); do
    "$AR" sessions list >/dev/null 2>&1 && return 0
    sleep 0.1
  done
  echo "$QA: daemon did not come back" >&2; exit 1
}

fail=0
sid="$(qa_new "你好,我等下会给你一张 CI 截图并让你编排调查。先待命。")"
SDIR="$(qa_sdir "$sid")"
echo "session $sid"
qa_wait_idle "$SDIR" 1

# Step 1: image + exactly 3 parallel sub-agents.
"$AR" send --image "$PNG" "$sid" \
  "结合截图,启动恰好 3 个 worker 子 agent(background=true)分别调查:A=截图中报错可能涉及的机制,B=binding 目录的职责,C=middleware 是什么。启动后等它们的结果。" >/dev/null
for i in $(seq 1 300); do
  [ "$(count_type spawn_requested "$SDIR/events.jsonl")" -ge 3 ] && break; sleep 0.2
done
spawns="$(count_type spawn_requested "$SDIR/events.jsonl")"
[ "$spawns" -ge 3 ] || { echo "$QA: FAIL step1 spawns = $spawns, want 3" >&2; cat "$WORK/daemon.log" >&2; exit 1; }

# Steps 3: steer while children are in flight — cancel B, spawn D.
"$AR" ps "$sid" || true
"$AR" send "$sid" "B(binding 目录)不用查了,立刻用 task_kill 取消它;另启动一个新的 worker 子 agent D(background=true)调查 gin 的路由树实现。" >/dev/null
for i in $(seq 1 300); do
  [ "$(count_type spawn_requested "$SDIR/events.jsonl")" -ge 4 ] && break; sleep 0.2
done
spawns="$(count_type spawn_requested "$SDIR/events.jsonl")"
[ "$spawns" -ge 4 ] || { echo "$QA: FAIL step3 spawns = $spawns, want 4 (D spawned)" >&2; fail=1; }
grep '"type":"activity_started"' "$SDIR/events.jsonl" | grep -q '"name":"task_kill"' || {
  echo "$QA: FAIL step3 no task_kill call — B was not cancelled by the model" >&2; fail=1; }

# Step 4: all receipts land (4 spawned, all settle one way or another).
for i in $(seq 1 400); do
  [ "$(count_type subagent_completed "$SDIR/events.jsonl")" -ge 4 ] && break; sleep 0.3
done
subs="$(count_type subagent_completed "$SDIR/events.jsonl")"
[ "$subs" -ge 4 ] || { echo "$QA: FAIL step4 receipts = $subs, want 4" >&2; fail=1; }

# Step 5: continued chat over this session's own history.
for i in $(seq 1 300); do tail -c 400 "$SDIR/events.jsonl" | grep -q '"type":"waiting_entered"' && break; sleep 0.2; done
"$AR" send "$sid" "简单说:哪个子 agent 的结果最先回来?你是怎么处理的?" >/dev/null
parks_before="$(count_type waiting_entered "$SDIR/events.jsonl")"
for i in $(seq 1 300); do
  [ "$(count_type waiting_entered "$SDIR/events.jsonl")" -gt "$parks_before" ] && break; sleep 0.2
done

# Step 6: kill -9 → revive → write SUMMARY.md via write_file.
crash_restart
"$AR" send "$sid" "把这次调查的最终结论(包括 B 被取消这件事)用 write_file 写进仓库根的 SUMMARY.md。" >/dev/null
for i in $(seq 1 300); do [ -s "$WS/SUMMARY.md" ] && break; sleep 0.3; done

# ---- Assertions (the journal tells the whole seven-step story) ----
[ -s "$WS/SUMMARY.md" ] || { echo "$QA: FAIL SUMMARY.md missing after revival" >&2; fail=1; }
grep '"type":"activity_started"' "$SDIR/events.jsonl" | grep '"name":"write_file"' | grep -q SUMMARY || {
  echo "$QA: FAIL SUMMARY.md not written via write_file" >&2; fail=1; }
# The image entered as a CAS ref (C9 in the chain).
grep '"type":"input_received"' "$SDIR/events.jsonl" | grep '"images"' | grep -q '"ref":"sha256-' || {
  echo "$QA: FAIL image input has no CAS ref" >&2; fail=1; }
# One session, one terminal (none yet — close now and verify).
"$AR" close "$sid" >/dev/null 2>&1 || true
for i in $(seq 1 300); do tail -c 200 "$SDIR/events.jsonl" | grep -q '"type":"run_ended"' && break; sleep 0.2; done
ends="$(count_type run_ended "$SDIR/events.jsonl")"
[ "$ends" = 1 ] || { echo "$QA: FAIL run_ended = $ends, want exactly 1" >&2; fail=1; }
# The cancelled child settled as a non-completed receipt.
if ! grep '"type":"subagent_completed"' "$SDIR/events.jsonl" | grep -qv '"reason":"completed"'; then
  echo "$QA: FAIL no cancelled/erred receipt — was B really killed?" >&2; fail=1
fi

if [ "$fail" = 0 ]; then
  echo "$QA PASS: seven-step orchestration in one session — image, 3+1 spawns, steer-kill, receipts, chat, crash, revived write_file"
fi
exit $fail
