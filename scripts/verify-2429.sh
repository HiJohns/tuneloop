#!/usr/bin/env bash
# #2429：编辑员工字段规则 + org 级站点幂等 + 多网点/双身份 + 人工回执。
# 用法：bash scripts/verify-2429.sh <r1-go|r2-pc-build|r3-probe|r4-manual|r5-multisite|r6-dual>
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1-go) ART="docs/evidence/2429/R1-go.txt" ;;
  r2-pc-build) ART="docs/evidence/2429/R2-pc-build.txt" ;;
  r3-probe) ART="docs/evidence/2429/R3-probe.txt" ;;
  r4-manual) ART="docs/evidence/2429/R4-manual.md" ;;
  r5-multisite) ART="docs/evidence/2429/R5-multisite.txt" ;;
  r6-dual) ART="docs/evidence/2429/R6-dual.txt" ;;
  *) echo "usage: $0 <r1-go|r2-pc-build|r3-probe|r4-manual|r5-multisite|r6-dual>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:${PATH}"
if [ -f "$REPO_ROOT/backend/.env" ]; then
  set -a; . "$REPO_ROOT/backend/.env"; set +a
fi
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_verify2429}"
export IAM_NAMESPACE="test-ns"
export IAM_SECRET="test-secret-1489"
export TUNELOOP_DB_SIMPLE_PROTOCOL=1
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE" "$(dirname "$ART")"

BUILD="1.0.$(git rev-parse --short HEAD)"
COMMIT="$(git rev-parse HEAD)"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2429/' || true)" ]; then
  DIRTY=true
fi
PROVENANCE="# build=$BUILD commit=$COMMIT dirty=$DIRTY"

case "$MODE" in
  r1-go)
    {
      echo "$PROVENANCE"
      ( cd "$REPO_ROOT/backend" && go test ./handlers/ -run 'TestUpdateUser_(SameSiteSkipsIAM|MultiSiteSameOrgSkipsIAM)_2429|TestGetStaffUser_ReturnsSiteAndDirectFlag_2429' -count=1 -v )
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r5-multisite)
    {
      echo "$PROVENANCE"
      ( cd "$REPO_ROOT/backend" && go test ./handlers/ -run 'TestUpdateUser_MultiSiteSameOrgSkipsIAM_2429|TestGetStaffUser_MultiSiteDual_2429' -count=1 -v )
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r6-dual)
    {
      echo "$PROVENANCE"
      ( cd "$REPO_ROOT/frontend-pc" && node --test src/utils/staffForm.test.js )
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r2-pc-build)
    {
      echo "$PROVENANCE"
      ( cd "$REPO_ROOT/frontend-pc" && NODE_OPTIONS="--max-old-space-size=2048" npm run build )
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r3-probe)
    {
      echo "$PROVENANCE"
      fail=0
      chk() { if eval "$1"; then echo "OK: $2"; else echo "MISSING: $2"; fail=1; fi; }
      chk "! grep -q 'siteIDChanged' backend/handlers/user_staff.go" "siteIDChanged removed"
      chk "! grep -q 'currentSiteID' backend/handlers/user_staff.go" "single-site Scan removed"
      chk "grep -q 'newOrgID != oldOrgID' backend/handlers/user_staff.go" "org-level compare"
      chk "grep -q 'site_memberships' backend/handlers/user_staff.go" "memberships returned"
      chk "grep -q 'func (h \\*UserStaffHandler) GetStaffUser' backend/handlers/user_staff.go" "GetStaffUser present"
      chk "grep -q '/admin/staff/:id' backend/main.go" "route registered"
      chk "grep -q 'staffApi.getUser(id)' frontend-pc/src/pages/StaffEdit.jsx" "edit loads by id"
      chk "grep -q 'AutoComplete' frontend-pc/src/pages/StaffEdit.jsx" "position AutoComplete"
      chk "grep -q 'resolveStaffFormMode' frontend-pc/src/pages/StaffEdit.jsx" "staffForm helper used"
      chk "! grep -q 'title=\"编辑员工\"' frontend-pc/src/pages/StaffManagement.jsx" "legacy modal removed"
      [ "$fail" = "0" ] && echo "PROBE-OK" || echo "PROBE-FAIL"
      exit "$fail"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r4-manual)
    if tail -5 docs/evidence/2429/R4-manual.md | grep -q '^RESULT: PASS'; then
      echo "MANUAL_OK: docs/evidence/2429/R4-manual.md"
      exit 0
    fi
    echo "MANUAL_PENDING: docs/evidence/2429/R4-manual.md（Owner 未签）"
    exit 1
    ;;
esac
