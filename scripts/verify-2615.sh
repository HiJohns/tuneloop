#!/usr/bin/env bash
# #2615 证据生成器 — 丢失台账 IA 收敛（记录列表 + 单一登记入口 + 详情页恢复）。
#
# 用法：bash scripts/verify-2615.sh <r1..r5|gate>
#   r1 docs/evidence/2615/R1-api.txt          后端 List/Get（分页/状态/摘要/数组化 + 404 负例）
#   r2 docs/evidence/2615/R2-list.txt         列表页渲染（行信息 / 单一入口 / 无行内危险按钮 / 点击进详情）
#   r3 docs/evidence/2615/R3-register.txt     登记页渲染（SN 检索 / 校验 / 提交载荷 / 返回列表）
#   r4 docs/evidence/2615/R4-detail.txt       详情页渲染（字段 / 照片 / 恢复 / 已恢复隐藏按钮）
#   r5 docs/evidence/2615/R5-build-wiring.txt 跨端接线 + build:weapp + build:h5 + wxml 探针 + eslint
#   gate                                      定向回归（r1 + 三个渲染用例 + 接线静态断言），供 pre-ready P4
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`，次行 `GENERATED_AT_HEAD=<sha>`。
#
# 注意：合同命令形如 `bash scripts/verify-2615.sh r1 > docs/evidence/2615/R1-api.txt`。
# 因此脚本在把 LOG 复制成 artifact 之后**不得再向 stdout 写任何字节**（否则 shell 的
# fd 偏移会从 0 覆盖 provenance 首行），状态行一律走 stderr。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4|r5|gate) ;;
  *)
    echo "usage: $0 <r1..r5|gate>" >&2
    exit 2
    ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
mkdir -p "$REPO/tmp" "$REPO/tmp/gocache" "$REPO/tmp/gobuild"
ISSUE=2615
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
HEAD_SHA="$(git rev-parse HEAD)"
if [ -z "${FIX_COMMIT:-}" ]; then
  FIX_COMMIT="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
fi
FIX_SHA="$(git rev-parse "${FIX_COMMIT:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: fix commit not ancestor of HEAD" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
DIRTY=false
[ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR")" ] && DIRTY=true
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
HEAD_LINE="GENERATED_AT_HEAD=$HEAD_SHA"

# ---- toolchain ------------------------------------------------------------
export PATH="/usr/local/go/bin:${PATH}"
export GOCACHE="${GOCACHE:-$REPO/tmp/gocache}"
export TMPDIR="${TMPDIR:-$REPO/tmp/gobuild}"
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"

NVM_SH="${NVM_SH:-$HOME/.nvm/nvm.sh}"
require_node22() {
  if [ ! -s "$NVM_SH" ]; then
    echo "FATAL: node 22 toolchain unavailable ($NVM_SH)" >&2
    exit 2
  fi
  # shellcheck disable=SC1090
  source "$NVM_SH"
  nvm use 22 >/dev/null 2>&1 || { echo "FATAL: nvm use 22 failed" >&2; exit 2; }
}

require_tokens() { # log tok...
  local log="$1"; shift
  local missing="" tok
  for tok in "$@"; do
    grep -Fq -- "$tok" "$log" || missing="$missing [$tok]"
  done
  if [ -n "$missing" ]; then
    echo "ASSERT_FAIL: missing tokens:$missing" >&2
    return 1
  fi
}

LOG="$REPO/tmp/verify-2615-${MODE}.log"
: > "$LOG"
RC=0
ART=""
NAME=""

backend_test() { # -> log path via stdout
  ( cd "$REPO/backend" && go test ./handlers/ -run '^TestInstrumentLoss2615' -count=1 -v ) 2>&1
}

mobile_tokens() { # spec-file
  ( cd "$REPO/frontend-mobile" && npx vitest run --config vitest.config.js "$1" ) 2>&1
}

case "$MODE" in
  r1)
    NAME="R1-api.txt"
    { printf '%s\n%s\n' "$PROV" "$HEAD_LINE"
      echo "BACKEND_CMD=cd backend && go test ./handlers/ -run '^TestInstrumentLoss2615' -count=1 -v"
      backend_test
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'LOSS_LIST_TOTAL=3' 'LOSS_LIST_PAGE_LEN=2' \
      'LOSS_LIST_STATUS_OPEN=2' 'LOSS_LIST_STATUS_RESTORED=1' \
      'LOSS_LIST_BRIEF_SN=1' 'LOSS_LIST_PHOTOS_ARRAY=1' \
      'LOSS_GET_CODE=20000' 'LOSS_GET_BRIEF_SN=1' \
      'LOSS_GET_MISSING_CODE=40400' 'LOSS_GET_CROSS_TENANT_CODE=40400' \
      'LOSS_RESPONSE_KEEPS_REVERSED_FIELDS=1' \
      'LOSS_LEGACY_NO_PARAMS_ROWS=200' 'LOSS_LEGACY_NO_PARAMS_TOTAL=200' \
      'LOSS_LEGACY_PAGE_ONLY_SIZE=20' || RC=1
    grep -q '^--- PASS: TestInstrumentLoss2615' "$LOG" || { echo "ASSERT_FAIL: backend test did not PASS" >&2; RC=1; }
    ;;
  r2|r3|r4)
    require_node22
    case "$MODE" in
      r2) NAME="R2-list.txt";  REQ=('LOSS_LIST_ROWS=3' 'LOSS_LIST_SN_SHOWN=1' 'LOSS_LIST_TIME_SHOWN=1' 'LOSS_LIST_STATUS_BADGE=2' 'LOSS_LIST_DANGER_BUTTON_IN_ROW=0' 'LOSS_LIST_REGISTER_ENTRY=1' 'LOSS_LIST_ROW_NAV=1' 'LOSS_LIST_RENDER_FAIL=0' 'LOSS_LIST_COVER_SRC_ABSOLUTE=1' 'LOSS_LIST_COVER_RAW_INPUT=0') ;;
      r3) NAME="R3-register.txt"; REQ=('LOSS_REG_SELECTOR_QUERY=1' 'LOSS_REG_PARTY_RATIO_DEFAULT=1' 'LOSS_REG_VALIDATE_NO_INSTRUMENT=1' 'LOSS_REG_VALIDATE_DESC_REQUIRED=1' 'LOSS_REG_SUBMIT_PAYLOAD=1' 'LOSS_REG_SUBMIT_BACK=1' 'LOSS_REG_RENDER_FAIL=0') ;;
      r4) NAME="R4-detail.txt"; REQ=('LOSS_DETAIL_FIELDS=1' 'LOSS_DETAIL_PHOTOS=1' 'LOSS_DETAIL_PHOTO_SRC_ABSOLUTE=1' 'LOSS_DETAIL_PHOTO_RAW_KEY=0' 'LOSS_DETAIL_PHOTO_PREVIEW_URLS_ABSOLUTE=1' 'LOSS_DETAIL_RESTORE_PHOTO_SRC_ABSOLUTE=1' 'LOSS_DETAIL_RESTORE_PHOTO_RAW_KEY=0' 'LOSS_DETAIL_RESTORE_PHOTO_PREVIEW_URLS_ABSOLUTE=1' 'LOSS_DETAIL_OPEN_HAS_RESTORE=1' 'LOSS_DETAIL_RESTORED_HIDES_RESTORE=1' 'LOSS_DETAIL_RESTORE_PAYLOAD=1' 'LOSS_DETAIL_RENDER_FAIL=0') ;;
    esac
    { printf '%s\n%s\n' "$PROV" "$HEAD_LINE"
      echo "RENDER_CMD=cd frontend-mobile && npx vitest run --config vitest.config.js scripts/page-smoke/instrument-loss-2615.test.jsx"
      mobile_tokens scripts/page-smoke/instrument-loss-2615.test.jsx
    } > "$LOG" 2>&1
    require_tokens "$LOG" "${REQ[@]}" || RC=1
    grep -qE 'Tests +[0-9]+ passed' "$LOG" || { echo "ASSERT_FAIL: vitest did not report passing tests" >&2; RC=1; }
    ;;
  r5)
    NAME="R5-build-wiring.txt"
    require_node22
    WEAPP_PAGES=$(grep -cE "pages-weapp/instrument-loss-(register|detail)/index" "$REPO/frontend-mobile/src/app.config.ts")
    H5_PAGES=$(grep -cE "'pages/instrument-loss-(register|detail)/index'" "$REPO/frontend-mobile/src/app.config.ts")
    NAV_ENTRIES=$(grep -cE "match: '/instrument-loss-(register|detail)'" "$REPO/frontend-mobile/src/platform/navigation.js")
    H5_ROUTES=$(grep -cE 'path="/instrument-loss-(register|detail)"' "$REPO/frontend-mobile/src/App-H5.jsx")
    WEAPP_BUILD=0
    H5_BUILD=0
    { printf '%s\n%s\n' "$PROV" "$HEAD_LINE"
      echo "WEAPP_PAGES_REGISTERED=$WEAPP_PAGES"
      echo "H5_PAGES_REGISTERED=$H5_PAGES"
      echo "H5_ROUTES_REGISTERED=$H5_ROUTES"
      echo "NAV_MAP_ENTRIES=$NAV_ENTRIES"

      echo "WEAPP_BUILD_CMD=cd frontend-mobile && npm run build:weapp"
      ( cd "$REPO/frontend-mobile" && "$HOME/.config/opencode/scripts/heavy-run.sh" --timeout 1800 -- npm run build:weapp ) > "$REPO/tmp/verify-2615-weapp.log" 2>&1
      WEAPP_BUILD=$?
      echo "WEAPP_BUILD_OK=$([ "$WEAPP_BUILD" = "0" ] && echo 1 || echo 0)"
      tail -5 "$REPO/tmp/verify-2615-weapp.log"

      echo "H5_BUILD_CMD=cd frontend-mobile && npm run build:h5"
      ( cd "$REPO/frontend-mobile" && "$HOME/.config/opencode/scripts/heavy-run.sh" --timeout 1800 -- npm run build:h5 ) > "$REPO/tmp/verify-2615-h5.log" 2>&1
      H5_BUILD=$?
      echo "H5_BUILD_OK=$([ "$H5_BUILD" = "0" ] && echo 1 || echo 0)"
      tail -5 "$REPO/tmp/verify-2615-h5.log"

      # 产物探针：weapp 页面 wxml 不得含原生 HTML 交互/媒体标签（base.wxml 除外）
      NATIVE=0
      if [ -d "$REPO/frontend-mobile/dist-weapp" ]; then
        NATIVE=$(grep -rlE '<(div|span|img|button|input|textarea|p|select|a)\b' "$REPO/frontend-mobile/dist-weapp" --include='*.wxml' 2>/dev/null | grep -v '/base.wxml$' | wc -l | tr -d ' ')
        echo "WXML_SCAN_FILES:"
        grep -rlE '<(div|span|img|button|input|textarea|p|select|a)\b' "$REPO/frontend-mobile/dist-weapp" --include='*.wxml' 2>/dev/null | grep -v '/base.wxml$' | sed "s#$REPO/##" | head -20
      else
        echo "WXML_SCAN_FILES: dist-weapp missing"
        NATIVE=1
      fi
      echo "WXML_NATIVE_TAG=$NATIVE"

      # 三个新页 wxml 必须真实产出（编译可达，不只是注册字符串）
      PRODUCED=0
      for page in instrument-loss-manage instrument-loss-register instrument-loss-detail; do
        [ -f "$REPO/frontend-mobile/dist-weapp/pages-weapp/$page/index.wxml" ] && PRODUCED=$((PRODUCED + 1))
      done
      echo "WEAPP_PAGE_WXML_PRODUCED=$PRODUCED"

      echo "ESLINT_CMD=cd frontend-mobile && npx eslint src/pages/InstrumentLoss{Manage,Register,Detail}.jsx src/pages/instrument-loss-{register,detail}/index.tsx src/pages-weapp/instrument-loss-{register,detail}/index.tsx src/platform/navigation.js scripts/page-smoke/instrument-loss-2615.test.jsx"
      ESLINT_OUT="$REPO/tmp/verify-2615-eslint.log"
      ( cd "$REPO/frontend-mobile" && npx eslint \
          "src/pages/InstrumentLossManage.jsx" \
          "src/pages/InstrumentLossRegister.jsx" \
          "src/pages/InstrumentLossDetail.jsx" \
          "src/pages/instrument-loss-register/index.tsx" \
          "src/pages/instrument-loss-detail/index.tsx" \
          "src/pages-weapp/instrument-loss-register/index.tsx" \
          "src/pages-weapp/instrument-loss-detail/index.tsx" \
          "src/platform/navigation.js" \
          "scripts/page-smoke/instrument-loss-2615.test.jsx" ) > "$ESLINT_OUT" 2>&1
      ESLINT_RC=$?
      tail -20 "$ESLINT_OUT"
      NOUNDEF=$(grep -c 'no-undef' "$ESLINT_OUT")
      echo "ESLINT_EXIT=$ESLINT_RC"
      echo "ESLINT_NOUNDEF=$NOUNDEF"
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'WEAPP_PAGES_REGISTERED=2' 'H5_PAGES_REGISTERED=2' 'H5_ROUTES_REGISTERED=2' \
      'NAV_MAP_ENTRIES=2' 'WEAPP_BUILD_OK=1' 'H5_BUILD_OK=1' 'WXML_NATIVE_TAG=0' \
      'WEAPP_PAGE_WXML_PRODUCED=3' 'ESLINT_NOUNDEF=0' || RC=1
    ;;
  gate)
    NAME="GATE-directed.txt"
    require_node22
    { printf '%s\n%s\n' "$PROV" "$HEAD_LINE"
      echo "GATE_MODE=directed (backend integration + render suite + wiring static)"
      echo "BACKEND_CMD=cd backend && go test ./handlers/ -run '^TestInstrumentLoss2615' -count=1 -v"
      backend_test
      echo "RENDER_CMD=cd frontend-mobile && npx vitest run --config vitest.config.js scripts/page-smoke/instrument-loss-2615.test.jsx"
      mobile_tokens scripts/page-smoke/instrument-loss-2615.test.jsx
      echo "WIRING:"
      echo "WEAPP_PAGES_REGISTERED=$(grep -cE "pages-weapp/instrument-loss-(register|detail)/index" "$REPO/frontend-mobile/src/app.config.ts")"
      echo "H5_PAGES_REGISTERED=$(grep -cE "'pages/instrument-loss-(register|detail)/index'" "$REPO/frontend-mobile/src/app.config.ts")"
      echo "NAV_MAP_ENTRIES=$(grep -cE "match: '/instrument-loss-(register|detail)'" "$REPO/frontend-mobile/src/platform/navigation.js")"
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'LOSS_LIST_TOTAL=3' 'LOSS_GET_CROSS_TENANT_CODE=40400' \
      'LOSS_RESPONSE_KEEPS_REVERSED_FIELDS=1' \
      'LOSS_LEGACY_NO_PARAMS_ROWS=200' 'LOSS_LEGACY_PAGE_ONLY_SIZE=20' \
      'LOSS_LIST_DANGER_BUTTON_IN_ROW=0' 'LOSS_LIST_COVER_SRC_ABSOLUTE=1' \
      'LOSS_REG_SUBMIT_PAYLOAD=1' \
      'LOSS_DETAIL_PHOTO_SRC_ABSOLUTE=1' 'LOSS_DETAIL_RESTORE_PHOTO_SRC_ABSOLUTE=1' \
      'LOSS_DETAIL_RESTORED_HIDES_RESTORE=1' 'WEAPP_PAGES_REGISTERED=2' \
      'H5_PAGES_REGISTERED=2' 'NAV_MAP_ENTRIES=2' || RC=1
    grep -qE 'Tests +[0-9]+ passed' "$LOG" || { echo "ASSERT_FAIL: render suite did not pass" >&2; RC=1; }
    ;;
esac

if [ -n "$NAME" ]; then
  ART="$OUT_DIR/$NAME"
fi

if [ "$RC" != "0" ]; then
  echo "RESULT: FAIL ($MODE)" >> "$LOG"
else
  echo "RESULT: PASS ($MODE)" >> "$LOG"
fi
# 每个渲染行只保留自己的 token（R2/R3/R4 共用同一个 vitest 文件，产物不得互相复用
# 其它行的断言结果；断言校验已在上面针对完整 LOG 执行）。
ROW_PREFIX=""
case "$MODE" in
  r2) ROW_PREFIX="LOSS_LIST_" ;;
  r3) ROW_PREFIX="LOSS_REG_" ;;
  r4) ROW_PREFIX="LOSS_DETAIL_" ;;
esac
if [ -n "$ART" ] && [ -n "$ROW_PREFIX" ]; then
  awk -v p="$ROW_PREFIX" '
    NR <= 3 { print; next }
    index($0, p) > 0 { print; next }
    /^RESULT: / { print; next }
    /Tests +[0-9]+ passed/ { print; next }
  ' "$LOG" > "$ART.row"
  mv "$ART.row" "$ART"
  echo "ARTIFACT=$ART (row-scoped: $ROW_PREFIX)" >&2
elif [ -n "$ART" ]; then
  cp "$LOG" "$ART"
  echo "ARTIFACT=$ART" >&2
fi
tail -1 "$LOG" >&2
if [ "$RC" != "0" ]; then
  exit 1
fi
exit 0
