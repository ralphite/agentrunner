#!/usr/bin/env bash
# QA-12 real-API gate (INC-6): manual compact (with directive) and clear as
# control inputs, against live Gemini. Drives the real ar binary + daemon.
# Runtime red lines only (journal facts): compact journals a ContextCompacted
# with a NON-EMPTY summary (the idle-compact empty-reply defect is fixed), and
# clear journals a cleared compaction. We do NOT assert the model's wording.
#
#   qa/run-qa12.sh <ar-binary>
set -euo pipefail

AR="${1:?usage: run-qa12.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "QA-12: GEMINI_API_KEY unset" >&2; exit 2; }

work="$(mktemp -d /tmp/qa12.XXXXXX)"
export XDG_DATA_HOME="$work/xdg"
trap 'kill ${DPID:-0} 2>/dev/null || true; sleep 0.3; rm -rf "$work" 2>/dev/null || true' EXIT

mkdir -p "$work/ws"
cat > "$work/base.yaml" <<'YAML'
name: dev
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 1024 }
system_prompt: |
  你是一个严谨的编码助手。简洁作答,基于会话已有的上下文继续。
tools: [read_file]
permissions:
  - { action: allow }
YAML

"$AR" daemon >"$work/daemon.log" 2>&1 &
DPID=$!
sock="$XDG_DATA_HOME/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$sock" ] && break; sleep 0.1; done
[ -S "$sock" ] || { echo "QA-12: daemon socket never appeared" >&2; cat "$work/daemon.log" >&2; exit 1; }

sid="$("$AR" new --detach --workspace "$work/ws" "$work/base.yaml" \
  "记住暗号一: 紫葡萄。一个字确认。" 2>>"$work/err.log")"
[ -n "$sid" ] || { echo "QA-12: no session id" >&2; cat "$work/err.log" "$work/daemon.log" >&2; exit 1; }
echo "session $sid"
sdir="$XDG_DATA_HOME/agentrunner/sessions/$sid"

count_type() { local n; n="$(grep -c "\"type\":\"$1\"" "$2" 2>/dev/null)" || n=0; printf '%s' "${n:-0}"; }
wait_asst() {
  local want="$1" i n
  for i in $(seq 1 300); do
    n="$(count_type assistant_message "$sdir/events.jsonl")"
    [ "$n" -ge "$want" ] && return 0
    sleep 0.2
  done
  echo "QA-12: timed out waiting for $want assistant messages (have ${n:-0})" >&2
  cat "$work/daemon.log" >&2; return 1
}

wait_asst 1
"$AR" send --detach "$sid" "记住暗号二: 黄香蕉。确认。" >/dev/null
wait_asst 2

# Manual compact at idle (the case that used to produce an empty summary).
"$AR" compact "$sid" "务必保留全部暗号原文" >/dev/null
for i in $(seq 1 200); do grep -q '"type":"context_compacted"' "$sdir/events.jsonl" && break; sleep 0.2; done

fail=0
# A compaction was journaled...
grep -q '"type":"context_compacted"' "$sdir/events.jsonl" || { echo "FAIL: no context_compacted after compact" >&2; fail=1; }
# ...with a NON-EMPTY summary (the fixed red line — an empty summary would
# have silently dropped the context).
summary="$(grep '"type":"context_compacted"' "$sdir/events.jsonl" | tail -1 | python3 -c "import sys,json; print(json.loads(sys.stdin.read()).get('payload',{}).get('summary',''))")"
[ -n "$summary" ] || { echo "FAIL: compact produced an EMPTY summary (context-loss defect)" >&2; fail=1; }

# A follow-up turn so there is fresh context to clear (a clear with nothing
# new since the last boundary is correctly a no-op).
"$AR" send --detach "$sid" "好的,继续。" >/dev/null
wait_asst 3

# Clear journals a cleared compaction with an empty summary.
"$AR" clear "$sid" >/dev/null
for i in $(seq 1 100); do [ "$(count_type context_compacted "$sdir/events.jsonl")" -ge 2 ] && break; sleep 0.2; done
cleared="$(grep '"type":"context_compacted"' "$sdir/events.jsonl" | tail -1 | python3 -c "import sys,json; p=json.loads(sys.stdin.read()).get('payload',{}); print('1' if p.get('cleared') and p.get('summary','')=='' else '0')")"
[ "$cleared" = 1 ] || { echo "FAIL: clear did not journal a cleared/empty compaction" >&2; fail=1; }

if [ "$fail" = 0 ]; then
  echo "QA-12 PASS: compact journaled a non-empty summary; clear journaled a cleared compaction"
else
  echo "QA-12 FAIL" >&2; exit 1
fi
