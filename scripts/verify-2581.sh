#!/usr/bin/env bash
# =============================================================================
# scripts/verify-2581.sh — 证据生成器（#2581 网点重名冲突：中文具体原因 + 40900 持久提示）
#
# 口径（docs/spec/api/api.md §4.5.1 注 #2581）：
#   POST /api/merchant/sites 的 IAM 组织名冲突 → 409/40900 + 中文具体原因
#   `该商户下已存在同名网点「<name>」，请换一个名称`；前端 40900 落表单内持久提示。
#
# 用法：bash scripts/verify-2581.sh <r1..r5>
#   r1 docs/evidence/2581/R1-create-duplicate.txt    同商户重名 → 409/40900 中文 + 不落第二个网点
#   r2 docs/evidence/2581/R2-cross-merchant.txt      跨商户同名仍允许（唯一性 scope = 商户内）
#   r3 docs/evidence/2581/R3-frontend-chain.txt      409 → rubric → 文案透传纯函数链路（node --test）
#   r4 docs/evidence/2581/R4-page-wiring.txt         SiteManagement 冲突落常驻 Alert（源码结构断言）
#   r5 docs/evidence/2581/R5-build-lint.txt          go build + eslint(no-undef) + 前端单测
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`
# 重命令一律经 heavy-run.sh 取锁（D7）。
# =============================================================================
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4|r5) ;;
  *)
    echo "usage: $0 <r1..r5>" >&2
    exit 2
    ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2581
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
SITE_GO="backend/handlers/site.go"
SITE_TEST_GO="backend/handlers/site_create_conflict_2581_test.go"
SITE_JS="frontend-pc/src/pages/SiteManagement.jsx"
FE_TEST="frontend-pc/tests/siteConflict2581.test.mjs"
API_DOC="docs/spec/api/api.md"

# 测试库：默认使用一次性 PostgreSQL（#2341 口径，不使用 jobmaster-postgres）。
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"

HEAD_SHA="$(git rev-parse HEAD)"
if [ -z "${FIX_REF:-}" ]; then
  FIX_REF="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
fi
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor of HEAD" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
DIRTY=false
[ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR")" ] && DIRTY=true
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
HEAVY_RUN="${HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"
LOCK_DIR="${HEAVY_LOCK_DIR:-/tmp/opencode/locks}"
WORK="$(mktemp -d "$REPO/tmp/verify-$ISSUE-XXXXXX")" || exit 2
trap 'rm -rf "$WORK"' EXIT

heavy() {
  if [ -x "$HEAVY_RUN" ]; then bash "$HEAVY_RUN" --dir "$LOCK_DIR" -- "$@"; else "$@"; fi
}

require_tokens() { # log tokens...
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

# go_test_r2581 <test regex> <out> → 运行定向测试并保留 stdout+stderr
go_test_r2581() {
  local re="$1" out="$2"
  ( cd "$REPO/backend" && heavy go test ./handlers -run "$re" -count=1 -v -timeout 10m ) > "$out" 2>&1
  return $?
}

NAME=""
RC=0
LOG="$WORK/$MODE.log"
: > "$LOG"

case "$MODE" in
  r1)
    NAME="R1-create-duplicate.txt"
    # 红样：fix^ 源码含 ASCII 英文 message 字面量（缺陷形态）
    RED_ENGLISH_MSG_PRE=0
    if git show "$FIX_SHA^:$SITE_GO" 2>/dev/null | grep -q 'organization name already exists: '; then
      RED_ENGLISH_MSG_PRE=1
    fi
    AFTER_ASCII="$(grep -c 'organization name already exists' "$REPO/$SITE_GO" || true)"
    AFTER_SENTINEL_COUNT="$(grep -c 'ErrOrganizationNameConflict' "$REPO/$SITE_GO" || true)"
    AFTER_SENTINEL_IS=0
    [ "$AFTER_SENTINEL_COUNT" -ge 1 ] && AFTER_SENTINEL_IS=1
    go_test_r2581 'TestCreateSiteDuplicateName2581' "$WORK/r1.test.log"; TEST_RC=$?
    {
      printf '%s\n' "$PROV"
      echo "# POST /api/merchant/sites：同商户第二次同名 → 409/40900 + 中文具体原因；本地不落第二个网点"
      printf 'RED_ENGLISH_MSG_PRE=%s\n' "$RED_ENGLISH_MSG_PRE"
      printf 'AFTER_ASCII_MSG_LITERAL=%s\n' "$AFTER_ASCII"
      printf 'AFTER_SENTINEL_COUNT=%s\n' "$AFTER_SENTINEL_COUNT"
      printf 'AFTER_SENTINEL_IS=%s\n' "$AFTER_SENTINEL_IS"
      printf 'GO_TEST_EXIT=%s\n' "$TEST_RC"
      grep -E 'DUP_(STATUS|CODE|MSG_CJK|MSG_HAS_NAME|SITES_COUNT|ORG_CALLS)=' "$WORK/r1.test.log" | sed 's/^ *//'
      if [ "$TEST_RC" = "0" ]; then echo "R1_OK"; else echo "R1_FAIL"; fi
    } > "$LOG" 2>&1
    grep -q '^R1_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'RED_ENGLISH_MSG_PRE=1' 'AFTER_ASCII_MSG_LITERAL=0' 'AFTER_SENTINEL_IS=1' \
      'GO_TEST_EXIT=0' 'DUP_STATUS=409' 'DUP_CODE=40900' 'DUP_MSG_CJK=1' 'DUP_MSG_HAS_NAME=1' \
      'DUP_SITES_COUNT=1' 'DUP_ORG_CALLS=2' || RC=1
    ;;
  r2)
    NAME="R2-cross-merchant.txt"
    go_test_r2581 'TestCreateSiteCrossMerchantSameName2581' "$WORK/r2.test.log"; TEST_RC=$?
    {
      printf '%s\n' "$PROV"
      echo "# 跨商户同名：两次 201 + 2 个 tenant（唯一性 scope = 商户内，防预检过宽）"
      printf 'GO_TEST_EXIT=%s\n' "$TEST_RC"
      grep -E 'CROSS_(FIRST|SECOND|TENANTS|SAME_NAME)=' "$WORK/r2.test.log" | sed 's/^ *//'
      if [ "$TEST_RC" = "0" ]; then echo "R2_OK"; else echo "R2_FAIL"; fi
    } > "$LOG" 2>&1
    grep -q '^R2_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'GO_TEST_EXIT=0' 'CROSS_FIRST=201' 'CROSS_SECOND=201' \
      'CROSS_TENANTS=2' 'CROSS_SAME_NAME=1' || RC=1
    ;;
  r3)
    NAME="R3-frontend-chain.txt"
    {
      printf '%s\n' "$PROV"
      echo "# 409 → resolveHttpFailure(result) → isBusinessCorrectableConflict → conflictText（含英文负例 + catch 路径）"
      ( cd "$REPO/frontend-pc" && heavy node --test --test-reporter=tap "tests/siteConflict2581.test.mjs" ) 2>&1
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'CHAIN_KIND=result' 'CHAIN_CODE=40900' 'CHAIN_MSG_CJK=1' 'CHAIN_TEXT_MATCH=1' \
      'CHAIN_RUBRIC=1' 'NEG_ENGLISH_GENERIC=1' 'CATCH_RUBRIC=1' '# fail 0' || RC=1
    grep -q '^# fail 0$' "$LOG" || RC=1
    ;;
  r4)
    NAME="R4-page-wiring.txt"
    # handleSubmit 函数体（到 renderEmptyState 为止）
    awk '/const handleSubmit = async \(\) => \{/ {inblk=1} inblk {print} inblk && /^  const renderEmptyState/ {exit}' \
      "$REPO/$SITE_JS" > "$WORK/handleSubmit.body"
    # try 内的冲突分支（isBusinessCorrectableConflict(result) → 到 return）
    awk '/if \(isBusinessCorrectableConflict\(result\)\) \{/ {inblk=1} inblk {print} inblk && /^      \}$/ {exit}' \
      "$WORK/handleSubmit.body" > "$WORK/conflict.block"
    # catch 块
    awk '/\} catch \(err\) \{/ {inblk=1} inblk {print} inblk && /^    \} finally/ {exit}' \
      "$WORK/handleSubmit.body" > "$WORK/catch.block"

    ALERT_MOUNTED="$(grep -c '<FormConflictAlert' "$REPO/$SITE_JS" || true)"
    CONFLICT_LINE="$(grep -n 'isBusinessCorrectableConflict(result)' "$WORK/handleSubmit.body" | head -1 | cut -d: -f1)"
    TOAST_LINE="$(grep -n 'message\.error(' "$WORK/handleSubmit.body" | head -1 | cut -d: -f1)"
    CONFLICT_BEFORE_TOAST=0
    if [ -n "$CONFLICT_LINE" ] && [ -n "$TOAST_LINE" ] && [ "$CONFLICT_LINE" -lt "$TOAST_LINE" ]; then
      CONFLICT_BEFORE_TOAST=1
    fi
    TOAST_ON_CONFLICT="$(grep -c 'message\.error(' "$WORK/conflict.block" || true)"
    CONFLICT_RETURN="$(grep -c '^        return$' "$WORK/conflict.block" || true)"
    CATCH_RUBRIC="$(grep -c 'isBusinessCorrectableConflict(err)' "$WORK/catch.block" || true)"
    TIMER_DISMISS="$(grep -cE 'setTimeout\([^)]*setFormConflict' "$REPO/$SITE_JS" || true)"
    CONFLICT_TEXT_FALLBACK="$(grep -c 'conflictText(result' "$WORK/conflict.block" || true)"
    {
      printf '%s\n' "$PROV"
      echo "# SiteManagement.handleSubmit：409 冲突先落表单内常驻 Alert 并 return；catch 仍按共享 rubric 判定"
      printf 'SITE_ALERT_MOUNTED=%s\n' "$ALERT_MOUNTED"
      printf 'SITE_CONFLICT_BEFORE_TOAST=%s\n' "$CONFLICT_BEFORE_TOAST"
      printf 'SITE_TOAST_ON_CONFLICT=%s\n' "$TOAST_ON_CONFLICT"
      printf 'SITE_CONFLICT_RETURN=%s\n' "$CONFLICT_RETURN"
      printf 'SITE_CONFLICT_TEXT_FALLBACK=%s\n' "$CONFLICT_TEXT_FALLBACK"
      printf 'SITE_CATCH_RUBRIC=%s\n' "$CATCH_RUBRIC"
      printf 'SITE_TIMER_DISMISS=%s\n' "$TIMER_DISMISS"
      echo "--- conflict block ---"
      cat "$WORK/conflict.block"
      if [ "$ALERT_MOUNTED" = "1" ] && [ "$CONFLICT_BEFORE_TOAST" = "1" ] && [ "$TOAST_ON_CONFLICT" = "0" ] && \
         [ "$CONFLICT_RETURN" -ge 1 ] && [ "$CONFLICT_TEXT_FALLBACK" = "1" ] && \
         [ "$CATCH_RUBRIC" -ge 1 ] && [ "$TIMER_DISMISS" = "0" ]; then
        echo "R4_OK"
      else
        echo "R4_FAIL"
      fi
    } > "$LOG" 2>&1
    grep -q '^R4_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'SITE_ALERT_MOUNTED=1' 'SITE_CONFLICT_BEFORE_TOAST=1' 'SITE_TOAST_ON_CONFLICT=0' \
      'SITE_CATCH_RUBRIC=1' 'SITE_TIMER_DISMISS=0' || RC=1
    # 文档契约（Core Update）：api.md §4.5.1 的 #2581 注必须存在且含关键口径
    DOC_NOTE="$(awk '/注（#2581）/{f=1} f{print} f && /^\*\*响应\*\*:/ {exit}' "$REPO/$API_DOC")"
    {
      echo "--- api.md §4.5.1 ---"
      printf 'DOC_SECTION_LINES=%s\n' "$(printf '%s\n' "$DOC_NOTE" | wc -l | tr -d ' ')"
      for tok in '该商户下已存在同名网点' '40900' 'FormConflictAlert' '跨商户同名仍允许'; do
        printf 'DOC_HAS[%s]=%s\n' "$tok" "$(printf '%s' "$DOC_NOTE" | grep -Fq -- "$tok" && echo 1 || echo 0)"
      done
      if printf '%s' "$DOC_NOTE" | grep -Fq '该商户下已存在同名网点' && \
         printf '%s' "$DOC_NOTE" | grep -Fq '40900' && \
         printf '%s' "$DOC_NOTE" | grep -Fq 'FormConflictAlert' && \
         printf '%s' "$DOC_NOTE" | grep -Fq '跨商户同名仍允许'; then
        echo "DOC_CONTRACT_OK=1"
        echo "R4_DOC_OK"
      else
        echo "DOC_CONTRACT_OK=0"
        echo "R4_DOC_FAIL"
      fi
    } >> "$LOG" 2>&1
    grep -q '^R4_DOC_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'DOC_CONTRACT_OK=1' || RC=1
    ;;
  r5)
    NAME="R5-build-lint.txt"
    {
      printf '%s\n' "$PROV"
      echo "BUILD_CMD=cd backend && go build ./..."
      ( cd "$REPO/backend" && heavy go build ./... ) > "$WORK/build.out" 2>&1
      BUILD=$?
      tail -5 "$WORK/build.out"
      echo "BUILD_EXIT=$BUILD"
      echo "ESLINT_CMD=frontend-pc: npx eslint src/pages/SiteManagement.jsx tests/siteConflict2581.test.mjs"
      ( cd "$REPO/frontend-pc" && npx eslint src/pages/SiteManagement.jsx ) > "$WORK/eslint.out" 2>&1
      LINT=$?
      tail -10 "$WORK/eslint.out"
      echo "ESLINT_EXIT=$LINT"
      NO_UNDEF="$(grep -c 'no-undef' "$WORK/eslint.out" || true)"
      echo "ESLINT_NOUNDEF=$NO_UNDEF"
      echo "UNIT_CMD=frontend-pc: node --test tests/siteConflict2581.test.mjs tests/formConflict.test.mjs tests/apiError.test.mjs tests/errorMessages.test.mjs"
      ( cd "$REPO/frontend-pc" && heavy node --test --test-reporter=tap \
          tests/siteConflict2581.test.mjs tests/formConflict.test.mjs tests/apiError.test.mjs tests/errorMessages.test.mjs ) \
        > "$WORK/unit.out" 2>&1
      UNIT=$?
      grep -E '^# (tests|pass|fail) ' "$WORK/unit.out"
      echo "NODE_TEST_EXIT=$UNIT"
      NODE_FAIL="$(grep -cE '^# fail [1-9]' "$WORK/unit.out" || true)"
      echo "NODE_TEST_FAIL=$NODE_FAIL"
      if [ "$BUILD" = "0" ] && [ "$NO_UNDEF" = "0" ] && [ "$NODE_FAIL" = "0" ]; then
        echo "GO_BUILD_OK=1"
        echo "ESLINT_NOUNDEF=0"
        echo "R5_OK"
      else
        echo "GO_BUILD_OK=0"
        echo "R5_FAIL"
      fi
    } > "$LOG" 2>&1
    grep -q '^R5_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'BUILD_EXIT=0' 'GO_BUILD_OK=1' 'ESLINT_NOUNDEF=0' 'NODE_TEST_FAIL=0' || RC=1
    ;;
esac

ART="$OUT_DIR/$NAME"
cp "$LOG" "$ART"
echo "ARTIFACT=$ART"
if [ "$RC" != "0" ]; then
  echo "RESULT: FAIL ($MODE)" >&2
  exit 1
fi
echo "RESULT: PASS ($MODE)"
