#!/usr/bin/env bash
# =============================================================================
# scripts/verify-2590.sh — #2590「api.md 围栏配对修复 + 文档围栏结构守卫」证据生成器
#
# 用法：bash scripts/verify-2590.sh <r0|r1|r2|r3|r4>
#   r0 docs/evidence/2590/R0-checker-selftest.txt     守卫自检（坏 fixture 必红 / 正常文件零误报）
#   r1 docs/evidence/2590/R1-fence-sweep.txt          docs/**/*.md 全量结构扫描（排除 docs/evidence/**）
#   r2 docs/evidence/2590/R2-api-structure.txt        api.md 结构断言 + §12.4 围栏外断言
#   r3 docs/evidence/2590/R3-red-prefix.txt           红样：git show <fix>^:… 原始 blob 上的同算法判定
#   r4 docs/evidence/2590/R4-content-preservation.txt 内容守恒（TOTAL_DELTA/NONFENCE_DELTA/§12.4 逐字不变）
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`，另起一行 GENERATED_AT_HEAD。
# 本单为纯文档结构改动，无重命令，不经 heavy-run 取锁（计划「变更点 3」）。
# =============================================================================
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
    r0|r1|r2|r3|r4) ;;
    *) echo "usage: $0 <r0|r1|r2|r3|r4>" >&2; exit 2 ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2590
OUT_DIR="docs/evidence/$ISSUE"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
GUARD="scripts/check-doc-fences.sh"
API_DOC="docs/spec/api/api.md"
REQ_HEADING="### 12.4 审计日志"

[ -f "$GUARD" ] || { echo "FATAL: missing $GUARD" >&2; exit 2; }

HEAD_SHA="$(git rev-parse HEAD)"
if [ -z "${FIX_REF:-}" ]; then
    FIX_REF="$(git log --format=%H --grep="^fix(issue-$ISSUE)" --reverse | head -1 2>/dev/null || true)"
fi
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor of HEAD" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
DIRTY=false
[ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR" ':(exclude)tmp')" ] && DIRTY=true
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"

WORK="$(mktemp -d "$REPO/tmp/verify-$ISSUE-XXXXXX")" || exit 2
trap 'rm -rf "$WORK"' EXIT
BODY="$WORK/body.txt"
: > "$BODY"
RC=0
emit() { printf '%s\n' "$*" | tee -a "$BODY" >/dev/null; }
require() { grep -Fq -- "$1" "$BODY" || { echo "ASSERT_FAIL: missing [$1]" >&2; RC=1; }; }
finish() { # artifact
    local name="$1"
    {
        printf '%s\n' "$PROV"
        printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
        cat "$BODY"
    } > "$OUT_DIR/$name"
    cat "$OUT_DIR/$name"
    [ "$RC" = "0" ] || return 1
}

count_fences() { awk '/^ {0,3}```/{c++} END{print c+0}' "$1"; }
count_lines() { awk 'END{print NR+0}' "$1"; }
count_headings() { # 文档标题行总数（^#{1,6} ，不论是否被围栏吞掉）——用于「标题零丢失」守恒断言
    awk '/^#{1,6} /{c++} END{print c+0}' "$1"
}
extract_section_12_4() { # 从 「### 12.4 审计日志」到下一个 ### 标题
    awk '/^### 12\.4 审计日志/{f=1} f && /^### / && !/^### 12\.4 审计日志/{exit} f{print}' "$1"
}

case "$MODE" in
r0)
    NAME="R0-checker-selftest.txt"
    bash "$GUARD" --self-test > "$WORK/r0.log" 2>&1
    R0_RC=$?
    sed 's/^/SELFTEST: /' "$WORK/r0.log" | while IFS= read -r l; do emit "$l"; done
    emit "SELFTEST_EXIT=$R0_RC"
    [ "$R0_RC" = "0" ] || RC=1
    require 'SELFTEST_CASES=4'
    require 'SELFTEST_DETECTED=3'
    require 'SELFTEST_FALSE_POSITIVE=0'
    require 'SELFTEST_CASES=4 SELFTEST_DETECTED=3 SELFTEST_FALSE_POSITIVE=0 RESULT=PASS'
    finish "$NAME" || RC=1
    ;;
r1)
    NAME="R1-fence-sweep.txt"
    ARGS=()
    SKIPPED=0
    while IFS= read -r f; do
        [ -n "$f" ] || continue
        if [ -f "$f" ]; then
            ARGS+=(--file "$f")
        else
            SKIPPED=$((SKIPPED + 1))
            emit "SKIPPED_BROKEN_SYMLINK=$f"
        fi
    done < <(find docs -type f -name '*.md' -not -path 'docs/evidence/*' 2>/dev/null | sort; \
             find docs -type l -name '*.md' -not -path 'docs/evidence/*' 2>/dev/null | sort)
    bash "$GUARD" "${ARGS[@]}" > "$WORK/r1.log" 2>&1
    R1_RC=$?
    grep -E '^(FILE=|VIOLATION |DOC_FILES_SCANNED=)' "$WORK/r1.log" | while IFS= read -r l; do emit "$l"; done
    emit "SKIPPED_BROKEN_SYMLINK_COUNT=$SKIPPED"
    emit "SWEEP_EXIT=$R1_RC"
    [ "$R1_RC" = "0" ] || RC=1
    SCANNED="$(sed -n 's/.*DOC_FILES_SCANNED=\([0-9]*\).*/\1/p' "$WORK/r1.log" | tail -1)"
    [ "${SCANNED:-0}" -ge 9 ] || RC=1
    require 'ODD_FENCE_FILES=0'
    require 'HEADINGS_INSIDE_FILES=0'
    require 'EOF_INSIDE_FILES=0'
    require 'RESULT=PASS'
    finish "$NAME" || RC=1
    ;;
r2)
    NAME="R2-api-structure.txt"
    bash "$GUARD" --file "$API_DOC" --require-heading-outside "$REQ_HEADING" > "$WORK/r2.log" 2>&1
    R2_RC=$?
    grep -E '^(FILE=|VIOLATION |HEADING_STATE=|DOC_FILES_SCANNED=)' "$WORK/r2.log" | while IFS= read -r l; do emit "$l"; done
    FILE_LINE="$(grep -m1 '^FILE=' "$WORK/r2.log" || true)"
    API_FENCES="$(printf '%s' "$FILE_LINE" | sed -n 's/.*FENCES=\([0-9]*\).*/\1/p')"
    API_EVEN="$(printf '%s' "$FILE_LINE" | sed -n 's/.*EVEN=\([0-9]*\).*/\1/p')"
    API_HI="$(printf '%s' "$FILE_LINE" | sed -n 's/.*HEADINGS_INSIDE=\([0-9]*\).*/\1/p')"
    API_EOF="$(printf '%s' "$FILE_LINE" | sed -n 's/.*EOF_INSIDE=\([0-9]*\).*/\1/p')"
    HSTATE="$(printf '%s\n' "$(grep -F "HEADING_STATE=$REQ_HEADING=" "$WORK/r2.log" || true)" | sed -n 's/.*=\(outside\|inside\|missing\)$/\1/p' | head -1)"
    SEC_LINE="$(grep -nF "$REQ_HEADING" "$API_DOC" | head -1 | cut -d: -f1)"
    emit "API_FENCES=${API_FENCES:-0}"
    emit "API_EVEN=${API_EVEN:-0}"
    emit "API_HEADINGS_INSIDE=${API_HI:-0}"
    emit "API_EOF_INSIDE=${API_EOF:-0}"
    emit "HEADING_STATE_12_4=${HSTATE:-missing}"
    emit "SEC_12_4_LINE=${SEC_LINE:-0}"
    emit "API_CHECK_EXIT=$R2_RC"
    [ "$R2_RC" = "0" ] || RC=1
    [ "${API_FENCES:-0}" -ge 590 ] || RC=1
    [ "${SEC_LINE:-0}" -gt 0 ] || RC=1
    require 'API_EVEN=1'
    require 'API_HEADINGS_INSIDE=0'
    require 'API_EOF_INSIDE=0'
    require 'HEADING_STATE_12_4=outside'
    finish "$NAME" || RC=1
    ;;
r3)
    NAME="R3-red-prefix.txt"
    PRE="$WORK/api-pre.md"
    git show "${FIX_SHA}^:$API_DOC" > "$PRE" 2>"$WORK/gitshow.err" || {
        echo "FATAL: cannot read ${FIX_SHA}^:$API_DOC: $(cat "$WORK/gitshow.err")" >&2
        exit 3
    }
    bash "$GUARD" --file "$PRE" --require-heading-outside "$REQ_HEADING" > "$WORK/r3.log" 2>&1
    R3_RC=$?
    # 归一化路径，使 token 与修复后产物同形（避免 tmp 随机目录名进入证据）
    sed "s|$PRE|docs/spec/api/api.md|g" "$WORK/r3.log" | grep -E '^(FILE=|VIOLATION |HEADING_STATE=|DOC_FILES_SCANNED=)' | while IFS= read -r l; do emit "$l"; done
    FILE_LINE="$(grep -m1 '^FILE=' "$WORK/r3.log" || true)"
    RED_FENCES="$(printf '%s' "$FILE_LINE" | sed -n 's/.*FENCES=\([0-9]*\).*/\1/p')"
    RED_EVEN="$(printf '%s' "$FILE_LINE" | sed -n 's/.*EVEN=\([0-9]*\).*/\1/p')"
    RED_EOF="$(printf '%s' "$FILE_LINE" | sed -n 's/.*EOF_INSIDE=\([0-9]*\).*/\1/p')"
    RED_HSTATE="$(grep -F "HEADING_STATE=$REQ_HEADING=" "$WORK/r3.log" | sed -n 's/.*=\(outside\|inside\|missing\)$/\1/p' | head -1)"
    emit "RED_SOURCE=git show ${FIX_SHA}^:$API_DOC"
    emit "RED_CHECKER_EXIT=$R3_RC"
    emit "RED_FENCES=${RED_FENCES:-0}"
    emit "RED_EVEN=${RED_EVEN:-1}"
    emit "RED_EOF_INSIDE=${RED_EOF:-0}"
    emit "RED_HEADING_STATE_12_4=${RED_HSTATE:-missing}"
    [ "$R3_RC" = "1" ] || RC=1
    [ "${RED_FENCES:-0}" = "597" ] || RC=1
    require 'RED_CHECKER_EXIT=1'
    require 'RED_EVEN=0'
    require 'RED_FENCES=597'
    require 'RED_HEADING_STATE_12_4=inside'
    finish "$NAME" || RC=1
    ;;
r4)
    NAME="R4-content-preservation.txt"
    PRE="$WORK/api-pre.md"
    git show "${FIX_SHA}^:$API_DOC" > "$PRE" 2>/dev/null || { echo "FATAL: cannot read pre blob" >&2; exit 3; }
    POST="$API_DOC"
    PRE_LINES="$(count_lines "$PRE")"; POST_LINES="$(count_lines "$POST")"
    TOTAL_DELTA=$((POST_LINES - PRE_LINES))
    PRE_F="$(count_fences "$PRE")"; POST_F="$(count_fences "$POST")"
    PRE_NF=$((PRE_LINES - PRE_F)); POST_NF=$((POST_LINES - POST_F))
    NONFENCE_DELTA=$((POST_NF - PRE_NF))
    PRE_H="$(count_headings "$PRE")"; POST_H="$(count_headings "$POST")"
    HEADINGS_DELTA=$((POST_H - PRE_H))

    diff -u "$PRE" "$POST" > "$WORK/r4.diff" || true
    REMOVED_FENCES="$(grep -E '^-' "$WORK/r4.diff" | grep -v '^---' | grep -cE '^- {0,3}```' || true)"
    ADDED_FENCES="$(grep -E '^\+' "$WORK/r4.diff" | grep -v '^+++' | grep -cE '^\+ {0,3}```' || true)"
    REMOVED_FENCE_LINES=$((REMOVED_FENCES - ADDED_FENCES))

    # 被删的非围栏、非空行必须仍是保留块的内容（重复片段判定）
    DUP_MISS=0
    while IFS= read -r del; do
        printf '%s' "$del" | grep -qE '^[[:space:]]*$' && continue
        printf '%s' "$del" | grep -qE '^[[:space:]]*```' && continue
        grep -Fq -- "$del" "$POST" || { DUP_MISS=$((DUP_MISS + 1)); emit "DUPLICATE_MISSING=$del"; }
    done < <(grep -E '^-' "$WORK/r4.diff" | grep -v '^---' | sed 's/^-//')
    REMOVED_LINES_ARE_DUPLICATES=1
    [ "$DUP_MISS" -eq 0 ] || REMOVED_LINES_ARE_DUPLICATES=0

    extract_section_12_4 "$PRE" > "$WORK/sec-pre.txt"
    extract_section_12_4 "$POST" > "$WORK/sec-post.txt"
    SEC_UNCHANGED=1
    cmp -s "$WORK/sec-pre.txt" "$WORK/sec-post.txt" || SEC_UNCHANGED=0

    emit "# 内容守恒：diff 只含「删 1 围栏 + 插 1 围栏 + 删 7 行重复片段」，正文/标题零丢失"
    emit "PRE_LINES=$PRE_LINES"
    emit "POST_LINES=$POST_LINES"
    emit "TOTAL_DELTA=$TOTAL_DELTA"
    emit "PRE_FENCES=$PRE_F"
    emit "POST_FENCES=$POST_F"
    emit "NONFENCE_DELTA=$NONFENCE_DELTA"
    emit "HEADINGS_PRE=$PRE_H"
    emit "HEADINGS_POST=$POST_H"
    emit "HEADINGS_DELTA=$HEADINGS_DELTA"
    emit "REMOVED_FENCE_LINES=$REMOVED_FENCE_LINES"
    emit "REMOVED_LINES_ARE_DUPLICATES=$REMOVED_LINES_ARE_DUPLICATES"
    emit "SEC_12_4_LINES_PRE=$(count_lines "$WORK/sec-pre.txt")"
    emit "SEC_12_4_UNCHANGED=$SEC_UNCHANGED"
    emit "--- diff ---"
    grep -E '^[-+][^-+]' "$WORK/r4.diff" | while IFS= read -r l; do emit "$l"; done

    [ "$TOTAL_DELTA" = "-7" ] || RC=1
    [ "$NONFENCE_DELTA" = "-6" ] || RC=1
    [ "$HEADINGS_DELTA" = "0" ] || RC=1
    [ "$REMOVED_FENCE_LINES" = "1" ] || RC=1
    [ "$REMOVED_LINES_ARE_DUPLICATES" = "1" ] || RC=1
    [ "$SEC_UNCHANGED" = "1" ] || RC=1
    [ "$(count_lines "$WORK/sec-pre.txt")" -gt 0 ] || RC=1
    require 'TOTAL_DELTA=-7'
    require 'NONFENCE_DELTA=-6'
    require 'HEADINGS_DELTA=0'
    require 'REMOVED_FENCE_LINES=1'
    require 'REMOVED_LINES_ARE_DUPLICATES=1'
    require 'SEC_12_4_UNCHANGED=1'
    finish "$NAME" || RC=1
    ;;
esac

if [ "$RC" != "0" ]; then
    echo "RESULT: FAIL ($MODE)" >&2
    exit 1
fi
echo "RESULT: PASS ($MODE)"
