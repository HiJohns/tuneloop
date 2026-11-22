#!/usr/bin/env bash
# #2428：撞库检测平台域 + 掩码/标识/跨租户绑定。
set -uo pipefail
MODE="${1:-}"
case "$MODE" in
  r1) ART="docs/evidence/2428/R1-same.txt" ;;
  r2) ART="docs/evidence/2428/R2-cross.txt" ;;
  r3) ART="docs/evidence/2428/R3-iam.txt" ;;
  r4) ART="docs/evidence/2428/R4-privacy.txt" ;;
  r5) ART="docs/evidence/2428/R5-build.txt" ;;
  r6) ART="docs/evidence/2428/R6-bind-guard.txt" ;;
  r7) ART="docs/evidence/2428/R7-identifier.txt" ;;
  r8) ART="docs/evidence/2428/R8-mask.txt" ;;
  r9-gate) ART="docs/evidence/2428/R9-full-gate.txt" ;;
  *) echo "usage: $0 <r1|r2|r3|r4|r5|r6|r7|r8|r9-gate>" >&2; exit 2 ;;
esac
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
if [ -f backend/.env ]; then set -a; . backend/.env; set +a; fi
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_verify2428}" IAM_NAMESPACE=test-ns IAM_SECRET=test-secret-1489 TUNELOOP_DB_SIMPLE_PROTOCOL=1
export PATH="/usr/local/go/bin:${PATH}"
# FIX_COMMIT 默认 HEAD；显式传入时校验它必须是 HEAD 的祖先（证据针对该 fix 生成）。
FIX_COMMIT="${FIX_COMMIT:-$(git rev-parse HEAD)}"
COMMIT="$(git rev-parse "$FIX_COMMIT")" || { echo "FATAL: bad FIX_COMMIT=$FIX_COMMIT" >&2; exit 3; }
git merge-base --is-ancestor "$COMMIT" HEAD || { echo "FATAL: FIX_COMMIT $COMMIT is not an ancestor of HEAD" >&2; exit 3; }
BUILD="1.0.$(git rev-parse --short "$COMMIT")"; DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2428/' || true)" ]; then DIRTY=true; fi
PROVENANCE="# build=$BUILD commit=$COMMIT dirty=$DIRTY"
mkdir -p "$(dirname "$ART")"
case "$MODE" in
  r1)
    { echo "$PROVENANCE"; ( cd backend && go test ./handlers/ -run 'TestCheckUserExistsPlatformScope_2428' -count=1 -v ) | tee /tmp/opencode/2428-scope.log; grep -q 'R1_SAME_OK' /tmp/opencode/2428-scope.log; } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r2)
    { echo "$PROVENANCE"; ( cd backend && go test ./handlers/ -run 'TestCheckUserExistsPlatformScope_2428' -count=1 -v ) | tee /tmp/opencode/2428-cross.log; grep -q 'R2_CROSS' /tmp/opencode/2428-cross.log; } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r3)
    { echo "$PROVENANCE"; ( cd backend && go test ./handlers/ -run 'TestCheckUserExistsIAMFallback_2428' -count=1 -v ); } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r4)
    { echo "$PROVENANCE"; ( cd backend && go test ./handlers/ -run 'TestCheckUserExistsPlatformScope_2428' -count=1 -v ) | tee /tmp/opencode/2428-priv.log; grep -q 'R4_PRIVACY_OK' /tmp/opencode/2428-priv.log; } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r5)
    { echo "$PROVENANCE"; ( cd backend && go build ./... ); ( cd frontend-pc && NODE_OPTIONS="--max-old-space-size=2048" npm run build ); ( cd frontend-pc && npx eslint --quiet src/pages/StaffManagement.jsx src/components/MerchantMemberManagement.jsx src/components/SiteMemberManagement.jsx ); } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r6)
    { echo "$PROVENANCE"; ( cd backend && go test ./handlers/ -run 'TestMerchantAddMemberCrossTenantBlocked_2428' -count=1 -v ); } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r7)
    { echo "$PROVENANCE"; fail=0
      grep -q 'matched_fields' frontend-pc/src/pages/StaffManagement.jsx && echo "STAFF_MATCHED_OK" || fail=1
      grep -q 'same_tenant' frontend-pc/src/components/MerchantMemberManagement.jsx && echo "MERCHANT_GUARD_OK" || fail=1
      grep -q 'same_tenant' frontend-pc/src/components/SiteMemberManagement.jsx && echo "SITE_GUARD_OK" || fail=1
      ! grep -q 'existingUser.phone || existingUser.email' frontend-pc/src/components/SiteMemberManagement.jsx && echo "NO_PLAINTEXT_DEP" || fail=1
      exit "$fail"; } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r8)
    { echo "$PROVENANCE"; ( cd backend && go test ./services/ -run 'ContactMask' -count=1 -v ); } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r9-gate)
    { echo "$PROVENANCE"
      fail=0
      if ( cd backend && go build ./... ); then echo "BUILD_OK"; else echo "BUILD_FAIL"; fail=1; fi
      ( cd backend && go test ./handlers/ -run '_2428' -count=1 -v ) 2>&1 | tee /tmp/opencode/2428-r9-handlers.log
      h_rc=${PIPESTATUS[0]}
      if [ "$h_rc" -eq 0 ] && ! grep -qE '^--- FAIL' /tmp/opencode/2428-r9-handlers.log; then
        echo "HANDLERS_2428_OK"
      else
        echo "HANDLERS_2428_FAIL"
        fail=1
      fi
      ( cd backend && go test ./services/ -run 'ContactMask' -count=1 -v ) 2>&1 | tee /tmp/opencode/2428-r9-services.log
      s_rc=${PIPESTATUS[0]}
      if [ "$s_rc" -eq 0 ] && ! grep -qE '^--- FAIL' /tmp/opencode/2428-r9-services.log; then
        echo "SERVICES_MASK_OK"
      else
        echo "SERVICES_MASK_FAIL"
        fail=1
      fi
      [ "$fail" -eq 0 ] && echo "FULL_GATE_OK" || echo "FULL_GATE_FAIL"
      exit "$fail"; } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
esac
