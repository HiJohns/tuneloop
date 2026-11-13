#!/usr/bin/env bash
# #2336 R3: targeted 14-case suite; also extracts the R7 tenant-negative segment.
set -euo pipefail
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2336}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TMPDIR="${TMPDIR:-$PWD/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$PWD/tmp/gocache}"
export PATH="/usr/local/go/bin:$PATH"
mkdir -p "$TMPDIR" "$GOCACHE" docs/evidence/2336 tmp
PROVENANCE="build=handlers-test-infra commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence')" && echo false || echo true)"
# 14 = 11 planned + 2 order-dependent + R7 tenant-isolation negative.
REGEX='^(TestConfirmationSessionHandler_Confirm|TestDiscountCodeFlow|TestIAMProxyHandler_Issue340_IntegrationFlow|TestCancelOrderByCustomer_StatusGuard|TestGuestBatchCreateOrder|TestOrgManagement|TestPersonnel2067_MembershipAnchorAndDirect|TestPrepayMembershipWithCoupon_OREZ|TestPaymentCallback_RegistrationComplete|TestPaymentCallback_SessionFlow_RealCallback|TestRenewal_Confirm_OpenIDBackfilled|TestStaffOrderActions|TestUpdateUser_SiteIDNoChange|TestPersonnel_MerchantView_TenantIsolation2336)$'
OUT=docs/evidence/2336/R3-targeted.txt
set +e
{
  echo "PROVENANCE $PROVENANCE"
  echo "CMD go test ./handlers/ -run '<14-case exact list>' -count=1 -v -timeout 10m"
  ( cd backend && go test ./handlers/ -run "$REGEX" -count=1 -v -timeout 10m )
} 2>&1 | tee "$OUT"
rc=${PIPESTATUS[0]}
set -e
{
  echo "PROVENANCE $PROVENANCE"
  awk -v t="TestPersonnel_MerchantView_TenantIsolation2336" '
    index($0, "=== RUN   " t) == 1 {f=1}
    f {print}
    f && (index($0, "--- PASS: " t) == 1 || index($0, "--- FAIL: " t) == 1) {exit}
  ' "$OUT"
} > docs/evidence/2336/R7-tenant-negative.txt
if ! grep -q -- "--- PASS: TestPersonnel_MerchantView_TenantIsolation2336" docs/evidence/2336/R7-tenant-negative.txt; then
  echo "R7 extraction failed: tenant-isolation negative did not pass" >&2
  exit 1
fi
exit "$rc"
