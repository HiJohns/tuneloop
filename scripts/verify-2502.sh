#!/usr/bin/env bash
# scripts/verify-2502.sh — #2502 移动端「我的」身份徽标（商户名 · 网点名 · 职位）证据生成器。
#
# 用法：bash scripts/verify-2502.sh <r1|r2|r3|r4|r5|r6|r7|r9|r10|r11|r12|r13>
#
# 每行把结果写入 docs/evidence/2502/R*-*.txt（首行为 PROVENANCE），并打印机器可
# 判定的断言键值；任一行断言不成立以非 0 退出。
#
# - r1–r5、r13 走 vitest 页烟测试（scripts/page-smoke/profile-identity-badge-2502.test.jsx，
#   用 `-t` 只跑对应用例；用例通过 process.stdout.write 输出 KEY=value）；
# - r6 走 npx eslint（3 个改动 JS 文件）；
# - r7 静态核对 ui/api/h5-weapp-differences 三篇文档；
# - r9/r10 自持移动端构建（build:h5 / build:weapp，Node 22）；r11 全量页烟；
# - r12 校验 canonical gate 报告（docs/evidence/2502/pre-ready-2502.json）为 PASS。
# R8（pre-ready gate）由 canonical pre-ready-check.sh 产出，不在此脚本内。
set -uo pipefail

ROW="${1:-}"
REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT" || exit 2
OUT_DIR="$REPO_ROOT/docs/evidence/2502"
ISSUE=2502
HEAVY_RUN="${OPENCODE_HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"
TEST_FILE="scripts/page-smoke/profile-identity-badge-2502.test.jsx"
IDENTITY_UTIL="src/utils/identity.js"
H5_PROFILE="src/pages/Profile.jsx"
WEAPP_PROFILE="src/pages-weapp/Profile.jsx"
UI_DOC="docs/spec/ui/ui.md"
API_DOC="docs/spec/api/api.md"
DIFF_DOC="docs/cases/h5-weapp-differences.md"

mkdir -p "$OUT_DIR" "$REPO_ROOT/tmp"

usage() { echo "usage: $0 <r1|r2|r3|r4|r5|r6|r7|r9|r10|r11|r12|r13>" >&2; exit 2; }
[ -n "$ROW" ] || usage

# Node 22（Taro v4 / vitest 兼容；AGENTS.md「Node.js 版本要求」）
NODE22="$(ls -d "$HOME"/.nvm/versions/node/v22*/bin 2>/dev/null | tail -1)"
if [ -z "$NODE22" ] && ! command -v node >/dev/null 2>&1; then
    echo "ERROR:node22-not-found" >&2
    exit 2
fi
[ -n "$NODE22" ] && export PATH="$NODE22:$PATH"

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

# 从 vitest 日志取机器行 `KEY=value`（值可含空格 / `·`，也允许空值）。
# 区分「空值」与「缺行」：缺行返回字面量 missing。
kv() { # log key
    if grep -qE "(^|[[:space:]])$2=" "$1" 2>/dev/null; then
        grep -m1 -E "(^|[[:space:]])$2=" "$1" | sed -E "s/.*$2=//" | head -1
        return 0
    fi
    printf 'missing'
}

# vitest 行：run_ui_row <artifact> <测试名> <KEY=期望值...>
run_ui_row() {
    local artifact=$1 pattern=$2; shift 2
    local log="$REPO_ROOT/tmp/verify-${ISSUE}-${ROW}.log"
    local rc=0 body="" pair key expect actual

    (
        cd "$REPO_ROOT/frontend-mobile" || exit 2
        run_heavy npx vitest run --config vitest.config.js "$TEST_FILE" -t "$pattern" --reporter=verbose
    ) > "$log" 2>&1
    local test_rc=$?
    body+="TEST_FILE=$TEST_FILE"$'\n'
    body+="TEST_PATTERN=$pattern"$'\n'
    body+="TEST_EXIT=$test_rc"$'\n'
    [ "$test_rc" = "0" ] || rc=1
    grep -qE "Tests[[:space:]]+[1-9][0-9]* passed" "$log" || rc=1

    while [ $# -gt 0 ]; do
        pair=$1; shift
        key="${pair%%=*}"
        expect="${pair#*=}"
        actual="$(kv "$log" "$key")"
        body+="$key=$actual"$'\n'
        [ "$actual" = "$expect" ] || rc=1
    done
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish "$artifact" "$rc" "$body"
}

rc=0
case "$ROW" in
r1)
    run_ui_row R1-weapp-badge.txt 'TestProfileBadgeWeapp_2502' \
        'BADGE_TEXT=cadenza · 卡丹萨乐器 · 管理员' 'OLD_TEXT_ABSENT=1'
    ;;
r2)
    run_ui_row R2-fallback.txt 'TestProfileBadgeWeappFallback_2502' \
        'BADGE_TEXT=卡丹萨乐器 · 管理员' 'BADGE_TEXT_ONLY_ROLE=管理员' 'NO_UNDEFINED=1' 'NO_LEADING_SEP=1'
    ;;
r3)
    run_ui_row R3-h5-badge.txt 'TestProfileBadgeH5_2502' \
        'H5_BADGE_TEXT=cadenza · 卡丹萨乐器 · 管理员' 'PARITY=1'
    ;;
r4)
    run_ui_row R4-customer.txt 'TestProfileBadgeCustomer_2502' \
        'CUSTOMER_BADGE_ABSENT=1' 'NO_TENANT_LEAK=1'
    ;;
r5)
    run_ui_row R5-helper-redgreen.txt 'TestIdentityHelperRedGreen_2502' \
        'FORMAT_FULL=a · b' 'FORMAT_SINGLE=b' 'FORMAT_EMPTY=' \
        'ROLE_MULTI=管理员 · 维修师傅' 'ROLE_FALLBACK=员工' \
        'OLD_TEXT=卡丹萨乐器' 'NEW_TEXT=cadenza · 卡丹萨乐器' 'RED_GREEN=1'
    ;;
r6)
    body="" rc=0
    log="$REPO_ROOT/tmp/verify-${ISSUE}-${ROW}.log"
    (
        cd "$REPO_ROOT/frontend-mobile" || exit 2
        run_heavy npx eslint "$IDENTITY_UTIL" "$H5_PROFILE" "$WEAPP_PROFILE"
    ) > "$log" 2>&1
    eslint_exit=$?
    no_undef="$(grep -cE "no-undef" "$log" 2>/dev/null || true)"
    body+="ESLINT_CMD=cd frontend-mobile && npx eslint $IDENTITY_UTIL $H5_PROFILE $WEAPP_PROFILE"$'\n'
    body+="ESLINT_EXIT=$eslint_exit"$'\n'
    body+="NO_UNDEF=${no_undef:-0}"$'\n'
    [ "$eslint_exit" = "0" ] && [ "${no_undef:-0}" = "0" ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R6-lint.txt "$rc" "$body"
    ;;
r7)
    body="" rc=0
    ui=0 api=0 diff=0
    grep -q "头部身份徽标（#2502）" "$UI_DOC" && ui=1
    grep -q "#2502" "$API_DOC" && grep -q "tenant_name" "$API_DOC" && api=1
    grep -q "A23" "$DIFF_DOC" && grep -q "#2502" "$DIFF_DOC" && diff=1
    body+="UI_DOC=$ui"$'\n'
    body+="API_DOC=$api"$'\n'
    body+="DIFF_DOC=$diff"$'\n'
    body+="--- ui.md ---"$'\n'
    body+="$(grep -n "头部身份徽标（#2502）" "$UI_DOC" || true)"$'\n'
    body+="--- api.md ---"$'\n'
    body+="$(grep -n -A3 "GET 组织上下文字段" "$API_DOC" || true)"$'\n'
    body+="--- h5-weapp-differences.md ---"$'\n'
    body+="$(grep -n "^| A23 " "$DIFF_DOC" || true)"$'\n'
    [ "$ui" = "1" ] && [ "$api" = "1" ] && [ "$diff" = "1" ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R7-docs.txt "$rc" "$body"
    ;;
r13)
    body="" rc=0
    log="$REPO_ROOT/tmp/verify-${ISSUE}-${ROW}.log"
    (
        cd "$REPO_ROOT/frontend-mobile" || exit 2
        run_heavy npx vitest run --config vitest.config.js "$TEST_FILE" -t 'TestProfileBadgeLegacyCustomer_2502' --reporter=verbose
    ) > "$log" 2>&1
    test_rc=$?
    absent="$(kv "$log" ORG_ABSENT_BOTH_ENDS)"
    body+="TEST_FILE=$TEST_FILE"$'\n'
    body+="TEST_PATTERN=TestProfileBadgeLegacyCustomer_2502"$'\n'
    body+="TEST_EXIT=$test_rc"$'\n'
    body+="ORG_ABSENT_BOTH_ENDS=$absent"$'\n'
    [ "$test_rc" = "0" ] || rc=1
    grep -qE "Tests[[:space:]]+[1-9][0-9]* passed" "$log" || rc=1
    [ "$absent" = "1" ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R13-legacy-customer.txt "$rc" "$body"
    ;;
r9)
    # 计划 R9 的入口 = `npm run build:h5`（Taro H5），期望结果 = `dist/index.html` 含新 hash。
    # 本仓 `taro` 的 outputRoot 为 dist-h5（无 HTML 入口），发布用 H5 包由 vite `npm run build`
    # 产出到 dist/（Makefile:206-207）→ 两条都跑，使 R9 的入口与产物两列同时成立。
    body="" rc=0
    log="$REPO_ROOT/tmp/verify-${ISSUE}-r9-build-h5.log"
    vite_log="$REPO_ROOT/tmp/verify-${ISSUE}-r9-build-vite.log"
    index_html="$REPO_ROOT/frontend-mobile/dist/index.html"
    taro_app="$REPO_ROOT/frontend-mobile/dist-h5/js/app.js"
    rm -rf "$REPO_ROOT/frontend-mobile/dist" "$REPO_ROOT/frontend-mobile/dist-h5"
    (
        cd "$REPO_ROOT/frontend-mobile" || exit 2
        run_heavy npm run build:h5
    ) > "$log" 2>&1
    build_exit=$?
    (
        cd "$REPO_ROOT/frontend-mobile" || exit 2
        run_heavy npm run build
    ) > "$vite_log" 2>&1
    vite_exit=$?
    hash="missing"
    if [ -f "$index_html" ]; then
        hash="$(grep -oE '(assets|src)/[A-Za-z0-9_./-]+\.js' "$index_html" | head -1)"
        [ -n "$hash" ] || hash="missing"
    fi
    taro_helper="0"
    [ -f "$taro_app" ] && taro_helper="$(grep -c "维修师傅" "$taro_app" 2>/dev/null || true)"
    dist_helper="0"
    if [ -d "$REPO_ROOT/frontend-mobile/dist/assets" ]; then
        dist_helper="$(grep -rl "维修师傅" "$REPO_ROOT/frontend-mobile/dist/assets" 2>/dev/null | wc -l | tr -d ' ')"
    fi
    body+="BUILD_CMD=cd frontend-mobile && npm run build:h5"$'\n'
    body+="BUILD_EXIT=$build_exit"$'\n'
    body+="TARO_H5_BUNDLE=$([ -f "$taro_app" ] && echo present || echo missing)"$'\n'
    body+="TARO_H5_ROLE_LABEL_HITS=${taro_helper:-0}"$'\n'
    body+="DIST_ASSETS_WITH_ROLE_LABEL=${dist_helper:-0}"$'\n'
    body+="VITE_BUILD_CMD=cd frontend-mobile && npm run build"$'\n'
    body+="VITE_BUILD_EXIT=$vite_exit"$'\n'
    body+="H5_INDEX_HTML=$([ -f "$index_html" ] && echo present || echo missing)"$'\n'
    body+="H5_BUNDLE_HASH=$hash"$'\n'
    [ "$build_exit" = "0" ] || rc=1
    [ "$vite_exit" = "0" ] || rc=1
    [ "${dist_helper:-0}" -ge 1 ] || rc=1
    [ -f "$index_html" ] || rc=1
    [ "$hash" != "missing" ] || rc=1
    if [ "$rc" = "0" ]; then body+="BUILD_H5_OK"$'\n'; fi
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R9-build-h5.txt "$rc" "$body"
    ;;
r10)
    body="" rc=0
    log="$REPO_ROOT/tmp/verify-${ISSUE}-r10-build-weapp.log"
    profile_js="$REPO_ROOT/frontend-mobile/dist-weapp/pages-weapp/profile/index.js"
    rm -rf "$REPO_ROOT/frontend-mobile/dist-weapp"
    (
        cd "$REPO_ROOT/frontend-mobile" || exit 2
        run_heavy npm run build:weapp
    ) > "$log" 2>&1
    build_exit=$?
    helper_refs="0"
    size="0"
    if [ -f "$profile_js" ]; then
        size="$(wc -c < "$profile_js" | tr -d ' ')"
        helper_refs="$(grep -c "identityBadgeText" "$profile_js" 2>/dev/null || true)"
    fi
    body+="BUILD_CMD=cd frontend-mobile && npm run build:weapp"$'\n'
    body+="BUILD_EXIT=$build_exit"$'\n'
    body+="WEAPP_PROFILE_BUNDLE=$([ -f "$profile_js" ] && echo present || echo missing)"$'\n'
    body+="WEAPP_PROFILE_BYTES=$size"$'\n'
    body+="HELPER_REF_COUNT=${helper_refs:-0}"$'\n'
    [ "$build_exit" = "0" ] || rc=1
    [ -f "$profile_js" ] || rc=1
    [ "${helper_refs:-0}" -ge 1 ] || rc=1
    if [ "$rc" = "0" ]; then body+="BUILD_WEAPP_OK"$'\n'; fi
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R10-build-weapp.txt "$rc" "$body"
    ;;
r11)
    body="" rc=0
    log="$REPO_ROOT/tmp/verify-${ISSUE}-r11-smoke.log"
    (
        cd "$REPO_ROOT/frontend-mobile" || exit 2
        run_heavy npm run smoke
    ) > "$log" 2>&1
    smoke_exit=$?
    fails="$(sed -nE 's/^[[:space:]]*Tests[[:space:]]+([0-9]+) failed.*/\1/p' "$log" | tail -1)"
    [ -n "$fails" ] || fails=0
    passed="$(sed -nE 's/^[[:space:]]*Tests[[:space:]]+([0-9]+) passed.*/\1/p' "$log" | tail -1)"
    body+="SMOKE_CMD=cd frontend-mobile && npm run smoke"$'\n'
    body+="SMOKE_EXIT=$smoke_exit"$'\n'
    body+="TESTS_PASSED=${passed:-0}"$'\n'
    body+="TESTS_FAIL=$fails"$'\n'
    [ "$smoke_exit" = "0" ] || rc=1
    [ "$fails" = "0" ] || rc=1
    [ "${passed:-0}" -ge 1 ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R11-smoke.txt "$rc" "$body"
    ;;
r12)
    body="" rc=0
    report="$OUT_DIR/pre-ready-$ISSUE.json"
    overall="missing"
    dirty="missing"
    if [ -f "$report" ]; then
        overall="$(python3 -c "import json,sys;d=json.load(open(sys.argv[1]));print(d.get('overall','missing'))" "$report" 2>/dev/null || echo missing)"
        dirty="$(python3 -c "import json,sys;d=json.load(open(sys.argv[1]));print(str((d.get('provenance') or {}).get('dirty','missing')).lower())" "$report" 2>/dev/null || echo missing)"
    fi
    body+="REPORT=$report"$'\n'
    body+="OVERALL=$overall"$'\n'
    body+="PROVENANCE_DIRTY=$dirty"$'\n'
    [ "$overall" = "PASS" ] || rc=1
    [ "$dirty" = "false" ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    # R12 的产物就是 gate 报告本身（canonical pre-ready 输出），此处只校验不覆写。
    printf '%s\n' "$body"
    [ "$rc" = "0" ] || exit 1
    exit 0
    ;;
*)
    usage
    ;;
esac
