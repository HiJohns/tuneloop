#!/usr/bin/env bash
# scripts/verify-2470.sh — #2470「IAM 孤儿面板看不到无组织关系孤儿」证据生成器（R1–R9）。
#
# 用法：bash scripts/verify-2470.sh <r1|r2|r3|r4|r5|r6|r7|r8|r9>
#
# 每行把结果写入 docs/evidence/2470/R*-*.txt（首行 provenance），并打印机器可判定的
# 断言键值；任一行断言不成立以非 0 退出。
#
# 判定口径（Accepted Plan v2）：
#   R1 新端点消费（路径/参数/解析，Unsupported=false）
#   R2 旧 IAM 404 回退（Unsupported=true，日志 fallback）
#   R3 面板可见性（无关系 pending 孤儿出现在 list；本地有行者排除）
#   R4 回退标记透出（响应 fallback_unsupported=true + PC 页面中性提示）
#   R5 判定语义不变（本地行 / org_id / 年龄 三条负例全部排除）
#   R7 清理同源（无关系孤儿 purged=1，不出现 not an orphan；含回退路径实时复算）
#   R6 构建与定向回归（go build + go test + PC 构建）
#   R8 全量后端回归（HEAD 与 origin/develop 基线各跑一次 go test -json ./...，按
#      (package::test) 归一化失败身份，断言「HEAD 失败集 ⊆ 基线」+ PANIC_COUNT=0；
#      口径见 Accepted Plan Rev.3（基线差异，逐单 gate；pre-51250 P5 同源））
#   R9 接口/镜像语义文档（api.md 端点小节 + iam-notes 上游契约）
set -uo pipefail

ROW="${1:-}"
REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2470
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"

export PATH="/usr/local/go/bin:$PATH"
# 本机测试库（与既有 handler 集成测试同一实例）
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_local}"
export TMPDIR="${TMPDIR:-$REPO/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"
HEAVY_RUN="${OPENCODE_HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"

HEAD_SHA="$(git rev-parse HEAD)"
FIX_REF="${FIX_REF:-$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)}"
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short=8 "$FIX_SHA")"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)docs/evidence/$ISSUE" ':(exclude)tmp')" ]; then
    DIRTY=true
fi
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
TREE_LINE="# tree=$HEAD_SHA"

BODY="$(mktemp "$REPO/tmp/verify-$ISSUE-$ROW.XXXXXX")" || exit 2
trap 'rm -f "$BODY"' EXIT
RC=0
emit() { printf '%s\n' "$*" | tee -a "$BODY"; }
require() { grep -Fq -- "$1" "$BODY" || { echo "ASSERT_FAIL: missing [$1]" >&2; RC=1; }; }
finish() { # artifact
    local name="$1"
    {
        printf '%s\n%s\n' "$PROV" "$TREE_LINE"
        cat "$BODY"
    } > "$OUT_DIR/$name"
    cat "$OUT_DIR/$name"
    [ "$RC" = "0" ] || return 1
}

run_go_test() { # pkg run-regex logfile
    ( cd "$REPO/backend" && go test "$1" -run "$2" -v -count=1 -timeout 10m ) > "$3" 2>&1
}

heavy() { # host-lock wrapper（#69/#123：重命令只在命令期间持锁）
    if [ -x "$HEAVY_RUN" ]; then
        "$HEAVY_RUN" --timeout 2400 -- "$@"
    else
        "$@"
    fi
}

usage() { echo "usage: $0 <r1|r2|r3|r4|r5|r6|r7|r8|r9>" >&2; exit 2; }

case "$ROW" in
r1)
    LOG="$REPO/tmp/verify-$ISSUE-r1.log"
    run_go_test ./services 'TestListOrphanCandidates_2470$' "$LOG"; TRC=$?
    grep -E 'PATH=|MIN_AGE=|UNSUPPORTED=' "$LOG" | sed 's/^[[:space:]]*[^:]*: //' | while IFS= read -r l; do emit "$l"; done
    emit "GO_TEST_EXIT=$TRC"
    [ "$TRC" = "0" ] || RC=1
    require 'PATH=/api/v1/users/orphans'
    require 'MIN_AGE=60'
    require 'UNSUPPORTED=0'
    finish R1-client-orphans.txt || RC=1
    ;;
r2)
    LOG="$REPO/tmp/verify-$ISSUE-r2.log"
    run_go_test ./services 'TestListOrphanCandidatesFallback_2470|TestListOrphanCandidatesProtocolErrorFallsBack_2470' "$LOG"; TRC=$?
    grep -E 'UNSUPPORTED=|FALLBACK=' "$LOG" | sed 's/^[[:space:]]*[^:]*: //' | while IFS= read -r l; do emit "$l"; done
    LOG_FALLBACK=0
    grep -Fq 'fallback to scoped pending list' "$LOG" && LOG_FALLBACK=1
    emit "LOG_FALLBACK=$LOG_FALLBACK"
    emit "GO_TEST_EXIT=$TRC"
    [ "$TRC" = "0" ] || RC=1
    [ "$LOG_FALLBACK" = "1" ] || RC=1
    require 'UNSUPPORTED=1'
    require 'FALLBACK=1'
    finish R2-client-fallback.txt || RC=1
    ;;
r3)
    LOG="$REPO/tmp/verify-$ISSUE-r3.log"
    run_go_test ./handlers 'TestIAMOrphanPanelNoRelationVisible_2470' "$LOG"; TRC=$?
    grep -E 'LIST_HAS_NOREL=|LIST_EXCLUDES_LOCAL=' "$LOG" | sed 's/^[[:space:]]*[^:]*: //' | while IFS= read -r l; do emit "$l"; done
    emit "GO_TEST_EXIT=$TRC"
    [ "$TRC" = "0" ] || RC=1
    require 'LIST_HAS_NOREL=1'
    require 'LIST_EXCLUDES_LOCAL=1'
    finish R3-panel-visibility.txt || RC=1
    ;;
r4)
    LOG="$REPO/tmp/verify-$ISSUE-r4.log"
    run_go_test ./handlers 'TestIAMOrphanPanelFallbackFlag_2470' "$LOG"; TRC=$?
    grep -E 'FALLBACK_FLAG=' "$LOG" | sed 's/^[[:space:]]*[^:]*: //' | while IFS= read -r l; do emit "$l"; done
    PAGE_HINT=0
    grep -Fq '当前 IAM 版本不支持全量孤儿扫描（仅显示可见 pending）' frontend-pc/src/pages/System/UserManagement.jsx && PAGE_HINT=1
    emit "PAGE_HINT=$PAGE_HINT"
    emit "GO_TEST_EXIT=$TRC"
    [ "$TRC" = "0" ] || RC=1
    [ "$PAGE_HINT" = "1" ] || RC=1
    require 'FALLBACK_FLAG=1'
    finish R4-fallback-flag.txt || RC=1
    ;;
r5)
    LOG="$REPO/tmp/verify-$ISSUE-r5.log"
    run_go_test ./handlers 'TestIAMOrphanPanelNoRelationVisible_2470' "$LOG"; TRC=$?
    grep -E 'EXCLUDED_ALL=' "$LOG" | sed 's/^[[:space:]]*[^:]*: //' | while IFS= read -r l; do emit "$l"; done
    emit "GO_TEST_EXIT=$TRC"
    [ "$TRC" = "0" ] || RC=1
    require 'EXCLUDED_ALL=3'
    finish R5-filter-semantics.txt || RC=1
    ;;
r7)
    LOG="$REPO/tmp/verify-$ISSUE-r7.log"
    run_go_test ./handlers 'TestIAMOrphanPurgeNoRelation_2470|TestIAMOrphanPurgeFallbackRealtime_2470' "$LOG"; TRC=$?
    grep -E 'PURGED=|NOT_ORPHAN=' "$LOG" | sed 's/^[[:space:]]*[^:]*: //' | while IFS= read -r l; do emit "$l"; done
    NOT_ORPHAN_HITS="$(grep -c 'not an orphan' "$LOG" || true)"
    emit "NOT_ORPHAN_ERRORS=${NOT_ORPHAN_HITS:-0}"
    emit "GO_TEST_EXIT=$TRC"
    [ "$TRC" = "0" ] || RC=1
    [ "${NOT_ORPHAN_HITS:-1}" = "0" ] || RC=1
    require 'PURGED=1'
    require 'NOT_ORPHAN=0'
    finish R7-purge.txt || RC=1
    ;;
r6)
    BUILD_LOG="$REPO/tmp/verify-$ISSUE-r6-build.log"
    ( cd backend && go build ./... ) > "$BUILD_LOG" 2>&1
    BUILD_EXIT=$?
    emit "BUILD_CMD=cd backend && go build ./..."
    emit "BUILD_EXIT=$BUILD_EXIT"
    [ "$BUILD_EXIT" = "0" ] && emit "BUILD_OK" || RC=1

    TEST_LOG="$REPO/tmp/verify-$ISSUE-r6-test.log"
    ( cd backend && go test ./services ./handlers -run '2470|2364' -count=1 -timeout 15m ) > "$TEST_LOG" 2>&1
    TEST_EXIT=$?
    emit "GO_TEST_CMD=go test ./services ./handlers -run '2470|2364' -count=1 -timeout 15m"
    emit "GO_TEST_EXIT=$TEST_EXIT"
    [ "$TEST_EXIT" = "0" ] && emit "GO_TEST_OK" || RC=1

    NPM_LOG="$REPO/tmp/verify-$ISSUE-r6-npm.log"
    if [ -x "$HEAVY_RUN" ]; then
        ( cd frontend-pc && "$HEAVY_RUN" -- npm run build ) > "$NPM_LOG" 2>&1
    else
        ( cd frontend-pc && npm run build ) > "$NPM_LOG" 2>&1
    fi
    NPM_EXIT=$?
    emit "NPM_BUILD_CMD=cd frontend-pc && npm run build"
    emit "NPM_BUILD_EXIT=$NPM_EXIT"
    [ "$NPM_EXIT" = "0" ] && emit "NPM_BUILD_OK" || RC=1
    tail -2 "$NPM_LOG" | sed 's/^/NPM_TAIL: /' | while IFS= read -r l; do emit "$l"; done
    finish R6-build.txt || RC=1
    ;;
r8)
    # R8（Rev.3 基线差异口径）：HEAD 全量失败集必须 ⊆ origin/develop 基线失败集。
    # 基线里 3 个 user_staff_rollback_2309 失败属 #2565 的环境耦合缺陷（与本单无关，已登记）。
    echo "== R8 full backend suite vs origin/develop baseline =="
    BASELINE_REF="${BASELINE_REF:-origin/develop}"
    BASELINE_DIR="$REPO/tmp/baseline-2470"
    cleanup_baseline() {
        if git -C "$REPO" worktree list --porcelain | grep -qx "worktree ${BASELINE_DIR}"; then
            git -C "$REPO" worktree remove --force "$BASELINE_DIR" >/dev/null 2>&1 || true
        fi
        rm -rf "$BASELINE_DIR"
    }
    trap cleanup_baseline EXIT
    cleanup_baseline
    git -C "$REPO" worktree add --detach "$BASELINE_DIR" "$BASELINE_REF" >/dev/null 2>&1 || {
        emit "BASELINE_WORKTREE=0"; RC=1; finish R8-gate.txt; exit 1;
    }
    BASELINE_SHA="$(git -C "$REPO" rev-parse "${BASELINE_REF}^{commit}")"

    extract_failures() { # json-log -> sorted pkg::test identities
        grep '^{' "$1" | jq -r 'select(.Action == "fail") | "\(.Package)::\(.Test // "PKG")"' | LC_ALL=C sort -u
    }

    CURRENT_LOG="$REPO/tmp/verify-$ISSUE-r8-current.log"
    BASELINE_LOG="$REPO/tmp/verify-$ISSUE-r8-baseline.log"
    ( cd "$REPO/backend" && heavy go test -json ./... -count=1 -timeout 20m -p 1 ) > "$CURRENT_LOG" 2>&1
    FULL_GATE_EXIT=$?
    ( cd "$BASELINE_DIR/backend" && heavy go test -json ./... -count=1 -timeout 20m -p 1 ) > "$BASELINE_LOG" 2>&1
    BASELINE_GATE_EXIT=$?

    CURRENT_FAILURES="$(mktemp)"
    BASELINE_FAILURES="$(mktemp)"
    NEW_FAILURES="$(mktemp)"
    extract_failures "$CURRENT_LOG" > "$CURRENT_FAILURES"
    extract_failures "$BASELINE_LOG" > "$BASELINE_FAILURES"
    LC_ALL=C comm -13 "$BASELINE_FAILURES" "$CURRENT_FAILURES" > "$NEW_FAILURES"

    CURRENT_COUNT="$(wc -l < "$CURRENT_FAILURES" | tr -d ' ')"
    BASELINE_COUNT="$(wc -l < "$BASELINE_FAILURES" | tr -d ' ')"
    NEW_COUNT="$(wc -l < "$NEW_FAILURES" | tr -d ' ')"
    PANIC_COUNT="$(cat "$CURRENT_LOG" "$BASELINE_LOG" | grep -c '^panic:\| panic: ' || true)"
    [ -n "$PANIC_COUNT" ] || PANIC_COUNT=0

    emit "BASELINE_REF=${BASELINE_REF}"
    emit "BASELINE_SHA=${BASELINE_SHA}"
    emit "BASELINE_GATE_EXIT=${BASELINE_GATE_EXIT}"
    emit "FULL_GATE_CMD=cd backend && go test -json ./... -count=1 -timeout 20m -p 1"
    emit "FULL_GATE_EXIT=${FULL_GATE_EXIT}"
    emit "FULL_GATE_CURRENT_FAILURES=${CURRENT_COUNT}"
    emit "FULL_GATE_BASELINE_FAILURES=${BASELINE_COUNT}"
    emit "FULL_GATE_NEW_FAILURES=${NEW_COUNT}"
    emit "BASELINE_FAIL_COUNT=${BASELINE_COUNT}"
    emit "PANIC_COUNT=${PANIC_COUNT}"
    sed 's/^/BASELINE_FAIL: /' "$BASELINE_FAILURES" | while IFS= read -r l; do emit "$l"; done
    if [ -s "$NEW_FAILURES" ]; then
        sed 's/^/NEW_FAIL: /' "$NEW_FAILURES" | while IFS= read -r l; do emit "$l"; done
    fi
    rm -f "$CURRENT_FAILURES" "$BASELINE_FAILURES" "$NEW_FAILURES"

    [ "$NEW_COUNT" = "0" ] || RC=1
    [ "$PANIC_COUNT" = "0" ] || RC=1
    require "FULL_GATE_NEW_FAILURES=0"
    require "PANIC_COUNT=0"
    require "BASELINE_FAIL_COUNT="
    finish R8-gate.txt || RC=1
    ;;
r9)
    # R9：接口/镜像语义文档（api.md 端点小节 + iam-notes 上游孤儿端点契约）。
    #
    # Rev.4 起 R9 从「token 存在性」升级为「契约值可判定」：
    #   DOC_TOKENS=3        —— §2.6.5 三项语义存在（存在性，保留）
    #   DOC_ERRCODE_40300=1 —— 权限拒绝必须写 403 + 40300，不得写成 40102/40303/40002
    #   DOC_NOTES_ARRAY=1   —— purge 响应 notes 必须声明为字符串数组，不得写成字符串
    # 每条都带负例自检（把正确写法 sed 成错误写法，断言必须转 0），证明断言真的能拦住错值。
    echo "== R9 docs: tuneloop endpoint + upstream orphan contract (contract values) =="
    API_DOC="docs/spec/api/api.md"
    IAM_NOTES="docs/topics/iam/iam-notes.md"
    DOC_TOKENS=0
    for tok in 'users/orphans' 'fallback_unsupported' 'scanned_pending'; do
        hit=0
        grep -Fq -- "$tok" "$API_DOC" && hit=1
        grep -Fq -- "$tok" "$IAM_NOTES" && hit=1
        emit "DOC_TOKEN[$tok]=${hit}"
        DOC_TOKENS=$((DOC_TOKENS + hit))
    done
    API_SECTION=0
    grep -Fq 'GET /api/admin/iam-orphans' "$API_DOC" && API_SECTION=1
    NOTES_SECTION=0
    grep -Fq 'IAM 孤儿查询端点' "$IAM_NOTES" && NOTES_SECTION=1
    emit "DOC_API_SECTION=${API_SECTION}"
    emit "DOC_NOTES_SECTION=${NOTES_SECTION}"
    emit "DOC_TOKENS=${DOC_TOKENS}"

    # ---- §2.6.5 段落切片（从 2.6.5 标题到下一个 ### 标题）----
    SECTION_FILE="$(mktemp)"
    awk '/^#### 2\.6\.5 /{f=1} f && /^### 2\.7 /{f=0} f' "$API_DOC" > "$SECTION_FILE"

    # 契约值断言 1：权限拒绝行必须含 40300，且同一行不得出现 40102。
    # 负例：doc_errcode_40300<file> 对「写成 40102」的变体必须返回 0。
    doc_errcode_40300() { # file
        local deny_line
        deny_line="$(grep -F '非系统管理员' "$1" | head -1)"
        [ -n "$deny_line" ] || { echo 0; return; }
        case "$deny_line" in
            *40102*) echo 0 ; return ;;
            *40300*) echo 1 ; return ;;
            *)       echo 0 ; return ;;
        esac
    }
    DOC_ERRCODE_40300="$(doc_errcode_40300 "$SECTION_FILE")"

    # 契约值断言 2：purge 响应 notes 必须是数组形态 `"notes": [`，不得是 `"notes": "`。
    doc_notes_array() { # file
        local array_hit string_hit
        array_hit=0; string_hit=0
        grep -Fq '"notes": [' "$1" && array_hit=1
        grep -Fq '"notes": "' "$1" && string_hit=1
        if [ "$array_hit" = "1" ] && [ "$string_hit" = "0" ]; then echo 1; else echo 0; fi
    }
    DOC_NOTES_ARRAY="$(doc_notes_array "$SECTION_FILE")"

    # ---- 负例自检（红绿实证）：故意写错 → 断言必须为 0 ----
    NEG_FILE="$(mktemp)"
    sed 's/`40300`：非系统管理员/`40102`：非系统管理员/' "$SECTION_FILE" > "$NEG_FILE"
    NEG_ERRCODE_DETECTED=0
    [ "$(doc_errcode_40300 "$NEG_FILE")" = "0" ] && NEG_ERRCODE_DETECTED=1
    sed 's/"notes": \[/"notes": "/' "$SECTION_FILE" > "$NEG_FILE"
    NEG_NOTES_DETECTED=0
    [ "$(doc_notes_array "$NEG_FILE")" = "0" ] && NEG_NOTES_DETECTED=1
    rm -f "$NEG_FILE" "$SECTION_FILE"

    emit "DOC_ERRCODE_40300=${DOC_ERRCODE_40300}"
    emit "DOC_NOTES_ARRAY=${DOC_NOTES_ARRAY}"
    emit "NEGATIVE_ERRCODE_DETECTED=${NEG_ERRCODE_DETECTED}"
    emit "NEGATIVE_NOTES_DETECTED=${NEG_NOTES_DETECTED}"

    [ "$API_SECTION" = "1" ] || RC=1
    [ "$NOTES_SECTION" = "1" ] || RC=1
    [ "$DOC_TOKENS" = "3" ] || RC=1
    [ "$DOC_ERRCODE_40300" = "1" ] || RC=1
    [ "$DOC_NOTES_ARRAY" = "1" ] || RC=1
    [ "$NEG_ERRCODE_DETECTED" = "1" ] || RC=1
    [ "$NEG_NOTES_DETECTED" = "1" ] || RC=1
    require "DOC_TOKENS=3"
    require "DOC_ERRCODE_40300=1"
    require "DOC_NOTES_ARRAY=1"
    finish R9-docs.txt || RC=1
    ;;
*)
    usage
    ;;
esac

exit "$RC"
