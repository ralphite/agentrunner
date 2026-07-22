#!/usr/bin/env bash
# QA-69: webui 双锚真浏览器验收(G30 收尾,audit-0717 F1)。
#  A. 用户消息折叠(INC-36):>10 渲染行的 user 消息在真布局下钳高
#     (.utext.clamped)+ Show more/less 往返;
#  B. composer progressive-disclosure(INC-19/23/38/40/65/95):Add 菜单
#     精确四个 root，Automation/Agent 子页承载 Loop/Best-of-N/spec。
# 无需 provider key:消息在 provider 调用前已入 journal,turn 失败不
# 影响时间线渲染——断言只钉 runtime 红线,不钉模型输出。
set -euo pipefail
cd "$(dirname "$0")/.."

RUNDIR="qa/runs/$(date +%F)-QA69"
mkdir -p "$RUNDIR"
ADDR="127.0.0.1:8797"

echo "== build =="
go build -o bin/ar ./cmd/agentrunner
(cd webui/frontend && { [ -d node_modules ] || npm ci; } && npm run build)
(cd webui && go build -o ../bin/arwebui .)

echo "== daemon (scripted provider — this QA needs no model, only rendering) =="
# 清掉本 QA 自己此前失败运行残留的 daemon(仅限 bin/ar 路径,不动别人)。
pkill -f "$PWD/bin/ar daemon" 2>/dev/null || pkill -f "bin/ar daemon" 2>/dev/null || true
sleep 0.5
cat > "$RUNDIR/fixture.yaml" <<'YAML'
steps:
  - respond:
      - text: "收到,已读完整段。"
      - finish: end_turn
YAML
AGENTRUNNER_SCRIPTED_FIXTURE="$PWD/$RUNDIR/fixture.yaml" ./bin/ar daemon > "$RUNDIR/daemon.log" 2>&1 &
DPID=$!
SOCK="${XDG_DATA_HOME:-$HOME/.local/share}/agentrunner/daemon.sock"
for i in $(seq 1 100); do [ -S "$SOCK" ] && break; sleep 0.1; done
[ -S "$SOCK" ] || { echo "daemon socket never appeared" >&2; cat "$RUNDIR/daemon.log" >&2; exit 1; }

echo "== seed session (long user message; provider failure expected) =="
SPEC="$RUNDIR/base.yaml"
cat > "$SPEC" <<'YAML'
name: qa69
model: { provider: scripted, id: fixture, max_tokens: 512 }
system_prompt: 简洁作答。
permissions:
  - { action: allow }
YAML
MSG="QA-69 折叠验收基准行,这一行要足够长以便在窄列下也稳定产生换行与渲染高度。"
BODY=$(for i in $(seq 1 24); do echo "$MSG line-$i"; done)
./bin/ar new --detach "$SPEC" "$BODY" > "$RUNDIR/ar-new.log" 2>&1 || true
SID=$(head -1 "$RUNDIR/ar-new.log")
for i in $(seq 1 50); do
  ./bin/ar sessions --json 2>/dev/null | grep -q "$SID" && break
  sleep 0.2
done
./bin/ar sessions --json | grep -q "$SID" || { echo "seeded session never appeared in store" >&2; cat "$RUNDIR/daemon.log" >&2; exit 1; }
echo "session: $SID" | tee "$RUNDIR/session.txt"

echo "== start webui =="
./bin/arwebui -ar "$PWD/bin/ar" -addr "$ADDR" > "$RUNDIR/webui.log" 2>&1 &
WEBUI_PID=$!
trap 'kill $WEBUI_PID $DPID 2>/dev/null || true' EXIT
for i in $(seq 1 50); do
  curl -fsS "http://$ADDR/api/health" > /dev/null 2>&1 && break
  sleep 0.2
done

echo "== playwright =="
PWDIR=$(mktemp -d)
(cd "$PWDIR" && npm init -y > /dev/null 2>&1 && npm i --no-audit --no-fund playwright > /dev/null 2>&1)
NODE_PATH="$PWDIR/node_modules" SID="$SID" ADDR="$ADDR" RUNDIR="$PWD/$RUNDIR" node qa/qa69-assert.mjs
echo "QA-69 PASS — evidence in $RUNDIR"
