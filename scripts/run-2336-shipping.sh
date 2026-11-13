#!/usr/bin/env bash
# #2336 R2: shipping backoff injectable (no real sleep / network).
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
{
  echo "PROVENANCE $PROVENANCE"
  echo "CMD go test ./handlers/ ./services/ -run 'TestWarehouse|TestUploadShipping|TestUpdateShipping|TestReportWechatShipping|TestShippingFlow|TestPaymentCallback.*Shipping' -count=1 -v"
  ( cd backend && go test ./handlers/ ./services/ -run 'TestWarehouse|TestUploadShipping|TestUpdateShipping|TestReportWechatShipping|TestShippingFlow|TestPaymentCallback.*Shipping' -count=1 -v )
} 2>&1 | tee docs/evidence/2336/R2-shipping.txt
