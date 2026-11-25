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
set -uo pipefail

ROW="${1:-}"
REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT" || exit 2
ISSUE=2512
OUT_DIR="$REPO_ROOT/docs/evidence/$ISSUE"
HANDLER="backend/handlers/user_rental.go"
HEAVY_RUN="${OPENCODE_HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"
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
    printf '%s\n' "$PROVENANCE"
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
        run_heavy "$GO_BIN" test ./handlers -run "$pattern" -count=1 -v
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
    [ "$rc" = "0" ] && body+="$sentinel"$'\n'
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish "$artifact" "$rc" "$body"
}

case "$ROW" in
r1)
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
    read -r -d '' SQL_TEXT <<'SQL'
-- #2512 R5 只读存量排查 SQL（禁止在 CI 中执行；数据修复需 Owner 授权，见 D3）
-- 1) orders.org_id 落在 sites.id 域的存量行（域污染）
SELECT count(*) AS polluted_orders
FROM orders o
JOIN sites s ON o.org_id::text = s.id::text
WHERE o.org_id IS NOT NULL AND o.org_id <> s.org_id;
-- 2) instruments.org_id 为空但 site 可反查的存量乐器（回退链会走 sites.org_id）
SELECT count(*) AS instruments_org_null_site_present
FROM instruments i
JOIN sites s ON s.id = i.site_id
WHERE i.org_id IS NULL AND i.org_id IS DISTINCT FROM s.org_id;
SQL
    printf '%s' "$SQL_TEXT" | grep -q 'polluted_orders' && \
      printf '%s' "$SQL_TEXT" | grep -q 'instruments_org_null_site_present' && sql_present=1
    body+="SCAN_FILE=$HANDLER"$'\n'
    body+="SITE_ID_AS_ORG=$site_as_org"$'\n'
    body+="CURRENT_SITE_AS_ORG=$current_as_org"$'\n'
    body+="RESOLVER_CALLS=$resolver_calls"$'\n'
    body+="SQL_PRESENT=$sql_present"$'\n'
    body+="--- resolver ---"$'\n'
    body+="$(grep -nE 'func resolveOrderOrgID|resolveOrderOrgID\(db, ' "$HANDLER" || true)"$'\n'
    body+="--- read-only SQL ---"$'\n'"$SQL_TEXT"$'\n'
    [ "$site_as_org" = "0" ] || rc=1
    [ "$current_as_org" = "0" ] || rc=1
    [ "$resolver_calls" = "2" ] || rc=1
    [ "$sql_present" = "1" ] || rc=1
    [ "$rc" = "0" ] && body+="R5_OK"$'\n'
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R5-scan-sql.txt "$rc" "$body"
    ;;
r6)
    body="" rc=0
    build_log="$REPO_ROOT/tmp/verify-${ISSUE}-r6-build.log"
    reg_log="$REPO_ROOT/tmp/verify-${ISSUE}-r6-reg.log"
    (
        cd "$REPO_ROOT/backend" || exit 2
        run_heavy "$GO_BIN" build ./...
    ) > "$build_log" 2>&1
    build_rc=$?
    (
        cd "$REPO_ROOT/backend" || exit 2
        run_heavy "$GO_BIN" test ./handlers -run 'CreateOrder|BatchCreateOrder' -count=1
    ) > "$reg_log" 2>&1
    reg_rc=$?
    body+="BUILD_CMD=cd backend && go build ./..."$'\n'
    body+="BUILD_RC=$build_rc"$'\n'
    body+="REG_CMD=cd backend && go test ./handlers -run 'CreateOrder|BatchCreateOrder' -count=1"$'\n'
    body+="REG_RC=$reg_rc"$'\n'
    body+="--- build tail ---"$'\n'"$(tail -5 "$build_log")"$'\n'
    body+="--- regression tail ---"$'\n'"$(tail -5 "$reg_log")"$'\n'
    [ "$build_rc" = "0" ] || rc=1
    [ "$reg_rc" = "0" ] || rc=1
    [ "$rc" = "0" ] && body+="R6_OK"$'\n'
    body+="RESULT: $([ "$rc" = "0" ] && echo PASS || echo FAIL)"$'\n'
    finish R6-build-regression.txt "$rc" "$body"
    ;;
gate)
    log="$REPO_ROOT/tmp/verify-${ISSUE}-gate.log"
    (
        cd "$REPO_ROOT/backend" || exit 2
        run_heavy "$GO_BIN" build ./... && run_heavy "$GO_BIN" test ./handlers -count=1 -timeout 30m
    ) > "$log" 2>&1
    rc=$?
    printf '%s\n' "$PROVENANCE"
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
