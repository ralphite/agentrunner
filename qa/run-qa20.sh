#!/usr/bin/env bash
# QA-20 real-API gate (INC-12, UJ-23): the engineering-team journey — a lead
# with agents_dynamic drafts team members inline (role spawn), members and
# lead exchange tree messages (send_message), a quiescent member is REVIVED
# by user mail (`ar send <child-sid>`), and the results flow back.
#
# Per CLAUDE.md QA 规则: runs against the SHARED data dir + global daemon
# (no XDG sandbox), keeps every session for post-hoc inspection, and
# archives the events export under qa/runs/.
#
#   qa/run-qa20.sh <ar-binary>
set -euo pipefail
QA=QA-20
AR="${1:?usage: run-qa20.sh <ar-binary>}"
qa_here="$(cd "$(dirname "$0")" && pwd)"

# .env for GEMINI_API_KEY; shared store (NO XDG override). A worktree keeps
# its .env in the MAIN checkout — resolve through the common git dir.
[ -f "$qa_here/../.env" ] && { set -a; . "$qa_here/../.env"; set +a; }
if [ -z "${GEMINI_API_KEY:-}" ]; then
  main_root="$(cd "$qa_here/.." && dirname "$(git rev-parse --git-common-dir)")"
  [ -f "$main_root/.env" ] && { set -a; . "$main_root/.env"; set +a; }
fi
[ -n "${GEMINI_API_KEY:-}" ] || { echo "$QA: GEMINI_API_KEY unset" >&2; exit 2; }
DATA="${XDG_DATA_HOME:-$HOME/.local/share}/agentrunner"
SOCK="$DATA/daemon.sock"

count_type() { local n; n="$(grep -c "\"type\":\"$1\"" "$2" 2>/dev/null)" || n=0; printf '%s' "${n:-0}"; }

# Global daemon: reuse a live one, else start one and LEAVE it running
# (it is the user's resident runtime, not a fixture).
if ! "$AR" sessions list >/dev/null 2>&1; then
  mkdir -p "$DATA"   # pristine runner: the redirect target dir must exist
  nohup "$AR" daemon >>"$DATA/qa20-daemon.log" 2>&1 &
  for i in $(seq 1 100); do [ -S "$SOCK" ] && break; sleep 0.1; done
fi
"$AR" sessions list >/dev/null 2>&1 || { echo "$QA: no daemon" >&2; exit 1; }

WORK="$(mktemp -d /tmp/qa20-XXXX)"   # spec + workspace (kept; path recorded)
WS="$WORK/ws"; mkdir -p "$WS"
cat > "$WORK/lead.yaml" <<'YAML'
name: lead
description: engineering team lead
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 2048 }
system_prompt: |
  你是工程团队 lead,负责组建和协调一个小团队完成用户的目标。
  - 用 spawn_agent 的 role 参数动态起草成员(给出 name/description/
    instructions),不要用预定义 agent 名。
  - spawn 结果里有每个成员的 child_session id;把队友的 id 写进后续
    成员的 task 里,并告诉他们可以用 send_message(to=<session id>)
    直接联系队友,用 send_message(to="parent") 向你汇报。
  - 成员完成的回执会以消息进入你的对话;全部完成后向用户简洁汇总。
tools: [read_file, write_file]
agents_dynamic: true
# This journey intentionally has engineer/reviewer collaborate on one tree;
# production defaults child workspaces to isolated, so shared is explicit.
agent_workspace: shared
permissions:
  - { action: allow }
YAML

sid="$("$AR" new --detach --workspace "$WS" "$WORK/lead.yaml" \
  "组一个两人团队完成:成员 engineer 用 write_file 创建 hello.py(打印 hello team);成员 reviewer 等 engineer 用 send_message 通知后读 hello.py 并把评审意见用 send_message 发回 engineer、同时向你(parent)汇报。先 spawn engineer,拿到它的 session id 后再 spawn reviewer 并把 engineer 的 id 告诉它、也把 reviewer 的 id 用 send_message 告诉 engineer。全部完成后向我汇总。" 2>/dev/null | head -1)"
[ -n "$sid" ] || { echo "$QA: no session id" >&2; exit 1; }
SDIR="$DATA/sessions/$sid"
echo "$QA session: $sid"
echo "$QA workspace: $WS (kept)"

# Wait for the collaboration to settle: ≥2 receipts on the lead and an idle
# lead journal tail — bounded by a real-API budget of 180s.
deadline=$((SECONDS + 180))
while [ $SECONDS -lt $deadline ]; do
  rc="$(count_type subagent_completed "$SDIR/events.jsonl")"
  idle="$(tail -1 "$SDIR/events.jsonl" 2>/dev/null | grep -c waiting_entered || true)"
  [ "${rc:-0}" -ge 2 ] && [ "${idle:-0}" -ge 1 ] && break
  sleep 3
done

fail=0
note() { echo "$QA: $*"; }
check() { # desc cond
  if eval "$2"; then note "PASS  $1"; else note "FAIL  $1"; fail=1; fi
}

EV="$SDIR/events.jsonl"
check "lead journal exists" '[ -f "$EV" ]'
check ">=2 dynamic role spawns (RoleSpec frozen)" '[ "$(grep -c "\"role_spec\"" "$EV")" -ge 2 ]'
check ">=2 member receipts on the lead" '[ "$(count_type subagent_completed "$EV")" -ge 2 ]'

# Tree messaging actually happened: some member journal carries an
# agent-source input (send_message delivery), runtime red line.
agent_mail=0
for cj in "$SDIR"/sub/*/events.jsonl; do
  [ -f "$cj" ] || continue
  if grep -q '"source":"agent"' "$cj"; then agent_mail=1; fi
done
lead_mail="$(grep -c '"source":"agent"' "$EV" || true)"
check "tree message delivered (member or lead inbox)" '[ "$agent_mail" -ge 1 ] || [ "${lead_mail:-0}" -ge 1 ]'

# Deterministic revive: user mail to the FIRST (now quiescent) member.
child_dir="$(ls -d "$SDIR"/sub/* 2>/dev/null | head -1)"
check "a member journal exists" '[ -n "$child_dir" ]'
if [ -n "$child_dir" ]; then
  child_sid="$sid-sub-$(basename "$child_dir")"
  note "reviving member: $child_sid"
  "$AR" send --detach "$child_sid" "请用一句话总结你在这个任务里做了什么。" >/dev/null 2>&1 || true
  deadline=$((SECONDS + 90))
  while [ $SECONDS -lt $deadline ]; do
    [ "$(count_type child_revived "$EV")" -ge 1 ] && break
    sleep 3
  done
  check "ChildRevived on the lead (user mail woke the member)" '[ "$(count_type child_revived "$EV")" -ge 1 ]'
  deadline=$((SECONDS + 90))
  while [ $SECONDS -lt $deadline ]; do
    [ "$(grep -Ec '"source":"(user|cli)".*总结' "$child_dir/events.jsonl")" -ge 1 ] && break
    sleep 3
  done
  check "member consumed the user mail (user-class input)" '[ "$(grep -Ec "\"source\":\"(user|cli)\".*总结" "$child_dir/events.jsonl")" -ge 1 ]'
  check "member context continues (exactly one SessionStarted)" '[ "$(count_type session_started "$child_dir/events.jsonl")" -eq 1 ]'
fi

# Archive (数据保留纪律): events export + workspace listing.
STAMP="$(date +%Y%m%d)"
DEST="$qa_here/runs/$STAMP-QA20"
mkdir -p "$DEST"
"$AR" events --json "$sid" > "$DEST/lead-events.jsonl" 2>/dev/null || cp "$EV" "$DEST/lead-events.jsonl"
for cj in "$SDIR"/sub/*/events.jsonl; do
  [ -f "$cj" ] || continue
  cp "$cj" "$DEST/$(basename "$(dirname "$cj")")-events.jsonl"
done
ls -la "$WS" > "$DEST/workspace-listing.txt" 2>/dev/null || true
cp -r "$WS" "$DEST/ws" 2>/dev/null || true
{ echo "session: $sid"; echo "workspace: $WS"; echo "spec: $WORK/lead.yaml"; } > "$DEST/README.txt"
note "archived to $DEST"

if [ "$fail" -eq 0 ]; then note "ALL PASS (session kept: $sid)"; else note "FAILURES (session kept: $sid)"; fi
exit "$fail"
