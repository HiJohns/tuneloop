#!/usr/bin/env bash
# scripts/verify-2616.sh — #2616（weapp 购物车：勾选框对比度 + 头部胶囊避让）证据生成器。
#
# 用法：
#   bash scripts/verify-2616.sh r1   > docs/evidence/2616/R1-cart-checkbox.txt
#   bash scripts/verify-2616.sh r2   > docs/evidence/2616/R2-cart-header.txt
#   bash scripts/verify-2616.sh r3   > docs/evidence/2616/R3-build-lint.txt
#   bash scripts/verify-2616.sh gate            # pre-ready 的 affected-cmd（只跑本单渲染用例 + 定向 eslint）
#
# 输出为完整 artifact（首两行 provenance，其后机器可判定 token）；任一行断言不成立以非 0 退出。
# 重命令（vitest / build:weapp / build:h5 / eslint）经 scripts/heavy-run.sh 取锁（D10）。
set -uo pipefail

ROW="${1:-}"
REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2616
MOBILE="$REPO/frontend-mobile"
HEAVY_RUN="${OPENCODE_HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"

# node 22（Taro v4 在 node 24 下不兼容）；缺少 nvm/node 22 时显式 exit 2，绝不静默通过。
if [ -s "$HOME/.nvm/nvm.sh" ]; then
  # shellcheck disable=SC1090
  . "$HOME/.nvm/nvm.sh" >/dev/null 2>&1
  nvm use 22 >/dev/null 2>&1 || true
fi
if ! command -v node >/dev/null 2>&1; then
  echo "ERROR:node-unavailable" >&2
  exit 2
fi
NODE_MAJOR="$(node -p 'process.versions.node.split(".")[0]' 2>/dev/null || echo 0)"
if [ "$NODE_MAJOR" != "22" ]; then
  echo "ERROR:node-22-required (found $(node -v 2>/dev/null))" >&2
  exit 2
fi

HEAD_SHA="$(git rev-parse HEAD)"
FIX_REF="${FIX_COMMIT:-$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)}"
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_COMMIT not ancestor of HEAD" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short=8 "$FIX_SHA")"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)docs/evidence/$ISSUE" ':(exclude)tmp')" ]; then
  DIRTY=true
fi

heavy() {
  if [ -x "$HEAVY_RUN" ]; then
    "$HEAVY_RUN" --timeout 3600 -- "$@"
  else
    "$@"
  fi
}

emit() { printf '%s\n' "$*"; }
provenance() {
  emit "# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
  emit "GENERATED_AT_HEAD=$HEAD_SHA"
}

TMP_DIR="$REPO/tmp"; mkdir -p "$TMP_DIR"

usage() { echo "usage: $0 <r1|r2|r3|gate>" >&2; exit 2; }

R1_TEST="scripts/page-smoke/cart-checkbox-2616.test.jsx"
R2_TEST="scripts/page-smoke/cart-header-2616.test.jsx"

run_vitest() { # test-file log-file
  ( cd "$MOBILE" && heavy ./node_modules/.bin/vitest run --config vitest.config.js "$1" ) > "$2" 2>&1
}

run_eslint() { # log-file ; prints no-undef count
  local log="$1"
  ( cd "$MOBILE" && heavy ./node_modules/.bin/eslint src/pages/Cart.jsx "$R1_TEST" "$R2_TEST" ) > "$log" 2>&1
  local rc=$?
  # eslint 可能因 warning 非 0；只统计 no-undef（阻断级）
  grep -c 'no-undef' "$log" 2>/dev/null | tr -d ' '
  return $rc
}

assert_row() { # log expect... ; emits tokens present in log, asserts required ones present
  local log="$1"; shift
  local rc=0 t
  for t in "$@"; do
    if grep -Fq -- "$t" "$log"; then emit "$t"; else emit "MISSING:$t"; rc=1; fi
  done
  return $rc
}

case "$ROW" in
r1)
  LOG="$TMP_DIR/verify-$ISSUE-r1.log"
  run_vitest "$R1_TEST" "$LOG"; VRC=$?
  RC=0
  [ "$VRC" = "0" ] || RC=1
  provenance
  emit "VITEST_CMD=vitest run --config vitest.config.js $R1_TEST"
  emit "VITEST_EXIT=$VRC"
  assert_row "$LOG" \
    'CART_CHECKBOX_SELECTED_FILL=1' \
    'CART_CHECKBOX_UNSELECTED_CONTRAST=1' \
    'CART_CHECKBOX_UNSELECTED_WHITE_BG=1' \
    'CART_CHECKBOX_DISABLED_DISTINCT=1' \
    'CART_CHECKBOX_DISABLED_NOT_SELECTABLE=1' \
    'CART_CHECKBOX_TAP_SIZE=36' \
    'CART_CHECKBOX_RENDER_FAIL=0' || RC=1
  # 负例：未选中态不得再出现旧灰阶 #d1d5db（实现里只有禁用/主色/新灰阶）
  if grep -Eq "#d1d5db" "$MOBILE/src/pages/Cart.jsx"; then
    emit "CART_CHECKBOX_OLD_GRAY_PRESENT=1"; RC=1
  else
    emit "CART_CHECKBOX_OLD_GRAY_PRESENT=0"
  fi
  exit "$RC"
  ;;
r2)
  LOG="$TMP_DIR/verify-$ISSUE-r2.log"
  run_vitest "$R2_TEST" "$LOG"; VRC=$?
  RC=0
  [ "$VRC" = "0" ] || RC=1
  provenance
  emit "VITEST_CMD=vitest run --config vitest.config.js $R2_TEST"
  emit "VITEST_EXIT=$VRC"
  assert_row "$LOG" \
    'CART_HEADER_WEAPP_PADDING_RIGHT=83' \
    'CART_HEADER_WEAPP_PADDING_TOP=28' \
    'CART_HEADER_H5_PADDING_RIGHT=16' \
    'CART_HEADER_H5_PADDING_TOP=12' \
    'CART_HEADER_FALLBACK_PADDING_RIGHT=96' \
    'CART_HEADER_DELETE_TEXT_FULL=1' \
    'CART_HEADER_DELETE_NO_SHRINK=1' \
    'CART_HEADER_TITLE_YIELDS=1' \
    'CART_HEADER_RENDER_FAIL=0' || RC=1
  exit "$RC"
  ;;
r3)
  RC=0
  WEAPP_LOG="$TMP_DIR/verify-$ISSUE-r3-weapp.log"
  H5_LOG="$TMP_DIR/verify-$ISSUE-r3-h5.log"
  ( cd "$MOBILE" && heavy npm run build:weapp ) > "$WEAPP_LOG" 2>&1; WRC=$?
  ( cd "$MOBILE" && heavy npm run build:h5 ) > "$H5_LOG" 2>&1; HRC=$?

  provenance
  emit "NODE_VERSION=$(node -v)"
  emit "WEAPP_BUILD_CMD=cd frontend-mobile && npm run build:weapp"
  emit "WEAPP_BUILD_EXIT=$WRC"
  [ "$WRC" = "0" ] && emit "WEAPP_BUILD_OK=1" || { emit "WEAPP_BUILD_OK=0"; RC=1; }
  emit "H5_BUILD_CMD=cd frontend-mobile && npm run build:h5"
  emit "H5_BUILD_EXIT=$HRC"
  [ "$HRC" = "0" ] && emit "H5_BUILD_OK=1" || { emit "H5_BUILD_OK=0"; RC=1; }

  CART_WXML_DIR="$MOBILE/dist-weapp/pages-weapp/cart"
  NATIVE_HITS="$(grep -rlE '<(div|span|img|button|input|textarea|p|select|a)\b' "$CART_WXML_DIR" --include='*.wxml' 2>/dev/null | grep -v '/base\.wxml$' | wc -l | tr -d ' ')"
  emit "WXML_CART_DIR=$CART_WXML_DIR"
  emit "WXML_CART_NATIVE_TAG=${NATIVE_HITS:-1}"
  [ "${NATIVE_HITS:-1}" = "0" ] || RC=1

  MARKER="$(grep -rli '#71717A' "$CART_WXML_DIR" 2>/dev/null | wc -l | tr -d ' ')"
  [ "${MARKER:-0}" -gt 0 ] || MARKER="$(grep -rli '#71717a' "$MOBILE/dist-weapp" 2>/dev/null | wc -l | tr -d ' ')"
  emit "WEAPP_BUNDLE_MARKER=$([ "${MARKER:-0}" -gt 0 ] && echo 1 || echo 0)"
  [ "${MARKER:-0}" -gt 0 ] || RC=1

  ESLINT_LOG="$TMP_DIR/verify-$ISSUE-r3-eslint.log"
  NOUNDEF="$(run_eslint "$ESLINT_LOG")"
  emit "ESLINT_CMD=cd frontend-mobile && eslint src/pages/Cart.jsx scripts/page-smoke/cart-*-2616.test.jsx"
  emit "ESLINT_NOUNDEF=${NOUNDEF:-0}"
  [ "${NOUNDEF:-0}" = "0" ] || RC=1
  exit "$RC"
  ;;
gate)
  # pre-ready 的 affected-cmd：只跑本单渲染用例 + 定向 eslint（D10，不做全量构建）
  RC=0
  L1="$TMP_DIR/verify-$ISSUE-gate-r1.log"
  L2="$TMP_DIR/verify-$ISSUE-gate-r2.log"
  run_vitest "$R1_TEST" "$L1"; [ "$?" = "0" ] || RC=1
  run_vitest "$R2_TEST" "$L2"; [ "$?" = "0" ] || RC=1
  NOUNDEF="$(run_eslint "$TMP_DIR/verify-$ISSUE-gate-eslint.log")"
  echo "GATE_VITEST_R1=$([ "$RC" = "0" ] && echo OK || echo FAIL)"
  echo "GATE_ESLINT_NOUNDEF=${NOUNDEF:-0}"
  [ "${NOUNDEF:-0}" = "0" ] || RC=1
  exit "$RC"
  ;;
*)
  usage
  ;;
esac
