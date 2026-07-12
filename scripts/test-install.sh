#!/usr/bin/env bash
# test-install.sh — offline scripted twin for install.sh (INC-63 gate A).
#
# Runs the real install.sh against a file:// tarball of stub binaries (the
# installer never inspects binary contents beyond `ar --version`, so stubs
# keep the gate fast enough for check.sh). Asserts the install layout,
# upgrade symlink switch, same-version reinstall, sha256 failure path and
# unsupported-platform message.
set -euo pipefail
cd "$(dirname "$0")/.."
REPO="$(pwd)"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
fail() { echo "test-install: FAIL: $*" >&2; exit 1; }

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$@"; else shasum -a 256 "$@"; fi
}

# make_asset <version> → $work/assets/<version>/agentrunner.tar.gz (+ .sha256)
make_asset() {
  local ver="$1" dir="$work/assets/$1" stage
  stage="$(mktemp -d)"
  printf '#!/bin/sh\nif [ "${1:-}" = "--version" ]; then echo "agentrunner %s (stub)"; fi\n' "$ver" >"$stage/ar"
  printf '#!/bin/sh\necho "arwebui stub %s"\n' "$ver" >"$stage/arwebui"
  chmod +x "$stage/ar" "$stage/arwebui"
  mkdir -p "$dir"
  tar -czf "$dir/agentrunner.tar.gz" -C "$stage" ar arwebui
  (cd "$dir" && sha256 agentrunner.tar.gz >agentrunner.tar.gz.sha256)
  rm -rf "$stage"
}

run_install() { # run_install <asset-url> [extra env...]
  local url="$1"; shift
  env HOME="$work/home" AR_HOME="$work/home/share" AR_BIN_DIR="$work/home/bin" \
    AR_ASSET_URL="$url" GITHUB_TOKEN= GH_TOKEN= "$@" sh "$REPO/install.sh"
}

make_asset v1.0.0
make_asset v1.1.0
mkdir -p "$work/home"

# --- 1. fresh install: versioned dir + working symlinks -------------------
run_install "file://$work/assets/v1.0.0/agentrunner.tar.gz" >"$work/out1" 2>&1 \
  || { cat "$work/out1" >&2; fail "fresh install exited non-zero"; }
[[ -x "$work/home/share/releases/v1.0.0/ar" ]] || fail "ar not in releases/v1.0.0"
[[ -L "$work/home/bin/ar" && -L "$work/home/bin/arwebui" ]] || fail "bin symlinks missing"
[[ "$("$work/home/bin/ar" --version)" == "agentrunner v1.0.0 (stub)" ]] || fail "linked ar not runnable"
grep -q "sha256 OK" "$work/out1" || fail "sha256 not verified on fresh install"

# --- 2. upgrade: symlink switches, old release dir stays ------------------
run_install "file://$work/assets/v1.1.0/agentrunner.tar.gz" >/dev/null 2>&1 || fail "upgrade exited non-zero"
[[ "$(readlink "$work/home/bin/ar")" == "$work/home/share/releases/v1.1.0/ar" ]] || fail "upgrade did not switch symlink"
[[ -d "$work/home/share/releases/v1.0.0" ]] || fail "upgrade removed previous release dir"

# --- 3. same-version reinstall replaces wholesale, keeps links valid ------
run_install "file://$work/assets/v1.1.0/agentrunner.tar.gz" >/dev/null 2>&1 || fail "reinstall exited non-zero"
[[ "$("$work/home/bin/ar" --version)" == "agentrunner v1.1.0 (stub)" ]] || fail "reinstall broke linked ar"
[[ -z "$(ls "$work/home/share/releases" | grep '^\.staging')" ]] || fail "staging leftovers after reinstall"

# --- 4. sha256 mismatch: hard fail, existing install untouched ------------
mkdir -p "$work/assets/bad"
cp "$work/assets/v1.0.0/agentrunner.tar.gz" "$work/assets/bad/"
echo "0000000000000000000000000000000000000000000000000000000000000000  agentrunner.tar.gz" \
  >"$work/assets/bad/agentrunner.tar.gz.sha256"
if run_install "file://$work/assets/bad/agentrunner.tar.gz" >"$work/out4" 2>&1; then
  fail "tampered sha256 did not fail the install"
fi
grep -q "sha256 mismatch" "$work/out4" || fail "missing sha256 mismatch diagnostic"
[[ "$("$work/home/bin/ar" --version)" == "agentrunner v1.1.0 (stub)" ]] || fail "failed install disturbed existing links"

# --- 5. unsupported platform message --------------------------------------
mkdir -p "$work/fakebin"
printf '#!/bin/sh\ncase "${1:-}" in -s) echo SunOS ;; -m) echo sparc64 ;; *) echo SunOS ;; esac\n' >"$work/fakebin/uname"
chmod +x "$work/fakebin/uname"
if PATH="$work/fakebin:$PATH" run_install "file://$work/assets/v1.0.0/agentrunner.tar.gz" >"$work/out5" 2>&1; then
  fail "unsupported platform did not fail"
fi
grep -q "Unsupported platform" "$work/out5" || fail "missing unsupported-platform diagnostic"

echo "test-install: OK (5 scenarios)"
