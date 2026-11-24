#!/usr/bin/env bash
# scripts/verify-2475.sh — site_members.roles jsonb 写入修复（#2475）证据生成器。
#
# 用法：bash scripts/verify-2475.sh <r1|r2|r3|r4|r5|r6>
#
# 每行把结果写入 docs/evidence/2475/R*-*.txt（首行为 provenance），
# 并打印机器可判定的断言键值；任一行失败以非 0 退出。
set -uo pipefail

ROW="${1:-}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$REPO_ROOT/docs/evidence/2475"
ISSUE=2475
BUILD_LOCK="${OPENCODE_BUILD_LOCK:-$HOME/.config/opencode/scripts/build-lock}"
PRE_READY="${OPENCODE_PRE_READY:-$HOME/.config/opencode/scripts/pre-ready-check.sh}"
PLAN_SOURCE="${OPENCODE_PLAN_SOURCE:-$HOME/.config/opencode/scripts/plan-source.sh}"

cd "$REPO_ROOT" || exit 2

usage() {
    echo "usage: $0 <r1|r2|r3|r4|r5|r6>" >&2
    exit 2
}
[ -n "$ROW" ] || usage

mkdir -p "$OUT_DIR"

# 测试库凭据：worktree 本地 backend/.env（gitignored）优先，其次环境变量。
if [ -f "$REPO_ROOT/backend/.env" ]; then
    set -a
    # shellcheck disable=SC1091
    . "$REPO_ROOT/backend/.env"
    set +a
fi
export PATH="/usr/local/go/bin:${PATH}"

# provenance 锚定到本单最新的 fix commit：证据提交后再次生成仍保持一致。
FIX_SHA="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
[ -n "$FIX_SHA" ] || FIX_SHA="$(git rev-parse HEAD)"
SHORT="$(git rev-parse --short=8 "$FIX_SHA")"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)docs/evidence/$ISSUE" ':(exclude)tmp')" ]; then DIRTY=true; fi
PROVENANCE="# build=1.0.$SHORT # commit=$FIX_SHA # dirty=$DIRTY"

run_heavy() {
    if [ -x "$BUILD_LOCK" ]; then
        BUILD_LOCK_TIMEOUT="${BUILD_LOCK_TIMEOUT:-1800}" "$BUILD_LOCK" "$@"
    else
        "$@"
    fi
}

finish() { # artifact-name exit-code body
    local name=$1 rc=$2 body=$3
    {
        printf '%s\n' "$PROVENANCE"
        printf '%s\n' "$body"
    } > "$OUT_DIR/$name"
    printf '%s\n' "$body"
    if [ "$rc" = "0" ]; then
        echo "ROW=$ROW PASS artifact=docs/evidence/$ISSUE/$name"
        exit 0
    fi
    echo "ROW=$ROW FAIL artifact=docs/evidence/$ISSUE/$name"
    exit 1
}

grep_key() {
    printf '%s\n' "$1" \
        | grep -oE "(^|[^A-Za-z0-9_])$2=[^[:space:]]+" \
        | sed -E "s/.*$2=//" \
        | tail -1
}

row_handlers() {
    local name=$1 regex=$2
    shift 2
    local log body="" rc=0 pair key want got
    log="$(cd "$REPO_ROOT/backend" && run_heavy go test ./handlers -run "$regex" -v -count=1 2>&1)"
    printf '%s' "$log" | grep -qE '^ok[[:space:]]' || rc=1
    for pair in "$@"; do
        key="${pair%%=*}"
        want="${pair#*=}"
        got="$(grep_key "$log" "$key")"
        body+="$key=${got:-missing}"$'\n'
        [ "${got:-}" = "$want" ] || rc=1
    done
    if [ "$rc" != "0" ]; then
        body+="--- go test output (tail) ---"$'\n'
        body+="$(printf '%s\n' "$log" | tail -30)"$'\n'
    fi
    finish "$name" "$rc" "$body"
}

case "$ROW" in
    r1)
        row_handlers "R1-staff-roles.txt" '^TestUpdateUser_RoleSyncWritesJSONB_2475$' \
            "NO_22P02=1" "ROLE=site_admin" "ROLE_ERRORS=0"
        ;;
    r2)
        row_handlers "R2-proxy-roles.txt" '^TestEnsureSiteMember_RoleCorrectionWritesJSONB_2475$' \
            "NO_22P02=1" 'ROLES_JSON=["site_admin"]'
        ;;
    r3)
        row_handlers "R3-helper.txt" '^TestSiteMemberRolesExpr_Normalization_2475$' \
            'DEDUP=["site_member","repair_technician"]' "EMPTY=[]" "NIL=[]"
        ;;
    r4)
        row_handlers "R4-multi-role.txt" '^TestUpdateMemberRole_MultiRoleKept_2475$' \
            "STATUS=200" "ROLES_KEPT=1"
        ;;
    r5)
        # R5 必须是可执行的全量 gate 证据（P5 不得为 N/A）。
        body="" rc=0
        GATE_REPORT="$OUT_DIR/pre-ready-2475.json"
        SUMMARY="$REPO_ROOT/tmp/work-summary-2475.md"
        CHANGED="$REPO_ROOT/tmp/changed-2475.txt"
        SUITE_LOG="$REPO_ROOT/tmp/full-suite-2475.log"
        FULL_CMD='cd backend && go build ./... && go test ./handlers -count=1 -timeout 15m && go test ./services -count=1 -timeout 15m'

        : > "$SUITE_LOG"
        run_heavy bash -c "$FULL_CMD" > "$SUITE_LOG" 2>&1
        suite_exit=$?
        fail_count="$(grep -cE '^--- FAIL' "$SUITE_LOG" || true)"
        panic_count="$(grep -cE '^panic:' "$SUITE_LOG" || true)"
        body+="FULL_SUITE_CMD=$FULL_CMD"$'\n'
        body+="FULL_SUITE_EXIT=$suite_exit"$'\n'
        body+="FAIL_COUNT=$fail_count"$'\n'
        body+="PANIC_COUNT=$panic_count"$'\n'
        if [ "$suite_exit" != "0" ] || [ "$fail_count" != "0" ] || [ "$panic_count" != "0" ]; then
            rc=1
            body+="--- full suite output (tail) ---"$'\n'
            body+="$(tail -40 "$SUITE_LOG")"$'\n'
        fi

        # 首次运行（产物尚未提交）时先落一版 interim（P12 只校验存在性）；
        # 已存在时不在 gate 期间覆写 —— 否则工作树变脏导致 P5 FAIL。
        if [ ! -f "$OUT_DIR/R5-gate.txt" ]; then
            {
                printf '%s\n' "$PROVENANCE"
                printf '%s\n' "$body"
                printf 'GATE_RESULT=PENDING\n'
            } > "$OUT_DIR/R5-gate.txt"
        fi

        if [ -f "$SUMMARY" ] && [ -f "$CHANGED" ]; then
            "$PRE_READY" "$ISSUE" \
                --plan-file "$OUT_DIR/plan.md" \
                --summary-file "$SUMMARY" \
                --changed-files "$CHANGED" \
                --evidence-file "$OUT_DIR/evidence.json" \
                --fix-commit "$FIX_SHA" \
                --affected-cmd "cd backend && go test ./handlers -run '_2475$|^TestUpdateMemberRole_|_2375$' -count=1" \
                --remote-ref origin/issue-2475 \
                --gate-adapter go \
                --full-gate-cmd "$FULL_CMD" \
                --gate-lock-scope host \
                --report "$GATE_REPORT" > "$REPO_ROOT/tmp/pre-ready-2475-r5.log" 2>&1 || true
        fi

        gate_result="$(jq -r '.overall // empty' "$GATE_REPORT" 2>/dev/null || true)"
        if [ -f "$GATE_REPORT" ] && [ "$gate_result" = "PASS" ]; then
            body+="GATE_RESULT=PASS"$'\n'
        else
            body+="GATE_RESULT=${gate_result:-missing}"$'\n'
            rc=1
            body+="$(tail -20 "$REPO_ROOT/tmp/pre-ready-2475-r5.log" 2>/dev/null)"$'\n'
        fi
        plan_sha="$(sha256sum "$OUT_DIR/plan.md" | awk '{print $1}')"
        gate_sha="$(jq -r '.plan_sha256 // empty' "$GATE_REPORT" 2>/dev/null || true)"
        if [ -n "$gate_sha" ] && [ "$gate_sha" = "$plan_sha" ]; then
            body+="GATE_PLAN_SHA_MATCH=1"$'\n'
        else
            body+="GATE_PLAN_SHA_MATCH=0"$'\n'
            rc=1
        fi
        p5_status="$(jq -r '[.checks[]? | select(.id == "P5")][0].status // "missing"' "$GATE_REPORT" 2>/dev/null || echo missing)"
        body+="P5=${p5_status:-missing}"$'\n'
        [ "$p5_status" = "PASS" ] || rc=1
        if "$PLAN_SOURCE" --issue "$ISSUE" >"$REPO_ROOT/tmp/plan-source-2475.log" 2>&1; then
            body+="PLAN_SOURCE=PASS"$'\n'
        else
            body+="PLAN_SOURCE=FAIL"$'\n'
            body+="$(tail -5 "$REPO_ROOT/tmp/plan-source-2475.log")"$'\n'
            rc=1
        fi
        finish "R5-gate.txt" "$rc" "$body"
        ;;
    r6)
        body="" rc=0
        build_log="$(cd "$REPO_ROOT/backend" && run_heavy go build ./... 2>&1)"
        build_exit=$?
        test_log="$(cd "$REPO_ROOT/backend" && run_heavy go test ./handlers -run '_2475$|^TestUpdateMemberRole_|_2375$' -count=1 2>&1)"
        test_exit=$?
        tests_fail="$(printf '%s\n' "$test_log" | grep -cE '^--- FAIL' || true)"
        body+="BUILD_EXIT=$build_exit"$'\n'
        body+="TESTS_EXIT=$test_exit"$'\n'
        body+="TESTS_FAIL=$tests_fail"$'\n'
        [ "$build_exit" = "0" ] || rc=1
        [ "$test_exit" = "0" ] || rc=1
        [ "$tests_fail" = "0" ] || rc=1
        if [ "$rc" != "0" ]; then
            body+="--- build output (tail) ---"$'\n'"$(printf '%s\n' "$build_log" | tail -20)"$'\n'
            body+="--- test output (tail) ---"$'\n'"$(printf '%s\n' "$test_log" | tail -30)"$'\n'
        fi
        finish "R6-build.txt" "$rc" "$body"
        ;;
    *)
        usage
        ;;
esac
