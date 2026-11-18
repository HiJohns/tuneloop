#!/usr/bin/env bash
# #2405 邀请接受按 kind 落库验收。
set -euo pipefail
MODE="${1:-}"
case "$MODE" in r1-merchant-staff|r2-technician|r3-idempotent|r4-site-regression|r6-cross-merchant|r7-cross-tenant-tech|build) ;; *) echo "usage: $0 <mode>" >&2; exit 2 ;; esac
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:${PATH}"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_verify2405}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
mkdir -p "$TMPDIR" "$GOCACHE"
cd "$REPO_ROOT/backend"
case "$MODE" in
  r1-merchant-staff) go test ./handlers/ -run '^TestAcceptInviteMerchantStaff_2405$' -count=1 -v ;;
  r2-technician)     go test ./handlers/ -run '^TestAcceptInviteTechnician_2405$' -count=1 -v ;;
  r3-idempotent)     go test ./handlers/ -run '^TestAcceptInviteIdempotent_2405$' -count=1 -v ;;
  r4-site-regression) go test ./handlers/ -run '^TestAcceptInviteSiteRegression_2405$' -count=1 -v ;;
  r6-cross-merchant) go test ./handlers/ -run '^TestAcceptInviteCrossMerchant_2405$' -count=1 -v ;;
  r7-cross-tenant-tech) go test ./handlers/ -run '^TestAcceptInviteCrossTenantTechnician_2405$' -count=1 -v ;;
  build)             go build ./... ;;
esac
