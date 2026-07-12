#!/bin/sh
set -eu

# AgentRunner one-line installer for macOS / Linux (INC-63).
#
#   curl -fsSL https://raw.githubusercontent.com/ralphite/agentrunner/main/install.sh | sh
#
# Downloads the prebuilt release for this OS/arch (two static Go binaries,
# no toolchain needed), unpacks it under ~/.local/share/agentrunner/releases/
# and links `ar` + `arwebui` into ~/.local/bin. Run it again to upgrade —
# a running binary is never overwritten in place (new versioned dir, then
# symlink switch).
#
# While the repo is private, set GITHUB_TOKEN (or GH_TOKEN) with repo read
# access; the installer then downloads release assets via the GitHub API.
#
# Env overrides:
#   AR_REPO       GitHub repo            (default: ralphite/agentrunner)
#   AR_VERSION    release tag            (default: latest)
#   AR_HOME       install root           (default: ~/.local/share/agentrunner)
#   AR_BIN_DIR    where to link binaries (default: ~/.local/bin)
#   AR_ASSET_URL  direct tarball URL (skips GitHub entirely; for tests/mirrors.
#                 sha256 is fetched from $AR_ASSET_URL.sha256)

REPO="${AR_REPO:-ralphite/agentrunner}"
VERSION="${AR_VERSION:-latest}"
AR_HOME="${AR_HOME:-$HOME/.local/share/agentrunner}"
BIN_DIR="${AR_BIN_DIR:-$HOME/.local/bin}"
TOKEN="${GITHUB_TOKEN:-${GH_TOKEN:-}}"

command -v curl >/dev/null 2>&1 || { echo "error: curl is required" >&2; exit 1; }

os="$(uname -s)"
arch="$(uname -m)"
case "$os/$arch" in
  Linux/x86_64|Linux/amd64)   target="linux-x86_64" ;;
  Linux/aarch64|Linux/arm64)  target="linux-arm64" ;;
  Darwin/arm64)               target="macos-arm64" ;;
  Darwin/x86_64)              target="macos-x86_64" ;;
  *)
    echo "Unsupported platform: $os/$arch" >&2
    echo "AgentRunner ships prebuilt for linux-x86_64/arm64 and macos-arm64/x86_64." >&2
    exit 1
    ;;
esac

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT INT TERM
tarball="$tmpdir/agentrunner.tar.gz"
sumfile="$tmpdir/agentrunner.tar.gz.sha256"

# fetch <url> <outfile> [curl args...]
fetch() {
  url="$1"; out="$2"; shift 2
  if [ -n "$TOKEN" ]; then
    curl -fsSL -H "Authorization: Bearer $TOKEN" "$@" "$url" -o "$out"
  else
    curl -fsSL "$@" "$url" -o "$out"
  fi
}

if [ -n "${AR_ASSET_URL:-}" ]; then
  echo "Downloading $AR_ASSET_URL"
  fetch "$AR_ASSET_URL" "$tarball"
  fetch "$AR_ASSET_URL.sha256" "$sumfile" || : # optional for mirrors
elif [ -n "$TOKEN" ]; then
  # Private repo: resolve the release via the API, then download assets by id.
  api="https://api.github.com/repos/$REPO/releases"
  if [ "$VERSION" = "latest" ]; then rel_url="$api/latest"; else rel_url="$api/tags/$VERSION"; fi
  release_json="$tmpdir/release.json"
  fetch "$rel_url" "$release_json" -H "Accept: application/vnd.github+json"

  # Stable-named asset (agentrunner-<target>.tar.gz). No jq dependency: the
  # asset object is small and "id" precedes nothing we could confuse it with
  # once we cut the JSON at our asset's name.
  asset_id() {
    tr ',' '\n' <"$release_json" \
      | grep -B20 "\"name\" *: *\"$1\"" | grep '"id"' | tail -1 \
      | sed 's/[^0-9]*//g'
  }
  id="$(asset_id "agentrunner-$target.tar.gz")"
  sum_id="$(asset_id "agentrunner-$target.tar.gz.sha256")"
  if [ -z "$id" ]; then
    echo "error: release has no asset agentrunner-$target.tar.gz (repo $REPO, version $VERSION)" >&2
    exit 1
  fi
  echo "Downloading agentrunner-$target.tar.gz (asset $id) from $REPO $VERSION"
  fetch "$api/assets/$id" "$tarball" -H "Accept: application/octet-stream"
  [ -n "$sum_id" ] && fetch "$api/assets/$sum_id" "$sumfile" -H "Accept: application/octet-stream"
else
  asset="agentrunner-$target.tar.gz"
  if [ "$VERSION" = "latest" ]; then
    base="https://github.com/$REPO/releases/latest/download"
  else
    base="https://github.com/$REPO/releases/download/$VERSION"
  fi
  echo "Downloading $base/$asset"
  if ! fetch "$base/$asset" "$tarball"; then
    echo "error: download failed. If $REPO is private, set GITHUB_TOKEN and re-run." >&2
    exit 1
  fi
  fetch "$base/$asset.sha256" "$sumfile" || :
fi

if [ -s "$sumfile" ]; then
  want="$(awk '{print $1}' "$sumfile")"
  if command -v sha256sum >/dev/null 2>&1; then
    got="$(sha256sum "$tarball" | awk '{print $1}')"
  else
    got="$(shasum -a 256 "$tarball" | awk '{print $1}')"
  fi
  if [ "$want" != "$got" ]; then
    echo "error: sha256 mismatch (want $want, got $got) — corrupted download, nothing was installed" >&2
    exit 1
  fi
  echo "sha256 OK"
else
  echo "warning: no sha256 published for this asset; skipping verification" >&2
fi

unpack="$tmpdir/unpack"
mkdir -p "$unpack"
tar -xzf "$tarball" -C "$unpack"
[ -x "$unpack/ar" ] && [ -x "$unpack/arwebui" ] || {
  echo "error: tarball does not contain ar + arwebui" >&2; exit 1; }

# `ar --version` prints: agentrunner <version> (<go toolchain>)
version="$("$unpack/ar" --version 2>/dev/null | awk '{print $2}')"
[ -n "$version" ] || version="unknown"

# Install to a fresh versioned dir, then switch symlinks. Never write over a
# path a running process may have mapped: a same-version reinstall unpacks
# beside the old dir and replaces it whole (old inodes stay valid for running
# processes; only the directory entry changes).
releases="$AR_HOME/releases"
dest="$releases/$version"
mkdir -p "$releases" "$BIN_DIR"
staging="$releases/.staging-$version-$$"
rm -rf "$staging"
mv "$unpack" "$staging"
rm -rf "$dest"
mv "$staging" "$dest"

for bin in ar arwebui; do
  ln -sf "$dest/$bin" "$BIN_DIR/.$bin.new-$$"
  mv "$BIN_DIR/.$bin.new-$$" "$BIN_DIR/$bin"
done

echo
echo "AgentRunner $version installed."
echo "  binaries: $BIN_DIR/ar, $BIN_DIR/arwebui → $dest"
case ":$PATH:" in
  *":$BIN_DIR:"*) echo "  Get started: ar init && ar help" ;;
  *) echo "  Add $BIN_DIR to your PATH, then: ar init && ar help" ;;
esac
