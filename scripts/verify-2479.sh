#!/usr/bin/env bash
# scripts/verify-2479.sh — #2479 CreateSite「无负责人（nil UUID）」不得作为 IAM 模板分配目标
# 的证据生成器（R1–R6）。
#
# 用法：bash scripts/verify-2479.sh <r1|r2|r3|r4|r5|r6>
#
# 每行把结果写入 docs/evidence/2479/R*-*.txt（首行为 provenance），并打印机器可判定
# 的断言键值；任一行断言不成立以非 0 退出。
#
# 说明：
# - r1–r4/r6 走 `go test ./handlers -run <单测>`（真实 handler + fake IAM + 测试库）；
# - r5 直接跑全量（handlers + services）记录 FULL_SUITE_EXIT/FAIL_COUNT/PANIC_COUNT，
#   并调用 pre-ready-check.sh 生成 gate 报告（P5 用同一 FULL_GATE_CMD），
#   再比对 plan.md 与 gate 的 plan_sha256（四源一致）。
set -uo pipefail

ROW="${1:-}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$REPO_ROOT/docs/evidence/2479"
ISSUE=2479
HEAVY_RUN="${OPENCODE_HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"
PRE_READY="${OPENCODE_PRE_READY:-$HOME/.config/opencode/scripts/pre-ready-check.sh}"
PLAN_SOURCE="${OPENCODE_PLAN_SOURCE:-$HOME/.config/opencode/scripts/plan-source.sh}"
GATE_REPORT="$OUT_DIR/pre-ready-$ISSUE.json"
FULL_CMD='cd backend && go test ./handlers -count=1 -timeout 15m && go test ./services -count=1 -timeout 15m'
AFFECTED_CMD='cd backend && go test ./handlers -run TestCreateSite -count=1 -timeout 15m'

cd "$REPO_ROOT" || exit 2
mkdir -p "$OUT_DIR" "$REPO_ROOT/tmp"

# Go 不一定在非交互 shell 的 PATH 中（本机为 /usr/local/go/bin）。
if ! command -v go >/dev/null 2>&1 && [ -x /usr/local/go/bin/go ]; then
    export PATH="/usr/local/go/bin:$PATH"
fi

usage() { echo "usage: $0 <r1|r2|r3|r4|r5|r6>" >&2; exit 2; }
[ -n "$ROW" ] || usage

FIX_SHA="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
[ -n "$FIX_SHA" ] || FIX_SHA="$(git rev-parse HEAD)"
SHORT="$(git rev-parse --short=8 "$FIX_SHA")"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)docs/evidence/$ISSUE" ':(exclude)tmp')" ]; then
    DIRTY=true
fi
PROVENANCE="# build=1.0.$SHORT # commit=$FIX_SHA # dirty=$DIRTY"

run_heavy() {
    if [ -x "$HEAVY_RUN" ]; then
        "$HEAVY_RUN" -- "$@"
    else
        "$@"
    fi
}

finish() { # artifact-file exit-code body
    local name=$1 rc=$2 body=$3
    {
        printf '%s\n' "$PROVENANCE"
        printf '%s\n' "$body"
    } > "$OUT_DIR/$name"
    printf '%s\n' "$body"
    [ "$rc" = "0" ] || return 1
}

# 从 `go test -v` 日志里取 `KEY=value`（t.Logf 前缀会被 grep -oE 吃掉）。
kv() { # log key
    grep -oE "(^|[[:space:]])$2=[^[:space:]]+" "$1" 2>/dev/null | tail -1 | sed 's/^[[:space:]]*//' | cut -d= -f2-
}

# 单测行：run <artifact> <测试名> <期望键值对...>
run_unit_row() {
    local artifact=$1 pattern=$2; shift 2
    local log="$REPO_ROOT/tmp/verify-${ISSUE}-${ROW}.log"
    local rc=0 body="" key expect actual

    ( cd "$REPO_ROOT/backend" && run_heavy go test ./handlers -run "$pattern" -count=1 -v ) > "$log" 2>&1
    local test_rc=$?
    body+="TEST_PATTERN=$pattern"$'\n'
    body+="TEST_EXIT=$test_rc"$'\n'
    [ "$test_rc" = "0" ] || rc=1
    grep -qE '^(--- )?(PASS|ok[[:space:]])' "$log" || rc=1
    body+="TEST_PASS=$([ "$test_rc" = "0" ] && echo 1 || echo 0)"$'\n'
    # 跨行负例：artifact 里不得出现「AssignRoleTemplate ... nil UUID」
    local nil_ref=0
    grep -qE "AssignRoleTemplate.*00000000-0000-0000-0000-000000000000" "$log" && nil_ref=1
    body+="NIL_UUID_ASSIGN_REF=$nil_ref"$'\n'
    [ "$nil_ref" = "0" ] || rc=1

    while [ $# -gt 0 ]; do
        key=$1 expect=$2; shift 2
        actual="$(kv "$log" "$key")"
        [ -n "$actual" ] || actual="missing"
        body+="$key=$actual"$'\n'
        [ "$actual" = "$expect" ] || rc=1
    done
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish "$artifact" "$rc" "$body"
}

rc=0
case "$ROW" in
r1)
    run_unit_row R1-nil-admin.txt '^TestCreateSite_NilUUIDAdminSkipsTemplate_2479$' \
        STATUS 201 ASSIGN_CALLS 0 ERR_LOG 0 INFO_SKIP 1
    ;;
r2)
    run_unit_row R2-empty-admin.txt '^TestCreateSite_(EmptyAdmin|InvalidAdmin)SkipsTemplate_2479$' \
        STATUS 201 ASSIGN_CALLS 0
    ;;
r3)
    run_unit_row R3-with-admin.txt '^TestCreateSite_WithAdminAssignsTemplate_2479$' \
        ASSIGN_CALLS 1 TEMPLATE_CODE site_admin
    ;;
r4)
    run_unit_row R4-retry.txt '^TestCreateSite_AdminTemplateRetryKept_2479$' \
        ASSIGN_CALLS 3 RETRY_OK 1 ERR_LOG_FAILED_AFTER 0
    ;;
r5)
    body="" rc=0
    suite_log="$REPO_ROOT/tmp/full-suite-$ISSUE.log"
    run_heavy bash -c "$FULL_CMD" > "$suite_log" 2>&1
    suite_exit=$?
    fail_count="$(grep -cE '^(FAIL|--- FAIL)' "$suite_log" 2>/dev/null || true)"
    panic_count="$(grep -cE '(^panic:|panic: )' "$suite_log" 2>/dev/null || true)"
    body+="FULL_SUITE_CMD=$FULL_CMD"$'\n'
    body+="FULL_SUITE_EXIT=$suite_exit"$'\n'
    body+="FAIL_COUNT=${fail_count:-0}"$'\n'
    body+="PANIC_COUNT=${panic_count:-0}"$'\n'
    [ "$suite_exit" = "0" ] && [ "${fail_count:-0}" = "0" ] && [ "${panic_count:-0}" = "0" ] || rc=1

    summary="$REPO_ROOT/tmp/work-summary-$ISSUE.md"
    changed="$REPO_ROOT/tmp/changed-$ISSUE.txt"
    evidence="$OUT_DIR/evidence.json"
    if [ -f "$summary" ] && [ -f "$changed" ] && [ -f "$evidence" ]; then
        "$PRE_READY" "$ISSUE" \
            --plan-file "$OUT_DIR/plan.md" \
            --summary-file "$summary" \
            --changed-files "$changed" \
            --evidence-file "$evidence" \
            --fix-commit "$FIX_SHA" \
            --affected-cmd "$AFFECTED_CMD" \
            --gate-adapter go \
            --full-gate-cmd "$FULL_CMD" \
            --gate-lock-scope host \
            --report "$GATE_REPORT" > "$REPO_ROOT/tmp/pre-ready-$ISSUE-r5.log" 2>&1 || true
    fi

    gate_result="$(jq -r '.overall // .result // empty' "$GATE_REPORT" 2>/dev/null || true)"
    if [ -f "$GATE_REPORT" ] && [ "$gate_result" = "PASS" ]; then
        body+="GATE_RESULT=PASS"$'\n'
    else
        body+="GATE_RESULT=${gate_result:-missing}"$'\n'
        rc=1
    fi
    plan_sha="$(sha256sum "$OUT_DIR/plan.md" 2>/dev/null | awk '{print $1}')"
    gate_plan_sha="$( [ -f "$GATE_REPORT" ] && jq -r '.plan_sha256 // "missing"' "$GATE_REPORT" 2>/dev/null || echo missing)"
    body+="GATE_PLAN_SHA=${gate_plan_sha:-missing}"$'\n'
    PS_OUT="$("$PLAN_SOURCE" --issue "$ISSUE" 2>&1 || true)"
    if [ "$gate_plan_sha" = "$plan_sha" ] && printf '%s' "$PS_OUT" | grep -q 'PLAN_SOURCE: PASS'; then
        body+="PLAN_SOURCE=PASS"$'\n'
    else
        body+="PLAN_SOURCE=FAIL"$'\n'
        rc=1
    fi
    body+="--- gate log tail ---"$'\n'
    body+="$(tail -25 "$REPO_ROOT/tmp/pre-ready-$ISSUE-r5.log" 2>/dev/null || true)"$'\n'
    finish R5-gate.txt "$rc" "$body"
    ;;
r6)
    body="" rc=0
    build_log="$REPO_ROOT/tmp/verify-$ISSUE-$ROW.log"
    ( cd "$REPO_ROOT/backend" && run_heavy go build ./... ) > "$build_log" 2>&1
    build_exit=$?
    body+="BUILD_CMD=cd backend && go build ./..."$'\n'
    body+="BUILD_EXIT=$build_exit"$'\n'
    [ "$build_exit" = "0" ] || rc=1

    tests_log="$REPO_ROOT/tmp/verify-$ISSUE-$ROW-tests.log"
    ( cd "$REPO_ROOT/backend" && run_heavy go test ./handlers -run 'TestCreateSite' -count=1 -timeout 15m -v ) > "$tests_log" 2>&1
    tests_exit=$?
    tests_fail="$(grep -cE '^--- FAIL' "$tests_log" 2>/dev/null || true)"
    tests_pass="$(grep -cE '^--- PASS' "$tests_log" 2>/dev/null || true)"
    body+="TESTS_CMD=cd backend && go test ./handlers -run TestCreateSite -count=1 -timeout 15m"$'\n'
    body+="TESTS_EXIT=$tests_exit"$'\n'
    body+="TESTS_FAIL=${tests_fail:-0}"$'\n'
    body+="TESTS_PASS_COUNT=${tests_pass:-0}"$'\n'
    [ "$tests_exit" = "0" ] && [ "${tests_fail:-0}" = "0" ] && [ "${tests_pass:-0}" -ge 1 ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R6-build.txt "$rc" "$body"
    ;;
*)
    usage
    ;;
esac
