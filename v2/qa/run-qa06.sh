#!/usr/bin/env bash
# QA-06 real-API gate (v2 M2 exit): interrupt vs input are distinct. A slow
# bash runs; `interrupt` CANCELS it (activity_cancelled, process gone),
# whereas `send` (QA-02) only queues. Then the session continues. Structural
# asserts only.
#
#   v2/qa/run-qa06.sh <ar-binary>
set -euo pipefail
QA=QA-06
AR="${1:?usage: run-qa06.sh <ar-binary>}"
. "$(cd "$(dirname "$0")" && pwd)/lib.sh"

qa_setup cobra
qa_spec "read_file, bash"
cat > "$WS/qa_slow.sh" <<'SH'
#!/usr/bin/env bash
sleep 30
echo SHOULD_NOT_PRINT
SH
chmod +x "$WS/qa_slow.sh"
qa_daemon

sid="$(qa_new "运行 ./qa_slow.sh。只运行这一个命令,等它结束。")"
SDIR="$(qa_sdir "$sid")"
echo "session $sid"

# Wait until the slow bash is in flight.
started=0
for i in $(seq 1 100); do
  grep -q '"name":"bash"' "$SDIR/events.jsonl" 2>/dev/null && { started=1; break; }
  sleep 0.1
done
[ "$started" = 1 ] || { echo "$QA: bash never started" >&2; cat "$WORK/daemon.log" >&2; exit 1; }
sleep 1  # let it get a beat into the 30s sleep

# INTERRUPT (not send): this must cancel the running bash.
"$AR" interrupt "$sid" >/dev/null

# The bash must be cancelled — an activity_cancelled appears and SHOULD_NOT_PRINT never does.
cancelled=0
for i in $(seq 1 100); do
  grep -q '"type":"activity_cancelled"' "$SDIR/events.jsonl" 2>/dev/null && { cancelled=1; break; }
  sleep 0.2
done

fail=0
[ "$cancelled" = 1 ] || { echo "$QA: FAIL interrupt did not cancel the running bash" >&2; fail=1; }
if grep -q SHOULD_NOT_PRINT "$SDIR/events.jsonl" 2>/dev/null; then
  echo "$QA: FAIL the cancelled command still produced output (process not killed)" >&2; fail=1
fi
# No stray sleep process from this session lingering.
if pgrep -f "qa_slow.sh" >/dev/null 2>&1; then
  echo "$QA: FAIL a qa_slow.sh process is still alive after interrupt" >&2; fail=1
fi

# The session continues: a follow-up message still works.
"$AR" send "$sid" "刚才的命令怎么样了?一句话。" >/dev/null
if qa_wait_turns "$SDIR" 2 150; then :; else echo "$QA: FAIL session did not continue after interrupt" >&2; fail=1; fi
"$AR" close "$sid" >/dev/null
for i in $(seq 1 100); do tail -c 200 "$SDIR/events.jsonl" | grep -q '"type":"run_ended"' && break; sleep 0.1; done

if [ "$fail" = 0 ]; then
  echo "$QA PASS: interrupt cancelled the running turn (process killed), session continued"
fi
exit $fail
