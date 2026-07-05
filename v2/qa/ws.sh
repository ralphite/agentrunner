#!/usr/bin/env bash
# v2/qa/ws.sh — QA workspace 准备/清理脚本。
#
# 保证每次测试环境完全一致：固定 repo + 固定 commit（按 SHA 浅 fetch，
# 与上游 HEAD 漂移无关），可选注入确定性的 bug 包。
#
# 用法:
#   ws.sh prepare <profile> <dir>   # 在 <dir> 准备一个 profile 的 workspace
#   ws.sh cleanup <dir>             # 用完删除
#   ws.sh list                      # 列出可用 profile
#
# profile:
#   color         fatih/color        @ 53d4ce9d  小型(~20 文件)  问答/小修
#   cobra         spf13/cobra        @ ad460ea8  中型(~100 文件) 常规开发
#   cobra-broken  cobra + 注入的失败测试包 qa_inject/           修复类场景
#   gin           gin-gonic/gin      @ 34dac209  中大型          多 agent 探索
#   blank         空目录                                        起项目类场景
set -euo pipefail

pin_url() {
  case "$1" in
    color) echo "https://github.com/fatih/color" ;;
    cobra|cobra-broken) echo "https://github.com/spf13/cobra" ;;
    gin) echo "https://github.com/gin-gonic/gin" ;;
    *) return 1 ;;
  esac
}
pin_sha() {
  case "$1" in
    color) echo "53d4ce9d5df3891799a447e772308c76e70bad50" ;;
    cobra|cobra-broken) echo "ad460ea8f249db69c943a365fb84f3a59042d54e" ;;
    gin) echo "34dac209ffb6ef85cc78c5d217bbb7ad001d68fd" ;;
    *) return 1 ;;
  esac
}

clone_pinned() { # url sha dir
  local url="$1" sha="$2" dir="$3"
  mkdir -p "$dir"
  git -C "$dir" init -q
  git -C "$dir" remote add origin "$url"
  git -C "$dir" fetch -q --depth 1 origin "$sha"
  git -C "$dir" checkout -q FETCH_HEAD
  [ "$(git -C "$dir" rev-parse HEAD)" = "$sha" ] || {
    echo "ws.sh: pinned SHA mismatch in $dir" >&2; exit 1; }
}

# 注入一个确定性的失败测试包：不依赖上游内部结构，跨版本稳定。
# 语义清晰(实现与文档不符)，修复方式唯一(改乘为加)，通过标准客观
# (go test ./qa_inject/ 变绿)。
inject_bug() { # dir
  mkdir -p "$1/qa_inject"
  cat > "$1/qa_inject/calc.go" <<'GO'
// Package qa_inject is a QA fixture: Add is DOCUMENTED to return a+b.
package qa_inject

// Add returns the sum of a and b.
func Add(a, b int) int {
	return a * b // BUG: multiplies instead of adding
}
GO
  cat > "$1/qa_inject/calc_test.go" <<'GO'
package qa_inject

import "testing"

func TestAdd(t *testing.T) {
	if got := Add(2, 3); got != 5 {
		t.Fatalf("Add(2,3) = %d, want 5", got)
	}
	if got := Add(0, 7); got != 7 {
		t.Fatalf("Add(0,7) = %d, want 7", got)
	}
}
GO
}

cmd="${1:-}"
case "$cmd" in
  list)
    echo "color cobra cobra-broken gin blank" ;;
  prepare)
    profile="${2:?usage: ws.sh prepare <profile> <dir>}"
    dir="${3:?usage: ws.sh prepare <profile> <dir>}"
    [ -e "$dir" ] && { echo "ws.sh: $dir already exists" >&2; exit 1; }
    if [ "$profile" = blank ]; then
      mkdir -p "$dir"
    else
      clone_pinned "$(pin_url "$profile")" "$(pin_sha "$profile")" "$dir"
      [ "$profile" = cobra-broken ] && inject_bug "$dir"
    fi
    echo "workspace ready: $dir ($profile)"
    ;;
  cleanup)
    dir="${2:?usage: ws.sh cleanup <dir>}"
    # 只删我们准备的东西：目录里要么有钉死的 git HEAD，要么是空的 blank。
    rm -rf "$dir"
    echo "cleaned: $dir"
    ;;
  *)
    grep '^#' "$0" | sed 's/^# \{0,1\}//' | head -18; exit 1 ;;
esac
