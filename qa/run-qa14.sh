#!/usr/bin/env bash
# QA-14 real-API gate (INC-5): a COMPLETE, real coding agent drives an entire
# feature to green against the live Gemini API — exercising the full tool face
# in one agentic flow:
#   grep/glob/read_file (explore) → web_fetch (consult the real spec) →
#   ask_user (align on the approach, answered via the inbox) →
#   write_file/edit_file (implement) → bash `go test` (verify) → report.
# The hard proof is the WORKSPACE TEST going green — the agent did real work,
# not just called tools. Structural asserts on journal facts per QA.md §0.1.
#
#   qa/run-qa14.sh <ar-binary>
# Requires GEMINI_API_KEY (repo .env), python3 (serve the spec page), go.
set -euo pipefail

AR="${1:?usage: run-qa14.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "QA-14: GEMINI_API_KEY unset" >&2; exit 2; }
command -v python3 >/dev/null || { echo "QA-14: python3 required" >&2; exit 2; }
command -v go >/dev/null || { echo "QA-14: go required" >&2; exit 2; }

work="$(mktemp -d /tmp/qa14.XXXXXX)"
export XDG_DATA_HOME="$work/xdg"
trap 'kill ${DPID:-0} ${HPID:-0} 2>/dev/null || true; sleep 0.3; rm -rf "$work" 2>/dev/null || true' EXIT

# Real Go project: Compare() is a panic stub, the tests are RED, and getting
# pre-release ordering right REQUIRES consulting the spec (not guessable).
ws="$work/ws"
mkdir -p "$ws"
cp "$here/fixtures/semver-broken/go.mod" "$here/fixtures/semver-broken/version.go" \
   "$here/fixtures/semver-broken/version_test.go" "$ws/"

# Baseline: tests MUST be red (the stub panics).
if (cd "$ws" && go test ./... >/dev/null 2>&1); then
  echo "QA-14: baseline is green — fixture stub missing" >&2; exit 2
fi

# Serve the real semver §11 spec page locally (no external dependency).
site="$here/fixtures/semver-broken"
port=18914
( cd "$site" && python3 -m http.server "$port" --bind 127.0.0.1 >"$work/http.log" 2>&1 ) &
HPID=$!
url="http://127.0.0.1:$port/semver-spec.html"
for i in $(seq 1 100); do curl -sf "$url" >/dev/null 2>&1 && break; sleep 0.1; done
curl -sf "$url" >/dev/null 2>&1 || { echo "QA-14: spec server never came up" >&2; cat "$work/http.log" >&2; exit 1; }

cat > "$work/coder.yaml" <<YAML
name: coder
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 4096 }
system_prompt: |
  你是一名严谨的资深 Go 工程师。工作纪律:
  - 先探索再动手:用 grep/glob/read_file 理解现有代码与测试到底期望什么。
  - 不确定的事实(尤其规范细节)用 web_fetch 查证,绝不凭记忆猜。
  - 关键设计决策(容易搞错的算法)动手前先用 ask_user 与用户对齐方案。
  - 改完必须用 bash 跑测试验证;测试不全绿就不算完成,继续修。
  - 如实汇报:做了什么、测试证据、失败也照说,不粉饰。
tools: [read_file, write_file, edit_file, bash, grep, glob, keyword_search, web_fetch, ask_user]
permissions:
  - { action: allow }
max_generation_steps: 60
YAML

"$AR" daemon >"$work/daemon.log" 2>&1 &
DPID=$!
sock="$XDG_DATA_HOME/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$sock" ] && break; sleep 0.1; done
[ -S "$sock" ] || { echo "QA-14: daemon socket never appeared" >&2; cat "$work/daemon.log" >&2; exit 1; }

prompt="version.go 里的 Compare 函数还没实现(现在直接 panic),version_test.go 的测试全挂。请完成它:
1. 先用 grep/glob/read_file 看清测试到底期望怎样的排序。
2. semver 的 pre-release 优先级规则很微妙、容易搞错——用 web_fetch 抓取 $url 查准确规则,不要凭记忆。
3. 动手写 pre-release 比较之前,先用 ask_user 把你的实现方案简述给我确认。
4. 实现 Compare,用 bash 跑 go test ./... 验证,全绿才算完成,如实汇报测试结果。"

sid="$("$AR" new --detach --workspace "$ws" "$work/coder.yaml" "$prompt" 2>>"$work/err.log")"
[ -n "$sid" ] || { echo "QA-14: no session id" >&2; cat "$work/err.log" "$work/daemon.log" >&2; exit 1; }
echo "session $sid"
sdir="$XDG_DATA_HOME/agentrunner/sessions/$sid"
ev="$sdir/events.jsonl"

wait_park() {
  local i
  for i in $(seq 1 600); do
    grep -q '"type":"waiting_entered"' "$ev" 2>/dev/null && grep '"type":"waiting_entered"' "$ev" | grep -q 'question' && return 0
    grep -q '"type":"actor_crashed"' "$ev" 2>/dev/null && { echo "QA-14: crashed before park" >&2; return 1; }
    # already idle without asking? the agent skipped ask_user — let the run finish, assert later
    tail -c 300 "$ev" 2>/dev/null | grep -q '"type":"session_closed"' && return 0
    sleep 0.3
  done
  echo "QA-14: timed out waiting for ask_user park" >&2; tail -20 "$work/daemon.log" >&2; return 1
}
wait_park

# If it parked on a question, approve the approach via the inbox.
if grep '"type":"waiting_entered"' "$ev" | grep -q 'question'; then
  if ! grep -q '"type":"ask_resolved"' "$ev"; then
    "$AR" send --detach "$sid" "方案没问题,就按 semver §11 的 pre-release 规则实现(数字段按数值比、数字段优先级低于字母段、字段多的更大、有 pre-release 的低于正式版)。动手吧,记得跑测试。" >/dev/null
  fi
fi

# Wait for the agent to finish (session goes idle / closed after reporting).
wait_done() {
  local i
  for i in $(seq 1 900); do
    (cd "$ws" && go test ./... >/dev/null 2>&1) && return 0   # tests green: real success
    grep -q '"type":"actor_crashed"' "$ev" 2>/dev/null && { echo "QA-14: crashed" >&2; return 1; }
    sleep 1
  done
  return 1  # fall through to assertions either way
}
wait_done || true

"$AR" close "$sid" >/dev/null 2>&1 || true
for i in $(seq 1 100); do tail -c 200 "$ev" | grep -q '"type":"session_closed"' && break; sleep 0.1; done

# ---- Assertions ----
fail=0
# THE hard proof: the workspace tests actually pass — the agent did real work.
if (cd "$ws" && go test ./... >"$work/gotest.log" 2>&1); then
  echo "  go test: GREEN"
else
  echo "FAIL: workspace tests are not green — agent did not complete the feature" >&2
  tail -20 "$work/gotest.log" >&2; fail=1
fi
# version.go was actually implemented (stub panic gone).
if grep -q 'not implemented' "$ws/version.go"; then echo "FAIL: Compare stub untouched" >&2; fail=1; fi
# Full tool face was exercised in the agentic flow.
grep -q '"name":"web_fetch"' "$ev" || { echo "FAIL: never consulted the spec via web_fetch" >&2; fail=1; }
grep -q 'Precedence\|pre-release\|precedence' "$ev" || { echo "WARN: spec text not obviously in journal" >&2; }
grep -q '"name":"bash"' "$ev" || { echo "FAIL: never ran tests via bash" >&2; fail=1; }
# ask_user park + inbox answer (if the agent asked, it must have been answered).
if grep '"type":"waiting_entered"' "$ev" | grep -q 'question'; then
  grep '"type":"ask_resolved"' "$ev" | grep -q 'answered' || { echo "FAIL: parked question never answered" >&2; fail=1; }
  echo "  ask_user: parked and answered via inbox"
else
  echo "  ask_user: agent did not ask (allowed; not asserted)"
fi
# Health.
if grep -q '"type":"actor_crashed"' "$ev"; then echo "FAIL: actor crashed" >&2; fail=1; fi
# Credential red line (grep -q avoids the pipefail+grep-c exit-code trap).
if grep -qF "$GEMINI_API_KEY" "$ev"; then echo "FAIL: API key leaked into journal" >&2; fail=1; fi

if [ -n "${QA_ARCHIVE:-}" ]; then
  mkdir -p "$QA_ARCHIVE"
  cp "$ev" "$QA_ARCHIVE/events.jsonl" 2>/dev/null || true
  cp "$ws/version.go" "$QA_ARCHIVE/version.go" 2>/dev/null || true
  cp "$work/gotest.log" "$QA_ARCHIVE/gotest.log" 2>/dev/null || true
fi

if [ "$fail" = 0 ]; then
  echo "QA-14 PASS: coding agent drove semver to green — web_fetch(spec) + ask_user(inbox) + bash(test) full flow"
else
  echo "QA-14 FAIL" >&2; exit 1
fi
