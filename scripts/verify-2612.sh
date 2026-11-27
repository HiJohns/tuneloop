#!/usr/bin/env bash
# #2612 证据生成器 — PC 端 117 处 catch 直出 e.message 收敛到 userFacingErrorMessage。
#
# 用法：bash scripts/verify-2612.sh <r1..r5|gate>
#   r1  docs/evidence/2612/R1-triage-table.txt   判定表（tier/reachable/fallback 全覆盖）
#   r2  docs/evidence/2612/R2-batch-b1b2.txt      B1+B2 批（52 行 / 10 文件）
#   r3  docs/evidence/2612/R3-batch-b3b4.txt      B3+B4 批（65 行 / 27 文件）
#   r4  docs/evidence/2612/R4-residual-scan.txt   全仓残留 + 改动文件集一致性
#   r5  docs/evidence/2612/R5-build-lint.txt      PC 构建 + ESLint
#   gate                                          定向回归（r1+r2+r3+r4 断言，供 pre-ready P4）
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`，次行 `# tree=<HEAD sha>`。
# 站点口径唯一实现 = scripts/scan-catch-guard-2609.sh（本单新增 --table / --base）。
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
mkdir -p "$REPO/tmp"
ISSUE=2612
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
HEAD_SHA="$(git rev-parse HEAD)"
if [ -z "${FIX_COMMIT:-}" ]; then
  FIX_COMMIT="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
fi
FIX_SHA="$(git rev-parse "${FIX_COMMIT:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: fix commit not ancestor of HEAD" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
BASE_REF="${BASE_REF:-${FIX_SHA}^}"
DIRTY=false
[ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR")" ] && DIRTY=true
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
TREE_LINE="# tree=$HEAD_SHA"

SCANNER="scripts/scan-catch-guard-2609.sh"
SRC="frontend-pc/src"

# 批次文件清单（与 #2612 正文 / 判定表 FILES= 同口径）
B1_FILES="$SRC/pages/System/UserManagement.jsx $SRC/pages/System/GiftPolicies.jsx $SRC/pages/System/AuditLogPage.jsx $SRC/pages/System/TechnicianProfiles.jsx"
B2_FILES="$SRC/pages/admin/instrument/Detail.jsx $SRC/pages/admin/instrument/List.jsx $SRC/pages/admin/property/List.jsx $SRC/pages/admin/category/List.jsx $SRC/pages/admin/instrument/BatchImport.jsx $SRC/pages/admin/instrument/Form.jsx"
B3_FILES="$SRC/pages/admin/inventory/RentSetting.jsx $SRC/pages/admin/face-review/index.jsx $SRC/pages/admin/warnings/WarningSettings.jsx $SRC/pages/admin/warnings/SmtpSettings.jsx $SRC/pages/admin/platform-staff/index.jsx $SRC/pages/admin/pricing/MerchantPricingConfig.jsx $SRC/pages/admin/audit/DamageAssessment.jsx $SRC/pages/admin/transit/TransitCenter.jsx $SRC/pages/admin/payments/PaymentList.jsx $SRC/pages/admin/billing/BillingDashboard.jsx"
B4_FILES="$SRC/pages/StaffManagement.jsx $SRC/pages/WarehouseManagement.jsx $SRC/pages/merchant/NotificationSettings.jsx $SRC/pages/StaffBulkImport.jsx $SRC/pages/SiteManagement.jsx $SRC/pages/SiteBulkImport.jsx $SRC/pages/LeaseLedger.jsx $SRC/pages/InviteManagement.jsx $SRC/pages/DirectMemberManagement.jsx $SRC/pages/DepositFlow.jsx $SRC/components/IdPhotoDisplay.jsx $SRC/pages/StaffResetPassword.jsx $SRC/pages/StaffEdit.jsx $SRC/pages/MerchantManagement.jsx $SRC/pages/IAMSyncPage.jsx $SRC/pages/FinanceConfig.jsx $SRC/components/MerchantMemberManagement.jsx"
B1B2_FILES="$B1_FILES $B2_FILES"
B3B4_FILES="$B3_FILES $B4_FILES"

HEAVY_RUN="$HOME/.config/opencode/scripts/heavy-run.sh"

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

count_sites() { # "<file list>" [base] -> PRE_SITES of that subset
  local files="$1" base="$2"
  if [ -n "$base" ]; then
    bash "$SCANNER" --table --base "$base" --files "$files" | sed -n 's/^PRE_SITES=//p'
  else
    bash "$SCANNER" --table --files "$files" | sed -n 's/^PRE_SITES=//p'
  fi
}

LOG="$REPO/tmp/verify-2612-${MODE}.log"
: > "$LOG"
RC=0
ART=""
NAME=""

case "$MODE" in
  r1)
    NAME="R1-triage-table.txt"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      echo "SCAN_CMD=bash $SCANNER --table --base $BASE_REF"
      echo "SCAN_BASE=$BASE_REF"
      bash "$SCANNER" --table --base "$BASE_REF" 2>&1
      echo "SCAN_EXIT=$?"
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'PRE_SITES=117' 'PRE_FILES=37' 'TIER_ROWS=117' \
      'UNCLASSIFIED_ROWS=0' 'FALLBACK_ASCII_LEAK=0' 'NO_FALLBACK_ROWS=0' \
      'TIER_T1_T6_SUM=117' 'SCAN_EXIT=0' || RC=1
    ;;
  r2|r3)
    if [ "$MODE" = "r2" ]; then
      NAME="R2-batch-b1b2.txt"; FILES="$B1B2_FILES"; WANT="B1_SITES=14 B2_SITES=38 B1B2_GUARDED=52"
    else
      NAME="R3-batch-b3b4.txt"; FILES="$B3B4_FILES"; WANT="B3_SITES=24 B4_SITES=41 B3B4_GUARDED=65"
    fi
    B1_SITES="$(count_sites "$B1_FILES" "$BASE_REF")"
    B2_SITES="$(count_sites "$B2_FILES" "$BASE_REF")"
    B3_SITES="$(count_sites "$B3_FILES" "$BASE_REF")"
    B4_SITES="$(count_sites "$B4_FILES" "$BASE_REF")"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      echo "SCAN_CMD=bash $SCANNER --files \"<batch files>\""
      echo "B1_SITES=$B1_SITES"
      echo "B2_SITES=$B2_SITES"
      echo "B3_SITES=$B3_SITES"
      echo "B4_SITES=$B4_SITES"
      # shellcheck disable=SC2086
      bash "$SCANNER" --files "$FILES" 2>&1
    } > "$LOG" 2>&1
    GUARDED="$(sed -n 's/^CATCH_GUARDED=//p' "$LOG" | tail -1)"
    RESIDUAL="$(sed -n 's/^CATCH_RAW=//p' "$LOG" | tail -1)"
    if [ "$MODE" = "r2" ]; then
      echo "B1B2_GUARDED=$GUARDED" >> "$LOG"
      echo "B1B2_RESIDUAL=$RESIDUAL" >> "$LOG"
      require_tokens "$LOG" $WANT 'B1B2_GUARDED=52' 'B1B2_RESIDUAL=0' 'CATCH_FALLBACK_OK=52' || RC=1
    else
      echo "B3B4_GUARDED=$GUARDED" >> "$LOG"
      echo "B3B4_RESIDUAL=$RESIDUAL" >> "$LOG"
      require_tokens "$LOG" $WANT 'B3B4_GUARDED=65' 'B3B4_RESIDUAL=0' 'CATCH_FALLBACK_OK=65' || RC=1
    fi
    ;;
  r4)
    NAME="R4-residual-scan.txt"
    DIFF_FILES="$(git diff --name-only "$BASE_REF"..HEAD -- "$SRC" | sed '/^$/d' | sort -u)"
    DIFF_COUNT="$(printf '%s\n' "$DIFF_FILES" | sed '/^$/d' | wc -l | tr -d ' ')"
    TABLE_FILES="$(bash "$SCANNER" --table --base "$BASE_REF" | sed -n 's/^FILES=//p' | tr ' ' '\n' | sed '/^$/d' | sort -u)"
    TABLE_COUNT="$(printf '%s\n' "$TABLE_FILES" | sed '/^$/d' | wc -l | tr -d ' ')"
    FILES_MATCH=0
    [ "$DIFF_COUNT" = "$TABLE_COUNT" ] && [ -z "$(diff <(printf '%s\n' "$DIFF_FILES") <(printf '%s\n' "$TABLE_FILES"))" ] && FILES_MATCH=1
    # #2609 三页必须未被本单触碰
    EXCL_PAGES="$SRC/pages/System/BannerManagePage.jsx $SRC/pages/System/MallImageManageCard.jsx $SRC/InstrumentStock.jsx"
    EXCL_UNTOUCHED=0
    for p in $EXCL_PAGES; do
      printf '%s\n' "$DIFF_FILES" | grep -Fxq "$p" || EXCL_UNTOUCHED=$((EXCL_UNTOUCHED + 1))
    done
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      echo "SCAN_CMD=bash $SCANNER (full src)"
      bash "$SCANNER" 2>&1
      echo "GIT_DIFF_FILES=$DIFF_COUNT"
      echo "TABLE_FILES=$TABLE_COUNT"
      echo "FILES_MATCH_TABLE=$FILES_MATCH"
      echo "INSCOPE_EXCLUDE_2609=$EXCL_UNTOUCHED"
      echo "DIFF_FILES:"
      printf '%s\n' "$DIFF_FILES"
    } > "$LOG" 2>&1
    RAW="$(sed -n 's/^CATCH_RAW=//p' "$LOG" | tail -1)"
    TBL_RAW="$(sed -n 's/^CATCH_RAW=//p' "$LOG" | head -1)"
    echo "RESIDUAL_PC_RAW=$RAW" >> "$LOG"
    if [ "$TBL_RAW" = "0" ]; then
      echo "OUT_OF_SCOPE_SITES=0" >> "$LOG"
    else
      echo "OUT_OF_SCOPE_SITES=$TBL_RAW" >> "$LOG"
    fi
    require_tokens "$LOG" 'RESIDUAL_PC_RAW=0' 'OUT_OF_SCOPE_SITES=0' \
      'INSCOPE_EXCLUDE_2609=3' 'GIT_DIFF_FILES=37' 'FILES_MATCH_TABLE=1' || RC=1
    ;;
  r5)
    NAME="R5-build-lint.txt"
    LINT_FILES="$(git diff --name-only "$BASE_REF"..HEAD -- "$SRC" | grep -E '\.(js|jsx)$' | tr '\n' ' ')"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      echo "BUILD_CMD=cd frontend-pc && npm run build (via heavy-run.sh)"
      ( cd "$REPO/frontend-pc" && "$HEAVY_RUN" --timeout 1800 -- npm run build ) > "$REPO/tmp/verify-2612-build.log" 2>&1
      PC_BUILD=$?
      echo "PC_BUILD=$PC_BUILD"
      tail -6 "$REPO/tmp/verify-2612-build.log"
      echo "LINT_CMD=cd frontend-pc && npx eslint <changed files>"
      LINT_LOG="$REPO/tmp/verify-2612-eslint.log"
      # shellcheck disable=SC2086
      ( cd "$REPO/frontend-pc" && npx eslint $(printf '%s\n' $LINT_FILES | sed "s#^$SRC/#src/#") ) > "$LINT_LOG" 2>&1
      LINT_RC=$?
      echo "LINT_EXIT=$LINT_RC"
      tail -12 "$LINT_LOG"
      if [ "$PC_BUILD" = "0" ] && [ "$LINT_RC" = "0" ]; then
        echo "BUILD_LINT_OK=1"
      else
        echo "BUILD_LINT_OK=0"
      fi
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'PC_BUILD=0' 'LINT_EXIT=0' 'BUILD_LINT_OK=1' || RC=1
    ;;
  gate)
    NAME=""
    DIFF_COUNT="$(git diff --name-only "$BASE_REF"..HEAD -- "$SRC" | sed '/^$/d' | wc -l | tr -d ' ')"
    TABLE_COUNT="$(bash "$SCANNER" --table --base "$BASE_REF" | sed -n 's/^FILES=//p' | tr ' ' '\n' | sed '/^$/d' | wc -l | tr -d ' ')"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      echo "GATE_MODE=directed (scanner residual + batch counts + diff/tables + fallback)"
      bash "$SCANNER" --table --base "$BASE_REF" 2>&1 | grep -E '^(PRE_|TIER_|UNCLASSIFIED_|FALLBACK_|NO_FALLBACK_|SCAN_EXIT)'
      bash "$SCANNER" 2>&1
      echo "B1_SITES=$(count_sites "$B1_FILES" "$BASE_REF")"
      echo "B2_SITES=$(count_sites "$B2_FILES" "$BASE_REF")"
      echo "B3_SITES=$(count_sites "$B3_FILES" "$BASE_REF")"
      echo "B4_SITES=$(count_sites "$B4_FILES" "$BASE_REF")"
      echo "GIT_DIFF_FILES=$DIFF_COUNT"
      echo "TABLE_FILES=$TABLE_COUNT"
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'PRE_SITES=117' 'PRE_FILES=37' 'UNCLASSIFIED_ROWS=0' \
      'FALLBACK_ASCII_LEAK=0' 'NO_FALLBACK_ROWS=0' 'CATCH_RAW=0' \
      'CATCH_NETWORK_FALLBACK=1' 'B1_SITES=14' 'B2_SITES=38' 'B3_SITES=24' 'B4_SITES=41' \
      'GIT_DIFF_FILES=37' 'TABLE_FILES=37' || RC=1
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
if [ -n "$ART" ]; then
  cp "$LOG" "$ART"
  echo "ARTIFACT=$ART" >&2
fi
tail -1 "$LOG" >&2
if [ "$RC" != "0" ]; then
  exit 1
fi
exit 0
