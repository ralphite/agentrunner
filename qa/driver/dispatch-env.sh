#!/usr/bin/env bash
# dispatch-env.sh — 起一个 remote-qa-env 并等到 driver ready。
# 用法: qa/driver/dispatch-env.sh [minutes=60] [store_prefix]
# 需要: GITHUB_TOKEN(repo 权限)。输出最后一行: READY issue=<N> run=<ID>
# 注意: workflow 是 cancel-in-progress——会取消当前活着的 QA env,
#       被取消的 run 不保存 store cache。
set -euo pipefail
REPO="ralphite/agentrunner"
MINUTES="${1:-60}"
PREFIX="${2:-}"
API="https://api.github.com/repos/$REPO"
AUTH=(-H "Authorization: Bearer ${GITHUB_TOKEN:?GITHUB_TOKEN required}")

# 并发护栏(QA-0719 实测险情):workflow 是 cancel-in-progress,dispatch
# 会杀掉当前活着的 env——而那个 env 可能是并发 session 正在驱动的
# (driver issue 几十条评论的活跃工作)。有 in_progress run 时拒绝,
# 除非 FORCE=1(明知要抢占)。
LIVE=$(curl -sf "${AUTH[@]}" "$API/actions/workflows/remote-qa-env.yml/runs?status=in_progress&per_page=1" |
  python3 -c "import sys,json; rs=json.load(sys.stdin)['workflow_runs']; print(rs[0]['id'] if rs else '')")
if [ -n "$LIVE" ] && [ "${FORCE:-0}" != "1" ]; then
  echo "REFUSED: run $LIVE is in_progress — dispatch would cancel it (可能是并发 session 的活跃 env)." >&2
  echo "确认无人在用后 FORCE=1 重试,或直接复用它的 driver issue(注意 n 计数器归属)." >&2
  exit 2
fi

inputs="{\"minutes\": \"$MINUTES\"}"
[ -n "$PREFIX" ] && inputs="{\"minutes\": \"$MINUTES\", \"store_prefix\": \"$PREFIX\"}"
curl -sf "${AUTH[@]}" -X POST "$API/actions/workflows/remote-qa-env.yml/dispatches" \
  -d "{\"ref\":\"main\",\"inputs\":$inputs}" >/dev/null
echo "dispatched remote-qa-env (${MINUTES}min)"

sleep 6
RUNID=$(curl -sf "${AUTH[@]}" "$API/actions/workflows/remote-qa-env.yml/runs?per_page=1" |
  python3 -c "import sys,json; print(json.load(sys.stdin)['workflow_runs'][0]['id'])")
echo "run: $RUNID (build ~5-8min)"

for _ in $(seq 1 100); do
  ISSUE=$(curl -sf "${AUTH[@]}" "$API/issues?state=open&per_page=15" | python3 -c "
import sys,json
for it in json.load(sys.stdin):
    if 'qa-driver $RUNID' in (it.get('title') or ''):
        print(it['number']); break") || true
  if [ -n "${ISSUE:-}" ]; then
    READY=$(curl -sf "${AUTH[@]}" "$API/issues/$ISSUE/comments?per_page=5" | python3 -c "
import sys,json
for c in json.load(sys.stdin):
    if 'driver ready' in (c.get('body') or ''):
        print('ready'); break") || true
    if [ -n "${READY:-}" ]; then
      echo "READY issue=$ISSUE run=$RUNID"
      exit 0
    fi
  fi
  sleep 20
done
echo "TIMEOUT waiting for driver ready (run $RUNID)" >&2
exit 1
