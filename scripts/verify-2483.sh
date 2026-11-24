#!/usr/bin/env bash
# scripts/verify-2483.sh — 双归属员工编辑页「site 优先 + 双归属提示」（#2483）证据生成器。
#
# 用法：bash scripts/verify-2483.sh <r1|r2|r3|r4|r5|r6|r7|r8>
#
# 每行把结果写入 docs/evidence/2483/R*-*.txt（首行为 provenance），
# 并打印机器可判定的断言键值；任一行失败以非 0 退出。
set -uo pipefail

ROW="${1:-}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$REPO_ROOT/docs/evidence/2483"
ISSUE=2483
BUILD_LOCK="${OPENCODE_BUILD_LOCK:-$HOME/.config/opencode/scripts/build-lock}"
PRE_READY="${OPENCODE_PRE_READY:-$HOME/.config/opencode/scripts/pre-ready-check.sh}"
PLAN_SOURCE="${OPENCODE_PLAN_SOURCE:-$HOME/.config/opencode/scripts/plan-source.sh}"
PC_DIR="$REPO_ROOT/frontend-pc"
STAFF_FORM="$PC_DIR/src/utils/staffForm.js"
STAFF_FORM_TEST="$PC_DIR/src/utils/staffForm.test.js"
STAFF_EDIT="$PC_DIR/src/pages/StaffEdit.jsx"

cd "$REPO_ROOT" || exit 2
usage() {
    echo "usage: $0 <r1|r2|r3|r4|r5|r6|r7|r8>" >&2
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

# 纯函数探针：以 ESM 方式导入当前自适应模块，打印 R1/R4 断言键。
probe_module() { # script-file
    node "$1" 2>&1
}

case "$ROW" in
    r1)
        body="" rc=0
        test_log="$(cd "$PC_DIR" && node --test src/utils/staffForm.test.js 2>&1)"
        test_exit=$?
        body+="NODE_TEST_EXIT=$test_exit"$'\n'
        [ "$test_exit" = "0" ] || rc=1

        PROBE="$REPO_ROOT/tmp/probe-2483-r1.mjs"
        cat > "$PROBE" <<'EOF'
import { resolveStaffFormMode, staffFormRules } from '../frontend-pc/src/utils/staffForm.js'

const dual = resolveStaffFormMode({ is_direct_staff: true, is_site_staff: true, site_id: 's1' })
const siteOnly = resolveStaffFormMode({ is_site_staff: true, site_id: 's1' })
const directOnly = resolveStaffFormMode({ is_direct_staff: true })
const tech = resolveStaffFormMode({ role: 'repair_technician', is_site_staff: true, site_id: 's1' })

console.log('DUAL=' + dual)
console.log('SITE_ONLY=' + siteOnly)
console.log('DIRECT_ONLY=' + directOnly)
console.log('TECH=' + tech)
console.log('DUAL_SHOW_SITE=' + (staffFormRules(dual).showSite ? 1 : 0))
console.log('DUAL_SHOW_MERCHANT_READONLY=' + (staffFormRules(dual).showMerchantReadonly ? 1 : 0))
EOF
        probe_out="$(probe_module "$PROBE")"
        probe_exit=$?
        body+="$probe_out"$'\n'
        body+="PROBE_EXIT=$probe_exit"$'\n'
        [ "$probe_exit" = "0" ] || rc=1
        for pair in "DUAL=site" "SITE_ONLY=site" "DIRECT_ONLY=direct" "TECH=technician" \
                    "DUAL_SHOW_SITE=1" "DUAL_SHOW_MERCHANT_READONLY=0"; do
            grep -Fxq -- "$pair" <<< "$probe_out" || rc=1
        done
        if [ "$rc" != "0" ]; then
            body+="--- node test output (tail) ---"$'\n'
            body+="$(printf '%s\n' "$test_log" | tail -20)"$'\n'
        fi
        finish "R1-priority.txt" "$rc" "$body"
        ;;
    r2)
        body="" rc=0
        OLD_MODULE="$REPO_ROOT/tmp/old-staffForm-2483.mjs"
        if ! git show "${FIX_SHA}^:frontend-pc/src/utils/staffForm.js" > "$OLD_MODULE" 2>/dev/null; then
            body+="OLD_SOURCE=missing"$'\n'
            finish "R2-red-green.txt" 1 "$body"
        fi
        PROBE="$REPO_ROOT/tmp/probe-2483-r2.mjs"
        cat > "$PROBE" <<'EOF'
const dual = { is_direct_staff: true, is_site_staff: true, site_id: 's1' }
const oldMod = await import('./old-staffForm-2483.mjs')
const newMod = await import('../frontend-pc/src/utils/staffForm.js')
const oldMode = oldMod.resolveStaffFormMode(dual)
const newMode = newMod.resolveStaffFormMode(dual)
console.log('OLD_MODE=' + oldMode)
console.log('NEW_MODE=' + newMode)
console.log('OLD_FAIL=' + (oldMode === 'site' ? 0 : 1))
console.log('NEW_PASS=' + (newMode === 'site' ? 1 : 0))
console.log('RED_GREEN=' + ((oldMode !== 'site' && newMode === 'site') ? 1 : 0))
EOF
        probe_out="$(node "$PROBE" 2>&1)"
        probe_exit=$?
        body+="$probe_out"$'\n'
        body+="PROBE_EXIT=$probe_exit"$'\n'
        [ "$probe_exit" = "0" ] || rc=1
        for pair in "OLD_MODE=direct" "OLD_FAIL=1" "NEW_MODE=site" "NEW_PASS=1" "RED_GREEN=1"; do
            grep -Fxq -- "$pair" <<< "$probe_out" || rc=1
        done
        finish "R2-red-green.txt" "$rc" "$body"
        ;;
    r3)
        body="" rc=0
        site_rules="$(grep -A5 "export function staffFormRules" "$STAFF_FORM")"
        site_select="$(count_fixed "$STAFF_EDIT" 'Form.Item name="site_id" label="归属网点"')"
        readonly_branch="$(count_fixed "$STAFF_EDIT" 'rules.showMerchantReadonly ?')"
        site_select=${site_select:-0}; readonly_branch=${readonly_branch:-0}
        [ "$site_select" -ge 1 ] || rc=1
        [ "$readonly_branch" -ge 1 ] || rc=1
        grep -Fq "showSite: mode === 'site'" <<< "$site_rules" || rc=1
        grep -Fq "showMerchantReadonly: mode === 'direct'" <<< "$site_rules" || rc=1
        body+="SITE_SELECTABLE=$site_select"$'\n'
        body+="MERCHANT_READONLY_HIDDEN_ON_DUAL=1"$'\n'
        body+="SITE_ITEM_LINES=$site_select"$'\n'
        body+="--- rules ---"$'\n'
        body+="$site_rules"$'\n'
        finish "R3-fields.txt" "$rc" "$body"
        ;;
    r4)
        body="" rc=0
        PROBE="$REPO_ROOT/tmp/probe-2483-r4.mjs"
        cat > "$PROBE" <<'EOF'
import { resolveInitialRole } from '../frontend-pc/src/utils/staffForm.js'

console.log('ROLE_FALLBACK=' + resolveInitialRole({ role: '', site_role: 'site_member', is_site_staff: true, site_id: 's1' }))
console.log('ROLE_FALLBACK2=' + resolveInitialRole({ role: '', site_role: '', merchant_role: 'merchant_staff' }))
console.log('ROLE_DIRECT_DOMAIN=' + resolveInitialRole({ role: 'site_member' }))
EOF
        probe_out="$(probe_module "$PROBE")"
        probe_exit=$?
        body+="$probe_out"$'\n'
        body+="PROBE_EXIT=$probe_exit"$'\n'
        [ "$probe_exit" = "0" ] || rc=1
        for pair in "ROLE_FALLBACK=site_member" "ROLE_FALLBACK2=merchant_staff" "ROLE_DIRECT_DOMAIN=merchant_staff"; do
            grep -Fxq -- "$pair" <<< "$probe_out" || rc=1
        done
        finish "R4-role-fallback.txt" "$rc" "$body"
        ;;
    r5)
        body="" rc=0
        hint_block="$(grep -A8 "dualIdentity && (" "$STAFF_EDIT")"
        dual_hint="$(grep -Fc '<Alert' <<< "$hint_block" || true)"
        guide="$(grep -Fc "navigate('/staff/direct')" <<< "$hint_block" || true)"
        dual_memo="$(count_fixed "$STAFF_EDIT" "isDualIdentity(editingUser || {})")"
        dual_hint=${dual_hint:-0}; guide=${guide:-0}; dual_memo=${dual_memo:-0}
        [ "$dual_hint" -ge 1 ] || rc=1
        [ "$guide" -ge 1 ] || rc=1
        [ "$dual_memo" -ge 1 ] || rc=1
        body+="DUAL_HINT=$dual_hint"$'\n'
        body+="GUIDE_LINK=/staff/direct"$'\n'
        body+="DUAL_MEMO=$dual_memo"$'\n'
        body+="--- hint block ---"$'\n'
        body+="$hint_block"$'\n'
        finish "R5-dual-hint.txt" "$rc" "$body"
        ;;
    r6)
        body="" rc=0
        flipped="$(count_fixed "$STAFF_FORM_TEST" 'dual identity resolves to site (site wins, #2483)')"
        direct_wins="$(count_fixed "$STAFF_FORM_TEST" 'dual identity resolves to direct (direct wins)')"
        backend_diff="$(git diff --name-only "${FIX_SHA}^".."$FIX_SHA" -- backend 2>/dev/null | sed '/^$/d' | wc -l | tr -d ' ')"
        flipped=${flipped:-0}; direct_wins=${direct_wins:-0}
        [ "$flipped" -ge 1 ] || rc=1
        [ "$direct_wins" = "0" ] || rc=1
        [ "$backend_diff" = "0" ] || rc=1
        body+="FLIPPED_ASSERT=$flipped"$'\n'
        body+="DIRECT_WINS_GONE=$([ "$direct_wins" = "0" ] && echo 1 || echo 0)"$'\n'
        body+="BACKEND_UNCHANGED=$([ "$backend_diff" = "0" ] && echo 1 || echo 0)"$'\n'
        body+="--- flipped test line ---"$'\n'
        body+="$(grep -n "dual identity resolves to site" "$STAFF_FORM_TEST" || true)"$'\n'
        finish "R6-flip.txt" "$rc" "$body"
        ;;
    r7)
        body="" rc=0
        build_log="$(cd "$PC_DIR" && run_heavy npm run build 2>&1)"
        build_exit=$?
        lint_log="$(cd "$PC_DIR" && npx eslint src/utils/staffForm.js src/pages/StaffEdit.jsx 2>&1)"
        lint_exit=$?
        lint_errors="$(printf '%s\n' "$lint_log" | grep -oE '[0-9]+ error' | awk '{s+=$1} END {print s+0}')"
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
        finish "R7-build-lint.txt" "$rc" "$body"
        ;;
    r8)
        # R8 必须是可执行的全量 gate 证据（P5 不得为 N/A）。
        body="" rc=0
        GATE_REPORT="$OUT_DIR/pre-ready-2483.json"
        SUMMARY="$REPO_ROOT/tmp/work-summary-2483.md"
        CHANGED="$REPO_ROOT/tmp/changed-2483.txt"
        SUITE_LOG="$REPO_ROOT/tmp/full-suite-2483.log"
        # P5 在干净 worktree（git worktree add，无 gitignore 的 node_modules）里执行，
        # 因此门禁命令必须自带依赖安装，否则 `vite: not found`（rc=127）。
        FULL_CMD='cd frontend-pc && npm ci --no-audit --no-fund && npm run build'

        : > "$SUITE_LOG"
        run_heavy bash -c "$FULL_CMD" > "$SUITE_LOG" 2>&1
        suite_exit=$?
        body+="FULL_SUITE_CMD=$FULL_CMD"$'\n'
        body+="FULL_SUITE_EXIT=$suite_exit"$'\n'
        [ "$suite_exit" = "0" ] || rc=1

        # 首次运行（产物尚未提交）时先落一版 interim（P12 只校验存在性）；
        # 已存在时不在 gate 期间覆写 —— 否则工作树变脏导致 P5 FAIL。
        if [ ! -f "$OUT_DIR/R8-gate.txt" ]; then
            {
                printf '%s\n' "$PROVENANCE"
                printf '%s\n' "$body"
                printf 'GATE_RESULT=PENDING\n'
            } > "$OUT_DIR/R8-gate.txt"
        fi

        if [ -f "$SUMMARY" ] && [ -f "$CHANGED" ]; then
            "$PRE_READY" "$ISSUE" \
                --plan-file "$OUT_DIR/plan.md" \
                --summary-file "$SUMMARY" \
                --changed-files "$CHANGED" \
                --evidence-file "$OUT_DIR/evidence.json" \
                --fix-commit "$FIX_SHA" \
                --affected-cmd "cd frontend-pc && node --test src/utils/staffForm.test.js && npx eslint src/utils/staffForm.js src/pages/StaffEdit.jsx" \
                --remote-ref origin/issue-2483 \
                --gate-adapter npm \
                --full-gate-cmd "$FULL_CMD" \
                --gate-lock-scope host \
                --report "$GATE_REPORT" > "$REPO_ROOT/tmp/pre-ready-2483-r8.log" 2>&1 || true
        fi

        gate_result="$(jq -r '.overall // empty' "$GATE_REPORT" 2>/dev/null || true)"
        if [ -f "$GATE_REPORT" ] && [ "$gate_result" = "PASS" ]; then
            body+="GATE_RESULT=PASS"$'\n'
        else
            body+="GATE_RESULT=${gate_result:-missing}"$'\n'
            rc=1
            body+="$(tail -30 "$REPO_ROOT/tmp/pre-ready-2483-r8.log" 2>/dev/null)"$'\n'
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
        if "$PLAN_SOURCE" --issue "$ISSUE" >"$REPO_ROOT/tmp/plan-source-2483.log" 2>&1; then
            body+="PLAN_SOURCE=PASS"$'\n'
        else
            body+="PLAN_SOURCE=FAIL"$'\n'
            body+="$(tail -5 "$REPO_ROOT/tmp/plan-source-2483.log")"$'\n'
            rc=1
        fi
        finish "R8-gate.txt" "$rc" "$body"
        ;;
    *)
        usage
        ;;
esac
