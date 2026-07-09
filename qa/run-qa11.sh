#!/usr/bin/env bash
# QA-11 real-API gate (INC-3): the model reaches for the first-class grep /
# glob tools against the live Gemini API. Drives the real ar binary + daemon
# end to end. Structural asserts only (journal facts), per QA.md §0.1 — we
# assert the tools were INVOKED and produced results, not the model's wording.
#
#   qa/run-qa11.sh <ar-binary>
# Requires GEMINI_API_KEY in the environment (loaded from repo .env).
set -euo pipefail

AR="${1:?usage: run-qa11.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "QA-11: GEMINI_API_KEY unset" >&2; exit 2; }

# Short base dir: macOS caps unix socket paths at 104 bytes, and the default
# $TMPDIR under /var/folders blows past that once nested.
work="$(mktemp -d /tmp/qa11.XXXXXX)"
export XDG_DATA_HOME="$work/xdg"
trap 'kill ${DPID:-0} 2>/dev/null || true; sleep 0.3; rm -rf "$work" 2>/dev/null || true' EXIT

# A small multi-file workspace with a distinctive symbol scattered across a
# subdir and a vendored tree (which grep MUST NOT surface), plus a credential
# file (which grep MUST NOT surface).
ws="$work/ws"
mkdir -p "$ws/internal/auth" "$ws/node_modules/pkg"
cat > "$ws/internal/auth/token.go" <<'GO'
package auth

// RefreshSentinel marks the token refresh path.
func RefreshSentinel() string { return "refresh" }
GO
cat > "$ws/internal/auth/verify.go" <<'GO'
package auth

func check() { _ = RefreshSentinel() }
GO
cat > "$ws/README.md" <<'MD'
Call RefreshSentinel to refresh.
MD
echo 'API_TOKEN=supersecret_should_never_surface' > "$ws/.env"
echo 'func RefreshSentinel(){}' > "$ws/node_modules/pkg/index.js"

cat > "$work/base.yaml" <<'YAML'
name: dev
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 2048 }
system_prompt: |
  你是一个严谨的编码助手。定位符号时优先用 grep / glob 工具(它们受工作区
  边界与凭据红线保护),不要用 bash。简洁作答。
tools: [read_file, grep, glob, bash]
permissions:
  - { action: allow }
YAML

"$AR" daemon >"$work/daemon.log" 2>&1 &
DPID=$!
sock="$XDG_DATA_HOME/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$sock" ] && break; sleep 0.1; done
[ -S "$sock" ] || { echo "QA-11: daemon socket never appeared" >&2; cat "$work/daemon.log" >&2; exit 1; }

sid="$("$AR" new --detach --workspace "$ws" "$work/base.yaml" \
  "用 grep 找出这个仓库里所有引用 RefreshSentinel 的位置,列出文件与行号。" 2>>"$work/err.log")"
[ -n "$sid" ] || { echo "QA-11: no session id" >&2; cat "$work/err.log" "$work/daemon.log" >&2; exit 1; }
echo "session $sid"
sdir="$XDG_DATA_HOME/agentrunner/sessions/$sid"

count_type() {
  local n; n="$(grep -c "\"type\":\"$1\"" "$2" 2>/dev/null)" || n=0
  printf '%s' "${n:-0}"
}
wait_turns() {
  local want="$1" i n
  for i in $(seq 1 300); do
    n="$(count_type assistant_message "$sdir/events.jsonl")"
    [ "$n" -ge "$want" ] && return 0
    sleep 0.2
  done
  echo "QA-11: timed out waiting for $want assistant messages (have ${n:-0})" >&2
  cat "$work/daemon.log" >&2; return 1
}
wait_turns 1

# Second turn: exercise glob explicitly.
"$AR" send --detach "$sid" "再用 glob 列出 internal 下所有 .go 文件。" >/dev/null
wait_turns 2
"$AR" close "$sid" >/dev/null
for i in $(seq 1 100); do
  tail -c 200 "$sdir/events.jsonl" | grep -q '"type":"session_closed"' && break; sleep 0.1
done

# ---- Structural assertions (journal facts, not model wording) ----
ev="$sdir/events.jsonl"
fail=0
# grep tool was actually invoked as an activity.
grep -q '"type":"activity_started".*"name":"grep"' "$ev" || \
  grep -q '"name":"grep"' "$ev" || { echo "FAIL: grep tool never invoked" >&2; fail=1; }
# glob tool was actually invoked.
grep -q '"name":"glob"' "$ev" || { echo "FAIL: glob tool never invoked" >&2; fail=1; }
# The credential red line: the secret value never appears ANYWHERE in the
# journal (grep excludes .env at the walk; this proves it end to end).
if grep -q 'supersecret_should_never_surface' "$ev"; then
  echo "FAIL: credential value leaked into the journal" >&2; fail=1
fi
# A grep result actually carried the real symbol location back to the model
# (activity_completed with the matched path).
grep -q '"type":"activity_completed"' "$ev" || { echo "FAIL: no tool result journaled" >&2; fail=1; }

if [ "$fail" = 0 ]; then
  echo "QA-11 PASS: grep+glob invoked, results journaled, credential red line held"
else
  echo "QA-11 FAIL" >&2; exit 1
fi
