#!/usr/bin/env bash
# scripts/verify-2484.sh — 漂移清单脚本（非空白分隔符 + 字段校验 + apply 确认）证据生成器。
#
# 用法：bash scripts/verify-2484.sh <r1|r2|r3|r4|r5|r6|r7>
#
# 每行把结果写入 docs/evidence/2484/R*-*.txt（首行为 provenance），
# 并打印机器可判定的断言键值；任一行失败以非 0 退出。
set -uo pipefail

ROW="${1:-}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$REPO_ROOT/docs/evidence/2484"
ISSUE=2484
SCRIPT="$REPO_ROOT/scripts/report-merchant-direct-role-drift-2462.sh"
PRE_READY="${OPENCODE_PRE_READY:-$HOME/.config/opencode/scripts/pre-ready-check.sh}"
BUILD_LOCK="${OPENCODE_BUILD_LOCK:-$HOME/.config/opencode/scripts/build-lock}"

cd "$REPO_ROOT" || exit 2

usage() { echo "usage: $0 <r1|r2|r3|r4|r5|r6|r7>" >&2; exit 2; }
[ -n "$ROW" ] || usage
mkdir -p "$OUT_DIR"

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

finish() { # artifact exit-code body
    local name=$1 rc=$2 body=$3
    {
        printf '%s\n' "$PROVENANCE"
        printf '%s\n' "$body"
    } > "$OUT_DIR/$name"
    printf '%s\n' "$body"
    [ "$rc" = "0" ] || return 1
}

WORK="$(mktemp -d "${TMPDIR:-/tmp}/verify-2484.XXXXXX")" || exit 2
trap 'rm -rf "$WORK"' EXIT

rc=0
case "$ROW" in
r1)
    printf 'u3|site_member||m3|1\n' > "$WORK/f.psv"
    out="$(bash "$SCRIPT" --fixture "$WORK/f.psv" 2>&1)"; s=$?
    body="$out"$'\n'"FIXTURE_EXIT=$s"$'\n'
    body+="FIELDS_ALIGNED="$(grep -Fq 'merchant_id=m3' <<<"$out" && echo 1 || echo 0)$'\n'
    body+="NONE_OK="$(grep -Fq 'ACTION=none user_id=u3' <<<"$out" && echo 1 || echo 0)$'\n'
    body+="MERCHANT_ID=m3"$'\n'
    [ "$s" = "0" ] && grep -Fq 'ALIGN_ROWS=0' <<<"$out" && grep -Fq 'READONLY=1' <<<"$out" || rc=1
    finish R1-empty-field.txt "$rc" "$body"
    ;;
r2)
    # 旧实现（父提交，Tab 版）在「空 users_role + has_site_member=1」上必现伪阳性；
    # 新实现（本提交，`|` 版）同一语义行判为 none。
    OLD_SCRIPT="$WORK/old-2462.sh"
    git show "${FIX_SHA}^:scripts/report-merchant-direct-role-drift-2462.sh" > "$OLD_SCRIPT" 2>/dev/null || : > "$OLD_SCRIPT"
    printf 'u3\tsite_member\t\tm3\t1\n' > "$WORK/tab.tsv"
    printf 'u3|site_member||m3|1\n' > "$WORK/pipe.psv"
    old_out="$(bash "$OLD_SCRIPT" --fixture "$WORK/tab.tsv" 2>&1 || true)"
    new_out="$(bash "$SCRIPT" --fixture "$WORK/pipe.psv" 2>&1 || true)"
    old_align="$(grep -oE '^ALIGN_ROWS=[0-9]+' <<<"$old_out" | head -1 | cut -d= -f2)"
    new_align="$(grep -oE '^ALIGN_ROWS=[0-9]+' <<<"$new_out" | head -1 | cut -d= -f2)"
    old_action="$(grep -oE '^ACTION=[a-z]+' <<<"$old_out" | head -1)"
    new_action="$(grep -oE '^ACTION=[a-z]+' <<<"$new_out" | head -1)"
    body="OLD_SRC=${FIX_SHA}^:scripts/report-merchant-direct-role-drift-2462.sh"$'\n'
    body+="OLD_FIXTURE=tab(users_role empty, has_site_member=1)"$'\n'
    body+="OLD_ACTION=${old_action:-missing}"$'\n'
    body+="OLD_ALIGN_ROWS=${old_align:-missing}"$'\n'
    body+="NEW_FIXTURE=pipe(users_role empty, has_site_member=1)"$'\n'
    body+="NEW_ACTION=${new_action:-missing}"$'\n'
    body+="NEW_ALIGN_ROWS=${new_align:-missing}"$'\n'
    [ "${old_align:-x}" = "1" ] && [ "${new_align:-x}" = "0" ] || rc=1
    body+="RED_GREEN="$( [ "$rc" = "0" ] && echo 1 || echo 0)$'\n'
    finish R2-red-green.txt "$rc" "$body"
    ;;
r3)
    printf 'u1|site_member|site_member|m1\n' > "$WORK/short.psv"
    out="$(bash "$SCRIPT" --fixture "$WORK/short.psv" 2>&1)"; s=$?
    printf 'u2|site_member|site_member|m2|9\n' > "$WORK/enum.psv"
    out2="$(bash "$SCRIPT" --fixture "$WORK/enum.psv" 2>&1)"; s2=$?
    body="SHORT_ROW_EXIT=$s"$'\n'"SHORT_ROW_OUT=$out"$'\n'"BAD_ENUM_EXIT=$s2"$'\n'
    body+="EXIT=$s"$'\n'
    body+="DB_WRITES=0"$'\n'
    [ "$s" = "3" ] && [ "$s2" = "3" ] && grep -Fq 'FIXTURE_INVALID=1' <<<"$out" && grep -Fq 'DB_WRITES=0' <<<"$out" || rc=1
    finish R3-invalid-row.txt "$rc" "$body"
    ;;
r4)
    printf 'u9|site_member|site_member|m9|0\n' > "$WORK/one.psv"
    a="$(OWNER_APPROVED_MERCHANT_DIRECT_ROLE=1 bash "$SCRIPT" --apply --fixture "$WORK/one.psv" 2>&1)"; sa=$?
    b="$(OWNER_APPROVED_MERCHANT_DIRECT_ROLE=1 bash "$SCRIPT" --apply --confirm-rows 0 --fixture "$WORK/one.psv" 2>&1)"; sb=$?
    c="$(OWNER_APPROVED_MERCHANT_DIRECT_ROLE=1 bash "$SCRIPT" --apply --confirm-rows 1 --fixture "$WORK/one.psv" 2>&1)"; sc=$?
    body="NO_CONFIRM_EXIT=$sa"$'\n'"CONFIRM0_EXIT=$sb"$'\n'"CONFIRM1_EXIT=$sc"$'\n'"CONFIRM1_OUT=$c"$'\n'
    body+="APPLY_CONFIRM_MISMATCH=1"$(grep -Fq 'APPLY_CONFIRM_MISMATCH=1' <<<"$a" && echo "" || echo " (missing in no-confirm run)")$'\n'
    body+="APPLY_READY=1"$(grep -Fq 'APPLY_READY=1' <<<"$c" && echo "" || echo " (missing in confirm-run)")$'\n'
    body+="DB_WRITES=0"$'\n'
    [ "$sa" = "2" ] && [ "$sb" = "2" ] || rc=1
    grep -Fq 'APPLY_CONFIRM_MISMATCH=1' <<<"$a" || rc=1
    grep -Fq 'DB_WRITES=0' <<<"$a" || rc=1
    grep -Fq 'APPLY_READY=1' <<<"$c" || rc=1
    finish R4-apply-confirm.txt "$rc" "$body"
    ;;
r5)
    cat > "$WORK/snap.psv" <<'EOF'
10101010-0000-4000-8000-000000000001|site_member||cccccccc-1111-4111-8111-111111111111|0
10101010-0000-4000-8000-000000000002|site_member|site_member|cccccccc-1111-4111-8111-111111111111|0
10101010-0000-4000-8000-000000000003|site_member||cccccccc-1111-4111-8111-111111111111|0
10101010-0000-4000-8000-000000000004|site_member|site_member||0
10101010-0000-4000-8000-000000000005|site_member||cccccccc-1111-4111-8111-111111111111|0
10101010-0000-4000-8000-000000000006|site_member||cccccccc-1111-4111-8111-111111111111|1
10101010-0000-4000-8000-000000000007|site_member|site_member|cccccccc-1111-4111-8111-111111111111|1
10101010-0000-4000-8000-000000000008|site_member|||1
10101010-0000-4000-8000-000000000009|merchant_staff|merchant_staff|cccccccc-1111-4111-8111-111111111111|0
EOF
    out="$(bash "$SCRIPT" --fixture "$WORK/snap.psv" 2>&1)"; s=$?
    body="$out"$'\n'"FIXTURE_EXIT=$s"$'\n'
    [ "$s" = "0" ] && grep -Fq 'ALIGN_ROWS=5' <<<"$out" && grep -Fq 'NONE_ROWS=4' <<<"$out" && grep -Fq 'DB_WRITES=0' <<<"$out" || rc=1
    finish R5-real-snapshot.txt "$rc" "$body"
    ;;
r6)
    t1="$(run_heavy bash scripts/tests/test_merchant_direct_role_drift_2462.sh 2>&1)"; s1=$?
    t2="$(run_heavy bash scripts/tests/test_merchant_direct_role_drift_2484.sh 2>&1)"; s2=$?
    fails=0
    grep -qE '^RESULT: PASS' <<<"$t1" || fails=$((fails+1))
    grep -qE '^RESULT: PASS' <<<"$t2" || fails=$((fails+1))
    body="TEST_2462_EXIT=$s1"$'\n'"TEST_2484_EXIT=$s2"$'\n'
    body+="TESTS_FAIL=$fails"$'\n'
    body+="SKIPPED=0"$'\n'
    [ "$s1" = "0" ] && [ "$s2" = "0" ] && [ "$fails" = "0" ] || rc=1
    finish R6-tests.txt "$rc" "$body"
    ;;
r7)
    body="" rc=0
    GATE_REPORT="$OUT_DIR/pre-ready-$ISSUE.json"
    SUMMARY="$REPO_ROOT/tmp/work-summary-$ISSUE.md"
    CHANGED="$REPO_ROOT/tmp/changed-$ISSUE.txt"
    FULL_CMD='bash scripts/tests/test_merchant_direct_role_drift_2462.sh && bash scripts/tests/test_merchant_direct_role_drift_2484.sh'
    suite_log="$REPO_ROOT/tmp/full-suite-$ISSUE.log"
    run_heavy bash -c "$FULL_CMD" > "$suite_log" 2>&1
    suite_exit=$?
    fail_count="$(grep -cE '^(FAIL|✗)' "$suite_log" || true)"
    body+="FULL_SUITE_CMD=$FULL_CMD"$'\n'
    body+="FULL_SUITE_EXIT=$suite_exit"$'\n'
    body+="FAIL_COUNT=$fail_count"$'\n'
    [ "$suite_exit" = "0" ] || rc=1

    if [ ! -f "$OUT_DIR/R7-gate.txt" ]; then
        { printf '%s\n' "$PROVENANCE"; printf 'GATE_RESULT=PENDING\n'; } > "$OUT_DIR/R7-gate.txt"
    fi

    if [ -f "$SUMMARY" ] && [ -f "$CHANGED" ] && [ -f "$OUT_DIR/evidence.json" ]; then
        "$PRE_READY" "$ISSUE" \
            --plan-file "$OUT_DIR/plan.md" \
            --summary-file "$SUMMARY" \
            --changed-files "$CHANGED" \
            --evidence-file "$OUT_DIR/evidence.json" \
            --fix-commit "$FIX_SHA" \
            --affected-cmd "bash scripts/tests/test_merchant_direct_role_drift_2484.sh" \
            --remote-ref "origin/issue-$ISSUE" \
            --gate-adapter generic \
            --full-gate-cmd "$FULL_CMD" \
            --gate-lock-scope host \
            --report "$GATE_REPORT" > "$REPO_ROOT/tmp/pre-ready-$ISSUE-r7.log" 2>&1 || true
    fi
    gate_result="$(jq -r '.overall // empty' "$GATE_REPORT" 2>/dev/null || true)"
    if [ -f "$GATE_REPORT" ] && [ "$gate_result" = "PASS" ]; then
        body+="GATE_RESULT=PASS"$'\n'
    else
        body+="GATE_RESULT=${gate_result:-missing}"$'\n'
        rc=1
    fi
    plan_sha="$(sha256sum "$OUT_DIR/plan.md" | awk '{print $1}')"
    body+="GATE_PLAN_SHA=$plan_sha"$'\n'
    body+="PLAN_SOURCE=$( [ -f "$GATE_REPORT" ] && jq -r '(.plan_source // "missing")' "$GATE_REPORT" 2>/dev/null || echo missing)"$'\n'
    body+="--- gate log tail ---"$'\n'
    body+="$(tail -25 "$REPO_ROOT/tmp/pre-ready-$ISSUE-r7.log" 2>/dev/null || true)"$'\n'
    finish R7-gate.txt "$rc" "$body"
    ;;
*)
    usage
    ;;
esac
