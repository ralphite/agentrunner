#!/usr/bin/env bash
# QA-17 real-API gate (INC-10, UJ-22 步骤 2b): a goal WITHOUT a command
# verifier is completable — the model works, then declares completion via the
# goal_complete tool; the turn boundary adjudicates the audited claim.
# Runtime red lines only (journal facts): single session_started (context
# continuity), a model-sourced goal_completion_claimed, goal_achieved with
# reason=satisfied, and the goal's artifact actually on disk. We do NOT
# assert the model's wording.
#
# Runs against the SHARED daemon/store by default (项目 QA 规则:测试会话要
# 在用户日常 CLI/webui 里可见、可追问;不起隔离沙箱)。The daemon must
# already be running ON A BINARY WITH INC-10 — this script does not restart
# it (a restart could disturb concurrent real sessions).
#
#   qa/run-qa17.sh <ar-binary>
set -euo pipefail

AR="${1:?usage: run-qa17.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "QA-17: GEMINI_API_KEY unset" >&2; exit 2; }

store="${XDG_DATA_HOME:-$HOME/.local/share}/agentrunner"
[ -S "$store/daemon.sock" ] || { echo "QA-17: shared daemon not running ($store/daemon.sock)" >&2; exit 2; }

work="$(mktemp -d /tmp/qa17.XXXXXX)"
ws="$work/ws"; mkdir -p "$ws"
cat > "$work/base.yaml" <<'YAML'
name: dev
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 2048 }
system_prompt: |
  你是一个严谨的编码助手。简洁作答,严格按用户指令行动,基于会话已有
  上下文继续。
tools: [read_file, write_file, bash]
permissions:
  - { action: allow }
YAML

count_type() { local n; n="$(grep -c "\"type\":\"$1\"" "$2" 2>/dev/null)" || n=0; printf '%s' "${n:-0}"; }

sid="$("$AR" new --detach --workspace "$ws" "$work/base.yaml" \
  "你好,请待命;接下来我会给你一个目标。" 2>>"$work/err.log")"
[ -n "$sid" ] || { echo "QA-17: no session id" >&2; cat "$work/err.log" >&2; exit 1; }
echo "session $sid"
sdir="$store/sessions/$sid"

for i in $(seq 1 300); do
  [ "$(count_type assistant_message "$sdir/events.jsonl")" -ge 1 ] && break; sleep 0.2
done

# The INC-10 form: NO --verify — a self-certified goal.
"$AR" goal "$sid" attach --max-checks 6 \
  "在 workspace 里创建 haiku.txt,内容是一首关于 event-sourced 系统的俳句(三行)。文件确实存在且内容完整后,目标即算完成。" >/dev/null

achieved=""
for i in $(seq 1 900); do
  if grep -q '"type":"goal_achieved"' "$sdir/events.jsonl" 2>/dev/null; then achieved=1; break; fi
  sleep 0.2
done
[ -n "$achieved" ] || { echo "QA-17 FAIL: no goal_achieved within timeout" >&2; tail -5 "$sdir/events.jsonl" >&2; exit 1; }

fail=0
py() { python3 -c "$1" < "$sdir/events.jsonl"; }

n_sess="$(count_type session_started "$sdir/events.jsonl")"
[ "$n_sess" = 1 ] || { echo "FAIL: session_started=$n_sess, want 1 (context continuity)" >&2; fail=1; }

n_claim="$(count_type goal_completion_claimed "$sdir/events.jsonl")"
[ "$n_claim" -ge 1 ] || { echo "FAIL: no goal_completion_claimed (model never declared)" >&2; fail=1; }

reason="$(py 'import sys,json
for l in sys.stdin:
    e=json.loads(l)
    if e.get("type")=="goal_achieved": print(e["payload"]["reason"])')"
[ "$reason" = "satisfied" ] || { echo "FAIL: goal_achieved reason=$reason, want satisfied" >&2; fail=1; }

detail="$(py 'import sys,json
d=""
for l in sys.stdin:
    e=json.loads(l)
    if e.get("type")=="goal_checkpoint": d=e["payload"].get("detail","")
print(d)')"
case "$detail" in *model-certified*) ;; *) echo "FAIL: final checkpoint detail=$detail, want model-certified" >&2; fail=1;; esac

[ -s "$ws/haiku.txt" ] || { echo "FAIL: haiku.txt missing/empty in workspace" >&2; fail=1; }

# Archive per QA 纪律 (事后可核查).
run_dir="$here/runs/2026-07-09-QA-17"
mkdir -p "$run_dir"
cp "$sdir/events.jsonl" "$run_dir/events.jsonl"
cp "$ws/haiku.txt" "$run_dir/haiku.txt" 2>/dev/null || true
{
  echo "QA-17 $(date '+%F %T')  session=$sid  store=$store (shared)"
  echo "session_started=$n_sess claims=$n_claim reason=$reason"
  echo "checkpoint_detail=$detail"
  echo "haiku.txt:"; sed 's/^/  /' "$ws/haiku.txt" 2>/dev/null || echo "  (missing)"
} > "$run_dir/summary.txt"

if [ "$fail" = 0 ]; then echo "QA-17 PASS  (archived to $run_dir; session kept in shared store)"; else
  echo "QA-17 FAIL  (archived to $run_dir)"; exit 1; fi
