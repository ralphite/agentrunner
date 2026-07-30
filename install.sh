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
#
# After installing, the script ALWAYS restarts a running daemon onto the new
# binary, and OFFERS to start the web UI. The store is durable, so a restart
# costs nothing but an in-flight turn's live output — which does die, loudly
# warned about rather than hidden. The web UI is opt-in instead: starting a
# listening server is not an installer's call to make, so with no terminal to
# ask (CI, `curl | sh` in a pipeline) the answer is no.
#   AR_NO_RESTART=1     leave a running daemon on its old binary
#   AR_START_WEBUI=1/0  answer the web UI prompt without a terminal
#   AR_WEBUI_ADDR       listen address for it (default: 0.0.0.0:<arwebui's port>,
#                       i.e. reachable on the LAN and UNAUTHENTICATED — set a
#                       127.0.0.1:PORT value to keep it on loopback)
#
# OS sandbox dependency (INC-75): on Linux, ar's bash/command tools require
# bubblewrap (fail-closed, 决策 #34). After installing the binaries this
# script probes for it and — when missing and running as root (or with
# passwordless sudo) — installs the distro package and clears the Ubuntu
# 23.10+ AppArmor userns restriction. macOS needs nothing (Seatbelt ships
# with the OS).
#   AR_SKIP_SANDBOX_DEPS=1  skip the sandbox dependency step entirely
#   AR_REQUIRE_SANDBOX=1    exit non-zero if the sandbox probe still fails
#                           (recommended for CI)

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

# --- OS sandbox dependency (Linux: bubblewrap, INC-75) -----------------------
# ar's bash/command tools refuse to run without the OS sandbox (fail-closed,
# 决策 #34); an install that leaves bwrap missing is an install of a product
# whose core tool cannot execute. Probe for real (run bwrap, not just PATH),
# auto-install when we have the privilege to, and say exactly what to do
# when we don't.

sandbox_probe() { # mirrors internal/tool/sandbox_linux.go platformSandboxProbe
  bwrap --ro-bind / / --proc /proc --dev /dev --unshare-pid /bin/true >/dev/null 2>&1
}

# as_root <cmd...> — run via direct root or passwordless sudo; fails otherwise.
as_root() {
  if [ "$(id -u)" = 0 ]; then "$@"
  elif command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then sudo -n "$@"
  else return 1
  fi
}

install_bwrap_pkg() {
  if command -v apt-get >/dev/null 2>&1; then
    as_root sh -c 'apt-get update -q && apt-get install -y -q bubblewrap'
  elif command -v dnf >/dev/null 2>&1; then as_root dnf install -y -q bubblewrap
  elif command -v yum >/dev/null 2>&1; then as_root yum install -y -q bubblewrap
  elif command -v pacman >/dev/null 2>&1; then as_root pacman -S --noconfirm --quiet bubblewrap
  elif command -v zypper >/dev/null 2>&1; then as_root zypper --non-interactive install bubblewrap
  elif command -v apk >/dev/null 2>&1; then as_root apk add --quiet bubblewrap
  else return 1
  fi
}

sandbox_status=ok
if [ "$os" = Linux ] && [ "${AR_SKIP_SANDBOX_DEPS:-}" != 1 ]; then
  echo
  echo "Checking the OS sandbox dependency (bubblewrap)..."
  if ! command -v bwrap >/dev/null 2>&1; then
    if install_bwrap_pkg; then
      echo "  installed bubblewrap via the system package manager"
    else
      sandbox_status=missing
    fi
  fi
  if [ "$sandbox_status" = ok ] && ! sandbox_probe; then
    # Present but not runnable — typically the Ubuntu 23.10+ AppArmor
    # restriction on unprivileged user namespaces.
    if as_root sysctl -qw kernel.apparmor_restrict_unprivileged_userns=0 2>/dev/null && sandbox_probe; then
      echo "  cleared kernel.apparmor_restrict_unprivileged_userns (this boot;"
      echo "  persist via /etc/sysctl.d/ if needed)"
    else
      sandbox_status=broken
    fi
  fi
  case "$sandbox_status" in
    ok) echo "  sandbox OK — bash/command tools will run OS-contained" ;;
    missing)
      echo "warning: bubblewrap is not installed and this shell cannot install it (no root/sudo)." >&2
      echo "  ar's bash/command tools will REFUSE to run until it is (fail-closed)." >&2
      echo "  Fix: sudo apt-get install -y bubblewrap   (dnf/pacman/zypper/apk ship it too)" >&2
      echo "  Then verify with: ar doctor" >&2
      ;;
    broken)
      echo "warning: bubblewrap is installed but the sandbox probe fails." >&2
      echo "  Likely the kernel restricts unprivileged user namespaces (Ubuntu 23.10+):" >&2
      echo "  sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0" >&2
      echo "  Then verify with: ar doctor" >&2
      ;;
  esac
  if [ "$sandbox_status" != ok ] && [ "${AR_REQUIRE_SANDBOX:-}" = 1 ]; then
    echo "error: AR_REQUIRE_SANDBOX=1 and the OS sandbox is unavailable — failing the install." >&2
    exit 1
  fi
fi

echo
echo "AgentRunner $version installed."
echo "  binaries: $BIN_DIR/ar, $BIN_DIR/arwebui → $dest"

# --- runtime handoff: restart the daemon, offer to start the web UI ----------
# An upgrade that leaves the OLD daemon running is the stale-binary class this
# project has lost time to twice: the new `ar` talks to a daemon that predates
# it, and a genuinely-fixed feature "fails" with a confusing error. The store
# under $AR_HOME is durable and survives restart, so the only thing a restart
# can destroy is an IN-FLIGHT model turn — which is exactly what we check for
# first, and refuse to trample.
#
# Nothing in this section may fail the install: the binaries are already in
# place and correct by this point. Every step degrades to a printed hint.
#   AR_NO_RESTART=1     leave the running daemon alone
#   AR_START_WEBUI=1/0  answer the web UI prompt non-interactively
#   AR_WEBUI_ADDR       listen address (default: 0.0.0.0:<arwebui's port>)
ar_bin="$dest/ar"
webui_bin="$dest/arwebui"

# Which daemon is "ours"? The one serving the store THIS binary resolves —
# mirroring runtime.DataDir(): $XDG_DATA_HOME/agentrunner, else
# ~/.local/share/agentrunner. Deliberately NOT $AR_HOME: that is where releases
# get unpacked and a caller may point it somewhere else entirely.
#
# The socket is the identity. Pattern-matching the process table instead
# (`pkill -f 'ar.*daemon'`) reaches straight out of whatever HOME we were given
# and can SIGTERM an unrelated daemon — a live one belonging to another store,
# another checkout, or another user. Learned the hard way: that exact pattern,
# run from a sandboxed installer test with a temp HOME, killed the real daemon
# on the developer's machine.
if [ -n "${XDG_DATA_HOME:-}" ]; then
  ar_data="$XDG_DATA_HOME/agentrunner"
else
  ar_data="$HOME/.local/share/agentrunner"
fi

# The socket is <datadir>/daemon.sock — EXCEPT when that path exceeds the unix
# sockaddr_un limit, where ar falls back to a hashed name under the temp dir
# (internal/cli/daemon.go socketPath). Replicating that length threshold here
# would be a drift waiting to happen, so probe BOTH candidates and take whichever
# actually exists. Found the hard way: a long XDG_DATA_HOME puts the socket in
# TMPDIR, and an installer that only knew the natural path saw "no daemon".
if command -v sha256sum >/dev/null 2>&1; then
  data_hash="$(printf %s "$ar_data" | sha256sum | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  data_hash="$(printf %s "$ar_data" | shasum -a 256 | awk '{print $1}')"
else
  data_hash=""
fi
sock_fallback=""
if [ -n "$data_hash" ]; then
  tmp_base="${TMPDIR:-/tmp}"
  case "$tmp_base" in */) tmp_base="${tmp_base%/}" ;; esac
  # hex.EncodeToString(h[:8]) — the first 8 bytes, i.e. 16 hex chars.
  sock_fallback="$tmp_base/ar-$(printf %s "$data_hash" | cut -c1-16).sock"
fi

# Echoes the live socket path, or nothing. Re-probed rather than cached: a
# freshly started daemon creates its socket after this script began.
find_sock() {
  if [ -S "$ar_data/daemon.sock" ]; then
    echo "$ar_data/daemon.sock"
  elif [ -n "$sock_fallback" ] && [ -S "$sock_fallback" ]; then
    echo "$sock_fallback"
  fi
}

ar_sock="$(find_sock)"

# A socket file can outlive a hard-killed daemon, so its presence means "there
# may be something here", not "a daemon is live". The pid lookup below settles it.
daemon_running() { [ -n "$(find_sock)" ]; }

# Likewise scoped: only a web UI serving OUR store's socket is ours to mention.
webui_running() {
  command -v pgrep >/dev/null 2>&1 || return 1
  pgrep -f "$dest/arwebui|$BIN_DIR/arwebui" >/dev/null 2>&1
}

# A turn in flight is the one thing a restart would destroy. Match only the
# structured status value: session ids and titles are free text and routinely
# contain the word "running".
turn_in_flight() {
  "$ar_bin" sessions --json 2>/dev/null |
    grep -qE '"status"[[:space:]]*:[[:space:]]*"running"'
}

start_daemon() {
  "$ar_bin" daemon --detach >/dev/null 2>&1 || return 1
  i=0
  while [ "$i" -lt 20 ]; do
    daemon_running && return 0
    i=$((i + 1))
    sleep 0.5
  done
  return 1
}

# Stop the old daemon. Preferred route is its exact pid via the socket; the
# fallback is a pattern scoped to paths THIS installer owns ($AR_HOME/releases
# and $BIN_DIR), which cannot match another store's or another user's daemon.
# What is never acceptable is a bare `pkill -f 'ar.*daemon'`: that ignores HOME
# and once SIGTERMed a live daemon belonging to a different store entirely.
stop_old_daemon() {
  if command -v lsof >/dev/null 2>&1; then
    old_pid="$(lsof -t "$ar_sock" 2>/dev/null | head -1)"
    if [ -n "$old_pid" ]; then
      kill -TERM "$old_pid" 2>/dev/null || true
      i=0
      while [ "$i" -lt 20 ] && kill -0 "$old_pid" 2>/dev/null; do
        i=$((i + 1)); sleep 0.5
      done
      return 0
    fi
  fi
  # No lsof, or a socket nobody holds. Sweep only our own install tree.
  if command -v pkill >/dev/null 2>&1; then
    pkill -TERM -f "$releases/.* daemon" 2>/dev/null || true
    pkill -TERM -f "$BIN_DIR/ar daemon" 2>/dev/null || true
    sleep 1
  fi
}

daemon_ready=no
if [ "${AR_NO_RESTART:-}" = 1 ]; then
  echo
  echo "  AR_NO_RESTART=1 — leaving any running daemon on its old binary."
  daemon_running && daemon_ready=stale
elif daemon_running; then
  echo
  # ALWAYS restart (operator's standing instruction). The store is durable, so a
  # restart costs nothing except an IN-FLIGHT turn — which does die. That is a
  # deliberate trade, so it is stated loudly rather than hidden; AR_NO_RESTART=1
  # is the way to opt out.
  if turn_in_flight; then
    echo "  WARNING: a session has a RUNNING turn. Restarting anyway, as configured —" >&2
    echo "  that turn's in-flight model output is lost (the session and its journal are" >&2
    echo "  durable and reopen fine). Use AR_NO_RESTART=1 to skip the restart instead." >&2
  fi
  printf '  Restarting the daemon on the new binary (the durable store survives)... '
  stop_old_daemon
  if start_daemon; then
    echo "ok"
    daemon_ready=yes
  else
    echo "failed"
    echo "  The daemon did not come up." >&2
    echo "  Start it and check the log: ar daemon --detach ; tail $ar_data/daemon.log" >&2
  fi
fi

# Web UI: opt-in, because starting a listening server is not something an
# installer should decide on its own. `curl | sh` puts the SCRIPT on stdin, so
# the prompt has to come from the terminal directly; with no terminal (CI,
# piped, provisioning) the answer is no.
want_webui="${AR_START_WEBUI:-}"
case "$want_webui" in
  1|y|Y|yes|YES) want_webui=yes ;;
  0|n|N|no|NO)   want_webui=no ;;
  "")
    if webui_running; then
      want_webui=no
      echo
      echo "  A web UI is already running — left alone (it may be on the previous build)."
      echo "  To move it to $version, stop it and run: arwebui --no-daemon"
    elif [ -r /dev/tty ] && [ -w /dev/tty ]; then
      echo
      printf 'Start the AgentRunner web UI now? [y/N] ' > /dev/tty
      read -r reply < /dev/tty 2>/dev/null || reply=n
      case "$reply" in y|Y|yes|YES) want_webui=yes ;; *) want_webui=no ;; esac
    else
      want_webui=no
    fi
    ;;
  *) want_webui=no ;;
esac

if [ "$want_webui" = yes ]; then
  # The web UI would otherwise spawn and manage a daemon of its own; when we
  # have one on the shared socket, --no-daemon keeps a single owner.
  if [ "$daemon_ready" = no ] && ! daemon_running; then
    printf '  Starting the daemon first... '
    if start_daemon; then echo "ok"; daemon_ready=yes; else echo "failed"; fi
  fi
  webui_log="$AR_HOME/webui.log"
  # Bind all interfaces so the UI is reachable from a phone or another machine
  # on the LAN (operator's standing instruction). Be clear-eyed about it: the
  # web UI has NO authentication, and the runtime behind it can run bash — so
  # this exposes it to everyone on the network. Set AR_WEBUI_ADDR=127.0.0.1:PORT
  # to keep it on loopback. arwebui's own default stays loopback; only the
  # installer's launch opts into 0.0.0.0.
  #
  webui_addr="${AR_WEBUI_ADDR:-0.0.0.0:8788}"
  printf '  Starting the web UI on %s... ' "$webui_addr"
  # Keep the log: it is where the listen address and any startup error live.
  nohup "$webui_bin" --no-daemon --ar "$ar_bin" --addr "$webui_addr" \
    >"$webui_log" 2>&1 &
  webui_pid=$!
  sleep 2
  if kill -0 "$webui_pid" 2>/dev/null; then
    echo "ok (pid $webui_pid)"
    # Report the address arwebui ITSELF logged, never a port hardcoded here —
    # a default that drifts would otherwise send the user to a dead URL.
    url="$(sed -n 's/.*listening on \(http:\/\/[^ ]*\).*/\1/p' "$webui_log" 2>/dev/null | tail -1)"
    if [ -n "$url" ]; then
      echo "  Open $url"
    else
      echo "  Listening — see $webui_log for the address"
    fi
    echo "  Log: $webui_log   Stop: kill $webui_pid"
  else
    echo "failed"
    echo "  See why: tail $webui_log   (or run: arwebui --no-daemon)" >&2
  fi
fi

echo
case ":$PATH:" in
  *":$BIN_DIR:"*) echo "  Get started: ar init && ar help" ;;
  *) echo "  Add $BIN_DIR to your PATH, then: ar init && ar help" ;;
esac
if [ "$daemon_ready" = no ] && [ "$want_webui" != yes ]; then
  echo "  Conversations need the runtime: ar daemon --detach"
fi
