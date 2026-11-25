#!/usr/bin/env bash
# =============================================================================
# scripts/scan-form-toasts-2489.sh — 判定表（**只读**扫描，#2489 / #2480 rubric）
#
# 目的：枚举本批 6 个文件的每个 `message.error(` 调用点，逐条给出
#   file:line / enclosing function / 是否读 code / tier / reason。
#
# tier 口径（与 #2480 D5 rubric 一致）：
#   keep:instant        瞬时 toast 合理（客户端校验、加载/网络、权限、5xx、
#                       以及已在共享判定之后出现的「非业务」兜底分支）
#   business-persisted  该调用点的业务可纠正分支已改持久提示
#                       （同一函数内存在 isBusinessCorrectableConflict / FormConflictAlert）
#   defer:tier2         形态待逐页判定（本批不猜改）
#   residual:business-toast  业务可纠正 code 仍走 toast（= 缺陷，必须为 0）
#
# 只读约束：本脚本**不写文件、不访问数据库**（READONLY=1、DB_WRITES=0）。
#
# 用法：bash scripts/scan-form-toasts-2489.sh
# 退出码：0 = 全部行已分类；1 = 存在 residual 行或未分类行。
# =============================================================================
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC_DIR="$REPO_ROOT/frontend-pc/src"
TARGETS=(
    "pages/admin/PermissionManage/index.jsx"
    "pages/admin/pricing/MerchantPricingConfig.jsx"
    "pages/admin/transit/TransitCenter.jsx"
    "pages/System/MembershipLevelsPage.jsx"
    "pages/MerchantManagement.jsx"
    "pages/TenantManagement.jsx"
)

READONLY=1
DB_WRITES=0

command -v grep >/dev/null 2>&1 || { echo "ERROR: grep not found" >&2; exit 1; }
[ -d "$SRC_DIR" ] || { echo "ERROR: source dir not found: $SRC_DIR" >&2; exit 1; }

SCAN_ROWS=0
CLASSIFIED_ROWS=0
KEEP_ROWS=0
PERSISTED_ROWS=0
DEFER_ROWS=0
RESIDUAL_ROWS=0
UNCLASSIFIED=0
AMBIGUOUS=""

# enclosing function name (nearest `const x = ` / `function x` above the line)
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

for rel in "${TARGETS[@]}"; do
    file="$SRC_DIR/$rel"
    [ -f "$file" ] || { echo "ERROR: target file missing: $rel" >&2; exit 1; }

    while IFS=: read -r _line _rest; do
        [ -n "${_line:-}" ] || continue
        start=$((_line - 12))
        [ "$start" -lt 1 ] && start=1
        ctx="$(sed -n "${start},${_line}p" "$file")"
        fn="$(enclosing_fn "$file" "$_line")"
        tier=""
        reason=""
        code_aware=0

        # 读 code：同一函数上下文出现共享判定或显式 code 比较
        if printf '%s\n' "$ctx" | grep -qE 'isBusinessCorrectableConflict\(|code[[:space:]]*===?[[:space:]]*[0-9]+'; then
            code_aware=1
        fi

        business_code="$(printf '%s\n' "$ctx" \
            | grep -oE 'code[[:space:]]*===[[:space:]]*[0-9]+' \
            | grep -oE '[0-9]+$' \
            | grep -E '^(40002|40004|40005|40900)$' | head -1)"

        if [ -n "$business_code" ]; then
            tier="residual:business-toast"
            reason="business-correctable code $business_code still rendered by a 3s toast"
        elif printf '%s\n' "$ctx" | grep -q 'isBusinessCorrectableConflict('; then
            tier="business-persisted"
            reason="business branch routed to FormConflictAlert; this toast is the non-business fallback"
        elif printf '%s\n' "$ctx" | grep -qE 'errorFields|不能为空|需要大于|必须在|必填'; then
            tier="keep:instant"
            reason="client-side validation feedback"
        elif printf '%s\n' "$ctx" | grep -qE 'api\.get\(|api\.delete\(|console\.error|加载'; then
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

        # ambiguous：同一上下文混有多个不同 code
        codes_count="$(printf '%s\n' "$ctx" | grep -oE 'code[[:space:]]*===[[:space:]]*[0-9]+' | sort -u | wc -l | tr -d ' ')"
        [ "${codes_count:-0}" -gt 1 ] && AMBIGUOUS="${AMBIGUOUS}${rel}:${_line} "

        SCAN_ROWS=$((SCAN_ROWS + 1))
        case "$tier" in
            keep:instant) KEEP_ROWS=$((KEEP_ROWS + 1)) ;;
            business-persisted) PERSISTED_ROWS=$((PERSISTED_ROWS + 1)) ;;
            defer:tier2) DEFER_ROWS=$((DEFER_ROWS + 1)) ;;
            residual:business-toast) RESIDUAL_ROWS=$((RESIDUAL_ROWS + 1)) ;;
            *) UNCLASSIFIED=$((UNCLASSIFIED + 1)) ;;
        esac
        [ -n "$tier" ] && CLASSIFIED_ROWS=$((CLASSIFIED_ROWS + 1))

        printf '%s:%s\tfn=%s\treads_code=%s\ttier=%s\treason=%s\n' \
            "$rel" "$_line" "${fn:-unknown}" "$code_aware" "$tier" "$reason"
    done < <(grep -n 'message\.error(' "$file")
done

SHARED_REUSE=0
LOCAL_CODE_LIST=0
CONVERTED_SITES=0
for rel in "${TARGETS[@]}"; do
    file="$SRC_DIR/$rel"
    grep -q "FormConflictAlert" "$file" && SHARED_REUSE=$((SHARED_REUSE + 1))
    # 禁止页面自写 code 白名单（数组/字面量枚举）
    grep -qE '\[[[:space:]]*40002|40004[[:space:]]*,[[:space:]]*40005' "$file" && LOCAL_CODE_LIST=$((LOCAL_CODE_LIST + 1))
    CONVERTED_SITES=$((CONVERTED_SITES + $(grep -c 'isBusinessCorrectableConflict(' "$file")))
done
LOCAL_ALERT_DUP=0
for rel in "${TARGETS[@]}"; do
    grep -q '<Alert' "$SRC_DIR/$rel" && LOCAL_ALERT_DUP=$((LOCAL_ALERT_DUP + 1))
done
AMBIGUOUS_TRIM="$(printf '%s' "$AMBIGUOUS" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"

echo "SCAN_ROWS=$SCAN_ROWS"
echo "CLASSIFIED_ROWS=$CLASSIFIED_ROWS"
echo "UNCLASSIFIED_ROWS=$UNCLASSIFIED"
echo "FILES=${#TARGETS[@]}"
echo "SHARED_REUSE=$SHARED_REUSE"
echo "CONVERTED_SITES=$CONVERTED_SITES"
echo "KEEP_ROWS=$KEEP_ROWS"
echo "PERSISTED_ROWS=$PERSISTED_ROWS"
echo "DEFER_ROWS=$DEFER_ROWS"
echo "BUSINESS_TOAST_RESIDUAL=$RESIDUAL_ROWS"
echo "LOCAL_CODE_LIST=$LOCAL_CODE_LIST"
echo "LOCAL_ALERT_DUP=$LOCAL_ALERT_DUP"
echo "AMBIGUOUS=$AMBIGUOUS_TRIM"
echo "READONLY=$READONLY"
echo "DB_WRITES=$DB_WRITES"

if [ "${SCAN_ROWS:-0}" -le 0 ]; then
    echo "RESULT: FAIL (no message.error call sites found)" >&2
    exit 1
fi
if [ "${CLASSIFIED_ROWS:-0}" -ne "${SCAN_ROWS:-0}" ]; then
    echo "RESULT: FAIL (unclassified rows: $SCAN_ROWS scanned vs $CLASSIFIED_ROWS classified)" >&2
    exit 1
fi
if [ "${RESIDUAL_ROWS:-0}" -ne 0 ]; then
    echo "RESULT: FAIL (business-correctable toasts still present: $RESIDUAL_ROWS)" >&2
    exit 1
fi
echo "RESULT: PASS (scan rows=$SCAN_ROWS classified=$CLASSIFIED_ROWS residual=0)"
