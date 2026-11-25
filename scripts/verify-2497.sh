#!/usr/bin/env bash
# scripts/verify-2497.sh — #2497「5xx 内部诊断不得透传」证据生成器（R1–R6）。
#
# 用法：bash scripts/verify-2497.sh <r1|r2|r3|r4|r5|r6>
#
# 每行把结果写入 docs/evidence/2497/R*-*.txt（首行 provenance），并打印机器可判定的
# 断言键值；任一行断言不成立以非 0 退出。
#
# 判定口径（Accepted Plan）：
#   R1 后端 CJK+err.Error() 拼接归零（正则即唯一口径，基线 12 文件）
#   R2 内部错误仍进日志（LOG_PRESENT >= 22，且无 detail 丢失）
#   R3 5xx 中文内部串不得回显（PC 解析器输出通用文案）
#   R4 业务码中文仍原样透传（#2480 不回退）
#   R5 PC/mobile 平价（PARITY=1）
#   R6 go build + eslint + PC 全量构建
set -uo pipefail

ROW="${1:-}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$REPO_ROOT/docs/evidence/2497"
ISSUE=2497
SCAN_RE='"message":\s*"[^"]*[\x{4e00}-\x{9fff}][^"]*"\s*\+\s*err\.Error\(\)'
BACKEND_FILES=(
    invoice.go merchant.go merchant_invoice.go merchant_member.go payscore_handler.go
    registration_session.go site_member.go smtp_config.go transit_site_admin.go
    user_management.go user_staff.go webhook_config.go
)
PC_FILES=(
    frontend-pc/src/services/errorMessages.js
    frontend-pc/tests/errorMessages-5xx-2497.test.mjs
)
INTERNAL_MSG='标记删除失败：pq: duplicate key value violates unique constraint'

cd "$REPO_ROOT" || exit 2
mkdir -p "$OUT_DIR" "$REPO_ROOT/tmp"

if ! command -v go >/dev/null 2>&1 && [ -x /usr/local/go/bin/go ]; then
    export PATH="/usr/local/go/bin:$PATH"
fi
# 后端测试库与构建缓存（可用环境变量覆盖）。
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_local}"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"

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

finish() { # artifact rc body
    local name=$1 rc=$2 body=$3
    {
        printf '%s\n' "$PROVENANCE"
        printf '%s\n' "$body"
    } > "$OUT_DIR/$name"
    printf '%s\n' "$body"
    [ "$rc" = "0" ] || return 1
}

cjk_err_hits() {
    local total=0 f n
    for f in "${BACKEND_FILES[@]}"; do
        n="$(grep -Pc "$SCAN_RE" "backend/handlers/$f" 2>/dev/null || true)"
        [ -n "$n" ] || n=0
        total=$((total + n))
    done
    printf '%s' "$total"
}

log_present() {
    local total=0 f n
    for f in "${BACKEND_FILES[@]}"; do
        n="$(grep -cE 'log\.Printf\(.*err' "backend/handlers/$f" 2>/dev/null || true)"
        [ -n "$n" ] || n=0
        total=$((total + n))
    done
    printf '%s' "$total"
}

node_row() { # code message
    node --input-type=module -e "
import { resolveErrorCore as pc } from '$REPO_ROOT/frontend-pc/src/services/errorMessages.js'
import { resolveErrorCore as mb } from '$REPO_ROOT/frontend-mobile/src/services/errorMessageResolution.mjs'
const input = { code: $1, message: process.env.MSG }
const pcOut = pc(input)
const mbOut = mb(input, { codeMap: { 40900: '数据冲突，请刷新后重试', 50000: '服务器开小差了，请稍后再试', 50001: '服务暂不可用，请稍后再试' } })
console.log('PC_TEXT=' + pcOut.text)
console.log('PC_LAYER=' + pcOut.layer)
console.log('MOBILE_TEXT=' + mbOut.text)
console.log('MOBILE_LAYER=' + mbOut.layer)
"
}

rc=0
case "$ROW" in
r1)
    hits="$(cjk_err_hits)"
    existing=0
    for f in "${BACKEND_FILES[@]}"; do
        [ -f "backend/handlers/$f" ] && existing=$((existing + 1))
    done
    body="SCAN_RE=$SCAN_RE"$'\n'
    body+="CJK_ERR_CONCAT=$hits"$'\n'
    body+="BASELINE_FILES=${#BACKEND_FILES[@]}"$'\n'
    body+="SCANNED_FILES=$existing"$'\n'
    [ "$hits" = "0" ] || rc=1
    [ "$existing" = "${#BACKEND_FILES[@]}" ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R1-scan.txt "$rc" "$body"
    ;;
r2)
    logs="$(log_present)"
    hits="$(cjk_err_hits)"
    body="LOG_PRESENT=$logs"$'\n'
    body+="LOG_MIN=22"$'\n'
    body+="CJK_ERR_CONCAT=$hits"$'\n'
    body+="LOST_DETAIL=$([ "$hits" = "0" ] && echo 0 || echo 1)"$'\n'
    [ "$logs" -ge 22 ] || rc=1
    [ "$hits" = "0" ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R2-logs.txt "$rc" "$body"
    ;;
r3)
    out="$(MSG="$INTERNAL_MSG" node_row 50000)"
    text="$(printf '%s\n' "$out" | sed -n 's/^PC_TEXT=//p')"
    layer="$(printf '%s\n' "$out" | sed -n 's/^PC_LAYER=//p')"
    leak=0
    case "$text" in
        *pq:*|*duplicate*|*err.Error*|*"$INTERNAL_MSG"*) leak=1 ;;
    esac
    [ "$text" = "服务器开小差了，请稍后再试" ] || leak=1
    body="INPUT_CODE=50000"$'\n'
    body+="INPUT_MESSAGE=$INTERNAL_MSG"$'\n'
    body+="$out"$'\n'
    body+="LEAK=$leak"$'\n'
    [ "$leak" = "0" ] || rc=1
    [ "$layer" = "code_map" ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R3-5xx-guard.txt "$rc" "$body"
    ;;
r4)
    out="$(MSG='网点名称已存在' node_row 40900)"
    text="$(printf '%s\n' "$out" | sed -n 's/^PC_TEXT=//p')"
    layer="$(printf '%s\n' "$out" | sed -n 's/^PC_LAYER=//p')"
    passthrough=0
    [ "$text" = "网点名称已存在" ] && passthrough=1
    body="INPUT_CODE=40900"$'\n'
    body+="INPUT_MESSAGE=网点名称已存在"$'\n'
    body+="$out"$'\n'
    body+="PASSTHROUGH=$passthrough"$'\n'
    [ "$passthrough" = "1" ] || rc=1
    [ "$layer" = "human_message" ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R4-passthrough.txt "$rc" "$body"
    ;;
r5)
    parity_log="$REPO_ROOT/tmp/verify-$ISSUE-r5.log"
    node scripts/check-error-message-parity-2480.mjs > "$parity_log" 2>&1
    parity_rc=$?
    cases="$(sed -n 's/.*CASES=\([0-9]*\).*/\1/p' "$parity_log" | tail -1)"
    diffs="$(sed -n 's/.*DIFFS=\([0-9]*\).*/\1/p' "$parity_log" | tail -1)"
    parity=0
    grep -q 'PARITY=1' "$parity_log" && parity=1
    body="PARITY_SCRIPT=scripts/check-error-message-parity-2480.mjs"$'\n'
    body+="PARITY_EXIT=$parity_rc"$'\n'
    body+="PARITY=$parity"$'\n'
    body+="CASES=${cases:-0}"$'\n'
    body+="DIFFS=${diffs:-0}"$'\n'
    [ "$parity_rc" = "0" ] && [ "$parity" = "1" ] && [ "${diffs:-1}" = "0" ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R5-parity.txt "$rc" "$body"
    ;;
r6)
    build_log="$REPO_ROOT/tmp/verify-$ISSUE-r6-build.log"
    ( cd "$REPO_ROOT/backend" && go build ./... ) > "$build_log" 2>&1
    build_exit=$?
    lint_log="$REPO_ROOT/tmp/verify-$ISSUE-r6-eslint.log"
    ( cd "$REPO_ROOT/frontend-pc" && ./node_modules/.bin/eslint "${PC_FILES[@]/frontend-pc\//}" ) > "$lint_log" 2>&1
    lint_exit=$?
    npm_log="$REPO_ROOT/tmp/verify-$ISSUE-r6-npm.log"
    ( cd "$REPO_ROOT/frontend-pc" && npm run build ) > "$npm_log" 2>&1
    npm_exit=$?
    body="BUILD_CMD=cd backend && go build ./..."$'\n'
    body+="BUILD_EXIT=$build_exit"$'\n'
    body+="ESLINT_EXIT=$lint_exit"$'\n'
    body+="NPM_BUILD_EXIT=$npm_exit"$'\n'
    [ "$build_exit" = "0" ] && body+="BUILD_OK"$'\n' || rc=1
    [ "$lint_exit" = "0" ] && body+="ESLINT_OK"$'\n' || rc=1
    [ "$npm_exit" = "0" ] && body+="NPM_BUILD_OK"$'\n' || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R6-build.txt "$rc" "$body"
    ;;
*)
    usage
    ;;
esac
