#!/usr/bin/env bash
set -euo pipefail

# Build outputs inherit standard-library vulnerabilities from the compiler.
# Keep the two currently supported Go lines above their July 2026 security
# releases (GO-2026-5856 / GO-2026-4918), and reject prerelease toolchains.
version="${GO_VERSION_OVERRIDE:-$(go env GOVERSION)}"
if [[ "$version" == *rc* || "$version" == *beta* || "$version" == devel* ]]; then
  echo "toolchain: stable Go required (found $version)" >&2
  exit 1
fi
if [[ ! "$version" =~ ^go([0-9]+)\.([0-9]+)(\.([0-9]+))? ]]; then
  echo "toolchain: cannot parse Go version $version" >&2
  exit 1
fi
major="${BASH_REMATCH[1]}"
minor="${BASH_REMATCH[2]}"
patch="${BASH_REMATCH[4]:-0}"

safe=0
if (( major > 1 )); then
  safe=1
elif (( major == 1 )); then
  if (( minor > 26 )); then
    safe=1
  elif (( minor == 26 && patch >= 5 )); then
    safe=1
  elif (( minor == 25 && patch >= 12 )); then
    safe=1
  fi
fi
if (( ! safe )); then
  echo "toolchain: $version is unsupported or has known standard-library vulnerabilities; use Go 1.25.12+, 1.26.5+, or a newer stable release" >&2
  exit 1
fi
