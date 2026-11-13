#!/usr/bin/env bash
# R3: stale meta stamp → rebuild, new stamp persisted.
set -euo pipefail
source scripts/lib-2341-env.sh
require_pg
echo "PROVENANCE $PROVENANCE"
( cd backend && FORCE_TEMPLATE_REBUILD=1 go test ./handlers/ -run '^$' -count=1 -timeout 8m >/dev/null 2>&1 || true)
PGPASSWORD="$POSTGRES_PASSWORD" "$PSQL_BIN" -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d tuneloop_test_template \
  -c "UPDATE _test_template_meta SET seed_hash='stale' WHERE id=1"
OUT=$( cd backend && go test ./handlers/ -run '^TestDepositTiers_2321_Single$' -count=1 -timeout 8m 2>&1)
printf '%s\n' "$OUT"
printf '%s\n' "$OUT" | grep -q "Built test template tuneloop_test_template"
NEW=$(PGPASSWORD="$POSTGRES_PASSWORD" "$PSQL_BIN" -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d tuneloop_test_template -tAc "SELECT seed_hash FROM _test_template_meta WHERE id=1")
test -n "$NEW" && test "$NEW" != "stale"
echo "R3 STALE-REBUILD PASS"
