#!/usr/bin/env bash
# verify-2543.sh — Evidence Contract runner for #2543
#   （确认会话建表 + IAM 主体改用 users.iam_sub；Owner 2026-11-26 裁定）
#
# Usage:
#   bash scripts/verify-2543.sh r1 … r5
#
# Each row prints a provenance header plus the row's assertion tokens and exits
# non-zero when any expected token is missing. Heavy commands (go build /
# go test) go through scripts/heavy-run.sh so the host lock is honoured per
# command.
#
# Capture a row into its artifact:
#   bash scripts/verify-2543.sh r1 > docs/evidence/2543/R1-schema.txt
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r2|r3|r4|r5) ;;
  *) echo "usage: $0 r1|r2|r3|r4|r5" >&2; exit 2 ;;
esac

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HEAVY_RUN="${HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"
LOCK_DIR="${HEAVY_LOCK_DIR:-/tmp/opencode/locks}"
WORK_TMP="$ROOT/tmp/verify-2543"
mkdir -p "$WORK_TMP"
cd "$ROOT" || exit 2

if ! command -v go >/dev/null 2>&1 && [ -x /usr/local/go/bin/go ]; then
  export PATH="/usr/local/go/bin:$PATH"
fi

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

export GOCACHE="${GOCACHE:-$ROOT/tmp/gocache}"
export TMPDIR="${TMPDIR:-$ROOT/tmp/gobuild}"
mkdir -p "$GOCACHE" "$TMPDIR"

MIGRATION="20261126002_confirmation_sessions"
UP_SQL="$ROOT/backend/database/migrations/${MIGRATION}.up.sql"
DOWN_SQL="$ROOT/backend/database/migrations/${MIGRATION}.down.sql"

COMMIT="$(git rev-parse HEAD)"
SHORT="$(git rev-parse --short HEAD)"
BUILD="1.0.$SHORT"
# Evidence artifacts are regenerated after the fix commit; their own churn must
# not mark the tree dirty (they are committed separately as docs(evidence)).
DIRTY="false"
if [ -n "$(git status --porcelain --untracked-files=no | grep -v -E '^.. docs/evidence/' || true)" ]; then
  DIRTY="true"
fi
echo "# build=$BUILD # commit=$COMMIT # dirty=$DIRTY"
echo "GENERATED_AT_HEAD=$COMMIT"

fail() { echo "R${ROW#r}_FAIL: $*" >&2; exit 1; }

heavy() {
  if [ -x "$HEAVY_RUN" ]; then
    bash "$HEAVY_RUN" --dir "$LOCK_DIR" -- "$@"
  else
    "$@"
  fi
}

# run_row_test PATTERN LOGFILE — run the targeted test (through the host lock)
# and emit the row's assertion tokens.
run_row_test() {
  local pattern="$1" log="$2"
  ( cd "$ROOT/backend" && heavy go test ./handlers -run "$pattern" -count=1 -v ) > "$log" 2>&1
  local rc=$?
  grep -E '^ *[a-z_0-9]+_test\.go:[0-9]+: (TABLE_EXISTS|COLUMN_COUNT|TOKEN_UNIQUE|EXEMPT_REMOVED|DOWN_TABLE_EXISTS|CONFIRM_STATUS|BIND_USER_ARG|PERM_USER_ARG|ROLE|IAM_LOCAL_ID_SENT|IAM_CALLS|MESSAGE_HAS_ACTIVATE|IAM_READY_|SITE_MEMBER_)' "$log" \
    | sed -E 's/^ *[a-z_0-9]+_test\.go:[0-9]+: //'
  grep -E '^--- (PASS|FAIL)' "$log" || true
  [ "$rc" = "0" ] || { tail -30 "$log"; fail "go test '$pattern' failed (see $log)"; }
}

assert_token() { # TOKEN — assert the row output (already echoed) contains it
  local token="$1"
  grep -Fq -- "$token" "$WORK_TMP/$ROW.out" || fail "missing assertion token '$token'"
}

collect() { # writes stdout to the row buffer and echoes it
  cat > "$WORK_TMP/$ROW.out"
  cat "$WORK_TMP/$ROW.out"
}

case "$ROW" in
  r1)
    {
      echo "== R1 confirmation_sessions migration (up / columns / token unique / exemption removed / down) =="
      [ -f "$UP_SQL" ] && [ -f "$DOWN_SQL" ] && echo "MIGRATION_PAIR=ok" || fail "migration up/down pair missing"
      echo "MIGRATION_FILES=$(basename "$UP_SQL"),$(basename "$DOWN_SQL")"
      run_row_test 'TestConfirmationSessionsMigration_2543' "$WORK_TMP/r1.log"
      echo "COLUMN_COUNT_MIN=17"
    } | collect
    assert_token "MIGRATION_PAIR=ok"
    assert_token "TABLE_EXISTS=1"
    assert_token "COLUMN_COUNT="
    assert_token "TOKEN_UNIQUE=1"
    assert_token "EXEMPT_REMOVED=1"
    assert_token "DOWN_TABLE_EXISTS=0"
    ;;

  r2)
    {
      echo "== R2 merchant_admin confirmation uses users.iam_sub (role OWNER) =="
      run_row_test 'TestExecuteAction_MerchantAdmin_UsesIAMSub_2543' "$WORK_TMP/r2.log"
    } | collect
    assert_token "BIND_USER_ARG=sub-2543-"
    assert_token "PERM_USER_ARG=sub-2543-"
    assert_token "ROLE=OWNER"
    assert_token "IAM_LOCAL_ID_SENT=0"
    assert_token "CONFIRM_STATUS=confirmed"
    ;;

  r3)
    {
      echo "== R3 empty iam_sub → explicit failure, zero IAM requests =="
      run_row_test 'TestExecuteAction_MerchantAdmin_EmptyIAMSub_2543' "$WORK_TMP/r3.log"
    } | collect
    assert_token "CONFIRM_STATUS=failed"
    assert_token "IAM_CALLS=0"
    assert_token "MESSAGE_HAS_ACTIVATE=1"
    ;;

  r4)
    {
      echo "== R4 docs: fact-source / delete boundary / iam_ready / owner invariant =="
      DOC_FACT_SOURCE=0
      DOC_DELETE_SYSADMIN_ONLY=0
      DOC_IAM_READY=0
      DOC_OWNER_INVARIANT=0
      grep -Fq 'IAM（beaconiam）为唯一事实来源' docs/topics/iam/iam-notes.md && DOC_FACT_SOURCE=1
      if grep -Fq '只有**命名空间管理员（系统管理员）**可以真正删除用户' docs/topics/iam/iam-notes.md &&
         grep -Fq '仅命名空间管理员（系统管理员）' docs/spec/permissions.md; then
        DOC_DELETE_SYSADMIN_ONLY=1
      fi
      if grep -Fq '"iam_ready": true' docs/spec/api/api.md &&
         grep -Fq '`iam_ready`' docs/spec/api/api.md; then
        DOC_IAM_READY=1
      fi
      if grep -Fq '组织所有人不变量' docs/topics/iam/iam-notes.md &&
         grep -Fq '必须 0' docs/topics/iam/iam-notes.md; then
        DOC_OWNER_INVARIANT=1
      fi
      echo "DOC_FACT_SOURCE=$DOC_FACT_SOURCE"
      echo "DOC_DELETE_SYSADMIN_ONLY=$DOC_DELETE_SYSADMIN_ONLY"
      echo "DOC_IAM_READY=$DOC_IAM_READY"
      echo "DOC_OWNER_INVARIANT=$DOC_OWNER_INVARIANT"
    } | collect
    assert_token "DOC_FACT_SOURCE=1"
    assert_token "DOC_DELETE_SYSADMIN_ONLY=1"
    assert_token "DOC_IAM_READY=1"
    assert_token "DOC_OWNER_INVARIANT=1"
    ;;

  r5)
    {
      echo "== R5 build + targeted tests =="
      ( cd "$ROOT/backend" && heavy go build ./... ) > "$WORK_TMP/r5-build.log" 2>&1
      rc=$?
      if [ "$rc" = "0" ]; then echo "GO_BUILD_OK=1"; else echo "GO_BUILD_OK=0"; tail -20 "$WORK_TMP/r5-build.log"; fi
      ( cd "$ROOT/backend" && heavy go test ./handlers -run '2543' -count=1 ) > "$WORK_TMP/r5-test.log" 2>&1
      rc2=$?
      if [ "$rc2" = "0" ]; then echo "GO_TEST_OK=1"; else echo "GO_TEST_OK=0"; tail -40 "$WORK_TMP/r5-test.log"; fi
      ( cd "$ROOT/backend" && heavy go test ./database -run 'Artifact' -count=1 ) > "$WORK_TMP/r5-artifacts.log" 2>&1
      rc3=$?
      if [ "$rc3" = "0" ]; then echo "GO_ARTIFACT_TESTS_OK=1"; else echo "GO_ARTIFACT_TESTS_OK=0"; tail -40 "$WORK_TMP/r5-artifacts.log"; fi
      ( cd "$ROOT/backend" && heavy go test ./handlers -run 'TestConfirmationSessionsMigration_2543' -count=1 ) > "$WORK_TMP/r5-migration.log" 2>&1
      rc4=$?
      if [ "$rc4" = "0" ]; then echo "MIGRATION_ROUNDTRIP=1"; else echo "MIGRATION_ROUNDTRIP=0"; tail -40 "$WORK_TMP/r5-migration.log"; fi
      echo "GO_BUILD_RC=$rc GO_TEST_RC=$rc2 GO_ARTIFACT_RC=$rc3 MIGRATION_RC=$rc4"
    } | collect
    assert_token "GO_BUILD_OK=1"
    assert_token "GO_TEST_OK=1"
    assert_token "GO_ARTIFACT_TESTS_OK=1"
    assert_token "MIGRATION_ROUNDTRIP=1"
    ;;

esac

exit 0
