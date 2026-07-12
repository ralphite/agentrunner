#!/usr/bin/env bash
# QA-48 real-API gate (INC-48, #8): in-session LLM goal judge. Journal red
# lines (real Gemini as BOTH the working model and the judge):
#   1. goal_attached carries {"kind":"llm_judge","rubric":...};
#   2. the model calls goal_complete → goal_completion_claimed lands;
#   3. the claimed boundary runs a verifier:llm_judge Activity (real llm_call);
#   4. goal_checkpoint detail carries the judge reason; pass → goal_achieved
#      {satisfied};
#   5. claim-gated: judge Activity count ≤ claim count (never per-boundary).
#
# Private new-binary daemon on an isolated runtime root (the goal-attach path
# runs in the daemon); session copied to shared store + export archived.
#
#   qa/run-qa48.sh <ar-binary>
set -euo pipefail

AR="${1:?usage: run-qa48.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "QA-48: GEMINI_API_KEY unset" >&2; exit 2; }

stamp="$(date +%Y%m%d-%H%M%S)"
work="${QA48_WORK:-/tmp/qa48-$stamp}"
mkdir -p "$work/ws"
export XDG_DATA_HOME="$work/xdg"

cat > "$work/spec.yaml" <<'YAML'
name: qa48
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 2048 }
system_prompt: |
  你是一个会用 bash 完成文件任务的 agent。当会话挂有 goal 时，完成全部
  要求后调用 goal_complete 并附一行证据摘要;一个独立 judge 会核验你的
  工作。
tools: [bash, read_file, write_file]
permissions:
  - { action: allow }
YAML

"$AR" daemon >"$work/daemon.log" 2>&1 &
DPID=$!
sock="$XDG_DATA_HOME/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$sock" ] && break; sleep 0.1; done
[ -S "$sock" ] || { echo "QA-48: daemon socket never appeared" >&2; cat "$work/daemon.log" >&2; exit 1; }
trap 'kill "$DPID" 2>/dev/null || true' EXIT

count() { local n; n="$(grep -ac "$2" "$1" 2>/dev/null || true)"; echo "${n:-0}"; }
wait_for() { # file pattern min timeout-iters
  local i n=0; for i in $(seq 1 "${4:-600}"); do n="$(count "$1" "$2")"; [ "$n" -ge "$3" ] && return 0; sleep 0.2; done
  echo "QA-48: timeout waiting for $2 (have $n)" >&2; return 1
}

sid="$("$AR" new --detach --workspace "$work/ws" "$work/spec.yaml" \
  "在 workspace 里创建 greeting.txt，内容是一句面向新用户的英文欢迎语（至少 8 个词），并创建 VERSION 文件内容为 1.0.0。")"
[ -n "$sid" ] || { echo "QA-48: no session id" >&2; exit 1; }
echo "QA-48 session: $sid"
sdir="$XDG_DATA_HOME/agentrunner/sessions/$sid"
ev="$sdir/events.jsonl"
wait_for "$ev" '"type":"assistant_message"' 1

# Attach a no-command goal with an llm_judge rubric.
"$AR" goal "$sid" attach --verify-llm \
  "workspace 必须有 greeting.txt(内容为不少于 8 个英文单词的欢迎语)和 VERSION(内容 1.0.0)。以 agent 的工作叙述为证据判断这两个文件是否都已按要求创建。" \
  --max-checks 6 \
  创建 greeting.txt 欢迎语与 VERSION 文件，全部就绪后声明完成
echo "goal attached"

wait_for "$ev" '"type":"goal_achieved"' 1 900

# Red line 1: attach carried the llm_judge verifier.
grep -a '"type":"goal_attached"' "$ev" | grep -q '"kind":"llm_judge"' \
  || { echo "QA-48 FAIL: goal_attached lacks llm_judge verifier" >&2; exit 1; }
echo "PASS(1): goal_attached carries llm_judge rubric"

# Red line 2: the model claimed via goal_complete.
claims="$(count "$ev" '"type":"goal_completion_claimed"')"
[ "$claims" -ge 1 ] || { echo "QA-48 FAIL: no goal_completion_claimed" >&2; exit 1; }
echo "PASS(2): goal_completion_claimed x$claims"

# Red line 3: a real judge Activity ran.
judges="$(grep -a '"type":"activity_started"' "$ev" | grep -ac '"name":"verifier:llm_judge"')" || judges=0; judges="${judges:-0}"
[ "$judges" -ge 1 ] || { echo "QA-48 FAIL: no verifier:llm_judge activity" >&2; exit 1; }
echo "PASS(3): verifier:llm_judge activity x$judges (real Gemini judge call)"

# Red line 4: checkpoint carries judge detail; achieved satisfied.
grep -a '"type":"goal_checkpoint"' "$ev" | grep -q 'judge' \
  || { echo "QA-48 FAIL: no judge attribution in goal_checkpoint" >&2; exit 1; }
grep -a '"type":"goal_achieved"' "$ev" | grep -q '"reason":"satisfied"' \
  || { echo "QA-48 FAIL: goal_achieved is not satisfied" >&2; grep -a '"type":"goal_achieved"' "$ev" >&2; exit 1; }
echo "PASS(4): goal_checkpoint judge detail + goal_achieved{satisfied}"

# Red line 5: claim-gated — judge calls never exceed claims.
[ "$judges" -le "$claims" ] \
  || { echo "QA-48 FAIL: judge ran more often than claims ($judges > $claims)" >&2; exit 1; }
echo "PASS(5): claim-gated (judge $judges <= claims $claims)"

# The work itself really landed.
[ -f "$work/ws/greeting.txt" ] && [ -f "$work/ws/VERSION" ] \
  || { echo "QA-48 FAIL: workspace files missing" >&2; ls "$work/ws" >&2; exit 1; }
echo "PASS(6): workspace really has greeting.txt + VERSION ($(cat "$work/ws/VERSION"))"

shared="$HOME/.local/share/agentrunner/sessions"
mkdir -p "$shared"; cp -R "$sdir" "$shared/$sid"
run_dir="$here/runs/$(date +%Y-%m-%d)-QA-48"
mkdir -p "$run_dir"
"$AR" events "$sid" > "$run_dir/events.export.txt" 2>/dev/null || cp "$ev" "$run_dir/events.export.txt"
cp "$ev" "$run_dir/events.jsonl"
cp "$work/spec.yaml" "$run_dir/spec.yaml"
{ echo "QA-48 in-session LLM goal judge — $(date)"; echo "session: $sid"; \
  echo "claims=$claims judges=$judges"; } > "$run_dir/notes.md"
echo "QA-48: all green. session kept in $shared; export archived at $run_dir"
