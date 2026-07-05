#!/usr/bin/env bash
# QA-08 real-API gate (v2 M5): the crash matrix, three states × kill -9 of
# the runtime → restart → `ar send` revives the SAME session (no special
# resume action). (a) parked at idle; (b) mid-turn with a bash in flight —
# in-doubt renders interrupted-by-crash, never re-runs; (c) background
# sub-agents in flight — each settles with a receipt from its own journal.
#
#   qa/run-qa08.sh <ar-binary>
set -euo pipefail
QA=QA-08
AR="${1:?usage: run-qa08.sh <ar-binary>}"
. "$(cd "$(dirname "$0")" && pwd)/lib.sh"

qa_setup cobra
cat > "$WS/qa_slow.sh" <<'SH'
#!/usr/bin/env bash
sleep 25
echo SLOW_DONE
SH
chmod +x "$WS/qa_slow.sh"
cat > "$WORK/base.yaml" <<'YAML'
name: dev
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 2048 }
system_prompt: |
  你是一个严谨的编码助手。简洁作答,严格按用户指令行动,基于会话已有
  上下文继续。要求并行调查时用 spawn_agent(background=true, agent=worker)。
tools: [read_file, bash, spawn_agent]
agents: [worker]
permissions:
  - { action: allow }
YAML
cat > "$WORK/worker.yaml" <<'YAML'
name: worker
description: 调查一个主题;如任务要求先运行命令(如 sleep)就先用 bash 运行
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 1024 }
system_prompt: 你是调查员。按任务行动,一句话汇报。
tools: [read_file, bash]
permissions:
  - { action: allow }
YAML
qa_daemon

crash_restart() { # kill -9 the daemon, restart it on the same socket
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
sid="$(qa_new "记住一个暗号:蓝色风筝。先待命。")"
SDIR="$(qa_sdir "$sid")"
echo "session $sid"
qa_wait_idle "$SDIR" 1

# ---- (a) parked at idle × kill -9 → send revives, context intact ----
crash_restart
"$AR" send "$sid" "接着刚才的话题:暗号是什么?原样说出来。" >/dev/null
qa_wait_idle "$SDIR" 2
grep '"type":"assistant_message"' "$SDIR/events.jsonl" | tail -1 | grep -q "蓝色风筝" || {
  echo "$QA: FAIL (a) revived answer lost pre-crash context" >&2; fail=1; }

# ---- (b) bash in flight × kill -9 → in-doubt renders, never re-runs;
#      a message QUEUED before the crash survives it (铁律 2) ----
"$AR" send "$sid" "运行 ./qa_slow.sh 并告诉我输出。只运行这一个命令。" >/dev/null
for i in $(seq 1 200); do
  grep -q '"name":"bash"' "$SDIR/events.jsonl" 2>/dev/null && break; sleep 0.2
done
# Queue a message DURING the turn — it is acked durable, then the crash
# eats the process before the consume-side journal sees it.
"$AR" send "$sid" "插一个问题:暗语是'青鸟',请原样重复一遍这个暗语。" >/dev/null
crash_restart
"$AR" send "$sid" "刚才那个命令什么状态?不要重新运行任何命令,只根据你已有的信息回答。" >/dev/null
qa_wait_idle "$SDIR" 3
grep '"type":"activity_failed"' "$SDIR/events.jsonl" | grep -q "interrupted by crash" || {
  echo "$QA: FAIL (b) no interrupted-by-crash rendering" >&2; fail=1; }
# 铁律 2 (无输入丢失): the queued message survived the crash — journaled
# after the restart (mailbox replay), exactly once, and CONSUMED (a turn
# started after it, so it entered the model's context). Whether the model
# repeats the codeword verbatim is wording, not gated (§0.1).
qn="$(grep '"type":"input_received"' "$SDIR/events.jsonl" | grep -c "青鸟")" || qn=0
[ "$qn" = 1 ] || { echo "$QA: FAIL (b) queued input journaled $qn times, want exactly 1" >&2; fail=1; }
q_line="$(grep -n '"type":"input_received"' "$SDIR/events.jsonl" | grep "青鸟" | head -1 | cut -d: -f1)"
t_after="$(awk -v n="$q_line" 'NR>n && /"type":"turn_started"/{print NR; exit}' "$SDIR/events.jsonl")"
[ -n "$q_line" ] && [ -n "$t_after" ] || {
  echo "$QA: FAIL (b) queued input never consumed by a turn" >&2; fail=1; }
grep '"type":"assistant_message"' "$SDIR/events.jsonl" | grep -q "青鸟" ||   echo "$QA: WARN (b) model did not echo the codeword (wording, not gated)" >&2
# The RUNTIME never re-runs on doubt: every qa_slow start is model-initiated
# (paired 1:1 with an assistant tool_call). A model that re-runs after
# SEEING the crash result is exercising legitimate agency — allowed.
bash_starts="$(grep '"type":"activity_started"' "$SDIR/events.jsonl" | grep -c qa_slow)" || bash_starts=0
model_calls="$(grep '"type":"assistant_message"' "$SDIR/events.jsonl" | grep -c qa_slow)" || model_calls=0
[ "$bash_starts" -le "$model_calls" ] || {
  echo "$QA: FAIL (b) qa_slow starts=$bash_starts > model calls=$model_calls — runtime re-ran on doubt" >&2; fail=1; }

# ---- (c) sub-agents in flight × kill -9 → receipts from child journals ----
"$AR" send "$sid" "并行启动 2 个 worker 子 agent(background=true):P=先运行 'sleep 20' 再调查 README,Q=先运行 'sleep 20' 再调查 LICENSE。启动后等结果。" >/dev/null
for i in $(seq 1 300); do
  [ "$(count_type spawn_requested "$SDIR/events.jsonl")" -ge 2 ] && break; sleep 0.2
done
[ "$(count_type spawn_requested "$SDIR/events.jsonl")" -ge 2 ] || {
  echo "$QA: FAIL (c) sub-agents never launched" >&2; cat "$WORK/daemon.log" >&2; exit 1; }
sleep 1
crash_restart
"$AR" send "$sid" "两个子 agent 现在什么状态?" >/dev/null
for i in $(seq 1 300); do
  [ "$(count_type subagent_completed "$SDIR/events.jsonl")" -ge 2 ] && break; sleep 0.2
done
subs="$(count_type subagent_completed "$SDIR/events.jsonl")"
[ "$subs" -ge 2 ] || { echo "$QA: FAIL (c) subagent receipts = $subs, want 2" >&2; fail=1; }

# Let the revived session finish consuming the receipts before closing.
for i in $(seq 1 300); do
  tail -c 400 "$SDIR/events.jsonl" | grep -q '"type":"waiting_entered"' && break; sleep 0.2
done
"$AR" close "$sid" >/dev/null 2>&1 || true
for i in $(seq 1 300); do tail -c 200 "$SDIR/events.jsonl" | grep -q '"type":"run_ended"' && break; sleep 0.2; done
ends="$(count_type run_ended "$SDIR/events.jsonl")"
[ "$ends" = 1 ] || { echo "$QA: FAIL run_ended = $ends, want exactly 1 (three crashes, one session)" >&2; fail=1; }

if [ "$fail" = 0 ]; then
  echo "$QA PASS: 3-state crash matrix — idle park, in-flight bash, in-flight sub-agents all revived by ar send"
fi
exit $fail
