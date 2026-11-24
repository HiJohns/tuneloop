#!/usr/bin/env bash
# scripts/verify-2478.sh — 直属成员管理「创建员工」一次性意图（#2478）证据生成器。
#
# 用法：bash scripts/verify-2478.sh <r1|r2|r3|r4|r5|r6|r7>
#
# 每行把结果写入 docs/evidence/2478/R*-*.txt（首行为 provenance），
# 并打印机器可判定的断言键值；任一行失败以非 0 退出。
set -uo pipefail

ROW="${1:-}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$REPO_ROOT/docs/evidence/2478"
ISSUE=2478
BUILD_LOCK="${OPENCODE_BUILD_LOCK:-$HOME/.config/opencode/scripts/build-lock}"
PRE_READY="${OPENCODE_PRE_READY:-$HOME/.config/opencode/scripts/pre-ready-check.sh}"
PLAN_SOURCE="${OPENCODE_PLAN_SOURCE:-$HOME/.config/opencode/scripts/plan-source.sh}"
PC_DIR="$REPO_ROOT/frontend-pc"
INTENT_MODULE="$REPO_ROOT/frontend-pc/src/utils/staffCreateIntent.js"
DIRECT_PAGE="$REPO_ROOT/frontend-pc/src/pages/DirectMemberManagement.jsx"
STAFF_PAGE="$REPO_ROOT/frontend-pc/src/pages/StaffManagement.jsx"

cd "$REPO_ROOT" || exit 2
usage() {
    echo "usage: $0 <r1|r2|r3|r4|r5|r6|r7>" >&2
    exit 2
}
[ -n "$ROW" ] || usage
mkdir -p "$OUT_DIR" "$REPO_ROOT/tmp"

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

count_fixed() { grep -Fc -- "$2" "$1" 2>/dev/null || true; }

case "$ROW" in
    r1)
        body="" rc=0
        test_log="$(cd "$PC_DIR" && node --test tests/staffCreateIntent.test.mjs 2>&1)"
        test_exit=$?
        body+="NODE_TEST_EXIT=$test_exit"$'\n'
        [ "$test_exit" = "0" ] || rc=1

        PROBE="$REPO_ROOT/tmp/probe-2478.mjs"
        cat > "$PROBE" <<'EOF'
import {
  CREATE_INTENT_OPEN,
  createIntentState,
  isConsumableIntent,
  isCreateIntent,
} from '../frontend-pc/src/utils/staffCreateIntent.js'

const flag = (v) => (v ? 'true' : 'false')
const intent = createIntentState()

console.log('NULL=' + flag(isCreateIntent(null)))
console.log('EMPTY=' + flag(isCreateIntent({})))
console.log('TRUE=' + flag(isCreateIntent({ [CREATE_INTENT_OPEN]: true })))
console.log('STRING=' + flag(isCreateIntent({ [CREATE_INTENT_OPEN]: 'true' })))
console.log('NUMBER=' + flag(isCreateIntent({ [CREATE_INTENT_OPEN]: 1 })))
console.log('STATE_HAS_OPEN=' + flag(intent[CREATE_INTENT_OPEN] === true))
console.log('STATE_TYPE=' + String(intent.userType))
console.log('CONSUMED=' + flag(isConsumableIntent(null)))
EOF
        probe_out="$(node "$PROBE" 2>&1)"
        probe_exit=$?
        body+="$probe_out"$'\n'
        body+="PROBE_EXIT=$probe_exit"$'\n'
        [ "$probe_exit" = "0" ] || rc=1
        for pair in "NULL=false" "EMPTY=false" "TRUE=true" "STRING=false" \
                    "NUMBER=false" "STATE_HAS_OPEN=true" "STATE_TYPE=merchant_direct" "CONSUMED=false"; do
            grep -Fxq -- "$pair" <<< "$probe_out" || rc=1
        done
        if [ "$rc" != "0" ]; then
            body+="--- node test output (tail) ---"$'\n'
            body+="$(printf '%s\n' "$test_log" | tail -20)"$'\n'
        fi
        finish "R1-pure.txt" "$rc" "$body"
        ;;
    r2)
        body="" rc=0
        intent_nav="$(count_fixed "$DIRECT_PAGE" "navigate('/staff', { state: createIntentState() })")"
        bare_nav="$(count_fixed "$DIRECT_PAGE" "navigate('/staff')")"
        import_ok="$(grep -Fc "from '../utils/staffCreateIntent'" "$DIRECT_PAGE" 2>/dev/null || true)"
        intent_nav=${intent_nav:-0}; bare_nav=${bare_nav:-0}; import_ok=${import_ok:-0}
        [ "$intent_nav" -ge 1 ] || rc=1
        [ "$bare_nav" = "0" ] || rc=1
        [ "$import_ok" -ge 1 ] || rc=1
        body+="INTENT_NAV=$intent_nav"$'\n'
        body+="BARE_NAV=$bare_nav"$'\n'
        body+="INTENT_IMPORT=$import_ok"$'\n'
        body+="--- matched lines ---"$'\n'
        body+="$(grep -n "navigate('/staff'" "$DIRECT_PAGE" || true)"$'\n'
        finish "R2-intent-static.txt" "$rc" "$body"
        ;;
    r3)
        body="" rc=0
        consume="$(count_fixed "$STAFF_PAGE" "isCreateIntent(location.state)")"
        consume=${consume:-0}
        set_create="$(grep -A3 "isCreateIntent(location.state)" "$STAFF_PAGE" | grep -Fc "setViewMode('create')" || true)"
        set_create=${set_create:-0}
        [ "$consume" -ge 1 ] || rc=1
        [ "$set_create" -ge 1 ] || rc=1
        body+="CONSUME_EFFECT=$consume"$'\n'
        body+="SET_CREATE=$set_create"$'\n'
        body+="--- consume block ---"$'\n'
        body+="$(grep -A5 "isCreateIntent(location.state)" "$STAFF_PAGE" || true)"$'\n'
        finish "R3-consume-static.txt" "$rc" "$body"
        ;;
    r4)
        body="" rc=0
        replace_true="$(count_fixed "$STAFF_PAGE" "replace: true")"
        state_null="$(count_fixed "$STAFF_PAGE" "state: null")"
        push_keep="$(count_fixed "$STAFF_PAGE" "state: createIntentState()")"
        replace_true=${replace_true:-0}; state_null=${state_null:-0}; push_keep=${push_keep:-0}
        [ "$replace_true" -ge 1 ] || rc=1
        [ "$state_null" -ge 1 ] || rc=1
        [ "$push_keep" = "0" ] || rc=1
        body+="REPLACE_TRUE=$replace_true"$'\n'
        body+="STATE_NULL=$state_null"$'\n'
        body+="PAGE_INTENT_LEAK=$push_keep"$'\n'
        body+="--- consume navigate ---"$'\n'
        body+="$(grep -n "replace: true" "$STAFF_PAGE" || true)"$'\n'
        finish "R4-replace-static.txt" "$rc" "$body"
        ;;
    r5)
        body="" rc=0
        cancel_to_list="$(grep -A2 '取消' "$STAFF_PAGE" | grep -Fc "setViewMode('list')" || true)"
        default_list="$(count_fixed "$STAFF_PAGE" "useState('list')")"
        cancel_to_list=${cancel_to_list:-0}; default_list=${default_list:-0}
        [ "$cancel_to_list" -ge 1 ] || rc=1
        [ "$default_list" -ge 1 ] || rc=1
        body+="CANCEL_TO_LIST=$cancel_to_list"$'\n'
        body+="DEFAULT_LIST=$default_list"$'\n'
        body+="--- cancel / default ---"$'\n'
        body+="$(grep -n "useState('list')" "$STAFF_PAGE" || true)"$'\n'
        body+="$(grep -n -A2 '取消' "$STAFF_PAGE" | head -8 || true)"$'\n'
        finish "R5-cancel-static.txt" "$rc" "$body"
        ;;
    r6)
        body="" rc=0
        build_log="$(cd "$PC_DIR" && run_heavy npm run build 2>&1)"
        build_exit=$?
        lint_log="$(cd "$PC_DIR" && npx eslint src/utils/staffCreateIntent.js src/pages/DirectMemberManagement.jsx src/pages/StaffManagement.jsx 2>&1)"
        lint_exit=$?
        lint_errors="$(printf '%s\n' "$lint_log" | grep -oE '[0-9]+ error' | awk '{s+=$1} END {print s+0}')"
        lint_errors=${lint_errors:-0}
        body+="PC_BUILD=$build_exit"$'\n'
        body+="LINT_EXIT=$lint_exit"$'\n'
        body+="LINT_ERRORS=$lint_errors"$'\n'
        [ "$build_exit" = "0" ] || rc=1
        [ "$lint_exit" = "0" ] || rc=1
        [ "$lint_errors" = "0" ] || rc=1
        if [ "$rc" != "0" ]; then
            body+="--- build output (tail) ---"$'\n'"$(printf '%s\n' "$build_log" | tail -20)"$'\n'
            body+="--- lint output (tail) ---"$'\n'"$(printf '%s\n' "$lint_log" | tail -20)"$'\n'
        fi
        finish "R6-build.txt" "$rc" "$body"
        ;;
    r7)
        # R7 必须是可执行的全量 gate 证据（P5 不得为 N/A）。
        body="" rc=0
        GATE_REPORT="$OUT_DIR/pre-ready-2478.json"
        SUMMARY="$REPO_ROOT/tmp/work-summary-2478.md"
        CHANGED="$REPO_ROOT/tmp/changed-2478.txt"
        SUITE_LOG="$REPO_ROOT/tmp/full-suite-2478.log"
        FULL_CMD='cd frontend-pc && npm run build'

        : > "$SUITE_LOG"
        run_heavy bash -c "$FULL_CMD" > "$SUITE_LOG" 2>&1
        suite_exit=$?
        body+="FULL_SUITE_CMD=$FULL_CMD"$'\n'
        body+="FULL_SUITE_EXIT=$suite_exit"$'\n'
        [ "$suite_exit" = "0" ] || rc=1

        # 首次运行（产物尚未提交）时先落一版 interim（P12 只校验存在性）；
        # 已存在时不在 gate 期间覆写 —— 否则工作树变脏导致 P5 FAIL。
        if [ ! -f "$OUT_DIR/R7-gate.txt" ]; then
            {
                printf '%s\n' "$PROVENANCE"
                printf '%s\n' "$body"
                printf 'GATE_RESULT=PENDING\n'
            } > "$OUT_DIR/R7-gate.txt"
        fi

        if [ -f "$SUMMARY" ] && [ -f "$CHANGED" ]; then
            "$PRE_READY" "$ISSUE" \
                --plan-file "$OUT_DIR/plan.md" \
                --summary-file "$SUMMARY" \
                --changed-files "$CHANGED" \
                --evidence-file "$OUT_DIR/evidence.json" \
                --fix-commit "$FIX_SHA" \
                --affected-cmd "cd frontend-pc && npx eslint src/utils/staffCreateIntent.js src/pages/DirectMemberManagement.jsx src/pages/StaffManagement.jsx && node --test tests/staffCreateIntent.test.mjs" \
                --remote-ref origin/issue-2478 \
                --gate-adapter npm \
                --full-gate-cmd "$FULL_CMD" \
                --gate-lock-scope host \
                --report "$GATE_REPORT" > "$REPO_ROOT/tmp/pre-ready-2478-r7.log" 2>&1 || true
        fi

        gate_result="$(jq -r '.overall // empty' "$GATE_REPORT" 2>/dev/null || true)"
        if [ -f "$GATE_REPORT" ] && [ "$gate_result" = "PASS" ]; then
            body+="GATE_RESULT=PASS"$'\n'
        else
            body+="GATE_RESULT=${gate_result:-missing}"$'\n'
            rc=1
            body+="$(tail -30 "$REPO_ROOT/tmp/pre-ready-2478-r7.log" 2>/dev/null)"$'\n'
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
        if "$PLAN_SOURCE" --issue "$ISSUE" >"$REPO_ROOT/tmp/plan-source-2478.log" 2>&1; then
            body+="PLAN_SOURCE=PASS"$'\n'
        else
            body+="PLAN_SOURCE=FAIL"$'\n'
            body+="$(tail -5 "$REPO_ROOT/tmp/plan-source-2478.log")"$'\n'
            rc=1
        fi
        finish "R7-gate.txt" "$rc" "$body"
        ;;
    *)
        usage
        ;;
esac
