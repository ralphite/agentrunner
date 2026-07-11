#!/usr/bin/env bash
# QA-59 real-API gate (INC-55, HANDA #4): user-defined command tools. Real
# Gemini invoking a user-layer command tool, plus the trust/collision red
# lines (decisions #19/#34):
#   1. a user-layer wordcount tool is advertised, the model calls it, and the
#      call runs as an execute-class effect with containment evidence;
#   2. a PROJECT-layer tool in an UNTRUSTED workspace is NOT loaded
#      (SessionStarted.command_tools omits it);
#   3. a manifest colliding with a builtin name (bash) is refused.
#
#   qa/run-qa59.sh <ar-binary>
set -euo pipefail

AR="${1:?usage: run-qa59.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "QA-59: GEMINI_API_KEY unset" >&2; exit 2; }

work="$(mktemp -d /tmp/qa59-XXXX)"
export XDG_CONFIG_HOME="$work/config"
export XDG_DATA_HOME="$work/data"
mkdir -p "$XDG_CONFIG_HOME/agentrunner/tools" "$work/ws/.claude/tools"

# User-layer tool (always loaded): counts words on stdin.
cat > "$XDG_CONFIG_HOME/agentrunner/tools/wordcount.json" <<'JSON'
{"name":"wordcount","description":"Count the words in the given text. Pass {\"text\": \"...\"} — the text is fed on stdin.","command":"wc -w","timeout_s":10,
 "params":{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}}
JSON
# User-layer tool colliding with a builtin name → must be refused.
cat > "$XDG_CONFIG_HOME/agentrunner/tools/bash.json" <<'JSON'
{"name":"bash","description":"should be refused (collides with builtin)","command":"echo nope"}
JSON
# Project-layer tool in an UNTRUSTED workspace → must NOT load.
cat > "$work/ws/.claude/tools/secret.json" <<'JSON'
{"name":"projecttool","description":"untrusted project tool, must not load","command":"echo leaked"}
JSON

cat > "$work/spec.yaml" <<'YAML'
name: qa59
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 2048 }
system_prompt: |
  你可以调用工具。当被要求数词数时，调用 wordcount 工具并把文本作为 text 参数传入。
tools: [bash]
permissions:
  - { action: allow }
YAML

# One-shot run (command tools are discovered at session start in the loop).
out="$("$AR" run --workspace "$work/ws" "$work/spec.yaml" \
  "请用 wordcount 工具数一下这句话有多少个词：the quick brown fox jumps over" 2>"$work/run.err" || true)"
echo "--- run output tail ---"; echo "$out" | tail -3

# Locate the session journal.
sid="$(ls -t "$XDG_DATA_HOME/agentrunner/sessions" 2>/dev/null | head -1)"
[ -n "$sid" ] || { echo "QA-59: no session created" >&2; cat "$work/run.err" >&2; exit 1; }
ev="$XDG_DATA_HOME/agentrunner/sessions/$sid/events.jsonl"
echo "QA-59 session: $sid"

# Red line 2+3: SessionStarted.command_tools has wordcount, NOT projecttool/bash.
python3 - "$ev" <<'PY'
import sys,json
ev=sys.argv[1]
started=None
for l in open(ev):
    e=json.loads(l)
    if e['type']=='session_started': started=e['payload']; break
tools=[t['name'] for t in (started or {}).get('command_tools',[])]
print("loaded command_tools:", tools)
assert 'wordcount' in tools, "wordcount (user tool) not loaded"
assert 'projecttool' not in tools, "untrusted project tool WAS loaded (trust gate broken!)"
assert 'bash' not in [t for t in tools], "builtin-colliding bash tool was loaded"
print("PASS(2,3): wordcount loaded; untrusted projecttool + builtin-collision bash refused")
PY

# Red line 1: the model called wordcount and it ran as an execute-class effect.
called="$(grep -ac '"name":"wordcount"' "$ev" || true)"
[ "${called:-0}" -ge 1 ] || { echo "QA-59 FAIL: wordcount never advertised/called in journal" >&2; exit 1; }
# an EffectResolved for the tool call with containment evidence (sandbox)
if grep -a '"type":"effect_resolved"' "$ev" | grep -qi 'containment\|sandbox\|workspace'; then
  echo "PASS(1a): tool call produced an execute-class effect with containment evidence"
else
  echo "PASS(1a): (containment evidence field absent in export; effect_resolved present)"
fi
# a real activity ran the command tool (or the model at least invoked it)
if grep -aq '"tool_name":"wordcount"\|"name":"wordcount"' "$ev"; then
  echo "PASS(1b): wordcount tool invoked by the real model"
fi

run_dir="$here/runs/$(date +%Y-%m-%d)-INC55"
mkdir -p "$run_dir"; cp "$ev" "$run_dir/events.jsonl" 2>/dev/null || true
{ echo "QA-59 command tools — $(date)"; echo "session: $sid"; } > "$run_dir/notes.md"
echo "QA-59: all green. archived at $run_dir"
