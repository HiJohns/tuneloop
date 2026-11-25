#!/usr/bin/env bash
# #2489 证据生成器 — 配置/定价/权限/商户/租户类页面：业务可纠正错误 → 持久提示。
#
# 用法：bash scripts/verify-2489.sh <r1|r2|r3|r4|r5>
#   r1  docs/evidence/2489/R1-classify.txt    判定表（6 文件每个 message.error 一行）
#   r2  docs/evidence/2489/R2-converted.txt   已转换行 + code-first 判定
#   r3  docs/evidence/2489/R3-shared.txt      共享件复用 / 本地 Alert 复制
#   r4  docs/evidence/2489/R4-residual.txt    残留业务 toast
#   r5  docs/evidence/2489/R5-build.txt       PC 构建 + eslint
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4|r5) ;;
  *)
    echo "usage: $0 <r1|r2|r3|r4|r5>" >&2
    exit 2
    ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2489
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
HEAD_SHA="$(git rev-parse HEAD)"
if [ -z "${FIX_REF:-}" ]; then
  FIX_REF="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
fi
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
DIRTY=false
[ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR" ':(exclude)tmp')" ] && DIRTY=true
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
TREE_LINE="# tree=$HEAD_SHA"

HEAVY_RUN="${OPENCODE_HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"
SRC_DIR="frontend-pc/src"
BASELINE="docs/evidence/2480/R11-scan.txt"
TARGETS=(
  "pages/admin/PermissionManage/index.jsx"
  "pages/admin/pricing/MerchantPricingConfig.jsx"
  "pages/admin/transit/TransitCenter.jsx"
  "pages/System/MembershipLevelsPage.jsx"
  "pages/MerchantManagement.jsx"
  "pages/TenantManagement.jsx"
)

LOG="$REPO/tmp/verify-2489-${MODE}.log"
: > "$LOG"
trap 'rm -f "$LOG"' EXIT
RC=0
BODY="$OUT_DIR/.body.$$"
: > "$BODY"
trap 'rm -f "$LOG" "$BODY"' EXIT

emit() { printf '%s\n' "$*" | tee -a "$BODY"; }
require() { # token-in-body
  grep -Fq -- "$1" "$BODY" || { echo "ASSERT_FAIL: missing [$1]" >&2; RC=1; }
}
finish() { # artifact
  local name="$1"
  {
    printf '%s\n%s\n' "$PROV" "$TREE_LINE"
    cat "$BODY"
  } > "$OUT_DIR/$name"
  cat "$OUT_DIR/$name"
  [ "$RC" = "0" ] || return 1
}

enclosing_fn() { # file line
  awk -v target="$2" '
    NR <= target && (/^[[:space:]]*(const|let)[[:space:]]+[A-Za-z0-9_]+[[:space:]]*=[[:space:]]*(async[[:space:]]*)?\(/ || /^[[:space:]]*function[[:space:]]+[A-Za-z0-9_]+/) {
      line = $0
      sub(/^[[:space:]]+/, "", line)
      if (line ~ /^function[[:space:]]+/) {
        sub(/^function[[:space:]]+/, "", line)
      } else {
        sub(/^(const|let)[[:space:]]+/, "", line)
        sub(/[[:space:]]*=.*/, "", line)
      }
      sub(/[^A-Za-z0-9_].*/, "", line)
      if (line != "") found = line
    }
    END { print (found == "" ? "unknown" : found) }
  ' "$1"
}

# 分类扫描：每个 message.error( 一行；tier 口径与 #2480 rubric 一致。
scan_rows() {
  local rel file line ctx fn tier reason code_aware business_code codes_count
  for rel in "${TARGETS[@]}"; do
    file="$SRC_DIR/$rel"
    [ -f "$file" ] || { echo "ERROR: target missing: $rel" >&2; return 2; }
    while IFS=: read -r line _rest; do
      [ -n "${line:-}" ] || continue
      local start=$((line - 12)); [ "$start" -lt 1 ] && start=1
      ctx="$(sed -n "${start},${line}p" "$file")"
      fn="$(enclosing_fn "$file" "$line")"
      code_aware=0
      printf '%s\n' "$ctx" | grep -qE 'isBusinessCorrectableConflict\(|code[[:space:]]*===?[[:space:]]*[0-9]+' && code_aware=1
      business_code="$(printf '%s\n' "$ctx" \
        | grep -oE 'code[[:space:]]*===[[:space:]]*[0-9]+' \
        | grep -oE '[0-9]+$' \
        | grep -E '^(40002|40004|40005|40900)$' | head -1)"
      reason=""
      if [ -n "$business_code" ]; then
        tier="residual:business-toast"
        reason="business-correctable code $business_code still rendered by a 3s toast"
      elif printf '%s\n' "$ctx" | grep -q 'isBusinessCorrectableConflict('; then
        tier="business-correctable"
        reason="business branch routed to FormConflictAlert; this toast is the non-business fallback"
      elif printf '%s\n' "$ctx" | grep -qE 'errorFields|不能为空|需要大于|必须在|必填'; then
        tier="keep:instant"
        reason="client-side validation feedback"
      elif printf '%s\n' "$ctx" | grep -qE 'api\.get\(|api\.delete\(|加载'; then
        tier="keep:instant"
        reason="load / delete / transport feedback"
      elif printf '%s\n' "$ctx" | grep -qE 'catch'; then
        tier="keep:instant"
        reason="catch-all fallback (network/unknown)"
      elif printf '%s\n' "$ctx" | grep -qE '\.message[[:space:]]*\|\|'; then
        tier="keep:instant"
        reason="server message without a business-correctable code"
      else
        tier="defer:tier2"
        reason="form falls back to toast without reading code (needs per-page judgement)"
      fi
      printf '%s:%s\tfn=%s\treads_code=%s\ttier=%s\treason=%s\n' \
        "$rel" "$line" "${fn:-unknown}" "$code_aware" "$tier" "$reason"
    done < <(grep -n 'message\.error(' "$file")
  done
}

scan_metrics() {
  local rows; rows="$(cat)"
  local total keep persisted defer residual unclassified
  total=$(printf '%s\n' "$rows" | grep -c $'\tfn=' || true)
  keep=$(printf '%s\n' "$rows" | grep -c $'tier=keep:instant' || true)
  persisted=$(printf '%s\n' "$rows" | grep -c $'tier=business-correctable' || true)
  defer=$(printf '%s\n' "$rows" | grep -c $'tier=defer:tier2' || true)
  residual=$(printf '%s\n' "$rows" | grep -c $'tier=residual:business-toast' || true)
  unclassified=$((total - keep - persisted - defer - residual))
  printf 'SCAN_ROWS=%s\nKEEP_ROWS=%s\nBUSINESS_FALLBACK_ROWS=%s\nDEFER_ROWS=%s\nBUSINESS_TOAST_RESIDUAL=%s\nUNCLASSIFIED=%s\n' \
    "${total:-0}" "${keep:-0}" "${persisted:-0}" "${defer:-0}" "${residual:-0}" "${unclassified:-0}"
}

case "$MODE" in
  r1)
    ROWS="$(scan_rows)" || { finish R1-classify.txt || true; exit 1; }
    emit "SCAN_TARGETS=${#TARGETS[@]}"
    emit "READONLY=1"
    emit "DB_WRITES=0"
    echo "$ROWS" | scan_metrics | while IFS= read -r l; do emit "$l"; done
    # 与 #2480 扫描基线（同 6 文件行数）对比
    BASE_ROWS=0
    if [ -f "$BASELINE" ]; then
      BASE_ROWS=$(grep -cE "frontend-pc/src/pages/(admin/PermissionManage/index|admin/pricing/MerchantPricingConfig|admin/transit/TransitCenter|System/MembershipLevelsPage|MerchantManagement|TenantManagement)\.jsx:" "$BASELINE" || true)
    fi
    emit "BASELINE_ROWS=${BASE_ROWS:-0}"
    emit "--- rows ---"
    while IFS= read -r l; do [ -n "$l" ] && emit "$l"; done <<< "$ROWS"
    SCAN_ROWS=$(sed -n 's/^SCAN_ROWS=//p' "$BODY" | tail -1)
    [ "${SCAN_ROWS:-0}" -gt 0 ] || { echo "ASSERT_FAIL: no message.error rows" >&2; RC=1; }
    require 'UNCLASSIFIED=0'
    require 'READONLY=1'
    if [ "${BASE_ROWS:-0}" -gt 0 ] && [ "${SCAN_ROWS:-0}" -lt "${BASE_ROWS:-0}" ]; then
      echo "ASSERT_FAIL: SCAN_ROWS($SCAN_ROWS) < BASELINE_ROWS($BASE_ROWS)" >&2; RC=1
    fi
    finish R1-classify.txt || RC=1
    ;;

  r2)
    CONVERTED=0; CODE_FIRST=1; PAGES_WITH_CONVERSION=0
    for rel in "${TARGETS[@]}"; do
      file="$SRC_DIR/$rel"
      # 转换点 = 任意 <set*Conflict>(conflictText(...)) 持久提示写入（各页 setter 命名不同，
      # 不要固定 setFormConflict，否则会漏计 TransitCenter / MembershipLevelsPage）。
      c=$(grep -oE 'set[A-Za-z0-9_]*Conflict\([[:space:]]*conflictText\(' "$file" 2>/dev/null | wc -l | tr -d ' ')
      c=${c:-0}
      has_pred=0; has_alert=0
      grep -q 'isBusinessCorrectableConflict(' "$file" && has_pred=1
      grep -q '<FormConflictAlert' "$file" && has_alert=1
      page_code_first=0
      if [ "$c" -gt 0 ] && [ "$has_pred" = "1" ] && [ "$has_alert" = "1" ]; then
        page_code_first=1
        PAGES_WITH_CONVERSION=$((PAGES_WITH_CONVERSION + 1))
      fi
      [ "$page_code_first" = "1" ] || CODE_FIRST=0
      CONVERTED=$((CONVERTED + c))
      emit "PAGE=$rel converted=$c code_first=$page_code_first"
    done
    emit "CONVERTED_ROWS=$CONVERTED"
    emit "CODE_FIRST=$CODE_FIRST"
    emit "PAGES_WITH_CONVERSION=$PAGES_WITH_CONVERSION"
    emit "PAGES_TOTAL=${#TARGETS[@]}"
    [ "$CONVERTED" -ge 1 ] || { echo "ASSERT_FAIL: CONVERTED_ROWS<1" >&2; RC=1; }
    [ "$PAGES_WITH_CONVERSION" -eq "${#TARGETS[@]}" ] || { echo "ASSERT_FAIL: PAGES_WITH_CONVERSION!=${#TARGETS[@]}" >&2; RC=1; }
    [ "$CODE_FIRST" = "1" ] || { echo "ASSERT_FAIL: CODE_FIRST!=1" >&2; RC=1; }
    finish R2-converted.txt || RC=1
    ;;

  r3)
    SHARED_REUSE=0; LOCAL_ALERT_DUP=0; LOCAL_CODE_LIST=0
    for rel in "${TARGETS[@]}"; do
      file="$SRC_DIR/$rel"
      if grep -q "from '.*components/FormConflictAlert'" "$file" && grep -q '<FormConflictAlert' "$file"; then
        SHARED_REUSE=$((SHARED_REUSE + 1))
        emit "REUSE=$rel"
      fi
      # 本单页面不得自写 Alert 组件（antd <Alert> 直接使用 / 本地组件定义）
      if grep -qE 'function[[:space:]]+[A-Za-z0-9_]*Alert|const[[:space:]]+[A-Za-z0-9_]*Alert[[:space:]]*=' "$file"; then
        LOCAL_ALERT_DUP=$((LOCAL_ALERT_DUP + 1))
      fi
      if grep -qE '\[[[:space:]]*40002|40004[[:space:]]*,[[:space:]]*40005' "$file"; then
        LOCAL_CODE_LIST=$((LOCAL_CODE_LIST + 1))
      fi
    done
    emit "SHARED_REUSE=$SHARED_REUSE"
    emit "LOCAL_ALERT_DUP=$LOCAL_ALERT_DUP"
    emit "LOCAL_CODE_LIST=$LOCAL_CODE_LIST"
    [ "$SHARED_REUSE" -ge 1 ] || { echo "ASSERT_FAIL: SHARED_REUSE<1" >&2; RC=1; }
    [ "$LOCAL_ALERT_DUP" = "0" ] || { echo "ASSERT_FAIL: LOCAL_ALERT_DUP!=0" >&2; RC=1; }
    [ "$LOCAL_CODE_LIST" = "0" ] || { echo "ASSERT_FAIL: LOCAL_CODE_LIST!=0" >&2; RC=1; }
    finish R3-shared.txt || RC=1
    ;;

  r4)
    ROWS="$(scan_rows)" || { finish R4-residual.txt || true; exit 1; }
    echo "$ROWS" | scan_metrics | while IFS= read -r l; do emit "$l"; done
    KEEP_WITH_REASON=$(printf '%s\n' "$ROWS" | grep $'tier=keep:instant' | grep -c 'reason=.' || true)
    KEEP_ROWS=$(printf '%s\n' "$ROWS" | grep -c $'tier=keep:instant' || true)
    emit "KEEP_WITH_REASON=${KEEP_WITH_REASON:-0}"
    require 'BUSINESS_TOAST_RESIDUAL=0'
    if [ "${KEEP_WITH_REASON:-0}" -lt "${KEEP_ROWS:-0}" ]; then
      echo "ASSERT_FAIL: keep rows without reason" >&2; RC=1
    fi
    finish R4-residual.txt || RC=1
    ;;

  r5)
    BUILD_LOG="$REPO/tmp/verify-2489-r5-build.log"
    if [ -x "$HEAVY_RUN" ]; then
      ( cd frontend-pc && "$HEAVY_RUN" -- npm run build ) > "$BUILD_LOG" 2>&1
    else
      ( cd frontend-pc && npm run build ) > "$BUILD_LOG" 2>&1
    fi
    BUILD_EXIT=$?
    emit "BUILD_CMD=cd frontend-pc && npm run build"
    emit "BUILD_EXIT=$BUILD_EXIT"
    [ "$BUILD_EXIT" = "0" ] && emit "BUILD_OK" || { echo "ASSERT_FAIL: build failed" >&2; RC=1; }
    tail -3 "$BUILD_LOG" | sed 's/^/BUILD_TAIL: /' | while IFS= read -r l; do emit "$l"; done

    LINT_LOG="$REPO/tmp/verify-2489-r5-lint.log"
    LINT_FILES=(
      "src/pages/admin/PermissionManage/index.jsx"
      "src/pages/admin/pricing/MerchantPricingConfig.jsx"
      "src/pages/admin/transit/TransitCenter.jsx"
      "src/pages/System/MembershipLevelsPage.jsx"
      "src/pages/MerchantManagement.jsx"
      "src/pages/TenantManagement.jsx"
    )
    if [ -x "$HEAVY_RUN" ]; then
      ( cd frontend-pc && "$HEAVY_RUN" -- npx eslint "${LINT_FILES[@]}" ) > "$LINT_LOG" 2>&1
    else
      ( cd frontend-pc && npx eslint "${LINT_FILES[@]}" ) > "$LINT_LOG" 2>&1
    fi
    LINT_EXIT=$?
    emit "LINT_CMD=cd frontend-pc && npx eslint ${LINT_FILES[*]}"
    emit "LINT_EXIT=$LINT_EXIT"
    [ "$LINT_EXIT" = "0" ] && emit "ESLINT_OK" || { echo "ASSERT_FAIL: eslint failed" >&2; RC=1; }
    finish R5-build.txt || RC=1
    ;;
esac

exit "$RC"
