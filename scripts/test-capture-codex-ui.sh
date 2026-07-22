#!/usr/bin/env bash
# Platform-independent contract checks for the macOS Codex screenshot driver.
set -euo pipefail
cd "$(dirname "$0")/.."

driver=qa/capture-codex-ui.sh
bash -n "$driver"

help=$($driver --help)
for surface in new-chat pull-requests sites scheduled plugins; do
  [[ "$help" == *"$surface"* ]] || {
    echo "capture driver help is missing surface: $surface" >&2
    exit 1
  }
done
[[ "$help" == *"--context-menu"* ]]
[[ "$help" == *"--keyboard-context-menu"* ]]
[[ "$help" == *"--account-menu"* ]]
[[ "$help" == *"--user-menu"* ]]

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

if $driver --surface destructive-target >"$tmpdir/unknown.out" 2>"$tmpdir/unknown.err"; then
  echo "capture driver accepted an unknown surface" >&2
  exit 1
fi
grep -Fq 'unsupported surface: destructive-target' "$tmpdir/unknown.err"

if $driver --settle 31 >"$tmpdir/settle.out" 2>"$tmpdir/settle.err"; then
  echo "capture driver accepted an invalid settle value" >&2
  exit 1
fi
grep -Fq 'whole seconds from 0 to 30' "$tmpdir/settle.err"

if $driver --palette-query QA >"$tmpdir/palette.out" 2>"$tmpdir/palette.err"; then
  echo "capture driver accepted a palette query without palette mode" >&2
  exit 1
fi
grep -Fq -- '--palette-query requires --command-palette' "$tmpdir/palette.err"

if $driver --restore-query QA >"$tmpdir/restore.out" 2>"$tmpdir/restore.err"; then
  echo "capture driver accepted restore query without a surface" >&2
  exit 1
fi
grep -Fq -- '--restore-query requires --surface' "$tmpdir/restore.err"

# Query entry must be reversible: the driver may borrow the clipboard to paste
# into Electron, but it has to preserve every original pasteboard item/type.
grep -Fq 'pasteboard.pasteboardItems' "$driver"
grep -Fq 'pasteboard.writeObjects(restoredItems)' "$driver"
grep -Fq 'defer { restorePasteboard() }' "$driver"
grep -Fq 'observation.boundingBox.midX < 0.30' "$driver"
grep -Fq 'screencapture -x -o -t png' "$driver"
grep -Fq 'send_key 109 2' "$driver"
# Literal source contract; expansion would weaken the assertion.
# shellcheck disable=SC2016
grep -Fq 'send_click "$point_x" "$point_y" right' "$driver"

if grep -Fq 'System Events' "$driver"; then
  echo "capture driver must not regress to the blocking System Events path" >&2
  exit 1
fi
grep -Fq 'trap close_transient EXIT' "$driver"
# Literal source contract; variable expansion would defeat this assertion.
# shellcheck disable=SC2016
grep -Fq 'if [[ -n "$restore_query" && -n "$surface" ]]' "$driver"

echo "test-capture-codex-ui: ok"
