#!/usr/bin/env bash
# R1: fresh lifecycle — no template → build → clone → targeted test → meta stamp.
set -euo pipefail
source scripts/lib-2341-env.sh
require_pg
echo "PROVENANCE $PROVENANCE"
psql_admin "DROP DATABASE IF EXISTS tuneloop_test_template WITH (FORCE)"
psql_admin "DROP DATABASE IF EXISTS $TUNELOOP_TEST_DB WITH (FORCE)"
( cd backend && FORCE_TEMPLATE_REBUILD=1 go test -v ./handlers/ -run '^TestDepositTiers_2321_Single$' -count=1 -timeout 8m 2>&1 | tee /tmp/2341-r1.log)
grep -q "Built test template tuneloop_test_template" /tmp/2341-r1.log
META=$(psql_admin "SELECT model_hash || ':' || seed_hash FROM dblink('dbname=tuneloop_test_template','SELECT model_hash, seed_hash FROM _test_template_meta WHERE id=1') AS t(model_hash text, seed_hash text)" 2>/dev/null || true)
if [ -z "$META" ]; then
  # dblink may be absent; use a direct connection instead
  META=$(PGPASSWORD="$POSTGRES_PASSWORD" "$PSQL_BIN" -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d tuneloop_test_template -tAc "SELECT model_hash || ':' || seed_hash FROM _test_template_meta WHERE id=1")
fi
echo "meta=$META"
test -n "$META"
RUN_EXISTS=$(psql_admin "SELECT 1 FROM pg_database WHERE datname='$TUNELOOP_TEST_DB'")
test -z "$RUN_EXISTS" || { echo "run DB not cleaned"; exit 1; }
echo "R1 PASS"
