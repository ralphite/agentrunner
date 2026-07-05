#!/usr/bin/env bash
# QA-03 real-API gate (v2 M4.3): fix an injected bug, then create a new file
# with write_file. Covers C1 (continued conversation doing real work) and
# core tool 9 (write_file as a first-class path, not a bash heredoc).
#
#   qa/run-qa03.sh <ar-binary>
set -euo pipefail
QA=QA-03
AR="${1:?usage: run-qa03.sh <ar-binary>}"
. "$(cd "$(dirname "$0")" && pwd)/lib.sh"

qa_setup cobra-broken
qa_spec "read_file, write_file, edit_file, bash"
qa_daemon

# Baseline: the injected test is RED. (qa_inject/ is untracked — record
# content hashes for the before/after comparison instead of git diff.)
if (cd "$WS" && go test ./qa_inject/ >/dev/null 2>&1); then
  echo "$QA: baseline is green — injection missing" >&2; exit 2
fi
impl_before="$(sha256sum "$WS/qa_inject/calc.go" | cut -d' ' -f1)"
test_before="$(sha256sum "$WS/qa_inject/calc_test.go" | cut -d' ' -f1)"

sid="$(qa_new "qa_inject 包的测试挂了。修复实现(文档说 Add 是加法),不要改测试,修完自己跑测试验证。")"
SDIR="$(qa_sdir "$sid")"
echo "session $sid"
qa_wait_idle "$SDIR" 1 900

# Step 4: a NEW file via write_file.
"$AR" send "$sid" "在仓库根新建 QA_NOTES.md,用 write_file 工具,两句话记录你刚才改了什么。" >/dev/null
qa_wait_idle "$SDIR" 2 600
"$AR" close "$sid" >/dev/null 2>&1 || true
for i in $(seq 1 100); do tail -c 200 "$SDIR/events.jsonl" | grep -q '"type":"run_ended"' && break; sleep 0.1; done

# ---- Assertions ----
fail=0
# a) The injected test is now GREEN.
if ! (cd "$WS" && go test ./qa_inject/ >/dev/null 2>&1); then
  echo "$QA: FAIL qa_inject tests still red" >&2; fail=1
fi
# b) The fix touched the implementation, NOT the test.
impl_after="$(sha256sum "$WS/qa_inject/calc.go" | cut -d' ' -f1)"
test_after="$(sha256sum "$WS/qa_inject/calc_test.go" | cut -d' ' -f1)"
[ "$impl_after" != "$impl_before" ] || {
  echo "$QA: FAIL calc.go untouched — what got fixed?" >&2; fail=1; }
[ "$test_after" = "$test_before" ] || {
  echo "$QA: FAIL the agent modified the test" >&2; fail=1; }
# c) QA_NOTES.md exists, non-empty, and was written by the write_file tool.
if [ ! -s "$WS/QA_NOTES.md" ]; then
  echo "$QA: FAIL QA_NOTES.md missing or empty" >&2; fail=1
fi
if ! grep '"type":"activity_started"' "$SDIR/events.jsonl" | grep '"name":"write_file"' | grep -q "QA_NOTES"; then
  echo "$QA: FAIL QA_NOTES.md was not created via the write_file tool" >&2; fail=1
fi
tail -1 "$SDIR/events.jsonl" | grep -q run_ended || { echo "$QA: FAIL session did not end" >&2; fail=1; }

if [ "$fail" = 0 ]; then
  echo "$QA PASS: injected bug fixed (impl only), QA_NOTES.md created via write_file"
fi
exit $fail
