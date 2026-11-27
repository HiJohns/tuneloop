#!/usr/bin/env bash
# #2598 verification harness — one subcommand per Evidence Contract row.
#
#   bash scripts/verify-2598.sh r1   # 上游失败分类器（四类 + 状态提取 + 文本保持）
#   bash scripts/verify-2598.sh r2   # 中转建组织持久失败分流（上游 400 / 403 → 500/50015）
#   bash scripts/verify-2598.sh r3   # 中转建组织瞬时失败不变（上游 500 / 建组织传输层失败 → 502/50200）
#   bash scripts/verify-2598.sh r4   # 微信绑定 / 合并 / 二维码合并三路径四类分流
#   bash scripts/verify-2598.sh r5   # 既有断言翻转与强化（merge 400 / bind 409 可行动）
#   bash scripts/verify-2598.sh r6   # 双端码表 50015 与后端常量一致性
#   bash scripts/verify-2598.sh r7   # 文档同步（新码 + 三元组口径 + 页面失败文案）
#   bash scripts/verify-2598.sh r8   # 构建 / gofmt / eslint（no-undef）
#
# Capture a row into its artifact:
#   bash scripts/verify-2598.sh r1 > docs/evidence/2598/R1-classifier.txt
#
# Every artifact starts with the provenance header consumed by
# scripts/evidence-artifacts.sh (P14):
#   # build=1.0.<short-sha> # commit=<full-sha> # dirty=<true|false>
#   GENERATED_AT_HEAD=<full-sha>
# The anchor is the issue's fix commit (FIX_COMMIT overrides), never HEAD after
# the evidence commit, so a later re-run keeps pointing at the same source.
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r2|r3|r4|r5|r6|r7|r8|gate) ;;
  *) echo "usage: $0 r1|r2|r3|r4|r5|r6|r7|r8|gate" >&2; exit 2 ;;
esac

ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not a git repository" >&2; exit 2; }
cd "$ROOT" || exit 2

if ! command -v go >/dev/null 2>&1 && [ -x /usr/local/go/bin/go ]; then
  export PATH="/usr/local/go/bin:$PATH"
fi

# Backend integration tests use the isolated test database created/cloned by
# backend/handlers/main_test.go (see docs/ops/test-db-lifecycle.md).
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test}"
export TMPDIR="${TMPDIR:-$ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"

FIX_SHA="${FIX_COMMIT:-$(git log --format=%H --grep='^fix(issue-2598)' -1 2>/dev/null)}"
ANCHOR_SHA="${FIX_SHA:-$(git rev-parse HEAD)}"
SHORT_SHA="$(git rev-parse --short "$ANCHOR_SHA")"
BUILD="1.0.${SHORT_SHA}"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence' ':!tmp')" ]; then
  DIRTY=true
fi
echo "# build=${BUILD} # commit=${ANCHOR_SHA} # dirty=${DIRTY}"
echo "GENERATED_AT_HEAD=$(git rev-parse HEAD)"

fail() {
  echo "FAIL(${ROW}): $*" >&2
  exit 1
}

HEAVY="$HOME/.config/opencode/scripts/heavy-run.sh"
heavy() { bash "$HEAVY" -- "$@"; }

TOKEN_RE='^(CLASSIFIER_|HTTPERR_|TRANSIT_|WXBIND_|WX_|WXMERGE_|MERGEBIND_|PERSISTENT_|LEGACY_|PC_MAP_|MOBILE_MAP_|MAP_TEXT_|CONFIG_MSG_|PARITY=|NODE_TEST_|API_CODE_|API_TRIPLE_|UI_WXBIND_|UI_TRANSIT_|GO_BUILD_|GOFMT_|ESLINT_|--- (PASS|FAIL))'

go_test_row() { # pkg test-pattern...
  local pkg="$1" pattern="$2" out rc
  out="$(mktemp)"
  ( cd backend && heavy go test "$pkg" -run "$pattern" -count=1 -v ) > "$out" 2>&1
  rc=$?
  grep -E "$TOKEN_RE" "$out" || true
  if [ "$rc" -ne 0 ]; then
    echo "--- go test output (tail) ---"
    tail -40 "$out"
    rm -f "$out"
    fail "go test ${pkg} -run ${pattern} exit=${rc}"
  fi
  rm -f "$out"
}

run_row() { # pkg pattern expected-token...
  local pkg="$1" pattern="$2"; shift 2
  local out; out="$(mktemp)"
  go_test_row "$pkg" "$pattern" > "$out" 2>&1
  cat "$out"
  local tok
  for tok in "$@"; do
    grep -qx -- "$tok" "$out" || fail "missing expected token '$tok'"
  done
  rm -f "$out"
}

map_text() { # file code
  sed -n "s/^ *$2: '\([^']*\)'.*/\1/p" "$1" | head -1
}

case "$ROW" in
  r1)
    echo "== R1 上游失败分类器：四类 + 状态提取 + Error() 文本保持 =="
    run_row ./services 'TestClassifyIAMFailure2598' \
      "CLASSIFIER_CASES=13" \
      "CLASSIFIER_MISMATCH=0" \
      "HTTPERR_TEXT_KEPT=1"
    echo "R1_OK"
    ;;

  r2)
    echo "== R2 中转建组织持久失败分流（上游 400 / 403 → 500/50015） =="
    run_row ./handlers 'TestTransitSiteCreate2598_PersistentFailure' \
      "TRANSIT_REJECTED_STATUS=500" \
      "TRANSIT_REJECTED_CODE=50015" \
      "TRANSIT_REJECTED_MSG_ACTIONABLE=1" \
      "TRANSIT_REJECTED_MSG_NO_RETRY=0" \
      "TRANSIT_REJECTED_SITES_CREATED=0"
    echo "R2_OK"
    ;;

  r3)
    echo "== R3 中转建组织瞬时失败保持不变（上游 500 / 传输层 → 502/50200） =="
    run_row ./handlers 'TestTransitSiteCreate2598_TransientFailure' \
      "TRANSIT_TRANSIENT_STATUS=502" \
      "TRANSIT_TRANSIENT_CODE=50200" \
      "TRANSIT_TRANSIENT_MSG_RETRY=1" \
      "TRANSIT_TRANSIENT_NOT_CONFIG=1"
    echo "R3_OK"
    ;;

  r4)
    echo "== R4 微信绑定 / 合并 / 二维码合并三路径四类分流 =="
    run_row ./handlers 'TestWxFailure2598' \
      "WXBIND_400_CODE=40002" \
      "WXBIND_403_CODE=50015" \
      "WXBIND_409_CODE=40900" \
      "WX_TRANSIENT_STATUS=502" \
      "WX_TRANSIENT_CODE=50200" \
      "WXMERGE_400_CODE=40002" \
      "MERGEBIND_400_CODE=40002" \
      "PERSISTENT_NO_RETRY_HINT=0" \
      "WXBIND_NEEDS_MERGE=1"
    echo "R4_OK"
    ;;

  r5)
    echo "== R5 既有断言翻转与强化（merge 400 → 400/40002；bind 409 可行动） =="
    OUT="$(mktemp)"
    ( cd backend && heavy go test ./handlers -count=1 -v \
        -run 'TestWxMergeCurrentUser_IAMFailureMapped_2228|TestWxBindCurrentUser' ) > "$OUT" 2>&1
    RC=$?
    grep -E "$TOKEN_RE" "$OUT" || true
    FAIL_COUNT="$(grep -c '^--- FAIL' "$OUT" || true)"
    echo "LEGACY_TESTS_FAIL=${FAIL_COUNT:-0}"
    if [ "$RC" -ne 0 ] || [ "${FAIL_COUNT:-1}" != "0" ]; then
      tail -40 "$OUT"
      rm -f "$OUT"
      fail "legacy assertion suite failed (rc=${RC}, fails=${FAIL_COUNT:-?})"
    fi
    rm -f "$OUT"
    echo "R5_OK"
    ;;

  r6)
    echo "== R6 双端码表 50015 与后端常量一致性 =="
    PC_FILE="frontend-pc/src/services/errorMessages.js"
    MOBILE_FILE="frontend-mobile/src/services/errorMessages.js"
    BACKEND_FILE="backend/handlers/iam_failure.go"
    PC_TEXT="$(map_text "$PC_FILE" 50015)"
    MOBILE_TEXT="$(map_text "$MOBILE_FILE" 50015)"
    CONFIG_TEXT="$(sed -n 's/.*iamConfigErrorMessage *= *"\([^"]*\)".*/\1/p' "$BACKEND_FILE" | head -1)"

    PC_MAP_50015=0; [ -n "$PC_TEXT" ] && PC_MAP_50015=1
    MOBILE_MAP_50015=0; [ -n "$MOBILE_TEXT" ] && MOBILE_MAP_50015=1
    MAP_TEXT_MATCH=0
    if [ -n "$PC_TEXT" ] && [ "$PC_TEXT" = "$MOBILE_TEXT" ]; then MAP_TEXT_MATCH=1; fi
    CONFIG_MSG_DRIFT=1
    if [ -n "$CONFIG_TEXT" ] && [ "$CONFIG_TEXT" = "$PC_TEXT" ] && [ "$CONFIG_TEXT" = "$MOBILE_TEXT" ]; then
      CONFIG_MSG_DRIFT=0
    fi

    TEST_OUT="$(mktemp)"
    node --test frontend-pc/tests/errorText-2598.test.mjs > "$TEST_OUT" 2>&1
    TEST_RC=$?
    NODE_TEST_FAIL="$(sed -n 's/^# fail \([0-9]*\)$/\1/p' "$TEST_OUT" | head -1)"
    NODE_TEST_FAIL="${NODE_TEST_FAIL:-1}"
    if [ "$TEST_RC" -ne 0 ]; then tail -25 "$TEST_OUT"; fi
    rm -f "$TEST_OUT"

    PARITY_OUT="$(mktemp)"
    node scripts/check-error-message-parity-2480.mjs > "$PARITY_OUT" 2>&1
    PARITY_RC=$?
    PARITY="$(sed -n 's/^PARITY=\([01]\)$/\1/p' "$PARITY_OUT" | head -1)"
    PARITY="${PARITY:-0}"
    if [ "$PARITY_RC" -ne 0 ]; then tail -25 "$PARITY_OUT"; fi
    rm -f "$PARITY_OUT"

    echo "PC_MAP_50015=${PC_MAP_50015}"
    echo "MOBILE_MAP_50015=${MOBILE_MAP_50015}"
    echo "MAP_TEXT_MATCH=${MAP_TEXT_MATCH}"
    echo "CONFIG_MSG_DRIFT=${CONFIG_MSG_DRIFT}"
    echo "PARITY=${PARITY}"
    echo "NODE_TEST_FAIL=${NODE_TEST_FAIL}"

    [ "$PC_MAP_50015" = "1" ] || fail "$PC_FILE 缺 50015"
    [ "$MOBILE_MAP_50015" = "1" ] || fail "$MOBILE_FILE 缺 50015"
    [ "$MAP_TEXT_MATCH" = "1" ] || fail "双端 50015 文案不一致: pc='${PC_TEXT}' mobile='${MOBILE_TEXT}'"
    [ "$CONFIG_MSG_DRIFT" = "0" ] || fail "后端常量与码表不一致: backend='${CONFIG_TEXT}' pc='${PC_TEXT}'"
    [ "$PARITY" = "1" ] || fail "PC/mobile 错误文案平价失败"
    [ "$NODE_TEST_FAIL" = "0" ] || fail "node --test 失败 ${NODE_TEST_FAIL} 项"
    echo "R6_OK"
    ;;

  r7)
    echo "== R7 文档同步（新码 + 三元组口径 + 页面失败文案） =="
    API="docs/spec/api/api.md"
    UI="docs/spec/ui/ui.md"

    API_CODE_50015=0
    grep -q '^| 50015 |' "$API" && API_CODE_50015=1
    API_TRIPLE_RULE=0
    if grep -q '错误三元组一致性（#2598）' "$API" && grep -q '500 / 50015' "$API"; then API_TRIPLE_RULE=1; fi
    UI_WXBIND_TEXTS=0
    if grep -q '失败提示（#2598）' "$UI" && grep -q '身份服务配置异常，请联系平台管理员处理' "$UI"; then UI_WXBIND_TEXTS=1; fi
    UI_TRANSIT_NOTE=0
    if grep -q '建组织失败提示（#2598）' "$UI" && grep -q '500/50015' "$UI"; then UI_TRANSIT_NOTE=1; fi

    echo "API_CODE_50015=${API_CODE_50015}"
    echo "API_TRIPLE_RULE=${API_TRIPLE_RULE}"
    echo "UI_WXBIND_TEXTS=${UI_WXBIND_TEXTS}"
    echo "UI_TRANSIT_NOTE=${UI_TRANSIT_NOTE}"

    [ "$API_CODE_50015" = "1" ] || fail "$API 缺 50015 行"
    [ "$API_TRIPLE_RULE" = "1" ] || fail "$API 缺错误三元组口径段"
    [ "$UI_WXBIND_TEXTS" = "1" ] || fail "$UI 缺微信绑定/合并失败文案表"
    [ "$UI_TRANSIT_NOTE" = "1" ] || fail "$UI 缺中转建组织失败提示"
    echo "R7_OK"
    ;;

  r8|gate)
    echo "== R8 构建 / gofmt / eslint（no-undef） =="
    BUILD_OUT="$(mktemp)"
    ( cd backend && heavy go build ./... ) > "$BUILD_OUT" 2>&1
    BUILD_RC=$?
    tail -5 "$BUILD_OUT"
    rm -f "$BUILD_OUT"
    GO_BUILD_OK=0
    [ "$BUILD_RC" -eq 0 ] && GO_BUILD_OK=1

    GOFMT_FILES=(
      backend/services/iam_failure.go
      backend/services/iam_client.go
      backend/services/iam_failure_2598_test.go
      backend/handlers/iam_failure.go
      backend/handlers/transit_site_admin.go
      backend/handlers/user_staff.go
      backend/handlers/wechat_bind.go
      backend/handlers/transit_site_admin_2552_test.go
      backend/handlers/transit_site_admin_2598_test.go
      backend/handlers/wx_failure_2598_test.go
      backend/handlers/auth_test.go
      backend/handlers/wx_merge_2228_test.go
    )
    GOFMT_OUT="$(mktemp)"
    gofmt -l "${GOFMT_FILES[@]}" > "$GOFMT_OUT" 2>&1
    GOFMT_DIFF="$(grep -c . "$GOFMT_OUT" || true)"
    GOFMT_DIFF="${GOFMT_DIFF:-0}"
    cat "$GOFMT_OUT"
    rm -f "$GOFMT_OUT"

    LINT_OUT="$(mktemp)"
    ( cd frontend-pc && npx --no-install eslint src/services/errorMessages.js ) >> "$LINT_OUT" 2>&1
    PC_LINT_RC=$?
    ( cd frontend-mobile && npx --no-install eslint src/services/errorMessages.js ) >> "$LINT_OUT" 2>&1
    MOBILE_LINT_RC=$?
    ESLINT_NOUNDEF="$(grep -c 'no-undef' "$LINT_OUT" || true)"
    ESLINT_NOUNDEF="${ESLINT_NOUNDEF:-0}"
    if [ "$PC_LINT_RC" -ne 0 ] || [ "$MOBILE_LINT_RC" -ne 0 ]; then
      tail -30 "$LINT_OUT"
    fi
    rm -f "$LINT_OUT"

    echo "GO_BUILD_OK=${GO_BUILD_OK}"
    echo "GOFMT_DIFF=${GOFMT_DIFF}"
    echo "ESLINT_NOUNDEF=${ESLINT_NOUNDEF}"

    [ "$GO_BUILD_OK" = "1" ] || fail "go build ./... failed (rc=${BUILD_RC})"
    [ "$GOFMT_DIFF" = "0" ] || fail "gofmt diff present (${GOFMT_DIFF} files)"
    [ "$ESLINT_NOUNDEF" = "0" ] || fail "eslint no-undef present (${ESLINT_NOUNDEF})"
    [ "$PC_LINT_RC" -eq 0 ] && [ "$MOBILE_LINT_RC" -eq 0 ] || fail "eslint failed (pc=${PC_LINT_RC} mobile=${MOBILE_LINT_RC})"
    echo "R8_OK"
    ;;
esac
