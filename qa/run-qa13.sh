#!/usr/bin/env bash
# QA-13 real-API gate (INC-5): the model fetches an external page with
# web_fetch, then pauses on ask_user for a human decision, then acts on the
# answer — all against the live Gemini API, driving the real ar binary +
# daemon end to end. Structural asserts only (journal facts), per QA.md §0.1:
# we assert the tools were INVOKED, the fetched content came back, the park
# was answered via the inbox, and the session stayed healthy — never the
# model's wording.
#
#   qa/run-qa13.sh <ar-binary>
# Requires GEMINI_API_KEY in the environment (loaded from repo .env) and
# python3 (to serve a local page — no external network dependency).
set -euo pipefail

AR="${1:?usage: run-qa13.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "QA-13: GEMINI_API_KEY unset" >&2; exit 2; }
command -v python3 >/dev/null || { echo "QA-13: python3 required to serve the page" >&2; exit 2; }

# Short base dir: macOS caps unix socket paths at 104 bytes, and the default
# $TMPDIR under /var/folders blows past that once nested.
work="$(mktemp -d /tmp/qa13.XXXXXX)"
export XDG_DATA_HOME="$work/xdg"
trap 'kill ${DPID:-0} ${HPID:-0} 2>/dev/null || true; sleep 0.3; rm -rf "$work" 2>/dev/null || true' EXIT

# A local page carrying a distinctive recommendation, plus script/style noise
# that the HTML→text reduction MUST strip. web_fetch reaching this proves the
# fetch + extraction path end to end without any external network dependency.
site="$work/site"
mkdir -p "$site"
cat > "$site/rec.html" <<'HTML'
<html><head><title>DB choice</title><style>b{color:red}</style>
<script>console.log("MUST_NOT_SURFACE")</script></head>
<body><h1>Recommendation</h1>
<p>For this workload we recommend <b>PostgreSQL</b>, not MySQL.</p></body></html>
HTML

# Serve on an ephemeral port; capture it from python.
port=18913
( cd "$site" && python3 -m http.server "$port" --bind 127.0.0.1 >"$work/http.log" 2>&1 ) &
HPID=$!
url="http://127.0.0.1:$port/rec.html"
for i in $(seq 1 100); do
  curl -sf "$url" >/dev/null 2>&1 && break; sleep 0.1
done
curl -sf "$url" >/dev/null 2>&1 || { echo "QA-13: local http server never came up" >&2; cat "$work/http.log" >&2; exit 1; }

ws="$work/ws"
mkdir -p "$ws"
cat > "$work/base.yaml" <<YAML
name: dev
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 2048 }
system_prompt: |
  你是一个严谨的助手。按用户给的步骤逐步执行:先用 web_fetch 抓取给定 URL,
  再用 ask_user 征询用户决定,最后按回答用 write_file 落盘。简洁作答。
tools: [web_fetch, ask_user, write_file]
permissions:
  - { action: allow }
YAML

"$AR" daemon >"$work/daemon.log" 2>&1 &
DPID=$!
sock="$XDG_DATA_HOME/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$sock" ] && break; sleep 0.1; done
[ -S "$sock" ] || { echo "QA-13: daemon socket never appeared" >&2; cat "$work/daemon.log" >&2; exit 1; }

sid="$("$AR" new --detach --workspace "$ws" "$work/base.yaml" \
  "第一步:用 web_fetch 抓取 $url,看它推荐哪个数据库。第二步:用 ask_user 问我'要采用它推荐的数据库吗?'。第三步:根据我的回答,用 write_file 在工作区写 decision.txt,一句话记录最终决定。" 2>>"$work/err.log")"
[ -n "$sid" ] || { echo "QA-13: no session id" >&2; cat "$work/err.log" "$work/daemon.log" >&2; exit 1; }
echo "session $sid"
sdir="$XDG_DATA_HOME/agentrunner/sessions/$sid"
ev="$sdir/events.jsonl"

# Wait for the ask_user park: a WAITING_INPUT carrying a question detail
# (a plain standby idle has no question in its detail).
wait_park() {
  local i
  for i in $(seq 1 400); do
    grep -q '"type":"waiting_entered"' "$ev" 2>/dev/null && grep '"type":"waiting_entered"' "$ev" | grep -q 'question' && return 0
    grep -q '"type":"actor_crashed"' "$ev" 2>/dev/null && { echo "QA-13: session crashed before park" >&2; return 1; }
    sleep 0.2
  done
  echo "QA-13: timed out waiting for ask_user park" >&2; cat "$work/daemon.log" >&2; return 1
}
wait_park

# Answer via the inbox — a plain send IS the reply (no special verb).
"$AR" send --detach "$sid" "采用,就用它推荐的数据库。" >/dev/null

# Wait for the resolution + the follow-through write.
wait_answered() {
  local i
  for i in $(seq 1 400); do
    grep -q '"type":"ask_resolved"' "$ev" 2>/dev/null && grep '"type":"ask_resolved"' "$ev" | grep -q 'answered' && return 0
    sleep 0.2
  done
  echo "QA-13: timed out waiting for ask_resolved(answered)" >&2; return 1
}
wait_answered

# Give the follow-up turn a moment to write the file, then close.
for i in $(seq 1 200); do [ -f "$ws/decision.txt" ] && break; sleep 0.2; done
"$AR" close "$sid" >/dev/null 2>&1 || true
for i in $(seq 1 100); do
  tail -c 200 "$ev" | grep -q '"type":"session_closed"' && break; sleep 0.1
done

# ---- Structural assertions (journal facts, not model wording) ----
fail=0
grep -q '"name":"web_fetch"' "$ev" || { echo "FAIL: web_fetch never invoked" >&2; fail=1; }
# The fetched + HTML-reduced content actually came back to the model.
grep -q 'PostgreSQL' "$ev" || { echo "FAIL: fetched page content not in journal" >&2; fail=1; }
# The HTML→text reduction stripped the <script> body.
if grep -q 'MUST_NOT_SURFACE' "$ev"; then echo "FAIL: script body leaked through HTML reduction" >&2; fail=1; fi
# The park was entered (question) and answered via the inbox.
grep '"type":"waiting_entered"' "$ev" | grep -q 'question' || { echo "FAIL: no ask_user park journaled" >&2; fail=1; }
grep '"type":"ask_resolved"' "$ev" | grep -q 'answered' || { echo "FAIL: park not answered" >&2; fail=1; }
grep '"type":"waiting_resolved"' "$ev" | grep -q 'answered' || { echo "FAIL: park not resolved as answered" >&2; fail=1; }
# The answer drove real follow-through work.
[ -f "$ws/decision.txt" ] || { echo "FAIL: decision.txt not written after the answer" >&2; fail=1; }
# Session stayed healthy.
if grep -q '"type":"actor_crashed"' "$ev"; then echo "FAIL: actor crashed" >&2; fail=1; fi

# Archive the journal + workspace diff before the trap wipes $work (QA.md
# 执行纪律). Set QA_ARCHIVE to a target dir to keep the evidence.
if [ -n "${QA_ARCHIVE:-}" ]; then
  mkdir -p "$QA_ARCHIVE"
  cp "$ev" "$QA_ARCHIVE/events.jsonl" 2>/dev/null || true
  [ -f "$ws/decision.txt" ] && cp "$ws/decision.txt" "$QA_ARCHIVE/decision.txt"
fi

if [ "$fail" = 0 ]; then
  echo "QA-13 PASS: web_fetch fetched+reduced, ask_user parked+answered via inbox, follow-through written"
else
  echo "QA-13 FAIL" >&2; exit 1
fi
