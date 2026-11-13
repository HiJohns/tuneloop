#!/usr/bin/env bash
# R2: corrupt template cents column → TestMain must fail fast with a rebuild hint.
set -euo pipefail
source scripts/lib-2341-env.sh
require_pg
echo "PROVENANCE $PROVENANCE"
( cd backend && FORCE_TEMPLATE_REBUILD=1 go test -v ./handlers/ -run '^TestDepositTiers_2321_Single$' -count=1 -timeout 8m >/dev/null 2>&1 || true)
PGPASSWORD="$POSTGRES_PASSWORD" "$PSQL_BIN" -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d tuneloop_test_template \
  -c "ALTER TABLE orders ALTER COLUMN deposit TYPE numeric(10,2) USING deposit::numeric(10,2)"
set +e
OUT=$( cd backend && go test -v ./handlers/ -run '^TestDepositTiers_2321_Single$' -count=1 -timeout 8m 2>&1)
RC=$?
set -e
printf '%s\n' "$OUT"
test "$RC" -ne 0
printf '%s\n' "$OUT" | grep -q "cents schema drifted (expected bigint): orders.deposit=numeric"
printf '%s\n' "$OUT" | grep -q "FORCE_TEMPLATE_REBUILD=1"
psql_admin "DROP DATABASE IF EXISTS tuneloop_test_template WITH (FORCE)" >/dev/null
echo "R2 FAIL-FAST PASS"
