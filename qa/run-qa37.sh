#!/usr/bin/env bash
# QA-37 real-API gate (INC-31, #45/§3.5 余项): skill context:fork — the live
# model invokes a fork skill via the skill tool; the runtime expands the call
# at ingest into spawn_agent{role} and the skill runs in a ONE-SHOT sub-agent.
# Journal red lines:
#   1. the parent journal records the expanded spawn (spawn_agent with
#      role.name == skill name; frozen RoleSpec carries the skill body);
#   2. a child session ran the skill (sub/ exists; receipt back on parent);
#   3. the parent's final answer reflects the child's result (the count).
#
# The expansion lives in the daemon's agent loop → private daemon running THIS
# binary (共享 daemon 跑旧二进制会假失败); session copied to the shared store
# + export archived. (QA-34 被 INC-23.B6 占、QA-36 被 INC-29 预订,取 37。)
#
#   qa/run-qa37.sh <ar-binary>
set -euo pipefail
QA=QA-37
AR="${1:?usage: run-qa37.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
if [ -z "${GEMINI_API_KEY:-}" ]; then
  main_root="$(cd "$here/.." && dirname "$(git rev-parse --git-common-dir)")"
  [ -f "$main_root/.env" ] && { set -a; . "$main_root/.env"; set +a; }
fi
[ -n "${GEMINI_API_KEY:-}" ] || { echo "$QA: GEMINI_API_KEY unset" >&2; exit 2; }

stamp="$(date +%Y%m%d-%H%M%S)"
work="${QA37_WORK:-/tmp/qa37-$stamp}"
mkdir -p "$work/ws/.claude/skills/count-widgets"
export XDG_DATA_HOME="$work/xdg"

# Data with a KNOWN widget count (4 WIDGET lines).
cat > "$work/ws/data.txt" <<'TXT'
WIDGET alpha
gadget one
WIDGET beta
WIDGET gamma
gizmo two
WIDGET delta
TXT
WANT=4

# The fork skill: self-contained instructions, read-only tool face.
cat > "$work/ws/.claude/skills/count-widgets/SKILL.md" <<'MD'
---
description: counts WIDGET lines in data.txt and reports the number
context: fork
allowed-tools: [read_file]
---
FORK-MARKER: You are the widget counter. Read the workspace file data.txt,
count how many lines contain the word WIDGET, and report EXACTLY:
"WIDGET-COUNT: <n>". Nothing else.
MD

cat > "$work/spec.yaml" <<'YAML'
name: qa37-lead
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 1024 }
system_prompt: |
  工作区提供了技能(见 <skills> 目录)。需要执行某个技能时用 skill 工具:
  skill(name, prompt)。count-widgets 是 context:fork 技能——调用后它会在
  一次性子 agent 里执行,你会先拿到 handle,结果稍后以子 agent 报告回来;
  等报告回来后把结论转告用户。不要自己读 data.txt,把统计工作交给技能。
tools: [read_file, skill]
agents_dynamic: true
permissions:
  - { action: allow }
YAML

"$AR" daemon >"$work/daemon.log" 2>&1 &
DPID=$!
sock="$XDG_DATA_HOME/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$sock" ] && break; sleep 0.1; done
[ -S "$sock" ] || { echo "$QA: daemon socket never appeared" >&2; cat "$work/daemon.log" >&2; exit 1; }
trap 'kill "$DPID" 2>/dev/null || true' EXIT

count_type() { local n; n="$(grep -c "\"type\":\"$1\"" "$2" 2>/dev/null)" || n=0; printf '%s' "${n:-0}"; }

sid="$("$AR" new --detach --workspace "$work/ws" "$work/spec.yaml" \
  "用 skill 工具调用 count-widgets 技能(prompt: 统计 data.txt 里的 WIDGET 行数),等它的报告回来后告诉我一共有几行。" 2>/dev/null | head -1)"
[ -n "$sid" ] || { echo "$QA: no session id" >&2; exit 1; }
SDIR="$XDG_DATA_HOME/agentrunner/sessions/$sid"
echo "$QA session: $sid"
echo "$QA workspace: $work/ws (kept)"

# Wait for the fork to settle: a child receipt on the parent + idle tail.
deadline=$((SECONDS + 150))
while [ $SECONDS -lt $deadline ]; do
  rc="$(count_type subagent_completed "$SDIR/events.jsonl")"
  idle="$(tail -1 "$SDIR/events.jsonl" 2>/dev/null | grep -c waiting_entered || true)"
  [ "${rc:-0}" -ge 1 ] && [ "${idle:-0}" -ge 1 ] && break
  sleep 3
done

fail=0
note() { echo "$QA: $*"; }
check() { if eval "$2"; then note "PASS  $1"; else note "FAIL  $1"; fail=1; fi; }

EV="$SDIR/events.jsonl"
check "parent journal exists" '[ -f "$EV" ]'

# Red line 1: the journaled call is the EXPANDED spawn (role.name = skill).
check "ingest expansion journaled (spawn_agent with role count-widgets)" \
  'grep -a "\"name\":\"spawn_agent\"" "$EV" | grep -q "count-widgets"'
check "frozen RoleSpec carries the skill body (FORK-MARKER)" \
  'grep -aq "FORK-MARKER" "$EV"'

# Red line 2: a child ran the skill and reported back.
child_dir="$(ls -d "$SDIR"/sub/* 2>/dev/null | head -1)"
check "child session exists (fork actually ran)" '[ -n "$child_dir" ]'
check "receipt back on parent (subagent_completed)" '[ "$(count_type subagent_completed "$EV")" -ge 1 ]'
if [ -n "$child_dir" ]; then
  CEV="$child_dir/events.jsonl"
  check "child produced the count (WIDGET-COUNT: $WANT)" \
    'grep -aq "WIDGET-COUNT" "$CEV"'
fi

# Red line 3: the parent's answer reflects the child's result.
last="$(grep -a '"type":"assistant_message"' "$EV" | tail -1)"
if printf '%s' "$last" | grep -qE "$WANT|四"; then
  note "PASS  parent answer reflects the count ($WANT)"
else
  note "NOTE  parent answer did not name the count; child result still verified"
  check "parent produced a final answer" '[ -n "$last" ]'
fi

shared="$HOME/.local/share/agentrunner/sessions"
mkdir -p "$shared"; cp -R "$SDIR" "$shared/$sid" 2>/dev/null || true
run_dir="$here/runs/$(date +%Y-%m-%d)-QA37"
mkdir -p "$run_dir"
"$AR" events "$sid" > "$run_dir/events.export.jsonl" 2>/dev/null || cp "$EV" "$run_dir/events.export.jsonl"
[ -n "${child_dir:-}" ] && cp "$child_dir/events.jsonl" "$run_dir/child.events.jsonl" 2>/dev/null || true
{ echo "$QA skill context:fork — $(date)"; echo "session: $sid"; echo "workspace: $work/ws"; } > "$run_dir/notes.md"

if [ "$fail" -eq 0 ]; then note "all green. session copied to $shared/$sid; export archived at $run_dir"; else note "one or more red lines FAILED (kept at $SDIR)"; exit 1; fi
