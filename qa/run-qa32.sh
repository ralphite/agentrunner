#!/usr/bin/env bash
# QA-32 real-API gate (INC-25, #78): the shipped built-in read-only agent —
# a parent whitelists `explore` (agents: [explore]) with NO sibling
# explore.yaml on disk, the live model spawns it, and the child runs the
# shipped read-only face. Journal red lines:
#   1. a child session is spawned though no explore.yaml exists next to the
#      spec — the ONLY resolution is the shipped built-in;
#   2. the child's tool face is read-only — it calls read_file/grep/glob and
#      NEVER edit_file/write_file/bash (the safety argument for shipping it
#      open);
#   3. the lead receives the child's findings (subagent_completed) and answers.
#
# Model inheritance (child runs on the PARENT's model, not the built-in's
# gemini default) is proven precisely by the twin
# TestResolverPrefersBuiltinAndInheritsModel; here both are gemini so it is
# only noted.
#
# WHY a private daemon (unlike the shared-store default): this feature lives
# in the daemon's spawn/resolver path, so it is only exercisable by a daemon
# running THIS binary. The resident shared daemon runs an older binary, and
# restarting it would disrupt the user's live sessions (CLAUDE.md 破坏性例外).
# So we mirror QA-31: a private daemon on an isolated runtime root runs the
# new binary, then the resulting session (+ child) is copied into the shared
# store so it stays visible in `ar sessions` / webui.
#
#   qa/run-qa32.sh <ar-binary>
set -euo pipefail
QA=QA-32
AR="${1:?usage: run-qa32.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"

# .env for GEMINI_API_KEY; a worktree keeps its .env in the MAIN checkout.
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
if [ -z "${GEMINI_API_KEY:-}" ]; then
  main_root="$(cd "$here/.." && dirname "$(git rev-parse --git-common-dir)")"
  [ -f "$main_root/.env" ] && { set -a; . "$main_root/.env"; set +a; }
fi
[ -n "${GEMINI_API_KEY:-}" ] || { echo "$QA: GEMINI_API_KEY unset" >&2; exit 2; }

stamp="$(date +%Y%m%d-%H%M%S)"
work="${QA32_WORK:-/tmp/qa32-$stamp}"
mkdir -p "$work/ws"
export XDG_DATA_HOME="$work/xdg"

count_type() { local n; n="$(grep -c "\"type\":\"$1\"" "$2" 2>/dev/null)" || n=0; printf '%s' "${n:-0}"; }

# A workspace file with a distinctive symbol the explorer must locate.
cat > "$work/ws/registry.go" <<'GO'
package registry

// FROBNICATE_LIMIT caps how many widgets a batch may frobnicate before the
// scheduler forces a flush. Tuned empirically; do not raise without a bench.
const FROBNICATE_LIMIT = 512

func frobnicate(batch []Widget) int {
	n := 0
	for _, w := range batch {
		if n >= FROBNICATE_LIMIT {
			flush()
			n = 0
		}
		w.apply()
		n++
	}
	return n
}
GO

# Parent whitelists the built-in `explore`. Crucially, NO explore.yaml is
# written next to this spec — the built-in is the only way `explore` resolves.
cat > "$work/lead.yaml" <<'YAML'
name: qa32-lead
description: lead that delegates read-only exploration to the built-in explorer
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 1024 }
system_prompt: |
  你不要自己读代码库。需要在代码里查找东西时,用 spawn_agent 委派给
  名为 explore 的只读子 agent(它随发行内置,你无需自带它的 spec):
  给它清晰的 prompt,让它去定位符号、读代码、报告 file:line。
  拿到它的 findings 后,用一句话把结论转告用户。
tools: [read_file]
agents: [explore]
permissions:
  - { action: allow }
YAML

# Guard the premise: there must be NO sibling explore.yaml. If one exists the
# test proves nothing.
[ ! -f "$work/explore.yaml" ] || { echo "$QA: premise broken — explore.yaml exists next to spec" >&2; exit 1; }

# Private daemon on the isolated root, running THIS binary.
"$AR" daemon >"$work/daemon.log" 2>&1 &
DPID=$!
sock="$XDG_DATA_HOME/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$sock" ] && break; sleep 0.1; done
[ -S "$sock" ] || { echo "$QA: daemon socket never appeared" >&2; cat "$work/daemon.log" >&2; exit 1; }
trap 'kill "$DPID" 2>/dev/null || true' EXIT

sid="$("$AR" new --detach --workspace "$work/ws" "$work/lead.yaml" \
  "委派内置 explore 子 agent 去代码库里找到 FROBNICATE_LIMIT 这个常量:它的值是多少、在哪个文件、干什么用的。等它报告后用一句话告诉我结论。" 2>/dev/null | head -1)"
[ -n "$sid" ] || { echo "$QA: no session id" >&2; exit 1; }
SDIR="$XDG_DATA_HOME/agentrunner/sessions/$sid"
echo "$QA session: $sid"
echo "$QA workspace: $work/ws (kept)"

# Wait for the delegation to settle: a child receipt on the lead + an idle
# lead journal tail — bounded by a real-API budget of 150s.
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
check "lead journal exists" '[ -f "$EV" ]'
check "no sibling explore.yaml existed (built-in was the only source)" '[ ! -f "$work/explore.yaml" ]'

# Red line 0: the spawn did NOT fail with a missing-spec error (the built-in
# resolved). A regression here reproduces the pre-fix behavior exactly.
if grep -q 'open .*explore.yaml: no such file' "$EV"; then
  note "FAIL  spawn fell through to sibling file — built-in did NOT resolve"
  fail=1
else
  note "PASS  spawn did not fall through to a missing sibling explore.yaml"
fi

# Red line 1: a child session was spawned (the built-in explore resolved).
child_dir="$(ls -d "$SDIR"/sub/* 2>/dev/null | head -1)"
check "built-in explore spawned (child session exists)" '[ -n "$child_dir" ]'
check "lead received a child receipt" '[ "$(count_type subagent_completed "$EV")" -ge 1 ]'

# Red line 2: the child's tool face is read-only.
if [ -n "$child_dir" ]; then
  CEV="$child_dir/events.jsonl"
  if grep -qE '"name":"(edit_file|write_file|bash|apply_patch)"' "$CEV"; then
    note "FAIL  child invoked a WRITE tool — built-in must be read-only"
    grep -E '"name":"(edit_file|write_file|bash|apply_patch)"' "$CEV" | head >&2
    fail=1
  else
    note "PASS  child never invoked a write tool (read-only face held)"
  fi
  check "child used a read tool (read_file/grep/glob/keyword_search)" \
    'grep -qE "\"name\":\"(read_file|grep|glob|keyword_search|semantic_search)\"" "$CEV"'
  check "child ran the explore system prompt (exactly one SessionStarted)" \
    '[ "$(count_type session_started "$CEV")" -eq 1 ]'
fi

# Red line 3: the lead's final answer reflects the findings.
last="$(grep '"type":"assistant_message"' "$EV" | tail -1)"
if printf '%s' "$last" | grep -qE '512|FROBNICATE|registry'; then
  note "PASS  lead answer reflects the explorer's findings (512/FROBNICATE/registry)"
else
  note "NOTE  lead answer did not name the value; child findings still returned"
  check "lead produced a final answer" '[ -n "$last" ]'
fi

# Copy the session (+ child) into the shared store so it stays visible in the
# user's daily CLI / webui, then archive the export.
shared="$HOME/.local/share/agentrunner/sessions"
mkdir -p "$shared"; cp -R "$SDIR" "$shared/$sid" 2>/dev/null || true
run_dir="$here/runs/$(date +%Y-%m-%d)-QA32"
mkdir -p "$run_dir"
"$AR" events "$sid" > "$run_dir/events.export.jsonl" 2>/dev/null || cp "$EV" "$run_dir/events.export.jsonl"
[ -n "${child_dir:-}" ] && cp "$child_dir/events.jsonl" "$run_dir/child.events.jsonl" 2>/dev/null || true
{ echo "$QA built-in read-only agent (explore) — $(date)"; echo "session: $sid"; echo "workspace: $work/ws"; } > "$run_dir/notes.md"

if [ "$fail" -eq 0 ]; then
  note "all green. session copied to $shared/$sid; export archived at $run_dir"
else
  note "one or more red lines FAILED (session kept at $SDIR and $shared/$sid)"; exit 1
fi
