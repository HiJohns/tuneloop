#!/usr/bin/env bash
# verify-2543.sh — Evidence Contract runner for #2543
#   (确认执行主体域收敛：merchant_admin 分支不写 IAM)
#
# Usage:
#   bash scripts/verify-2543.sh r1 … r5
#
# Each row prints a provenance header, runs the row's commands, writes the
# artifact under docs/evidence/2543/ and fails (exit 1) when any expected
# assertion token is missing. Heavy commands (go build/test) go through
# scripts/heavy-run.sh when available so the host lock is honoured per command.
set -uo pipefail

ROW="${1:-}"
ISSUE=2543
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

if [ -z "${POSTGRES_PASSWORD:-}" ]; then
  echo "ERROR: POSTGRES_PASSWORD is empty (backend/.env missing?). Tests need a real test-DB password." >&2
  exit 2
fi

COMMIT="$(git rev-parse HEAD)"
SHORT="$(git rev-parse --short HEAD)"
BUILD="1.0.$SHORT"
# Evidence artifacts are regenerated after the fix commit; their own churn must not
# mark the tree dirty (they are committed separately as docs(evidence): issue-2543).
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
  local a="$OUT/R1-no-iam-write.txt"
  {
    echo "$PROVENANCE"
    echo "# R1 merchant_admin 动作零 IAM 写入：确认成功 + 用户激活 + 会话 confirmed"
  } >"$a"
  go_test_row "$a" 'TestExecuteAction_MerchantAdmin_NoIAMWrite_2543' ./handlers/
  assert_grep "$a" 'CONFIRM_STATUS=200' 'SESSION_STATUS=confirmed' 'USER_STATUS=active' 'IAM_CALLS=0'
}

row_r2() {
  local a="$OUT/R2-subject-negative.txt"
  local src="$ROOT/backend/handlers/confirmation_session.go"
  {
    echo "$PROVENANCE"
    echo "# R2 负例：不得出现以本地 users.id 为主体的 IAM 请求（源码 + 运行时 0 调用）"
  } >"$a"

  go_test_row "$a" 'TestExecuteAction_MerchantAdmin_NoIAMWrite_2543' ./handlers/

  local iam_refs bind_hits subject_hits=0
  iam_refs=$(grep -c 'iamClient\.' "$src" || true)
  bind_hits=$(grep -cE 'BindUserToOrganization\(|SetUserCustomerPermissions\(|UpdateUserRoleInOrg\(' "$src" || true)
  if [ "$iam_refs" != "0" ] || [ "$bind_hits" != "0" ]; then
    subject_hits=1
  fi

  {
    echo "IAM_CLIENT_REFS=$iam_refs"
    echo "BIND_SESSION_USERID_HITS=$bind_hits"
    echo "LOCAL_ID_AS_IAM_SUBJECT=$subject_hits"
  } >>"$a"

  assert_grep "$a" 'LOCAL_ID_AS_IAM_SUBJECT=0' 'BIND_SESSION_USERID_HITS=0' 'IAM_CLIENT_REFS=0' 'IAM_CALLS=0'
}

row_r3() {
  local a="$OUT/R3-site-branch-regression.txt"
  {
    echo "$PROVENANCE"
    echo "# R3 site_manager/site_staff 回归：本地写 site_members、0 次 IAM 调用"
  } >"$a"
  go_test_row "$a" 'TestExecuteAction_SiteStaff_Regression_2543' ./handlers/
  assert_grep "$a" 'SITE_MEMBER_CREATED=1' 'SITE_MEMBER_ROLE=Staff' 'IAM_CALLS=0'
}

row_r4() {
  local a="$OUT/R4-build-regression.txt"
  local build_rc regression_rc
  {
    echo "$PROVENANCE"
    echo "# R4 go build ./... + 既有 ConfirmationSession 用例回归"
  } >"$a"

  ( cd "$ROOT/backend" && heavy go build ./... ) >>"$a" 2>&1
  build_rc=$?
  echo "BUILD_RC=$build_rc" >>"$a"

  ( cd "$ROOT/backend" && heavy go test ./handlers/ -run 'ConfirmationSession' -count=1 ) >>"$a" 2>&1
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

row_r5() {
  local a="$OUT/R5-doc-comment.txt"
  local api_doc="$ROOT/docs/spec/api/api.md"
  local model_doc="$ROOT/backend/models/models.go"
  local src="$ROOT/backend/handlers/confirmation_session.go"

  local doc_no_iam=0 doc_owner=0 model_comment=0 forbidden=0
  grep -qF '审批确认路径不写 IAM（#2543）' "$api_doc" && doc_no_iam=1
  grep -qF '商户管理员的 `OWNER` 绑定只发生在 `POST /api/merchants`（创建）与 `POST /api/merchants/:id/transfer-ownership`（转让）' "$api_doc" \
    && doc_owner=1
  grep -qF '非持久化（无迁移建表' "$model_doc" && model_comment=1
  grep -qF '#2543：本特性无持久化表' "$src" && forbidden=1

  {
    echo "$PROVENANCE"
    echo "# R5 文档与注释口径纠正（api.md 不写 IAM + 模型非持久化 + 代码禁令注释）"
    echo "DOC_API_NO_IAM_ON_CONFIRM=$doc_no_iam"
    echo "DOC_API_OWNER_PATHS=$doc_owner"
    echo "MODEL_COMMENT_NONPERSISTED=$model_comment"
    echo "FORBIDDEN_IAM_COMMENT=$forbidden"
  } >"$a"

  assert_grep "$a" 'DOC_API_NO_IAM_ON_CONFIRM=1' 'DOC_API_OWNER_PATHS=1' \
    'MODEL_COMMENT_NONPERSISTED=1' 'FORBIDDEN_IAM_COMMENT=1'
}

case "$ROW" in
  r1) row_r1 ;;
  r2) row_r2 ;;
  r3) row_r3 ;;
  r4) row_r4 ;;
  r5) row_r5 ;;
  *)
    echo "Usage: bash scripts/verify-2543.sh r1..r5" >&2
    exit 2
    ;;
esac

if [ "$FAILURES" != "0" ]; then
  echo "RESULT: FAIL ($ROW, failures=$FAILURES)" >&2
  exit 1
fi
echo "RESULT: PASS ($ROW)"
