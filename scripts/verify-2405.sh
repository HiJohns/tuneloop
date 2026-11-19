#!/usr/bin/env bash
# #2405 邀请接受按 kind 落库验收（Rev.2：产物带 provenance）。
# 用法：bash scripts/verify-2405.sh <r1-merchant-staff|r2-technician|r3-idempotent|r4-site-regression|r5-build|r6-cross-merchant|r7-cross-tenant-tech>
set -uo pipefail
MODE="${1:-}"
case "$MODE" in
  r1-merchant-staff) ART="docs/evidence/2405/R1-merchant-staff.txt" ;;
  r2-technician) ART="docs/evidence/2405/R2-technician.txt" ;;
  r3-idempotent) ART="docs/evidence/2405/R3-idempotent.txt" ;;
  r4-site-regression) ART="docs/evidence/2405/R4-site-regression.txt" ;;
  r5-build) ART="docs/evidence/2405/R5-build.txt" ;;
  r6-cross-merchant) ART="docs/evidence/2405/R6-cross-merchant.txt" ;;
  r7-cross-tenant-tech) ART="docs/evidence/2405/R7-cross-tenant-tech.txt" ;;
  *) echo "usage: $0 <r1-merchant-staff|r2-technician|r3-idempotent|r4-site-regression|r5-build|r6-cross-merchant|r7-cross-tenant-tech>" >&2; exit 2 ;;
esac
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
mkdir -p "$TMPDIR" "$GOCACHE" "$(dirname "$ART")"
BUILD="1.0.$(git rev-parse --short HEAD)"; COMMIT="$(git rev-parse HEAD)"; DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2405/' || true)" ]; then DIRTY=true; fi
{
  echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"
  cd "$REPO_ROOT/backend"
  case "$MODE" in
    r1-merchant-staff) go test ./handlers/ -run '^TestAcceptInviteMerchantStaff_2405$' -count=1 -v ;;
    r2-technician) go test ./handlers/ -run '^TestAcceptInviteTechnician_2405$' -count=1 -v ;;
    r3-idempotent) go test ./handlers/ -run '^TestAcceptInviteIdempotent_2405$' -count=1 -v ;;
    r4-site-regression) go test ./handlers/ -run '^TestAcceptInviteSiteRegression_2405$' -count=1 -v ;;
    r6-cross-merchant) go test ./handlers/ -run '^TestAcceptInviteCrossMerchant_2405$' -count=1 -v ;;
    r7-cross-tenant-tech) go test ./handlers/ -run '^TestAcceptInviteCrossTenantTechnician_2405$' -count=1 -v ;;
    r5-build) go build ./... && echo "BUILD OK" ;;
  esac
} 2>&1 | tee "$ART"
exit "${PIPESTATUS[0]}"
