#!/usr/bin/env bash
# #2342 R4: the four migration-class tests must pass on the strict template.
set -euo pipefail
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2342}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TMPDIR="${TMPDIR:-$PWD/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$PWD/tmp/gocache}"
export PATH="/usr/local/go/bin:$PATH"
mkdir -p docs/evidence/2342
PROVENANCE="build=migrations-fresh-install commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- backend scripts)" && echo false || echo true)"
# psql 定位：优先 PATH，其次本机历史路径（审计环境 PGBIN=/usr/bin 覆盖）。
if [ -z "${PGBIN:-}" ]; then
  if command -v psql >/dev/null 2>&1; then
    PGBIN="$(dirname "$(command -v psql)")"
  else
    PGBIN="/home/coder/opt/pg/bin"
  fi
fi
# 非交互传播密码（meta_dump 等后续调用不得再次索要密码）。
export PGPASSWORD="${PGPASSWORD:-$POSTGRES_PASSWORD}"
{
  echo "PROVENANCE $PROVENANCE"
  echo "CMD go test ./handlers/ -run '^(TestCentsMigration_20260820001|TestGiftPolicyReferralPresetMigration|TestPromoOverride_CheckConstraintMigration|TestPromoOverride_CrossMerchantForbidden_2260)$' -count=1 -v"
  ( cd backend && go test ./handlers/ -run '^(TestCentsMigration_20260820001|TestGiftPolicyReferralPresetMigration|TestPromoOverride_CheckConstraintMigration|TestPromoOverride_CrossMerchantForbidden_2260)$' -count=1 -v )
} 2>&1 | tee docs/evidence/2342/R4-migration-tests.txt
echo "R4 PASS: 4/4"
