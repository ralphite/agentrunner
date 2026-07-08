#!/usr/bin/env bash
# QA-10: session 内换 agent(决策 #32,UJ-11)——同一会话、用户免确认、
# prefix 显式换代、上下文延续。真实 API。
# Requires GEMINI_API_KEY in the environment (loaded from repo .env).
set -euo pipefail
QA="QA-10"
here="$(cd "$(dirname "$0")" && pwd)"
AR="${1:?usage: run-qa10.sh <ar-binary>}"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "$QA: GEMINI_API_KEY unset" >&2; exit 2; }

work="$(mktemp -d)"
export XDG_DATA_HOME="$work/xdg"
trap 'kill ${DPID:-0} 2>/dev/null || true; sleep 0.3; rm -rf "$work" 2>/dev/null || true' EXIT
mkdir -p "$work/ws"

cat > "$work/poet.yaml" <<'YAML'
name: poet
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 1024 }
system_prompt: 你是诗人。永远以"【诗人】"开头回答,只用一句诗回应。
permissions:
  - { action: allow }
YAML
cat > "$work/auditor.yaml" <<'YAML'
name: auditor
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 1024 }
system_prompt: 你是审计员。永远以"【审计员】"开头回答,用一句话严肃作答。
permissions:
  - { action: allow }
YAML

"$AR" daemon >"$work/daemon.log" 2>&1 & DPID=$!
for i in $(seq 1 50); do "$AR" sessions list >/dev/null 2>&1 && break; sleep 0.1; done

# 1. 诗人身份开场。
out1="$("$AR" new --workspace "$work/ws" "$work/poet.yaml" "你好,介绍一下你自己" 2>"$work/e1")"
sid="$(grep -o 'session [a-z0-9-]*' "$work/e1" | head -1 | cut -d' ' -f2)"
[ -n "$sid" ] || { echo "$QA: FAIL no session id" >&2; exit 1; }
echo "$out1" | grep -q "【诗人】" || { echo "$QA: FAIL opening reply not from poet: $out1" >&2; exit 1; }

# 2. 切换 agent——单条命令,无确认交互。
"$AR" agent "$sid" "$work/auditor.yaml" | grep -q "agent switched to auditor" || {
  echo "$QA: FAIL agent switch not acknowledged" >&2; exit 1; }

# 3. 同一 session 续聊:新身份作答,且上下文延续(runtime 红线只钉
#    spec_changed 落盘与新身份前缀,不钉模型对上文的措辞)。
out2="$("$AR" send "$sid" "现在你是谁?只报身份。" 2>/dev/null)"
fail=0
echo "$out2" | grep -q "【审计员】" || { echo "$QA: FAIL post-switch reply not from auditor: $out2" >&2; fail=1; }
sdir="$XDG_DATA_HOME/agentrunner/sessions/$sid"
n="$(grep -c '"type":"spec_changed"' "$sdir/events.jsonl" || true)"
[ "$n" = 1 ] || { echo "$QA: FAIL spec_changed = $n, want exactly 1" >&2; fail=1; }
grep -q '"spec_name":"auditor"' "$sdir/events.jsonl" || { echo "$QA: FAIL new spec identity not journaled" >&2; fail=1; }
# 同一 journal:开场与切换后的回答都在(会话未分叉、未新建)。
starts="$(grep -c '"type":"session_started"' "$sdir/events.jsonl" || true)"
[ "$starts" = 1 ] || { echo "$QA: FAIL session_started = $starts, want 1 (same session)" >&2; fail=1; }

[ "$fail" = 0 ] && echo "$QA: PASS (poet → auditor in one session, context carried)"
exit "$fail"
