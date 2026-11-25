#!/usr/bin/env bash
# =============================================================================
# scripts/scan-form-error-toasts-2480.sh — Tier 2 输入清单（**只读**扫描，#2480 v2 / D5）
#
# 目的：枚举 frontend-pc/src 全部 `message.error(` 调用点，按 #2480 唯一 rubric 分类：
#   converted     Tier 1 已转持久提示（业务可纠正 code + 本单两页）
#   keep:instant  瞬时 toast 合理（成功提示、网络/异常兜底、权限、Modal 引导分流）
#   defer:tier2   形态待逐页判定 → 输入 #2485
#
# 该脚本**绝不写文件、绝不访问数据库**（READONLY=1、DB_WRITES=0），
# 只把分类清单打到 stdout，供 R11 证据与 #2485 使用。
#
# 用法：bash scripts/scan-form-error-toasts-2480.sh
# 退出码：0 = 扫描完成且全部行已分类；1 = 存在未分类行或扫描失败。
# =============================================================================
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC_DIR="$REPO_ROOT/frontend-pc/src"
TARGETS=("SiteManagement.jsx" "StaffManagement.jsx")

READONLY=1
DB_WRITES=0

command -v grep >/dev/null 2>&1 || { echo "ERROR: grep not found" >&2; exit 1; }
[ -d "$SRC_DIR" ] || { echo "ERROR: source dir not found: $SRC_DIR" >&2; exit 1; }

code_in_context() { # context: one code number or none
    local ctx="$1"
    printf '%s\n' "$ctx" | grep -oE 'code[[:space:]]*===[[:space:]]*[0-9]+' \
        | grep -oE '[0-9]+$' | head -1
}

is_target_page() { # basename
    local name="$1" t
    for t in "${TARGETS[@]}"; do
        [ "$name" = "$t" ] && return 0
    done
    return 1
}

SCAN_ROWS=0
CLASSIFIED_ROWS=0
TIER1_SITES=""
UNCLASSIFIED=0
AMBIGUOUS=""
DEFER_ROWS=0
KEEP_ROWS=0
CONVERTED_ROWS=0
RESIDUAL_ROWS=0

while IFS=: read -r file line _rest; do
    [ -n "${file:-}" ] && [ -n "${line:-}" ] || continue
    [ -f "$file" ] || continue
    base="$(basename "$file")"
    start=$((line - 8))
    [ "$start" -lt 1 ] && start=1
    ctx="$(sed -n "${start},${line}p" "$file")"
    code="$(code_in_context "$ctx")"
    modality="toast"
    reason=""

    codes_count="$(printf '%s\n' "$ctx" | grep -oE 'code[[:space:]]*===[[:space:]]*[0-9]+' | sort -u | wc -l | tr -d ' ')"
    if [ "${codes_count:-0}" -gt 1 ]; then
        AMBIGUOUS="${AMBIGUOUS}${file}:${line} "
    fi

    if printf '%s\n' "$ctx" | grep -q 'Modal'; then
        modality="modal-guided"
    fi

    if [ -n "$code" ]; then
        case "$code" in
            40002|40004|40005|40900)
                if is_target_page "$base"; then
                    reason="business-correctable code $code still on toast in a Tier 1 page"
                    tier="residual:tier1"
                else
                    reason="business-correctable code $code outside Tier 1 pages"
                    tier="defer:tier2"
                fi
                ;;
            40901|40902)
                reason="code $code = existing account, Modal guided (D4)"
                tier="keep:instant"
                ;;
            *)
                reason="code $code not a user-correctable conflict"
                tier="keep:instant"
                ;;
        esac
    elif [ "$modality" = "modal-guided" ]; then
        reason="Modal guided flow"
        tier="keep:instant"
    elif printf '%s\n' "$ctx" | grep -qE 'err(or)?\.message|网络|请求失败|超时|timeout'; then
        reason="transport/unknown error fallback"
        tier="keep:instant"
    elif printf '%s\n' "$ctx" | grep -qE '权限|暂无权限|403'; then
        reason="permission feedback"
        tier="keep:instant"
    else
        reason="needs per-page judgement"
        tier="defer:tier2"
    fi

    SCAN_ROWS=$((SCAN_ROWS + 1))
    case "$tier" in
        converted)
            CONVERTED_ROWS=$((CONVERTED_ROWS + 1))
            ;;
        residual:tier1) RESIDUAL_ROWS=$((RESIDUAL_ROWS + 1)) ;;
        keep:instant) KEEP_ROWS=$((KEEP_ROWS + 1)) ;;
        defer:tier2) DEFER_ROWS=$((DEFER_ROWS + 1)) ;;
        *)
            UNCLASSIFIED=$((UNCLASSIFIED + 1))
            ;;
    esac
    [ -n "$tier" ] && CLASSIFIED_ROWS=$((CLASSIFIED_ROWS + 1))

    printf '%s:%s\ttier=%s\tcode=%s\tmodal=%s\treason=%s\n' \
        "${file#"$REPO_ROOT"/}" "$line" "$tier" "${code:-none}" \
        "$([ "$modality" = "modal-guided" ] && echo 1 || echo 0)" "$reason"
done < <(grep -rn 'message\.error(' "$SRC_DIR" --include='*.jsx' --include='*.js' | sort)

# Tier 1 页面（本单两页）已转换的判据：引用共享件 FormConflictAlert + 维护冲突 state。
TIER1_SITES_COUNT=0
TIER1_CONVERTED=0
SHARED_REUSE=0
for t in "${TARGETS[@]}"; do
    f="$SRC_DIR/pages/$t"
    [ -f "$f" ] || continue
    TIER1_SITES_COUNT=$((TIER1_SITES_COUNT + 1))
    if grep -q "FormConflictAlert" "$f"; then
        SHARED_REUSE=$((SHARED_REUSE + 1))
    fi
    if grep -q "FormConflictAlert" "$f" && grep -q "setFormConflict(" "$f"; then
        TIER1_CONVERTED=$((TIER1_CONVERTED + 1))
    fi
done
AMBIGUOUS_TRIM="$(printf '%s' "$AMBIGUOUS" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"

echo "SCAN_ROWS=$SCAN_ROWS"
echo "CLASSIFIED_ROWS=$CLASSIFIED_ROWS"
echo "UNCLASSIFIED_ROWS=$UNCLASSIFIED"
echo "TIER1_SITES=${TIER1_SITES_COUNT:-0}"
echo "TIER1_CONVERTED=${TIER1_CONVERTED:-0}"
echo "SHARED_REUSE=${SHARED_REUSE:-0}"
echo "TIER1_RESIDUAL_TOAST=${RESIDUAL_ROWS:-0}"
echo "CONVERTED_ROWS=$CONVERTED_ROWS"
echo "KEEP_ROWS=$KEEP_ROWS"
echo "DEFER_ROWS=$DEFER_ROWS"
echo "AMBIGUOUS=${AMBIGUOUS_TRIM}"
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
echo "RESULT: PASS (scan rows=$SCAN_ROWS classified=$CLASSIFIED_ROWS)"
