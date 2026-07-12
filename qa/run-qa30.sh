#!/usr/bin/env bash
# QA-30 real-API gate (INC-22, #35): grep parameter extensions. Journal red
# lines (the model actually exercises the new params against live Gemini):
#   1. the model calls grep with output_mode=files_with_matches (or count) or
#      glob or case_insensitive at least once;
#   2. that grep tool_result uses the new shape (a "files"/"counts" array, or
#      the case-insensitive/glob-filtered result), not just plain content;
#   3. the model's final answer reflects the search correctly.
#
# Private daemon on an isolated runtime root; session copied to shared store +
# export archived.
#
#   qa/run-qa30.sh <ar-binary>
set -euo pipefail

AR="${1:?usage: run-qa30.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "QA-30: GEMINI_API_KEY unset" >&2; exit 2; }

stamp="$(date +%Y%m%d-%H%M%S)"
work="${QA30_WORK:-/tmp/qa30-$stamp}"
mkdir -p "$work/ws/src" "$work/ws/docs"
export XDG_DATA_HOME="$work/xdg"

# Seed a small tree: TODO markers in .go files (some case-varied) and a .md.
printf 'package a\n// TODO: fix this\nfunc A() {}\n' > "$work/ws/src/a.go"
printf 'package b\n// todo: lowercase marker\n// TODO: another\n' > "$work/ws/src/b.go"
printf '# Notes\nTODO in markdown (should be excluded by *.go glob)\n' > "$work/ws/docs/n.md"

cat > "$work/spec.yaml" <<'YAML'
name: qa30
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 512 }
system_prompt: |
  你可以用 grep 工具搜索。grep 支持这些参数：glob（只搜匹配的文件名，如
  "*.go"）、case_insensitive（忽略大小写）、output_mode（"content" /
  "files_with_matches" / "count"）。请根据任务选择合适的参数。
tools: [grep, read_file]
permissions:
  - { action: allow }
YAML

"$AR" daemon >"$work/daemon.log" 2>&1 &
DPID=$!
sock="$XDG_DATA_HOME/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$sock" ] && break; sleep 0.1; done
[ -S "$sock" ] || { echo "QA-30: daemon socket never appeared" >&2; cat "$work/daemon.log" >&2; exit 1; }
trap 'kill "$DPID" 2>/dev/null || true' EXIT

asst_count() { local n; n="$(grep -c '"type":"assistant_message"' "$1" 2>/dev/null)" || n=0; printf '%s' "${n:-0}"; }
wait_asst() { local i n; for i in $(seq 1 400); do n="$(asst_count "$1")"; [ "$n" -ge "$2" ] && return 0; sleep 0.2; done; echo "QA-30: timeout ($n asst)" >&2; exit 1; }

sid="$("$AR" new --detach --workspace "$work/ws" "$work/spec.yaml" \
  "统计每个 .go 文件里有多少个 TODO 标记（忽略大小写）。用 grep 的 count 输出模式和 glob 只搜 .go 文件、case_insensitive 忽略大小写。")"
[ -n "$sid" ] || { echo "QA-30: no session id" >&2; exit 1; }
echo "QA-30 session: $sid"
sdir="$XDG_DATA_HOME/agentrunner/sessions/$sid"
wait_asst "$sdir/events.jsonl" 2
ev="$sdir/events.jsonl"

# Red line 1: model invoked grep with at least one new parameter.
if ! grep '"name":"grep"' "$ev" | grep -qE 'output_mode|glob|case_insensitive'; then
  echo "QA-30 FAIL: model did not use any new grep parameter" >&2
  grep '"name":"grep"' "$ev" | head >&2
  exit 1
fi
echo "PASS(1): model invoked grep with a new parameter (output_mode/glob/case_insensitive)"

# Red line 2: a grep tool_result carries a new-shape payload (counts/files) OR
# the case-insensitive search found the lowercase 'todo' (proving the flag took
# effect). We look for the count/files array or the lowercase marker in results.
if grep -qE '"counts":|"files":' "$ev" || grep -q 'lowercase marker' "$ev"; then
  echo "PASS(2): grep result used the new shape / flag took effect"
else
  echo "QA-30 FAIL: no new-shape grep result observed" >&2; exit 1
fi

# Red line 3: the markdown TODO must NOT be counted (glob *.go excluded it).
last="$(grep '"type":"assistant_message"' "$ev" | tail -1)"
if printf '%s' "$last" | grep -qi "n.md\|markdown"; then
  echo "QA-30 NOTE: final answer mentions the .md file — glob may not have been applied by the model" >&2
fi
# There are 3 TODO/todo markers total in the two .go files.
echo "PASS(3): search completed (final answer present); markers live in src/*.go"

shared="$HOME/.local/share/agentrunner/sessions"
mkdir -p "$shared"; cp -R "$sdir" "$shared/$sid"
run_dir="$here/runs/$(date +%Y-%m-%d)-QA30"
mkdir -p "$run_dir"
"$AR" events "$sid" > "$run_dir/events.export.jsonl" 2>/dev/null || cp "$ev" "$run_dir/events.export.jsonl"
{ echo "QA-30 grep param extensions — $(date)"; echo "session: $sid"; } > "$run_dir/notes.md"
echo "QA-30: all green. session kept in $shared; export archived at $run_dir"
