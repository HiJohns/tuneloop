#!/usr/bin/env bash
# scripts/verify-2497.sh — #2497「5xx 内部诊断不得透传」证据生成器（R1–R8）。
#
# 用法：bash scripts/verify-2497.sh <r1|r2|r3|r4|r5|r6|r7|r8|r9>
#
# 每行把结果写入 docs/evidence/2497/R*-*.txt（首行 provenance），并打印机器可判定的
# 断言键值；任一行断言不成立以非 0 退出。
#
# 判定口径（Accepted Plan v3 / D5）：
#   R1 后端 CJK+任意 .Error() 拼接归零（**目录级**扫描 backend/handlers/*.go，不硬编码文件清单）
#   R2 内部错误仍进日志（逐点：origin/develop 基线中每个拼接点都有引用同一 error 的 log 调用）
#   R3 5xx 中文内部串不得回显（PC 解析器输出通用文案）
#   R4 业务码中文仍原样透传（#2480 不回退）
#   R5 PC/mobile 平价（PARITY=1）+ 5xx 双端回归 + mobile 生产 chokepoint 可达
#   R6 go build + eslint + PC 全量构建 + mobile chokepoint 接线
#   R7 全量后端回归（go test ./... -timeout 20m）
#   R8 四源可复现（plan-source.sh → PLAN_SOURCE: PASS）
#   R9 mobile H5 构建（Node 22，frontend-mobile npm run build:h5）
#
# 重命令（go build/test、npm build）经 heavy-run.sh 持 host 锁；已处锁内（gate）时用
# VERIFY_2497_NO_LOCK=1 跳过二次取锁。
set -uo pipefail

ROW="${1:-}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$REPO_ROOT/docs/evidence/2497"
ISSUE=2497
SCAN_RE='"message":\s*"[^"]*[\x{4e00}-\x{9fff}][^"]*"\s*\+\s*[A-Za-z_][\w.]*\.Error\(\)'
BASELINE_REF="${VERIFY_2497_BASELINE_REF:-origin/develop}"
D5_BASELINE_FILES_MIN=15
PC_FILES=(
    frontend-pc/src/services/errorMessages.js
    frontend-pc/tests/errorMessages-5xx-2497.test.mjs
)
INTERNAL_MSG='标记删除失败：pq: duplicate key value violates unique constraint'
HEAVY="${OPENCODE_HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"

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

usage() { echo "usage: $0 <r1|r2|r3|r4|r5|r6|r7|r8|r9>" >&2; exit 2; }
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

heavy() { # run a heavy command under the host lock (standalone rows only)
    if [ "${VERIFY_2497_NO_LOCK:-0}" = "1" ]; then
        "$@"
    else
        bash "$HEAVY" -- "$@"
    fi
}

handler_files() { # directory-wide scan set: backend/handlers/*.go
    find backend/handlers -maxdepth 1 -type f -name '*.go' | LC_ALL=C sort
}

baseline_sites() { # "file|line|receiver" for every CJK+.Error() join on the baseline ref
    git grep -nP "$SCAN_RE" "$BASELINE_REF" -- 'backend/handlers/*.go' 2>/dev/null \
        | sed "s|^$BASELINE_REF:||" \
        | sed -E 's/^([^:]+):([0-9]+):.*\+\s*([A-Za-z_][A-Za-z0-9_.]*)\.Error\(\).*/\1|\2|\3/'
}

cjk_err_hits() { # directory-wide hit count in the current tree
    local total=0 f n
    while IFS= read -r f; do
        n="$(grep -Pc "$SCAN_RE" "$f" 2>/dev/null || true)"
        [ -n "$n" ] || n=0
        total=$((total + n))
    done < <(handler_files)
    printf '%s' "$total"
}

cjk_err_hit_lines() { # file:line:content for every hit in the current tree
    local f
    while IFS= read -r f; do
        grep -Pn "$SCAN_RE" "$f" 2>/dev/null | sed "s|^|$f:|"
    done < <(handler_files)
}

log_present() { # legacy global count of log lines carrying an error receiver
    local total=0 f n
    while IFS= read -r f; do
        n="$(grep -cE 'log\.(Printf|Println|Print)\(.*\b(err|Err|error)\b' "$f" 2>/dev/null || true)"
        [ -n "$n" ] || n=0
        total=$((total + n))
    done < <(handler_files)
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
    mapfile -t SCANNED < <(handler_files)
    hits="$(cjk_err_hits)"
    hit_lines="$(cjk_err_hit_lines)"
    baseline_rows="$(baseline_sites)"
    baseline_files="$(printf '%s\n' "$baseline_rows" | cut -d'|' -f1 | sed '/^$/d' | sort -u)"
    baseline_file_count="$(printf '%s\n' "$baseline_files" | sed '/^$/d' | wc -l | tr -d ' ')"
    baseline_hit_count="$(printf '%s\n' "$baseline_rows" | sed '/^$/d' | wc -l | tr -d ' ')"
    covered=1
    missing_files=""
    while IFS= read -r bf; do
        [ -n "$bf" ] || continue
        if [ ! -f "$bf" ]; then
            covered=0
            missing_files+="$bf "
        fi
    done <<< "$baseline_files"
    body="SCAN_SCOPE=backend/handlers/*.go（目录级，不硬编码文件清单）"$'\n'
    body+="SCAN_RE=$SCAN_RE"$'\n'
    body+="BASELINE_REF=$BASELINE_REF"$'\n'
    body+="BASELINE_FILES=$baseline_file_count"$'\n'
    body+="BASELINE_HITS=$baseline_hit_count"$'\n'
    body+="D5_BASELINE_FILES_MIN=$D5_BASELINE_FILES_MIN"$'\n'
    body+="SCANNED_FILES=${#SCANNED[@]}"$'\n'
    body+="BASELINE_SET_COVERED=$covered"$'\n'
    body+="BASELINE_FILE_SET=$(printf '%s' "$baseline_files" | paste -sd, -)"$'\n'
    body+="CJK_ERR_CONCAT=$hits"$'\n'
    if [ "$hits" != "0" ]; then
        body+="HIT_LINES:"$'\n'"$hit_lines"$'\n'
    fi
    [ "$hits" = "0" ] || rc=1
    [ "$covered" = "1" ] || { rc=1; body+="MISSING_BASELINE_FILES=$missing_files"$'\n'; }
    [ "${baseline_file_count:-0}" -ge "$D5_BASELINE_FILES_MIN" ] 2>/dev/null || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R1-scan.txt "$rc" "$body"
    ;;
r2)
    # 逐点断言：基线中每一个拼接点（file|line|receiver）在当前树都必须有引用同一 error
    # 的 log 调用；LOST_DETAIL 不得由 R1 结果推导（#2497 Round 1 Bug 6）。
    mapfile -t SITES < <(baseline_sites)
    log_per_site=0
    lost_detail=0
    site_lines=""
    while IFS= read -r site; do
        [ -n "$site" ] || continue
        file="${site%%|*}"
        rest="${site#*|}"
        bline="${rest%%|*}"
        recv="${rest#*|}"
        root="${recv%%.*}"
        [ -f "$file" ] || { lost_detail=$((lost_detail + 1)); site_lines+="SITE $file:$bline recv=$recv covered=0 reason=missing-file"$'\n'; continue; }
        if grep -qE "log\.(Printf|Println|Print)\(.*\b${root}\b" "$file"; then
            log_per_site=$((log_per_site + 1))
            site_lines+="SITE $file:$bline recv=$recv covered=1"$'\n'
        else
            lost_detail=$((lost_detail + 1))
            site_lines+="SITE $file:$bline recv=$recv covered=0 reason=no-log-of-receiver"$'\n'
        fi
    done <<< "$(printf '%s\n' "${SITES[@]:-}")"
    baseline_site_count="$(printf '%s\n' "${SITES[@]:-}" | sed '/^$/d' | wc -l | tr -d ' ')"
    logs="$(log_present)"
    body="LOG_PRESENT=$logs"$'\n'
    body+="LOG_MIN=22"$'\n'
    body+="BASELINE_REF=$BASELINE_REF"$'\n'
    body+="BASELINE_SITES=$baseline_site_count"$'\n'
    body+="LOG_PER_SITE=$log_per_site"$'\n'
    body+="LOST_DETAIL=$lost_detail"$'\n'
    body+="$site_lines"
    [ "$logs" -ge 22 ] || rc=1
    [ "$lost_detail" = "0" ] || rc=1
    [ "$log_per_site" = "$baseline_site_count" ] || rc=1
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
    # R5（Round 1 Bug 4）：平价脚本 + 含 5xx fixture 的双端回归 + mobile 生产 chokepoint 断言。
    parity_log="$REPO_ROOT/tmp/verify-$ISSUE-r5-parity.log"
    node scripts/check-error-message-parity-2480.mjs > "$parity_log" 2>&1
    parity_rc=$?
    cases="$(sed -n 's/.*CASES=\([0-9]*\).*/\1/p' "$parity_log" | tail -1)"
    diffs="$(sed -n 's/.*DIFFS=\([0-9]*\).*/\1/p' "$parity_log" | tail -1)"
    parity=0
    grep -q 'PARITY=1' "$parity_log" && parity=1

    fivexx_log="$REPO_ROOT/tmp/verify-$ISSUE-r5-5xx.log"
    node --test frontend-pc/tests/errorMessages-5xx-2497.test.mjs > "$fivexx_log" 2>&1
    fivexx_rc=$?
    fivexx_pass="$(grep -cE '^# pass [0-9]+' "$fivexx_log" || true)"
    test_pass="$(sed -n 's/^# pass \([0-9]*\).*/\1/p' "$fivexx_log" | tail -1)"
    test_fail="$(sed -n 's/^# fail \([0-9]*\).*/\1/p' "$fivexx_log" | tail -1)"
    parity_5xx_cases="$(sed -n 's/.*PARITY_CASES=\([0-9]*\).*/\1/p' "$fivexx_log" | tail -1)"
    mobile_prod_path=0
    grep -q 'MOBILE_PROD_PATH=1' "$fivexx_log" && mobile_prod_path=1

    body="PARITY_SCRIPT=scripts/check-error-message-parity-2480.mjs"$'\n'
    body+="PARITY_EXIT=$parity_rc"$'\n'
    body+="PARITY=$parity"$'\n'
    body+="CASES=${cases:-0}"$'\n'
    body+="DIFFS=${diffs:-0}"$'\n'
    body+="PARITY_5XX_TEST=frontend-pc/tests/errorMessages-5xx-2497.test.mjs"$'\n'
    body+="PARITY_5XX_EXIT=$fivexx_rc"$'\n'
    body+="PARITY_5XX_PASS=$test_pass"$'\n'
    body+="PARITY_5XX_FAIL=${test_fail:-missing}"$'\n'
    body+="PARITY_5XX_CASES=${parity_5xx_cases:-0}"$'\n'
    body+="MOBILE_PROD_PATH=$mobile_prod_path"$'\n'
    body+="PARITY_5XX_TAIL:"$'\n'"$(tail -6 "$fivexx_log" 2>/dev/null)"$'\n'
    [ "$parity_rc" = "0" ] && [ "$parity" = "1" ] && [ "${diffs:-1}" = "0" ] || rc=1
    [ "$fivexx_rc" = "0" ] || rc=1
    [ "${test_fail:-1}" = "0" ] || rc=1
    [ "${parity_5xx_cases:-0}" -ge 1 ] 2>/dev/null || rc=1
    [ "$mobile_prod_path" = "1" ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R5-parity.txt "$rc" "$body"
    ;;
r6)
    build_log="$REPO_ROOT/tmp/verify-$ISSUE-r6-build.log"
    ( cd "$REPO_ROOT/backend" && heavy go build ./... ) > "$build_log" 2>&1
    build_exit=$?
    lint_log="$REPO_ROOT/tmp/verify-$ISSUE-r6-eslint.log"
    ( cd "$REPO_ROOT/frontend-pc" && ./node_modules/.bin/eslint "${PC_FILES[@]/frontend-pc\//}" ) > "$lint_log" 2>&1
    lint_exit=$?
    npm_log="$REPO_ROOT/tmp/verify-$ISSUE-r6-npm.log"
    ( cd "$REPO_ROOT/frontend-pc" && heavy npm run build ) > "$npm_log" 2>&1
    npm_exit=$?
    # v2/v3：mobile 生产 chokepoint 必须经解析层（5xx 守卫在移动端可达）。
    mobile_api="frontend-mobile/src/services/api.js"
    mobile_calls=0
    grep -q 'resolveErrorMessage(' "$mobile_api" && mobile_calls=1
    mobile_raw_throw=0
    grep -q "throw new Error(error.message || error.code" "$mobile_api" && mobile_raw_throw=1
    mobile_syntax=1
    cp "$mobile_api" "$REPO_ROOT/tmp/mobile-api-2497.mjs"
    node --check "$REPO_ROOT/tmp/mobile-api-2497.mjs" >/dev/null 2>&1 || mobile_syntax=0
    body="BUILD_CMD=cd backend && go build ./..."$'\n'
    body+="BUILD_EXIT=$build_exit"$'\n'
    body+="ESLINT_EXIT=$lint_exit"$'\n'
    body+="NPM_BUILD_EXIT=$npm_exit"$'\n'
    body+="MOBILE_CHOKEPOINT_CALLS=$mobile_calls"$'\n'
    body+="MOBILE_RAW_THROW=$mobile_raw_throw"$'\n'
    body+="MOBILE_SYNTAX_OK=$mobile_syntax"$'\n'
    [ "$build_exit" = "0" ] && body+="BUILD_OK"$'\n' || rc=1
    [ "$lint_exit" = "0" ] && body+="ESLINT_OK"$'\n' || rc=1
    [ "$npm_exit" = "0" ] && body+="NPM_BUILD_OK"$'\n' || rc=1
    [ "$mobile_calls" = "1" ] && [ "$mobile_raw_throw" = "0" ] && [ "$mobile_syntax" = "1" ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R6-build.txt "$rc" "$body"
    ;;
r7)
    # 全量后端回归（plan R7）：clean tree + -timeout 20m。
    full_log="$REPO_ROOT/tmp/verify-$ISSUE-r7-gotest.log"
    ( cd "$REPO_ROOT/backend" && heavy go test ./... -count=1 -timeout 20m ) > "$full_log" 2>&1
    full_exit=$?
    fails="$(grep -cE '^(--- FAIL|FAIL[[:space:]])' "$full_log" || true)"
    panics="$(grep -c '^panic:' "$full_log" || true)"
    [ -n "$fails" ] || fails=0
    [ -n "$panics" ] || panics=0
    body="FULL_SUITE_CMD=cd backend && go test ./... -count=1 -timeout 20m"$'\n'
    body+="FULL_SUITE_EXIT=$full_exit"$'\n'
    body+="FAIL_COUNT=$fails"$'\n'
    body+="PANIC_COUNT=$panics"$'\n'
    body+="$(tail -8 "$full_log" | sed 's/^/LOG_TAIL: /')"$'\n'
    [ "$full_exit" = "0" ] || rc=1
    [ "$fails" = "0" ] || rc=1
    [ "$panics" = "0" ] || rc=1
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R7-gotest.txt "$rc" "$body"
    ;;
r8)
    # 四源可复现（plan R8）：latest Accepted Plan 评论 ↔ docs/evidence/2497/plan.md。
    plan_out="$(bash "$HOME/.config/opencode/scripts/plan-source.sh" --issue "$ISSUE" 2>&1)"
    plan_rc=$?
    pass_token=0
    printf '%s\n' "$plan_out" | grep -q 'PLAN_SOURCE: PASS' && pass_token=1
    body="PLAN_SOURCE_CMD=scripts/plan-source.sh --issue $ISSUE"$'\n'
    body+="PLAN_SOURCE_EXIT=$plan_rc"$'\n'
    body+="$plan_out"$'\n'
    if [ "$plan_rc" = "0" ] && [ "$pass_token" = "1" ]; then
        body+="PLAN_SOURCE: PASS"$'\n'
    else
        rc=1
    fi
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R8-plan-source.txt "$rc" "$body"
    ;;
r9)
    # v4 R9：mobile H5 生产构建（Taro v4 要求 Node 22；Node 24 会报 ESM 兼容错误）。
    body="" rc=0
    mobile="$REPO_ROOT/frontend-mobile"
    node22_bin="${NODE22_BIN:-}"
    if [ -z "$node22_bin" ]; then
        for d in "$HOME"/.nvm/versions/node/v22*/bin; do
            [ -x "$d/node" ] && node22_bin="$d"
        done
    fi
    node_path="${node22_bin:+$node22_bin:}$PATH"
    if [ ! -d "$mobile/node_modules" ]; then
        body+="MOBILE_NODE_MODULES=missing"$'\n'
        rc=1
    else
        mobile_log="$REPO_ROOT/tmp/verify-$ISSUE-r9-mobile-build.log"
        ( cd "$mobile" && PATH="$node_path" heavy npm run build:h5 ) > "$mobile_log" 2>&1
        mobile_exit=$?
        node_full="$(PATH="$node_path" node -v 2>/dev/null || true)"
        node_major="${node_full#v}"
        node_major="${node_major%%.*}"
        body+="MOBILE_BUILD_CMD=cd frontend-mobile && npm run build:h5（Node 22）"$'\n'
        body+="MOBILE_BUILD_EXIT=$mobile_exit"$'\n'
        body+="NODE_VERSION=${node_major:-unknown}"$'\n'
        body+="NODE_VERSION_FULL=${node_full:-unknown}"$'\n'
        body+="--- mobile build tail ---"$'\n'
        body+="$(tail -8 "$mobile_log")"$'\n'
        [ "$mobile_exit" = "0" ] || rc=1
        [ "$node_major" = "22" ] || rc=1
        [ "$rc" = "0" ] && body+="MOBILE_BUILD_OK=1"$'\n'
    fi
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R9-mobile-build.txt "$rc" "$body"
    ;;
*)
    usage
    ;;
esac
