#!/usr/bin/env bash
# Capture the real Codex desktop window on macOS. Optional command-palette mode
# proves low-risk interaction without depending on the Electron AX tree.
set -euo pipefail

usage() {
  cat <<'EOF'
usage: qa/capture-codex-ui.sh [--command-palette [--palette-query TEXT] | --surface NAME |
                              --new-chat-control NAME [--control-query TEXT] |
                              --composer-text TEXT --composer-validate VISIBLE_TEXT |
                              --composer-send TEXT --composer-validate VISIBLE_TEXT
                                [--composer-mode default|plan] |
                              --thread-composer-send TEXT --composer-validate VISIBLE_TEXT
                                [--thread-shortcut enter|cmd-enter] |
                              --thread-disclosure VISIBLE_TEXT --disclosure-validate VISIBLE_TEXT |
                              --context-menu VISIBLE_TEXT | --keyboard-context-menu VISIBLE_TEXT |
                              --account-menu | --user-menu]
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
--composer-send fills and submits a New chat prompt, then retains the created thread.
--composer-mode plan enables sticky Plan mode before --composer-send. It intentionally
  hands that mode to the created interactive thread; Codex currently restores the
  composer to Full access after the Plan request is accepted.
--thread-composer-send submits a follow-up in the currently open thread without navigating away.
--thread-shortcut chooses Enter or Cmd+Enter for that follow-up (default: enter).
--thread-disclosure opens one visible disclosure in the current thread, validates its body,
  captures it, then restores the collapsed state.
--composer-validate is a short visible substring required in the draft/thread capture.
--context-menu OCR-locates visible sidebar text, right-clicks it, captures the menu,
  then dismisses it with Escape. Matches are restricted to the sidebar.
--keyboard-context-menu focuses visible sidebar text, opens its menu with Shift+F10,
  captures it, then dismisses it with Escape.
--account-menu opens the top-left Codex/ChatGPT product switcher.
--user-menu opens the bottom-left profile/status menu.
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
composer_mode="default"
thread_shortcut="enter"
context_target=""
disclosure_target=""
disclosure_validate=""
restore_query=""
settle_seconds=1
output=""
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
    --composer-validate)
      if (($# < 2)) || [[ -z "$2" ]]; then
        echo "capture-codex-ui: --composer-validate requires visible text" >&2
        exit 2
      fi
      composer_validate="$2"
      shift 2
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
    --restore-query)
      if (($# < 2)); then
        echo "capture-codex-ui: --restore-query requires text" >&2
        exit 2
      fi
      restore_query="$2"
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
if [[ -n "$composer_validate" && "$mode" != "composer-text" && "$mode" != "composer-send" &&
      "$mode" != "thread-composer-send" ]]; then
  echo "capture-codex-ui: --composer-validate requires --composer-text, --composer-send, or --thread-composer-send" >&2
  exit 2
fi
if [[ "$thread_shortcut" != "enter" && "$mode" != "thread-composer-send" ]]; then
  echo "capture-codex-ui: --thread-shortcut requires --thread-composer-send" >&2
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
if [[ "$composer_mode" != "default" && "$mode" != "composer-send" ]]; then
  echo "capture-codex-ui: --composer-mode plan requires --composer-send" >&2
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

window_info=$(swift - "$codex_pid" <<'SWIFT'
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
)
IFS=$'\t' read -r window_id window_x window_y window_width window_height <<<"$window_info"
if [[ ! "$window_id" =~ ^[0-9]+$ || -z "$window_height" ]]; then
  echo "capture-codex-ui: could not resolve the Codex window" >&2
  exit 1
fi

transient_open=0
nested_open=0
starter_seeded=0
composer_seeded=0
thread_composer_seeded=0
goal_enabled=0
plan_enabled=0
disclosure_open=0
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
  swift - "$image" "$query" "$region" "$window_x" "$window_y" "$window_width" "$window_height" <<'SWIFT'
import CoreGraphics
import Foundation
import ImageIO
import Vision

guard CommandLine.arguments.count == 8,
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
let request = VNRecognizeTextRequest()
request.recognitionLevel = .accurate
request.usesLanguageCorrection = true
request.recognitionLanguages = ["zh-Hans", "en-US"]
try VNImageRequestHandler(cgImage: image).perform([request])
let matches = (request.results ?? []).compactMap { observation -> (Bool, Float, CGRect, String)? in
  guard let candidate = observation.topCandidates(1).first else { return nil }
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
  case "thread":
    inRegion = observation.boundingBox.midX > 0.30 && observation.boundingBox.midY > 0.20
  case "thread-tail":
    inRegion = observation.boundingBox.midX > 0.30 &&
      observation.boundingBox.midY > 0.06 && observation.boundingBox.midY < 0.30
  default:
    inRegion = false
  }
  guard inRegion else { return nil }
  let exact = candidate.string.compare(query, options: [.caseInsensitive, .diacriticInsensitive]) == .orderedSame
  let contains = candidate.string.range(of: query, options: [.caseInsensitive, .diacriticInsensitive]) != nil
  return (exact || contains) ? (exact, candidate.confidence, observation.boundingBox, candidate.string) : nil
}
guard let match = matches.sorted(by: {
  if $0.0 != $1.0 { return $0.0 && !$1.0 }
  return $0.1 > $1.1
}).first else {
  fputs("window text not found in \(region): \(query)\n", stderr)
  exit(1)
}
let pointX = windowX + Double(match.2.midX) * windowWidth
let pointY = windowY + (1.0 - Double(match.2.midY)) * windowHeight
print("\(pointX)\t\(pointY)")
SWIFT
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
  if ((disclosure_open)); then
    send_click "$point_x" "$point_y" >/dev/null 2>&1 || true
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
    window_text_center "$ocr_capture" "Explore and" "starter" >/dev/null
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
      access) validation_text="Ask for approval"; validation_region="popover-low" ;;
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
  window_text_center "$ocr_capture" "What should we build" "main" >/dev/null
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
  window_text_center "$ocr_capture" "$composer_validate" "composer" >/dev/null
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
  disclosure_point=$(window_text_center "$ocr_capture" "$disclosure_target" "main")
  IFS=$'\t' read -r point_x point_y <<<"$disclosure_point"
  send_click "$point_x" "$point_y"
  disclosure_open=1
  sleep "$settle_seconds"
  rm -f -- "$ocr_capture"
  ocr_capture=$(mktemp -t codex-thread-disclosure-validate)
  screencapture -x -o -t png -l "$window_id" "$ocr_capture"
  window_text_center "$ocr_capture" "$disclosure_validate" "main" >/dev/null
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

if ! screencapture -x -l "$window_id" "$output"; then
  echo "capture-codex-ui: capture failed; grant Screen Recording permission to the invoking app" >&2
  exit 1
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
