#!/usr/bin/env bash
# QA-07 real-API gate (v2 M4): image input. Send a CI-failure screenshot
# (fixtures/build-error.png) into a live conversational session; the model
# must actually READ it (file/line/identifier), the journal must carry only
# the CAS ref (never bytes), and the image's content must persist into a
# later text-only turn. Structural asserts + the three vision facts.
#
#   qa/run-qa07.sh <ar-binary>
set -euo pipefail
QA=QA-07
AR="${1:?usage: run-qa07.sh <ar-binary>}"
. "$(cd "$(dirname "$0")" && pwd)/lib.sh"

PNG="$qa_here/fixtures/build-error.png"
[ -f "$PNG" ] || { echo "$QA: fixture $PNG missing" >&2; exit 2; }

qa_setup cobra
qa_spec "read_file, bash"
qa_daemon

sid="$(qa_new "你好。我接下来会发你一张 CI 报错截图,先待命。")"
SDIR="$(qa_sdir "$sid")"
echo "session $sid"
qa_wait_idle "$SDIR" 1

# Step 1: the screenshot, with a question only the image can answer.
"$AR" send --image "$PNG" "$sid" \
  "这是 CI 的截图。哪个文件的哪一行报了什么错?给出文件名、行号、和未定义的标识符原文。" >/dev/null
qa_wait_idle "$SDIR" 2

# Step 3 (context continuity): the identifier must come from the session
# context (we never type it here).
"$AR" send "$sid" "在这个仓库里搜一下:截图里那个未定义的标识符,去掉结尾数字后的名字存不存在?给出结论。" >/dev/null
qa_wait_idle "$SDIR" 3
"$AR" close "$sid" >/dev/null 2>&1 || true
for i in $(seq 1 100); do tail -c 200 "$SDIR/events.jsonl" | grep -q '"type":"session_closed"' && break; sleep 0.1; done

# ---- Assertions ----
fail=0
# a) Vision three facts: the answer names the file, the line, the identifier.
answers="$(grep '"type":"assistant_message"' "$SDIR/events.jsonl")"
for tok in "command.go" "1234" "EnableTraverseRunHooks2"; do
  echo "$answers" | grep -q "$tok" || {
    echo "$QA: FAIL answer never mentions '$tok' (image not actually read?)" >&2; fail=1; }
done
# b) ref-not-bytes: the image input journals a CAS ref, and the event line
#    stays small (no base64 payload).
img_line="$(grep '"type":"input_received"' "$SDIR/events.jsonl" | grep '"images"' | head -1)"
[ -n "$img_line" ] || { echo "$QA: FAIL no input_received with images" >&2; fail=1; }
echo "$img_line" | grep -q '"ref":"sha256-' || { echo "$QA: FAIL image input has no CAS ref" >&2; fail=1; }
if [ -n "$img_line" ] && [ "${#img_line}" -gt 2048 ]; then
  echo "$QA: FAIL image input line is ${#img_line} bytes — bytes leaked into the journal" >&2; fail=1
fi
# The blob itself is IN the CAS (blob-before-event held end to end).
ref="$(printf '%s' "$img_line" | sed -n 's/.*"ref":"\([^"]*\)".*/\1/p')"
if [ -n "$ref" ] && [ ! -f "$SDIR/artifacts/blobs/$ref" ]; then
  echo "$QA: FAIL CAS blob $ref not found under $SDIR/artifacts/blobs" >&2; fail=1
fi
# c) Continuity: the later text-only turn works with the identifier from
#    context — some activity or answer after send #3 mentions the base name.
tail_after="$(awk '/input_received/{n++} n>=3' "$SDIR/events.jsonl")"
echo "$tail_after" | grep -q "EnableTraverseRunHooks" || {
  echo "$QA: FAIL follow-up turn never engaged the identifier from context" >&2; fail=1; }
# Session sanity: ended exactly once, at the tail.
tail -1 "$SDIR/events.jsonl" | grep -q session_closed || { echo "$QA: FAIL session did not end" >&2; fail=1; }

if [ "$fail" = 0 ]; then
  echo "$QA PASS: vision three facts, ref-not-bytes journal, context continuity"
fi
exit $fail
