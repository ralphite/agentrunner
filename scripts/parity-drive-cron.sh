#!/bin/zsh
# parity-drive-cron.sh — launchd 每 30 分钟触发一轮 /parity-drive(headless)。
# 跨会话存活的 Codex UI parity 驱动器:交互 session 关了它照跑。
# 安装:~/Library/LaunchAgents/com.agentrunner.parity-drive.plist
# 停用:launchctl bootout gui/$UID/com.agentrunner.parity-drive
# 日志:~/Library/Logs/parity-drive.log

set -u
REPO=/Users/yadong/dev2/agentrunner
LOCK=/tmp/parity-drive.lock
LOG=$HOME/Library/Logs/parity-drive.log
ROUND_TIMEOUT=1500   # 25min watchdog:防单轮挂死占锁

ts() { date "+%Y-%m-%d %H:%M:%S"; }
log() { echo "[$(ts)] $*" >> "$LOG"; }

# ---- 锁:同刻只允许一轮(交互 session 的轮也认这把锁) ----
if mkdir "$LOCK" 2>/dev/null; then
  echo "$$" > "$LOCK/pid"
else
  # 陈锁(>45min)判上一轮崩死:清掉重占;否则让路
  if [ -n "$(find "$LOCK" -maxdepth 0 -mmin +45 2>/dev/null)" ]; then
    log "stale lock (>45min) — stealing"
    rm -rf "$LOCK"; mkdir "$LOCK" || { log "steal failed, skip"; exit 0; }
    echo "$$" > "$LOCK/pid"
  else
    log "lock held (another round in progress) — skip this fire"
    exit 0
  fi
fi
trap 'rm -rf "$LOCK"' EXIT

# ---- 环境 ----
export PATH="$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
NODE24=$(ls -d "$HOME"/.nvm/versions/node/v24* 2>/dev/null | tail -1)
[ -n "$NODE24" ] && export PATH="$NODE24/bin:$PATH"
export PARITY_DRIVE_HEADLESS=1
cd "$REPO" || { log "repo missing"; exit 1; }

log "=== round start (headless) ==="
# env -i 白名单环境:隔离宿主(桌面 app harness)泄漏的 ANTHROPIC_*/CLAUDE_*
# 变量——它们会把 CLI 指到 app 内部代理导致 401(2026-07-11 实测)。
env -i HOME="$HOME" USER="$USER" LOGNAME="$USER" SHELL=/bin/zsh \
  PATH="$PATH" LANG=en_US.UTF-8 TMPDIR="${TMPDIR:-/tmp}" \
  PARITY_DRIVE_HEADLESS=1 \
  claude -p "/parity-drive" --permission-mode bypassPermissions >> "$LOG" 2>&1 &
CPID=$!
( sleep "$ROUND_TIMEOUT" && kill "$CPID" 2>/dev/null && echo "[$(ts)] watchdog killed round after ${ROUND_TIMEOUT}s" >> "$LOG" ) &
WPID=$!
wait "$CPID"
RC=$?
kill "$WPID" 2>/dev/null
log "=== round end rc=$RC ==="
exit 0
