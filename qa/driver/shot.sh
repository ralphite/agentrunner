#!/usr/bin/env bash
# shot.sh — 下载 driver 截图(release asset)到本地,供 Read 工具看图。
# 用法: qa/driver/shot.sh <run_id> <shot-file-name.jpg> <out.jpg>
# shot 文件名来自 bot 回帖的 shots 数组,如 shot-003-mobile-rail.jpg
set -euo pipefail
RUNID="${1:?run_id}"; NAME="${2:?shot name}"; OUT="${3:?out path}"
curl -sfL -H "Authorization: Bearer ${GITHUB_TOKEN:?GITHUB_TOKEN required}" \
  -o "$OUT" \
  "https://github.com/ralphite/agentrunner/releases/download/qa-driver-$RUNID/$NAME"
echo "saved $OUT"
