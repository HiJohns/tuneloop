#!/usr/bin/env bash
# R4: run DB removed, template kept.
set -euo pipefail
source scripts/lib-2341-env.sh
require_pg
echo "PROVENANCE $PROVENANCE"
( cd backend && go test -v ./handlers/ -run '^TestDepositTiers_2321_Single$' -count=1 -timeout 8m >/dev/null 2>&1)
RUN_EXISTS=$(psql_admin "SELECT 1 FROM pg_database WHERE datname='$TUNELOOP_TEST_DB'")
TPL_EXISTS=$(psql_admin "SELECT 1 FROM pg_database WHERE datname='tuneloop_test_template'")
test -z "$RUN_EXISTS" || { echo "run DB leaked"; exit 1; }
test -n "$TPL_EXISTS" || { echo "template missing"; exit 1; }
echo "R4 CLEAN PASS"
