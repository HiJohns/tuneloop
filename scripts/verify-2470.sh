#!/usr/bin/env bash
# scripts/verify-2470.sh — #2470「IAM 孤儿面板看不到无组织关系孤儿」证据生成器（R1–R7）。
#
# 用法：bash scripts/verify-2470.sh <r1|r2|r3|r4|r5|r6|r7>
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

usage() { echo "usage: $0 <r1|r2|r3|r4|r5|r6|r7>" >&2; exit 2; }

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
*)
    usage
    ;;
esac

exit "$RC"
