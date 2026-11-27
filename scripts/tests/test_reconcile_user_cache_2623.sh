#!/usr/bin/env bash
# #2623 证据/回归测试：scripts/reconcile-user-cache.sh 的 6 类差异判定、只读硬闸门与失败负例。
#
# 用法：
#   bash scripts/tests/test_reconcile_user_cache_2623.sh r1   # fixture 离线分类（6 类 + 角色集合 + JSON 一致性）
#   bash scripts/tests/test_reconcile_user_cache_2623.sh r2   # 只读硬闸门 + 失败负例（坏 fixture / 不可达 / 缺 env）
#   bash scripts/tests/test_reconcile_user_cache_2623.sh r3   # 一次性 PG（127.0.0.1:15433）真实本地库只读跑 + 双路径一致性
#
# 每行输出 machine-readable token；任一断言不成立即打印期望/实际并以非 0 退出。
set -uo pipefail

REPO="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$REPO" || exit 2

ISSUE=2623
TOOL="scripts/reconcile-user-cache.sh"
FIXTURE_DIR="scripts/fixtures/reconcile-2623"
ROW="${1:-}"

# ---------------------------------------------------------------- provenance
HEAD_SHA="$(git rev-parse HEAD)"
FIX_REF="${FIX_COMMIT:-$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)}"
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}" 2>/dev/null || git rev-parse HEAD)"
if ! git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" 2>/dev/null; then
  echo "FATAL: FIX_COMMIT is not an ancestor of HEAD" >&2
  exit 3
fi
FIX_SHORT="$(git rev-parse --short=8 "$FIX_SHA")"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)docs/evidence/$ISSUE" ':(exclude)tmp')" ]; then
  DIRTY=true
fi
echo "# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
echo "GENERATED_AT_HEAD=$HEAD_SHA"

# ------------------------------------------------------------------- helpers
TMP="$(mktemp -d "${TMPDIR:-$REPO/tmp}/reconcile-$ISSUE.XXXXXX")" || exit 2
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

token_of() { # file name -> value (last occurrence)
  local file="$1" name="$2"
  grep -E "^${name}=" "$file" 2>/dev/null | tail -n 1 | cut -d= -f2-
}

run_tool() { # outfile args...
  local out="$1"; shift
  bash "$TOOL" "$@" > "$out" 2>&1
  printf '%s' "$?"
}

finish() { # mode
  if [ "$FAILED" = "0" ]; then
    echo "TEST_EXIT=0"
    echo "RESULT: PASS"
    exit 0
  fi
  echo "TEST_EXIT=1"
  echo "RESULT: FAIL"
  exit 1
}

case "$ROW" in
r1)
  OUT="$TMP/r1.txt"
  RC="$(run_tool "$OUT" --fixture "$FIXTURE_DIR")"
  sed -n '1,200p' "$OUT"
  expect TOOL_EXIT 0 "$RC"
  expect TENANT_MISMATCH 2 "$(token_of "$OUT" TENANT_MISMATCH)"
  expect ORG_MISMATCH 2 "$(token_of "$OUT" ORG_MISMATCH)"
  expect NIL_UUID 1 "$(token_of "$OUT" NIL_UUID)"
  expect MISSING_LOCAL 1 "$(token_of "$OUT" MISSING_LOCAL)"
  expect ORPHAN_LOCAL 1 "$(token_of "$OUT" ORPHAN_LOCAL)"
  expect ROLE_MISMATCH 2 "$(token_of "$OUT" ROLE_MISMATCH)"
  expect ROLE_UNVERIFIED 1 "$(token_of "$OUT" ROLE_UNVERIFIED)"
  expect CONTEXT_AMBIGUOUS 1 "$(token_of "$OUT" CONTEXT_AMBIGUOUS)"
  expect MEMBER_ONLY 1 "$(token_of "$OUT" MEMBER_ONLY)"
  expect ROLE_REPAIR_SUPPORTED 0 "$(token_of "$OUT" ROLE_REPAIR_SUPPORTED)"
  expect DB_WRITES 0 "$(token_of "$OUT" DB_WRITES)"
  expect RESULT_PASS_LINES 1 "$(grep -c '^RESULT: PASS$' "$OUT")"

  # 计数恒等式：六类可修复/可报告差异之和 + 两类只计数（CONTEXT_AMBIGUOUS/MEMBER_ONLY）
  # = 明细行数；明细按 iam_sub 唯一，一用户一行。
  SIX=$(( $(token_of "$OUT" TENANT_MISMATCH) + $(token_of "$OUT" ORG_MISMATCH) + \
          $(token_of "$OUT" NIL_UUID) + $(token_of "$OUT" MISSING_LOCAL) + \
          $(token_of "$OUT" ORPHAN_LOCAL) + $(token_of "$OUT" ROLE_MISMATCH) ))
  SUM=$(( SIX + $(token_of "$OUT" ROLE_UNVERIFIED) + \
          $(token_of "$OUT" CONTEXT_AMBIGUOUS) + $(token_of "$OUT" MEMBER_ONLY) ))
  expect COUNT_SUM_MATCHES_ROWS "$(token_of "$OUT" DETAIL_COUNT)" "$SUM"
  expect SIX_CLASS_ROWS "$SIX" "$(grep -cE '^DETAIL .*category=(TENANT_MISMATCH|ORG_MISMATCH|NIL_UUID|MISSING_LOCAL|ORPHAN_LOCAL|ROLE_MISMATCH) ' "$OUT")"

  # --json 与文本口径一致 + rows 按 iam_sub 升序
  JSON_OUT="$TMP/r1.json"
  JRC="$(run_tool "$JSON_OUT" --fixture "$FIXTURE_DIR" --json --limit 0)"
  expect JSON_EXIT 0 "$JRC"
  JSON_MATCH=0
  if jq -e . "$JSON_OUT" >/dev/null 2>&1; then
    mismatch=0
    for name in RECONCILE_DATE RECONCILE_SOURCE USERS_LOCAL USERS_IAM_ACTIVE \
                TENANT_MISMATCH ORG_MISMATCH NIL_UUID MISSING_LOCAL ORPHAN_LOCAL \
                ROLE_MISMATCH ROLE_UNVERIFIED CONTEXT_AMBIGUOUS MEMBER_ONLY \
                IAM_MISSING ORG_UNRESOLVED ROLE_REPAIR_SUPPORTED DB_WRITES; do
      [ "$(jq -r --arg k "$name" '.tokens[$k] // ""' "$JSON_OUT")" = "$(token_of "$OUT" "$name")" ] || mismatch=1
    done
    [ "$mismatch" = "0" ] && JSON_MATCH=1
  fi
  expect JSON_TOKENS_MATCH 1 "$JSON_MATCH"

  SORTED=0
  if jq -e . "$JSON_OUT" >/dev/null 2>&1; then
    ids="$(jq -r '.rows[].iam_sub' "$JSON_OUT")"
    if [ "$ids" = "$(printf '%s\n' "$ids" | LC_ALL=C sort)" ]; then SORTED=1; fi
    [ "$(jq -r '.rows | length' "$JSON_OUT")" = "$SUM" ] || FAILED=1
  fi
  expect ROWS_SORTED 1 "$SORTED"

  # 顾客零 UUID 不得判为 NIL_UUID；多上下文不得判 TENANT/ORG
  CUSTOMER_HIT="$(jq -r '[.rows[] | select(.iam_sub == "u-customer")] | length' "$JSON_OUT" 2>/dev/null || echo 1)"
  expect CUSTOMER_ZERO_UUID_NOT_FLAGGED 0 "$CUSTOMER_HIT"
  MULTI_HIT="$(jq -r '[.rows[] | select(.iam_sub == "u-context-multi" and (.category == "TENANT_MISMATCH" or .category == "ORG_MISMATCH"))] | length' "$JSON_OUT" 2>/dev/null || echo 1)"
  expect MULTI_CONTEXT_NOT_FLAGGED_AS_CONTEXT_DIFF 0 "$MULTI_HIT"

  # 父链成环：不得死循环，且计入 ORG_UNRESOLVED
  CYCLE_IAM="$TMP/iam-cycle.tsv"
  CYCLE_LOCAL="$TMP/local-cycle.tsv"
  printf 'u-cycle\taaaa1111-0000-0000-0000-000000000000\tSTAFF\tsite_member\t1\taaaa1111-0000-0000-0000-000000000000\n' > "$CYCLE_IAM"
  printf 'u-cycle\tlocal-cycle\taaaa1111-0000-0000-0000-000000000000\taaaa1111-0000-0000-0000-000000000000\tsite_member\tactive\n' > "$CYCLE_LOCAL"
  CYCLE_OUT="$TMP/r1-cycle.txt"
  CYCLE_RC="$(run_tool "$CYCLE_OUT" --iam-fixture "$CYCLE_IAM" --local-fixture "$CYCLE_LOCAL")"
  expect ORG_CYCLE_EXIT 0 "$CYCLE_RC"
  expect ORG_CYCLE_UNRESOLVED 1 "$(token_of "$CYCLE_OUT" ORG_UNRESOLVED)"
  finish r1
  ;;

r2)
  # 1) 源码静态：零写语句 + 只读闸门存在
  FORBIDDEN="$(grep -nEi '(^|[;&|[:space:]])(insert|update|delete|alter|drop|truncate)[[:space:]]' "$TOOL" 2>/dev/null | wc -l | tr -d ' ')"
  expect FORBIDDEN_SQL_STMTS 0 "${FORBIDDEN:-1}"
  GUARD="$(grep -c 'default_transaction_read_only' "$TOOL" 2>/dev/null | tr -d ' ')"
  expect READONLY_DSN_GUARD 1 "$([ "${GUARD:-0}" -ge 1 ] && echo 1 || echo 0)"

  # 2) 畸形 fixture → exit 3 且报告字段数，不得跳过继续
  BAD_IAM="$TMP/iam-bad.tsv"
  printf 'u-bad\torg-id\tSTAFF\t\tsite_member\n' > "$BAD_IAM"
  BAD_OUT="$TMP/r2-bad.txt"
  BAD_RC="$(run_tool "$BAD_OUT" --iam-fixture "$BAD_IAM" --local-fixture "$FIXTURE_DIR/local.tsv")"
  expect BAD_FIXTURE_EXIT 3 "$BAD_RC"
  expect BAD_FIXTURE_FIELDS_REPORTED 1 "$([ "$(grep -c '^ROW_FIELDS=' "$BAD_OUT")" -ge 1 ] && echo 1 || echo 0)"
  expect BAD_FIXTURE_NO_PASS 0 "$(grep -c '^RESULT: PASS$' "$BAD_OUT" | tr -d ' ')"
  grep -E '^ROW_FIELDS=|^FIXTURE_INVALID=|^RESULT: FAIL' "$BAD_OUT" || true

  # 2b) 非法枚举（is_active=2）→ exit 3
  BAD_ENUM="$TMP/iam-enum.tsv"
  printf 'u-enum\torg-id\tSTAFF\t\t2\t\n' > "$BAD_ENUM"
  ENUM_OUT="$TMP/r2-enum.txt"
  ENUM_RC="$(run_tool "$ENUM_OUT" --iam-fixture "$BAD_ENUM" --local-fixture "$FIXTURE_DIR/local.tsv")"
  expect BAD_ENUM_EXIT 3 "$ENUM_RC"

  # 3) 库不可达 → exit 2 + RESULT: FAIL（绝不冒充 0 差异）
  UN_OUT="$TMP/r2-unreachable.txt"
  UN_RC="$(BEACONIAM_DB_NAME=beaconiam_reachable TUNELOOP_DB_NAME=tuneloop_reachable \
           BEACONIAM_DB_HOST=127.0.0.1 BEACONIAM_DB_PORT=1 \
           TUNELOOP_DB_HOST=127.0.0.1 TUNELOOP_DB_PORT=1 \
           bash -c "$(printf 'bash %q' "$TOOL")" > "$UN_OUT" 2>&1; echo $?)"
  expect UNREACHABLE_EXIT 2 "$UN_RC"
  expect UNREACHABLE_RESULT_FAIL 1 "$([ "$(grep -c '^RESULT: FAIL' "$UN_OUT")" -ge 1 ] && echo 1 || echo 0)"
  sed -n '1,10p' "$UN_OUT"

  # 4) env 缺失 → exit 2
  ME_OUT="$TMP/r2-missing-env.txt"
  ME_RC="$(env -u BEACONIAM_DB_NAME -u BEACONIAM_DB -u TUNELOOP_DB_NAME -u TUNELOOP_DB \
           bash "$TOOL" > "$ME_OUT" 2>&1; echo $?)"
  expect MISSING_ENV_EXIT 2 "$ME_RC"
  expect MISSING_ENV_RESULT_FAIL 1 "$([ "$(grep -c '^RESULT: FAIL' "$ME_OUT")" -ge 1 ] && echo 1 || echo 0)"

  # 5) 失败路径不得输出「0 差异」形态（USERS_LOCAL=0 且 RESULT: PASS）
  FALSE_ZERO=0
  for f in "$BAD_OUT" "$ENUM_OUT" "$UN_OUT" "$ME_OUT"; do
    if grep -q '^RESULT: PASS$' "$f"; then FALSE_ZERO=1; fi
    if grep -q '^USERS_LOCAL=0$' "$f" && grep -q '^DB_WRITES=0$' "$f"; then FALSE_ZERO=1; fi
  done
  expect FALSE_ZERO_DIFF 0 "$FALSE_ZERO"
  finish r2
  ;;

r3)
  # 一次性 PostgreSQL（scripts/lib-2341-env.sh 口径），绝不使用 jobmaster-postgres。
  # shellcheck disable=SC1091
  . scripts/lib-2341-env.sh
  require_pg

  DB="reconcile_2623_$$"
  PORT="${POSTGRES_PORT:-15433}"
  PSQL=("$PSQL_BIN" -h "$POSTGRES_HOST" -p "$PORT" -U "$POSTGRES_USER" -v ON_ERROR_STOP=1)
  export PGPASSWORD="$POSTGRES_PASSWORD"

  psql_admin "CREATE DATABASE $DB" >/dev/null 2>&1 || { echo "FATAL: cannot create test database" >&2; exit 2; }
  DBCLEAN=1
  drop_db() {
    if [ "${DBCLEAN:-0}" = "1" ]; then
      psql_admin "DROP DATABASE IF EXISTS $DB" >/dev/null 2>&1 || true
    fi
  }
  trap 'drop_db; cleanup' EXIT

  "${PSQL[@]}" -d "$DB" -q -c "
    CREATE TABLE users (
      id uuid PRIMARY KEY,
      iam_sub varchar(255),
      tenant_id uuid NOT NULL,
      org_id uuid NOT NULL,
      role varchar(50),
      status varchar(20),
      email varchar(255),
      deleted_at timestamptz
    );
  " >/dev/null || { echo "FATAL: cannot create users table" >&2; exit 2; }

  # 5 行：1 行 #2620 形态（tenant_id=平台组织，IAM=商户 ADMIN）+ 3 行一致 + 1 行空 iam_sub
  "${PSQL[@]}" -d "$DB" -q -c "
    INSERT INTO users (id, iam_sub, tenant_id, org_id, role, status, email) VALUES
      ('11111111-0000-0000-0000-000000000001','2623-a','33333333-3333-3333-3333-333333333333','11111111-1111-1111-1111-111111111111','merchant_admin','active','a-2623@example.test'),
      ('11111111-0000-0000-0000-000000000002','2623-b','11111111-1111-1111-1111-111111111111','22222222-2222-2222-2222-222222222222','site_member','active','b-2623@example.test'),
      ('11111111-0000-0000-0000-000000000003','2623-c','11111111-1111-1111-1111-111111111111','11111111-1111-1111-1111-111111111111','merchant_admin','active','c-2623@example.test'),
      ('11111111-0000-0000-0000-000000000004','2623-d','11111111-1111-1111-1111-111111111111','22222222-2222-2222-2222-222222222222','site_member','active','d-2623@example.test'),
      ('11111111-0000-0000-0000-000000000005','','00000000-0000-0000-0000-000000000000','00000000-0000-0000-0000-000000000000','USER','active','e-2623@example.test');
  " >/dev/null || { echo "FATAL: cannot seed users" >&2; exit 2; }

  # IAM 侧用 fixture（本单不接触预生产/生产库）：2623-a 为商户 ADMIN（期望 tenant=merchant-root）
  IAM_FX="$TMP/iam-live.tsv"
  cat > "$IAM_FX" <<'TSV'
2623-a	11111111-1111-1111-1111-111111111111	ADMIN	site_admin	1	
2623-b	22222222-2222-2222-2222-222222222222	STAFF	site_member	1	11111111-1111-1111-1111-111111111111
2623-c	11111111-1111-1111-1111-111111111111	OWNER	merchant_admin	1	
2623-d	22222222-2222-2222-2222-222222222222	STAFF	site_member	1	11111111-1111-1111-1111-111111111111
TSV

  ROWS_BEFORE="$("${PSQL[@]}" -d "$DB" -Atc 'SELECT count(*) FROM users')"

  LIVE_OUT="$TMP/r3-live.txt"
  LIVE_RC="$(TUNELOOP_DB_HOST="$POSTGRES_HOST" TUNELOOP_DB_PORT="$PORT" \
             TUNELOOP_DB_USER="$POSTGRES_USER" TUNELOOP_DB_PASSWORD="$POSTGRES_PASSWORD" \
             TUNELOOP_DB_NAME="$DB" \
             bash "$TOOL" --iam-fixture "$IAM_FX" > "$LIVE_OUT" 2>&1; echo $?)"
  sed -n '1,40p' "$LIVE_OUT"
  expect LIVE_EXIT 0 "$LIVE_RC"
  expect TENANT_MISMATCH 1 "$(token_of "$LIVE_OUT" TENANT_MISMATCH)"
  expect RECONCILE_SOURCE_TOKEN "live" "$(token_of "$LIVE_OUT" RECONCILE_SOURCE)"
  expect EXIT_0_WITH_DIFF 1 "$([ "$LIVE_RC" = "0" ] && [ "$(token_of "$LIVE_OUT" TENANT_MISMATCH)" = "1" ] && echo 1 || echo 0)"

  ROWS_AFTER="$("${PSQL[@]}" -d "$DB" -Atc 'SELECT count(*) FROM users')"
  expect DB_WRITES 0 "$([ "$ROWS_BEFORE" = "$ROWS_AFTER" ] && echo 0 || echo 1)"

  # #2620 故障形态必须被识别为 TENANT_MISMATCH 并出现在明细里
  LIVE_JSON="$TMP/r3-live.json"
  JRC2="$(TUNELOOP_DB_HOST="$POSTGRES_HOST" TUNELOOP_DB_PORT="$PORT" \
          TUNELOOP_DB_USER="$POSTGRES_USER" TUNELOOP_DB_PASSWORD="$POSTGRES_PASSWORD" \
          TUNELOOP_DB_NAME="$DB" \
          bash "$TOOL" --iam-fixture "$IAM_FX" --json --limit 0 > "$LIVE_JSON" 2>&1; echo $?)"
  expect LIVE_JSON_EXIT 0 "$JRC2"
  expect CASE_2620_DETECTED 1 "$(jq -r '[.rows[] | select(.iam_sub == "2623-a" and .category == "TENANT_MISMATCH")] | length' "$LIVE_JSON" 2>/dev/null || echo 0)"
  expect CASE_2620_EMAIL '"a-2623@example.test"' "$(jq -r '.rows[] | select(.iam_sub == "2623-a") | .email' "$LIVE_JSON" 2>/dev/null | head -1 | jq -R -s 'gsub("\n";"")' 2>/dev/null || echo '""')"

  # 只读事务证明：同一会话设置只读后写操作必须被拒绝
  RO_OUT="$TMP/r3-readonly.txt"
  if "${PSQL[@]}" -d "$DB" -Atc "SET default_transaction_read_only = on" \
       -c "INSERT INTO users (id, iam_sub, tenant_id, org_id, role, status) VALUES ('11111111-0000-0000-0000-0000000000ff','ro-probe','11111111-1111-1111-1111-111111111111','11111111-1111-1111-1111-111111111111','site_member','active')" > "$RO_OUT" 2>&1; then
    READONLY_OK=0
  else
    if grep -qi 'read-only transaction' "$RO_OUT"; then READONLY_OK=1; else READONLY_OK=0; fi
  fi
  expect READONLY_TXN_OK 1 "$READONLY_OK"
  sed -n '1,5p' "$RO_OUT"

  # D9 双路径一致性：把库里的本地行导出为同格式 TSV，再跑全 fixture，token 必须逐项相同
  DUMP="$TMP/local-from-db.tsv"
  "${PSQL[@]}" -d "$DB" -At -F $'\t' \
    -c "SELECT COALESCE(iam_sub,''), id::text, tenant_id::text, org_id::text, COALESCE(role,''), COALESCE(status,'') FROM users WHERE deleted_at IS NULL ORDER BY COALESCE(iam_sub,''), id" \
    > "$DUMP"
  PARITY_OUT="$TMP/r3-parity.txt"
  PRC="$(bash "$TOOL" --iam-fixture "$IAM_FX" --local-fixture "$DUMP" > "$PARITY_OUT" 2>&1; echo $?)"
  expect PARITY_EXIT 0 "$PRC"
  PARITY=1
  for name in USERS_LOCAL USERS_IAM_ACTIVE TENANT_MISMATCH ORG_MISMATCH NIL_UUID \
              MISSING_LOCAL ORPHAN_LOCAL ROLE_MISMATCH ROLE_UNVERIFIED CONTEXT_AMBIGUOUS \
              MEMBER_ONLY IAM_MISSING ORG_UNRESOLVED; do
    [ "$(token_of "$LIVE_OUT" "$name")" = "$(token_of "$PARITY_OUT" "$name")" ] || PARITY=0
  done
  expect FIXTURE_PARITY 1 "$PARITY"
  finish r3
  ;;

*)
  echo "usage: $0 <r1|r2|r3>" >&2
  exit 2
  ;;
esac
