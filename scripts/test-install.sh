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
  # AR_SKIP_SANDBOX_DEPS keeps the base scenarios offline and host-agnostic;
  # the sandbox-dep step has its own scenarios below with stubbed bwrap.
  #
  # XDG_DATA_HOME is pinned INSIDE the sandbox because the installer's runtime
  # step resolves the daemon store exactly as runtime.DataDir() does — leaving
  # the host's value set would point it at the developer's real daemon.
  # AR_NO_RESTART/AR_START_WEBUI are belt-and-braces on top of that: no scenario
  # here may touch a live daemon or spawn a server. Scenario 9 covers the
  # runtime step's own behavior explicitly.
  env HOME="$work/home" AR_HOME="$work/home/share" AR_BIN_DIR="$work/home/bin" \
    XDG_DATA_HOME="$work/home/share-xdg" \
    AR_ASSET_URL="$url" GITHUB_TOKEN= GH_TOKEN= AR_SKIP_SANDBOX_DEPS=1 \
    AR_NO_RESTART=1 AR_START_WEBUI=0 "$@" sh "$REPO/install.sh"
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

# --- 9. runtime step must stay INSIDE the sandbox --------------------------
# Regression test for a real incident: the runtime step originally found the
# daemon with `pkill -f 'ar.*daemon'`, which ignores HOME entirely. Run from
# this harness — temp HOME, stub binaries — it SIGTERMed the developer's live
# daemon and could not restart it (the stub cannot). The step now identifies the
# daemon only through the socket in the store it resolves, so a process that
# merely LOOKS like a daemon must survive untouched.
#
# Deliberately runs without AR_NO_RESTART: the point is to exercise the restart
# path and prove it finds nothing to restart here.
scenario9() {
  mkdir -p "$work/decoy"
  printf '#!/bin/sh\nsleep 30\n' >"$work/decoy/ar"
  chmod +x "$work/decoy/ar"
  "$work/decoy/ar" daemon &
  decoy=$!
  # Give it a moment to appear in the process table.
  sleep 1
  kill -0 "$decoy" 2>/dev/null || fail "decoy daemon did not start"

  env HOME="$work/home" AR_HOME="$work/home/share" AR_BIN_DIR="$work/home/bin" \
    XDG_DATA_HOME="$work/home/share-xdg" \
    AR_ASSET_URL="file://$work/assets/v1.1.0/agentrunner.tar.gz" \
    GITHUB_TOKEN= GH_TOKEN= AR_SKIP_SANDBOX_DEPS=1 AR_START_WEBUI=0 \
    sh "$REPO/install.sh" >"$work/out9" 2>&1 \
    || { cat "$work/out9" >&2; fail "runtime-step install exited non-zero"; }

  if ! kill -0 "$decoy" 2>/dev/null; then
    cat "$work/out9" >&2
    fail "installer killed a process that only LOOKED like our daemon (scope regression)"
  fi
  kill -TERM "$decoy" 2>/dev/null || true
  wait "$decoy" 2>/dev/null || true

  # No daemon in this store, so there was nothing to restart, and the web UI
  # must not have been started either.
  grep -q "Restarting the daemon" "$work/out9" &&
    fail "nothing should have been restarted in an empty store"
  grep -q "Starting the web UI" "$work/out9" &&
    fail "AR_START_WEBUI=0 must not start the web UI"
  # The install itself still has to have succeeded.
  grep -q "AgentRunner .* installed" "$work/out9" || fail "missing install confirmation"
  # And it should tell the user how to get a runtime, since it started none.
  grep -q "ar daemon --detach" "$work/out9" || fail "missing runtime hint when no daemon was started"
}

# --- 6-8. OS sandbox dependency step (INC-75, Linux only) ------------------
# Stubbed bwrap/sysctl keep these offline and side-effect free. The
# missing-bwrap branch (no bwrap anywhere on PATH) is deliberately not
# twinned: hiding a system binary needs fragile whole-PATH surgery, and the
# branch is pure diagnostics — gate B (sandbox-doctor workflow) covers the
# real no-bwrap runner.
if [[ "$(uname -s)" == Linux ]]; then
  sandboxbin="$work/sandboxbin"
  mkdir -p "$sandboxbin"

  # 6. bwrap present and probe passes → sandbox OK, exit 0
  printf '#!/bin/sh\nexit 0\n' >"$sandboxbin/bwrap"
  chmod +x "$sandboxbin/bwrap"
  PATH="$sandboxbin:$PATH" run_install "file://$work/assets/v1.1.0/agentrunner.tar.gz" \
    AR_SKIP_SANDBOX_DEPS= >"$work/out6" 2>&1 \
    || { cat "$work/out6" >&2; fail "sandbox-ok install exited non-zero"; }
  grep -q "sandbox OK" "$work/out6" || fail "missing sandbox OK confirmation"

  # 7. bwrap present but probe fails, sysctl remedy fails → warning, exit 0
  printf '#!/bin/sh\nexit 1\n' >"$sandboxbin/bwrap"
  printf '#!/bin/sh\nexit 1\n' >"$sandboxbin/sysctl"
  printf '#!/bin/sh\nexit 1\n' >"$sandboxbin/sudo"
  chmod +x "$sandboxbin/bwrap" "$sandboxbin/sysctl" "$sandboxbin/sudo"
  PATH="$sandboxbin:$PATH" run_install "file://$work/assets/v1.1.0/agentrunner.tar.gz" \
    AR_SKIP_SANDBOX_DEPS= >"$work/out7" 2>&1 \
    || { cat "$work/out7" >&2; fail "broken-sandbox install must still exit 0 by default"; }
  grep -q "sandbox probe fails" "$work/out7" || fail "missing broken-sandbox warning"
  grep -q "ar doctor" "$work/out7" || fail "broken-sandbox warning must point at ar doctor"

  # 8. same broken sandbox with AR_REQUIRE_SANDBOX=1 → hard fail
  if PATH="$sandboxbin:$PATH" run_install "file://$work/assets/v1.1.0/agentrunner.tar.gz" \
    AR_SKIP_SANDBOX_DEPS= AR_REQUIRE_SANDBOX=1 >"$work/out8" 2>&1; then
    fail "AR_REQUIRE_SANDBOX=1 did not fail on a broken sandbox"
  fi
  grep -q "AR_REQUIRE_SANDBOX=1" "$work/out8" || fail "missing require-sandbox diagnostic"

  scenario9
  echo "test-install: OK (9 scenarios)"
else
  scenario9
  echo "test-install: OK (6 scenarios; sandbox-dep scenarios are Linux-only)"
fi
