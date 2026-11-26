#!/usr/bin/env bash
# =============================================================================
# scripts/scan-form-toasts-2488.sh — 判定表（**只读**扫描，#2488 / #2480 rubric）
#
# 目的：枚举本批 3 个页面的每个 `message.error(` 调用点，逐条给出
#   file:line / enclosing handler / 是否读 code / **可达性（reachable=yes|no）**
#   / tier / reason。
#
# tier 口径（与 #2480 D5 rubric 一致）：
#   keep:instant        瞬时 toast 合理（客户端校验/载荷预检、加载/删除/传输、网络、
#                       权限、5xx，以及共享判定之后的「非业务」分支）
#   business-persisted  该 handler 的 rubric 分支已改持久提示
#                       （handler 内既有 isBusinessCorrectableConflict 又有
#                         set*Conflict(conflictText(...)) 写入）
#   defer:tier2         形态待逐页判定（必须写 reason；本批目标为 0）
#   residual:business-toast  业务可纠正错误仍由 toast 承载（= 缺陷，必须为 0）
#       两个**互不重叠**的谓词（#2488 Rev.5，可分别注入证伪）：
#         · code 型      ：handler 内出现业务码字面量比较（40002/40004/40005/40900）
#                          且该 handler 没有任何持久提示写入
#         · predicate 型 ：handler 内用了共享 predicate，但冲突文案仍走
#                          `message.error(...conflictText(...))`，或该 handler
#                          完全没有 `set*Conflict(conflictText(...))` 写入
#
# 可达性口径（#2488 Rev.5 R1）：
#   reachable=yes  该错误路径由页面真实控件触发（JSX 事件处理器，或 handler 名字
#                  在本文件被引用 ≥2 次 = 定义 + 至少一处挂载）
#   reachable=no   handler 定义后从未被引用（死代码），此时 tier=business-persisted
#                  属「无证据的声明」，计入 UNREACHABLE_CLAIMED
#
# 只读约束：本脚本**不写文件、不访问数据库**（READONLY=1、DB_WRITES=0）。
#
# 用法：bash scripts/scan-form-toasts-2488.sh
#   SCAN_SRC_DIR=<dir>  覆盖扫描根（verify-2488.sh 的缺陷注入用它扫 tmp 副本）
# 退出码：0 = 全部行已分类且无残留；1 = 存在 residual 行或未分类行。
# =============================================================================
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC_DIR="${SCAN_SRC_DIR:-$REPO_ROOT/frontend-pc/src}"
TARGETS=(
    "pages/System/BannerManagePage.jsx"
    "pages/System/MallImageManageCard.jsx"
    "pages/InstrumentStock.jsx"
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
RESIDUAL_CODE_SITES=0
RESIDUAL_PREDICATE_SITES=0
REACHABLE_ROWS=0
UNREACHABLE_ROWS=0
UNREACHABLE_CLAIMED=0
UNCLASSIFIED=0
AMBIGUOUS=""

# 每个 handler 边界一行：`<line> <kind> <name>`
#   kind=named   具名函数/箭头函数（const x = (...) => / function x()）
#   kind=inline  JSX 事件处理器（onClick/onOk/onChange/render/onError/...）
# 内联处理器让「站点归属哪个 handler」不再靠 12 行上下文窗口猜（#2488 Rev.5 R1）。
AWK_BOUNDS='
function named(ln,   s) {
  s = ln
  sub(/^[[:space:]]+/, "", s)
  if (s ~ /^function[[:space:]]+[A-Za-z0-9_]+/) {
    sub(/^function[[:space:]]+/, "", s)
    sub(/[^A-Za-z0-9_].*/, "", s)
    return s
  }
  if (s ~ /^(const|let)[[:space:]]+[A-Za-z0-9_]+[[:space:]]*=[[:space:]]*(async[[:space:]]+)?(function[[:space:]]+)?\(/) {
    sub(/^(const|let)[[:space:]]+/, "", s)
    sub(/[^A-Za-z0-9_].*/, "", s)
    return s
  }
  return ""
}
function inl(ln,   s, h) {
  s = ln
  if (s !~ /(onClick|onChange|onOk|onCancel|onConfirm|beforeUpload|onError|onFinish|render)[[:space:]]*[:=]/) return ""
  if (match(s, /onClick|onChange|onOk|onCancel|onConfirm|beforeUpload|onError|onFinish|render/)) {
    h = substr(s, RSTART, RLENGTH)
    return "inline:" h
  }
  return ""
}
{
  n = named($0)
  if (n != "") { print NR " named " n; next }
  i = inl($0)
  if (i != "") { print NR " inline " i }
}
'

for rel in "${TARGETS[@]}"; do
    file="$SRC_DIR/$rel"
    [ -f "$file" ] || { echo "ERROR: target file missing: $rel" >&2; exit 1; }
    TOTAL_LINES="$(wc -l < "$file" | tr -d ' ')"
    mapfile -t BOUNDS < <(awk "$AWK_BOUNDS" "$file")

    while IFS=: read -r _line _rest; do
        [ -n "${_line:-}" ] || continue

        # ---- handler 归属 + handler 作用域 ----
        start=0; kind="module"; fn="module"
        for b in "${BOUNDS[@]:-}"; do
            [ -n "$b" ] || continue
            read -r bl bk bn <<< "$b"
            [ "$bl" -le "$_line" ] || break
            start="$bl"; kind="$bk"; fn="$bn"
        done
        if [ "$kind" = "inline" ]; then
            end="$_line"
        else
            end="$TOTAL_LINES"
            for b in "${BOUNDS[@]:-}"; do
                [ -n "$b" ] || continue
                read -r bl bk bn <<< "$b"
                if [ "$bl" -gt "$start" ]; then end=$((bl - 1)); break; fi
            done
        fi
        [ "$end" -lt "$_line" ] && end="$_line"
        [ "$start" -lt 1 ] && start=1
        scope="$(sed -n "${start},${end}p" "$file")"
        near_start=$((_line - 3)); [ "$near_start" -lt 1 ] && near_start=1
        near="$(sed -n "${near_start},${_line}p" "$file")"
        site_line="$(sed -n "${_line}p" "$file")"

        # ---- 读 code / rubric / 持久写入 ----
        reads_code=0
        printf '%s\n' "$scope" | grep -qE 'isBusinessCorrectableConflict\(|code[[:space:]]*[=!]==?[[:space:]]*[0-9]+' && reads_code=1
        predicate=0
        printf '%s\n' "$scope" | grep -q 'isBusinessCorrectableConflict(' && predicate=1
        persist_calls="$(printf '%s\n' "$scope" | grep -cE 'set[A-Za-z]*Conflict[[:space:]]*\(')"
        persist_text="$(printf '%s\n' "$scope" | grep -cE 'set[A-Za-z]*Conflict\([[:space:]]*conflictText')"
        biz_code="$(printf '%s\n' "$scope" | grep -oE 'code[[:space:]]*===[[:space:]]*(40002|40004|40005|40900)' | head -1 | grep -oE '[0-9]+$')"

        # ---- tier 判定（顺序即优先级）----
        tier=""
        reason=""
        if [ -n "$biz_code" ] && [ "${persist_calls:-0}" -eq 0 ]; then
            tier="residual:business-toast"
            reason="business-correctable code $biz_code still rendered by a 3s toast"
        elif printf '%s\n' "$site_line" | grep -qE 'message\.error\(.*conflictText\('; then
            tier="residual:business-toast"
            reason="rubric conflict text rendered through message.error (predicate branch reverted to toast)"
        elif printf '%s\n' "$near" | grep -qE 'errorFields|built\.error|不能为空|请选择|请填写|请输入|必填|需要大于|必须在'; then
            tier="keep:instant"
            reason="client-side validation / payload precheck feedback"
        elif [ "$predicate" = "1" ] && [ "${persist_text:-0}" -ge 1 ]; then
            tier="business-persisted"
            reason="rubric branch routed to FormConflictAlert; this toast is the non-business branch"
        elif [ "$predicate" = "1" ]; then
            tier="residual:business-toast"
            reason="rubric predicate present but no persistent conflict write (branch reverted to toast)"
        elif printf '%s\n' "$scope" | grep -qE '\.list\(|\.delete\(|api\.get\(|api\.delete\(|fetch\('; then
            tier="keep:instant"
            reason="list/delete/transport feedback (no business-correctable code)"
        elif printf '%s\n' "$near" | grep -qE '\.message[[:space:]]*\|\|'; then
            tier="keep:instant"
            reason="server message passthrough without a business-correctable code"
        elif printf '%s\n' "$scope" | grep -qE 'catch'; then
            tier="keep:instant"
            reason="transport/network fallback in catch"
        else
            tier="defer:tier2"
            reason="no rubric code and no transport signal; per-page judgement required"
        fi

        # ---- 可达性（claim → 验证）----
        case "$kind" in
            inline) reachable=yes ;;
            named)
                refs="$(grep -cE "(^|[^A-Za-z0-9_])${fn}([^A-Za-z0-9_]|$)" "$file")"
                if [ "${refs:-0}" -ge 2 ]; then
                    reachable=yes
                else
                    reachable=no
                    UNREACHABLE_CLAIMED=$((UNREACHABLE_CLAIMED + 1))
                    reason="$reason; handler $fn is never referenced (dead error path)"
                fi
                ;;
            *) reachable=yes ;;
        esac
        if [ "$reachable" = "yes" ]; then
            REACHABLE_ROWS=$((REACHABLE_ROWS + 1))
        else
            UNREACHABLE_ROWS=$((UNREACHABLE_ROWS + 1))
        fi

        # ambiguous：同一 handler 内混有多个**不同的非成功** code
        # （20000/20100 是成功码，不构成 tier 歧义）
        codes_count="$(printf '%s\n' "$scope" \
            | grep -oE 'code[[:space:]]*===[[:space:]]*[0-9]+' \
            | grep -oE '[0-9]+$' | grep -vE '^(20000|20100)$' | sort -u | wc -l | tr -d ' ')"
        [ "${codes_count:-0}" -gt 1 ] && AMBIGUOUS="${AMBIGUOUS}${rel}:${_line} "

        SCAN_ROWS=$((SCAN_ROWS + 1))
        case "$tier" in
            keep:instant) KEEP_ROWS=$((KEEP_ROWS + 1)) ;;
            business-persisted) PERSISTED_ROWS=$((PERSISTED_ROWS + 1)) ;;
            defer:tier2) DEFER_ROWS=$((DEFER_ROWS + 1)) ;;
            residual:business-toast)
                if [ -n "$biz_code" ] && [ "${persist_calls:-0}" -eq 0 ]; then
                    RESIDUAL_CODE_SITES=$((RESIDUAL_CODE_SITES + 1))
                else
                    RESIDUAL_PREDICATE_SITES=$((RESIDUAL_PREDICATE_SITES + 1))
                fi
                ;;
            *) UNCLASSIFIED=$((UNCLASSIFIED + 1)) ;;
        esac
        [ -n "$tier" ] && CLASSIFIED_ROWS=$((CLASSIFIED_ROWS + 1))

        printf '%s:%s\tfn=%s\treads_code=%s\treachable=%s\ttier=%s\treason=%s\n' \
            "$rel" "$_line" "${fn:-unknown}" "$reads_code" "$reachable" "$tier" "$reason"
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
# LOCAL_ALERT_DUP = 页面**本地重复实现「冲突提示」**的文件数（=0 才合格）。
# 只统计把冲突文案渲染进 antd Alert 的块（块内引用 Conflict / conflictText / formConflict）；
# 页面里与本 Issue 无关的说明性 <Alert type="info">（如 #2514 在库存页加的批量设价说明）
# 不计入——否则该项会变成「页面是否用过 antd Alert」的假阳性。
local_alert_dup_in() {
    awk '
        /<Alert/ { inblock = 1; block = "" }
        inblock { block = block $0 "\n" }
        inblock && (/\/>/ || /<\/Alert>/) {
            if (block ~ /Conflict|conflictText|formConflict/) { print "dup"; exit }
            inblock = 0
        }
    ' "$1" | grep -q dup
}
LOCAL_ALERT_DUP=0
for rel in "${TARGETS[@]}"; do
    local_alert_dup_in "$SRC_DIR/$rel" && LOCAL_ALERT_DUP=$((LOCAL_ALERT_DUP + 1))
done
RESIDUAL_TOTAL=$((RESIDUAL_CODE_SITES + RESIDUAL_PREDICATE_SITES))
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
echo "RESIDUAL_CODE_SITES=$RESIDUAL_CODE_SITES"
echo "RESIDUAL_PREDICATE_SITES=$RESIDUAL_PREDICATE_SITES"
echo "BUSINESS_TOAST_RESIDUAL=$RESIDUAL_TOTAL"
echo "RESIDUAL_PREDICATE_SPLIT=1"
echo "REACHABLE_ROWS=$REACHABLE_ROWS"
echo "UNREACHABLE_ROWS=$UNREACHABLE_ROWS"
echo "UNREACHABLE_CLAIMED=$UNREACHABLE_CLAIMED"
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
if [ "${RESIDUAL_TOTAL:-0}" -ne 0 ]; then
    echo "RESULT: FAIL (business-correctable toasts still present: code=$RESIDUAL_CODE_SITES predicate=$RESIDUAL_PREDICATE_SITES)" >&2
    exit 1
fi
echo "RESULT: PASS (scan rows=$SCAN_ROWS classified=$CLASSIFIED_ROWS residual=0 reachable=$REACHABLE_ROWS)"
