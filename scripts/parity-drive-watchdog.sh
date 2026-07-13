#!/bin/zsh
# parity-drive-watchdog.sh — 每 ~5min 由 launchd(StartInterval=300)触发。
# 职责:保证 parity-drive 连续循环【永远活着】,并检查 5min CL 产出节奏。
#   1) 清陈锁(>60min):某轮崩死后残留的 /tmp/parity-drive.lock 会把后续所有
#      轮永久挡住——清掉它。
#   2) 若 plist 被改名成 .stopped(旧的自杀行为),恢复回来。
#   3) 若定时器没 loaded(被 bootout 了),重新 bootstrap;RunAtLoad 会立刻起一轮。
#   4) 若 origin/main 连续 5min 没有新 CL,记录诊断;仅在执行流也无活动时重启。
# 安装:~/Library/LaunchAgents/com.agentrunner.parity-drive-watchdog.plist
# 日志:~/Library/Logs/parity-drive-watchdog.log
set -u

UID_="$(id -u)"
LABEL=com.agentrunner.parity-drive
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
LOG="$HOME/Library/Logs/parity-drive-watchdog.log"
LOCK=/tmp/parity-drive.lock
REPO=/Users/yadong/dev2/agentrunner
STATE_DIR="$HOME/.local/share/agentrunner"
CL_STATE="$STATE_DIR/parity-drive-watchdog.state"
NO_CL_FLAG="$STATE_DIR/parity-drive-no-cl.flag"
NO_CL_TIMEOUT=300

setopt null_glob

ts()  { date "+%Y-%m-%d %H:%M:%S"; }
log() { echo "[$(ts)] $*" >> "$LOG"; }

# 1) 清陈锁。优先清 pid 已死的锁;否则清 >60min 的锁。锁是目录(mkdir 原子锁)。
if [ -f "$LOCK/pid" ]; then
  LOCKPID=$(cat "$LOCK/pid" 2>/dev/null || echo "")
  if [[ "$LOCKPID" == <-> ]] && ! kill -0 "$LOCKPID" 2>/dev/null; then
    rm -rf "$LOCK" && log "cleared dead lock pid=$LOCKPID"
  fi
fi
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

# 4) 每 5min 检查 origin/main 是否有新 CL。没有 CL 不等于挂死:只在 driver log
#    和 agent transcript 也都静默 >=5min 时 kickstart;仍有活动则只留下诊断信号,
#    避免杀掉正在跑 targeted test / build gate 的高质量轮次。
mkdir -p "$STATE_DIR"
GIT_TERMINAL_PROMPT=0 git -C "$REPO" -c http.lowSpeedLimit=1 -c http.lowSpeedTime=15 \
  fetch -q origin main 2>/dev/null || true
CURRENT_SHA=$(git -C "$REPO" rev-parse origin/main 2>/dev/null || echo unknown)
NOW=$(date +%s)
LAST_SHA=""
LAST_CHANGE="$NOW"
if [ -f "$CL_STATE" ]; then
  read LAST_SHA LAST_CHANGE < "$CL_STATE" 2>/dev/null || true
  [[ "$LAST_CHANGE" == <-> ]] || LAST_CHANGE="$NOW"
fi

if [ "$CURRENT_SHA" != "$LAST_SHA" ]; then
  echo "$CURRENT_SHA $NOW" > "$CL_STATE"
  rm -f "$NO_CL_FLAG"
  [ -n "$LAST_SHA" ] && log "CL progress: ${LAST_SHA[1,8]} -> ${CURRENT_SHA[1,8]}"
elif [ $(( NOW - LAST_CHANGE )) -ge "$NO_CL_TIMEOUT" ]; then
  LATEST_ACTIVITY=0
  for f in "$HOME/Library/Logs/parity-drive.log" "$HOME"/.claude/projects/*agentrunner*/*.jsonl; do
    [ -f "$f" ] || continue
    MTIME=$(stat -f %m "$f" 2>/dev/null || echo 0)
    [ "$MTIME" -gt "$LATEST_ACTIVITY" ] && LATEST_ACTIVITY="$MTIME"
  done
  IDLE=$(( NOW - LATEST_ACTIVITY ))
  printf '%s sha=%s no_cl_for=%ss activity_idle=%ss\n' "$(ts)" "$CURRENT_SHA" "$(( NOW - LAST_CHANGE ))" "$IDLE" > "$NO_CL_FLAG"
  if [ "$IDLE" -ge "$NO_CL_TIMEOUT" ]; then
    log "NO-CL + STALL: main unchanged for $(( NOW - LAST_CHANGE ))s, activity idle ${IDLE}s — kickstarting $LABEL"
    launchctl kickstart -k "gui/$UID_/$LABEL" 2>>"$LOG" || log "kickstart FAILED for $LABEL"
  else
    log "NO-CL check: main unchanged for $(( NOW - LAST_CHANGE ))s, workflow active ${IDLE}s ago — no restart"
  fi
  # 下一次 300s schedule 再检查并重新诊断,不在同一陈旧窗口反复告警。
  echo "$CURRENT_SHA $NOW" > "$CL_STATE"
fi

# 5) 剪枝 implementer worktree(防无界堆积)。每个 implementer 用 isolation:worktree,
#    提交后有改动、不会被自动回收,会无界堆积(实测曾到 91 个 / 11GB,险些撑爆盘)。
#    删 dir-mtime >60min 的:in-flight implementer 只跑几分钟,>60min = 早完事已 push,
#    60min 阈值绝不误删在跑的;删掉不丢数据(commit 都在 origin/main)。
WT="$REPO/.claude/worktrees"
if [ -d "$WT" ]; then
  n=$(find "$WT" -maxdepth 1 -mindepth 1 -type d -mmin +60 2>/dev/null | wc -l | tr -d ' ')
  if [ "$n" -gt 0 ]; then
    find "$WT" -maxdepth 1 -mindepth 1 -type d -mmin +60 -exec rm -rf {} + 2>/dev/null
    git -C "$REPO" worktree prune 2>/dev/null
    log "pruned $n idle implementer worktrees (dir mtime >60min)"
  fi
fi
