#!/bin/zsh
# parity-drive-cron.sh — 【连续】跑 /parity-drive(headless)。一轮结束立刻起下一轮。
# velocity 硬规则(2026-07-11 用户裁决):**idle 即失败**。没有 30 分钟 sleep,只有
#   秒级 guard 防热转。launchd 只负责把本脚本【永远保活】(KeepAlive);本脚本自身
#   是 while-true 连续循环,一轮完立即进下一轮。
# 设计意图:永不停的持续改进循环。脚本自身绝不 bootout/unload、绝不重命名/删除 plist。
# 安装:~/Library/LaunchAgents/com.agentrunner.parity-drive.plist(直接跑本脚本 + KeepAlive)
# 只应由真人手动停:launchctl bootout gui/$UID/com.agentrunner.parity-drive + mv plist 为 .stopped
# 日志:~/Library/Logs/parity-drive.log

set -u
REPO=/Users/yadong/dev2/agentrunner
LOCK=/tmp/parity-drive.lock
LOG=$HOME/Library/Logs/parity-drive.log
PLIST="$HOME/Library/LaunchAgents/com.agentrunner.parity-drive.plist"
ROUND_TIMEOUT=3300   # 55min 硬顶 watchdog per round:杀超时轮,循环照进下一轮
STALL_TIMEOUT=600    # 10min 停滞 watchdog:log 与 transcript 都 >10min 没动=挂了,杀掉重起
GUARD=8              # 轮间只睡这几秒防热转——绝不是 30min heartbeat
PROJDIR="$HOME/.claude/projects/-Users-yadong-dev2-agentrunner"  # headless 轮的 transcript 落这

ts()  { date "+%Y-%m-%d %H:%M:%S"; }
log() { echo "[$(ts)] $*" >> "$LOG"; }

# ---- 环境(设一次) ----
export PATH="$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
NODE24=$(ls -d "$HOME"/.nvm/versions/node/v24* 2>/dev/null | tail -1)
[ -n "$NODE24" ] && export PATH="$NODE24/bin:$PATH"
export PARITY_DRIVE_HEADLESS=1
cd "$REPO" || { log "repo missing"; exit 1; }

run_round() {
  # ---- 锁:同刻只允许一轮(交互 session 的轮也认这把锁) ----
  if mkdir "$LOCK" 2>/dev/null; then
    echo "$$" > "$LOCK/pid"
  else
    # 陈锁(>45min)判上一轮崩死:清掉重占;否则让路(交互 session 正在跑)
    if [ -n "$(find "$LOCK" -maxdepth 0 -mmin +45 2>/dev/null)" ]; then
      log "stale lock (>45min) — stealing"
      rm -rf "$LOCK"; mkdir "$LOCK" 2>/dev/null || { log "steal failed, skip"; return; }
      echo "$$" > "$LOCK/pid"
    else
      return   # 让路;由外层 guard 稍后重试,不空等 30min
    fi
  fi

  # 防自杀自愈:若 plist 被改名成 .stopped(旧行为残留),恢复回来给 watchdog 重挂
  if [ ! -f "$PLIST" ] && [ -f "$PLIST.stopped" ]; then
    mv "$PLIST.stopped" "$PLIST" && log "restored plist from .stopped (self-heal)"
  fi

  log "=== round start (headless) ==="
  # env -i 白名单环境:隔离宿主泄漏的 ANTHROPIC_*/CLAUDE_*(会把 CLI 指到内部代理→401)
  env -i HOME="$HOME" USER="$USER" LOGNAME="$USER" SHELL=/bin/zsh \
    PATH="$PATH" LANG=en_US.UTF-8 TMPDIR="${TMPDIR:-/tmp}" \
    PARITY_DRIVE_HEADLESS=1 \
    claude -p "/parity-drive" --permission-mode bypassPermissions >> "$LOG" 2>&1 &
  local CPID=$!

  # (a) 硬顶 watchdog:55min 无论如何杀
  ( sleep "$ROUND_TIMEOUT" && kill -TERM "$CPID" 2>/dev/null; sleep 5; kill -KILL "$CPID" 2>/dev/null \
    && echo "[$(ts)] HARDCAP watchdog killed round after ${ROUND_TIMEOUT}s" >> "$LOG" ) &
  local WPID=$!

  # (b) 停滞 watchdog:log 或 transcript 只要有一个在动就算活;两者都 >STALL_TIMEOUT
  #     没动 = 这轮挂了(401 静默重试 / 死等一个卡死的子 agent / 死锁),杀掉让循环重起。
  #     用 transcript mtime 是关键:轮在同步等子 agent 时 log 会静默,但主 agent 的
  #     transcript 仍在长——所以正常等子 agent【不会】误杀,只有真挂了才杀。
  (
    while kill -0 "$CPID" 2>/dev/null; do
      sleep 45
      now=$(date +%s)
      lg=$(stat -f %m "$LOG" 2>/dev/null || echo 0)
      tx=$(ls -t "$PROJDIR"/*.jsonl 2>/dev/null | head -1)
      tm=0; [ -n "$tx" ] && tm=$(stat -f %m "$tx" 2>/dev/null || echo 0)
      last=$lg; [ "$tm" -gt "$last" ] && last=$tm
      idle=$(( now - last ))
      if [ "$idle" -ge "$STALL_TIMEOUT" ]; then
        echo "[$(ts)] STALL watchdog: no log/transcript activity for ${idle}s (>${STALL_TIMEOUT}s) — killing hung round" >> "$LOG"
        pkill -TERM -P "$CPID" 2>/dev/null; kill -TERM "$CPID" 2>/dev/null
        sleep 5; pkill -KILL -P "$CPID" 2>/dev/null; kill -KILL "$CPID" 2>/dev/null
        break
      fi
    done
  ) &
  local SPID=$!

  wait "$CPID"; local RC=$?
  kill "$WPID" "$SPID" 2>/dev/null
  rm -rf "$LOCK"
  log "=== round end rc=$RC ==="
}

# ---- 连续循环:一轮完立即下一轮,只隔 GUARD 秒。绝无长 sleep。 ----
trap 'rm -rf "$LOCK"' EXIT
while true; do
  run_round
  sleep "$GUARD"
done
