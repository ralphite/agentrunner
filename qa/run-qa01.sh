#!/usr/bin/env bash
# QA-01 real-API gate (v2 M1 milestone exit): three-turn conversational
# continuity against the live Gemini API. Drives the actual ar binary +
# daemon end to end. Structural asserts only (journal facts), per QA.md §0.1.
#
#   qa/run-qa01.sh <ar-binary>
# Requires GEMINI_API_KEY in the environment (loaded from repo .env).
set -euo pipefail

AR="${1:?usage: run-qa01.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../../.env" ] && { set -a; . "$here/../../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "QA-01: GEMINI_API_KEY unset" >&2; exit 2; }

work="$(mktemp -d)"
export XDG_DATA_HOME="$work/xdg"
trap 'kill ${DPID:-0} 2>/dev/null || true; sleep 0.3; rm -rf "$work" 2>/dev/null || true' EXIT

"$here/ws.sh" prepare color "$work/ws" >/dev/null

cat > "$work/base.yaml" <<'YAML'
name: dev
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 2048 }
system_prompt: |
  你是一个严谨的编码助手。简洁作答,基于会话已有的上下文继续,不要
  在每次回答时重新自我介绍。
tools: [read_file, bash]
permissions:
  - { action: allow }
YAML

# Daemon with GEMINI_API_KEY in ITS environment (provider resolves daemon-side).
"$AR" daemon >"$work/daemon.log" 2>&1 &
DPID=$!
sock="$XDG_DATA_HOME/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$sock" ] && break; sleep 0.1; done
[ -S "$sock" ] || { echo "QA-01: daemon socket never appeared" >&2; cat "$work/daemon.log" >&2; exit 1; }

# Turn 1: open the conversational session.
sid="$("$AR" new --workspace "$work/ws" "$work/base.yaml" \
  "这个库的 NoColor 开关在哪个文件实现的？一句话回答。" 2>>"$work/err.log")"
[ -n "$sid" ] || { echo "QA-01: no session id" >&2; cat "$work/err.log" "$work/daemon.log" >&2; exit 1; }
echo "session $sid"
sdir="$XDG_DATA_HOME/agentrunner/sessions/$sid"

# Wait for N assistant messages to appear in the journal.
count_type() { # type file — errexit/pipefail-safe (grep -c returns 1 on 0 matches)
  local n; n="$(grep -c "\"type\":\"$1\"" "$2" 2>/dev/null)" || n=0
  printf '%s' "${n:-0}"
}
wait_turns() {
  local want="$1" i n
  for i in $(seq 1 300); do
    n="$(count_type assistant_message "$sdir/events.jsonl")"
    [ "$n" -ge "$want" ] && return 0
    sleep 0.2
  done
  echo "QA-01: timed out waiting for $want assistant messages (have ${n:-0})" >&2
  cat "$work/daemon.log" >&2; return 1
}

wait_turns 1
# Turn 2: a follow-up that only makes sense with continuity, carrying a
# pinned codeword the final answer must echo.
"$AR" send "$sid" "它和环境变量 NO_COLOR 有关系吗？一句话。另外记住暗号一:红苹果。" >/dev/null
wait_turns 2
# Turn 3: force a reference to BOTH prior rounds (file answer + codeword).
"$AR" send "$sid" "记住暗号二:绿梨子。现在把两个暗号连起来说一遍,并再说一次 NoColor 在哪个文件。" >/dev/null
wait_turns 3
# Close.
"$AR" close "$sid" >/dev/null
for i in $(seq 1 100); do
  tail -c 200 "$sdir/events.jsonl" | grep -q '"type":"run_ended"' && break; sleep 0.1
done

# ---- Structural assertions (QA.md §0.1: facts, not model wording) ----
inputs="$(count_type input_received "$sdir/events.jsonl")"
ends="$(count_type run_ended "$sdir/events.jsonl")"
turns="$(count_type generation_started "$sdir/events.jsonl")"
tail_type="$(tail -1 "$sdir/events.jsonl" | grep -o '"type":"[^"]*"' | head -1)"

fail=0
[ "$inputs" = 3 ] || { echo "FAIL: user inputs = $inputs, want 3" >&2; fail=1; }
[ "$ends" = 1 ]  || { echo "FAIL: run_ended = $ends, want exactly 1" >&2; fail=1; }
[ "$turns" -ge 3 ] || { echo "FAIL: turns = $turns, want >= 3" >&2; fail=1; }
echo "$tail_type" | grep -q run_ended || { echo "FAIL: journal tail = $tail_type, want run_ended" >&2; fail=1; }
# Continuity CONTENT (QA.md: 回答包含前两轮各自的要素): the final answer
# echoes both pinned codewords — objective proof both rounds are in context.
final="$(grep '"type":"assistant_message"' "$sdir/events.jsonl" | tail -1)"
for tok in "红苹果" "绿梨子"; do
  echo "$final" | grep -q "$tok" || { echo "FAIL: final answer misses codeword $tok (context lost)" >&2; fail=1; }
done

if [ "$fail" = 0 ]; then
  echo "QA-01 PASS: 1 session, $turns turns, 3 inputs, 1 terminal (closed)"
else
  echo "--- last 30 journal events ---" >&2
  tail -30 "$sdir/events.jsonl" | grep -o '"type":"[^"]*"' >&2
fi
exit $fail
