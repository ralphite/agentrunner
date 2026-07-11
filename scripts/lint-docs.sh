#!/usr/bin/env bash
# 登记簿真实性闸门(PROCESS §五;2026-07-10 G29 复盘落地):
#  A. SPEC.md ✅ 行的验收锚必须点名 Test*/QA-n——档期名(S2/INC-n)不是锚。
#     存量弱锚在 scripts/spec-anchor-debt.txt 基线里,只减不增(G30 燃尽)。
#  B. 锚点名的 Test 必须真实存在于 *_test.go、QA-n 必须在 docs/QA.md
#     有条目(防幻影锚)。
#  C. GAPS.md 条目编号唯一(曾重号 G24/G25)。
set -euo pipefail
cd "$(dirname "$0")/.."

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fail=0

# --- A: ✅ 行弱锚 vs 基线(键 = 行首列全文,trim 后 byte-exact) ---
awk -F'|' '/^\|/ && NF>=6 && $3 ~ /✅/ {
  anchor=$5
  if (anchor !~ /Test[A-Za-z]/ && anchor !~ /QA-[0-9]/) {
    key=$2; gsub(/^[ \t]+|[ \t]+$/, "", key); print key
  }
}' docs/SPEC.md | sort >"$tmp/weak"

grep -v '^#' scripts/spec-anchor-debt.txt | grep -v '^[[:space:]]*$' | sort >"$tmp/debt"

if comm -23 "$tmp/weak" "$tmp/debt" | grep .; then
  echo "lint-docs: SPEC 新增 ✅ 行无可执行锚(Test*/QA-n)——上列各行要么补真锚,要么如实降级状态并挂 GAPS(PROCESS §五)" >&2
  fail=1
fi
if comm -13 "$tmp/weak" "$tmp/debt" | grep .; then
  echo "lint-docs: spec-anchor-debt.txt 上列基线行已不再弱锚(已修或行名已改)——从基线删除(只减不增)" >&2
  fail=1
fi

# --- B: 幻影锚 ---
awk -F'|' '/^\|/ && NF>=6 && $3 ~ /✅/ {print $5}' docs/SPEC.md \
  | grep -o 'Test[A-Za-z0-9_]*' | sort -u >"$tmp/tests" || true
while IFS= read -r t; do
  # 允许 TestFoo* 缩写:按前缀匹配函数名
  if ! git grep -q "func $t" -- '*_test.go'; then
    echo "lint-docs: SPEC 锚点名的测试不存在: $t" >&2
    fail=1
  fi
done <"$tmp/tests"

awk -F'|' '/^\|/ && NF>=6 && $3 ~ /✅/ {print $5}' docs/SPEC.md \
  | grep -o 'QA-[0-9][0-9]*' | sort -u >"$tmp/qas" || true
while IFS= read -r q; do
  if ! grep -q "$q" docs/QA.md; then
    echo "lint-docs: SPEC 锚点名的 QA 场景不在 QA.md: $q" >&2
    fail=1
  fi
done <"$tmp/qas"

# --- C: GAPS 编号唯一 ---
if grep -o '^\*\*G[0-9]*' docs/GAPS.md | sort | uniq -d | grep .; then
  echo "lint-docs: GAPS.md 条目编号重复(上列)——重号会让缺口互相遮蔽" >&2
  fail=1
fi

exit $fail
