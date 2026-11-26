#!/usr/bin/env bash
# =============================================================================
# scripts/verify-2563.sh — 证据生成器（#2563：PC api 客户端非 2xx 保留业务 code）
#
# 口径（docs/spec/api/api.md §1.3「PC 客户端非 2xx 契约」）：
#   404 / 409 → 返回对象（409 保留后端 code，业务可纠正冲突由此可达）
#   其余非 2xx（400/403/429/5xx）→ 抛 Error，并把 code/status/data 挂到 Error 上
#
# 用法：bash scripts/verify-2563.sh <r1..r6>
#   r1 docs/evidence/2563/R1-transport-unit.txt        非 2xx 分类纯函数回归（node --test）
#   r2 docs/evidence/2563/R2-client-wiring-static.txt  api.js 接线结构（401 分支在前 + 唯一 owner）
#   r3 docs/evidence/2563/R3-consumer-wiring.txt       SiteManagement catch 判定 + rubric 用例
#   r4 docs/evidence/2563/R4-redgreen-inventory.txt    修前/修后红绿对照 + 调用点计数
#   r5 docs/evidence/2563/R5-docs-contract.txt         api.md §1.3 契约段落
#   r6 docs/evidence/2563/R6-build-lint.txt            PC 构建 + eslint（no-undef）
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`
# =============================================================================
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4|r5|r6) ;;
  *)
    echo "usage: $0 <r1..r6>" >&2
    exit 2
    ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2563
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
API_JS="frontend-pc/src/services/api.js"
API_ERROR_JS="frontend-pc/src/services/apiError.js"
SITE_JS="frontend-pc/src/pages/SiteManagement.jsx"
API_DOC="docs/spec/api/api.md"

HEAD_SHA="$(git rev-parse HEAD)"
if [ -z "${FIX_REF:-}" ]; then
  FIX_REF="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
fi
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor" >&2; exit 3; }
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

# nonok_block <file> → stdout: the `if (!response.ok) { ... }` block of request()
nonok_block() {
  awk '
    /if \(!response\.ok\) \{/ {inblk = 1}
    inblk {print}
    inblk && /^  }$/ {exit}
  ' "$1"
}

# request_body <file> → stdout: request() function body (up to `export const api = {`)
# 注意：api.js 里 request() 内部存在一处列 0 的 `}`（历史缩进），不能用 `^}$` 作为终止条件。
request_body() {
  awk '
    /^export const api = \{/ {if (inblk) exit}
    /^async function request\(/ {inblk = 1}
    inblk {print}
  ' "$1"
}

# handle_submit <file> → stdout: handleSubmit function body
handle_submit() {
  awk '
    /const handleSubmit = async \(\) => \{/ {inblk = 1}
    inblk {print}
    inblk && /^  const renderEmptyState/ {exit}
  ' "$1"
}

NAME=""
RC=0
LOG="$WORK/$MODE.log"
: > "$LOG"

case "$MODE" in
  r1)
    NAME="R1-transport-unit.txt"
    {
      printf '%s\n' "$PROV"
      echo "# 非 2xx 分类纯函数回归（resolveHttpFailure）：400/403/429/500/502 抛 Error 且带 code；404/409 返回对象"
      ( cd "$REPO/frontend-pc" && heavy node --test tests/apiError.test.mjs ) 2>&1
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'THROW_40002=1' 'THROW_40004=1' 'THROW_403=1' 'THROW_429=1' \
      'THROW_500=1' 'THROW_502=1' 'MSG_40002=CJK_PASSTHROUGH' 'RESULT_404=1' 'RESULT_409=1' \
      'RESULT_409_DEFAULT_CODE=40900' 'CODE_STRING_COERCED=1' 'BAD_PAYLOAD_SAFE=1' '# fail 0' || RC=1
    ;;
  r2)
    NAME="R2-client-wiring-static.txt"
    BODY="$WORK/request.body"; request_body "$REPO/$API_JS" > "$BODY"; nonok_block "$BODY" > "$WORK/nonok.block"
    RESOLVER_CALLS="$(grep -c 'resolveHttpFailure(' "$WORK/nonok.block" || true)"
    INLINE_MAP="$(grep -cE 'response\.status === (404|409)' "$BODY" || true)"
    THROW_PATH="$(grep -c 'throw failure\.value' "$WORK/nonok.block" || true)"
    # 块内任何形态的内联对象返回（含同行 `if (...) { return {...} }`）都算违规
    INLINE_RETURNS="$(grep -cF 'return {' "$WORK/nonok.block" || true)"
    AUTH_LINE="$(grep -nE 'response\.status === 401|handleAuthError\(' "$BODY" | head -1 | cut -d: -f1)"
    NONOK_LINE="$(grep -n 'if (!response\.ok)' "$BODY" | head -1 | cut -d: -f1)"
    AUTH_BEFORE=0
    if [ -n "$AUTH_LINE" ] && [ -n "$NONOK_LINE" ] && [ "$AUTH_LINE" -lt "$NONOK_LINE" ]; then AUTH_BEFORE=1; fi
    AUTH_CODES=0
    for c in 40104 40105 40106 40107; do grep -q "$c" "$BODY" && AUTH_CODES=$((AUTH_CODES + 1)); done
    PERM_VERSION="$(grep -c 'x-perm-version' "$BODY" || true)"
    # idle 守卫在 refreshAccessToken 内（request() 只调用 isIdleLogoutPending），故按整文件计数。
    IDLE_GUARD="$(grep -cE 'isIdleTimeoutPayload|isIdleLogoutPending' "$REPO/$API_JS" || true)"
    {
      printf '%s\n' "$PROV"
      echo "# api.js request() 结构：非 2xx 只经唯一 owner 映射；401/perm_version/idle 分支顺序不变"
      printf 'FAILURE_RESOLVER_CALLS=%s\nINLINE_STATUS_MAP=%s\nTHROW_PATH=%s\nNONOK_INLINE_RETURNS=%s\n' \
        "$RESOLVER_CALLS" "$INLINE_MAP" "$THROW_PATH" "$INLINE_RETURNS"
      printf 'AUTH_BLOCK_BEFORE_NONOK=%s\nAUTH_CODES=%s\nPERM_VERSION=%s\nIDLE_GUARD=%s\n' \
        "$AUTH_BEFORE" "$AUTH_CODES" "$PERM_VERSION" "$IDLE_GUARD"
      printf 'AUTH_LINE=%s\nNONOK_LINE=%s\n' "${AUTH_LINE:-0}" "${NONOK_LINE:-0}"
      if [ "$RESOLVER_CALLS" = "1" ] && [ "$INLINE_MAP" = "0" ] && [ "$THROW_PATH" = "1" ] && \
         [ "$INLINE_RETURNS" = "0" ] && [ "$AUTH_BEFORE" = "1" ] && [ "$AUTH_CODES" = "4" ] && \
         [ "$PERM_VERSION" -ge 1 ] && [ "$IDLE_GUARD" -ge 1 ]; then
        echo "R2_OK"
      else
        echo "R2_FAIL"
      fi
    } > "$LOG" 2>&1
    grep -q '^R2_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'NONOK_INLINE_RETURNS=0' 'FAILURE_RESOLVER_CALLS=1' \
      'INLINE_STATUS_MAP=0' 'AUTH_BLOCK_BEFORE_NONOK=1' 'AUTH_CODES=4' || RC=1
    ;;
  r3)
    NAME="R3-consumer-wiring.txt"
    handle_submit "$REPO/$SITE_JS" > "$WORK/submit.body"
    awk '/\} catch \(err\) \{/ {inblk = 1} inblk {print} inblk && /^    \} finally/ {exit}' "$WORK/submit.body" > "$WORK/catch.block"
    CONFLICT_BRANCH="$(grep -c 'isBusinessCorrectableConflict(err)' "$WORK/catch.block" || true)"
    CONFLICT_TEXT="$(grep -c 'conflictText(err' "$WORK/catch.block" || true)"
    FALLBACK_TOAST="$(grep -c 'message\.error(' "$WORK/catch.block" || true)"
    ALERT_RENDERED="$(grep -c '<FormConflictAlert' "$REPO/$SITE_JS" || true)"
    {
      printf '%s\n' "$PROV"
      echo "# SiteManagement.handleSubmit catch：先判共享 rubric → 持久提示，其余保持瞬时 toast"
      printf 'CONFLICT_BRANCH_IN_CATCH=%s\nCONFLICT_TEXT_FALLBACK=%s\nFALLBACK_TOAST=%s\nALERT_RENDERED=%s\n' \
        "$([ "$CONFLICT_BRANCH" -ge 1 ] && echo 1 || echo 0)" "$([ "$CONFLICT_TEXT" -ge 1 ] && echo 1 || echo 0)" \
        "$([ "$FALLBACK_TOAST" -ge 1 ] && echo 1 || echo 0)" "$([ "$ALERT_RENDERED" -ge 1 ] && echo 1 || echo 0)"
      echo "# rubric 对 Error 实例的判定（node --test frontend-pc/tests/formConflict.test.mjs）"
      ( cd "$REPO/frontend-pc" && heavy node --test tests/formConflict.test.mjs ) 2>&1
      if [ "$CONFLICT_BRANCH" -ge 1 ] && [ "$CONFLICT_TEXT" -ge 1 ] && [ "$FALLBACK_TOAST" -ge 1 ] && \
         [ "$ALERT_RENDERED" -ge 1 ]; then
        echo "R3_OK"
      else
        echo "R3_FAIL"
      fi
    } > "$LOG" 2>&1
    grep -q '^R3_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'CONFLICT_BRANCH_IN_CATCH=1' 'TRUE_ENRICHED_ERROR=1' 'FALSE_PLAIN_ERROR=1' '# fail 0' || RC=1
    ;;
  r4)
    NAME="R4-redgreen-inventory.txt"
    RED_BASE="$(git rev-parse "$FIX_SHA^")"
    git show "$RED_BASE:$API_JS" > "$WORK/api.before.js" 2>/dev/null || { echo "FATAL: cannot read pre-fix api.js" >&2; exit 3; }
    request_body "$WORK/api.before.js" > "$WORK/api.before.body"
    request_body "$REPO/$API_JS" > "$WORK/api.after.body"
    [ -s "$WORK/api.before.body" ] || { echo "FATAL: cannot scope pre-fix request()" >&2; exit 3; }
    nonok_block "$WORK/api.before.body" > "$WORK/nonok.before"
    nonok_block "$WORK/api.after.body" > "$WORK/nonok.after"
    RED_NO_CODE=0
    if grep -q 'throw new Error(resolveErrorMessage' "$WORK/nonok.before" && \
       ! grep -q 'error\.code =' "$WORK/nonok.before"; then RED_NO_CODE=1; fi
    GREEN_CODE_ATTACHED=0
    grep -q 'error\.code = code' "$REPO/$API_ERROR_JS" && GREEN_CODE_ATTACHED=1
    INLINE_RETURNS="$(grep -cF 'return {' "$WORK/nonok.after" || true)"
    CALL_SITES="$(grep -rhoE 'await (api|[a-zA-Z]+Api)\.' "$REPO/frontend-pc/src" --include='*.js' --include='*.jsx' | wc -l | tr -d ' ')"
    GE150=0; [ "$CALL_SITES" -ge 150 ] && GE150=1
    {
      printf '%s\n' "$PROV"
      echo "# 红绿对照：修前非 2xx 丢弃 code（红）→ 修后 Error 携带 code（绿）；调用点仍依赖 throw 语义"
      printf 'RED_BASE=%s\n' "$RED_BASE"
      printf 'RED_THROW_SITE=%s\n' "$(grep -c 'throw new Error(resolveErrorMessage' "$WORK/nonok.before" || true)"
      printf 'RED_NO_CODE=%s\nGREEN_CODE_ATTACHED=%s\nNONOK_INLINE_RETURNS=%s\nAWAIT_CALL_SITES=%s\nCALL_SITES_GE_150=%s\n' \
        "$RED_NO_CODE" "$GREEN_CODE_ATTACHED" "$INLINE_RETURNS" "$CALL_SITES" "$GE150"
      if [ "$RED_NO_CODE" = "1" ] && [ "$GREEN_CODE_ATTACHED" = "1" ] && [ "$INLINE_RETURNS" = "0" ] && [ "$GE150" = "1" ]; then
        echo "R4_OK"
      else
        echo "R4_FAIL"
      fi
    } > "$LOG" 2>&1
    grep -q '^R4_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'RED_NO_CODE=1' 'GREEN_CODE_ATTACHED=1' 'NONOK_INLINE_RETURNS=0' || RC=1
    ;;
  r5)
    NAME="R5-docs-contract.txt"
    SECTION="$(awk '/PC 客户端非 2xx 契约（#2563）/{f=1} f{print} f && /^### 1\.4/ {exit}' "$REPO/$API_DOC")"
    {
      printf '%s\n' "$PROV"
      echo "# docs/spec/api/api.md §1.3：PC 客户端非 2xx 契约段落"
      printf 'DOC_SECTION_LINES=%s\n' "$(printf '%s\n' "$SECTION" | wc -l | tr -d ' ')"
      for tok in 'PC 客户端非 2xx 契约' 'error.code' 'error.status' '40002' 'catch' '本域行为不变' '40900'; do
        printf 'DOC_HAS[%s]=%s\n' "$tok" "$(printf '%s' "$SECTION" | grep -Fq -- "$tok" && echo 1 || echo 0)"
      done
      if printf '%s' "$SECTION" | grep -Fq 'PC 客户端非 2xx 契约' && \
         printf '%s' "$SECTION" | grep -Fq 'error.code' && \
         printf '%s' "$SECTION" | grep -Fq '40002' && \
         printf '%s' "$SECTION" | grep -Fq 'catch' && \
         printf '%s' "$SECTION" | grep -Fq '本域行为不变'; then
        echo "DOC_CONTRACT_OK=1"
        echo "R5_OK"
      else
        echo "DOC_CONTRACT_OK=0"
        echo "R5_FAIL"
      fi
    } > "$LOG" 2>&1
    grep -q '^R5_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'DOC_CONTRACT_OK=1' || RC=1
    ;;
  r6)
    NAME="R6-build-lint.txt"
    {
      printf '%s\n' "$PROV"
      echo "BUILD_CMD=cd frontend-pc && npm run build"
      ( cd "$REPO/frontend-pc" && heavy npm run build ) 2>&1 | tail -12
      BUILD=${PIPESTATUS[0]}
      echo "BUILD_EXIT=$BUILD"
      echo "ESLINT_CMD=cd frontend-pc && npx eslint src/services/apiError.js src/services/api.js src/pages/SiteManagement.jsx"
      ( cd "$REPO/frontend-pc" && heavy npx eslint \
          src/services/apiError.js src/services/api.js src/pages/SiteManagement.jsx ) > "$WORK/eslint.out" 2>&1
      LINT=$?
      cat "$WORK/eslint.out" | tail -20
      echo "ESLINT_EXIT=$LINT"
      NO_UNDEF="$(grep -c 'no-undef' "$WORK/eslint.out" || true)"
      echo "ESLINT_NO_UNDEF=$NO_UNDEF"
      if [ "$BUILD" = "0" ] && [ "$NO_UNDEF" = "0" ]; then echo "R6_OK"; else echo "R6_FAIL"; fi
    } > "$LOG" 2>&1
    grep -q '^R6_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'BUILD_EXIT=0' 'ESLINT_NO_UNDEF=0' || RC=1
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
