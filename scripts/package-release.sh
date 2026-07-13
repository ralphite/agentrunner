#!/usr/bin/env bash
# package-release.sh — build the distributable AgentRunner tarballs (INC-63).
#
# One tar.gz per target, each holding the two static binaries (`ar`,
# `arwebui`) stamped with the same version via -ldflags, plus a matching
# .sha256. Pure-Go + CGO_ENABLED=0 means every target cross-compiles from
# this one machine — no per-OS runners (unlike handa's Python bundles).
#
# The frontend must be built BEFORE the Go build: arwebui go:embeds
# webui/frontend/dist and the repo only commits a .gitkeep placeholder.
#
# Usage:
#   scripts/package-release.sh                    # all targets → dist/
#   scripts/package-release.sh linux-x86_64       # one target
#
# Env:
#   AR_RELEASE_VERSION    version stamp (default: git describe/short SHA)
#   SKIP_FRONTEND_BUILD=1 reuse an existing webui/frontend/dist
#   DIST_DIR              output dir (default: dist)
set -euo pipefail
cd "$(dirname "$0")/.."

scripts/check-go-toolchain.sh

ALL_TARGETS=(linux-x86_64 linux-arm64 macos-arm64 macos-x86_64)
if [[ $# -gt 0 ]]; then TARGETS=("$@"); else TARGETS=("${ALL_TARGETS[@]}"); fi

VERSION="${AR_RELEASE_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
DIST_DIR="${DIST_DIR:-dist}"

goos_goarch() {
  case "$1" in
    linux-x86_64)  echo "linux amd64" ;;
    linux-arm64)   echo "linux arm64" ;;
    macos-arm64)   echo "darwin arm64" ;;
    macos-x86_64)  echo "darwin amd64" ;;
    *) echo "unknown target: $1 (expected: ${ALL_TARGETS[*]})" >&2; return 1 ;;
  esac
}
for t in "${TARGETS[@]}"; do goos_goarch "$t" >/dev/null; done

if [[ "${SKIP_FRONTEND_BUILD:-}" != 1 ]]; then
  echo "==> building frontend (webui/frontend → dist)"
  (cd webui/frontend && npm ci --no-audit --no-fund && npm run build)
fi
if [[ ! -f webui/frontend/dist/index.html ]]; then
  echo "package-release: webui/frontend/dist/index.html missing — build the frontend first (or unset SKIP_FRONTEND_BUILD)" >&2
  exit 1
fi

sha256() { # portable: sha256sum on linux, shasum on macOS
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$@"; else shasum -a 256 "$@"; fi
}

mkdir -p "$DIST_DIR"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

for target in "${TARGETS[@]}"; do
  read -r goos goarch <<<"$(goos_goarch "$target")"
  stage="$work/$target"
  mkdir -p "$stage"

  echo "==> building $target (GOOS=$goos GOARCH=$goarch, version $VERSION)"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -buildvcs=false -trimpath -ldflags "-s -w -X main.version=$VERSION" \
    -o "$stage/ar" ./cmd/agentrunner
  (cd webui && CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -buildvcs=false -trimpath -ldflags "-s -w -X main.version=$VERSION" \
    -o "$stage/arwebui" .)

  asset="agentrunner-$VERSION-$target.tar.gz"
  tar -czf "$DIST_DIR/$asset" -C "$stage" ar arwebui
  (cd "$DIST_DIR" && sha256 "$asset" > "$asset.sha256")
  echo "    $DIST_DIR/$asset"
done

echo
echo "Done. Version $VERSION → $DIST_DIR/"
