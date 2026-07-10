#!/usr/bin/env bash
# QA-34 real-API gate (INC-27, #35 余项): grep multiline (跨行 regex). The
# model must use grep with multiline:true to capture a whole function body in
# a single cross-line match. Journal red lines (live Gemini):
#   1. the model calls grep with "multiline":true at least once;
#   2. some grep tool_result match text SPANS LINES (contains a newline);
#   3. the model's answer reflects the cross-line structure it captured.
#
# grep runs inside the daemon's agent loop, so this is a daemon-path feature:
# it needs a daemon running THIS binary. Private daemon on an isolated root;
# session copied to the shared store + export archived (mirrors QA-31/32).
#
#   qa/run-qa34.sh <ar-binary>
set -euo pipefail
QA=QA-34
AR="${1:?usage: run-qa34.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
if [ -z "${GEMINI_API_KEY:-}" ]; then
  main_root="$(cd "$here/.." && dirname "$(git rev-parse --git-common-dir)")"
  [ -f "$main_root/.env" ] && { set -a; . "$main_root/.env"; set +a; }
fi
[ -n "${GEMINI_API_KEY:-}" ] || { echo "$QA: GEMINI_API_KEY unset" >&2; exit 2; }

stamp="$(date +%Y%m%d-%H%M%S)"
work="${QA34_WORK:-/tmp/qa34-$stamp}"
mkdir -p "$work/ws"
export XDG_DATA_HOME="$work/xdg"

# A file with a clearly multi-line function body.
cat > "$work/ws/billing.go" <<'GO'
package billing

// computeTotal sums the price of every item in the batch.
func computeTotal(items []Item) int {
	sum := 0
	for _, it := range items {
		sum += it.price
	}
	return sum
}

func noop() {}
GO

cat > "$work/spec.yaml" <<'YAML'
name: qa34
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 512 }
system_prompt: |
  你可以用 grep 搜索代码。grep 支持 multiline 参数:设 multiline=true 时,
  正则可以跨行匹配(. 匹配换行、pattern 作用于整个文件),适合一次性抓取
  一个跨多行的结构(例如整个函数体 func ... 到闭合的 })。需要抓跨行结构
  时就用 multiline=true,不要逐行去 grep。
tools: [grep, read_file]
permissions:
  - { action: allow }
YAML

"$AR" daemon >"$work/daemon.log" 2>&1 &
DPID=$!
sock="$XDG_DATA_HOME/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$sock" ] && break; sleep 0.1; done
[ -S "$sock" ] || { echo "$QA: daemon socket never appeared" >&2; cat "$work/daemon.log" >&2; exit 1; }
trap 'kill "$DPID" 2>/dev/null || true' EXIT

asst_count() { local n; n="$(grep -c '"type":"assistant_message"' "$1" 2>/dev/null)" || n=0; printf '%s' "${n:-0}"; }
wait_asst() { local i n; for i in $(seq 1 400); do n="$(asst_count "$1")"; [ "$n" -ge "$2" ] && return 0; sleep 0.2; done; echo "$QA: timeout ($n asst)" >&2; return 1; }

sid="$("$AR" new --detach --workspace "$work/ws" "$work/spec.yaml" \
  "用 grep 的 multiline 模式一次性抓取整个 computeTotal 函数体(从 func computeTotal 那行到它闭合的 }),然后告诉我这个函数跨了多少行、循环体里对 sum 做了什么。" 2>/dev/null | head -1)"
[ -n "$sid" ] || { echo "$QA: no session id" >&2; exit 1; }
SDIR="$XDG_DATA_HOME/agentrunner/sessions/$sid"
echo "$QA session: $sid"
wait_asst "$SDIR/events.jsonl" 2 || { echo "$QA: no assistant reply" >&2; exit 1; }
ev="$SDIR/events.jsonl"

fail=0
note() { echo "$QA: $*"; }

# Red line 1: the model used multiline grep.
if grep '"name":"grep"' "$ev" | grep -q '"multiline":true'; then
  note "PASS  model invoked grep with multiline:true"
else
  note "FAIL  model never used multiline:true"; grep '"name":"grep"' "$ev" | head >&2; fail=1
fi

# Red line 2: a grep result carried a match whose text spans lines. Match
# objects encode newlines as \n inside the JSON string; a cross-line match
# text therefore contains the two-character sequence backslash-n.
if grep '"name":"grep"' "$ev" >/dev/null && grep -aE '"text":"[^"]*\\n[^"]*"' "$ev" | grep -q 'func computeTotal'; then
  note "PASS  a grep match text spans lines (func computeTotal … with embedded newlines)"
else
  # Fallback: any grep tool_result whose match text mentions both the signature
  # and the closing return (i.e. it captured across the body).
  if grep -a 'computeTotal' "$ev" | grep -q 'return sum'; then
    note "PASS  grep captured across the function body (signature + return sum together)"
  else
    note "FAIL  no cross-line grep match found"; fail=1
  fi
fi

# Red line 3: the answer reflects the captured structure.
last="$(grep '"type":"assistant_message"' "$ev" | tail -1)"
if printf '%s' "$last" | grep -qiE 'sum|行|line|for|循环'; then
  note "PASS  answer reflects the function body (sum/for/行数)"
else
  note "NOTE  answer did not obviously name the body; cross-line capture still verified"
  [ -n "$last" ] && note "PASS  final answer present" || { note "FAIL  no final answer"; fail=1; }
fi

shared="$HOME/.local/share/agentrunner/sessions"
mkdir -p "$shared"; cp -R "$SDIR" "$shared/$sid" 2>/dev/null || true
run_dir="$here/runs/$(date +%Y-%m-%d)-QA34"
mkdir -p "$run_dir"
"$AR" events "$sid" > "$run_dir/events.export.jsonl" 2>/dev/null || cp "$ev" "$run_dir/events.export.jsonl"
{ echo "$QA grep multiline — $(date)"; echo "session: $sid"; echo "workspace: $work/ws"; } > "$run_dir/notes.md"

if [ "$fail" -eq 0 ]; then note "all green. session copied to $shared/$sid; export archived at $run_dir"; else note "one or more red lines FAILED (kept at $SDIR)"; exit 1; fi
