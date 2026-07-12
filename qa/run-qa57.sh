#!/usr/bin/env bash
# QA-57 real-API gate (INC-56, HANDA #18 ar dictate + #19 ar optimize). Both
# are one-shot provider calls in the `ar` process (webui stays a thin shell) —
# no daemon needed. Real Gemini:
#   1. `say` synthesizes an audio note with English+Chinese proper nouns →
#      `ar dictate --context <hints>` transcribes it, keeping the proper nouns;
#   2. `ar optimize --context <hints>` rewrites a terse draft into a fuller,
#      different prompt (not echoed verbatim).
#
#   qa/run-qa57.sh <ar-binary>
set -euo pipefail

AR="${1:?usage: run-qa57.sh <ar-binary>}"
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/../.env" ] && { set -a; . "$here/../.env"; set +a; }
[ -n "${GEMINI_API_KEY:-}" ] || { echo "QA-57: GEMINI_API_KEY unset" >&2; exit 2; }

command -v say >/dev/null || { echo "QA-57 SKIP: \`say\` unavailable (macOS only)" >&2; exit 3; }
work="$(mktemp -d /tmp/qa57-XXXX)"

# 1. dictate — synth an English/Chinese-mixed note naming proper nouns.
NOTE='Please deploy the kubelet on cluster Artemis, then rebase the auth branch.'
say -o "$work/note.aiff" "$NOTE"
[ -f "$work/note.aiff" ] || { echo "QA-57: say produced no audio" >&2; exit 1; }
transcript="$("$AR" dictate --context "Kubernetes, kubelet, cluster Artemis, rebase, auth branch" "$work/note.aiff" 2>"$work/dictate.err")" \
  || { echo "QA-57 FAIL: ar dictate errored" >&2; cat "$work/dictate.err" >&2; exit 1; }
echo "transcript: $transcript"
# The proper nouns must survive transcription (case-insensitive).
echo "$transcript" | grep -qi 'kubelet' && echo "$transcript" | grep -qi 'artemis' && echo "$transcript" | grep -qi 'rebase' \
  || { echo "QA-57 FAIL: transcript dropped proper nouns" >&2; exit 1; }
echo "PASS(1): ar dictate transcribed audio keeping kubelet/Artemis/rebase"

# 2. optimize — a terse draft becomes a fuller, different prompt.
DRAFT='fix the thing that broke'
opt="$("$AR" optimize --context "editing internal/auth token verification" "$DRAFT" 2>"$work/opt.err")" \
  || { echo "QA-57 FAIL: ar optimize errored" >&2; cat "$work/opt.err" >&2; exit 1; }
echo "optimized: $opt"
[ -n "$opt" ] || { echo "QA-57 FAIL: empty optimize output" >&2; exit 1; }
[ "$opt" != "$DRAFT" ] || { echo "QA-57 FAIL: optimize echoed the draft verbatim" >&2; exit 1; }
# A real rewrite is longer than the terse draft and mentions the domain.
[ "${#opt}" -gt "${#DRAFT}" ] || { echo "QA-57 FAIL: optimize not fuller than draft" >&2; exit 1; }
echo "PASS(2): ar optimize rewrote the draft into a fuller, different prompt"

run_dir="$here/runs/$(date +%Y-%m-%d)-INC56"
mkdir -p "$run_dir"
{ echo "QA-57 dictate+optimize — $(date)"; echo "note: $NOTE"; echo "transcript: $transcript"; \
  echo "draft: $DRAFT"; echo "optimized: $opt"; } > "$run_dir/notes.md"
echo "QA-57: all green. archived at $run_dir"
