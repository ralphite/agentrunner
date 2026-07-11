#!/usr/bin/env bash
# deploy.sh — build → version-stamped install → restart daemon/webui, in one
#固化 step. Exists because we have twice lost time to the *stale shared binary*
# class: a fresh webui dist (new flags/features) driving an old `ar`/daemon in
# the shared environment, so a real feature "failed" with a cryptic error
# (INC-43 --steer → "flag provided but not defined"; see docs/LOG.md 2026-07-10
# 复盘 and docs/GAPS.md G33).
#
# Two hard rules from painful experience are baked in:
#   1. NEVER overwrite a running binary in place — install to a NEW versioned
#      path every time, then point the fresh process at it. Overwriting a live
#      daemon binary wedges old+new daemons together.
#   2. NEVER restart the daemon while a real turn is running — durable design
#      survives restart, but an in-flight model turn is killed. We check first.
#
# Both `ar` and `arwebui` are stamped with the same git commit via -ldflags, so
# webui can detect (and shout about) any future skew at startup and in /api/health.
#
# Usage:
#   scripts/deploy.sh                      # build + install + restart daemon + webui@8809
#   scripts/deploy.sh --addr 127.0.0.1:8809
#   scripts/deploy.sh --no-restart         # build + install only, print next steps
#   scripts/deploy.sh --force              # restart daemon even with a running turn (dangerous)
set -euo pipefail
cd "$(dirname "$0")/.."
REPO="$(pwd)"

ADDR="127.0.0.1:8809"
ENV_FILE="$REPO/.env"
DO_RESTART=1
FORCE=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --addr) ADDR="$2"; shift 2 ;;
    --env-file) ENV_FILE="$2"; shift 2 ;;
    --no-restart) DO_RESTART=0; shift ;;
    --force) FORCE=1; shift ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

BINDIR="${XDG_DATA_HOME:-$HOME/.local/share}/agentrunner/bin"
mkdir -p "$BINDIR"

COMMIT="$(git rev-parse --short HEAD)"
DIRTY=""
if ! git diff --quiet || ! git diff --cached --quiet; then DIRTY="-dirty"; fi
# Unique per-deploy suffix so we never clobber a binary a live process still
# maps (rule 1) even when redeploying the same commit.
STAMP="${COMMIT}${DIRTY}-$(date +%H%M%S)"

AR_OUT="$BINDIR/ar-$STAMP"
WEBUI_OUT="$BINDIR/arwebui-$STAMP"

echo "==> building ar    → $AR_OUT   (commit $STAMP)"
go build -ldflags "-X main.version=$STAMP" -o "$AR_OUT" ./cmd/agentrunner

echo "==> building arwebui → $WEBUI_OUT (embeds committed frontend/dist)"
( cd webui && go build -ldflags "-X main.version=$STAMP" -o "$WEBUI_OUT" . )

echo
echo "built:"
echo "  $AR_OUT     → $("$AR_OUT" --version)"
echo "  $WEBUI_OUT  → $("$WEBUI_OUT" --version)"

if [[ $DO_RESTART -eq 0 ]]; then
  cat <<EOF

--no-restart: install-only. To go live:
  # daemon (only if no turn is running):
  pkill -f 'ar.* daemon' ; "$AR_OUT" daemon &
  # webui:
  pkill -f "arwebui.*--addr $ADDR" ; \\
    "$WEBUI_OUT" --addr $ADDR --ar "$AR_OUT" --env-file "$ENV_FILE" &
EOF
  exit 0
fi

# ---- daemon restart (durable, but guard against a live turn) --------------
echo
echo "==> checking for running turns before restarting the daemon"
SESS="$("$AR_OUT" sessions 2>/dev/null || true)"
if echo "$SESS" | grep -qiE '\b(running|working|executing|thinking)\b'; then
  echo "!! a session has a RUNNING turn:" >&2
  echo "$SESS" | grep -iE 'running|working|executing|thinking' >&2
  if [[ $FORCE -eq 0 ]]; then
    echo "!! refusing to restart the daemon (would kill the turn). Re-run with --force to override, or wait." >&2
    exit 1
  fi
  echo "!! --force given: restarting anyway" >&2
fi

echo "==> restarting global daemon on new binary (durable store survives)"
# SIGTERM the old daemon(s); the store at ~/.local/share/agentrunner survives.
pkill -TERM -f '(^|/)(ar|ar-[^ ]*|arwebui) .*daemon' 2>/dev/null || true
pkill -TERM -f 'agentrunner daemon' 2>/dev/null || true
sleep 1
nohup "$AR_OUT" daemon >/dev/null 2>&1 &
# Wait for the socket to answer.
for _ in $(seq 1 20); do
  if "$AR_OUT" sessions >/dev/null 2>&1; then break; fi
  sleep 0.5
done
if ! "$AR_OUT" sessions >/dev/null 2>&1; then
  echo "!! new daemon did not come up — check ~/.local/share/agentrunner/daemon.log" >&2
  exit 1
fi
echo "   daemon up: $("$AR_OUT" --version)"

# ---- webui restart on new binary ------------------------------------------
echo "==> restarting webui on $ADDR"
pkill -f "arwebui.*--addr[= ]$ADDR" 2>/dev/null || true
sleep 1
# --no-daemon: the daemon we just started owns the shared socket; webui must not
# try to spawn/manage its own.
nohup "$WEBUI_OUT" --addr "$ADDR" --ar "$AR_OUT" --no-daemon \
  ${ENV_FILE:+--env-file "$ENV_FILE"} >/dev/null 2>&1 &
sleep 2
echo
echo "==> health check"
curl -fsS "http://$ADDR/api/health" || { echo "!! webui health failed" >&2; exit 1; }
echo
echo "deploy done — ar & webui both stamped $STAMP"
