#!/usr/bin/env bash
# scripts/verify-2510.sh — #2510「丢失/找回归属校验 ID 域纠偏」证据生成器（R1–R7）。
#
# 用法：bash scripts/verify-2510.sh <r1|r2|r3|r4|r5|r6|r7>
#
# 判定口径（Accepted Plan）：
#   R1 ID 域根因（旧字符串谓词不成立 / 新集合成员判定成立 / 红例确认）
#   R2 网点账号（oid = sites.org_id）可登记丢失
#   R3 网点账号可找回
#   R4 跨组织仍被拒（不落库、不改状态）
#   R5 旧口径（oid = sites.id）不再放行
#   R6 既有 TestInstrumentLoss* 全量回归
#   R7 构建 + 同类扫描（无 oid↔site_id 字符串比较、List 未消费 oid）
set -uo pipefail

ROW="${1:-}"
REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2510
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR" "$REPO/tmp"

export PATH="/usr/local/go/bin:$PATH"
# 真实 Postgres 测试库（skip 视为 FAIL：不设置 TUNELOOP_TEST_ALLOW_SKIP）
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_local}"
unset TUNELOOP_TEST_ALLOW_SKIP
export TMPDIR="${TMPDIR:-$REPO/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"

HEAD_SHA="$(git rev-parse HEAD)"
FIX_REF="${FIX_REF:-$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)}"
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short=8 "$FIX_SHA")"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)docs/evidence/$ISSUE" ':(exclude)tmp')" ]; then
    DIRTY=true
fi
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
TREE_LINE="# tree=$HEAD_SHA"

BODY="$(mktemp "$REPO/tmp/verify-$ISSUE-$ROW.XXXXXX")" || exit 2
trap 'rm -f "$BODY"' EXIT
RC=0
emit() { printf '%s\n' "$*" | tee -a "$BODY"; }
require() { grep -Fq -- "$1" "$BODY" || { echo "ASSERT_FAIL: missing [$1]" >&2; RC=1; }; }
finish() {
    local name="$1"
    {
        printf '%s\n%s\n' "$PROV" "$TREE_LINE"
        cat "$BODY"
    } > "$OUT_DIR/$name"
    cat "$OUT_DIR/$name"
    [ "$RC" = "0" ] || return 1
}

# 抽取 go test -v 的 t.Logf 断言键（去掉 "    file.go:NN: " 前缀）。
emit_keys() { # logfile keys-regex
    grep -E "$2" "$1" | sed 's/^[[:space:]]*[^:]*: //' | while IFS= read -r l; do emit "$l"; done
}

run_test() { # run-regex logfile
    ( cd "$REPO/backend" && go test ./handlers -run "$1" -v -count=1 -timeout 15m ) > "$2" 2>&1
}

usage() { echo "usage: $0 <r1|r2|r3|r4|r5|r6|r7>" >&2; exit 2; }

case "$ROW" in
r1)
    LOG="$REPO/tmp/verify-$ISSUE-r1.log"
    run_test 'TestInstrumentLoss2510_IDDomainRootCause' "$LOG"; TRC=$?
    emit_keys "$LOG" 'SITE_ID=|SITE_ORG=|OLD_PREDICATE_MATCH=|NEW_RULE_MATCH=|RED_CONFIRMED='
    emit "GO_TEST_EXIT=$TRC"
    [ "$TRC" = "0" ] || RC=1
    require 'OLD_PREDICATE_MATCH=0'
    require 'NEW_RULE_MATCH=1'
    require 'RED_CONFIRMED=1'
    finish R1-id-domain.txt || RC=1
    ;;
r2)
    LOG="$REPO/tmp/verify-$ISSUE-r2.log"
    run_test 'TestInstrumentLoss2510_RegisterBySiteOrg' "$LOG"; TRC=$?
    emit_keys "$LOG" 'STATUS=|CODE=|LOSS_RECORD_CREATED=|INSTRUMENT_LOST='
    emit "GO_TEST_EXIT=$TRC"
    [ "$TRC" = "0" ] || RC=1
    require 'STATUS=200'
    require 'CODE=20000'
    require 'LOSS_RECORD_CREATED=1'
    require 'INSTRUMENT_LOST=1'
    finish R2-register.txt || RC=1
    ;;
r3)
    LOG="$REPO/tmp/verify-$ISSUE-r3.log"
    run_test 'TestInstrumentLoss2510_RestoreBySiteOrg' "$LOG"; TRC=$?
    emit_keys "$LOG" 'STATUS=|RESTORED_DAMAGED=|INSTRUMENT_AVAILABLE='
    emit "GO_TEST_EXIT=$TRC"
    [ "$TRC" = "0" ] || RC=1
    require 'STATUS=200'
    require 'RESTORED_DAMAGED=0'
    require 'INSTRUMENT_AVAILABLE=1'
    finish R3-restore.txt || RC=1
    ;;
r4)
    LOG="$REPO/tmp/verify-$ISSUE-r4.log"
    run_test 'TestInstrumentLoss2510_CrossOrgDenied' "$LOG"; TRC=$?
    emit_keys "$LOG" 'LOST_STATUS=|RESTORE_STATUS=|LEAK_RECORDS=|STOCK_STATUS_UNCHANGED='
    emit "GO_TEST_EXIT=$TRC"
    [ "$TRC" = "0" ] || RC=1
    require 'LOST_STATUS=403'
    require 'RESTORE_STATUS=403'
    require 'LEAK_RECORDS=0'
    require 'STOCK_STATUS_UNCHANGED=1'
    finish R4-cross-org.txt || RC=1
    ;;
r5)
    LOG="$REPO/tmp/verify-$ISSUE-r5.log"
    run_test 'TestInstrumentLoss2510_OldOidConventionDenied' "$LOG"; TRC=$?
    emit_keys "$LOG" 'OLD_OID_LOST_STATUS=|OLD_OID_RESTORE_STATUS=|OLD_OID_ALLOWED='
    emit "GO_TEST_EXIT=$TRC"
    [ "$TRC" = "0" ] || RC=1
    require 'OLD_OID_LOST_STATUS=403'
    require 'OLD_OID_RESTORE_STATUS=403'
    require 'OLD_OID_ALLOWED=0'
    finish R5-old-behavior-gone.txt || RC=1
    ;;
r6)
    LOG="$REPO/tmp/verify-$ISSUE-r6.log"
    run_test 'TestInstrumentLoss' "$LOG"; TRC=$?
    FAIL_COUNT="$(grep -c -- '--- FAIL' "$LOG" || true)"
    PASS_COUNT="$(grep -c -- '--- PASS' "$LOG" || true)"
    SKIP_COUNT="$(grep -c -- '--- SKIP' "$LOG" || true)"
    PASS_COUNT=${PASS_COUNT:-0}; FAIL_COUNT=${FAIL_COUNT:-0}; SKIP_COUNT=${SKIP_COUNT:-0}
    emit "TESTS_RUN=$((PASS_COUNT + FAIL_COUNT + SKIP_COUNT))"
    emit "PASS_COUNT=$PASS_COUNT"
    emit "FAIL_COUNT=$FAIL_COUNT"
    emit "SKIP_COUNT=$SKIP_COUNT"
    emit "GO_TEST_EXIT=$TRC"
    REGRESSION_PASS=0
    if [ "$TRC" = "0" ] && [ "$FAIL_COUNT" = "0" ] && [ "$SKIP_COUNT" = "0" ] && [ "$PASS_COUNT" -ge 6 ]; then
        REGRESSION_PASS=1
    fi
    emit "REGRESSION_PASS=$REGRESSION_PASS"
    [ "$TRC" = "0" ] || RC=1
    [ "$REGRESSION_PASS" = "1" ] || RC=1
    finish R6-regression.txt || RC=1
    ;;
r7)
    BUILD_LOG="$REPO/tmp/verify-$ISSUE-r7-build.log"
    ( cd "$REPO/backend" && go build ./... ) > "$BUILD_LOG" 2>&1
    BUILD_RC=$?
    STRING_CMP="$(grep -cE 'CurrentSiteID\.String\(\)[[:space:]]*!=[[:space:]]*oid|CurrentSiteID\.String\(\)[[:space:]]*==[[:space:]]*oid' backend/handlers/instrument_loss.go || true)"
    STRING_CMP=${STRING_CMP:-0}
    LIST_SCOPE_OID=0
    LIST_BODY="$(awk '/func \(h \*InstrumentLossHandler\) List\(/{f=1} f{print} f&&/^}/{exit}' backend/handlers/instrument_loss.go)"
    if printf '%s\n' "$LIST_BODY" | grep -q 'GetOrgID'; then
        LIST_SCOPE_OID=1
    fi
    CURRENT_SITE_HITS="$(grep -c 'CurrentSiteID' backend/handlers/instrument_loss.go || true)"
    emit "BUILD_CMD=cd backend && go build ./..."
    emit "BUILD_RC=$BUILD_RC"
    emit "OID_SITE_STRING_COMPARE=$STRING_CMP"
    emit "LIST_SCOPE_OID=$LIST_SCOPE_OID"
    emit "CURRENT_SITE_HITS=${CURRENT_SITE_HITS:-0}"
    [ "$BUILD_RC" = "0" ] || RC=1
    [ "$STRING_CMP" = "0" ] || RC=1
    [ "$LIST_SCOPE_OID" = "0" ] || RC=1
    finish R7-build-scan.txt || RC=1
    ;;
*)
    usage
    ;;
esac

exit "$RC"
