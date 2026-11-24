#!/usr/bin/env bash
# #2454：PC 选网点后空组织 token —— IAM 授权码绑定组织 + tuneloop 显式拒绝。
# 用法：bash scripts/verify-2454.sh <r1|r2|r3|r4|r5|r6|r7>
set -uo pipefail

MODE="${1:-}"
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT" || exit 2
export PATH="/usr/local/go/bin:${PATH}"
OUT_DIR="${OUT_DIR:-$REPO_ROOT/docs/evidence/2454}"
TMPBASE="${TMPBASE:-/tmp/opencode}"
# /tmp 是 3.8G tmpfs：编译缓存放 HOME（大磁盘），避免 disk quota exceeded。
export TMPDIR="${TMPDIR:-$TMPBASE/2454-gobuild}"
export GOCACHE="${GOCACHE:-$HOME/.cache/go-build}"
mkdir -p "$TMPDIR" "$GOCACHE" "$OUT_DIR"

BEACONIAM_DIR="${BEACONIAM_DIR:-$HOME/beaconiam}"

if [ -f "$REPO_ROOT/backend/.env" ]; then
  set -a; . "$REPO_ROOT/backend/.env"; set +a
fi
# 本单的 handler 用例只使用 fake IAM（httptest），禁止误打真实 IAM 实例。
unset BEACONIAM_INTERNAL_URL BEACONIAM_EXTERNAL_URL IAM_URL
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TUNELOOP_DB_SIMPLE_PROTOCOL=1

FIX_SHA="$(git log --format=%H --grep='^fix(issue-2454)' -1 2>/dev/null || true)"
[ -n "$FIX_SHA" ] || FIX_SHA="$(git rev-parse HEAD)"
HEAD_SHA="$(git rev-parse "$FIX_SHA")"
HEAD_SHORT="$(git rev-parse --short "$FIX_SHA")"
DIRTY=false
# 只统计已被跟踪文件的修改，且排除本单证据目录本身（证据生成会重写这些文件）：
# 否则计划要求的 dirty=false 永远无法满足。
if [ -n "$(git status --porcelain --untracked-files=no -- . ':(exclude)docs/evidence/2454')" ]; then DIRTY=true; fi
PROVENANCE="# build=1.0.$HEAD_SHORT # commit=$HEAD_SHA # dirty=$DIRTY"

BEACONIAM_COMMIT=""
if [ -d "$BEACONIAM_DIR/.git" ]; then
  BEACONIAM_COMMIT="$(git -C "$BEACONIAM_DIR" rev-parse HEAD)"
fi

fail() { echo "FAIL: $*" >&2; exit 1; }

require_beaconiam() {
  [ -n "$BEACONIAM_COMMIT" ] || fail "beaconiam checkout missing (BEACONIAM_DIR=$BEACONIAM_DIR) — INFRA-BLOCKED"
}

run_iam_test() { # artifact-name test-regex
  require_beaconiam
  local art="$OUT_DIR/$1" regex="$2"
  {
    echo "$PROVENANCE"
    echo "BEACONIAM_COMMIT=$BEACONIAM_COMMIT"
    ( cd "$BEACONIAM_DIR" && go test ./internal/auth -run "$regex" -count=1 -v )
  } 2>&1 | tee "$art"
  return "${PIPESTATUS[0]}"
}

run_handler_test() { # artifact-name test-regex
  local art="$OUT_DIR/$1" regex="$2"
  {
    echo "$PROVENANCE"
    ( cd "$REPO_ROOT/backend" && TUNELOOP_TEST_DB="tuneloop_test_2454_${MODE}" go test ./handlers -run "$regex" -count=1 -v )
  } 2>&1 | tee "$art"
  return "${PIPESTATUS[0]}"
}

case "$MODE" in
  r1)
    ART="$OUT_DIR/R1-iam-code-org.txt"
    run_iam_test "R1-iam-code-org.txt" '^TestAuthorizationCodeGrant_BoundOrgContext_500$'
    rc=$?
    grep -q -- '--- PASS' "$ART" || fail "r1 missing PASS"
    grep -qF 'OID=76d46658-0000-4000-8000-000000000001' "$ART" || fail "r1 oid not bound to selected org"
    grep -qF 'TID=bfce6c6d-0000-4000-8000-000000000001' "$ART" || fail "r1 tenant id wrong"
    grep -qF 'ROLE=STAFF' "$ART" || fail "r1 role must be STAFF"
    grep -qF 'FN_ROLES_NONEMPTY=1' "$ART" || fail "r1 fn_roles missing"
    grep -qF 'REFRESH_CTX=ok' "$ART" || fail "r1 refresh token missing"
    exit "$rc"
    ;;
  r2)
    ART="$OUT_DIR/R2-iam-legacy.txt"
    run_iam_test "R2-iam-legacy.txt" '^TestAuthorizationCodeGrant_LegacyCodeMultiOrg_OrgNotSelected_500$'
    rc=$?
    grep -q -- '--- PASS' "$ART" || fail "r2 missing PASS"
    grep -qF 'HTTP=400' "$ART" || fail "r2 legacy multi-org code must be rejected with 400"
    grep -qF 'ERROR=org_not_selected' "$ART" || fail "r2 missing org_not_selected"
    grep -qF 'ACCESS_TOKEN_EMPTY=1' "$ART" || fail "r2 must not issue empty-context token"
    grep -qF 'HTTP=200' "$ART" && fail "r2 negative: legacy code must not succeed"
    exit "$rc"
    ;;
  r3)
    ART="$OUT_DIR/R3-iam-select-org.txt"
    run_iam_test "R3-iam-select-org.txt" '^(TestSelectOrg_BindsCodeAndIsExchangeable_500|TestSelectOrg_RejectsForeignCode_500)$'
    rc=$?
    grep -q -- '--- PASS' "$ART" || fail "r3 missing PASS"
    grep -qF 'RESP_CODE_PRESENT=1' "$ART" || fail "r3 select-org must return code"
    grep -qF 'CODE_EXCHANGE_OID=76d46658-0000-4000-8000-000000000001' "$ART" || fail "r3 bound code exchange wrong org"
    grep -qF 'REUSE_HTTP=400' "$ART" || fail "r3 used code must not be reused"
    exit "$rc"
    ;;
  r4)
    ART="$OUT_DIR/R4-pc-callback-guard.txt"
    run_handler_test "R4-pc-callback-guard.txt" '^Test(PCCallbackRejectsEmptyOrgContext_2454|H5CallbackKeepsEmptyContextToken_2454)$'
    rc=$?
    grep -q -- '--- PASS' "$ART" || fail "r4 missing PASS"
    grep -qF 'PC_HTTP=502' "$ART" || fail "r4 PC branch must reject with 502"
    grep -qF 'CODE=50202' "$ART" || fail "r4 must use code 50202"
    grep -qF 'COOKIE_SET=0' "$ART" || fail "r4 must not set auth cookie"
    grep -qF 'H5_HTTP=200' "$ART" || fail "r4 H5 branch must stay 200"
    grep -qF 'PC_HTTP=200' "$ART" && fail "r4 negative: PC branch must not accept empty context"
    exit "$rc"
    ;;
  r5)
    ART="$OUT_DIR/R5-pc-callback-ui.txt"
    FILE="$REPO_ROOT/frontend-pc/src/pages/AuthCallback/index.tsx"
    {
      echo "$PROVENANCE"
      fail_br="0"
      if grep -qF "data.code !== 20000" "$FILE"; then fail_br="1"; fi
      msg="0"
      if grep -qF "message.error(data.message" "$FILE"; then msg="1"; fi
      echo "FAILURE_BRANCH_NO_TOKEN=$fail_br"
      echo "SHOWS_SERVER_MESSAGE=$msg"
      echo "EVIDENCE_TYPE=static"
    } 2>&1 | tee "$ART"
    grep -qx 'FAILURE_BRANCH_NO_TOKEN=1' "$ART" || fail "r5 failure branch guard missing"
    grep -qx 'SHOWS_SERVER_MESSAGE=1' "$ART" || fail "r5 must surface backend message"
    exit 0
    ;;
  r6)
    ART="$OUT_DIR/R6-auth-error-copy.txt"
    API="$REPO_ROOT/frontend-pc/src/services/api.js"
    LOGOUT="$REPO_ROOT/frontend-pc/src/pages/LogoutPage.jsx"
    {
      echo "$PROVENANCE"
      reason="0"
      if grep -qF "body.code === 40104" "$API" && grep -qF "localStorage.setItem('logout_reason', 'org_context_missing')" "$API"; then
        reason="1"
      fi
      copy="0"
      if grep -qF "'org_context_missing': '登录上下文缺失，请重新登录并选择组织'" "$LOGOUT"; then
        copy="1"
      fi
      echo "ORG_CTX_REASON=$reason"
      echo "LOGOUT_COPY=$copy"
      echo "EVIDENCE_TYPE=static"
    } 2>&1 | tee "$ART"
    grep -qx 'ORG_CTX_REASON=1' "$ART" || fail "r6 40104 → org_context_missing missing"
    grep -qx 'LOGOUT_COPY=1' "$ART" || fail "r6 logout copy missing"
    exit 0
    ;;
  r7)
    ART="$OUT_DIR/R7-build.txt"
    require_beaconiam
    {
      echo "$PROVENANCE"
      echo "BEACONIAM_COMMIT=$BEACONIAM_COMMIT"
      ( cd "$REPO_ROOT/backend" && go build ./... )
      echo "TUNELOOP_BUILD=$?"
      ( cd "$BEACONIAM_DIR" && go build ./... )
      echo "BEACONIAM_BUILD=$?"
      ( cd "$REPO_ROOT/frontend-pc" && npm run build >/dev/null 2>&1 )
      echo "PC_BUILD=$?"
    } 2>&1 | tee "$ART"
    grep -qE '^TUNELOOP_BUILD=0$' "$ART" || fail "r7 tuneloop build failed"
    grep -qE '^BEACONIAM_BUILD=0$' "$ART" || fail "r7 beaconiam build failed"
    grep -qE '^PC_BUILD=0$' "$ART" || fail "r7 PC build failed"
    exit 0
    ;;
  *)
    echo "usage: $0 <r1|r2|r3|r4|r5|r6|r7>" >&2
    exit 2
    ;;
esac
