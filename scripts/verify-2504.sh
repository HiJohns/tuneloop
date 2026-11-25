#!/usr/bin/env bash
# scripts/verify-2504.sh — #2504「40303 未映射导致英文 access denied 直出」证据生成器（R1–R6）。
#
# 用法：bash scripts/verify-2504.sh <r1|r2|r3|r4|r5|r6>
#
# 每行把结果写入 docs/evidence/2504/R*-*.txt（首行 provenance），并打印机器可判定的
# 断言键值；任一行断言不成立以非 0 退出。
#
# 判定口径（Accepted Plan）：
#   R1 PC 解析器：40303 + access denied → code_map 中文；49999 + ASCII → fallback；8 条 fixture 层序
#   R2 mobile 解析核心：同文案、同层序（与 PC 逐层一致）
#   R3 双端平价脚本：PARITY=1 DIFFS=0 CASES>=12；49999+ASCII 两端 LAYER=fallback
#   R4 旧断言翻转无残留（4 处翻转点，raw_message ASCII 期望归零）
#   R5 入口门控 + 真实链路接线（静态）
#   R6 PC 构建 + 双端 lint
set -uo pipefail

ROW="${1:-}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$REPO_ROOT/docs/evidence/2504"
ISSUE=2504

PC_TEST="frontend-pc/tests/errorMessages.test.mjs"
PC_RESOLVER="frontend-pc/src/services/errorMessages.js"
MB_TEST="frontend-mobile/src/services/errorMessageResolution.test.mjs"
MB_CORE="frontend-mobile/src/services/errorMessageResolution.mjs"
MB_MAP="frontend-mobile/src/services/errorMessages.js"
PARITY="scripts/check-error-message-parity-2480.mjs"
API_DOC="docs/spec/api/api.md"

EXPECT_LAYERS="human_message,code_map,code_map,human_message,message_map,fallback,fallback,human_message"
EXPECT_40303_TEXT="暂无权限访问该页面"
EXPECT_FALLBACK_TEXT="操作失败，请重试"

cd "$REPO_ROOT" || exit 2
mkdir -p "$OUT_DIR" "$REPO_ROOT/tmp"

FIX_SHA="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
[ -n "$FIX_SHA" ] || FIX_SHA="$(git rev-parse HEAD)"
SHORT="$(git rev-parse --short=8 "$FIX_SHA")"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)docs/evidence/$ISSUE" ':(exclude)tmp')" ]; then
    DIRTY=true
fi
PROVENANCE="# build=1.0.$SHORT # commit=$FIX_SHA # dirty=$DIRTY"

finish() { # artifact rc body
    local name=$1 rc=$2 body=$3
    {
        printf '%s\n' "$PROVENANCE"
        printf '%s\n' "$body"
    } > "$OUT_DIR/$name"
    printf '%s\n' "$body"
    [ "$rc" = "0" ] || return 1
}

# tap_val <log> <key>：抽取 node --test TAP 诊断行（形如 "# KEY=VALUE"）里的值。
tap_val() {
    sed -n "s/^# ${2}=//p" "$1" | tail -1
}

count_tests_fail() { # log
    sed -n 's/^# fail \([0-9]*\)$/\1/p' "$1" | tail -1
}

run_pc_tests() { # log
    node --test "$PC_TEST" > "$1" 2>&1
}

run_mb_tests() { # log
    node --test "$MB_TEST" > "$1" 2>&1
}

usage() { echo "usage: $0 <r1|r2|r3|r4|r5|r6>" >&2; exit 2; }
[ -n "$ROW" ] || usage

rc=0
case "$ROW" in
r1)
    log="$REPO_ROOT/tmp/verify-$ISSUE-r1-pc.log"
    run_pc_tests "$log"
    run_rc=$?
    l40303="$(tap_val "$log" L40303)"
    c40303="$(tap_val "$log" C40303)"
    l49999="$(tap_val "$log" L49999_ASCII)"
    c49999="$(tap_val "$log" C49999_ASCII)"
    layers="$(tap_val "$log" LAYERS)"
    fails="$(count_tests_fail "$log")"
    body="TEST_CMD=node --test $PC_TEST"$'\n'
    body+="TEST_EXIT=$run_rc"$'\n'
    body+="TESTS_FAIL=${fails:-unknown}"$'\n'
    body+="L40303=$l40303"$'\n'
    body+="C40303=$c40303"$'\n'
    body+="L49999_ASCII=$l49999"$'\n'
    body+="C49999_ASCII=$c49999"$'\n'
    body+="LAYERS=$layers"$'\n'
    [ "$run_rc" = "0" ] || rc=1
    [ "${fails:-1}" = "0" ] || rc=1
    [ "$l40303" = "code_map" ] || rc=1
    [ "$c40303" = "$EXPECT_40303_TEXT" ] || rc=1
    [ "$l49999" = "fallback" ] || rc=1
    [ "$c49999" = "$EXPECT_FALLBACK_TEXT" ] || rc=1
    [ "$layers" = "$EXPECT_LAYERS" ] || rc=1
    # 负例：任何 fixture 把英文 `access denied` 直出给用户
    if printf '%s\n' "$body" | grep -q 'access denied'; then
        body+="LEAK=1"$'\n'
        rc=1
    else
        body+="LEAK=0"$'\n'
    fi
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R1-pc-resolver.txt "$rc" "$body"
    ;;
r2)
    log="$REPO_ROOT/tmp/verify-$ISSUE-r2-mobile.log"
    run_mb_tests "$log"
    run_rc=$?
    l40303="$(tap_val "$log" L40303)"
    c40303="$(tap_val "$log" C40303)"
    l49999="$(tap_val "$log" L49999_ASCII)"
    c49999="$(tap_val "$log" C49999_ASCII)"
    layers="$(tap_val "$log" LAYERS)"
    fails="$(count_tests_fail "$log")"
    # 与 PC 层序交叉比对（同一 8 条 fixture）
    pc_log="$REPO_ROOT/tmp/verify-$ISSUE-r2-pc-cross.log"
    run_pc_tests "$pc_log"
    pc_layers="$(tap_val "$pc_log" LAYERS)"
    body="TEST_CMD=node --test $MB_TEST"$'\n'
    body+="TEST_EXIT=$run_rc"$'\n'
    body+="TESTS_FAIL=${fails:-unknown}"$'\n'
    body+="L40303=$l40303"$'\n'
    body+="C40303=$c40303"$'\n'
    body+="L49999_ASCII=$l49999"$'\n'
    body+="C49999_ASCII=$c49999"$'\n'
    body+="LAYERS=$layers"$'\n'
    body+="PC_LAYERS=$pc_layers"$'\n'
    [ "$run_rc" = "0" ] || rc=1
    [ "${fails:-1}" = "0" ] || rc=1
    [ "$l40303" = "code_map" ] || rc=1
    [ "$c40303" = "$EXPECT_40303_TEXT" ] || rc=1
    [ "$l49999" = "fallback" ] || rc=1
    [ "$c49999" = "$EXPECT_FALLBACK_TEXT" ] || rc=1
    [ "$layers" = "$EXPECT_LAYERS" ] || rc=1
    [ "$pc_layers" = "$layers" ] || rc=1
    if printf '%s\n' "$body" | grep -q 'access denied'; then
        body+="LEAK=1"$'\n'
        rc=1
    else
        body+="LEAK=0"$'\n'
    fi
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R2-mobile-resolver.txt "$rc" "$body"
    ;;
r3)
    log="$REPO_ROOT/tmp/verify-$ISSUE-r3-parity.log"
    node "$PARITY" > "$log" 2>&1
    parity_rc=$?
    parity="$(sed -n 's/^PARITY=//p' "$log" | tail -1)"
    diffs="$(sed -n 's/^DIFFS=//p' "$log" | tail -1)"
    layer_diffs="$(sed -n 's/^LAYER_DIFFS=//p' "$log" | tail -1)"
    cases="$(sed -n 's/^CASES=//p' "$log" | tail -1)"
    neg_line="$(grep -F 'CASE=49999+ASCII ' "$log" | tail -1)"
    body="PARITY_SCRIPT=$PARITY"$'\n'
    body+="PARITY_EXIT=$parity_rc"$'\n'
    body+="PARITY=$parity"$'\n'
    body+="DIFFS=${diffs:-unknown}"$'\n'
    body+="LAYER_DIFFS=${layer_diffs:-unknown}"$'\n'
    body+="CASES=${cases:-0}"$'\n'
    body+="NEG_CASE=$neg_line"$'\n'
    [ "$parity_rc" = "0" ] || rc=1
    [ "$parity" = "1" ] || rc=1
    [ "${diffs:-1}" = "0" ] || rc=1
    [ "${layer_diffs:-1}" = "0" ] || rc=1
    [ "${cases:-0}" -ge 12 ] || rc=1
    case "$neg_line" in
        *"PARITY=1 LAYER=fallback"*) ;;
        *) rc=1 ;;
    esac
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R3-parity.txt "$rc" "$body"
    ;;
r4)
    pc_unknown="$(grep -cE "\{ key: 'UNKNOWN', input: \{ code: 49999, message: 'unknown conflict' \}, layer: 'fallback'" "$PC_TEST" || true)"
    mb_unknown="$(grep -cE "\{ key: 'UNKNOWN', input: \{ code: 49999, message: 'unknown conflict' \}, layer: 'fallback'" "$MB_TEST" || true)"
    mb_special="$(grep -cF "}).layer, 'fallback')" "$MB_TEST" || true)"
    parity_neg="$(grep -cF "expectLayer: 'fallback'" "$PARITY" || true)"
    flip_count=$(( $( [ "${pc_unknown:-0}" -ge 1 ] && echo 1 || echo 0 ) \
                 + $( [ "${mb_unknown:-0}" -ge 1 ] && echo 1 || echo 0 ) \
                 + $( [ "${mb_special:-0}" -ge 1 ] && echo 1 || echo 0 ) \
                 + $( [ "${parity_neg:-0}" -ge 1 ] && echo 1 || echo 0 ) ))
    # 旧行为残留：断言中出现 raw_message 层，或仍期望把 ASCII 原文当作文案
    stale_pc="$(grep -cE "layer: 'raw_message'" "$PC_TEST" || true)"
    stale_mb="$(grep -cE "layer: 'raw_message'" "$MB_TEST" || true)"
    old_text_pc="$(grep -cF "text: 'unknown conflict'" "$PC_TEST" || true)"
    body="FLIP_PC_UNKNOWN=$pc_unknown"$'\n'
    body+="FLIP_MOBILE_UNKNOWN=$mb_unknown"$'\n'
    body+="FLIP_MOBILE_SPECIAL=$mb_special"$'\n'
    body+="FLIP_PARITY_NEG=$parity_neg"$'\n'
    body+="FLIP_COUNT=$flip_count"$'\n'
    body+="STALE_RAW_PC=${stale_pc:-0}"$'\n'
    body+="STALE_RAW_MOBILE=${stale_mb:-0}"$'\n'
    body+="OLD_TEXT_PC=${old_text_pc:-0}"$'\n'
    [ "$flip_count" = "4" ] || rc=1
    [ "${stale_pc:-1}" = "0" ] || rc=1
    [ "${stale_mb:-1}" = "0" ] || rc=1
    [ "${old_text_pc:-1}" = "0" ] || rc=1
    old_gone=0
    if [ "${stale_pc:-1}" = "0" ] && [ "${stale_mb:-1}" = "0" ] && [ "${old_text_pc:-1}" = "0" ]; then
        old_gone=1
    fi
    body+="OLD_BEHAVIOR_GONE=$old_gone"$'\n'
    [ "$old_gone" = "1" ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R4-flip.txt "$rc" "$body"
    ;;
r5)
    menu_gated=0
    if grep -Eq "\{ key: '/staff', label: '人员管理', permission: \{ sysPermBits: \[15\], cusPermCodes: \[[^]]*\], requireAll: true \} \}" frontend-pc/src/App.jsx; then
        menu_gated=1
    fi
    require_all_logic=0
    if grep -Fq 'if (requireAll || requireAllGroups) return sysMatch && cusMatch' frontend-pc/src/config/menuPermissions.js; then
        require_all_logic=1
    fi
    api_resolves=0
    if grep -Fq 'throw new Error(resolveErrorMessage({ code: error.code, message: error.message }' frontend-pc/src/services/api.js; then
        api_resolves=1
    fi
    page_from_resolver=0
    if grep -Fq "error.message" frontend-pc/src/pages/StaffManagement.jsx; then
        page_from_resolver=1
    fi
    direct_leak="$(grep -cE "error\.response|response\?\.data\?\.message" frontend-pc/src/pages/StaffManagement.jsx || true)"
    body="MENU_GATED=$menu_gated"$'\n'
    body+="REQUIRE_ALL_LOGIC=$require_all_logic"$'\n'
    body+="API_RESOLVES=$api_resolves"$'\n'
    body+="PAGE_MESSAGE_FROM_RESOLVER=$page_from_resolver"$'\n'
    body+="DIRECT_ENGLISH_LEAK=${direct_leak:-0}"$'\n'
    assert_keys="App.jsx '/staff' menu + menuPermissions.requireAll + api.js resolveErrorMessage + StaffManagement error.message"$'\n'
    body+="ASSERTS=$assert_keys"
    [ "$menu_gated" = "1" ] || rc=1
    [ "$require_all_logic" = "1" ] || rc=1
    [ "$api_resolves" = "1" ] || rc=1
    [ "$page_from_resolver" = "1" ] || rc=1
    [ "${direct_leak:-1}" = "0" ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R5-wiring-static.txt "$rc" "$body"
    ;;
r6)
    build_log="$REPO_ROOT/tmp/verify-$ISSUE-r6-build.log"
    ( cd "$REPO_ROOT/frontend-pc" && npm run build ) > "$build_log" 2>&1
    build_exit=$?
    lint_pc_log="$REPO_ROOT/tmp/verify-$ISSUE-r6-lint-pc.log"
    ( cd "$REPO_ROOT/frontend-pc" && ./node_modules/.bin/eslint src/services/errorMessages.js ) > "$lint_pc_log" 2>&1
    lint_pc_exit=$?
    lint_mb_log="$REPO_ROOT/tmp/verify-$ISSUE-r6-lint-mobile.log"
    ( cd "$REPO_ROOT/frontend-mobile" && ./node_modules/.bin/eslint src/services/errorMessages.js src/services/errorMessageResolution.mjs src/services/errorMessageResolution.test.mjs ) > "$lint_mb_log" 2>&1
    lint_mb_exit=$?
    body="BUILD_CMD=cd frontend-pc && npm run build"$'\n'
    body+="PC_BUILD=$build_exit"$'\n'
    body+="LINT_PC=$lint_pc_exit"$'\n'
    body+="LINT_MOBILE=$lint_mb_exit"$'\n'
    [ "$build_exit" = "0" ] || rc=1
    [ "$lint_pc_exit" = "0" ] || rc=1
    [ "$lint_mb_exit" = "0" ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R6-build-lint.txt "$rc" "$body"
    ;;
*)
    usage
    ;;
esac
