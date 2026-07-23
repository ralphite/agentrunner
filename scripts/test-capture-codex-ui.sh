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
[[ "$help" == *"--settings"* ]]
[[ "$help" == *"--settings-tab"* ]]
[[ "$help" == *"--new-chat-control"* ]]
[[ "$help" == *"--composer-text"* ]]
[[ "$help" == *"--composer-send"* ]]
[[ "$help" == *"--composer-mode"* ]]
[[ "$help" == *"--composer-access"* ]]
[[ "$help" == *"--thread-composer-send"* ]]
[[ "$help" == *"--thread-shortcut"* ]]
[[ "$help" == *"--thread-approval"* ]]
[[ "$help" == *"--thread-disclosure"* ]]
[[ "$help" == *"--disclosure-validate"* ]]
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

if $driver --composer-send QA >"$tmpdir/composer-send.out" 2>"$tmpdir/composer-send.err"; then
  echo "capture driver accepted a send without visual validation" >&2
  exit 1
fi
grep -Fq 'composer text/send requires --composer-validate' "$tmpdir/composer-send.err"

if $driver --composer-mode plan >"$tmpdir/composer-mode.out" 2>"$tmpdir/composer-mode.err"; then
  echo "capture driver accepted Plan mode without composer send" >&2
  exit 1
fi
grep -Fq -- '--composer-mode plan requires --composer-send' "$tmpdir/composer-mode.err"

if $driver --composer-send QA --composer-validate QA --composer-mode destructive >"$tmpdir/composer-mode-kind.out" 2>"$tmpdir/composer-mode-kind.err"; then
  echo "capture driver accepted an unknown composer mode" >&2
  exit 1
fi
grep -Fq -- '--composer-mode requires default or plan' "$tmpdir/composer-mode-kind.err"

if $driver --composer-access ask >"$tmpdir/composer-access.out" 2>"$tmpdir/composer-access.err"; then
  echo "capture driver accepted access without composer send" >&2
  exit 1
fi
grep -Fq -- '--composer-access requires --composer-send' "$tmpdir/composer-access.err"

if $driver --composer-send QA --composer-validate QA --composer-access destructive >"$tmpdir/composer-access-kind.out" 2>"$tmpdir/composer-access-kind.err"; then
  echo "capture driver accepted an unknown composer access" >&2
  exit 1
fi
grep -Fq -- '--composer-access requires current, ask, approve, or full' "$tmpdir/composer-access-kind.err"

if $driver --composer-send QA --composer-validate QA --composer-mode plan --composer-access ask >"$tmpdir/composer-access-plan.out" 2>"$tmpdir/composer-access-plan.err"; then
  echo "capture driver combined access with Plan mode" >&2
  exit 1
fi
grep -Fq -- '--composer-access cannot be combined with Plan mode' "$tmpdir/composer-access-plan.err"

if $driver --thread-composer-send QA >"$tmpdir/thread-send.out" 2>"$tmpdir/thread-send.err"; then
  echo "capture driver accepted a thread send without visual validation" >&2
  exit 1
fi
grep -Fq 'composer text/send requires --composer-validate' "$tmpdir/thread-send.err"

if $driver --thread-shortcut cmd-enter >"$tmpdir/thread-shortcut.out" 2>"$tmpdir/thread-shortcut.err"; then
  echo "capture driver accepted a thread shortcut without thread send mode" >&2
  exit 1
fi
grep -Fq -- '--thread-shortcut requires --thread-composer-send' "$tmpdir/thread-shortcut.err"

if $driver --thread-approval destructive >"$tmpdir/thread-approval.out" 2>"$tmpdir/thread-approval.err"; then
  echo "capture driver accepted an unknown approval action" >&2
  exit 1
fi
grep -Fq -- '--thread-approval requires allow-once or deny' "$tmpdir/thread-approval.err"

if $driver --thread-disclosure Asked >"$tmpdir/disclosure.out" 2>"$tmpdir/disclosure.err"; then
  echo "capture driver accepted a disclosure without visual validation" >&2
  exit 1
fi
grep -Fq 'thread disclosure requires --disclosure-validate' "$tmpdir/disclosure.err"

if $driver --disclosure-validate Alpha >"$tmpdir/disclosure-validate.out" 2>"$tmpdir/disclosure-validate.err"; then
  echo "capture driver accepted disclosure validation without a target" >&2
  exit 1
fi
grep -Fq -- '--disclosure-validate requires --thread-disclosure' "$tmpdir/disclosure-validate.err"

if $driver --disclosure-nested 'Ran a command' >"$tmpdir/disclosure-nested.out" 2>"$tmpdir/disclosure-nested.err"; then
  echo "capture driver accepted a nested disclosure without an outer target" >&2
  exit 1
fi
grep -Fq -- '--disclosure-nested requires --thread-disclosure' "$tmpdir/disclosure-nested.err"

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

if $driver --settings-tab appearance >"$tmpdir/settings-tab.out" 2>"$tmpdir/settings-tab.err"; then
  echo "capture driver accepted a Settings tab without Settings mode" >&2
  exit 1
fi
grep -Fq -- '--settings-tab requires --settings' "$tmpdir/settings-tab.err"

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
grep -Fq 'case "thread"' "$driver"
grep -Fq 'case "thread-tail"' "$driver"
grep -Fq 'case "approval-tail"' "$driver"
grep -Fq 'case "settings-sidebar"' "$driver"
# Literal source contract; expansion would weaken the assertion.
# shellcheck disable=SC2016
grep -Fq 'window_text_center "$ocr_capture" "$validation_text" "$validation_region"' "$driver"
grep -Fq 'if ((starter_seeded))' "$driver"
grep -Fq 'if ((composer_seeded))' "$driver"
# Literal source contract; expansion would weaken the assertion.
# shellcheck disable=SC2016
grep -Fq 'if [[ "$mode" == "composer-send" ]]' "$driver"
# Literal source contract; expansion would weaken the assertion.
# shellcheck disable=SC2016
grep -Fq 'if [[ "$mode" == "composer-send" && "$composer_mode" == "plan" ]]' "$driver"
# Literal source contract; expansion would weaken the assertion.
# shellcheck disable=SC2016
grep -Fq 'if [[ "$mode" == "composer-send" && "$composer_access" != "current" ]]' "$driver"
grep -Fq 'for ask_ocr in "Ask for approval" "Askfor approval"' "$driver"
grep -Fq 'for full_ocr in "Full access" "FullAccess"' "$driver"
grep -Fq 'access_current_full=0' "$driver"
# Literal source contract; expansion would weaken the assertion.
# shellcheck disable=SC2016
grep -Fq 'if [[ "$composer_access" == "full" && "$access_current_full" == "1" ]]' "$driver"
# Literal source contract; expansion would weaken the assertion.
# shellcheck disable=SC2016
grep -Fq 'if [[ "$access_current_full" != "1" ]]' "$driver"
# Literal source contracts; expansion would weaken the assertions.
# shellcheck disable=SC2016
grep -Fq 'window_text_center "$ocr_capture" "Turn on Full Access" "main"' "$driver"
grep -Fq 'access-confirm-debug.png' "$driver"
# Literal source contract; expansion would weaken the assertion.
# shellcheck disable=SC2016
grep -Fq 'window_text_center "$ocr_capture" "Turn plan mode off" "popover-low"' "$driver"
# Literal source contract; expansion would weaken the assertion.
# shellcheck disable=SC2016
grep -Fq 'window_text_center "$ocr_capture" "What should we build" "main"' "$driver"
# Literal source contract; expansion would weaken the assertion.
# shellcheck disable=SC2016
grep -Fq 'window_text_center "$ocr_capture" "Explore and" "starter"' "$driver"
# Literal source contract; expansion would weaken the assertion.
# shellcheck disable=SC2016
grep -Fq 'window_text_center "$ocr_capture" "$composer_validate" "main"' "$driver"
# Literal source contract; expansion would weaken the assertion.
# shellcheck disable=SC2016
grep -Fq 'if [[ "$mode" == "thread-composer-send" ]]' "$driver"
# Literal source contract; expansion would weaken the assertion.
# shellcheck disable=SC2016
grep -Fq 'if [[ "$mode" == "thread-disclosure" ]]' "$driver"
# Literal source contracts; expansion would weaken the assertions.
# shellcheck disable=SC2016
grep -Fq 'if [[ "$mode" == "thread-approval" ]]' "$driver"
# shellcheck disable=SC2016
grep -Fq 'approval_target=$([[ "$thread_approval" == "allow-once" ]]' "$driver"
grep -Fq 'if ((disclosure_open))' "$driver"
grep -Fq 'if ((disclosure_nested_open))' "$driver"
# Literal source contracts; expansion would weaken the assertions.
# shellcheck disable=SC2016
grep -Fq 'window_text_center "$ocr_capture" "$disclosure_nested_target" "main"' "$driver"
# shellcheck disable=SC2016
grep -Fq 'window_text_center "$ocr_capture" "$disclosure_nested_prefix" "main"' "$driver"
grep -Fq 'codex-thread-disclosure-normalize-outer' "$driver"
grep -Fq 'nested disclosure did not collapse' "$driver"
grep -Fq 'disclosure-nested-debug.png' "$driver"
grep -Fq 'disclosure-validation-debug.png' "$driver"
grep -Fq 'if matches.isEmpty, let doubledImage = doubled(image)' "$driver"
grep -Fq 'foldedQuery.count >= 6 && foldedCandidate.contains(foldedQuery)' "$driver"
grep -Fq 'codex-settings-validate' "$driver"
grep -Fq 'if ((thread_composer_seeded))' "$driver"
grep -Fq 'send_key 36 1' "$driver"
# Literal source contract; expansion would weaken the assertion.
# shellcheck disable=SC2016
grep -Fq 'window_text_center "$ocr_capture" "$composer_validate" "thread"' "$driver"
grep -Fq 'if ((plan_enabled))' "$driver"
grep -Fq 'if ((goal_enabled))' "$driver"
grep -Fq '"Turn plan mode on" "popover-low"' "$driver"
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
grep -Fq 'trap - EXIT' "$driver"
# Literal source contract; variable expansion would defeat this assertion.
# shellcheck disable=SC2016
grep -Fq 'if [[ -n "$restore_query" && -n "$surface" ]]' "$driver"

echo "test-capture-codex-ui: ok"
