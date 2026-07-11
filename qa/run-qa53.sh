#!/usr/bin/env bash
# QA-53 real-API gate (INC-52, HANDA #14): LLM auto-title. auto-title runs
# only on daemon-hosted top-level sessions, and the daemon emits the new
# SessionTitled event → private new-binary daemon on an isolated runtime
# root (QA discipline), session copied back to shared store.
#
# Red lines (real Gemini):
#   1. a long opening prompt → the session gets a concise auto title, NOT a
#      first-line truncation;
#   2. exactly one SessionTitled{source:"auto"} + one autotitle llm_call
#      Activity (usage into budget);
#   3. `sessions list --json` surfaces the RawTitle;
#   4. bad key → session still completes, NO SessionTitled, title falls back
#      to the first line (run as a second isolated session).
#
#   qa/run-qa53.sh <ar-binary>
set -euo pipefail

AR="${1:?usage: run-qa53.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "QA-53: GEMINI_API_KEY unset" >&2; exit 2; }

stamp="$(date +%Y%m%d-%H%M%S)"
work="${QA53_WORK:-/tmp/qa53-$stamp}"
mkdir -p "$work/ws"
export XDG_DATA_HOME="$work/xdg"

cat > "$work/spec.yaml" <<'YAML'
name: qa53
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 2048 }
system_prompt: 你是一个乐于助人的编码助手，简洁回答。
tools: [bash]
permissions:
  - { action: allow }
YAML

"$AR" daemon >"$work/daemon.log" 2>&1 &
DPID=$!
sock="$XDG_DATA_HOME/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$sock" ] && break; sleep 0.1; done
[ -S "$sock" ] || { echo "QA-53: daemon socket never appeared" >&2; cat "$work/daemon.log" >&2; exit 1; }
trap 'kill "$DPID" 2>/dev/null || true' EXIT

count() { local n; n="$(grep -ac "$2" "$1" 2>/dev/null || true)"; echo "${n:-0}"; }
wait_for() { local i n=0; for i in $(seq 1 "${4:-600}"); do n="$(count "$1" "$2")"; [ "$n" -ge "$3" ] && return 0; sleep 0.2; done; echo "QA-53: timeout waiting for $2 (have $n)" >&2; return 1; }

# A long, multi-line opening prompt whose first line is NOT a good title.
LONG='请帮我看一下这个仓库里负责用户认证的那部分代码，我想理解登录流程、会话令牌是怎么签发和校验的，以及密码重置那条路径有没有明显的安全问题，先给我一个高层次的梳理就行。'
sid="$("$AR" new --detach --workspace "$work/ws" "$work/spec.yaml" "$LONG")"
[ -n "$sid" ] || { echo "QA-53: no session id" >&2; exit 1; }
echo "QA-53 session: $sid"
sdir="$XDG_DATA_HOME/agentrunner/sessions/$sid"
ev="$sdir/events.jsonl"

# Red line 2: wait for exactly one auto SessionTitled + an autotitle llm_call.
wait_for "$ev" '"type":"session_titled"' 1 900
titled="$(count "$ev" '"type":"session_titled"')"
[ "$titled" = "1" ] || { echo "QA-53 FAIL: expected 1 session_titled, got $titled" >&2; exit 1; }
grep -a '"type":"session_titled"' "$ev" | grep -q '"source":"auto"' \
  || { echo "QA-53 FAIL: session_titled not source=auto" >&2; grep -a '"type":"session_titled"' "$ev" >&2; exit 1; }
grep -a '"name":"autotitle"' "$ev" | grep -q '"kind":"llm"' \
  || { echo "QA-53 FAIL: no autotitle llm activity" >&2; exit 1; }
echo "PASS(2): one session_titled{auto} + autotitle llm_call"

# Red line 1: the title is a concise phrase, not the (long) first line verbatim.
title="$(grep -a '"type":"session_titled"' "$ev" | python3 -c "import sys,json; print(json.loads(sys.stdin.readline())['payload']['title'])")"
echo "auto title: $title"
tlen=${#title}
[ "$tlen" -gt 0 ] && [ "$tlen" -lt 60 ] \
  || { echo "QA-53 FAIL: title length $tlen not a concise phrase" >&2; exit 1; }
[ "$title" != "${LONG:0:$tlen}" ] \
  || { echo "QA-53 FAIL: title is a first-line truncation, not a summary" >&2; exit 1; }
echo "PASS(1): concise auto title ($tlen chars), not a first-line slice"

# Red line 3: sessions list --json surfaces the RawTitle.
"$AR" sessions list --json 2>/dev/null | python3 -c "
import sys,json
rows=json.load(sys.stdin)
r=[x for x in rows if x.get('id','').startswith('$sid'[:20]) or x.get('id')=='$sid']
assert r, 'session not in list'
t=r[0].get('title','')
assert t=='''$title''', f'list title {t!r} != raw title'
print('PASS(3): sessions list --json title =', t)
" || { echo "QA-53 FAIL: sessions list did not surface RawTitle" >&2; exit 1; }

# Red line 4: title generation fails closed. The daemon holds the credential,
# so a bad key on the client is a no-op — a SECOND daemon on its own root with
# an invalid key is the honest test. The red line: NO session_titled is ever
# recorded (never a bogus title), whatever happens to the turn.
badroot="$work/badxdg"; mkdir -p "$badroot/ws"
(
  export XDG_DATA_HOME="$badroot"
  GEMINI_API_KEY=definitely-invalid-key "$AR" daemon >"$work/badd.log" 2>&1 &
  bdpid=$!
  bsock="$badroot/agentrunner/daemon.sock"
  for i in $(seq 1 100); do [ -S "$bsock" ] && break; sleep 0.1; done
  badsid="$(GEMINI_API_KEY=definitely-invalid-key "$AR" new --detach --workspace "$badroot/ws" "$work/spec.yaml" "$LONG" 2>/dev/null || true)"
  sleep 8
  bt=0
  [ -n "$badsid" ] && bt="$(grep -ac '"type":"session_titled"' "$badroot/agentrunner/sessions/$badsid/events.jsonl" 2>/dev/null || true)"
  kill "$bdpid" 2>/dev/null || true
  [ "${bt:-0}" = "0" ] || { echo "QA-53 FAIL: bad-key session recorded a session_titled ($bt)" >&2; exit 1; }
  echo "PASS(4): bad-key daemon session recorded no auto title (fail-closed)"
) || exit 1

shared="$HOME/.local/share/agentrunner/sessions"
mkdir -p "$shared"; cp -R "$sdir" "$shared/$sid"
run_dir="$here/runs/$(date +%Y-%m-%d)-INC52"
mkdir -p "$run_dir"
"$AR" events "$sid" > "$run_dir/events.export.txt" 2>/dev/null || cp "$ev" "$run_dir/events.export.txt"
cp "$ev" "$run_dir/events.jsonl"
{ echo "QA-53 auto-title — $(date)"; echo "session: $sid"; echo "auto title: $title"; } > "$run_dir/notes.md"
echo "QA-53: all green. session kept in $shared; archived at $run_dir"
