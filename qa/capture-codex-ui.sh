#!/usr/bin/env bash
# Capture the real Codex desktop window on macOS. Optional command-palette mode
# proves low-risk interaction without depending on Computer Use access to Codex.
set -euo pipefail

usage() {
  cat <<'EOF'
usage: qa/capture-codex-ui.sh [--command-palette] [--output PATH]

Captures the largest on-screen layer-0 window owned by bundle com.openai.codex.
--command-palette opens Cmd+K, captures it, then restores the prior state with Escape.
EOF
}

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "capture-codex-ui: macOS is required" >&2
  exit 1
fi

mode="current"
output=""
while (($#)); do
  case "$1" in
    --command-palette)
      mode="command-palette"
      shift
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

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
if [[ -z "$output" ]]; then
  stamp=$(date '+%Y%m%d-%H%M%S')
  output="$repo_root/qa/runs/$(date '+%Y-%m-%d')-codex-live-capture/$stamp-$mode.png"
fi
mkdir -p "$(dirname "$output")"

codex_pid=$(osascript <<'APPLESCRIPT'
tell application "System Events"
  set codexProcess to first application process whose bundle identifier is "com.openai.codex"
  return unix id of codexProcess
end tell
APPLESCRIPT
)
if [[ ! "$codex_pid" =~ ^[0-9]+$ ]]; then
  echo "capture-codex-ui: could not resolve the Codex process" >&2
  exit 1
fi

window_id=$(swift - "$codex_pid" <<'SWIFT'
import CoreGraphics
import Foundation

guard CommandLine.arguments.count == 2,
      let targetPid = Int32(CommandLine.arguments[1]) else {
  fputs("invalid Codex pid\n", stderr)
  exit(2)
}

let options: CGWindowListOption = [.optionOnScreenOnly, .excludeDesktopElements]
let windows = CGWindowListCopyWindowInfo(options, kCGNullWindowID) as? [[String: Any]] ?? []
let candidates = windows.compactMap { window -> (id: Int, area: Double)? in
  guard (window[kCGWindowOwnerPID as String] as? Int32) == targetPid,
        (window[kCGWindowLayer as String] as? Int) == 0,
        let id = window[kCGWindowNumber as String] as? Int,
        let bounds = window[kCGWindowBounds as String] as? [String: Any],
        let width = bounds["Width"] as? Double,
        let height = bounds["Height"] as? Double else { return nil }
  return (id, width * height)
}

guard let target = candidates.max(by: { $0.area < $1.area }) else {
  fputs("no on-screen Codex window found\n", stderr)
  exit(1)
}
print(target.id)
SWIFT
)
if [[ ! "$window_id" =~ ^[0-9]+$ ]]; then
  echo "capture-codex-ui: could not resolve the Codex window" >&2
  exit 1
fi

palette_open=0
close_palette() {
  if ((palette_open)); then
    osascript - "$codex_pid" <<'APPLESCRIPT' >/dev/null 2>&1 || true
on run argv
  tell application "System Events"
    set codexProcess to first application process whose unix id is (item 1 of argv as integer)
    set frontmost of codexProcess to true
    key code 53
  end tell
end run
APPLESCRIPT
    palette_open=0
  fi
}
trap close_palette EXIT

if [[ "$mode" == "command-palette" ]]; then
  osascript - "$codex_pid" <<'APPLESCRIPT'
on run argv
  tell application "System Events"
    set codexProcess to first application process whose unix id is (item 1 of argv as integer)
    set frontmost of codexProcess to true
    keystroke "k" using command down
  end tell
end run
APPLESCRIPT
  palette_open=1
  sleep 1
fi

if ! screencapture -x -l "$window_id" "$output"; then
  echo "capture-codex-ui: capture failed; grant Screen Recording permission to the invoking app" >&2
  exit 1
fi
close_palette

echo "$output"
