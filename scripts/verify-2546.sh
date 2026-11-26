#!/usr/bin/env bash
# verify-2546.sh — Evidence Contract runner for #2546
#   (商户所有人（唯一 OWNER）+ 转让所有权；原 OWNER 降 ADMIN)
#
# Usage:
#   bash scripts/verify-2546.sh r1 … r11
#
# Each row prints a provenance header, runs the row's commands, writes the
# artifact under docs/evidence/2546/ and fails (exit 1) when any expected
# assertion token is missing. Heavy commands (go build/test, npm build) go
# through scripts/heavy-run.sh when available so the host lock is honoured per
# command (never around the whole row).
#
# Requires: Go toolchain, psql reachable test DB (POSTGRES_*/TUNELOOP_DB, or
# backend/.env), node + frontend-pc dependencies for R8.
set -uo pipefail

ROW="${1:-}"
ISSUE=2546
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/docs/evidence/$ISSUE"
HEAVY_RUN="${HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"
LOCK_DIR="${HEAVY_LOCK_DIR:-/tmp/opencode/locks}"

mkdir -p "$OUT" "$ROOT/tmp"
cd "$ROOT" || exit 2

if [ -f "$ROOT/backend/.env" ]; then
  set -a
  # shellcheck disable=SC1091
  . "$ROOT/backend/.env"
  set +a
fi
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-5432}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export TUNELOOP_DB="${TUNELOOP_DB:-tuneloop_debug}"

# lib/pq mis-parses `password= dbname=…` when the value is empty (it silently
# connects to a DB named after the user), so fail loudly instead of producing
# misleading evidence.
if [ -z "${POSTGRES_PASSWORD:-}" ]; then
  echo "ERROR: POSTGRES_PASSWORD is empty (backend/.env missing?). Tests need a real test-DB password." >&2
  exit 2
fi

COMMIT="$(git rev-parse HEAD)"
SHORT="$(git rev-parse --short HEAD)"
BUILD="1.0.$SHORT"
# Evidence artifacts are regenerated after the fix commit; their own churn must not
# mark the tree dirty (they are committed separately as docs(evidence): issue-2546).
DIRTY="false"
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/' || true)" ]; then
  DIRTY="true"
fi

PROVENANCE="# build=$BUILD # commit=$COMMIT # dirty=$DIRTY"

FAILURES=0

heavy() {
  if [ -x "$HEAVY_RUN" ]; then
    bash "$HEAVY_RUN" --dir "$LOCK_DIR" -- "$@"
  else
    "$@"
  fi
}

assert_grep() { # artifact token...
  local artifact="$1"; shift
  local missing=0
  for token in "$@"; do
    if ! grep -qF -- "$token" "$artifact"; then
      echo "MISSING: $token (in ${artifact#$ROOT/})" >&2
      missing=1
      FAILURES=$((FAILURES + 1))
    fi
  done
  if [ "$missing" = "0" ]; then
    echo "ASSERT_OK: ${artifact#$ROOT/} ($# tokens)"
  fi
}

go_test_row() { # artifact run-regex package...
  local artifact="$1"; shift
  local regex="$1"; shift
  local pkgs=("$@")
  {
    echo "$PROVENANCE"
    echo "# CMD go test -count=1 -run '$regex' ${pkgs[*]}"
    ( cd "$ROOT/backend" && heavy go test "${pkgs[@]}" -run "$regex" -count=1 -v )
  } >>"$artifact" 2>&1
  local rc=$?
  echo "GO_TEST_RC=$rc" >>"$artifact"
  if [ "$rc" != "0" ]; then
    echo "GO_TEST_FAIL: $regex (${artifact#$ROOT/})" >&2
    FAILURES=$((FAILURES + 1))
  fi
}

row_r1() {
  local a="$OUT/R1-transfer-success.txt"
  {
    echo "$PROVENANCE"
    echo "# R1 转让成功：IAM 契约（method/path/body）+ 本地指针与业务角色同步"
  } >"$a"
  go_test_row "$a" 'TestTransferOwnershipSuccess_2546' ./handlers/
  go_test_row "$a" 'TestTransferOwnershipClientContract_2546' ./services/
  assert_grep "$a" \
    'TRANSFER_STATUS=200' 'IAM_METHOD=POST' 'IAM_PATH_SUFFIX=/transfer-ownership' \
    'IAM_BODY_USER_ID=1' 'LOCAL_OWNER_UPDATED=1' 'OLD_OWNER_LOCAL_ROLE=merchant_admin' \
    'TARGET_LOCAL_ROLE=merchant_admin' 'ADMIN_PENDING_CLEARED=1' 'OWNER_FIELD_PRESENT=1'
}

row_r2() {
  local a="$OUT/R2-iam-errors.txt"
  {
    echo "$PROVENANCE"
    echo "# R2 IAM 失败映射 403/404/409/400/5xx + 本地指针不变（不得吞成 200）"
  } >"$a"
  go_test_row "$a" 'TestTransferOwnershipIAMErrorMapping_2546' ./handlers/
  go_test_row "$a" 'TestTransferOwnershipClientErrorMapping_2546' ./services/
  assert_grep "$a" \
    'IAM403_STATUS=403' 'IAM403_CODE=40303' 'IAM404_STATUS=404' 'IAM404_CODE=40400' \
    'IAM409_STATUS=409' 'IAM409_CODE=40900' 'IAM400_CODE=40002' 'IAM500_STATUS=502' \
    'IAM500_CODE=50200' 'LOCAL_OWNER_UNCHANGED=1'
}

row_r3() {
  local a="$OUT/R3-preflight-negatives.txt"
  {
    echo "$PROVENANCE"
    echo "# R3 前置拒绝（非成员 / 目标已是 owner / user_id 空）均不调 IAM"
  } >"$a"
  go_test_row "$a" 'TestTransferOwnershipPreflightNegatives_2546' ./handlers/
  assert_grep "$a" \
    'NONMEMBER_STATUS=404' 'NONMEMBER_CODE=40400' 'NONMEMBER_IAM_CALLS=0' \
    'SAME_OWNER_STATUS=400' 'SAME_OWNER_CODE=40002' 'SAME_OWNER_IAM_CALLS=0' \
    'EMPTY_UID_STATUS=400' 'EMPTY_UID_CODE=40001'
}

row_r4() {
  local a="$OUT/R4-admin-bind.txt"
  {
    echo "$PROVENANCE"
    echo "# R4 添加/改角色只 bind ADMIN；显式 OWNER 请求 400/40002 且不写 IAM"
  } >"$a"
  go_test_row "$a" 'TestMerchantMemberRoleBindsAdmin_2546' ./handlers/
  assert_grep "$a" \
    'ADD_BIND_ROLE=ADMIN' 'UPDATE_BIND_ROLE=ADMIN' 'OWNER_BINDS=0' \
    'OWNER_REQUEST_STATUS=400' 'OWNER_REQUEST_CODE=40002'
}

row_r5() {
  local a="$OUT/R5-owner-protection.txt"
  {
    echo "$PROVENANCE"
    echo "# R5 OWNER 移除/降级保护（不调 IAM）+ 非 owner 管理员移除回归"
  } >"$a"
  go_test_row "$a" 'TestMerchantOwnerProtection_2546' ./handlers/
  assert_grep "$a" \
    'REMOVE_OWNER_STATUS=403' 'REMOVE_OWNER_CODE=40303' 'REMOVE_OWNER_IAM_CALLS=0' \
    'OWNER_ROW_KEPT=1' 'DEMOTE_OWNER_STATUS=400' 'DEMOTE_OWNER_CODE=40002' 'REMOVE_ADMIN_STATUS=200'
}

row_r6() {
  local a="$OUT/R6-update-merchant-transfer.txt"
  {
    echo "$PROVENANCE"
    echo "# R6 PUT /merchants/:id admin_uid 变化走转让端点（翻转 #2461 bind+demote 断言）；置空 400/40002"
  } >"$a"
  go_test_row "$a" 'TestUpdateMerchantOwnershipTransfer_2546' ./handlers/
  go_test_row "$a" 'TestUpdateMerchantAdmin' ./handlers/
  assert_grep "$a" \
    'UPD_ADMIN_STATUS=200' 'UPD_ADMIN_IAM_PATH_SUFFIX=/transfer-ownership' \
    'UPD_ADMIN_LOCAL=1' 'UPD_ADMIN_IS_OWNER_FLIP=1' 'CLEAR_OWNER_STATUS=400' 'CLEAR_OWNER_CODE=40002' \
    'IAM_TRANSFER_CALLS=1' 'IAM_BIND_CALLS=0' 'IAM_ROLE_CALLS=0' 'ADMIN_UID_SWITCHED=1'
}

row_r7() {
  local a="$OUT/R7-list-owner-flag.txt"
  {
    echo "$PROVENANCE"
    echo "# R7 列表 is_owner/status/owner_uid + include=all（默认口径不变）"
  } >"$a"
  go_test_row "$a" 'TestListMembersOwnerFlag_2546' ./handlers/
  assert_grep "$a" \
    'OWNER_FLAG_HITS=1' 'OWNER_UID_MATCH=1' 'DEFAULT_ROWS=2' 'ALL_ROWS=3' 'OWNER_FIELD_PRESENT=1'
}

row_r8() {
  local a="$OUT/R8-pc-owner-tab.txt"
  local component="$ROOT/frontend-pc/src/components/MerchantMemberManagement.jsx"
  local page="$ROOT/frontend-pc/src/pages/MerchantManagement.jsx"
  local util="$ROOT/frontend-pc/src/utils/merchantOwnership.js"
  {
    echo "$PROVENANCE"
    echo "# R8 PC「所有人」Tab 接线：纯函数 node --test + 源码结构断言 + eslint + build"
  } >"$a"

  local node_rc lint_rc build_rc
  ( cd "$ROOT/frontend-pc" && node --test src/utils/merchantOwnership.test.js ) >>"$a" 2>&1
  node_rc=$?

  local tab_renamed=0 transfer_ui=0 owner_remove=0 owner_card=0
  grep -qF 'Tabs.TabPane tab="所有人"' "$page" && tab_renamed=1
  grep -qF '负责人管理' "$page" && tab_renamed=0
  grep -qF 'transfer-ownership' "$component" && grep -qF '转让所有权' "$component" \
    && grep -qF 'transferConfirmText' "$component" && transfer_ui=1
  grep -qF 'canRemoveMember(record, ownerUid)' "$component" && owner_remove=1
  grep -qF 'pickOwnerRow(members)' "$component" && grep -qF '未锚定所有人' "$component" \
    && grep -qF '转让后你将成为管理员' "$component" && owner_card=1

  ( cd "$ROOT/frontend-pc" && npx eslint src/components/MerchantMemberManagement.jsx src/pages/MerchantManagement.jsx src/utils/merchantOwnership.js ) >>"$a" 2>&1
  lint_rc=$?

  ( cd "$ROOT/frontend-pc" && heavy npm run build ) >>"$a" 2>&1
  build_rc=$?

  {
    echo "NODE_TEST_RC=$node_rc"
    echo "TAB_RENAMED=$tab_renamed"
    echo "TRANSFER_UI_WIRED=$transfer_ui"
    echo "OWNER_REMOVE_DISABLED=$owner_remove"
    echo "OWNER_CARD_RENDERED=$owner_card"
    echo "FRONTEND_LINT_RC=$lint_rc"
    echo "FRONTEND_BUILD_RC=$build_rc"
  } >>"$a"

  assert_grep "$a" 'NODE_TEST_RC=0' 'TAB_RENAMED=1' 'TRANSFER_UI_WIRED=1' \
    'OWNER_REMOVE_DISABLED=1' 'OWNER_CARD_RENDERED=1' 'FRONTEND_LINT_RC=0' 'FRONTEND_BUILD_RC=0'
  # 纯函数消费方必须存在，否则后端端点/字段没有前端入口（死代码）。
  [ -f "$util" ] || { echo "MISSING: $util" >&2; FAILURES=$((FAILURES + 1)); }
}

row_r9() {
  local a="$OUT/R9-audit-route.txt"
  {
    echo "$PROVENANCE"
    echo "# R9 审计路由接线（auditRouteMap 表项驱动，handler 不手写审计）"
  } >"$a"
  go_test_row "$a" 'TestAuditRouteTransferOwnership_2546' ./middleware/
  assert_grep "$a" \
    'AUDIT_ROUTE_FOUND=1' 'AUDIT_ACTION=TRANSFER_OWNERSHIP' 'AUDIT_RESOURCE=merchant' 'AUDIT_PRIORITY=CRITICAL'
}

row_r10() {
  local a="$OUT/R10-regression.txt"
  local build_rc regression_rc
  {
    echo "$PROVENANCE"
    echo "# R10 go build ./... + 成员/商户用例定向回归"
  } >"$a"

  ( cd "$ROOT/backend" && heavy go build ./... ) >>"$a" 2>&1
  build_rc=$?
  echo "BUILD_RC=$build_rc" >>"$a"

  local regression_regex='TestAddMember|TestRemoveMember|TestRemoveLastMerchantAdmin|TestUpdateMemberRole|TestUpdateMerchant|TestMerchantMember|TestTransferOwnership|TestListMembersOwnerFlag|TestAuditRouteTransferOwnership_2546'
  ( cd "$ROOT/backend" && heavy go test ./handlers/ ./services/ ./middleware/ -run "$regression_regex" -count=1 ) >>"$a" 2>&1
  regression_rc=$?
  {
    echo "REGRESSION_RC=$regression_rc"
    if [ "$regression_rc" = "0" ] && [ "$build_rc" = "0" ]; then
      echo "REGRESSION_PASS=1"
    else
      echo "REGRESSION_PASS=0"
    fi
  } >>"$a"

  assert_grep "$a" 'BUILD_RC=0' 'REGRESSION_RC=0' 'REGRESSION_PASS=1'
}

row_r11() {
  local a="$OUT/R11-docs.txt"
  local api_doc="$ROOT/docs/spec/api/api.md"
  local ui_doc="$ROOT/docs/spec/ui/ui.md"
  local db_doc="$ROOT/docs/spec/database/database.md"
  local case_doc="$ROOT/docs/cases/organization.md"

  local api_transfer=0 api_members=0 ui_owner=0 db_owner=0 case_path=0
  grep -qF 'POST /api/merchants/:id/transfer-ownership' "$api_doc" \
    && grep -qF 'TRANSFER_OWNERSHIP' "$api_doc" && api_transfer=1
  grep -qF 'GET /api/admin/merchants/:id/members' "$api_doc" \
    && grep -qF 'is_owner' "$api_doc" && grep -qF 'owner_uid' "$api_doc" && api_members=1
  grep -qF '「所有人」Tab（#2546）' "$ui_doc" \
    && grep -qF '转让后你将成为管理员' "$ui_doc" && ui_owner=1
  grep -qF '所有人（OWNER）用户 ID' "$db_doc" && db_owner=1
  grep -qF '/api/merchants/:id/transfer-ownership' "$case_doc" \
    && ! grep -qF '/admin/merchants/:id/transfer-ownership' "$case_doc" \
    && ! grep -qF 'ADMIN 语义？见下' "$case_doc" && case_path=1

  {
    echo "$PROVENANCE"
    echo "# R11 契约文档落字（api/ui/database/cases）"
    echo "DOC_API_TRANSFER=$api_transfer"
    echo "DOC_API_MEMBERS_OWNER=$api_members"
    echo "DOC_UI_OWNER_TAB=$ui_owner"
    echo "DOC_DB_ADMIN_UID=$db_owner"
    echo "DOC_CASE_PATH_ALIGNED=$case_path"
  } >"$a"

  assert_grep "$a" 'DOC_API_TRANSFER=1' 'DOC_API_MEMBERS_OWNER=1' 'DOC_UI_OWNER_TAB=1' \
    'DOC_DB_ADMIN_UID=1' 'DOC_CASE_PATH_ALIGNED=1'
}

case "$ROW" in
  r1) row_r1 ;;
  r2) row_r2 ;;
  r3) row_r3 ;;
  r4) row_r4 ;;
  r5) row_r5 ;;
  r6) row_r6 ;;
  r7) row_r7 ;;
  r8) row_r8 ;;
  r9) row_r9 ;;
  r10) row_r10 ;;
  r11) row_r11 ;;
  *)
    echo "Usage: bash scripts/verify-2546.sh r1..r11" >&2
    exit 2
    ;;
esac

if [ "$FAILURES" != "0" ]; then
  echo "RESULT: FAIL ($ROW, failures=$FAILURES)" >&2
  exit 1
fi
echo "RESULT: PASS ($ROW)"
