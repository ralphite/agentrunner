#!/usr/bin/env bash
# QA-58 real gate (INC-54, HANDA #28b): cron cross-restart wake + boot sweep
# (G22 crash-restart). A local `ar drive` runs a cron */1 spec and is KILLED -9
# (crash); after missing slots, a fresh daemon's BOOT SWEEP scans the store,
# re-hosts the orphaned drive, and backfills the missed slots.
#   1. `ar drive` cron */1 → >=1 real iteration; kill -9 (crash);
#   2. sleep past >=2 missed slots;
#   3. start a daemon → boot sweep re-hosts + backfills (skipped/catch-up).
#   qa/run-qa58.sh <ar-binary>
set -uo pipefail
AR="${1:?usage: run-qa58.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "QA-58: GEMINI_API_KEY unset" >&2; exit 2; }
work="$(mktemp -d /tmp/qa58-XXXX)"
export XDG_DATA_HOME="$work/data"
mkdir -p "$work/ws" "$XDG_DATA_HOME/agentrunner/sessions"
cat > "$work/spec.yaml" <<'YAML'
name: qa58child
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 256 }
system_prompt: 你每次迭代只回一句 "tick"。
tools: []
permissions: [ { action: allow } ]
YAML
cat > "$work/driver.yaml" <<'YAML'
name: qa58cron
task: 说一句 "tick"
agent_spec: spec.yaml
max_iterations: 50
schedule: cron
cron: "* * * * *"
overlap: skip
YAML
count() { local n; n="$(grep -ac "$2" "$1" 2>/dev/null)"; echo "${n:-0}"; }
find_drive_ev() {
  local s d
  for s in $(ls -t "$XDG_DATA_HOME/agentrunner/sessions" 2>/dev/null); do
    d="$XDG_DATA_HOME/agentrunner/sessions/$s"
    if head -1 "$d/events.jsonl" 2>/dev/null | grep -q driver_started; then echo "$d/events.jsonl"; return 0; fi
  done
  return 0
}

# 1. local ar drive (background), wait for >=1 iteration, then crash it.
"$AR" drive "$work/driver.yaml" --workspace "$work/ws" >"$work/drive.log" 2>&1 &
DRIVEPID=$!
echo "ar drive pid $DRIVEPID (cron * * * * *), waiting for first iteration…"
ev=""
for i in $(seq 1 650); do
  ev="$(find_drive_ev)"
  if [ -n "$ev" ] && [ "$(count "$ev" iteration_completed)" -ge 1 ]; then break; fi
  sleep 0.2
done
if [ -z "$ev" ] || [ "$(count "$ev" iteration_completed)" -lt 1 ]; then
  echo "QA-58 FAIL: no iteration completed" >&2; cat "$work/drive.log" >&2; kill -9 $DRIVEPID 2>/dev/null; exit 1
fi
before="$(count "$ev" iteration_completed)"
sid="$(basename "$(dirname "$ev")")"
echo "PASS(1): drive $sid ran $before iteration(s); killing -9 (crash)"
kill -9 $DRIVEPID 2>/dev/null; sleep 1

# 2. sleep past >=2 missed slots.
echo "sleeping 140s to miss >=2 cron slots…"; sleep 140

# 3. start a daemon → boot sweep must re-host + backfill.
"$AR" daemon >"$work/daemon.log" 2>&1 &
DPID=$!
trap 'kill -9 $DPID 2>/dev/null' EXIT
sock="$XDG_DATA_HOME/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$sock" ] && break; sleep 0.1; done
echo "daemon up (pid $DPID); waiting for boot-sweep re-host + backfill…"
ok=0
for i in $(seq 1 650); do
  sk="$(count "$ev" iteration_skipped)"; ic="$(count "$ev" iteration_completed)"
  if [ "$sk" -ge 1 ] || [ "$ic" -gt "$before" ]; then ok=1; break; fi
  sleep 0.2
done
sk="$(count "$ev" iteration_skipped)"; ic="$(count "$ev" iteration_completed)"
if [ "$ok" != 1 ]; then
  echo "QA-58 FAIL: boot sweep did not re-host/backfill (skipped=$sk completed=$ic before=$before)" >&2
  tail -8 "$ev" >&2; cat "$work/daemon.log" >&2; exit 1
fi
echo "PASS(2,3): boot sweep re-hosted — skipped=$sk completed=$ic (was $before); missed slots backfilled"

run_dir="$here/runs/$(date +%Y-%m-%d)-INC54"; mkdir -p "$run_dir"; cp "$ev" "$run_dir/events.jsonl" 2>/dev/null
{ echo "QA-58 cron boot sweep — $(date)"; echo "session: $sid"; echo "before crash=$before completed; after boot sweep: skipped=$sk completed=$ic"; } > "$run_dir/notes.md"
echo "QA-58: all green. archived at $run_dir"
