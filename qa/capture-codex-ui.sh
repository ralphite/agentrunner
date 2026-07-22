#!/usr/bin/env bash
# Capture the real Codex desktop window on macOS. Optional command-palette mode
# proves low-risk interaction without depending on the Electron AX tree.
set -euo pipefail

usage() {
  cat <<'EOF'
usage: qa/capture-codex-ui.sh [--command-palette [--palette-query TEXT] | --surface NAME |
                              --new-chat-control NAME [--control-query TEXT] |
                              --context-menu VISIBLE_TEXT | --keyboard-context-menu VISIBLE_TEXT |
                              --account-menu | --user-menu]
                              [--settle SECONDS] [--restore-query TEXT] [--output PATH]

Captures the largest on-screen layer-0 window owned by bundle com.openai.codex.
--command-palette opens Cmd+K, captures it, then restores the prior state with Escape.
--palette-query enters a read-only search in the open command palette before capture.
--surface navigates to one read-only sidebar surface before capture. NAME is one of:
  new-chat, pull-requests, sites, scheduled, plugins
--new-chat-control opens one reversible New chat control before capture. NAME is one of:
  project, worktree, environment, branch, add, access, model,
  starter-explore, starter-build, starter-review, starter-fix
--control-query enters read-only search text in an opened New chat picker.
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
context_target=""
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
    project|worktree|environment|branch|add|access|model|starter-explore|starter-build|starter-review|starter-fix) ;;
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
starter_seeded=0
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
  if ((transient_open)); then
    send_key 53 >/dev/null 2>&1 || true
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
    add) target_text="Full access"; target_region="composer" ;;
    access) target_text="Full access"; target_region="composer" ;;
    model) target_text="5.6"; target_region="composer" ;;
    starter-explore) target_text="Explore and"; target_region="starter" ;;
    starter-build) target_text="Build a new feature"; target_region="starter" ;;
    starter-review) target_text="Review code and"; target_region="starter" ;;
    starter-fix) target_text="Fix issues and failures"; target_region="starter" ;;
  esac
  point=$(window_text_center "$ocr_capture" "$target_text" "$target_region")
  IFS=$'\t' read -r point_x point_y <<<"$point"
  if [[ "$new_chat_control" == "add" ]]; then
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
      add) validation_text="Files and folders" ;;
      access) validation_text="Ask for approval" ;;
      model) validation_text="Advanced" ;;
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
