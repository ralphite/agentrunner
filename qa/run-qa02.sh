#!/usr/bin/env bash
# QA-02 real-API gate (v2 M2 exit): busy-time input queuing. A slow bash runs
# a turn; two messages sent DURING it must not interrupt it, must be journaled
# in order, and must be consumed at the boundary after the bash. Structural
# asserts only (QA.md §0.1).
#
#   qa/run-qa02.sh <ar-binary>
set -euo pipefail
QA=QA-02
AR="${1:?usage: run-qa02.sh <ar-binary>}"
. "$(cd "$(dirname "$0")" && pwd)/lib.sh"

qa_setup cobra
qa_spec "read_file, bash"
# A slow command the agent will run so we have a window to send during it.
cat > "$WS/qa_slow.sh" <<'SH'
#!/usr/bin/env bash
sleep 6
echo SLOW_DONE
SH
chmod +x "$WS/qa_slow.sh"
qa_daemon

sid="$(qa_new "运行 ./qa_slow.sh 并把它的输出原样告诉我。只运行这一个命令。")"
SDIR="$(qa_sdir "$sid")"
echo "session $sid"

# Wait until the bash activity has STARTED (a tool call is in flight), then
# send two messages during the run.
started=0
for i in $(seq 1 100); do
  if grep -q '"name":"bash"' "$SDIR/events.jsonl" 2>/dev/null && \
     ! grep -q SLOW_DONE "$SDIR/events.jsonl" 2>/dev/null; then started=1; break; fi
  sleep 0.1
done
[ "$started" = 1 ] || { echo "$QA: bash never started" >&2; cat "$WORK/daemon.log" >&2; exit 1; }

# Two type-ahead messages during the slow bash.
"$AR" send "$sid" "顺便数一下仓库里有多少个 .go 文件,用 bash。" >/dev/null
"$AR" send "$sid" "全部用中文回答。" >/dev/null
send_done_marker="$(count_type input_received "$SDIR/events.jsonl")"

# The bash must run to completion (not cancelled by the sends).
for i in $(seq 1 150); do grep -q SLOW_DONE "$SDIR/events.jsonl" 2>/dev/null && break; sleep 0.2; done
grep -q SLOW_DONE "$SDIR/events.jsonl" || { echo "$QA: FAIL bash did not finish (interrupted by send?)" >&2; exit 1; }

# The two sends produce follow-up turns; wait for them to settle, then close.
qa_wait_turns "$SDIR" 3
"$AR" close "$sid" >/dev/null
for i in $(seq 1 100); do tail -c 200 "$SDIR/events.jsonl" | grep -q '"type":"session_closed"' && break; sleep 0.1; done

# ---- Assertions (facts only) ----
fail=0
# a) The slow bash was NOT cancelled.
if grep -q '"type":"activity_cancelled"' "$SDIR/events.jsonl"; then
  echo "$QA: FAIL an activity was cancelled — a send interrupted the turn" >&2; fail=1
fi
# b) Three user inputs, in order (opening + two sends).
inputs="$(count_type input_received "$SDIR/events.jsonl")"
[ "$inputs" = 3 ] || { echo "$QA: FAIL user inputs = $inputs, want 3" >&2; fail=1; }
# c) The two sends were journaled AFTER the bash started (queued during run):
#    their input_received events come after the first bash activity_started.
bash_line="$(grep -n '"name":"bash"' "$SDIR/events.jsonl" | head -1 | cut -d: -f1)"
send_lines="$(grep -n '"type":"input_received"' "$SDIR/events.jsonl" | sed -n '2,3p' | cut -d: -f1)"
for ln in $send_lines; do
  [ "$ln" -gt "$bash_line" ] || { echo "$QA: FAIL a send was journaled before the bash started" >&2; fail=1; }
done
# d) One terminal, at the tail.
tail_type="$(tail -1 "$SDIR/events.jsonl" | grep -o '"type":"[^"]*"' | head -1)"
echo "$tail_type" | grep -q session_closed || { echo "$QA: FAIL tail = $tail_type, want session_closed" >&2; fail=1; }

if [ "$fail" = 0 ]; then
  echo "$QA PASS: bash completed uninterrupted, 3 inputs in order, queued during run"
fi
exit $fail
