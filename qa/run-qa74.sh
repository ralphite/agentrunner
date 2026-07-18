#!/usr/bin/env bash
# QA-74 real-API gate (INC-74, E1①): session 内 schedule 三红线。
#  1. attach 1 分钟 cron 后,session **自主**唤醒(零 send)完成一个真
#     Gemini turn(schedule_wake + assistant_message 增长)。
#  2. daemon 重启后(session 未托管),daemon timer sweep 到点 hostResume
#     → 第二次自主唤醒仍完成一个 turn——唤醒跨重启存活。
#  3. pause 后不再醒(等一个完整 cron slot,wake 计数不动),status 呈现
#     paused。
#   qa/run-qa74.sh <ar-binary>
set -uo pipefail
QA=QA-74
AR="${1:?usage: run-qa74.sh <ar-binary>}"
AR="$(cd "$(dirname "$AR")" && pwd)/$(basename "$AR")"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "$QA: GEMINI_API_KEY unset" >&2; exit 2; }
work="$(mktemp -d /tmp/qa74-XXXX)"
export XDG_DATA_HOME="$work/data"
mkdir -p "$work/ws" "$XDG_DATA_HOME/agentrunner/sessions"
EVIDENCE="${EVIDENCE_DIR:-$here/runs/$(date +%F)-QA74}"
mkdir -p "$EVIDENCE"
finish() {
  kill -9 ${DPID:-0} 2>/dev/null || true
  cp "$work"/*.log "$EVIDENCE/" 2>/dev/null || true
  for d in "$XDG_DATA_HOME/agentrunner/sessions"/*/; do
    [ -f "$d/events.jsonl" ] && cp "$d/events.jsonl" "$EVIDENCE/$(basename "$d").events.jsonl"
  done
}
trap finish EXIT
count() { local n; n="$(grep -ac "$2" "$1" 2>/dev/null)"; echo "${n:-0}"; }

daemon_up() {
  "$AR" daemon >>"$work/daemon.log" 2>&1 &
  DPID=$!
  local sock="$XDG_DATA_HOME/agentrunner/daemon.sock" i
  for i in $(seq 1 100); do [ -S "$sock" ] && return 0; sleep 0.1; done
  echo "$QA: daemon socket never appeared" >&2; cat "$work/daemon.log" >&2; exit 1
}

cat > "$work/spec.yaml" <<'YAML'
name: watcher
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 512 }
system_prompt: 你是简洁的值守助手,每次只说一两句话。
tools: []
permissions: [ { action: allow } ]
YAML

daemon_up
sid="$("$AR" new --detach --workspace "$work/ws" "$work/spec.yaml" '你好,一句话自我介绍。')"
[ -n "$sid" ] || { echo "$QA FAIL: no sid" >&2; exit 1; }
ev="$XDG_DATA_HOME/agentrunner/sessions/$sid/events.jsonl"
echo "session $sid — waiting for the opening turn to park"
ok=0
for i in $(seq 1 600); do
  if [ "$(count "$ev" '"type":"assistant_message"')" -ge 1 ] && grep -q '"type":"waiting_entered"' "$ev"; then ok=1; break; fi
  sleep 0.2
done
[ "$ok" = 1 ] || { echo "$QA FAIL: opening turn never parked" >&2; tail -5 "$ev" >&2; exit 1; }

# ---------- 1. attach 1 分钟 cron → 第一次自主唤醒完成一个 turn ----------
"$AR" schedule "$sid" attach --cron "* * * * *" "报告一下你醒来的轮次,一句话即可。" >>"$work/cli.log" 2>&1 \
  || { echo "$QA FAIL: schedule attach refused" >&2; cat "$work/cli.log" >&2; exit 1; }
ok=0
for i in $(seq 1 100); do
  if grep -q '"type":"schedule_attached"' "$ev" && grep -q '"type":"timer_set"' "$ev"; then ok=1; break; fi
  sleep 0.2
done
[ "$ok" = 1 ] || { echo "$QA FAIL(1): attach did not journal schedule_attached + timer_set" >&2; exit 1; }
asst_before="$(count "$ev" '"type":"assistant_message"')"
echo "1: schedule armed — waiting for the first autonomous wake (cron slot ≤ ~75s)"
ok=0
for i in $(seq 1 500); do  # 100s: one whole-minute slot + margin
  if [ "$(count "$ev" '"type":"schedule_wake"')" -ge 1 ] && [ "$(count "$ev" '"type":"assistant_message"')" -gt "$asst_before" ]; then ok=1; break; fi
  sleep 0.2
done
[ "$ok" = 1 ] || { echo "$QA FAIL(1): no autonomous wake+turn (wakes=$(count "$ev" '"type":"schedule_wake"') asst=$(count "$ev" '"type":"assistant_message"')/before=$asst_before)" >&2; exit 1; }
echo "PASS(1): first wake ran a real turn with zero send"

# ---------- 2. daemon 重启 → timer sweep 唤醒未托管 session ----------
echo "2: SIGTERM daemon; restart — the session is unhosted, the sweep must wake it"
kill -TERM "$DPID" 2>/dev/null
for i in $(seq 1 300); do kill -0 "$DPID" 2>/dev/null || break; sleep 0.2; done
kill -0 "$DPID" 2>/dev/null && { echo "$QA FAIL(2): daemon did not exit on SIGTERM" >&2; exit 1; }
wakes_before="$(count "$ev" '"type":"schedule_wake"')"
asst_before="$(count "$ev" '"type":"assistant_message"')"
daemon_up
ok=0
for i in $(seq 1 500); do
  if [ "$(count "$ev" '"type":"schedule_wake"')" -gt "$wakes_before" ] && [ "$(count "$ev" '"type":"assistant_message"')" -gt "$asst_before" ]; then ok=1; break; fi
  sleep 0.2
done
[ "$ok" = 1 ] || { echo "$QA FAIL(2): no wake after daemon restart (wakes=$(count "$ev" '"type":"schedule_wake"')/before=$wakes_before)" >&2; exit 1; }
echo "PASS(2): wake survived the daemon restart (timer sweep resumed the unhosted session)"

# ---------- 3. pause 后不再醒 ----------
"$AR" schedule "$sid" pause >>"$work/cli.log" 2>&1 || { echo "$QA FAIL(3): pause refused" >&2; exit 1; }
ok=0
for i in $(seq 1 100); do
  grep -q '"type":"schedule_paused"' "$ev" && { ok=1; break; }
  sleep 0.2
done
[ "$ok" = 1 ] || { echo "$QA FAIL(3): pause did not journal schedule_paused" >&2; exit 1; }
wakes_paused="$(count "$ev" '"type":"schedule_wake"')"
echo "3: paused at wakes=$wakes_paused — sleeping through a whole cron slot (80s)"
sleep 80
wakes_after="$(count "$ev" '"type":"schedule_wake"')"
if [ "$wakes_after" -ne "$wakes_paused" ]; then
  echo "$QA FAIL(3): paused schedule still woke ($wakes_paused → $wakes_after)" >&2
  exit 1
fi
"$AR" schedule "$sid" status >"$work/status.log" 2>&1
grep -q "paused" "$work/status.log" || { echo "$QA FAIL(3): status does not show paused" >&2; cat "$work/status.log" >&2; exit 1; }
echo "PASS(3): no wake while paused; status shows paused"
echo "$QA PASS — evidence in $EVIDENCE"
