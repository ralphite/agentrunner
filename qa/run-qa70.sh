#!/usr/bin/env bash
# QA-70 real-API gate (INC-71 + INC-72, audit-0717 F2): daemon 生命周期两红线。
#  A(INC-71 stranded boot sweep): mid-turn(bash 在飞)kill -9 daemon →
#    重启 → **零 send** 下 session 自动接续:in-doubt 渲染
#    interrupted-by-crash,turn 继续并 park。
#  B(INC-72 优雅停机保活 cron): 本地 ar drive 起 cron 系列 → kill -9
#    (QA-58 已验路径)→ daemon boot sweep 收编托管 → **SIGTERM 优雅
#    停机** → journal 无 driver_completed 终态 → 再次重启 → 系列复活
#    (新 skipped/completed 出现)。
#   qa/run-qa70.sh <ar-binary>
set -uo pipefail
QA=QA-70
AR="${1:?usage: run-qa70.sh <ar-binary>}"
AR="$(cd "$(dirname "$AR")" && pwd)/$(basename "$AR")"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "$QA: GEMINI_API_KEY unset" >&2; exit 2; }
work="$(mktemp -d /tmp/qa70-XXXX)"
export XDG_DATA_HOME="$work/data"
mkdir -p "$work/ws" "$XDG_DATA_HOME/agentrunner/sessions"
EVIDENCE="${EVIDENCE_DIR:-$here/runs/$(date +%F)-QA70}"
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

# ---------- A. INC-71: mid-turn kill -9 → 重启零 send 自动接续 ----------
cat > "$work/base.yaml" <<'YAML'
name: dev
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 2048 }
system_prompt: 你是严谨的编码助手。严格按用户指令行动,简洁作答。
tools: [bash]
permissions:
  - { action: allow }
YAML
daemon_up
sid="$("$AR" new --detach --workspace "$work/ws" "$work/base.yaml" \
  '请立刻用 bash 运行这条命令(不要改动它): sleep 25 && echo QA70_A_DONE ,然后一句话确认。')"
[ -n "$sid" ] || { echo "$QA FAIL(A): no sid" >&2; exit 1; }
ev="$XDG_DATA_HOME/agentrunner/sessions/$sid/events.jsonl"
echo "A: session $sid — waiting for the bash to be in flight"
inflight=0
for i in $(seq 1 300); do
  if grep -q '"activity_started"' "$ev" 2>/dev/null && grep -q 'sleep 25' "$ev" 2>/dev/null; then inflight=1; break; fi
  sleep 0.2
done
[ "$inflight" = 1 ] || { echo "$QA FAIL(A): bash never started" >&2; exit 1; }
sleep 1
echo "A: kill -9 daemon mid-turn"
kill -9 "$DPID" 2>/dev/null; sleep 1
parks_before="$(count "$ev" waiting_entered)"
daemon_up
echo "A: daemon restarted — NO send; waiting for auto-resume settle + park"
okA=0
for i in $(seq 1 600); do
  if grep -q 'interrupted by crash' "$ev" && [ "$(count "$ev" waiting_entered)" -gt "$parks_before" ]; then okA=1; break; fi
  sleep 0.2
done
if [ "$okA" != 1 ]; then
  echo "$QA FAIL(A): no zero-send auto-continuation (crash-settle=$(count "$ev" 'interrupted by crash') parks=$(count "$ev" waiting_entered)/before=$parks_before)" >&2
  exit 1
fi
echo "PASS(A): interrupted-by-crash settled and session parked with zero send"
kill -9 "$DPID" 2>/dev/null; sleep 1

# ---------- B. INC-72: 收编 → SIGTERM 优雅停机 → 无终态 → 复活 ----------
cat > "$work/spec.yaml" <<'YAML'
name: ticker
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 256 }
system_prompt: 只说一句话。
tools: []
permissions: [ { action: allow } ]
YAML
cat > "$work/driver.yaml" <<'YAML'
name: qa70cron
prompt: 说一句 "tick"
agent_spec: spec.yaml
max_iterations: 50
schedule: cron
cron: "* * * * *"
overlap: skip
YAML
find_drive_ev() {
  local s d
  for s in $(ls -t "$XDG_DATA_HOME/agentrunner/sessions" 2>/dev/null); do
    d="$XDG_DATA_HOME/agentrunner/sessions/$s"
    if head -1 "$d/events.jsonl" 2>/dev/null | grep -q driver_started; then echo "$d/events.jsonl"; return 0; fi
  done
  return 0
}
"$AR" drive "$work/driver.yaml" --workspace "$work/ws" >"$work/drive.log" 2>&1 &
DRIVEPID=$!
echo "B: local drive pid $DRIVEPID — waiting for iteration 1"
dev=""
for i in $(seq 1 650); do
  dev="$(find_drive_ev)"
  [ -n "$dev" ] && [ "$(count "$dev" iteration_completed)" -ge 1 ] && break
  sleep 0.2
done
[ -n "$dev" ] && [ "$(count "$dev" iteration_completed)" -ge 1 ] || { echo "$QA FAIL(B): no iteration" >&2; cat "$work/drive.log" >&2; kill -9 $DRIVEPID 2>/dev/null; exit 1; }
echo "B: crash the local drive; daemon boot sweep takes the series over"
kill -9 $DRIVEPID 2>/dev/null; sleep 1
before="$(count "$dev" iteration_completed)"; skipped_before="$(count "$dev" iteration_skipped)"
daemon_up
adopted=0
for i in $(seq 1 650); do
  if [ "$(count "$dev" iteration_completed)" -gt "$before" ] || [ "$(count "$dev" iteration_skipped)" -gt "$skipped_before" ]; then adopted=1; break; fi
  sleep 0.2
done
[ "$adopted" = 1 ] || { echo "$QA FAIL(B): boot sweep never adopted the series" >&2; exit 1; }
echo "B: series daemon-hosted — SIGTERM (graceful shutdown)"
kill -TERM "$DPID" 2>/dev/null
for i in $(seq 1 300); do kill -0 "$DPID" 2>/dev/null || break; sleep 0.2; done
kill -0 "$DPID" 2>/dev/null && { echo "$QA FAIL(B): daemon did not exit on SIGTERM" >&2; exit 1; }
if grep -q '"driver_completed"' "$dev"; then
  echo "$QA FAIL(B): graceful shutdown wrote a terminal driver_completed — INC-72 red line" >&2
  grep '"driver_completed"' "$dev" >&2
  exit 1
fi
echo "PASS(B1): graceful shutdown left NO terminal"
before2="$(count "$dev" iteration_completed)"; skipped2="$(count "$dev" iteration_skipped)"
daemon_up
revived=0
for i in $(seq 1 650); do
  if [ "$(count "$dev" iteration_completed)" -gt "$before2" ] || [ "$(count "$dev" iteration_skipped)" -gt "$skipped2" ]; then revived=1; break; fi
  sleep 0.2
done
[ "$revived" = 1 ] || { echo "$QA FAIL(B): series did not revive after graceful restart" >&2; exit 1; }
echo "PASS(B2): series revived after the graceful restart"
echo "$QA PASS — evidence in $EVIDENCE"
