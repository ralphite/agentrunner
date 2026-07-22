#!/usr/bin/env bash
# Capture the real Codex desktop window on macOS. Optional command-palette mode
# proves low-risk interaction without depending on the Electron AX tree.
set -euo pipefail

usage() {
  cat <<'EOF'
usage: qa/capture-codex-ui.sh [--command-palette | --surface NAME]
                              [--settle SECONDS] [--restore-query TEXT] [--output PATH]

Captures the largest on-screen layer-0 window owned by bundle com.openai.codex.
--command-palette opens Cmd+K, captures it, then restores the prior state with Escape.
--surface navigates to one read-only sidebar surface before capture. NAME is one of:
  new-chat, pull-requests, sites, scheduled, plugins
--settle waits for a navigated surface to become visually stable (default: 1 second).
--restore-query returns to a Codex thread through Cmd+K after a surface capture.
EOF
}

mode="current"
surface=""
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

palette_open=0
send_key() {
  local key_code=$1
  local command_flag=${2:-0}
  swift - "$codex_pid" "$key_code" "$command_flag" <<'SWIFT'
import AppKit
import CoreGraphics
import Foundation

guard CommandLine.arguments.count == 4,
      let pid = Int32(CommandLine.arguments[1]),
      let keyCode = UInt16(CommandLine.arguments[2]),
      let commandFlag = Int(CommandLine.arguments[3]),
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
if commandFlag == 1 {
  down.flags = .maskCommand
  up.flags = .maskCommand
}
down.post(tap: .cghidEventTap)
up.post(tap: .cghidEventTap)
SWIFT
}

send_click() {
  local x=$1
  local y=$2
  swift - "$codex_pid" "$x" "$y" <<'SWIFT'
import AppKit
import CoreGraphics
import Foundation

guard CommandLine.arguments.count == 4,
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
guard let down = CGEvent(mouseEventSource: source, mouseType: .leftMouseDown,
                         mouseCursorPosition: point, mouseButton: .left),
      let up = CGEvent(mouseEventSource: source, mouseType: .leftMouseUp,
                       mouseCursorPosition: point, mouseButton: .left) else {
  fputs("could not create mouse event\n", stderr)
  exit(1)
}
down.post(tap: .cghidEventTap)
up.post(tap: .cghidEventTap)
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
var units = Array(CommandLine.arguments[2].utf16)
let source = CGEventSource(stateID: .hidSystemState)
guard let down = CGEvent(keyboardEventSource: source, virtualKey: 0, keyDown: true),
      let up = CGEvent(keyboardEventSource: source, virtualKey: 0, keyDown: false) else {
  fputs("could not create text event\n", stderr)
  exit(1)
}
units.withUnsafeMutableBufferPointer { buffer in
  guard let base = buffer.baseAddress else { return }
  down.keyboardSetUnicodeString(stringLength: buffer.count, unicodeString: base)
  up.keyboardSetUnicodeString(stringLength: buffer.count, unicodeString: base)
}
down.post(tap: .cghidEventTap)
up.post(tap: .cghidEventTap)
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

close_palette() {
  if ((palette_open)); then
    send_key 53 >/dev/null 2>&1 || true
    palette_open=0
  fi
}
trap close_palette EXIT

if [[ "$mode" == "command-palette" ]]; then
  send_key 40 1
  palette_open=1
  sleep 1
fi

if ! screencapture -x -l "$window_id" "$output"; then
  echo "capture-codex-ui: capture failed; grant Screen Recording permission to the invoking app" >&2
  exit 1
fi
close_palette

if [[ -n "$restore_query" && -n "$surface" ]]; then
  send_key 40 1
  sleep 1
  send_text "$restore_query"
  sleep 1
  send_key 36
  sleep 1
fi

echo "$output"
