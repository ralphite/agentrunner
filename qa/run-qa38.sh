#!/usr/bin/env bash
# QA-38 real-API gate (INC-33, #32): read_file multimodal — the live model
# READS an image from the workspace with read_file and answers from its
# pixels. Journal red lines:
#   1. the read_file tool result is a media envelope (kind:image + CAS ref);
#   2. the journal carries NO blob bytes (every line stays small);
#   3. the model's answer names what is IN the image (file/line/symbol from
#      the CI screenshot) — proof the pixels reached the model via the tool.
#
# Media reads run inside the daemon's agent loop → private daemon running
# THIS binary; session copied to the shared store + export archived.
#
#   qa/run-qa38.sh <ar-binary>
set -euo pipefail
QA=QA-38
AR="${1:?usage: run-qa38.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
if [ -z "${GEMINI_API_KEY:-}" ]; then
  main_root="$(cd "$here/.." && dirname "$(git rev-parse --git-common-dir)")"
  [ -f "$main_root/.env" ] && { set -a; . "$main_root/.env"; set +a; }
fi
[ -n "${GEMINI_API_KEY:-}" ] || { echo "$QA: GEMINI_API_KEY unset" >&2; exit 2; }

stamp="$(date +%Y%m%d-%H%M%S)"
work="${QA38_WORK:-/tmp/qa38-$stamp}"
mkdir -p "$work/ws"
export XDG_DATA_HOME="$work/xdg"

cp "$here/fixtures/build-error.png" "$work/ws/ci-shot.png"

cat > "$work/spec.yaml" <<'YAML'
name: qa38
model: { provider: gemini, id: gemini-flash-latest, max_tokens: 1024 }
system_prompt: |
  你可以用 read_file 直接读取图片(png/jpeg)与 PDF——其内容会作为图像
  附到对话里,你能直接看到并分析。用户要你看图时,直接 read_file 那个
  文件即可。
tools: [read_file]
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

sid="$("$AR" new --detach --workspace "$work/ws" "$work/spec.yaml" \
  "工作区里有一张 CI 报错截图 ci-shot.png。用 read_file 读它,然后告诉我:哪个文件、哪一行、报了什么错(引用图里的符号名)。" 2>/dev/null | head -1)"
[ -n "$sid" ] || { echo "$QA: no session id" >&2; exit 1; }
SDIR="$XDG_DATA_HOME/agentrunner/sessions/$sid"
echo "$QA session: $sid"

deadline=$((SECONDS + 120))
while [ $SECONDS -lt $deadline ]; do
  n="$(asst_count "$SDIR/events.jsonl")"
  idle="$(tail -1 "$SDIR/events.jsonl" 2>/dev/null | grep -c waiting_entered || true)"
  [ "${n:-0}" -ge 1 ] && [ "${idle:-0}" -ge 1 ] && break
  sleep 2
done
EV="$SDIR/events.jsonl"

fail=0
note() { echo "$QA: $*"; }
check() { if eval "$2"; then note "PASS  $1"; else note "FAIL  $1"; fail=1; fi; }

check "journal exists" '[ -f "$EV" ]'

# Red line 1: read_file returned a media envelope with a CAS ref.
check "read_file returned a media envelope (kind:image + ref)" \
  'grep -a "\"kind\":\"image\"" "$EV" | grep -q "\"ref\":"'

# Red line 2: no blob bytes in the journal — the PNG is ~3KB (≈4KB base64);
# every journal line must stay well under that inflate size.
maxline="$(awk '{ if (length($0) > m) m = length($0) } END { print m+0 }' "$EV")"
if [ "$maxline" -lt 3000 ]; then
  note "PASS  journal is byte-free (longest line ${maxline}B < 3000B)"
else
  note "FAIL  journal has a ${maxline}B line — blob bytes may have leaked"
  fail=1
fi

# Red line 3: the answer names content only visible IN the image.
last="$(grep -a '"type":"assistant_message"' "$EV" | tail -1)"
if printf '%s' "$last" | grep -qiE 'command\.go|1234|EnableTraverseRunHooks'; then
  note "PASS  answer names the screenshot's content (command.go/1234/EnableTraverseRunHooks)"
else
  note "FAIL  answer does not reflect the image content"
  printf '%s\n' "$last" | head -c 400 >&2; echo >&2
  fail=1
fi

shared="$HOME/.local/share/agentrunner/sessions"
mkdir -p "$shared"; cp -R "$SDIR" "$shared/$sid" 2>/dev/null || true
run_dir="$here/runs/$(date +%Y-%m-%d)-QA38"
mkdir -p "$run_dir"
"$AR" events "$sid" > "$run_dir/events.export.jsonl" 2>/dev/null || cp "$EV" "$run_dir/events.export.jsonl"
{ echo "$QA read_file multimodal — $(date)"; echo "session: $sid"; echo "workspace: $work/ws"; } > "$run_dir/notes.md"

if [ "$fail" -eq 0 ]; then note "all green. session copied to $shared/$sid; export archived at $run_dir"; else note "one or more red lines FAILED (kept at $SDIR)"; exit 1; fi
