#!/usr/bin/env bash
# QA-23 real-API gate (INC-14, UJ-09, G9): memory writeback — `ar remember`
# writes a note to the workspace-root CLAUDE.md and a LATER session, freezing
# that file into its prompt prefix, honors it. Runtime red lines:
#   1. remember writes the note under "## Remembered" in <ws>/CLAUDE.md;
#   2. the remember control surfaces a program-source input in session 1
#      (this-run visibility);
#   3. a FRESH session over the same workspace, asked which package manager to
#      use, answers pnpm — the remembered constraint reached the model through
#      the frozen memory prefix (cross-session persistence, the whole point).
#
# Private daemon on an isolated runtime root (the shared daemon serves other
# live sessions); sessions copied into the shared store + export archived.
#
#   qa/run-qa23.sh <ar-binary>
set -euo pipefail

AR="${1:?usage: run-qa23.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "QA-23: GEMINI_API_KEY unset" >&2; exit 2; }

stamp="$(date +%Y%m%d-%H%M%S)"
work="${QA23_WORK:-/tmp/qa23-$stamp}"
mkdir -p "$work"
export XDG_DATA_HOME="$work/xdg"
# No cleanup trap (project QA rule): session data is kept and copied to the
# shared store below. Only the private daemon is stopped.

mkdir -p "$work/ws"
cat > "$work/spec.yaml" <<'YAML'
name: qa23
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 512 }
system_prompt: |
  你是一个严谨的编码助手。严格遵循项目记忆（CLAUDE.md / <memory> 块）里的
  约定作答。简洁作答。
tools: [read_file]
permissions:
  - { action: allow }
YAML

"$AR" daemon >"$work/daemon.log" 2>&1 &
DPID=$!
sock="$XDG_DATA_HOME/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$sock" ] && break; sleep 0.1; done
[ -S "$sock" ] || { echo "QA-23: daemon socket never appeared" >&2; cat "$work/daemon.log" >&2; exit 1; }
trap 'kill "$DPID" 2>/dev/null || true' EXIT

count_type() { local n; n="$(grep -c "\"type\":\"$1\"" "$1x" 2>/dev/null)" || n=0; printf '%s' "${n:-0}"; }
asst_count() { grep -c '"type":"assistant_message"' "$1" 2>/dev/null || echo 0; }
wait_asst() { # $1=events.jsonl $2=want
  local i n
  for i in $(seq 1 400); do
    n="$(asst_count "$1")"
    [ "$n" -ge "$2" ] && return 0
    sleep 0.2
  done
  echo "QA-23: timed out waiting for $2 assistant messages in $1 (got $n)" >&2
  exit 1
}

# --- Session 1: establish, then remember a constraint ---
sid1="$("$AR" new --detach --workspace "$work/ws" "$work/spec.yaml" "你好，先自我介绍一句。")"
[ -n "$sid1" ] || { echo "QA-23: no session id" >&2; exit 1; }
echo "QA-23 session 1: $sid1"
sdir1="$XDG_DATA_HOME/agentrunner/sessions/$sid1"
wait_asst "$sdir1/events.jsonl" 1

NOTE="本项目一律使用 pnpm 作为包管理器，禁止使用 npm 或 yarn"
"$AR" remember "$sid1" "$NOTE" >/dev/null
wait_asst "$sdir1/events.jsonl" 2   # remember's confirmation turn

# Red line 1: the note is in the workspace CLAUDE.md under Remembered.
CLAUDE_MD="$work/ws/CLAUDE.md"
if ! grep -q "## Remembered" "$CLAUDE_MD" 2>/dev/null || ! grep -q "pnpm" "$CLAUDE_MD" 2>/dev/null; then
  echo "QA-23 FAIL: note not written to $CLAUDE_MD" >&2; cat "$CLAUDE_MD" 2>/dev/null >&2; exit 1
fi
echo "PASS(1): note written to workspace CLAUDE.md under ## Remembered"

# Red line 2: session 1 saw a program-source input carrying the note.
if ! grep '"type":"input_received"' "$sdir1/events.jsonl" | grep -q '"source":"program"'; then
  echo "QA-23 FAIL: no program-source input in session 1" >&2; exit 1
fi
if ! grep '"type":"input_received"' "$sdir1/events.jsonl" | grep -q "pnpm"; then
  echo "QA-23 FAIL: program input did not carry the note" >&2; exit 1
fi
echo "PASS(2): remember surfaced a program-source input in session 1"

# --- Session 2 (FRESH): the remembered constraint reaches the model ---
sid2="$("$AR" new --detach --workspace "$work/ws" "$work/spec.yaml" \
  "这个项目应该用哪个包管理器？只回答包管理器的名字。")"
[ -n "$sid2" ] || { echo "QA-23: no session 2 id" >&2; exit 1; }
echo "QA-23 session 2: $sid2"
sdir2="$XDG_DATA_HOME/agentrunner/sessions/$sid2"
wait_asst "$sdir2/events.jsonl" 1

reply2="$(grep '"type":"assistant_message"' "$sdir2/events.jsonl" | tail -1)"
if ! printf '%s' "$reply2" | grep -qi "pnpm"; then
  echo "QA-23 FAIL: fresh session did not honor the remembered constraint" >&2
  printf '%s\n' "$reply2" >&2
  exit 1
fi
echo "PASS(3): fresh session honored the remembered pnpm constraint"

# Keep evidence: copy both sessions into the shared store, archive exports.
shared="$HOME/.local/share/agentrunner/sessions"
mkdir -p "$shared"
cp -R "$sdir1" "$shared/$sid1"
cp -R "$sdir2" "$shared/$sid2"
run_dir="$here/runs/$(date +%Y-%m-%d)-QA23"
mkdir -p "$run_dir"
"$AR" events "$sid1" > "$run_dir/session1.events.jsonl" 2>/dev/null || cp "$sdir1/events.jsonl" "$run_dir/session1.events.jsonl"
"$AR" events "$sid2" > "$run_dir/session2.events.jsonl" 2>/dev/null || cp "$sdir2/events.jsonl" "$run_dir/session2.events.jsonl"
cp "$CLAUDE_MD" "$run_dir/CLAUDE.md"
{
  echo "QA-23 memory writeback — $(date)"
  echo "session1 (remember): $sid1"
  echo "session2 (fresh, honored): $sid2"
  echo "note: $NOTE"
} > "$run_dir/notes.md"
echo "QA-23: all green. sessions kept in $shared; export archived at $run_dir"
