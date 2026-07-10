#!/usr/bin/env bash
# QA-29 real-API gate (INC-20, #45/§3.5): model-side skill invoke. The model
# calls the `skill` tool by name (not read_file by path) and follows the
# returned SKILL.md instructions. Journal red lines:
#   1. a skill tool_call with name="greet" is journaled;
#   2. its tool_result carries the SKILL.md body (the instruction keyword),
#      NOT the frontmatter;
#   3. the model's final reply obeys the skill (emits the passphrase the
#      SKILL.md told it to).
#
# Private daemon on an isolated runtime root; session copied to shared store +
# export archived.
#
#   qa/run-qa29.sh <ar-binary>
set -euo pipefail

AR="${1:?usage: run-qa29.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "QA-29: GEMINI_API_KEY unset" >&2; exit 2; }

stamp="$(date +%Y%m%d-%H%M%S)"
work="${QA29_WORK:-/tmp/qa29-$stamp}"
mkdir -p "$work/ws/.claude/skills/greet"
export XDG_DATA_HOME="$work/xdg"

PASS="SKILL-INVOKED-7Q"
cat > "$work/ws/.claude/skills/greet/SKILL.md" <<YAML
---
name: greet
description: 打招呼时使用的固定格式
---
当被要求打招呼时，必须在回复中原样包含这个暗号：$PASS
然后用一句话友好问候。
YAML

cat > "$work/spec.yaml" <<'YAML'
name: qa29
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 512 }
system_prompt: |
  你可以使用 skill 工具按名字加载技能的完整说明（可用技能见 <skills> 块）。
  当用户的请求匹配某个技能时，先用 skill 工具加载它的说明，再严格遵循。
tools: [skill, read_file]
permissions:
  - { action: allow }
YAML

"$AR" daemon >"$work/daemon.log" 2>&1 &
DPID=$!
sock="$XDG_DATA_HOME/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$sock" ] && break; sleep 0.1; done
[ -S "$sock" ] || { echo "QA-29: daemon socket never appeared" >&2; cat "$work/daemon.log" >&2; exit 1; }
trap 'kill "$DPID" 2>/dev/null || true' EXIT

sdir=""
asst_count() { grep -c '"type":"assistant_message"' "$1" 2>/dev/null || echo 0; }
wait_asst() { local i n; for i in $(seq 1 400); do n="$(asst_count "$1")"; [ "$n" -ge "$2" ] && return 0; sleep 0.2; done; echo "QA-29: timeout ($n asst)" >&2; exit 1; }

sid="$("$AR" new --detach --workspace "$work/ws" "$work/spec.yaml" \
  "请用 greet 技能跟我打个招呼。")"
[ -n "$sid" ] || { echo "QA-29: no session id" >&2; exit 1; }
echo "QA-29 session: $sid"
sdir="$XDG_DATA_HOME/agentrunner/sessions/$sid"
# skill invoke + follow-up reply → at least 2 assistant messages (tool-call turn + answer)
wait_asst "$sdir/events.jsonl" 2

ev="$sdir/events.jsonl"
# Red line 1: a skill tool_call named greet.
if ! grep -q '"name":"skill"' "$ev"; then
  echo "QA-29 FAIL: model did not call the skill tool" >&2; exit 1
fi
if ! grep '"name":"skill"' "$ev" | grep -q 'greet'; then
  echo "QA-29 FAIL: skill tool_call did not name greet" >&2; exit 1
fi
echo "PASS(1): model invoked skill(name=greet)"

# Red line 2: the skill tool_result carried the SKILL.md body (the passphrase
# instruction), not the frontmatter.
if ! grep -q "$PASS" "$ev"; then
  echo "QA-29 FAIL: skill body (passphrase) not present in journal" >&2; exit 1
fi
if grep -q "description: 打招呼时使用的固定格式" "$ev"; then
  echo "QA-29 FAIL: frontmatter leaked into the skill result" >&2; exit 1
fi
echo "PASS(2): skill result carried the body (frontmatter stripped)"

# Red line 3: the model's FINAL reply obeys the skill (emits the passphrase).
last="$(grep '"type":"assistant_message"' "$ev" | tail -1)"
if ! printf '%s' "$last" | grep -q "$PASS"; then
  echo "QA-29 FAIL: final reply did not follow the skill (no passphrase)" >&2
  printf '%s\n' "$last" >&2; exit 1
fi
echo "PASS(3): model followed the skill instructions (passphrase in final reply)"

shared="$HOME/.local/share/agentrunner/sessions"
mkdir -p "$shared"; cp -R "$sdir" "$shared/$sid"
run_dir="$here/runs/$(date +%Y-%m-%d)-QA29"
mkdir -p "$run_dir"
"$AR" events "$sid" > "$run_dir/events.export.jsonl" 2>/dev/null || cp "$ev" "$run_dir/events.export.jsonl"
{ echo "QA-29 skill model-side invoke — $(date)"; echo "session: $sid"; } > "$run_dir/notes.md"
echo "QA-29: all green. session kept in $shared; export archived at $run_dir"
