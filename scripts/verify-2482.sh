#!/usr/bin/env bash
# scripts/verify-2482.sh — 改主角色时保留其它身份（roles 合并）证据生成器。
#
# 用法：bash scripts/verify-2482.sh <r1|r2|r3|r4|r5|r6|r7|r8>
set -uo pipefail

ROW="${1:-}"
ISSUE=2482
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$REPO_ROOT/docs/evidence/$ISSUE"
BUILD_LOCK="${OPENCODE_BUILD_LOCK:-$HOME/.config/opencode/scripts/build-lock}"
PRE_READY="${OPENCODE_PRE_READY:-$HOME/.config/opencode/scripts/pre-ready-check.sh}"

cd "$REPO_ROOT" || exit 2
case "$ROW" in r1|r2|r3|r4|r5|r6|r7|r8) ;; *) echo "usage: $0 <r1..r8>" >&2; exit 2 ;; esac
mkdir -p "$OUT_DIR"

if [ -f "$REPO_ROOT/backend/.env" ]; then set -a; . "$REPO_ROOT/backend/.env"; set +a; fi
if [ -z "${TUNELOOP_DB:-}" ] && [ -f /home/coder/tuneloop/backend/.env ]; then
    set -a; . /home/coder/tuneloop/backend/.env; set +a
fi
export PATH="/usr/local/go/bin:${PATH}"

FIX_SHA="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
[ -n "$FIX_SHA" ] || FIX_SHA="$(git rev-parse HEAD)"
SHORT="$(git rev-parse --short=8 "$FIX_SHA")"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)docs/evidence/$ISSUE" ':(exclude)tmp')" ]; then DIRTY=true; fi
PROVENANCE="# build=1.0.$SHORT # commit=$FIX_SHA # dirty=$DIRTY"

run_heavy() {
    if [ -x "$BUILD_LOCK" ]; then BUILD_LOCK_TIMEOUT="${BUILD_LOCK_TIMEOUT:-1800}" "$BUILD_LOCK" "$@"; else "$@"; fi
}

finish() { # name rc body
    { printf '%s\n' "$PROVENANCE"; printf '%s\n' "$3"; } > "$OUT_DIR/$1"
    printf '%s\n' "$3"
    [ "$2" = "0" ] || return 1
}

run_test() { # regex — 归一化 go test -v 的 file.go:LINE: 前缀，便于 KEY= 断言
    ( cd "$REPO_ROOT/backend" && run_heavy go test ./handlers -run "$1" -count=1 -v 2>&1 ) \
        | sed -E 's/^[[:space:]]*[^[:space:]]+_test\.go:[0-9]+: //'
}

keys() { # regex
    grep -E "$1"
}

rc=0
case "$ROW" in
r1)
    out="$(run_test 'TestMergeSiteMemberRoles_2482')"; s=$?
    body="$(keys '^(CASE[0-9]=|ORDER_STABLE=)' <<<"$out")"$'\n'
    body+="EMPTY_NEW=$(grep -E '^CASE5=' <<<"$out" | cut -d= -f2)"$'\n'
    body+="TESTS_EXIT=$s"$'\n'
    [ "$s" = "0" ] && grep -q '^CASE1=site_admin,repair_technician' <<<"$out" && grep -q '^ORDER_STABLE=1' <<<"$out" || rc=1
    finish R1-merge-fn.txt "$rc" "$body"
    ;;
r2)
    out="$(run_test 'TestUpdateUser_RoleChangeKeepsOtherRoles_2482')"; s=$?
    body="$(keys '^(ROLES_JSON=|ROLE=|NO_22P02=|IAM_ASSIGN_CALLS=|IAM_UNASSIGN_CALLS=|DUAL_SOURCE_OK=|RED_GREEN=)' <<<"$out")"$'\n'"TESTS_EXIT=$s"$'\n'
    [ "$s" = "0" ] && grep -q 'ROLES_JSON=\["site_admin","repair_technician"\]' <<<"$out" && grep -q '^RED_GREEN=1' <<<"$out" || rc=1
    finish R2-handler.txt "$rc" "$body"
    ;;
r3)
    out="$(run_test 'TestUpdateUser_RoleChangeMultiSite_2482')"; s=$?
    body="$(keys '^(ROW1_OK=|ROW2_OK=|ROW1_JSON=|ROW2_JSON=)' <<<"$out")"$'\n'"TESTS_EXIT=$s"$'\n'
    [ "$s" = "0" ] && grep -q '^ROW1_OK=1' <<<"$out" && grep -q '^ROW2_OK=1' <<<"$out" || rc=1
    finish R3-multi-site.txt "$rc" "$body"
    ;;
r4)
    out="$(run_test 'TestUpdateUser_RoleChangeKeepsOtherRoles_2482')"; s=$?
    body="$(keys '^(NO_22P02=|INVARIANT=|ROLES_NOT_NULL=|ROLES_JSON=)' <<<"$out")"$'\n'"TESTS_EXIT=$s"$'\n'
    [ "$s" = "0" ] && grep -q '^NO_22P02=1' <<<"$out" && grep -q '^INVARIANT=1' <<<"$out" || rc=1
    finish R4-invariant.txt "$rc" "$body"
    ;;
r5)
    out1="$(run_test 'TestUpdateUser_RoleUnchangedNoWrite_2482')"; s1=$?
    out2="$(run_test 'TestUpdateUser_SingleRoleNoResidue_2482')"; s2=$?
    body="$(keys '^(IAM_ROLE_CALLS=|ROLES_UNCHANGED=)' <<<"$out1")"$'\n'"$(keys '^(NO_OLD_RESIDUE=|ROLES_JSON=)' <<<"$out2")"$'\n'
    body+="TEST_UNCHANGED_EXIT=$s1"$'\n'"TEST_SINGLE_EXIT=$s2"$'\n'
    [ "$s1" = "0" ] && [ "$s2" = "0" ] && grep -q '^IAM_ROLE_CALLS=0' <<<"$out1" && grep -q '^NO_OLD_RESIDUE=1' <<<"$out2" || rc=1
    finish R5-no-flip.txt "$rc" "$body"
    ;;
r6)
    out="$(run_test 'TestUpdateUser_RoleChangeKeepsOtherRoles_2482')"; s=$?
    body="$(keys '^(IAM_ASSIGN_CALLS=|IAM_UNASSIGN_CALLS=|DUAL_SOURCE_OK=)' <<<"$out")"$'\n'"TESTS_EXIT=$s"$'\n'
    [ "$s" = "0" ] && grep -q '^IAM_UNASSIGN_CALLS=0' <<<"$out" && grep -q '^DUAL_SOURCE_OK=1' <<<"$out" || rc=1
    finish R6-dual-source.txt "$rc" "$body"
    ;;
r7)
    build_log="$REPO_ROOT/tmp/build-2482.log"
    ( cd "$REPO_ROOT/backend" && run_heavy go build ./... ) > "$build_log" 2>&1
    b=$?
    test_log="$REPO_ROOT/tmp/targeted-2482.log"
    ( cd "$REPO_ROOT/backend" && run_heavy go test ./handlers -run '_2482$|_2475$|_2375$' -count=1 ) > "$test_log" 2>&1
    t=$?
    fails="$(grep -cE '^--- FAIL' "$test_log" || true)"
    body="BUILD_EXIT=$b"$'\n'"TESTS_EXIT=$t"$'\n'"TESTS_FAIL=$fails"$'\n'
    [ "$b" = "0" ] && [ "$t" = "0" ] && [ "$fails" = "0" ] || rc=1
    finish R7-build.txt "$rc" "$body"
    ;;
r8)
    body="" rc=0
    GATE_REPORT="$OUT_DIR/pre-ready-$ISSUE.json"
    SUMMARY="$REPO_ROOT/tmp/work-summary-$ISSUE.md"
    CHANGED="$REPO_ROOT/tmp/changed-$ISSUE.txt"
    SUITE_LOG="$REPO_ROOT/tmp/full-suite-$ISSUE.log"
    FULL_CMD='cd backend && go build ./... && go test ./handlers -count=1 -timeout 15m && go test ./services -count=1 -timeout 15m'
    : > "$SUITE_LOG"
    ( cd "$REPO_ROOT" && run_heavy bash -c "$FULL_CMD" ) > "$SUITE_LOG" 2>&1
    suite_exit=$?
    fail_count="$(grep -cE '^--- FAIL' "$SUITE_LOG" || true)"
    panic_count="$(grep -cE '^panic:' "$SUITE_LOG" || true)"
    body+="FULL_SUITE_CMD=$FULL_CMD"$'\n'"FULL_SUITE_EXIT=$suite_exit"$'\n'
    body+="FAIL_COUNT=$fail_count"$'\n'"PANIC_COUNT=$panic_count"$'\n'
    [ "$suite_exit" = "0" ] && [ "$fail_count" = "0" ] && [ "$panic_count" = "0" ] || rc=1
    if [ ! -f "$OUT_DIR/R8-gate.txt" ]; then
        { printf '%s\n' "$PROVENANCE"; printf 'GATE_RESULT=PENDING\n'; } > "$OUT_DIR/R8-gate.txt"
    fi
    if [ -f "$SUMMARY" ] && [ -f "$CHANGED" ] && [ -f "$OUT_DIR/evidence.json" ]; then
        "$PRE_READY" "$ISSUE" \
            --plan-file "$OUT_DIR/plan.md" \
            --summary-file "$SUMMARY" \
            --changed-files "$CHANGED" \
            --evidence-file "$OUT_DIR/evidence.json" \
            --fix-commit "$FIX_SHA" \
            --affected-cmd "cd backend && go test ./handlers -run '_2482$|_2475\$|_2375\$' -count=1" \
            --remote-ref "origin/issue-$ISSUE" \
            --gate-adapter go \
            --full-gate-cmd "$FULL_CMD" \
            --gate-lock-scope host \
            --report "$GATE_REPORT" > "$REPO_ROOT/tmp/pre-ready-$ISSUE-r8.log" 2>&1 || true
    fi
    gate_result="$(jq -r '.overall // empty' "$GATE_REPORT" 2>/dev/null || true)"
    plan_sha="$(sha256sum "$OUT_DIR/plan.md" | awk '{print $1}')"
    gate_plan_sha="$(jq -r '.plan_sha256 // "missing"' "$GATE_REPORT" 2>/dev/null || echo missing)"
    body+="GATE_RESULT=${gate_result:-missing}"$'\n'
    body+="GATE_PLAN_SHA=$plan_sha"$'\n'
    if [ "$gate_result" = "PASS" ] && [ "$gate_plan_sha" = "$plan_sha" ]; then
        body+="PLAN_SOURCE=PASS"$'\n'
    else
        body+="PLAN_SOURCE=FAIL"$'\n'; rc=1
    fi
    body+="--- gate log tail ---"$'\n'"$(tail -20 "$REPO_ROOT/tmp/pre-ready-$ISSUE-r8.log" 2>/dev/null || true)"$'\n'
    finish R8-gate.txt "$rc" "$body"
    ;;
esac
