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
[[ "$help" == *"--new-chat-control"* ]]
[[ "$help" == *"--composer-text"* ]]
[[ "$help" == *"--composer-validate"* ]]
for control in project worktree environment branch add goal plan access model model-list effort speed starter-explore starter-build starter-review starter-fix; do
  [[ "$help" == *"$control"* ]] || {
    echo "capture driver help is missing New chat control: $control" >&2
    exit 1
  }
done

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

if $driver --new-chat-control destructive-control >"$tmpdir/control.out" 2>"$tmpdir/control.err"; then
  echo "capture driver accepted an unknown New chat control" >&2
  exit 1
fi
grep -Fq 'unsupported New chat control: destructive-control' "$tmpdir/control.err"

if $driver --control-query QA >"$tmpdir/control-query.out" 2>"$tmpdir/control-query.err"; then
  echo "capture driver accepted a control query without New chat control mode" >&2
  exit 1
fi
grep -Fq -- '--control-query requires --new-chat-control' "$tmpdir/control-query.err"

if $driver --new-chat-control access --control-query QA >"$tmpdir/control-query-kind.out" 2>"$tmpdir/control-query-kind.err"; then
  echo "capture driver accepted a query for a non-searchable New chat control" >&2
  exit 1
fi
grep -Fq -- '--control-query is only supported for project or branch' "$tmpdir/control-query-kind.err"

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
grep -Fq 'observation.boundingBox.midX > 0.30 && observation.boundingBox.midY < 0.20' "$driver"
grep -Fq 'target_text="New worktree"; target_region="composer"' "$driver"
grep -Fq 'target_text="Explore and"; target_region="starter"' "$driver"
grep -Fq 'case "popover"' "$driver"
grep -Fq 'case "popover-low"' "$driver"
# Literal source contract; expansion would weaken the assertion.
# shellcheck disable=SC2016
grep -Fq 'window_text_center "$ocr_capture" "$validation_text" "$validation_region"' "$driver"
grep -Fq 'if ((starter_seeded))' "$driver"
grep -Fq 'if ((composer_seeded))' "$driver"
grep -Fq 'if ((goal_enabled || plan_enabled))' "$driver"
# Literal source contract; expansion would weaken the assertion.
# shellcheck disable=SC2016
grep -Fq 'window_text_center "$ocr_capture" "Explore and" "starter"' "$driver"
grep -Fq 'if ((nested_open))' "$driver"
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
