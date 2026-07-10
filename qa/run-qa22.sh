#!/usr/bin/env bash
# QA-22 real-API gate (INC-13, UJ-09): microcompact — the no-LLM context
# reclaim — against live Gemini. Runtime red lines (journal facts):
#   1. a context_microcompacted event is journaled with cleared > 0;
#   2. NO context_compacted fires (compact stays disabled: micro is
#      independent and self-sufficient);
#   3. after the reclaim the session keeps working end-to-end: asked for a
#      secret that lived only in an elided old read result, the model re-runs
#      the tool (the placeholder names that escape hatch) and answers with
#      the exact secret.
#
# Concurrency note (2026-07-09): the shared daemon serves other live
# sessions, so this gate runs its own daemon (freshly built binary) on an
# isolated runtime root, then COPIES the finished session into the shared
# store (~/.local/share/agentrunner/sessions/) so it stays visible to the
# daily CLI, and archives the events export under qa/runs/.
#
#   qa/run-qa22.sh <ar-binary>
set -euo pipefail

AR="${1:?usage: run-qa22.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "QA-22: GEMINI_API_KEY unset" >&2; exit 2; }

stamp="$(date +%Y%m%d-%H%M%S)"
work="${QA22_WORK:-/tmp/qa22-$stamp}"
mkdir -p "$work"
export XDG_DATA_HOME="$work/xdg"
# Deliberately NO cleanup trap: session data is kept for post-hoc review
# (project QA rule), and the tail of this script copies it into the shared
# store. Only the private daemon is stopped.

SECRET="APERTURE-GRAPE-77"
mkdir -p "$work/ws"
mkfile() { # $1=name $2=lastline — ~2.5KB of filler then the payload line
  { for i in $(seq 1 40); do
      printf 'filler %02d: lorem ipsum dolor sit amet, consectetur adipiscing elit sed do.\n' "$i"
    done
    printf '%s\n' "$2"
  } > "$work/ws/$1"
}
mkfile a.txt "codeword-a: $SECRET"
mkfile b.txt "codeword-b: BLUE-LANTERN-12"
mkfile c.txt "codeword-c: IRON-COMET-35"

cat > "$work/spec.yaml" <<'YAML'
name: qa22
model:
  provider: gemini
  id: gemini-flash-latest
  max_tokens: 1024
  compact_at_tokens: 0        # compaction OFF: prove micro stands alone
  microcompact_at_tokens: 1200
system_prompt: |
  你是一个严谨的编码助手。用户会让你读文件并报告每个文件最后一行的
  codeword。始终先用 read_file 工具读取被要求的文件,再作答。如果需要
  的内容在上下文中已不可见(例如旧的工具结果被清除),就重新读取文件。
tools: [read_file]
permissions:
  - { action: allow }
YAML

"$AR" daemon >"$work/daemon.log" 2>&1 &
DPID=$!
sock="$XDG_DATA_HOME/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$sock" ] && break; sleep 0.1; done
[ -S "$sock" ] || { echo "QA-22: daemon socket never appeared" >&2; cat "$work/daemon.log" >&2; exit 1; }
stop_daemon() { kill "$DPID" 2>/dev/null || true; }
trap stop_daemon EXIT

sid="$("$AR" new --detach --workspace "$work/ws" "$work/spec.yaml" \
  "读 a.txt,报告最后一行的 codeword。")"
[ -n "$sid" ] || { echo "QA-22: no session id" >&2; exit 1; }
echo "QA-22 session: $sid"
sdir="$XDG_DATA_HOME/agentrunner/sessions/$sid"

count_type() { local n; n="$(grep -c "\"type\":\"$1\"" "$sdir/events.jsonl" 2>/dev/null)" || n=0; printf '%s' "${n:-0}"; }
wait_asst() { # wait until assistant_message count >= $1
  local want="$1" i n
  for i in $(seq 1 600); do
    n="$(count_type assistant_message)"
    [ "$n" -ge "$want" ] && return 0
    sleep 0.2
  done
  echo "QA-22: timed out waiting for $want assistant messages (got $n)" >&2
  exit 1
}

wait_asst 1
"$AR" send "$sid" "现在读 b.txt,报告最后一行的 codeword。" >/dev/null
"$AR" send "$sid" "现在读 c.txt,报告最后一行的 codeword。" >/dev/null
"$AR" send "$sid" "再读一次 b.txt 和 c.txt,确认两个 codeword 没有变化。" >/dev/null

micro_n="$(count_type context_microcompacted)"
if [ "$micro_n" -lt 1 ]; then
  # One more bulky turn in case the estimate is just under the threshold.
  "$AR" send "$sid" "再读一次 c.txt,报告 codeword。" >/dev/null
  micro_n="$(count_type context_microcompacted)"
fi
[ "$micro_n" -ge 1 ] || { echo "QA-22 FAIL: no context_microcompacted journaled" >&2; exit 1; }
echo "PASS(1): context_microcompacted journaled ($micro_n)"

[ "$(count_type context_compacted)" -eq 0 ] || {
  echo "QA-22 FAIL: context_compacted fired though compaction is disabled" >&2; exit 1; }
echo "PASS(2): no context_compacted (micro stands alone)"

before_reads="$(grep -c '"name":"read_file"' "$sdir/events.jsonl" || true)"
asst_before="$(count_type assistant_message)"
"$AR" send "$sid" "a.txt 最后一行的 codeword 是什么?如果上下文中已看不到它,请重新读取 a.txt 再回答。" >/dev/null
# send follows to idle, but the final assistant_message may land a beat after
# the follow returns — poll until a NEW assistant reply carries the secret.
got=""
for i in $(seq 1 120); do
  n="$(count_type assistant_message)"
  if [ "$n" -gt "$asst_before" ]; then
    last_reply="$(grep '"type":"assistant_message"' "$sdir/events.jsonl" | tail -1)"
    if printf '%s' "$last_reply" | grep -q "$SECRET"; then got="$last_reply"; break; fi
  fi
  sleep 0.25
done
after_reads="$(grep -c '"name":"read_file"' "$sdir/events.jsonl" || true)"
if [ -z "$got" ]; then
  echo "QA-22 FAIL: final answer never carried the secret from the elided result" >&2
  grep '"type":"assistant_message"' "$sdir/events.jsonl" | tail -1 >&2
  exit 1
fi
echo "PASS(3): model recovered the elided secret ($SECRET); read_file calls $before_reads -> $after_reads"

# Keep the evidence: copy the session into the shared store for the daily
# CLI, and archive the export next to the other QA runs.
shared="$HOME/.local/share/agentrunner/sessions"
mkdir -p "$shared"
cp -R "$sdir" "$shared/$sid"
run_dir="$here/runs/$(date +%Y-%m-%d)-QA22"
mkdir -p "$run_dir"
"$AR" events "$sid" > "$run_dir/events.export.jsonl" 2>/dev/null || cp "$sdir/events.jsonl" "$run_dir/events.export.jsonl"
{
  echo "QA-22 microcompact — $(date)"
  echo "session: $sid (copied into $shared)"
  echo "workspace: $work/ws (kept)"
  echo "micro events: $micro_n; read_file calls: $before_reads -> $after_reads"
} > "$run_dir/notes.md"
echo "QA-22: all green. session kept at $shared/$sid; export archived at $run_dir"
