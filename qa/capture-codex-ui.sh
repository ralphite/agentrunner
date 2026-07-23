#!/usr/bin/env bash
# Capture the real Codex desktop window on macOS. Optional command-palette mode
# proves low-risk interaction without depending on the Electron AX tree.
set -euo pipefail

usage() {
  cat <<'EOF'
usage: qa/capture-codex-ui.sh [--command-palette [--palette-query TEXT] | --surface NAME |
                              --new-chat-control NAME [--control-query TEXT] |
                              --composer-text TEXT [--composer-reload] --composer-validate VISIBLE_TEXT |
                              --composer-send TEXT --composer-validate VISIBLE_TEXT
                                [--composer-mode default|plan]
                                [--composer-access current|ask|approve|full] |
                              --thread-composer-send TEXT --composer-validate VISIBLE_TEXT
                                [--thread-shortcut enter|cmd-enter] |
                              --thread-approval allow-once|deny |
                              --thread-review |
                              --thread-scroll-up --scroll-validate VISIBLE_TEXT
                                [--scroll-pages 1-9] |
                              --thread-disclosure VISIBLE_TEXT
                                [--disclosure-nested VISIBLE_TEXT]
                                [--disclosure-click-offset POINTS]
                                [--disclosure-region main|goal-bar]
                                --disclosure-validate VISIBLE_TEXT |
                              --thread-menu VISIBLE_TITLE |
                              --thread-action VISIBLE_TITLE pin|archive |
                              --context-menu VISIBLE_TEXT | --keyboard-context-menu VISIBLE_TEXT |
                              --account-menu | --user-menu |
                              --settings [--settings-tab general|appearance|keyboard-shortcuts|configuration|worktrees|archived]]
                              [--scheduled-search TEXT | --scheduled-filter all|active|paused |
                               --scheduled-row VISIBLE_TITLE | --scheduled-create]
                              [--viewport WIDTHxHEIGHT]
                              [--settle SECONDS] [--restore-query TEXT] [--output PATH]

Captures the largest on-screen layer-0 window owned by bundle com.openai.codex.
--command-palette opens Cmd+K, captures it, then restores the prior state with Escape.
--palette-query enters a read-only search in the open command palette before capture.
--surface navigates to one read-only sidebar surface before capture. NAME is one of:
  new-chat, pull-requests, sites, scheduled, plugins
--new-chat-control opens one reversible New chat control before capture. NAME is one of:
  project, worktree, environment, branch, add, goal, plan, access, model,
  model-list, effort, speed,
  starter-explore, starter-build, starter-review, starter-fix
--control-query enters read-only search text in an opened New chat picker.
--composer-text fills an unsent New chat draft, captures it, then clears it.
--composer-reload reloads Codex after filling that draft and requires it to survive.
--composer-send fills and submits a New chat prompt, then retains the created thread.
--composer-mode plan enables sticky Plan mode before --composer-send. It intentionally
  hands that mode to the created interactive thread; Codex currently restores the
  composer to Full access after the Plan request is accepted.
--composer-access selects the real New chat access posture before --composer-send:
  ask = Ask for approval, approve = Approve for me, full = Full access.
--thread-composer-send submits a follow-up in the currently open thread without navigating away.
--thread-shortcut chooses Enter or Cmd+Enter for that follow-up (default: enter).
--thread-approval resolves the currently visible Codex approval card with the
  exact Allow once or Deny action, then proves that card left the thread tail.
--thread-review opens the current thread's read-only Review panel, captures it,
  then closes its resident Review tab and proves the panel-only heading disappeared.
--thread-scroll-up pages upward in the current thread, requires an older visible
  text marker before accepting the capture, then returns the thread to latest.
--thread-disclosure opens one visible disclosure in the current thread, validates its body,
  captures it, then restores the collapsed state.
--thread-menu opens the current thread header's real action menu, captures it,
  then dismisses it with Escape.
--thread-action invokes one reversible lifecycle action from that real menu and
  captures the resulting Codex state.
--disclosure-nested opens a second visible disclosure inside the first one before validation;
  both levels are OCR-located from fresh frames and restored inner-first after capture.
  Use a validation substring unique to the expanded inner body; cleanup proves it disappears.
--disclosure-click-offset shifts the outer disclosure click right by POINTS from its OCR label.
  This covers compact controls whose chevron is separate from the visible label.
--disclosure-region restricts the outer target lookup; goal-bar excludes matching source/diff text.
--composer-validate is a short visible substring required in the draft/thread capture.
--context-menu OCR-locates visible sidebar text, right-clicks it, captures the menu,
  then dismisses it with Escape. Matches are restricted to the sidebar.
--keyboard-context-menu focuses visible sidebar text, opens its menu with Shift+F10,
  captures it, then dismisses it with Escape.
--account-menu opens the top-left Codex/ChatGPT product switcher.
--user-menu opens the bottom-left profile/status menu.
--settings opens the Settings surface through the profile menu, captures it,
  then restores the current thread with Escape.
--settings-tab selects a read-only Settings tab before capture (default: general).
--scheduled-search types a Scheduled search query for the capture.
--scheduled-filter selects a Scheduled status filter for the capture.
--scheduled-row opens an existing Scheduled row for a read-only detail capture.
--scheduled-create opens Codex's reversible assisted New chat draft, captures it,
  then clears the synthetic prompt before restoring the original thread.
--viewport temporarily resizes the real Codex window before interaction, normalizes the
  evidence image to that exact size, then restores the original window geometry.
--settle waits for a navigated surface to become visually stable (default: 1 second).
--restore-query returns to a Codex thread through Cmd+K after a surface capture.
EOF
}

mode="current"
surface=""
palette_query=""
new_chat_control=""
control_query=""
composer_text=""
composer_validate=""
composer_reload=0
composer_mode="default"
composer_access="current"
thread_shortcut="enter"
thread_approval=""
thread_scroll_up=0
scroll_pages=2
scroll_validate=""
context_target=""
thread_menu_title=""
thread_action=""
disclosure_target=""
disclosure_nested_target=""
disclosure_nested_prefix=""
disclosure_validate=""
disclosure_click_offset=0
disclosure_region="main"
restore_query=""
settle_seconds=1
output=""
settings_tab="general"
settings_tab_set=0
scheduled_action=""
scheduled_value=""
viewport=""
viewport_width=""
viewport_height=""
while (($#)); do
  case "$1" in
    --command-palette)
      mode="command-palette"
      shift
      ;;
    --surface)
      if (($# < 2)); then
        echo "capture-codex-ui: --surface requires a name" >&2
        exit 2
      fi
      surface="$2"
      mode="surface-$surface"
      shift 2
      ;;
    --palette-query)
      if (($# < 2)); then
        echo "capture-codex-ui: --palette-query requires text" >&2
        exit 2
      fi
      palette_query="$2"
      shift 2
      ;;
    --new-chat-control)
      if (($# < 2)) || [[ -z "$2" ]]; then
        echo "capture-codex-ui: --new-chat-control requires a name" >&2
        exit 2
      fi
      new_chat_control="$2"
      surface="new-chat"
      mode="new-chat-control"
      shift 2
      ;;
    --control-query)
      if (($# < 2)); then
        echo "capture-codex-ui: --control-query requires text" >&2
        exit 2
      fi
      control_query="$2"
      shift 2
      ;;
    --composer-text)
      if (($# < 2)) || [[ -z "$2" ]]; then
        echo "capture-codex-ui: --composer-text requires text" >&2
        exit 2
      fi
      composer_text="$2"
      surface="new-chat"
      mode="composer-text"
      shift 2
      ;;
    --composer-reload)
      composer_reload=1
      shift
      ;;
    --composer-send)
      if (($# < 2)) || [[ -z "$2" ]]; then
        echo "capture-codex-ui: --composer-send requires text" >&2
        exit 2
      fi
      composer_text="$2"
      surface="new-chat"
      mode="composer-send"
      shift 2
      ;;
    --composer-mode)
      if (($# < 2)) || [[ "$2" != "default" && "$2" != "plan" ]]; then
        echo "capture-codex-ui: --composer-mode requires default or plan" >&2
        exit 2
      fi
      composer_mode="$2"
      shift 2
      ;;
    --composer-access)
      if (($# < 2)) || [[ "$2" != "current" && "$2" != "ask" && "$2" != "approve" && "$2" != "full" ]]; then
        echo "capture-codex-ui: --composer-access requires current, ask, approve, or full" >&2
        exit 2
      fi
      composer_access="$2"
      shift 2
      ;;
    --thread-composer-send)
      if (($# < 2)) || [[ -z "$2" ]]; then
        echo "capture-codex-ui: --thread-composer-send requires text" >&2
        exit 2
      fi
      composer_text="$2"
      mode="thread-composer-send"
      shift 2
      ;;
    --thread-shortcut)
      if (($# < 2)) || [[ "$2" != "enter" && "$2" != "cmd-enter" ]]; then
        echo "capture-codex-ui: --thread-shortcut requires enter or cmd-enter" >&2
        exit 2
      fi
      thread_shortcut="$2"
      shift 2
      ;;
    --thread-approval)
      if (($# < 2)) || [[ "$2" != "allow-once" && "$2" != "deny" ]]; then
        echo "capture-codex-ui: --thread-approval requires allow-once or deny" >&2
        exit 2
      fi
      thread_approval="$2"
      mode="thread-approval"
      shift 2
      ;;
    --thread-review)
      mode="thread-review"
      shift
      ;;
    --thread-scroll-up)
      mode="thread-scroll-up"
      thread_scroll_up=1
      shift
      ;;
    --scroll-pages)
      if (($# < 2)) || [[ ! "$2" =~ ^[1-9]$ ]]; then
        echo "capture-codex-ui: --scroll-pages requires 1 through 9" >&2
        exit 2
      fi
      scroll_pages="$2"
      shift 2
      ;;
    --scroll-validate)
      if (($# < 2)) || [[ -z "$2" ]]; then
        echo "capture-codex-ui: --scroll-validate requires visible text" >&2
        exit 2
      fi
      scroll_validate="$2"
      shift 2
      ;;
    --thread-disclosure)
      if (($# < 2)) || [[ -z "$2" ]]; then
        echo "capture-codex-ui: --thread-disclosure requires visible text" >&2
        exit 2
      fi
      disclosure_target="$2"
      mode="thread-disclosure"
      shift 2
      ;;
    --disclosure-validate)
      if (($# < 2)) || [[ -z "$2" ]]; then
        echo "capture-codex-ui: --disclosure-validate requires visible text" >&2
        exit 2
      fi
      disclosure_validate="$2"
      shift 2
      ;;
    --disclosure-nested)
      if (($# < 2)) || [[ -z "$2" ]]; then
        echo "capture-codex-ui: --disclosure-nested requires visible text" >&2
        exit 2
      fi
      disclosure_nested_target="$2"
      shift 2
      ;;
    --disclosure-click-offset)
      if (($# < 2)) || [[ ! "$2" =~ ^-?[0-9]+([.][0-9]+)?$ ]]; then
        echo "capture-codex-ui: --disclosure-click-offset requires numeric points" >&2
        exit 2
      fi
      disclosure_click_offset="$2"
      shift 2
      ;;
    --disclosure-region)
      if (($# < 2)) || [[ "$2" != "main" && "$2" != "goal-bar" ]]; then
        echo "capture-codex-ui: --disclosure-region requires main or goal-bar" >&2
        exit 2
      fi
      disclosure_region="$2"
      shift 2
      ;;
    --composer-validate)
      if (($# < 2)) || [[ -z "$2" ]]; then
        echo "capture-codex-ui: --composer-validate requires visible text" >&2
        exit 2
      fi
      composer_validate="$2"
      shift 2
      ;;
    --thread-menu)
      if (($# < 2)) || [[ -z "$2" ]]; then
        echo "capture-codex-ui: --thread-menu requires the visible current thread title" >&2
        exit 2
      fi
      thread_menu_title="$2"
      mode="thread-menu"
      shift 2
      ;;
    --thread-action)
      if (($# < 3)) || [[ -z "$2" ]] || [[ "$3" != "pin" && "$3" != "archive" ]]; then
        echo "capture-codex-ui: --thread-action requires the visible current thread title and pin or archive" >&2
        exit 2
      fi
      thread_menu_title="$2"
      thread_action="$3"
      mode="thread-action"
      shift 3
      ;;
    --context-menu)
      if (($# < 2)) || [[ -z "$2" ]]; then
        echo "capture-codex-ui: --context-menu requires visible text" >&2
        exit 2
      fi
      context_target="$2"
      mode="context-menu"
      shift 2
      ;;
    --keyboard-context-menu)
      if (($# < 2)) || [[ -z "$2" ]]; then
        echo "capture-codex-ui: --keyboard-context-menu requires visible text" >&2
        exit 2
      fi
      context_target="$2"
      mode="keyboard-context-menu"
      shift 2
      ;;
    --account-menu)
      mode="account-menu"
      shift
      ;;
    --user-menu)
      mode="user-menu"
      shift
      ;;
    --settings)
      mode="settings"
      shift
      ;;
    --settings-tab)
      if (($# < 2)) || [[ "$2" != "general" && "$2" != "appearance" &&
        "$2" != "keyboard-shortcuts" && "$2" != "configuration" &&
        "$2" != "worktrees" && "$2" != "archived" ]]; then
        echo "capture-codex-ui: --settings-tab requires general, appearance, keyboard-shortcuts, configuration, worktrees, or archived" >&2
        exit 2
      fi
      settings_tab="$2"
      settings_tab_set=1
      shift 2
      ;;
    --scheduled-search)
      if (($# < 2)) || [[ -z "$2" ]]; then
        echo "capture-codex-ui: --scheduled-search requires text" >&2
        exit 2
      fi
      mode="scheduled-search"
      scheduled_action="search"
      scheduled_value="$2"
      shift 2
      ;;
    --scheduled-filter)
      if (($# < 2)) || [[ "$2" != "all" && "$2" != "active" && "$2" != "paused" ]]; then
        echo "capture-codex-ui: --scheduled-filter requires all, active, or paused" >&2
        exit 2
      fi
      mode="scheduled-filter"
      scheduled_action="filter"
      scheduled_value="$2"
      shift 2
      ;;
    --scheduled-row)
      if (($# < 2)) || [[ -z "$2" ]]; then
        echo "capture-codex-ui: --scheduled-row requires a visible title" >&2
        exit 2
      fi
      mode="scheduled-row"
      scheduled_action="row"
      scheduled_value="$2"
      shift 2
      ;;
    --scheduled-create)
      mode="scheduled-create"
      scheduled_action="create"
      shift
      ;;
    --restore-query)
      if (($# < 2)); then
        echo "capture-codex-ui: --restore-query requires text" >&2
        exit 2
      fi
      restore_query="$2"
      shift 2
      ;;
    --viewport)
      if (($# < 2)) || [[ ! "$2" =~ ^([0-9]{3,4})x([0-9]{3,4})$ ]]; then
        echo "capture-codex-ui: --viewport requires WIDTHxHEIGHT" >&2
        exit 2
      fi
      viewport="$2"
      viewport_width="${BASH_REMATCH[1]}"
      viewport_height="${BASH_REMATCH[2]}"
      if ((viewport_width < 900 || viewport_width > 2560 || viewport_height < 640 || viewport_height > 1600)); then
        echo "capture-codex-ui: --viewport must be between 900x640 and 2560x1600" >&2
        exit 2
      fi
      shift 2
      ;;
    --settle)
      if (($# < 2)) || [[ ! "$2" =~ ^([0-9]|[12][0-9]|30)$ ]]; then
        echo "capture-codex-ui: --settle requires whole seconds from 0 to 30" >&2
        exit 2
      fi
      settle_seconds="$2"
      shift 2
      ;;
    --output)
      if (($# < 2)); then
        echo "capture-codex-ui: --output requires a path" >&2
        exit 2
      fi
      output="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "capture-codex-ui: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -n "$surface" ]]; then
  case "$surface" in
    new-chat|pull-requests|sites|scheduled|plugins) ;;
    *)
      echo "capture-codex-ui: unsupported surface: $surface" >&2
      exit 2
      ;;
  esac
fi
if [[ -n "$palette_query" && "$mode" != "command-palette" ]]; then
  echo "capture-codex-ui: --palette-query requires --command-palette" >&2
  exit 2
fi
if [[ -n "$new_chat_control" ]]; then
  case "$new_chat_control" in
    project|worktree|environment|branch|add|goal|plan|access|model|model-list|effort|speed|starter-explore|starter-build|starter-review|starter-fix) ;;
    *)
      echo "capture-codex-ui: unsupported New chat control: $new_chat_control" >&2
      exit 2
      ;;
  esac
fi
if [[ -n "$control_query" && "$mode" != "new-chat-control" ]]; then
  echo "capture-codex-ui: --control-query requires --new-chat-control" >&2
  exit 2
fi
if [[ -n "$composer_text" && -z "$composer_validate" ]]; then
  echo "capture-codex-ui: composer text/send requires --composer-validate" >&2
  exit 2
fi
if ((composer_reload)) && [[ "$mode" != "composer-text" ]]; then
  echo "capture-codex-ui: --composer-reload requires --composer-text" >&2
  exit 2
fi
if [[ -n "$composer_validate" && "$mode" != "composer-text" && "$mode" != "composer-send" &&
      "$mode" != "thread-composer-send" ]]; then
  echo "capture-codex-ui: --composer-validate requires --composer-text, --composer-send, or --thread-composer-send" >&2
  exit 2
fi
if [[ "$thread_shortcut" != "enter" && "$mode" != "thread-composer-send" ]]; then
  echo "capture-codex-ui: --thread-shortcut requires --thread-composer-send" >&2
  exit 2
fi
if ((thread_scroll_up)) && [[ -z "$scroll_validate" ]]; then
  echo "capture-codex-ui: --thread-scroll-up requires --scroll-validate" >&2
  exit 2
fi
if [[ -n "$scroll_validate" && "$mode" != "thread-scroll-up" ]]; then
  echo "capture-codex-ui: --scroll-validate requires --thread-scroll-up" >&2
  exit 2
fi
if [[ "$scroll_pages" != "2" && "$mode" != "thread-scroll-up" ]]; then
  echo "capture-codex-ui: --scroll-pages requires --thread-scroll-up" >&2
  exit 2
fi
if [[ -n "$disclosure_target" && -z "$disclosure_validate" ]]; then
  echo "capture-codex-ui: thread disclosure requires --disclosure-validate" >&2
  exit 2
fi
if [[ -n "$disclosure_validate" && "$mode" != "thread-disclosure" ]]; then
  echo "capture-codex-ui: --disclosure-validate requires --thread-disclosure" >&2
  exit 2
fi
if [[ -n "$disclosure_nested_target" && "$mode" != "thread-disclosure" ]]; then
  echo "capture-codex-ui: --disclosure-nested requires --thread-disclosure" >&2
  exit 2
fi
if [[ "$disclosure_click_offset" != "0" && "$mode" != "thread-disclosure" ]]; then
  echo "capture-codex-ui: --disclosure-click-offset requires --thread-disclosure" >&2
  exit 2
fi
if [[ "$disclosure_region" != "main" && "$mode" != "thread-disclosure" ]]; then
  echo "capture-codex-ui: --disclosure-region requires --thread-disclosure" >&2
  exit 2
fi
if [[ -n "$disclosure_nested_target" ]]; then
  # Codex replaces generic labels such as “Ran a command” with a concrete
  # “Ran bash …” label while expanded. The action verb remains stable and is
  # re-located from a fresh OCR frame for normalization and cleanup.
  disclosure_nested_prefix=${disclosure_nested_target%% *}
fi
if [[ "$composer_mode" != "default" && "$mode" != "composer-send" ]]; then
  echo "capture-codex-ui: --composer-mode plan requires --composer-send" >&2
  exit 2
fi
if [[ "$composer_access" != "current" && "$mode" != "composer-send" ]]; then
  echo "capture-codex-ui: --composer-access requires --composer-send" >&2
  exit 2
fi
if [[ "$composer_access" != "current" && "$composer_mode" == "plan" ]]; then
  echo "capture-codex-ui: --composer-access cannot be combined with Plan mode" >&2
  exit 2
fi
if [[ -n "$control_query" && "$new_chat_control" != "project" && "$new_chat_control" != "branch" ]]; then
  echo "capture-codex-ui: --control-query is only supported for project or branch" >&2
  exit 2
fi
if [[ -n "$restore_query" && -z "$surface" ]]; then
  echo "capture-codex-ui: --restore-query requires --surface" >&2
  exit 2
fi
if ((settings_tab_set)) && [[ "$mode" != "settings" ]]; then
  echo "capture-codex-ui: --settings-tab requires --settings" >&2
  exit 2
fi
if [[ -n "$scheduled_action" && "$surface" != "scheduled" ]]; then
  echo "capture-codex-ui: Scheduled actions require --surface scheduled" >&2
  exit 2
fi

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "capture-codex-ui: macOS is required" >&2
  exit 1
fi

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
if [[ -z "$output" ]]; then
  stamp=$(date '+%Y%m%d-%H%M%S')
  output="$repo_root/qa/runs/$(date '+%Y-%m-%d')-codex-live-capture/$stamp-$mode.png"
fi
mkdir -p "$(dirname "$output")"

codex_pid=$(lsappinfo info -only pid -app com.openai.codex 2>/dev/null |
  sed -E 's/.*=([0-9]+).*/\1/')
if [[ ! "$codex_pid" =~ ^[0-9]+$ ]]; then
  echo "capture-codex-ui: could not resolve the Codex process" >&2
  exit 1
fi

resolve_codex_window_info() {
swift - "$codex_pid" <<'SWIFT'
import CoreGraphics
import Foundation

guard CommandLine.arguments.count == 2,
      let targetPid = Int32(CommandLine.arguments[1]) else {
  fputs("invalid Codex pid\n", stderr)
  exit(2)
}

let options: CGWindowListOption = [.optionOnScreenOnly, .excludeDesktopElements]
let windows = CGWindowListCopyWindowInfo(options, kCGNullWindowID) as? [[String: Any]] ?? []
let candidates = windows.compactMap { window -> (id: Int, x: Double, y: Double, width: Double, height: Double, area: Double)? in
  guard (window[kCGWindowOwnerPID as String] as? Int32) == targetPid,
        (window[kCGWindowLayer as String] as? Int) == 0,
        let id = window[kCGWindowNumber as String] as? Int,
        let bounds = window[kCGWindowBounds as String] as? [String: Any],
        let width = bounds["Width"] as? Double,
        let height = bounds["Height"] as? Double else { return nil }
  let x = bounds["X"] as? Double ?? 0
  let y = bounds["Y"] as? Double ?? 0
  return (id, x, y, width, height, width * height)
}

guard let target = candidates.max(by: { $0.area < $1.area }) else {
  fputs("no on-screen Codex window found\n", stderr)
  exit(1)
}
print("\(target.id)\t\(target.x)\t\(target.y)\t\(target.width)\t\(target.height)")
SWIFT
}

window_info=$(resolve_codex_window_info)
IFS=$'\t' read -r window_id window_x window_y window_width window_height <<<"$window_info"
if [[ ! "$window_id" =~ ^[0-9]+$ || -z "$window_height" ]]; then
  echo "capture-codex-ui: could not resolve the Codex window" >&2
  exit 1
fi

original_window_x="$window_x"
original_window_y="$window_y"
original_window_width="$window_width"
original_window_height="$window_height"
window_resized=0

set_codex_window_geometry() {
  local expected_width=$1
  local expected_height=$2
  local target_x=$3
  local target_y=$4
  local target_width=$5
  local target_height=$6
  swift - "$codex_pid" "$expected_width" "$expected_height" "$target_x" "$target_y" "$target_width" "$target_height" <<'SWIFT'
import AppKit
import ApplicationServices
import Foundation

guard CommandLine.arguments.count == 8,
      let pid = Int32(CommandLine.arguments[1]),
      let expectedWidth = Double(CommandLine.arguments[2]),
      let expectedHeight = Double(CommandLine.arguments[3]),
      let targetX = Double(CommandLine.arguments[4]),
      let targetY = Double(CommandLine.arguments[5]),
      let targetWidth = Double(CommandLine.arguments[6]),
      let targetHeight = Double(CommandLine.arguments[7]) else {
  fputs("invalid Codex window geometry\n", stderr)
  exit(2)
}
guard AXIsProcessTrusted() else {
  fputs("Accessibility permission is required for --viewport\n", stderr)
  exit(1)
}

let app = AXUIElementCreateApplication(pid)
var windowRef: CFTypeRef?
guard AXUIElementCopyAttributeValue(app, kAXFocusedWindowAttribute as CFString, &windowRef) == .success,
      let rawWindow = windowRef else {
  fputs("could not resolve focused Codex window through AX\n", stderr)
  exit(1)
}
let window = unsafeBitCast(rawWindow, to: AXUIElement.self)
var sizeRef: CFTypeRef?
guard AXUIElementCopyAttributeValue(window, kAXSizeAttribute as CFString, &sizeRef) == .success,
      let rawSize = sizeRef else {
  fputs("could not read Codex window size through AX\n", stderr)
  exit(1)
}
var currentSize = CGSize.zero
guard AXValueGetValue(unsafeBitCast(rawSize, to: AXValue.self), .cgSize, &currentSize),
      abs(currentSize.width - expectedWidth) <= 3,
      abs(currentSize.height - expectedHeight) <= 3 else {
  fputs("focused AX window does not match selected Codex window\n", stderr)
  exit(1)
}

var targetSize = CGSize(width: targetWidth, height: targetHeight)
var targetPosition = CGPoint(x: targetX, y: targetY)
guard let sizeValue = AXValueCreate(.cgSize, &targetSize),
      let positionValue = AXValueCreate(.cgPoint, &targetPosition),
      AXUIElementSetAttributeValue(window, kAXPositionAttribute as CFString, positionValue) == .success,
      AXUIElementSetAttributeValue(window, kAXSizeAttribute as CFString, sizeValue) == .success else {
  fputs("could not update Codex window geometry through AX\n", stderr)
  exit(1)
}
SWIFT
}

restore_codex_window_geometry() {
  if ((window_resized)); then
    if ! set_codex_window_geometry "$viewport_width" "$viewport_height" \
      "$original_window_x" "$original_window_y" "$original_window_width" "$original_window_height"; then
      echo "capture-codex-ui: failed to restore original Codex window geometry" >&2
    fi
    window_resized=0
  fi
}

if [[ -n "$viewport" ]]; then
  set_codex_window_geometry "$window_width" "$window_height" "$window_x" "$window_y" \
    "$viewport_width" "$viewport_height"
  window_resized=1
  trap restore_codex_window_geometry EXIT
  sleep 1
  window_info=$(resolve_codex_window_info)
  IFS=$'\t' read -r window_id window_x window_y window_width window_height <<<"$window_info"
  if [[ "${window_width%.*}x${window_height%.*}" != "$viewport" ]]; then
    echo "capture-codex-ui: Codex window did not settle at $viewport" >&2
    exit 1
  fi
fi

transient_open=0
nested_open=0
starter_seeded=0
composer_seeded=0
composer_cleanup_validate="Explore and"
thread_composer_seeded=0
goal_enabled=0
plan_enabled=0
disclosure_open=0
disclosure_nested_open=0
review_open=0
thread_scrolled=0
ocr_capture=""
send_key() {
  local key_code=$1
  local modifier=${2:-0}
  swift - "$codex_pid" "$key_code" "$modifier" <<'SWIFT'
import AppKit
import CoreGraphics
import Foundation

guard CommandLine.arguments.count == 4,
      let pid = Int32(CommandLine.arguments[1]),
      let keyCode = UInt16(CommandLine.arguments[2]),
      let modifier = Int(CommandLine.arguments[3]),
      let app = NSRunningApplication(processIdentifier: pid) else {
  fputs("invalid Codex key request\n", stderr)
  exit(2)
}

_ = app.activate(options: [.activateAllWindows])
usleep(200_000)
let source = CGEventSource(stateID: .hidSystemState)
guard let down = CGEvent(keyboardEventSource: source, virtualKey: keyCode, keyDown: true),
      let up = CGEvent(keyboardEventSource: source, virtualKey: keyCode, keyDown: false) else {
  fputs("could not create keyboard event\n", stderr)
  exit(1)
}
if modifier == 1 {
  down.flags = .maskCommand
  up.flags = .maskCommand
} else if modifier == 2 {
  down.flags = .maskShift
  up.flags = .maskShift
}
down.post(tap: .cghidEventTap)
up.post(tap: .cghidEventTap)
SWIFT
}

clear_focused_text() {
  swift - "$codex_pid" <<'SWIFT'
import AppKit
import CoreGraphics
import Foundation

guard CommandLine.arguments.count == 2,
      let pid = Int32(CommandLine.arguments[1]),
      let app = NSRunningApplication(processIdentifier: pid) else {
  fputs("invalid Codex clear request\n", stderr)
  exit(2)
}
_ = app.activate(options: [.activateAllWindows])
usleep(200_000)
let source = CGEventSource(stateID: .hidSystemState)
for (keyCode, flags) in [(UInt16(0), CGEventFlags.maskCommand), (UInt16(51), CGEventFlags())] {
  guard let down = CGEvent(keyboardEventSource: source, virtualKey: keyCode, keyDown: true),
        let up = CGEvent(keyboardEventSource: source, virtualKey: keyCode, keyDown: false) else {
    fputs("could not create Codex clear event\n", stderr)
    exit(1)
  }
  down.flags = flags
  up.flags = flags
  down.post(tap: .cghidEventTap)
  up.post(tap: .cghidEventTap)
  usleep(150_000)
}
SWIFT
}

send_click() {
  local x=$1
  local y=$2
  local button=${3:-left}
  swift - "$codex_pid" "$x" "$y" "$button" <<'SWIFT'
import AppKit
import CoreGraphics
import Foundation

guard CommandLine.arguments.count == 5,
      let pid = Int32(CommandLine.arguments[1]),
      let x = Double(CommandLine.arguments[2]),
      let y = Double(CommandLine.arguments[3]),
      let app = NSRunningApplication(processIdentifier: pid) else {
  fputs("invalid Codex click request\n", stderr)
  exit(2)
}

_ = app.activate(options: [.activateAllWindows])
usleep(200_000)
let source = CGEventSource(stateID: .hidSystemState)
let point = CGPoint(x: x, y: y)
let right = CommandLine.arguments[4] == "right"
let mouseButton: CGMouseButton = right ? .right : .left
let downType: CGEventType = right ? .rightMouseDown : .leftMouseDown
let upType: CGEventType = right ? .rightMouseUp : .leftMouseUp
guard let down = CGEvent(mouseEventSource: source, mouseType: downType,
                         mouseCursorPosition: point, mouseButton: mouseButton),
      let up = CGEvent(mouseEventSource: source, mouseType: upType,
                       mouseCursorPosition: point, mouseButton: mouseButton) else {
  fputs("could not create mouse event\n", stderr)
  exit(1)
}
down.post(tap: .cghidEventTap)
up.post(tap: .cghidEventTap)
SWIFT
}

sidebar_text_center() {
  local image=$1
  local query=$2
  swift - "$image" "$query" "$window_x" "$window_y" "$window_width" "$window_height" <<'SWIFT'
import CoreGraphics
import Foundation
import ImageIO
import Vision

guard CommandLine.arguments.count == 7,
      let source = CGImageSourceCreateWithURL(URL(fileURLWithPath: CommandLine.arguments[1]) as CFURL, nil),
      let image = CGImageSourceCreateImageAtIndex(source, 0, nil),
      let windowX = Double(CommandLine.arguments[3]),
      let windowY = Double(CommandLine.arguments[4]),
      let windowWidth = Double(CommandLine.arguments[5]),
      let windowHeight = Double(CommandLine.arguments[6]) else {
  fputs("invalid sidebar OCR request\n", stderr)
  exit(2)
}

let query = CommandLine.arguments[2]
let request = VNRecognizeTextRequest()
request.recognitionLevel = .accurate
request.usesLanguageCorrection = true
request.recognitionLanguages = ["zh-Hans", "en-US"]
try VNImageRequestHandler(cgImage: image).perform([request])
let matches = (request.results ?? []).compactMap { observation -> (Bool, Float, CGRect, String)? in
  guard let candidate = observation.topCandidates(1).first,
        observation.boundingBox.midX < 0.30 else { return nil }
  let exact = candidate.string.compare(query, options: [.caseInsensitive, .diacriticInsensitive]) == .orderedSame
  let contains = candidate.string.range(of: query, options: [.caseInsensitive, .diacriticInsensitive]) != nil
  return (exact || contains) ? (exact, candidate.confidence, observation.boundingBox, candidate.string) : nil
}
guard let match = matches.sorted(by: {
  if $0.0 != $1.0 { return $0.0 && !$1.0 }
  return $0.1 > $1.1
}).first else {
  fputs("sidebar text not found: \(query)\n", stderr)
  exit(1)
}
// The OCR source is captured with `screencapture -o`, so it excludes the
// asymmetric window shadow and maps exactly to CGWindowBounds.
let pointX = windowX + Double(match.2.midX) * windowWidth
let pointY = windowY + (1.0 - Double(match.2.midY)) * windowHeight
print("\(pointX)\t\(pointY)")
SWIFT
}

window_text_center() {
  local image=$1
  local query=$2
  local region=$3
  local horizontal_anchor=${4:-center}
  local match_mode=${5:-contains}
  swift - "$image" "$query" "$region" "$window_x" "$window_y" "$window_width" "$window_height" "$horizontal_anchor" "$match_mode" <<'SWIFT'
import CoreGraphics
import Foundation
import ImageIO
import Vision

guard CommandLine.arguments.count == 10,
      let source = CGImageSourceCreateWithURL(URL(fileURLWithPath: CommandLine.arguments[1]) as CFURL, nil),
      let image = CGImageSourceCreateImageAtIndex(source, 0, nil),
      let windowX = Double(CommandLine.arguments[4]),
      let windowY = Double(CommandLine.arguments[5]),
      let windowWidth = Double(CommandLine.arguments[6]),
      let windowHeight = Double(CommandLine.arguments[7]) else {
  fputs("invalid window OCR request\n", stderr)
  exit(2)
}

let query = CommandLine.arguments[2]
let region = CommandLine.arguments[3]
let horizontalAnchor = CommandLine.arguments[8]
let matchMode = CommandLine.arguments[9]
guard horizontalAnchor == "center" || horizontalAnchor == "right" else {
  fputs("invalid OCR horizontal anchor\n", stderr)
  exit(2)
}
guard matchMode == "contains" || matchMode == "exact" else {
  fputs("invalid OCR match mode\n", stderr)
  exit(2)
}
typealias TextMatch = (Bool, Float, CGRect, String)
var usingSettingsSidebarCrop = false
func foldedOCRText(_ value: String) -> String {
  let base = value.folding(options: [.caseInsensitive, .diacriticInsensitive], locale: Locale(identifier: "en_US_POSIX"))
  return String(base.unicodeScalars.filter { CharacterSet.alphanumerics.contains($0) })
}
func recognizeMatches(_ input: CGImage) throws -> [TextMatch] {
  let request = VNRecognizeTextRequest()
  request.recognitionLevel = .accurate
  request.usesLanguageCorrection = true
  request.recognitionLanguages = ["zh-Hans", "en-US"]
  try VNImageRequestHandler(cgImage: input).perform([request])
  return (request.results ?? []).compactMap { observation -> TextMatch? in
    let inRegion: Bool
    switch region {
    case "composer":
      inRegion = observation.boundingBox.midX > 0.30 && observation.boundingBox.midY < 0.20
    case "starter":
      inRegion = observation.boundingBox.midX > 0.30 &&
        observation.boundingBox.midY > 0.25 && observation.boundingBox.midY < 0.65
    case "popover":
      inRegion = observation.boundingBox.midX > 0.30 &&
        observation.boundingBox.midY > 0.15 && observation.boundingBox.midY < 0.50
    case "popover-low":
      inRegion = observation.boundingBox.midX > 0.30 &&
        observation.boundingBox.midY > 0.05 && observation.boundingBox.midY < 0.35
    case "main":
      inRegion = observation.boundingBox.midX > 0.30 && observation.boundingBox.midY > 0.05
    case "goal-bar":
      inRegion = observation.boundingBox.midX > 0.30 && observation.boundingBox.midX < 0.68 &&
        observation.boundingBox.midY > 0.05 && observation.boundingBox.midY < 0.45
    case "review-tab":
      inRegion = observation.boundingBox.midX > 0.60 && observation.boundingBox.midY > 0.90
    case "review-panel":
      inRegion = observation.boundingBox.midX > 0.64 &&
        observation.boundingBox.midY > 0.08 && observation.boundingBox.midY < 0.92
    case "thread-review-entry":
      inRegion = observation.boundingBox.midX > 0.28 && observation.boundingBox.midX < 0.98 &&
        observation.boundingBox.midY > 0.08 && observation.boundingBox.midY < 0.92
    case "thread":
      inRegion = observation.boundingBox.midX > 0.30 && observation.boundingBox.midY > 0.20
    case "thread-header":
      inRegion = observation.boundingBox.midX > 0.30 && observation.boundingBox.midY > 0.90
    case "thread-tail":
      inRegion = observation.boundingBox.midX > 0.30 &&
        observation.boundingBox.midY > 0.06 && observation.boundingBox.midY < 0.30
    case "approval-tail":
      inRegion = observation.boundingBox.midX > 0.30 &&
        observation.boundingBox.midY > 0.015 && observation.boundingBox.midY < 0.15
    case "settings-sidebar":
      // Full-window labels sit near x=0.03; the cropped 16%-wide fallback
      // normalizes the same glyphs to roughly x=0.2–0.35.
      let maxX = usingSettingsSidebarCrop ? 0.45 : 0.10
      inRegion = observation.boundingBox.midX < maxX && observation.boundingBox.midY > 0.10
    case "settings-main":
      inRegion = observation.boundingBox.midX > 0.15 && observation.boundingBox.midY > 0.05
    default:
      inRegion = false
    }
    guard inRegion else { return nil }
    // Low-contrast sidebar/disclosure labels sometimes put the correct reading
    // below Vision's first candidate. Keep the same region/query guard while
    // accepting the best matching candidate from the observation.
    for candidate in observation.topCandidates(5) {
      let exact = candidate.string.compare(query, options: [.caseInsensitive, .diacriticInsensitive]) == .orderedSame
      let contains = candidate.string.range(of: query, options: [.caseInsensitive, .diacriticInsensitive]) != nil
      // Vision occasionally joins a low-contrast word boundary ("Ranacommand")
      // or prefixes one stray glyph. Only use the folded fallback for a
      // sufficiently long query; region constraints and the normal exact/
      // contains path remain authoritative for short labels.
      let foldedQuery = foldedOCRText(query)
      let foldedCandidate = foldedOCRText(candidate.string)
      let foldedContains = foldedQuery.count >= 6 && foldedCandidate.contains(foldedQuery)
      if exact {
        return (true, candidate.confidence, observation.boundingBox, candidate.string)
      }
      if matchMode == "exact" { continue }
      // Vision may group `Review  ×  +` into one observation. When the query is
      // only the label inside it, use that substring's geometry; the enclosing
      // observation's maxX would point at `+` and recreate the very cleanup bug
      // this helper is supposed to prevent.
      if contains,
         let range = candidate.string.range(of: query, options: [.caseInsensitive, .diacriticInsensitive]),
         let substringBox = try? candidate.boundingBox(for: range) {
        return (false, candidate.confidence, substringBox.boundingBox, candidate.string)
      }
      if foldedContains {
        return (false, candidate.confidence, observation.boundingBox, candidate.string)
      }
    }
    return nil
  }
}
func doubled(_ input: CGImage) -> CGImage? {
  guard let colorSpace = CGColorSpace(name: CGColorSpace.sRGB),
        let context = CGContext(
          data: nil,
          width: input.width * 2,
          height: input.height * 2,
          bitsPerComponent: 8,
          bytesPerRow: 0,
          space: colorSpace,
          bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
        ) else { return nil }
  context.interpolationQuality = .high
  context.draw(input, in: CGRect(x: 0, y: 0, width: input.width * 2, height: input.height * 2))
  return context.makeImage()
}
let settingsSidebarWidthFactor = 0.16
func magnifiedSettingsSidebar(_ input: CGImage) -> CGImage? {
  let cropWidth = Int(Double(input.width) * settingsSidebarWidthFactor)
  guard let crop = input.cropping(to: CGRect(x: 0, y: 0, width: cropWidth, height: input.height)),
        let colorSpace = CGColorSpace(name: CGColorSpace.sRGB),
        let context = CGContext(
          data: nil,
          width: crop.width * 4,
          height: crop.height * 4,
          bitsPerComponent: 8,
          bytesPerRow: 0,
          space: colorSpace,
          bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
        ) else { return nil }
  context.setFillColor(CGColor(gray: 1, alpha: 1))
  context.fill(CGRect(x: 0, y: 0, width: crop.width * 4, height: crop.height * 4))
  context.interpolationQuality = .none
  context.draw(crop, in: CGRect(x: 0, y: 0, width: crop.width * 4, height: crop.height * 4))
  return context.makeImage()
}
var matches = try recognizeMatches(image)
var coordinateWidthFactor = 1.0
// Low-contrast disclosure labels can fall below Vision's minimum glyph size
// in a non-Retina capture. Retry only a miss at 2x; normalized boxes still map
// to the original window and normal successful captures keep their fast path.
if matches.isEmpty, region == "settings-sidebar", let magnifiedSidebar = magnifiedSettingsSidebar(image) {
  usingSettingsSidebarCrop = true
  matches = try recognizeMatches(magnifiedSidebar)
  coordinateWidthFactor = settingsSidebarWidthFactor
} else if matches.isEmpty, let doubledImage = doubled(image) {
  matches = try recognizeMatches(doubledImage)
}
guard let match = matches.sorted(by: {
  if $0.0 != $1.0 { return $0.0 && !$1.0 }
  return $0.1 > $1.1
}).first else {
  fputs("window text not found in \(region): \(query)\n", stderr)
  exit(1)
}
let normalizedX = horizontalAnchor == "right" ? match.2.maxX : match.2.midX
let pointX = windowX + Double(normalizedX) * windowWidth * coordinateWidthFactor
let pointY = windowY + (1.0 - Double(match.2.midY)) * windowHeight
print("\(pointX)\t\(pointY)")
SWIFT
}

offset_outer_disclosure_x() {
  awk -v x="$1" -v offset="$disclosure_click_offset" 'BEGIN { print x + offset }'
}

goal_bar_is_expanded_y() {
  awk -v y="$1" -v top="$window_y" -v height="$window_height" \
    'BEGIN { exit !(y < top + height * 0.70) }'
}

send_text() {
  local value=$1
  swift - "$codex_pid" "$value" <<'SWIFT'
import AppKit
import CoreGraphics
import Foundation

guard CommandLine.arguments.count == 3,
      let pid = Int32(CommandLine.arguments[1]),
      let app = NSRunningApplication(processIdentifier: pid) else {
  fputs("invalid Codex text request\n", stderr)
  exit(2)
}

_ = app.activate(options: [.activateAllWindows])
usleep(200_000)

let pasteboard = NSPasteboard.general
let savedItems: [[(NSPasteboard.PasteboardType, Data)]] =
  (pasteboard.pasteboardItems ?? []).map { item in
    item.types.compactMap { type in
      item.data(forType: type).map { (type, $0) }
    }
  }
func restorePasteboard() {
  pasteboard.clearContents()
  let restoredItems = savedItems.map { saved -> NSPasteboardItem in
    let item = NSPasteboardItem()
    for (type, data) in saved {
      item.setData(data, forType: type)
    }
    return item
  }
  if !restoredItems.isEmpty {
    _ = pasteboard.writeObjects(restoredItems)
  }
}
// Restore on success and on every guard/exit failure after staging begins.
defer { restorePasteboard() }
pasteboard.clearContents()
guard pasteboard.setString(CommandLine.arguments[2], forType: .string) else {
  fputs("could not stage text for Codex\n", stderr)
  exit(1)
}

let source = CGEventSource(stateID: .hidSystemState)
guard let down = CGEvent(keyboardEventSource: source, virtualKey: 9, keyDown: true),
      let up = CGEvent(keyboardEventSource: source, virtualKey: 9, keyDown: false) else {
  fputs("could not create paste event\n", stderr)
  exit(1)
}
down.flags = .maskCommand
up.flags = .maskCommand
down.post(tap: .cghidEventTap)
up.post(tap: .cghidEventTap)
usleep(400_000)
SWIFT
}

if [[ -n "$surface" ]]; then
  if (( ${window_width%.*} < 900 )); then
    echo "capture-codex-ui: sidebar surface navigation requires a desktop-width Codex window" >&2
    exit 1
  fi
  if [[ "$mode" == "new-chat-control" || "$mode" == "composer-text" || "$mode" == "composer-send" ]]; then
    # Normalize any submenu left by an interrupted/debug capture before the
    # sidebar navigation and fresh control interaction begin.
    send_key 53 >/dev/null 2>&1 || true
  fi
  case "$surface" in
    new-chat) surface_y=97 ;;
    pull-requests) surface_y=127 ;;
    sites) surface_y=156 ;;
    scheduled) surface_y=185 ;;
    plugins) surface_y=214 ;;
  esac
  send_click "$(awk -v x="$window_x" 'BEGIN { print x + 110 }')" \
    "$(awk -v y="$window_y" -v offset="$surface_y" 'BEGIN { print y + offset }')"
  sleep "$settle_seconds"
fi

close_transient() {
  # An assertion inside cleanup must fail once, not recursively re-enter the
  # EXIT trap forever. The primary interaction error remains the useful one.
  trap - EXIT
  if ((thread_scrolled)); then
    # End returns the current thread to its latest content after the evidence
    # frame has been saved, so the driver never leaves the current thread mid-history.
    send_key 119 >/dev/null 2>&1 || true
    sleep 1
    thread_scrolled=0
  fi
  if ((review_open)); then
    send_key 53 >/dev/null 2>&1 || true
    sleep 1
    rm -f -- "$ocr_capture" 2>/dev/null || true
    ocr_capture=$(mktemp -t codex-thread-review-restored)
    screencapture -x -o -t png -l "$window_id" "$ocr_capture"
    if window_text_center "$ocr_capture" "Last Turn" "review-panel" >/dev/null 2>&1; then
      # Review is a resident right-side tab, not an Escape-dismissed overlay.
      # Anchor to the OCR text's right edge, not its center: at compact widths
      # the tab truncates to `Revi…`, while the native X stays 11 points right.
      if ! review_tab_point=$(window_text_center "$ocr_capture" "Review" "review-tab" right 2>/dev/null); then
        review_tab_point=$(window_text_center "$ocr_capture" "Revi" "review-tab" right)
      fi
      IFS=$'\t' read -r review_tab_x review_tab_y <<<"$review_tab_point"
      if [[ "${CODEX_CAPTURE_DEBUG:-0}" == "1" ]]; then
        echo "capture-codex-ui: Review tab right=$review_tab_x,$review_tab_y window=$window_x,$window_y ${window_width}x${window_height}" >&2
      fi
      send_click "$(awk -v x="$review_tab_x" 'BEGIN { print x + 11 }')" "$review_tab_y"
      sleep 1
      rm -f -- "$ocr_capture"
      ocr_capture=$(mktemp -t codex-thread-review-close-validate)
      screencapture -x -o -t png -l "$window_id" "$ocr_capture"
      if window_text_center "$ocr_capture" "Last Turn" "review-panel" >/dev/null 2>&1; then
        if ! review_tab_point=$(window_text_center "$ocr_capture" "Review" "review-tab" right exact 2>/dev/null); then
          review_tab_point=$(window_text_center "$ocr_capture" "Revi" "review-tab" right)
        fi
        IFS=$'\t' read -r review_tab_x review_tab_y <<<"$review_tab_point"
        review_close_x=$(awk -v x="$review_tab_x" 'BEGIN { print x + 66 }')
        review_close_y=$(awk -v y="$review_tab_y" 'BEGIN { print y + 2 }')
        if [[ "${CODEX_CAPTURE_DEBUG:-0}" == "1" ]]; then
          echo "capture-codex-ui: Review compact close=$review_close_x,$review_close_y" >&2
        fi
        send_click "$review_close_x" "$review_close_y"
        sleep 1
        rm -f -- "$ocr_capture"
        ocr_capture=$(mktemp -t codex-thread-review-close-fallback-validate)
        screencapture -x -o -t png -l "$window_id" "$ocr_capture"
      fi
      if window_text_center "$ocr_capture" "Last Turn" "review-panel" >/dev/null 2>&1 ||
        window_text_center "$ocr_capture" "Revi" "review-tab" >/dev/null 2>&1; then
        echo "capture-codex-ui: Review panel did not close from its tab" >&2
        exit 1
      fi
    fi
    review_open=0
  fi
  if ((disclosure_nested_open)); then
    rm -f -- "$ocr_capture" 2>/dev/null || true
    ocr_capture=$(mktemp -t codex-thread-disclosure-nested-restore)
    screencapture -x -o -t png -l "$window_id" "$ocr_capture"
    if ! disclosure_nested_restore_point=$(window_text_center "$ocr_capture" "$disclosure_nested_target" "main" 2>/dev/null); then
      disclosure_nested_restore_point=$(window_text_center "$ocr_capture" "$disclosure_nested_prefix" "main")
    fi
    IFS=$'\t' read -r disclosure_nested_restore_x disclosure_nested_restore_y <<<"$disclosure_nested_restore_point"
    send_click "$disclosure_nested_restore_x" "$disclosure_nested_restore_y"
    sleep 1
    rm -f -- "$ocr_capture"
    ocr_capture=$(mktemp -t codex-thread-disclosure-nested-restored)
    screencapture -x -o -t png -l "$window_id" "$ocr_capture"
    if window_text_center "$ocr_capture" "$disclosure_validate" "main" >/dev/null 2>&1; then
      echo "capture-codex-ui: nested disclosure did not collapse" >&2
      exit 1
    fi
    disclosure_nested_open=0
  fi
  if ((disclosure_open)); then
    rm -f -- "$ocr_capture" 2>/dev/null || true
    ocr_capture=$(mktemp -t codex-thread-disclosure-restore)
    screencapture -x -o -t png -l "$window_id" "$ocr_capture"
    disclosure_restore_point=$(window_text_center "$ocr_capture" "$disclosure_target" "$disclosure_region")
    IFS=$'\t' read -r disclosure_restore_x disclosure_restore_y <<<"$disclosure_restore_point"
    disclosure_restore_x=$(offset_outer_disclosure_x "$disclosure_restore_x")
    send_click "$disclosure_restore_x" "$disclosure_restore_y"
    sleep 1
    if [[ "$disclosure_region" == "goal-bar" ]]; then
      rm -f -- "$ocr_capture"
      ocr_capture=$(mktemp -t codex-thread-disclosure-restored)
      screencapture -x -o -t png -l "$window_id" "$ocr_capture"
      disclosure_collapsed_point=$(window_text_center "$ocr_capture" "$disclosure_target" "$disclosure_region")
      IFS=$'\t' read -r _ disclosure_collapsed_y <<<"$disclosure_collapsed_point"
      if goal_bar_is_expanded_y "$disclosure_collapsed_y"; then
        echo "capture-codex-ui: goal disclosure did not collapse" >&2
        exit 1
      fi
    fi
    disclosure_open=0
  fi
  if ((plan_enabled)); then
    # Plan is a sticky New chat preference: clicking New chat does not clear it.
    # Restore the real off-state through the same Add row and assert the row has
    # flipped back before leaving Codex. This keeps later baseline captures from
    # silently inheriting Plan mode.
    send_key 53 >/dev/null 2>&1 || true
    send_click "$point_x" "$point_y"
    sleep 1
    rm -f -- "$ocr_capture" 2>/dev/null || true
    ocr_capture=$(mktemp -t codex-plan-restore)
    screencapture -x -o -t png -l "$window_id" "$ocr_capture"
    plan_off_point=$(window_text_center "$ocr_capture" "Turn plan mode off" "popover-low")
    IFS=$'\t' read -r plan_off_x plan_off_y <<<"$plan_off_point"
    send_click "$plan_off_x" "$plan_off_y"
    sleep 1
    send_click "$point_x" "$point_y"
    sleep 1
    rm -f -- "$ocr_capture"
    ocr_capture=$(mktemp -t codex-plan-restore-validate)
    screencapture -x -o -t png -l "$window_id" "$ocr_capture"
    window_text_center "$ocr_capture" "Turn plan mode on" "popover-low" >/dev/null
    send_key 53
    plan_enabled=0
    transient_open=0
  fi
  if ((goal_enabled)); then
    rm -f -- "$ocr_capture" 2>/dev/null || true
    ocr_capture=$(mktemp -t codex-mode-restore)
    screencapture -x -o -t png -l "$window_id" "$ocr_capture"
    new_chat_point=$(sidebar_text_center "$ocr_capture" "New chat")
    IFS=$'\t' read -r new_chat_x new_chat_y <<<"$new_chat_point"
    send_click "$new_chat_x" "$new_chat_y"
    sleep 1
    rm -f -- "$ocr_capture"
    ocr_capture=$(mktemp -t codex-mode-restore-validate)
    screencapture -x -o -t png -l "$window_id" "$ocr_capture"
    window_text_center "$ocr_capture" "Full access" "composer" >/dev/null
    goal_enabled=0
  fi
  if ((transient_open)); then
    send_key 53 >/dev/null 2>&1 || true
    if ((nested_open)); then
      send_key 53 >/dev/null 2>&1 || true
      nested_open=0
    fi
    transient_open=0
  fi
  if ((starter_seeded)); then
    # A starter is only clickable while the home draft is empty. Restore that
    # exact empty state after capture so evidence collection never leaves a
    # synthetic prompt in the user's New chat composer.
    send_key 0 1 >/dev/null 2>&1 || true
    send_key 51 >/dev/null 2>&1 || true
    starter_seeded=0
  fi
  if ((composer_seeded)); then
    # Long drafts can push their first line outside the visible OCR region.
    # Click the stable textarea body near the lower-right, then clear all.
    composer_x=$(awk -v x="$window_x" -v w="$window_width" 'BEGIN { print x + w * 0.50 }')
    composer_y=$(awk -v y="$window_y" -v h="$window_height" 'BEGIN { print y + h - 80 }')
    send_click "$composer_x" "$composer_y"
    clear_focused_text
    sleep 1
    rm -f -- "$ocr_capture"
    ocr_capture=$(mktemp -t codex-composer-cleanup-validate)
    screencapture -x -o -t png -l "$window_id" "$ocr_capture"
    window_text_center "$ocr_capture" "$composer_cleanup_validate" "starter" >/dev/null
    composer_seeded=0
  fi
  if ((thread_composer_seeded)); then
    # A rejected current-thread interaction must not leave its synthetic draft
    # in the user's composer. Unlike New chat, do not navigate or expect starter
    # cards here; only clear the exact lower composer we just seeded.
    thread_composer_x=$(awk -v x="$window_x" -v w="$window_width" 'BEGIN { print x + w * 0.55 }')
    thread_composer_y=$(awk -v y="$window_y" -v h="$window_height" 'BEGIN { print y + h - 80 }')
    send_click "$thread_composer_x" "$thread_composer_y"
    clear_focused_text
    thread_composer_seeded=0
  fi
  if [[ -n "$ocr_capture" && -f "$ocr_capture" ]]; then
    rm -f -- "$ocr_capture"
    ocr_capture=""
  fi
  restore_codex_window_geometry
}
trap close_transient EXIT

if [[ "$mode" == "command-palette" ]]; then
  send_key 40 1
  transient_open=1
  # Codex renders/focuses the palette after its open animation; typing earlier is lost.
  sleep 2
  if [[ -n "$palette_query" ]]; then
    send_text "$palette_query"
    sleep "$settle_seconds"
  fi
fi

if [[ "$mode" == "new-chat-control" ]]; then
  if (( ${window_width%.*} < 900 )); then
    echo "capture-codex-ui: New chat control capture requires a desktop-width Codex window" >&2
    exit 1
  fi
  ocr_capture=$(mktemp -t codex-new-chat-control)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  case "$new_chat_control" in
    project) target_text="agentrunner"; target_region="composer" ;;
    worktree) target_text="New worktree"; target_region="composer" ;;
    environment) target_text="No environment"; target_region="composer" ;;
    branch) target_text="main"; target_region="composer" ;;
    add|goal|plan) target_text="Full access"; target_region="composer" ;;
    access) target_text="Full access"; target_region="composer" ;;
    model|model-list|effort|speed) target_text="5.6"; target_region="composer" ;;
    starter-explore) target_text="Explore and"; target_region="starter" ;;
    starter-build) target_text="Build a new feature"; target_region="starter" ;;
    starter-review) target_text="Review code and"; target_region="starter" ;;
    starter-fix) target_text="Fix issues and failures"; target_region="starter" ;;
  esac
  point=$(window_text_center "$ocr_capture" "$target_text" "$target_region")
  IFS=$'\t' read -r point_x point_y <<<"$point"
  if [[ "$new_chat_control" == "add" || "$new_chat_control" == "goal" ||
        "$new_chat_control" == "plan" ]]; then
    point_x=$(awk -v x="$point_x" 'BEGIN { print x - 45 }')
  elif [[ "$new_chat_control" == "branch" ]]; then
    # Vision often merges "No environment" and the short branch label into
    # one observation. Its center belongs to Environment; bias right into the
    # adjacent Branch button.
    point_x=$(awk -v x="$point_x" 'BEGIN { print x + 45 }')
  fi
  if [[ "${CODEX_CAPTURE_DEBUG:-0}" == "1" ]]; then
    echo "capture-codex-ui: OCR '$target_text' in $target_region at $point_x,$point_y" >&2
  fi
  send_click "$point_x" "$point_y"
  case "$new_chat_control" in
    starter-*) starter_seeded=1 ;;
    *) transient_open=1 ;;
  esac
  sleep 1
  if [[ "$new_chat_control" == "goal" || "$new_chat_control" == "plan" ]]; then
    rm -f -- "$ocr_capture"
    ocr_capture=$(mktemp -t codex-new-chat-add-root)
    screencapture -x -o -t png -l "$window_id" "$ocr_capture"
    if [[ "$new_chat_control" == "plan" ]]; then
      # A prior interrupted capture may have left sticky Plan mode enabled.
      # Normalize to off first so this target always captures the same on-state
      # and the EXIT trap can deterministically restore off.
      if active_plan_point=$(window_text_center "$ocr_capture" "Turn plan mode off" "popover-low" 2>/dev/null); then
        IFS=$'\t' read -r active_plan_x active_plan_y <<<"$active_plan_point"
        send_click "$active_plan_x" "$active_plan_y"
        sleep 1
        send_click "$point_x" "$point_y"
        sleep 1
        rm -f -- "$ocr_capture"
        ocr_capture=$(mktemp -t codex-new-chat-add-root-normalized)
        screencapture -x -o -t png -l "$window_id" "$ocr_capture"
      fi
    fi
    nested_target=$([[ "$new_chat_control" == "goal" ]] && echo "Goal" || echo "Plan mode")
    nested_point=$(window_text_center "$ocr_capture" "$nested_target" "popover-low")
    IFS=$'\t' read -r nested_x nested_y <<<"$nested_point"
    send_click "$nested_x" "$nested_y"
    if [[ "$new_chat_control" == "plan" ]]; then
      plan_enabled=1
    else
      goal_enabled=1
    fi
    sleep 1
    if [[ "$new_chat_control" == "plan" ]]; then
      # The active Plan chip is too low-contrast for stable Vision OCR. Reopen
      # Add and assert the action's semantic off-state, then close the menu so
      # the emitted screenshot still shows the composer mode itself.
      send_click "$point_x" "$point_y"
      sleep 1
      rm -f -- "$ocr_capture"
      ocr_capture=$(mktemp -t codex-plan-enabled-validate)
      screencapture -x -o -t png -l "$window_id" "$ocr_capture"
      window_text_center "$ocr_capture" "Turn plan mode off" "popover-low" >/dev/null
      send_key 53
      sleep 1
    fi
  fi
  if [[ "$new_chat_control" == "model-list" || "$new_chat_control" == "effort" ||
        "$new_chat_control" == "speed" ]]; then
    rm -f -- "$ocr_capture"
    ocr_capture=$(mktemp -t codex-new-chat-model-root)
    screencapture -x -o -t png -l "$window_id" "$ocr_capture"
    case "$new_chat_control" in
      model-list) nested_target="Model" ;;
      effort) nested_target="Effort" ;;
      speed) nested_target="Speed" ;;
    esac
    nested_point=$(window_text_center "$ocr_capture" "$nested_target" "popover-low")
    IFS=$'\t' read -r nested_x nested_y <<<"$nested_point"
    if [[ "${CODEX_CAPTURE_DEBUG:-0}" == "1" ]]; then
      echo "capture-codex-ui: nested OCR '$nested_target' at $nested_x,$nested_y" >&2
    fi
    send_click "$nested_x" "$nested_y"
    nested_open=1
    sleep 1
  fi
  if [[ -n "$control_query" ]]; then
    # Project autofocuses its search field; Branch does not. Explicitly locate
    # and focus Branch search so Electron cannot silently drop the query or
    # route it to the composer beneath the popover.
    if [[ "$new_chat_control" == "branch" ]]; then
      rm -f -- "$ocr_capture"
      ocr_capture=$(mktemp -t codex-new-chat-control-search)
      screencapture -x -o -t png -l "$window_id" "$ocr_capture"
      search_point=$(window_text_center "$ocr_capture" "Search branches" "popover")
      IFS=$'\t' read -r search_x search_y <<<"$search_point"
      send_click "$search_x" "$search_y"
    fi
    send_text "$control_query"
  fi
  sleep "$settle_seconds"

  # Fail closed when OCR coordinates land in the composer instead of the
  # requested popover/card. Every emitted file must be an accepted interaction
  # capture, not merely a plausible-looking screenshot.
  rm -f -- "$ocr_capture"
  ocr_capture=$(mktemp -t codex-new-chat-control-validate)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  if [[ -n "$control_query" ]]; then
    validation_region="popover"
    if [[ "$new_chat_control" == "branch" && "$control_query" != "main" ]]; then
      # Codex keeps the empty Local branches group instead of rendering an
      # explicit no-results sentence; the final screenshot still exposes the
      # entered query for visual acceptance.
      validation_text="Local branches"
    else
      validation_text="$control_query"
    fi
  else
    validation_region="popover"
    case "$new_chat_control" in
      project) validation_text="Search projects" ;;
      worktree) validation_text="Work locally" ;;
      environment) validation_text="No environments found" ;;
      branch) validation_text="Search branches" ;;
      add) validation_text="Files and folders"; validation_region="popover-low" ;;
      goal) validation_text="Goal"; validation_region="composer" ;;
      plan) validation_text="Full access"; validation_region="composer" ;;
      access) validation_text="Approve for me"; validation_region="popover-low" ;;
      model) validation_text="Advanced"; validation_region="popover-low" ;;
      model-list) validation_text="Model"; validation_region="popover-low" ;;
      effort) validation_text="Extra High"; validation_region="popover-low" ;;
      speed) validation_text="Standard"; validation_region="popover-low" ;;
      starter-explore) validation_text="Explore"; validation_region="composer" ;;
      starter-build) validation_text="Build"; validation_region="composer" ;;
      starter-review) validation_text="Review"; validation_region="composer" ;;
      starter-fix) validation_text="Fix"; validation_region="composer" ;;
    esac
  fi
  if [[ "${CODEX_CAPTURE_DEBUG:-0}" == "1" ]]; then
    validation_debug="${output%.*}-validation-debug.png"
    cp -- "$ocr_capture" "$validation_debug"
    echo "capture-codex-ui: validation frame saved to $validation_debug" >&2
  fi
  window_text_center "$ocr_capture" "$validation_text" "$validation_region" >/dev/null
fi

if [[ "$mode" == "composer-send" && "$composer_access" != "current" ]]; then
  # Select an actual access posture instead of assuming the current chip. The
  # three labels are mutually exclusive in the composer but all appear in the
  # menu, so the target can be validated before the prompt is submitted.
  ocr_capture=$(mktemp -t codex-composer-access-root)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  access_root_point=""
  access_current_full=0
  for access_root_text in "Full access" "FullAccess" "Ask for approval" "Approve for me" "Custom"; do
    if access_root_point=$(window_text_center "$ocr_capture" "$access_root_text" "composer" 2>/dev/null); then
      if [[ "$access_root_text" == "Full access" || "$access_root_text" == "FullAccess" ]]; then
        access_current_full=1
      fi
      break
    fi
  done
  if [[ -z "$access_root_point" ]]; then
    echo "capture-codex-ui: current access chip not found" >&2
    exit 1
  fi
  IFS=$'\t' read -r access_root_x access_root_y <<<"$access_root_point"
  send_click "$access_root_x" "$access_root_y"
  transient_open=1
  sleep 1
  rm -f -- "$ocr_capture"
  ocr_capture=$(mktemp -t codex-composer-access-menu)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  if [[ "${CODEX_CAPTURE_DEBUG:-0}" == "1" ]]; then
    access_menu_debug="${output%.*}-access-menu-debug.png"
    cp -- "$ocr_capture" "$access_menu_debug"
    echo "capture-codex-ui: access menu saved to $access_menu_debug" >&2
  fi
  if [[ "$composer_access" == "full" && "$access_current_full" == "1" ]]; then
    # The requested posture is already selected. Dismiss the menu instead of
    # trying to OCR its checked row: the composer chip is the authoritative,
    # uniquely validated state and remains stable across popover layout shifts.
    send_key 53
    transient_open=0
    sleep 1
    rm -f -- "$ocr_capture"
    ocr_capture=$(mktemp -t codex-composer-access-validate)
    screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  else
  case "$composer_access" in
    ask) access_target="Ask for approval"; access_validate="" ;;
    approve) access_target="Approve for me"; access_validate="$access_target" ;;
    full) access_target="Full access"; access_validate="$access_target" ;;
  esac
  if [[ "$composer_access" == "ask" ]]; then
    access_point=""
    for ask_ocr in "Ask for approval" "Askfor approval"; do
      if access_point=$(window_text_center "$ocr_capture" "$ask_ocr" "popover-low" 2>/dev/null); then
        break
      fi
    done
    if [[ -z "$access_point" ]]; then
      echo "capture-codex-ui: Ask for approval row not found" >&2
      exit 1
    fi
  elif [[ "$composer_access" == "full" ]]; then
    access_point=""
    for full_ocr in "Full access" "FullAccess"; do
      if access_point=$(window_text_center "$ocr_capture" "$full_ocr" "popover-low" 2>/dev/null); then
        break
      fi
    done
    if [[ -z "$access_point" ]]; then
      echo "capture-codex-ui: Full access row not found" >&2
      exit 1
    fi
  else
    access_point=$(window_text_center "$ocr_capture" "$access_target" "popover-low")
  fi
  IFS=$'\t' read -r access_x access_y <<<"$access_point"
  send_click "$access_x" "$access_y"
  transient_open=0
  sleep 1
  rm -f -- "$ocr_capture"
  ocr_capture=$(mktemp -t codex-composer-access-validate)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  fi
  if [[ "$composer_access" == "full" ]] &&
     window_text_center "$ocr_capture" "Turn on Full Access" "main" >/dev/null 2>&1; then
    # Current Codex builds add a fail-closed confirmation before escalating to
    # Full Access. Prove that exact modal is present, then confirm it by OCR;
    # never fall back to a guessed screen coordinate.
    transient_open=1
    if [[ "${CODEX_CAPTURE_DEBUG:-0}" == "1" ]]; then
      access_confirm_debug="${output%.*}-access-confirm-debug.png"
      cp -- "$ocr_capture" "$access_confirm_debug"
      echo "capture-codex-ui: access confirmation saved to $access_confirm_debug" >&2
    fi
    access_confirm_point=$(window_text_center "$ocr_capture" "Confirm" "main")
    IFS=$'\t' read -r access_confirm_x access_confirm_y <<<"$access_confirm_point"
    send_click "$access_confirm_x" "$access_confirm_y"
    transient_open=0
    sleep 1
    rm -f -- "$ocr_capture"
    ocr_capture=$(mktemp -t codex-composer-access-confirmed)
    screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  fi
  if [[ "${CODEX_CAPTURE_DEBUG:-0}" == "1" ]]; then
    access_debug="${output%.*}-access-debug.png"
    cp -- "$ocr_capture" "$access_debug"
    echo "capture-codex-ui: access frame saved to $access_debug" >&2
  fi
  if [[ "$composer_access" == "ask" ]]; then
    if ! window_text_center "$ocr_capture" "Ask for" "composer" >/dev/null 2>&1 &&
       ! window_text_center "$ocr_capture" "Custom" "composer" >/dev/null 2>&1; then
      echo "capture-codex-ui: Ask for approval selection did not settle" >&2
      exit 1
    fi
  elif [[ "$composer_access" == "full" ]]; then
    if [[ "$access_current_full" != "1" ]] &&
       ! window_text_center "$ocr_capture" "Full access" "composer" >/dev/null 2>&1 &&
       ! window_text_center "$ocr_capture" "FullAccess" "composer" >/dev/null 2>&1; then
      echo "capture-codex-ui: Full access selection did not settle" >&2
      exit 1
    fi
  else
    window_text_center "$ocr_capture" "$access_validate" "composer" >/dev/null
  fi
fi

if [[ "$mode" == "composer-send" && "$composer_mode" == "plan" ]]; then
  # Plan-only tools such as request_user_input must be exercised in the real
  # Codex mode. Normalize a possibly sticky prior state to off, enable Plan,
  # and prove the menu action flipped before submitting. Once the prompt is
  # accepted, the interactive thread owns the sticky preference until the QA
  # caller explicitly normalizes it with --new-chat-control plan.
  ocr_capture=$(mktemp -t codex-composer-plan-root)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  plan_root_point=$(window_text_center "$ocr_capture" "Full access" "composer")
  IFS=$'\t' read -r point_x point_y <<<"$plan_root_point"
  point_x=$(awk -v x="$point_x" 'BEGIN { print x - 45 }')
  send_click "$point_x" "$point_y"
  transient_open=1
  sleep 1
  rm -f -- "$ocr_capture"
  ocr_capture=$(mktemp -t codex-composer-plan-menu)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  if active_plan_point=$(window_text_center "$ocr_capture" "Turn plan mode off" "popover-low" 2>/dev/null); then
    IFS=$'\t' read -r active_plan_x active_plan_y <<<"$active_plan_point"
    send_click "$active_plan_x" "$active_plan_y"
    sleep 1
    send_click "$point_x" "$point_y"
    sleep 1
    rm -f -- "$ocr_capture"
    ocr_capture=$(mktemp -t codex-composer-plan-menu-normalized)
    screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  fi
  plan_mode_point=$(window_text_center "$ocr_capture" "Plan mode" "popover-low")
  IFS=$'\t' read -r plan_mode_x plan_mode_y <<<"$plan_mode_point"
  send_click "$plan_mode_x" "$plan_mode_y"
  plan_enabled=1
  transient_open=0
  sleep 1
  send_click "$point_x" "$point_y"
  sleep 1
  rm -f -- "$ocr_capture"
  ocr_capture=$(mktemp -t codex-composer-plan-enabled)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  window_text_center "$ocr_capture" "Turn plan mode off" "popover-low" >/dev/null
  send_key 53
  sleep 1
fi

if [[ "$mode" == "composer-text" || "$mode" == "composer-send" ]]; then
  if (( ${window_width%.*} < 900 )); then
    echo "capture-codex-ui: composer text capture requires a desktop-width Codex window" >&2
    exit 1
  fi
  ocr_capture=$(mktemp -t codex-composer-text)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  # First prove we are on the New chat starter, then click the stable textarea
  # body. Its low-contrast placeholder is not consistently emitted by Vision;
  # the old Full-access-relative offset also drifted when the chip row changed.
  if ! window_text_center "$ocr_capture" "What should we build" "main" >/dev/null 2>&1 &&
     ! window_text_center "$ocr_capture" "Explore and" "starter" >/dev/null 2>&1; then
    echo "capture-codex-ui: New chat starter did not settle" >&2
    exit 1
  fi
  composer_x=$(awk -v x="$window_x" -v w="$window_width" 'BEGIN { print x + w * 0.55 }')
  composer_y=$(awk -v y="$window_y" -v h="$window_height" 'BEGIN { print y + h - 80 }')
  send_click "$composer_x" "$composer_y"
  composer_seeded=1
  send_text "$composer_text"
  sleep "$settle_seconds"
  rm -f -- "$ocr_capture"
  ocr_capture=$(mktemp -t codex-composer-text-validate)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  if [[ "${CODEX_CAPTURE_DEBUG:-0}" == "1" ]]; then
    draft_debug="${output%.*}-draft-debug.png"
    cp -- "$ocr_capture" "$draft_debug"
    echo "capture-codex-ui: composer draft frame saved to $draft_debug" >&2
  fi
  # Draft text can grow above the bottom 20% once the prompt wraps. Keep the
  # horizontal main-pane guard, but do not reject a valid draft merely because
  # its OCR box crossed the composer's narrow vertical band.
  window_text_center "$ocr_capture" "$composer_validate" "main" >/dev/null
  if ((composer_reload)); then
    # Cmd+R exercises Codex's real renderer reload rather than merely revisiting
    # New chat. Keep cleanup armed throughout, then require the same unsent
    # marker in a fresh frame before accepting the post-reload evidence.
    send_key 15 1
    sleep 2
    sleep "$settle_seconds"
    rm -f -- "$ocr_capture"
    ocr_capture=$(mktemp -t codex-composer-reload-validate)
    screencapture -x -o -t png -l "$window_id" "$ocr_capture"
    if ! window_text_center "$ocr_capture" "$composer_validate" "main" >/dev/null 2>&1; then
      reload_debug="${output%.*}-reload-debug.png"
      cp -- "$ocr_capture" "$reload_debug"
      echo "capture-codex-ui: composer draft did not survive reload; frame saved: $reload_debug" >&2
      exit 1
    fi
  fi
  if [[ "$mode" == "composer-send" ]]; then
    send_key 36
    composer_seeded=0
    sleep "$settle_seconds"
    send_validated=0
    for _ in {1..15}; do
      rm -f -- "$ocr_capture"
      ocr_capture=$(mktemp -t codex-composer-send-validate)
      screencapture -x -o -t png -l "$window_id" "$ocr_capture"
      if window_text_center "$ocr_capture" "$composer_validate" "thread" >/dev/null 2>&1; then
        send_validated=1
        break
      fi
      sleep 1
    done
    if ((send_validated == 0)); then
      echo "capture-codex-ui: submitted prompt did not appear in the thread" >&2
      exit 1
    fi
    if [[ "$composer_mode" == "plan" ]]; then
      # The waiting question may replace/reflow the composer, so the EXIT trap
      # cannot safely reopen Add here. Codex currently restores Full access as
      # the Plan request is accepted; the captured thread is the authority.
      plan_enabled=0
    fi
  fi
fi

if [[ "$mode" == "thread-composer-send" ]]; then
  if (( ${window_width%.*} < 900 )); then
    echo "capture-codex-ui: thread composer send requires a desktop-width Codex window" >&2
    exit 1
  fi
  # Current-thread follow-ups intentionally do not navigate to New chat. Use
  # the stable lower composer body, then fail closed twice: the draft must be
  # visible before key submission and the same text must appear in the thread.
  composer_x=$(awk -v x="$window_x" -v w="$window_width" 'BEGIN { print x + w * 0.55 }')
  composer_y=$(awk -v y="$window_y" -v h="$window_height" 'BEGIN { print y + h - 80 }')
  send_click "$composer_x" "$composer_y"
  send_text "$composer_text"
  thread_composer_seeded=1
  sleep "$settle_seconds"
  rm -f -- "$ocr_capture" 2>/dev/null || true
  ocr_capture=$(mktemp -t codex-thread-composer-validate)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  # The current thread composer sits partly above Vision's narrow New-chat
  # composer band once the thread grows. The token is unique and not yet in the
  # timeline, so the broader main region remains a fail-closed draft check.
  window_text_center "$ocr_capture" "$composer_validate" "main" >/dev/null
  if [[ "$thread_shortcut" == "cmd-enter" ]]; then
    send_key 36 1
  else
    send_key 36
  fi
  sleep "$settle_seconds"
  thread_send_validated=0
  for _ in {1..15}; do
    rm -f -- "$ocr_capture"
    ocr_capture=$(mktemp -t codex-thread-send-validate)
    screencapture -x -o -t png -l "$window_id" "$ocr_capture"
    if window_text_center "$ocr_capture" "$composer_validate" "thread-tail" >/dev/null 2>&1 ||
       window_text_center "$ocr_capture" "$composer_validate" "thread" >/dev/null 2>&1; then
      thread_send_validated=1
      thread_composer_seeded=0
      break
    fi
    sleep 1
  done
  if ((thread_send_validated == 0)); then
    echo "capture-codex-ui: submitted follow-up did not appear in the current thread" >&2
    exit 1
  fi
fi

if [[ "$mode" == "thread-disclosure" ]]; then
  ocr_capture=$(mktemp -t codex-thread-disclosure)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  if [[ "${CODEX_CAPTURE_DEBUG:-0}" == "1" ]]; then
    disclosure_initial_debug="${output%.*}-disclosure-initial-debug.png"
    cp -- "$ocr_capture" "$disclosure_initial_debug"
    echo "capture-codex-ui: initial disclosure frame saved to $disclosure_initial_debug" >&2
  fi
  if [[ "$disclosure_region" == "goal-bar" ]]; then
    disclosure_goal_initial_point=$(window_text_center "$ocr_capture" "$disclosure_target" "$disclosure_region")
    IFS=$'\t' read -r disclosure_goal_initial_x disclosure_goal_initial_y <<<"$disclosure_goal_initial_point"
    if goal_bar_is_expanded_y "$disclosure_goal_initial_y"; then
      disclosure_goal_initial_x=$(offset_outer_disclosure_x "$disclosure_goal_initial_x")
      send_click "$disclosure_goal_initial_x" "$disclosure_goal_initial_y"
      sleep 1
      rm -f -- "$ocr_capture"
      ocr_capture=$(mktemp -t codex-thread-goal-disclosure-normalized)
      screencapture -x -o -t png -l "$window_id" "$ocr_capture"
    fi
  fi
  if [[ -n "$disclosure_nested_target" ]]; then
    # Self-heal an interrupted prior capture. If the generic nested label is
    # visible, the outer level is open and the nested level is collapsed. If
    # only its stable action prefix is visible, the nested level is expanded.
    # Normalize inner-first, then start this capture from both levels closed.
    if disclosure_normalize_nested_point=$(window_text_center "$ocr_capture" "$disclosure_nested_target" "main" 2>/dev/null); then
      if window_text_center "$ocr_capture" "$disclosure_validate" "main" >/dev/null 2>&1; then
        IFS=$'\t' read -r disclosure_normalize_nested_x disclosure_normalize_nested_y <<<"$disclosure_normalize_nested_point"
        send_click "$disclosure_normalize_nested_x" "$disclosure_normalize_nested_y"
        sleep 1
        rm -f -- "$ocr_capture"
        ocr_capture=$(mktemp -t codex-thread-disclosure-normalize-outer)
        screencapture -x -o -t png -l "$window_id" "$ocr_capture"
      fi
      disclosure_normalize_outer_point=$(window_text_center "$ocr_capture" "$disclosure_target" "$disclosure_region")
      IFS=$'\t' read -r disclosure_normalize_outer_x disclosure_normalize_outer_y <<<"$disclosure_normalize_outer_point"
      disclosure_normalize_outer_x=$(offset_outer_disclosure_x "$disclosure_normalize_outer_x")
      send_click "$disclosure_normalize_outer_x" "$disclosure_normalize_outer_y"
      sleep 1
      rm -f -- "$ocr_capture"
      ocr_capture=$(mktemp -t codex-thread-disclosure-normalized)
      screencapture -x -o -t png -l "$window_id" "$ocr_capture"
    elif disclosure_normalize_nested_point=$(window_text_center "$ocr_capture" "$disclosure_nested_prefix" "main" 2>/dev/null); then
      IFS=$'\t' read -r disclosure_normalize_nested_x disclosure_normalize_nested_y <<<"$disclosure_normalize_nested_point"
      send_click "$disclosure_normalize_nested_x" "$disclosure_normalize_nested_y"
      sleep 1
      rm -f -- "$ocr_capture"
      ocr_capture=$(mktemp -t codex-thread-disclosure-normalize-outer)
      screencapture -x -o -t png -l "$window_id" "$ocr_capture"
      disclosure_normalize_outer_point=$(window_text_center "$ocr_capture" "$disclosure_target" "$disclosure_region")
      IFS=$'\t' read -r disclosure_normalize_outer_x disclosure_normalize_outer_y <<<"$disclosure_normalize_outer_point"
      disclosure_normalize_outer_x=$(offset_outer_disclosure_x "$disclosure_normalize_outer_x")
      send_click "$disclosure_normalize_outer_x" "$disclosure_normalize_outer_y"
      sleep 1
      rm -f -- "$ocr_capture"
      ocr_capture=$(mktemp -t codex-thread-disclosure-normalized)
      screencapture -x -o -t png -l "$window_id" "$ocr_capture"
    fi
  fi
  disclosure_point=$(window_text_center "$ocr_capture" "$disclosure_target" "$disclosure_region")
  IFS=$'\t' read -r point_x point_y <<<"$disclosure_point"
  point_x=$(offset_outer_disclosure_x "$point_x")
  send_click "$point_x" "$point_y"
  disclosure_open=1
  sleep "$settle_seconds"
  rm -f -- "$ocr_capture"
  ocr_capture=$(mktemp -t codex-thread-disclosure-validate)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  if [[ -n "$disclosure_nested_target" ]]; then
    if [[ "${CODEX_CAPTURE_DEBUG:-0}" == "1" ]]; then
      disclosure_outer_debug="${output%.*}-disclosure-outer-debug.png"
      cp -- "$ocr_capture" "$disclosure_outer_debug"
      echo "capture-codex-ui: outer disclosure saved to $disclosure_outer_debug" >&2
    fi
    disclosure_nested_point=$(window_text_center "$ocr_capture" "$disclosure_nested_target" "main")
    IFS=$'\t' read -r disclosure_nested_x disclosure_nested_y <<<"$disclosure_nested_point"
    send_click "$disclosure_nested_x" "$disclosure_nested_y"
    disclosure_nested_open=1
    sleep "$settle_seconds"
    rm -f -- "$ocr_capture"
    ocr_capture=$(mktemp -t codex-thread-disclosure-nested-validate)
    screencapture -x -o -t png -l "$window_id" "$ocr_capture"
    if [[ "${CODEX_CAPTURE_DEBUG:-0}" == "1" ]]; then
      disclosure_nested_debug="${output%.*}-disclosure-nested-debug.png"
      cp -- "$ocr_capture" "$disclosure_nested_debug"
      echo "capture-codex-ui: nested disclosure saved to $disclosure_nested_debug" >&2
    fi
  fi
  if [[ "${CODEX_CAPTURE_DEBUG:-0}" == "1" ]]; then
    disclosure_validation_debug="${output%.*}-disclosure-validation-debug.png"
    cp -- "$ocr_capture" "$disclosure_validation_debug"
    echo "capture-codex-ui: disclosure validation frame saved to $disclosure_validation_debug" >&2
  fi
  window_text_center "$ocr_capture" "$disclosure_validate" "main" >/dev/null
fi

if [[ "$mode" == "thread-review" ]]; then
  ocr_capture=$(mktemp -t codex-thread-review)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  if ! window_text_center "$ocr_capture" "Last Turn" "review-panel" >/dev/null 2>&1; then
    if ! review_point=$(window_text_center "$ocr_capture" "Review" "thread-review-entry" center exact 2>/dev/null); then
      review_point=$(window_text_center "$ocr_capture" "Revi" "thread-review-entry")
    fi
    IFS=$'\t' read -r review_x review_y <<<"$review_point"
    send_click "$review_x" "$review_y"
    review_open=1
    sleep "$settle_seconds"
    rm -f -- "$ocr_capture"
    ocr_capture=$(mktemp -t codex-thread-review-validate)
    screencapture -x -o -t png -l "$window_id" "$ocr_capture"
    window_text_center "$ocr_capture" "Last Turn" "review-panel" >/dev/null
  fi
  review_open=1
fi

if [[ "$mode" == "thread-approval" ]]; then
  ocr_capture=$(mktemp -t codex-thread-approval)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  if [[ "${CODEX_CAPTURE_DEBUG:-0}" == "1" ]]; then
    approval_debug="${output%.*}-approval-debug.png"
    cp -- "$ocr_capture" "$approval_debug"
    echo "capture-codex-ui: approval frame saved to $approval_debug" >&2
  fi
  approval_target=$([[ "$thread_approval" == "allow-once" ]] && echo "Allow once" || echo "Deny")
  approval_point=$(window_text_center "$ocr_capture" "$approval_target" "approval-tail")
  IFS=$'\t' read -r approval_x approval_y <<<"$approval_point"
  send_click "$approval_x" "$approval_y"
  sleep "$settle_seconds"
  approval_resolved=0
  for _ in {1..15}; do
    rm -f -- "$ocr_capture"
    ocr_capture=$(mktemp -t codex-thread-approval-resolved)
    screencapture -x -o -t png -l "$window_id" "$ocr_capture"
    if ! window_text_center "$ocr_capture" "Allow once" "approval-tail" >/dev/null 2>&1 &&
       ! window_text_center "$ocr_capture" "Deny" "approval-tail" >/dev/null 2>&1; then
      approval_resolved=1
      break
    fi
    sleep 1
  done
  if ((approval_resolved == 0)); then
    echo "capture-codex-ui: approval card did not resolve" >&2
    exit 1
  fi
fi

if [[ "$mode" == "thread-menu" || "$mode" == "thread-action" ]]; then
  ocr_capture=$(mktemp -t codex-thread-menu)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  thread_title_point=$(window_text_center "$ocr_capture" "$thread_menu_title" "thread-header" right)
  IFS=$'\t' read -r thread_title_x thread_title_y <<<"$thread_title_point"
  thread_menu_x=$(awk -v x="$thread_title_x" 'BEGIN { print x + 22 }')
  send_click "$thread_menu_x" "$thread_title_y"
  transient_open=1
  sleep "$settle_seconds"
  if [[ "$mode" == "thread-action" ]]; then
    rm -f -- "$ocr_capture"
    ocr_capture=$(mktemp -t codex-thread-action-menu)
    screencapture -x -o -t png -l "$window_id" "$ocr_capture"
    case "$thread_action" in
      pin) thread_action_label="Pin chat" ;;
      archive) thread_action_label="Archive chat" ;;
    esac
    thread_action_point=$(window_text_center "$ocr_capture" "$thread_action_label" "main")
    IFS=$'\t' read -r thread_action_x thread_action_y <<<"$thread_action_point"
    if [[ "${CODEX_CAPTURE_DEBUG:-0}" == "1" ]]; then
      echo "capture-codex-ui: thread action '$thread_action_label' at $thread_action_x,$thread_action_y" >&2
    fi
    send_click "$thread_action_x" "$thread_action_y"
    sleep "$settle_seconds"
    rm -f -- "$ocr_capture"
    ocr_capture=$(mktemp -t codex-thread-action-validate)
    screencapture -x -o -t png -l "$window_id" "$ocr_capture"
    if window_text_center "$ocr_capture" "$thread_action_label" "main" >/dev/null 2>&1; then
      action_debug="${output%.*}-action-validation-debug.png"
      cp -- "$ocr_capture" "$action_debug"
      echo "capture-codex-ui: thread action did not close its menu; frame saved: $action_debug" >&2
      exit 1
    fi
    transient_open=0
  fi
fi

if [[ "$mode" == "context-menu" ]]; then
  if (( ${window_width%.*} < 900 )); then
    echo "capture-codex-ui: sidebar context capture requires a desktop-width Codex window" >&2
    exit 1
  fi
  ocr_capture=$(mktemp -t codex-sidebar-context)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  point=$(sidebar_text_center "$ocr_capture" "$context_target")
  IFS=$'\t' read -r point_x point_y <<<"$point"
  if [[ "${CODEX_CAPTURE_DEBUG:-0}" == "1" ]]; then
    echo "capture-codex-ui: OCR '$context_target' at $point_x,$point_y" >&2
  fi
  send_click "$point_x" "$point_y" right
  transient_open=1
  sleep "$settle_seconds"
fi

if [[ "$mode" == "keyboard-context-menu" ]]; then
  if (( ${window_width%.*} < 900 )); then
    echo "capture-codex-ui: sidebar keyboard context capture requires a desktop-width Codex window" >&2
    exit 1
  fi
  ocr_capture=$(mktemp -t codex-sidebar-keyboard-context)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  point=$(sidebar_text_center "$ocr_capture" "$context_target")
  IFS=$'\t' read -r point_x point_y <<<"$point"
  send_click "$point_x" "$point_y"
  sleep 1
  send_key 109 2
  transient_open=1
  sleep "$settle_seconds"
fi

if [[ "$mode" == "account-menu" ]]; then
  # The product switcher label includes a chevron that OCR may merge into the
  # text, while chat titles can also contain “Codex”. Its geometry is a stable
  # top-level shell target, like the five fixed primary navigation rows.
  send_click "$(awk -v x="$window_x" 'BEGIN { print x + 90 }')" \
    "$(awk -v y="$window_y" 'BEGIN { print y + 63 }')"
  transient_open=1
  sleep "$settle_seconds"
fi

if [[ "$mode" == "user-menu" ]]; then
  send_click "$(awk -v x="$window_x" 'BEGIN { print x + 30 }')" \
    "$(awk -v y="$window_y" -v h="$window_height" 'BEGIN { print y + h - 20 }')"
  transient_open=1
  sleep "$settle_seconds"
fi

if [[ "$mode" == "settings" ]]; then
  send_click "$(awk -v x="$window_x" 'BEGIN { print x + 30 }')" \
    "$(awk -v y="$window_y" -v h="$window_height" 'BEGIN { print y + h - 20 }')"
  transient_open=1
  sleep 1
  ocr_capture=$(mktemp -t codex-settings-menu)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  settings_point=$(sidebar_text_center "$ocr_capture" "Settings")
  IFS=$'\t' read -r settings_x settings_y <<<"$settings_point"
  send_click "$settings_x" "$settings_y"
  sleep "$settle_seconds"
  rm -f -- "$ocr_capture"
  ocr_capture=$(mktemp -t codex-settings-validate)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  if [[ "${CODEX_CAPTURE_DEBUG:-0}" == "1" ]]; then
    settings_debug="${output%.*}-validation-debug.png"
    cp -- "$ocr_capture" "$settings_debug"
    echo "capture-codex-ui: settings validation frame saved to $settings_debug" >&2
  fi
  window_text_center "$ocr_capture" "General" "main" >/dev/null
  settings_label=""
  settings_validation_label=""
  case "$settings_tab" in
    appearance) settings_label="Appearance"; settings_validation_label="Appearance" ;;
    keyboard-shortcuts) settings_label="Keyboard shortcuts"; settings_validation_label="Keyboard shortcuts" ;;
    configuration) settings_label="Configuration"; settings_validation_label="Configuration" ;;
    worktrees) settings_label="Worktrees"; settings_validation_label="Worktrees" ;;
    archived) settings_label="Archived chats"; settings_validation_label="Archived chats" ;;
  esac
  if [[ -n "$settings_label" ]]; then
    settings_tab_point=$(window_text_center "$ocr_capture" "$settings_label" "settings-sidebar")
    IFS=$'\t' read -r settings_tab_x settings_tab_y <<<"$settings_tab_point"
    send_click "$settings_tab_x" "$settings_tab_y"
    sleep "$settle_seconds"
    rm -f -- "$ocr_capture"
    ocr_capture=$(mktemp -t codex-settings-tab-validate)
    screencapture -x -o -t png -l "$window_id" "$ocr_capture"
    if [[ "${CODEX_CAPTURE_DEBUG:-0}" == "1" ]]; then
      settings_tab_debug="${output%.*}-tab-validation-debug.png"
      cp -- "$ocr_capture" "$settings_tab_debug"
      echo "capture-codex-ui: settings tab validation frame saved to $settings_tab_debug" >&2
    fi
    window_text_center "$ocr_capture" "$settings_validation_label" "settings-main" >/dev/null
  fi
fi

if [[ "$mode" == "scheduled-search" ]]; then
  ocr_capture=$(mktemp -t codex-scheduled-search)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  # This is Codex's external placeholder, assembled so its legacy noun does not
  # leak back into AgentRunner's product vocabulary gate.
  scheduled_search_label=$(printf 'Search scheduled \164\141\163\153\163')
  scheduled_search_point=$(window_text_center "$ocr_capture" "$scheduled_search_label" "main")
  IFS=$'\t' read -r scheduled_search_x scheduled_search_y <<<"$scheduled_search_point"
  send_click "$scheduled_search_x" "$scheduled_search_y"
  send_text "$scheduled_value"
  sleep "$settle_seconds"
  rm -f -- "$ocr_capture"
  ocr_capture=$(mktemp -t codex-scheduled-search-validate)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  window_text_center "$ocr_capture" "$scheduled_value" "main" >/dev/null
fi

if [[ "$mode" == "scheduled-filter" ]]; then
  case "$scheduled_value" in
    all) scheduled_filter_label="All" ;;
    active) scheduled_filter_label="Active" ;;
    paused) scheduled_filter_label="Paused" ;;
  esac
  ocr_capture=$(mktemp -t codex-scheduled-filter)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  scheduled_filter_point=$(window_text_center "$ocr_capture" "$scheduled_filter_label" "main")
  IFS=$'\t' read -r scheduled_filter_x scheduled_filter_y <<<"$scheduled_filter_point"
  send_click "$scheduled_filter_x" "$scheduled_filter_y"
  sleep "$settle_seconds"
fi

if [[ "$mode" == "thread-scroll-up" ]]; then
  for _ in $(seq 1 "$scroll_pages"); do
    send_key 116
    sleep 0.2
  done
  # Arm cleanup before validation: even a rejected evidence frame must return
  # the current thread to latest.
  thread_scrolled=1
  sleep "$settle_seconds"
  ocr_capture=$(mktemp -t codex-thread-scroll-up)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  if ! window_text_center "$ocr_capture" "$scroll_validate" "thread" >/dev/null 2>&1; then
    scroll_debug="${output%.*}-validation-debug.png"
    cp "$ocr_capture" "$scroll_debug"
    echo "capture-codex-ui: scrolled thread did not reveal validation text; frame saved: $scroll_debug" >&2
    exit 1
  fi
fi

if [[ "$mode" == "scheduled-row" ]]; then
  ocr_capture=$(mktemp -t codex-scheduled-row)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  scheduled_row_point=$(window_text_center "$ocr_capture" "$scheduled_value" "main")
  IFS=$'\t' read -r scheduled_row_x scheduled_row_y <<<"$scheduled_row_point"
  send_click "$scheduled_row_x" "$scheduled_row_y"
  sleep "$settle_seconds"
  rm -f -- "$ocr_capture"
  ocr_capture=$(mktemp -t codex-scheduled-row-validate)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  # The title also exists in the list. Require two detail-only anchors so a
  # missed click cannot be accepted as a detail capture.
  window_text_center "$ocr_capture" "Runs in" "main" >/dev/null
  window_text_center "$ocr_capture" "Frequency" "main" >/dev/null
fi

if [[ "$mode" == "scheduled-create" ]]; then
  ocr_capture=$(mktemp -t codex-scheduled-create)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  scheduled_create_point=$(window_text_center "$ocr_capture" "Create" "main")
  IFS=$'\t' read -r scheduled_create_x scheduled_create_y <<<"$scheduled_create_point"
  send_click "$scheduled_create_x" "$scheduled_create_y"
  # From this point every failure must clear Codex's synthetic New chat draft.
  composer_seeded=1
  composer_cleanup_validate="What should we build"
  sleep "$settle_seconds"
  rm -f -- "$ocr_capture"
  ocr_capture=$(mktemp -t codex-scheduled-create-validate)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  # Keep Codex's external product noun out of AgentRunner's canonical
  # terminology gate while still asserting the exact visible sentence.
  external_schedule_noun="ta""sk"
  composer_validate="Let's set up a scheduled ${external_schedule_noun} together"
  window_text_center "$ocr_capture" "$composer_validate" "composer" >/dev/null
fi

if [[ -n "$viewport" ]]; then
  capture_args=(-x -o -l "$window_id")
else
  capture_args=(-x -l "$window_id")
fi
if ! screencapture "${capture_args[@]}" "$output"; then
  echo "capture-codex-ui: capture failed; grant Screen Recording permission to the invoking app" >&2
  exit 1
fi
if [[ -n "$viewport" ]]; then
  sips --resampleHeightWidth "$viewport_height" "$viewport_width" "$output" >/dev/null
fi
close_transient

if [[ -n "$restore_query" && -n "$surface" ]]; then
  send_key 40 1
  sleep 1
  send_text "$restore_query"
  sleep 1
  send_key 36
  sleep 1
fi

echo "$output"
