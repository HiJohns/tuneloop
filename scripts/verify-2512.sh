#!/usr/bin/env bash
# scripts/verify-2512.sh — #2509/#2512 游客下单 org 域回退链证据生成器。
#
# 用法：bash scripts/verify-2512.sh <r1|r2|r3|r4|r5|r6|gate>
#
# 每行把结果写入 docs/evidence/2512/R*-*.txt（首行为 PROVENANCE），并打印机器可
# 判定的断言键值；任一行断言不成立以非 0 退出。
#
# - r1–r4 跑 backend/handlers 的 #2512 定向用例（真实 Postgres 测试库）；
# - r5 静态核对回退链（不得再写 sites.id）+ 只读存量排查 SQL 文本；
# - r6 构建 + CreateOrder/BatchCreateOrder 定向回归；
# - gate 全量 handlers 套件（供 pre-ready --full-gate-cmd 使用）。
#
# 锁纪律（#2512 run-aborted 审计项）：本脚本**不得**自己申请 heavy-lock。
# pre-ready-check.sh 已用 heavy-run.sh 包住 --affected-cmd，gate-run.sh 也已持有
# host lock；脚本内再调 heavy-lock/heavy-run 会等待同一把锁形成自死锁（外层持锁
# 31 分钟，queue 越积越长）。需要锁的场景由调用方在外层包裹：
#   ~/.config/opencode/scripts/heavy-run.sh -- bash scripts/verify-2512.sh r6
# r6 以 NESTED_LOCK_REFS=0 作为防漂移断言（脚本内出现 heavy-lock/heavy-run 即 FAIL）。
set -uo pipefail

ROW="${1:-}"
REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT" || exit 2
ISSUE=2512
OUT_DIR="$REPO_ROOT/docs/evidence/$ISSUE"
HANDLER="backend/handlers/user_rental.go"
GO_BIN="${GO_BIN:-$(command -v go 2>/dev/null || true)}"
[ -n "$GO_BIN" ] || GO_BIN=/usr/local/go/bin/go

mkdir -p "$OUT_DIR" "$REPO_ROOT/tmp"

usage() { echo "usage: $0 <r1|r2|r3|r4|r5|r6|gate>" >&2; exit 2; }
[ -n "$ROW" ] || usage

FIX_SHA="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
[ -n "$FIX_SHA" ] || FIX_SHA="$(git rev-parse HEAD)"
SHORT="$(git rev-parse --short=8 "$FIX_SHA")"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)docs/evidence/$ISSUE" ':(exclude)tmp')" ]; then
    DIRTY=true
fi
PROVENANCE="# build=1.0.$SHORT # commit=$FIX_SHA # dirty=$DIRTY"
# Rev.2：首行格式不变（P14 依赖），另起一行记录「产物生成树」，区分锚点 commit 与 HEAD。
GENERATED_AT_HEAD="$(git rev-parse HEAD)"

finish() { # artifact-file exit-code body
    local name=$1 rc=$2 body=$3
    {
        printf '%s\n' "$PROVENANCE"
        printf 'GENERATED_AT_HEAD=%s\n' "$GENERATED_AT_HEAD"
        printf '%s\n' "$body"
    } > "$OUT_DIR/$name"
    printf '%s\n' "$PROVENANCE"
    printf 'GENERATED_AT_HEAD=%s\n' "$GENERATED_AT_HEAD"
    printf '%s\n' "$body"
    [ "$rc" = "0" ] || return 1
}

# backend 测试环境（本地 worktree 有 backend/.env；gate 干净树继承调用方 env）
if [ -f "$REPO_ROOT/backend/.env" ]; then
    set -a
    # shellcheck disable=SC1091
    . "$REPO_ROOT/backend/.env"
    set +a
fi
export PATH="$(dirname "$GO_BIN"):$PATH"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_${ISSUE}_${ROW}}"

kv() { # log key → value or missing
    if grep -qE "(^|[[:space:]])$2=" "$1" 2>/dev/null; then
        grep -m1 -E "(^|[[:space:]])$2=" "$1" | sed -E "s/.*$2=//" | head -1
        return 0
    fi
    printf 'missing'
}

run_go_row() { # artifact test-regex sentinel key=value...
    local artifact=$1 pattern=$2 sentinel=$3; shift 3
    local log="$REPO_ROOT/tmp/verify-${ISSUE}-${ROW}.log"
    local rc=0 body="" pair key expect actual sentinel_ok=0

    (
        cd "$REPO_ROOT/backend" || exit 2
        "$GO_BIN" test ./handlers -run "$pattern" -count=1 -v
    ) > "$log" 2>&1
    local test_rc=$?
    body+="TEST_CMD=cd backend && go test ./handlers -run '$pattern' -count=1 -v"$'\n'
    body+="TEST_EXIT=$test_rc"$'\n'
    [ "$test_rc" = "0" ] || rc=1

    # 哨兵（R*_OK）由用例在全部断言通过后 t.Logf 输出；缺失即 FAIL。
    grep -qE "(^|[[:space:]])${sentinel}$" "$log" && sentinel_ok=1
    body+="SENTINEL_${sentinel}=${sentinel_ok}"$'\n'
    [ "$sentinel_ok" = "1" ] || rc=1

    while [ $# -gt 0 ]; do
        pair=$1; shift
        key="${pair%%=*}"
        expect="${pair#*=}"
        actual="$(kv "$log" "$key")"
        body+="$key=$actual"$'\n'
        [ "$actual" = "$expect" ] || rc=1
    done
    body+="--- go test tail ---"$'\n'
    body+="$(grep -E '^(--- (PASS|FAIL)|PASS|FAIL|ok )' "$log" | tail -20)"$'\n'
    # 持久化关键键值（含夹具侧 SITE_ORG 与实际落库 ORDER_ORG），使契约断言可直接核对。
    body+="--- go test kv ---"$'\n'
    body+="$(grep -E '(^|[[:space:]])(ORDER_ORG|SITE_ORG|SITE_ID_IS|BATCH_ORG|STATUS|CODE|ORDERS_AFTER|BATCH_ORDERS_AFTER)=' "$log" \
        | sed -E 's/^[[:space:]]+//' | sort -u | head -20)"$'\n'
    if [ -n "${EXTRA_BODY:-}" ]; then
        body+="$EXTRA_BODY"$'\n'
        [ "${EXTRA_RC:-0}" = "0" ] || rc=1
    fi
    [ "$rc" = "0" ] && body+="$sentinel"$'\n'
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish "$artifact" "$rc" "$body"
}

case "$ROW" in
r1)
    # Rev.2 修订 3：红侧不得只靠 RED_CONFIRMED 常量推导 —— 产物必须含旧实现片段与
    # 可判定行数 PRE_FIX_SITE_ID_AS_ORG_LINES（>=1 即通过）。优先取 origin/develop 的
    # 旧实现；若该分支已并入修复（行数归零），回退到本单 fix commit 的父提交（语义等价且稳定）。
    PRE_FIX_REF="origin/develop"
    PRE_FIX_PATTERN='effectiveOrgID = .*(SiteID|CurrentSiteID)\.String\(\)'
    pre_fix_src="$(git show "$PRE_FIX_REF:backend/handlers/user_rental.go" 2>/dev/null || true)"
    pre_fix_lines="$(printf '%s' "$pre_fix_src" | grep -cE "$PRE_FIX_PATTERN" || true)"
    if [ "${pre_fix_lines:-0}" -lt 1 ]; then
        PRE_FIX_REF="$(git rev-parse "${FIX_SHA}^")"
        pre_fix_src="$(git show "$PRE_FIX_REF:backend/handlers/user_rental.go" 2>/dev/null || true)"
        pre_fix_lines="$(printf '%s' "$pre_fix_src" | grep -cE "$PRE_FIX_PATTERN" || true)"
    fi
    pre_fix_snippet="$(printf '%s' "$pre_fix_src" | grep -nE "$PRE_FIX_PATTERN" || true)"
    EXTRA_RC=0
    [ "${pre_fix_lines:-0}" -ge 1 ] || EXTRA_RC=1
    EXTRA_BODY="PRE_FIX_REF=$PRE_FIX_REF"$'\n'"PRE_FIX_SITE_ID_AS_ORG_LINES=$pre_fix_lines"$'\n'"--- pre-fix code ---"$'\n'"$pre_fix_snippet"
    run_go_row R1-guest-org-from-site.txt '^TestCreateOrder2512_GuestOrgFromSite$' R1_OK \
        'OLD_FALLBACK_IS_SITE_ID=1' 'RED_CONFIRMED=1' \
        'ORDER_ORG_MATCHES_SITE_ORG=1'
    ;;
r2)
    run_go_row R2-current-site.txt '^TestCreateOrder2512_GuestOrgFromCurrentSite$' R2_OK \
        'RESOLVED_FROM_CURRENT_SITE=1' \
        'ORDER_ORG_MATCHES_CURRENT_SITE_ORG=1'
    ;;
r3)
    run_go_row R3-reject.txt '^TestCreateOrder2512_UnresolvableRejected$' R3_OK \
        'STATUS=400' 'CODE=40002' 'ORDERS_AFTER=0'
    ;;
r4)
    run_go_row R4-batch.txt '^TestBatchCreateOrder2512_GuestOrgFromSiteAndReject$' R4_OK \
        'BATCH_ORGS_UNIQUE=1' 'BATCH_ORG_MATCHES_SITE_ORG=1' \
        'REJECT_STATUS=400' 'REJECT_CODE=40002' 'BATCH_ORDERS_AFTER=0'
    ;;
r5)
    body="" rc=0
    site_as_org="$(grep -cE 'effectiveOrgID = .*SiteID\.String\(\)' "$HANDLER" 2>/dev/null || true)"
    current_as_org="$(grep -cE 'effectiveOrgID = .*CurrentSiteID\.String\(\)' "$HANDLER" 2>/dev/null || true)"
    resolver_calls="$(grep -c 'resolveOrderOrgID(db, ' "$HANDLER" 2>/dev/null || true)"
    sql_present=0
    sql_set_semantics=0
    read -r -d '' SQL_TEXT <<'SQL'
-- #2512 R5 只读存量排查 SQL（禁止在 CI 中执行；数据修复需 Owner 授权，见 D3）
-- 1) orders.org_id 落在 sites.id 域的存量行（本 bug 的直接产物，细分项）
SELECT count(*) AS polluted_orders
FROM orders o
JOIN sites s ON o.org_id::text = s.id::text
WHERE o.org_id IS NOT NULL AND o.org_id <> s.org_id;
-- 2) AC4 集合语义：orders.org_id 不落在组织集合
--    （sites.org_id ∪ merchants.org_id）内的存量行。
--    既覆盖「站点 ID 域污染」，也覆盖组织已删除 / 导入脏值 / 跨环境拷库等
--    不属于任何 sites.id 的污染值（仅用 1) 会低报）。
SELECT count(*) AS orders_org_outside_org_sets
FROM orders o
WHERE o.org_id IS NOT NULL
  AND o.org_id::text NOT IN (
    SELECT s.org_id::text FROM sites s
    WHERE s.org_id IS NOT NULL AND s.org_id::text <> ''
    UNION
    SELECT m.org_id::text FROM merchants m
    WHERE m.org_id IS NOT NULL AND m.org_id::text <> ''
  );
-- 3) instruments.org_id 为空但 site 可反查的存量乐器（回退链会走 sites.org_id）
SELECT count(*) AS instruments_org_null_site_present
FROM instruments i
JOIN sites s ON s.id = i.site_id
WHERE i.org_id IS NULL AND i.org_id IS DISTINCT FROM s.org_id;
SQL
    printf '%s' "$SQL_TEXT" | grep -q 'polluted_orders' && \
      printf '%s' "$SQL_TEXT" | grep -q 'instruments_org_null_site_present' && sql_present=1
    # 集合语义可判定：必须同时出现「不在集合内」查询的三要素（NOT IN + 两个组织集合）。
    printf '%s' "$SQL_TEXT" | grep -q 'orders_org_outside_org_sets' && \
      printf '%s' "$SQL_TEXT" | grep -q 'NOT IN' && \
      printf '%s' "$SQL_TEXT" | grep -q 'FROM sites' && \
      printf '%s' "$SQL_TEXT" | grep -q 'FROM merchants' && sql_set_semantics=1
    body+="SCAN_FILE=$HANDLER"$'\n'
    body+="SITE_ID_AS_ORG=$site_as_org"$'\n'
    body+="CURRENT_SITE_AS_ORG=$current_as_org"$'\n'
    body+="RESOLVER_CALLS=$resolver_calls"$'\n'
    body+="SQL_PRESENT=$sql_present"$'\n'
    body+="SQL_SET_SEMANTICS=$sql_set_semantics"$'\n'
    body+="--- resolver ---"$'\n'
    body+="$(grep -nE 'func resolveOrderOrgID|resolveOrderOrgID\(db, ' "$HANDLER" || true)"$'\n'
    body+="--- read-only SQL ---"$'\n'"$SQL_TEXT"$'\n'
    [ "$site_as_org" = "0" ] || rc=1
    [ "$current_as_org" = "0" ] || rc=1
    [ "$resolver_calls" = "2" ] || rc=1
    [ "$sql_present" = "1" ] || rc=1
    [ "$sql_set_semantics" = "1" ] || rc=1
    [ "$rc" = "0" ] && body+="R5_OK"$'\n'
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R5-scan-sql.txt "$rc" "$body"
    ;;
r6)
    body="" rc=0
    build_log="$REPO_ROOT/tmp/verify-${ISSUE}-r6-build.log"
    reg_log="$REPO_ROOT/tmp/verify-${ISSUE}-r6-reg.log"
    # 防漂移：脚本内不得出现锁调用（外层 pre-ready/gate 已持锁，内层再申请会自死锁）。
    # bracket 写法避免断言行匹配自身。
    nested_lock_refs="$(grep -vE '^[[:space:]]*#' "$0" | grep -cE 'run_[h]eavy|heavy-[l]ock\.sh|heavy-[r]un\.sh' || true)"
    (
        cd "$REPO_ROOT/backend" || exit 2
        "$GO_BIN" build ./...
    ) > "$build_log" 2>&1
    build_rc=$?
    (
        cd "$REPO_ROOT/backend" || exit 2
        "$GO_BIN" test ./handlers -run 'CreateOrder|BatchCreateOrder' -count=1
    ) > "$reg_log" 2>&1
    reg_rc=$?
    body+="BUILD_CMD=cd backend && go build ./..."$'\n'
    body+="BUILD_RC=$build_rc"$'\n'
    body+="REG_CMD=cd backend && go test ./handlers -run 'CreateOrder|BatchCreateOrder' -count=1"$'\n'
    body+="REG_RC=$reg_rc"$'\n'
    body+="NESTED_LOCK_REFS=$nested_lock_refs"$'\n'
    body+="--- build tail ---"$'\n'"$(tail -5 "$build_log")"$'\n'
    body+="--- regression tail ---"$'\n'"$(tail -5 "$reg_log")"$'\n'
    [ "$build_rc" = "0" ] || rc=1
    [ "$reg_rc" = "0" ] || rc=1
    [ "$nested_lock_refs" = "0" ] || rc=1
    [ "$rc" = "0" ] && body+="R6_OK"$'\n'
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R6-build-regression.txt "$rc" "$body"
    ;;
gate)
    log="$REPO_ROOT/tmp/verify-${ISSUE}-gate.log"
    (
        cd "$REPO_ROOT/backend" || exit 2
        "$GO_BIN" build ./... && "$GO_BIN" test ./handlers -count=1 -timeout 30m
    ) > "$log" 2>&1
    rc=$?
    printf '%s\n' "$PROVENANCE"
    printf 'GENERATED_AT_HEAD=%s\n' "$GENERATED_AT_HEAD"
    printf 'GATE_CMD=cd backend && go build ./... && go test ./handlers -count=1 -timeout 30m\n'
    printf 'FULL_SUITE_EXIT=%d\n' "$rc"
    tail -20 "$log"
    if [ "$rc" = "0" ]; then
        printf 'GATE_OK\n'
    else
        printf 'GATE_FAIL\n'
    fi
    [ "$rc" = "0" ] || exit 1
    ;;
*)
    usage
    ;;
esac
