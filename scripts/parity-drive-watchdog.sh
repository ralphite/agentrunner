#!/bin/zsh
# parity-drive-watchdog.sh — 每 ~10min 由 launchd(StartInterval=600)触发。
# 职责:保证 parity-drive 30 分钟循环【永远活着】,不参与任何业务。
#   1) 清陈锁(>60min):某轮崩死后残留的 /tmp/parity-drive.lock 会把后续所有
#      轮永久挡住——清掉它。
#   2) 若 plist 被改名成 .stopped(旧的自杀行为),恢复回来。
#   3) 若定时器没 loaded(被 bootout 了),重新 bootstrap;RunAtLoad 会立刻起一轮。
# 安装:~/Library/LaunchAgents/com.agentrunner.parity-drive-watchdog.plist
# 日志:~/Library/Logs/parity-drive-watchdog.log
set -u

UID_="$(id -u)"
LABEL=com.agentrunner.parity-drive
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
LOG="$HOME/Library/Logs/parity-drive-watchdog.log"
LOCK=/tmp/parity-drive.lock

ts()  { date "+%Y-%m-%d %H:%M:%S"; }
log() { echo "[$(ts)] $*" >> "$LOG"; }

# 1) 清陈锁(>60min)。锁是目录(mkdir 原子锁),用 find -mmin 判龄。
if [ -d "$LOCK" ] && [ -n "$(find "$LOCK" -maxdepth 0 -mmin +60 2>/dev/null)" ]; then
  rm -rf "$LOCK" && log "cleared stale lock (>60min)"
fi

# 2) plist 被改名成 .stopped 就恢复回来。
if [ ! -f "$PLIST" ] && [ -f "$PLIST.stopped" ]; then
  mv "$PLIST.stopped" "$PLIST" && log "restored plist from .stopped"
fi

# 3) 定时器没 loaded 就重新 bootstrap(RunAtLoad=true 会立刻触发一轮)。
if launchctl print "gui/$UID_/$LABEL" >/dev/null 2>&1; then
  : # loaded & healthy — 无需动作(静默,避免刷日志)
else
  # 定时器没 loaded ⇒ 没有合法的轮在跑 ⇒ 此刻若还有锁,必是被硬杀的轮
  # 遗留的孤儿锁。清掉它,否则重新 bootstrap 后的新轮会因"锁新鲜"而跳过。
  if [ -d "$LOCK" ]; then rm -rf "$LOCK" && log "cleared orphan lock (job was down)"; fi
  if [ -f "$PLIST" ]; then
    if launchctl bootstrap "gui/$UID_" "$PLIST" 2>>"$LOG"; then
      log "re-bootstrapped $LABEL (was not loaded)"
    else
      log "bootstrap FAILED for $LABEL"
    fi
  else
    log "cannot bootstrap: $PLIST missing (and no .stopped to restore)"
  fi
fi

# 4) 剪枝 implementer worktree(防无界堆积)。每个 implementer 用 isolation:worktree,
#    提交后有改动、不会被自动回收,会无界堆积(实测曾到 91 个 / 11GB,险些撑爆盘)。
#    删 dir-mtime >60min 的:in-flight implementer 只跑几分钟,>60min = 早完事已 push,
#    60min 阈值绝不误删在跑的;删掉不丢数据(commit 都在 origin/main)。
REPO=/Users/yadong/dev2/agentrunner
WT="$REPO/.claude/worktrees"
if [ -d "$WT" ]; then
  n=$(find "$WT" -maxdepth 1 -mindepth 1 -type d -mmin +60 2>/dev/null | wc -l | tr -d ' ')
  if [ "$n" -gt 0 ]; then
    find "$WT" -maxdepth 1 -mindepth 1 -type d -mmin +60 -exec rm -rf {} + 2>/dev/null
    git -C "$REPO" worktree prune 2>/dev/null
    log "pruned $n idle implementer worktrees (dir mtime >60min)"
  fi
fi
