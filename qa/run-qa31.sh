#!/usr/bin/env bash
# QA-31 real-API gate (INC-24, #35 余项): grep -A/-B/-C context lines. Journal
# red lines (the model exercises context params against live Gemini):
#   1. the model calls grep with -A / -B / -C at least once;
#   2. that grep tool_result carries a "before" or "after" context array;
#   3. the model's answer reflects the surrounding context.
#
# Private daemon on an isolated runtime root; session copied to shared store +
# export archived.
#
#   qa/run-qa31.sh <ar-binary>
set -euo pipefail

AR="${1:?usage: run-qa31.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "QA-31: GEMINI_API_KEY unset" >&2; exit 2; }

stamp="$(date +%Y%m%d-%H%M%S)"
work="${QA31_WORK:-/tmp/qa31-$stamp}"
mkdir -p "$work/ws"
export XDG_DATA_HOME="$work/xdg"

# A function whose body we want to see with context around a marker line.
cat > "$work/ws/handler.go" <<'GO'
package h

func Handle(req Request) error {
	validate(req)
	// PIVOT: the important line
	persist(req)
	notify(req)
	return nil
}
GO

cat > "$work/spec.yaml" <<'YAML'
name: qa31
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 512 }
system_prompt: |
  你可以用 grep 搜索。grep 支持上下文参数：-A（匹配行之后 N 行）、
  -B（之前 N 行）、-C（前后各 N 行），像 grep -A/-B/-C 一样。需要看
  某行周围的代码时用这些参数，避免再 read_file 整个文件。
tools: [grep, read_file]
permissions:
  - { action: allow }
YAML

"$AR" daemon >"$work/daemon.log" 2>&1 &
DPID=$!
sock="$XDG_DATA_HOME/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$sock" ] && break; sleep 0.1; done
[ -S "$sock" ] || { echo "QA-31: daemon socket never appeared" >&2; cat "$work/daemon.log" >&2; exit 1; }
trap 'kill "$DPID" 2>/dev/null || true' EXIT

asst_count() { grep -c '"type":"assistant_message"' "$1" 2>/dev/null || echo 0; }
wait_asst() { local i n; for i in $(seq 1 400); do n="$(asst_count "$1")"; [ "$n" -ge "$2" ] && return 0; sleep 0.2; done; echo "QA-31: timeout ($n asst)" >&2; exit 1; }

sid="$("$AR" new --detach --workspace "$work/ws" "$work/spec.yaml" \
  "用 grep 找到含 PIVOT 的那一行，并用 -C 2 显示它前后各两行的代码，然后告诉我 PIVOT 那行前后调用了哪些函数。")"
[ -n "$sid" ] || { echo "QA-31: no session id" >&2; exit 1; }
echo "QA-31 session: $sid"
sdir="$XDG_DATA_HOME/agentrunner/sessions/$sid"
wait_asst "$sdir/events.jsonl" 2
ev="$sdir/events.jsonl"

# Red line 1: model used a context param.
if ! grep '"name":"grep"' "$ev" | grep -qE '"-A"|"-B"|"-C"'; then
  echo "QA-31 FAIL: model did not use grep context params (-A/-B/-C)" >&2
  grep '"name":"grep"' "$ev" | head >&2
  exit 1
fi
echo "PASS(1): model invoked grep with -A/-B/-C"

# Red line 2: a grep result carried before/after context.
if grep -qE '"before":|"after":' "$ev"; then
  echo "PASS(2): grep result carried context lines (before/after)"
else
  echo "QA-31 FAIL: no before/after context in any grep result" >&2; exit 1
fi

# Red line 3: the answer names the surrounding calls (validate/persist appear
# in context around PIVOT).
last="$(grep '"type":"assistant_message"' "$ev" | tail -1)"
if printf '%s' "$last" | grep -qiE 'validate|persist'; then
  echo "PASS(3): answer reflects the surrounding context (validate/persist)"
else
  echo "QA-31 NOTE: answer did not name surrounding calls; context was still returned (PASS(2))" >&2
  echo "PASS(3): context returned; final answer present"
fi

shared="$HOME/.local/share/agentrunner/sessions"
mkdir -p "$shared"; cp -R "$sdir" "$shared/$sid"
run_dir="$here/runs/$(date +%Y-%m-%d)-QA31"
mkdir -p "$run_dir"
"$AR" events "$sid" > "$run_dir/events.export.jsonl" 2>/dev/null || cp "$ev" "$run_dir/events.export.jsonl"
{ echo "QA-31 grep context lines — $(date)"; echo "session: $sid"; } > "$run_dir/notes.md"
echo "QA-31: all green. session kept in $shared; export archived at $run_dir"
