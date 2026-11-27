#!/usr/bin/env bash
# #2624 证据/回归测试：用户缓存对账的「可选修复」通道（--apply）。
#
# 用法：
#   bash scripts/tests/test_reconcile_user_cache_apply_2624.sh r1   # 闸门 + SQL 形状（离线 --sql-only）+ R5 disabled 跳过 + R6 产物写失败
#   bash scripts/tests/test_reconcile_user_cache_apply_2624.sh r2   # 一次性 PG：单事务真修复 + 审计 + 复查归零 + R5 disabled 行不被修
#   bash scripts/tests/test_reconcile_user_cache_apply_2624.sh r3   # 回滚还原 + 改值型并发中止 + R4 行被删除型并发中止
#
# 全部在一次性 PostgreSQL（127.0.0.1:15433，#2341 口径）上跑；未触碰 dev jobmaster-postgres、
# 预生产/生产库。判定逻辑与 token 契约仍由 #2623 拥有——本测试只消费它的输出。
set -uo pipefail

REPO="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$REPO" || exit 2

ISSUE=2624
TOOL="scripts/reconcile-user-cache.sh"
FIXTURE_DIR="scripts/fixtures/reconcile-2624"
ROW="${1:-}"

# ---------------------------------------------------------------- provenance
HEAD_SHA="$(git rev-parse HEAD)"
FIX_REF="${FIX_COMMIT:-$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)}"
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}" 2>/dev/null || git rev-parse HEAD)"
if ! git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" 2>/dev/null; then
  echo "FATAL: FIX_COMMIT is not an ancestor of HEAD" >&2
  exit 3
fi
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)docs/evidence/$ISSUE" ':(exclude)tmp')" ]; then
  DIRTY=true
fi
echo "# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
echo "GENERATED_AT_HEAD=$HEAD_SHA"

# ------------------------------------------------------------------- helpers
TMP="$(mktemp -d "${TMPDIR:-$REPO/tmp}/apply-$ISSUE.XXXXXX")" || exit 2
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT

FAILED=0
expect() { # name expected actual
  local name="$1" expected="$2" actual="$3"
  if [ "$expected" = "$actual" ]; then
    printf '%s=%s\n' "$name" "$actual"
  else
    printf '%s=MISMATCH(expected=%s actual=%s)\n' "$name" "$expected" "$actual"
    FAILED=1
  fi
}
tok() { grep -E "^$2=" "$1" 2>/dev/null | head -n 1 | cut -d= -f2-; }
finish() {
  if [ "$FAILED" = "0" ]; then
    echo "TEST_EXIT=0"
    echo "RESULT: PASS"
    exit 0
  fi
  echo "TEST_EXIT=1"
  echo "RESULT: FAIL"
  exit 1
}

# ------------------------------------------------------------ PG (disposable)
pg_setup() { # -> sets PSQL array + creates DB
  # shellcheck disable=SC1091
  . scripts/lib-2341-env.sh
  # lib-2341-env.sh 自带 set -e；本测试必须能观察「失败路径」的退出码，故显式关闭。
  set +e
  mkdir -p "${TMPDIR:-$REPO/tmp/gobuild}" 2>/dev/null || true
  require_pg
  DB="apply_2624_$$_${RANDOM}"
  PORT="${POSTGRES_PORT:-15433}"
  PSQL=("$PSQL_BIN" -h "$POSTGRES_HOST" -p "$PORT" -U "$POSTGRES_USER" -v ON_ERROR_STOP=1)
  export PGPASSWORD="$POSTGRES_PASSWORD"
  psql_admin "CREATE DATABASE $DB" >/dev/null 2>&1 || { echo "FATAL: cannot create test database" >&2; exit 2; }
  trap 'psql_admin "DROP DATABASE IF EXISTS '"$DB"'" >/dev/null 2>&1 || true; cleanup' EXIT
  "${PSQL[@]}" -d "$DB" -q -c "
    CREATE TABLE users (
      id uuid PRIMARY KEY,
      iam_sub varchar(255),
      tenant_id uuid NOT NULL,
      org_id uuid NOT NULL,
      role varchar(50),
      status varchar(20),
      email varchar(255),
      deleted_at timestamptz,
      updated_at timestamptz NOT NULL DEFAULT now()
    );
    CREATE TABLE audit_logs (
      id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      tenant_id uuid NOT NULL,
      org_id uuid,
      user_id uuid NOT NULL,
      actor_name varchar(100),
      actor_role varchar(50),
      action varchar(50) NOT NULL,
      resource_type varchar(50) NOT NULL,
      resource_id varchar(100),
      status_code int,
      status varchar(20),
      error_message text,
      details jsonb,
      request_body jsonb,
      ip_address varchar(45),
      user_agent varchar(500),
      created_at timestamptz DEFAULT now()
    );
  " >/dev/null || { echo "FATAL: cannot create schema" >&2; exit 2; }
}

seed_drift() { # inserts the 8 local rows that exist in the fixture (6 drift + 2 orphan)
  "${PSQL[@]}" -d "$DB" -q -c "
    INSERT INTO users (id, iam_sub, tenant_id, org_id, role, status, email) VALUES
      ('aaaa0000-0000-4000-8000-000000000001','a-tenant-1','33333333-3333-3333-3333-333333333333','22222222-2222-2222-2222-222222222222','site_member','active','t1@x'),
      ('aaaa0000-0000-4000-8000-000000000002','a-tenant-2','33333333-3333-3333-3333-333333333333','22222222-2222-2222-2222-222222222222','site_admin','active','t2@x'),
      ('aaaa0000-0000-4000-8000-000000000003','a-tenant-3','33333333-3333-3333-3333-333333333333','11111111-1111-1111-1111-111111111111','merchant_admin','active','t3@x'),
      ('aaaa0000-0000-4000-8000-000000000004','a-org-1','11111111-1111-1111-1111-111111111111','33333333-3333-3333-3333-333333333333','site_member','active','o1@x'),
      ('aaaa0000-0000-4000-8000-000000000005','a-org-2','11111111-1111-1111-1111-111111111111','22222222-2222-2222-2222-222222222222','site_admin','active','o2@x'),
      ('aaaa0000-0000-4000-8000-000000000006','a-nil-1','00000000-0000-0000-0000-000000000000','00000000-0000-0000-0000-000000000000','site_member','active','n1@x'),
      ('aaaa0000-0000-4000-8000-000000000007','a-orphan-1','11111111-1111-1111-1111-111111111111','22222222-2222-2222-2222-222222222222','site_member','active','x1@x'),
      ('aaaa0000-0000-4000-8000-000000000008','a-orphan-2','11111111-1111-1111-1111-111111111111','22222222-2222-2222-2222-222222222222','site_member','active','x2@x'),
      ('aaaa0000-0000-4000-8000-000000000009','a-role-unverified','11111111-1111-1111-1111-111111111111','22222222-2222-2222-2222-222222222222','site_member','active','r1@x');
  " >/dev/null || { echo "FATAL: cannot seed users" >&2; exit 2; }
}

sql_scalar() { "${PSQL[@]}" -d "$DB" -Atc "$1"; }

apply_env() { # prints env prefix for the tool against the test DB
  printf 'TUNELOOP_DB_HOST=%s TUNELOOP_DB_PORT=%s TUNELOOP_DB_USER=%s TUNELOOP_DB_PASSWORD=%s TUNELOOP_DB_NAME=%s' \
    "$POSTGRES_HOST" "$PORT" "$POSTGRES_USER" "$POSTGRES_PASSWORD" "$DB"
}

run_apply() { # outfile extra-args...
  local out="$1"; shift
  env OWNER_APPROVED_USER_CACHE_RECONCILE=1 \
      TUNELOOP_DB_HOST="$POSTGRES_HOST" TUNELOOP_DB_PORT="$PORT" \
      TUNELOOP_DB_USER="$POSTGRES_USER" TUNELOOP_DB_PASSWORD="$POSTGRES_PASSWORD" \
      TUNELOOP_DB_NAME="$DB" \
      bash "$TOOL" "$@" > "$out" 2>&1
  printf '%s' "$?"
}

case "$ROW" in
r1)
  # 1) 未授权：不得写库、不得产出 SQL
  UNAUTH_OUT="$TMP/r1-unauth.txt"
  rm -f "$TMP/unauth.sql" "$TMP/unauth-rb.sql"
  bash "$TOOL" --apply --confirm-rows 6 --sql-out "$TMP/unauth.sql" --rollback-file "$TMP/unauth-rb.sql" \
    --fixture "$FIXTURE_DIR" > "$UNAUTH_OUT" 2>&1
  UNAUTH_RC=$?
  sed -n '1,25p' "$UNAUTH_OUT"
  expect UNAUTHORIZED_EXIT 2 "$UNAUTH_RC"
  expect UNAUTHORIZED_APPLY 1 "$(tok "$UNAUTH_OUT" UNAUTHORIZED_APPLY)"
  expect UNAUTHORIZED_DB_WRITES 0 "$(tok "$UNAUTH_OUT" DB_WRITES)"
  expect UNAUTHORIZED_NO_SQL 0 "$( { [ -f "$TMP/unauth.sql" ] || [ -f "$TMP/unauth-rb.sql" ]; } && echo 1 || echo 0)"

  # 2) 已授权但 --confirm-rows 不符：必须拒绝
  MISMATCH_OUT="$TMP/r1-mismatch.txt"
  OWNER_APPROVED_USER_CACHE_RECONCILE=1 bash "$TOOL" --apply --confirm-rows 5 \
    --sql-out "$TMP/mismatch.sql" --rollback-file "$TMP/mismatch-rb.sql" \
    --fixture "$FIXTURE_DIR" > "$MISMATCH_OUT" 2>&1
  MISMATCH_RC=$?
  expect CONFIRM_MISMATCH_EXIT 2 "$MISMATCH_RC"
  expect APPLY_CONFIRM_MISMATCH 1 "$(tok "$MISMATCH_OUT" APPLY_CONFIRM_MISMATCH)"
  expect CONFIRM_MISMATCH_DB_WRITES 0 "$(tok "$MISMATCH_OUT" DB_WRITES)"

  # 3) 授权 + 行数相符 + --sql-only：只生成 SQL，DB_WRITES=0
  OK_OUT="$TMP/r1-ok.txt"
  OWNER_APPROVED_USER_CACHE_RECONCILE=1 bash "$TOOL" --apply --sql-only --confirm-rows 6 \
    --sql-out "$TMP/fix.sql" --rollback-file "$TMP/rollback.sql" \
    --fixture "$FIXTURE_DIR" > "$OK_OUT" 2>&1
  OK_RC=$?
  sed -n '1,30p' "$OK_OUT"
  expect SQL_ONLY_EXIT 0 "$OK_RC"
  expect REPAIR_ROWS 6 "$(tok "$OK_OUT" REPAIR_ROWS)"
  expect SKIPPED_ORPHAN 2 "$(tok "$OK_OUT" SKIPPED_ORPHAN)"
  expect SKIPPED_MISSING_LOCAL 1 "$(tok "$OK_OUT" SKIPPED_MISSING_LOCAL)"
  expect SKIPPED_ROLE_UNVERIFIED 1 "$(tok "$OK_OUT" SKIPPED_ROLE_UNVERIFIED)"
  expect ROLE_APPLY_SKIPPED 1 "$(tok "$OK_OUT" ROLE_APPLY_SKIPPED)"
  expect SQL_ONLY_DB_WRITES 0 "$(tok "$OK_OUT" DB_WRITES)"

  CAS_ROWS="$(grep -cE "^ WHERE id = '[0-9a-f-]+' AND iam_sub = '[^']+' AND tenant_id = '[0-9a-f-]+' AND org_id = '[0-9a-f-]+' AND deleted_at IS NULL;$" "$TMP/fix.sql" 2>/dev/null | tr -d ' ')"
  expect SQL_HAS_CAS 1 "$([ "${CAS_ROWS:-0}" = "6" ] && echo 1 || echo 0)"
  AUDIT_INSERTS="$(grep -c 'INSERT INTO audit_logs' "$TMP/fix.sql" 2>/dev/null | tr -d ' ')"
  expect SQL_HAS_AUDIT 6 "$AUDIT_INSERTS"
  POSTCHECKS="$(grep -c 'RAISE EXCEPTION' "$TMP/fix.sql" 2>/dev/null | tr -d ' ')"
  expect SQL_HAS_POSTCHECK 1 "$([ "${POSTCHECKS:-0}" = "6" ] && echo 1 || echo 0)"
  # B1 回归：后置校验必须是「断言达到新值」（NOT EXISTS），不能是「断言不匹配」（行被删除时漏检）
  POSTCHECK_NOT_EXISTS="$(grep -c 'IF NOT EXISTS (SELECT 1 FROM users' "$TMP/fix.sql" 2>/dev/null | tr -d ' ')"
  expect SQL_POSTCHECK_ASSERTS_TARGET 1 "$([ "${POSTCHECK_NOT_EXISTS:-0}" = "6" ] && echo 1 || echo 0)"
  expect SQL_POSTCHECK_LEGACY_MISMATCH_FORM 0 "$(grep -c 'IF EXISTS (SELECT 1 FROM users WHERE id' "$TMP/fix.sql" 2>/dev/null | tr -d ' ')"
  SUBS="$(grep -oE '^-- iam_sub=[^ ]+' "$TMP/fix.sql" | sed 's/^-- iam_sub=//')"
  expect SQL_ORDERED_BY_IAM_SUB 1 "$([ "$SUBS" = "$(printf '%s\n' "$SUBS" | LC_ALL=C sort)" ] && echo 1 || echo 0)"
  expect SQL_SINGLE_TRANSACTION 1 "$([ "$(grep -c '^BEGIN;$' "$TMP/fix.sql")" = "1" ] && [ "$(grep -c '^COMMIT;$' "$TMP/fix.sql")" = "1" ] && echo 1 || echo 0)"
  expect SQL_NO_UNAUTHORIZED_APPLY_FLAG 1 "$([ "$(grep -c -- '--apply' "$TMP/fix.sql")" = "0" ] && echo 1 || echo 0)"
  expect ROLLBACK_ACTION_ROWS 6 "$(grep -c "'USER_CACHE_RECONCILE_ROLLBACK'" "$TMP/rollback.sql" 2>/dev/null | tr -d ' ')"
  expect ROLLBACK_MIRRORS_OLD 1 "$( [ "$(grep -c "tenant_id = '33333333-3333-3333-3333-333333333333'" "$TMP/rollback.sql")" -ge 3 ] && echo 1 || echo 0)"

  # 4) #2624 R5/B2（离线）：status='disabled' 的漂移行禁止触碰
  #    fixture 由计划内的 local.tsv 派生（只把 a-tenant-1 的 status 改为 disabled），不新增计划外文件。
  DIS_LOCAL="$TMP/local-disabled.tsv"
  awk -F'\t' -v OFS='\t' '{ if ($1 == "a-tenant-1") $6 = "disabled"; print }' \
    "$FIXTURE_DIR/local.tsv" > "$DIS_LOCAL"
  DIS_OUT="$TMP/r1-disabled.txt"
  rm -f "$TMP/fix-disabled.sql" "$TMP/rollback-disabled.sql"
  OWNER_APPROVED_USER_CACHE_RECONCILE=1 bash "$TOOL" --apply --sql-only --confirm-rows 5 \
    --sql-out "$TMP/fix-disabled.sql" --rollback-file "$TMP/rollback-disabled.sql" \
    --iam-fixture "$FIXTURE_DIR/iam.tsv" --local-fixture "$DIS_LOCAL" > "$DIS_OUT" 2>&1
  DIS_RC=$?
  sed -n '1,20p' "$DIS_OUT"
  expect DISABLED_OFFLINE_EXIT 0 "$DIS_RC"
  expect SKIPPED_DISABLED 1 "$(tok "$DIS_OUT" SKIPPED_DISABLED)"
  expect DISABLED_REPAIR_ROWS 5 "$(tok "$DIS_OUT" REPAIR_ROWS)"
  expect DISABLED_DB_WRITES 0 "$(tok "$DIS_OUT" DB_WRITES)"
  DIS_HITS="$(grep -c 'a-tenant-1' "$TMP/fix-disabled.sql" 2>/dev/null | tr -d ' ')"
  expect DISABLED_NOT_IN_SQL 1 "$([ "${DIS_HITS:-0}" = "0" ] && echo 1 || echo 0)"
  # D1：跳过修复不得改变只读分类结果（disabled 行仍计入 TENANT_MISMATCH）
  expect DISABLED_CATEGORY_UNCHANGED 3 "$(tok "$DIS_OUT" TENANT_MISMATCH)"

  # 5) #2624 R6/B3：SQL 产物写不出来 → 必须显式失败（exit 2 + SQL_WRITE_FAILED=1，不得 RESULT: PASS）
  WRITE_FAIL_OUT="$TMP/r1-write-fail.txt"
  OWNER_APPROVED_USER_CACHE_RECONCILE=1 bash "$TOOL" --apply --sql-only --confirm-rows 6 \
    --sql-out /proc/notallowed/fix.sql --rollback-file /proc/notallowed/rb.sql \
    --fixture "$FIXTURE_DIR" > "$WRITE_FAIL_OUT" 2>&1
  WRITE_FAIL_RC=$?
  sed -n '1,25p' "$WRITE_FAIL_OUT"
  expect SQL_WRITE_FAIL_EXIT 2 "$WRITE_FAIL_RC"
  expect SQL_WRITE_FAIL_TOKEN 1 "$(tok "$WRITE_FAIL_OUT" SQL_WRITE_FAILED)"
  expect SQL_WRITE_FAIL_DB_WRITES 0 "$(tok "$WRITE_FAIL_OUT" DB_WRITES)"
  expect SQL_WRITE_FAIL_NO_PASS 0 "$(grep -c 'RESULT: PASS' "$WRITE_FAIL_OUT" 2>/dev/null | tr -d ' ')"
  expect SQL_WRITE_FAIL_NO_FILE 0 "$( [ -f /proc/notallowed/fix.sql ] && echo 1 || echo 0)"
  finish
  ;;

r2)
  pg_setup
  seed_drift
  APPLY_OUT="$TMP/r2-apply.txt"
  APPLY_RC="$(run_apply "$APPLY_OUT" --apply --confirm-rows 6 \
    --sql-out "$TMP/r2-fix.sql" --rollback-file "$TMP/r2-rollback.sql" \
    --iam-fixture "$FIXTURE_DIR/iam.tsv")"
  sed -n '1,30p' "$APPLY_OUT"
  expect APPLY_EXIT 0 "$APPLY_RC"
  expect REPAIR_ROWS 6 "$(tok "$APPLY_OUT" REPAIR_ROWS)"
  expect DB_WRITES 6 "$(tok "$APPLY_OUT" DB_WRITES)"
  expect AUDIT_ROWS 6 "$(tok "$APPLY_OUT" AUDIT_ROWS)"

  BATCH="$(tok "$APPLY_OUT" BATCH_ID)"
  AUDIT_IN_DB="$(sql_scalar "SELECT count(*) FROM audit_logs WHERE action='USER_CACHE_RECONCILE'")"
  expect AUDIT_ACTION_USER_CACHE_RECONCILE 6 "$AUDIT_IN_DB"
  BATCH_IN_DB="$(sql_scalar "SELECT count(*) FROM audit_logs WHERE details->>'batch_id' = '$BATCH'")"
  expect BATCH_ID_MATCH 1 "$([ "$BATCH_IN_DB" = "6" ] && echo 1 || echo 0)"
  DETAILS_OK="$(sql_scalar "SELECT count(*) FROM audit_logs WHERE details ? 'old' AND details ? 'new' AND details ? 'category'")"
  expect AUDIT_DETAILS_FIELDS 6 "$DETAILS_OK"

  # 复查（只读 L1 口径）
  RECHECK_OUT="$TMP/r2-recheck.txt"
  TUNELOOP_DB_HOST="$POSTGRES_HOST" TUNELOOP_DB_PORT="$PORT" TUNELOOP_DB_USER="$POSTGRES_USER" \
  TUNELOOP_DB_PASSWORD="$POSTGRES_PASSWORD" TUNELOOP_DB_NAME="$DB" \
    bash "$TOOL" --iam-fixture "$FIXTURE_DIR/iam.tsv" > "$RECHECK_OUT" 2>&1
  RECHECK_RC=$?
  expect RECHECK_EXIT 0 "$RECHECK_RC"
  expect RECHECK_TENANT_MISMATCH 0 "$(tok "$RECHECK_OUT" TENANT_MISMATCH)"
  expect RECHECK_ORG_MISMATCH 0 "$(tok "$RECHECK_OUT" ORG_MISMATCH)"
  expect RECHECK_NIL_UUID 0 "$(tok "$RECHECK_OUT" NIL_UUID)"
  expect RECHECK_ORPHAN_LOCAL 2 "$(tok "$RECHECK_OUT" ORPHAN_LOCAL)"
  expect RECHECK_DB_WRITES 0 "$(tok "$RECHECK_OUT" DB_WRITES)"

  # #2624 R5/B2（真修复路径）：status='disabled' 的漂移行不得被 UPDATE/审计
  "${PSQL[@]}" -d "$DB" -q -c "
    UPDATE users SET status='disabled', tenant_id='33333333-3333-3333-3333-333333333333' WHERE iam_sub='a-tenant-2';
    UPDATE users SET tenant_id='33333333-3333-3333-3333-333333333333' WHERE iam_sub='a-org-1';
  " >/dev/null || { echo "FATAL: cannot build disabled scenario" >&2; exit 2; }

  DIS_APPLY_OUT="$TMP/r2-disabled-apply.txt"
  DIS_AUDIT_BEFORE="$(sql_scalar "SELECT count(*) FROM audit_logs WHERE resource_id='a-tenant-2'")"
  DIS_APPLY_RC="$(run_apply "$DIS_APPLY_OUT" --apply --confirm-rows 1 \
    --sql-out "$TMP/r2-disabled-fix.sql" --rollback-file "$TMP/r2-disabled-rb.sql" \
    --iam-fixture "$FIXTURE_DIR/iam.tsv")"
  sed -n '1,25p' "$DIS_APPLY_OUT"
  expect DISABLED_APPLY_EXIT 0 "$DIS_APPLY_RC"
  expect DISABLED_SKIPPED 1 "$(tok "$DIS_APPLY_OUT" SKIPPED_DISABLED)"
  expect DISABLED_REPAIR_ROWS 1 "$(tok "$DIS_APPLY_OUT" REPAIR_ROWS)"
  expect DISABLED_DB_WRITES 1 "$(tok "$DIS_APPLY_OUT" DB_WRITES)"
  DIS_SQL_HITS="$(grep -c 'a-tenant-2' "$TMP/r2-disabled-fix.sql" 2>/dev/null | tr -d ' ')"
  expect DISABLED_NOT_IN_SQL 1 "$([ "${DIS_SQL_HITS:-0}" = "0" ] && echo 1 || echo 0)"
  DIS_ROW="$(sql_scalar "SELECT status || '|' || tenant_id::text FROM users WHERE iam_sub='a-tenant-2'")"
  expect DISABLED_ROW_UNCHANGED 1 "$([ "$DIS_ROW" = 'disabled|33333333-3333-3333-3333-333333333333' ] && echo 1 || echo 0)"
  DIS_AUDIT_AFTER="$(sql_scalar "SELECT count(*) FROM audit_logs WHERE resource_id='a-tenant-2'")"
  expect DISABLED_AUDIT_ROWS 0 "$(( DIS_AUDIT_AFTER - DIS_AUDIT_BEFORE ))"
  finish
  ;;

r3)
  pg_setup
  seed_drift
  HASH_BEFORE="$(sql_scalar "SELECT md5(string_agg(id::text||tenant_id::text||org_id::text, ',' ORDER BY id)) FROM users")"

  APPLY_OUT="$TMP/r3-apply.txt"
  APPLY_RC="$(run_apply "$APPLY_OUT" --apply --confirm-rows 6 \
    --sql-out "$TMP/r3-fix.sql" --rollback-file "$TMP/r3-rollback.sql" \
    --iam-fixture "$FIXTURE_DIR/iam.tsv")"
  expect APPLY_EXIT 0 "$APPLY_RC"
  expect APPLY_DB_WRITES 6 "$(tok "$APPLY_OUT" DB_WRITES)"

  # 回滚：直接执行生成的 rollback 文件（运维可复核后手动执行）
  PGPASSWORD="$POSTGRES_PASSWORD" "${PSQL[@]}" -d "$DB" -q -f "$TMP/r3-rollback.sql" > "$TMP/r3-rollback.log" 2>&1
  ROLLBACK_RC=$?
  expect ROLLBACK_SQL_EXIT 0 "$ROLLBACK_RC"
  RESTORED="$(sql_scalar "SELECT count(*) FROM users WHERE id::text||tenant_id::text||org_id::text IN (
      'aaaa0000-0000-4000-8000-00000000000133333333-3333-3333-3333-33333333333322222222-2222-2222-2222-222222222222',
      'aaaa0000-0000-4000-8000-00000000000233333333-3333-3333-3333-33333333333322222222-2222-2222-2222-222222222222',
      'aaaa0000-0000-4000-8000-00000000000333333333-3333-3333-3333-33333333333311111111-1111-1111-1111-111111111111',
      'aaaa0000-0000-4000-8000-00000000000411111111-1111-1111-1111-11111111111133333333-3333-3333-3333-333333333333',
      'aaaa0000-0000-4000-8000-00000000000511111111-1111-1111-1111-11111111111122222222-2222-2222-2222-222222222222',
      'aaaa0000-0000-4000-8000-00000000000600000000-0000-0000-0000-00000000000000000000-0000-0000-0000-000000000000'
    )")"
  expect RESTORED_ROWS 6 "$RESTORED"
  ROLLBACK_AUDIT="$(sql_scalar "SELECT count(*) FROM audit_logs WHERE action='USER_CACHE_RECONCILE_ROLLBACK'")"
  expect ROLLBACK_AUDIT_ROWS 6 "$ROLLBACK_AUDIT"
  POST_ROLLBACK_MATCH="$(sql_scalar "SELECT count(*) FROM users u JOIN (VALUES
      ('a-tenant-1','33333333-3333-3333-3333-333333333333','22222222-2222-2222-2222-222222222222'),
      ('a-tenant-2','33333333-3333-3333-3333-333333333333','22222222-2222-2222-2222-222222222222'),
      ('a-tenant-3','33333333-3333-3333-3333-333333333333','11111111-1111-1111-1111-111111111111'),
      ('a-org-1','11111111-1111-1111-1111-111111111111','33333333-3333-3333-3333-333333333333'),
      ('a-org-2','11111111-1111-1111-1111-111111111111','22222222-2222-2222-2222-222222222222'),
      ('a-nil-1','00000000-0000-0000-0000-000000000000','00000000-0000-0000-0000-000000000000')
    ) AS v(sub, tenant, org)
    ON u.iam_sub = v.sub AND u.tenant_id::text = v.tenant AND u.org_id::text = v.org")"
  expect POST_ROLLBACK_TENANT_MATCH 6 "$POST_ROLLBACK_MATCH"

  # 并发中止：SQL 生成后、执行前把一行改成「第三值」→ CAS 不命中 → 后置校验 RAISE → 整体回滚
  # （用 --sql-only 取快照 SQL，再直接执行该文件，精确复现「生成与执行之间被并发修改」）
  CONC_GEN_OUT="$TMP/r3-concurrent-gen.txt"
  CONC_GEN_RC="$(run_apply "$CONC_GEN_OUT" --apply --sql-only --confirm-rows 6 \
    --sql-out "$TMP/r3-concurrent.sql" --rollback-file "$TMP/r3-concurrent-rb.sql" \
    --iam-fixture "$FIXTURE_DIR/iam.tsv")"
  expect CONCURRENT_GEN_EXIT 0 "$CONC_GEN_RC"
  # 并发例：SQL 生成阶段不写库（DB_WRITES=0）；实际写库为 0 由下面的「行不变 + 审计不变」证明
  expect DB_WRITES 0 "$(tok "$CONC_GEN_OUT" DB_WRITES)"

  "${PSQL[@]}" -d "$DB" -q -c "UPDATE users SET tenant_id='44444444-4444-4444-4444-444444444444' WHERE iam_sub='a-tenant-1'" >/dev/null
  HASH_TAMPERED="$(sql_scalar "SELECT md5(string_agg(id::text||tenant_id::text||org_id::text, ',' ORDER BY id)) FROM users")"
  AUDIT_BEFORE_CONC="$(sql_scalar "SELECT count(*) FROM audit_logs")"

  PGPASSWORD="$POSTGRES_PASSWORD" "${PSQL[@]}" -d "$DB" -q -f "$TMP/r3-concurrent.sql" > "$TMP/r3-concurrent-exec.log" 2>&1
  CONC_RC=$?
  sed -n '1,10p' "$TMP/r3-concurrent-exec.log"
  expect CONCURRENT_SQL_EXIT_NONZERO 1 "$([ "$CONC_RC" != "0" ] && echo 1 || echo 0)"
  expect CONCURRENT_ABORT 1 "$(grep -q 'concurrent change detected' "$TMP/r3-concurrent-exec.log" && echo 1 || echo 0)"
  HASH_AFTER="$(sql_scalar "SELECT md5(string_agg(id::text||tenant_id::text||org_id::text, ',' ORDER BY id)) FROM users")"
  expect ROWS_CHANGED_AFTER_ABORT 0 "$([ "$HASH_AFTER" = "$HASH_TAMPERED" ] && echo 0 || echo 1)"
  expect NO_PARTIAL_WRITE 0 "$( [ "$(sql_scalar "SELECT count(*) FROM audit_logs")" = "$AUDIT_BEFORE_CONC" ] && echo 0 || echo 1)"
  expect FULL_TABLE_UNCHANGED_BY_ABORT 1 "$([ "$HASH_AFTER" = "$HASH_TAMPERED" ] && echo 1 || echo 0)"

  # #2624 R4/B1：快照后目标行被**硬删除** → 后置校验必须 RAISE（NOT EXISTS 形态）→ 整体回滚
  # 先把并发例篡改的行还原，回到「干净漂移态」再生成 SQL 快照
  "${PSQL[@]}" -d "$DB" -q -c \
    "UPDATE users SET tenant_id='33333333-3333-3333-3333-333333333333' WHERE iam_sub='a-tenant-1'" >/dev/null
  DEL_GEN_OUT="$TMP/r3-delete-gen.txt"
  DEL_GEN_RC="$(run_apply "$DEL_GEN_OUT" --apply --sql-only --confirm-rows 6 \
    --sql-out "$TMP/r3-delete.sql" --rollback-file "$TMP/r3-delete-rb.sql" \
    --iam-fixture "$FIXTURE_DIR/iam.tsv")"
  expect DELETE_ROW_GEN_EXIT 0 "$DEL_GEN_RC"
  expect DELETE_ROW_GEN_DB_WRITES 0 "$(tok "$DEL_GEN_OUT" DB_WRITES)"

  "${PSQL[@]}" -d "$DB" -q -c "DELETE FROM users WHERE iam_sub='a-nil-1'" >/dev/null
  HASH_AFTER_DELETE="$(sql_scalar "SELECT md5(string_agg(id::text||tenant_id::text||org_id::text, ',' ORDER BY id)) FROM users")"
  AUDIT_AFTER_DELETE="$(sql_scalar "SELECT count(*) FROM audit_logs")"

  PGPASSWORD="$POSTGRES_PASSWORD" "${PSQL[@]}" -d "$DB" -q -f "$TMP/r3-delete.sql" > "$TMP/r3-delete-exec.log" 2>&1
  DEL_RC=$?
  sed -n '1,10p' "$TMP/r3-delete-exec.log"
  expect DELETE_ROW_SQL_EXIT_NONZERO 1 "$([ "$DEL_RC" != "0" ] && echo 1 || echo 0)"
  expect DELETE_ROW_ABORT 1 "$(grep -q 'concurrent change detected' "$TMP/r3-delete-exec.log" && echo 1 || echo 0)"
  expect DELETE_ROW_NO_PASS 0 "$(grep -c 'RESULT: PASS' "$TMP/r3-delete-exec.log" 2>/dev/null | tr -d ' ')"
  HASH_AFTER_DELETE_ABORT="$(sql_scalar "SELECT md5(string_agg(id::text||tenant_id::text||org_id::text, ',' ORDER BY id)) FROM users")"
  DELETE_ROW_CHANGED=0
  [ "$HASH_AFTER_DELETE_ABORT" = "$HASH_AFTER_DELETE" ] || DELETE_ROW_CHANGED=1
  expect ROWS_CHANGED_AFTER_DELETE_ABORT 0 "$DELETE_ROW_CHANGED"
  expect DELETE_ROW_DB_WRITES 0 "$DELETE_ROW_CHANGED"
  expect DELETE_ROW_AUDIT_ROWS 0 "$(( $(sql_scalar "SELECT count(*) FROM audit_logs") - AUDIT_AFTER_DELETE ))"
  finish
  ;;

*)
  echo "usage: $0 <r1|r2|r3>" >&2
  exit 2
  ;;
esac
