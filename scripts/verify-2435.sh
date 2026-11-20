#!/usr/bin/env bash
# #2435：实名通过补建 second_doc 批次 + UI 兜底 + requeue 排除已清退 + rejected 重开。
# 用法：bash scripts/verify-2435.sh <r1-go|r2-smoke|r3-probe|r4-requeue|r5-rejected>
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1-go) ART="docs/evidence/2435/R1-go.txt"; TEST='^(TestFaceReview_Registration_(AutoEnqueuesSecondDoc|NoRequeueWhenSecondDocVerified)_2435|TestFaceReview_Registration_ApprovesWithoutSecondDocType|TestFaceReview_SecondDoc_(RequiresType|ApproveAssignsType))$' ;;
  r2-smoke) ART="docs/evidence/2435/R2-smoke.txt" ;;
  r3-probe) ART="docs/evidence/2435/R3-probe.txt" ;;
  r4-requeue) ART="docs/evidence/2435/R4-requeue.txt"; TEST='^TestRequeueSecondDocBatches' ;;
  r5-rejected) ART="docs/evidence/2435/R5-rejected.txt"; TEST='^TestFaceReview_Registration_RejectedBatchAllowsOneNewOpen_2435$' ;;
  *) echo "usage: $0 <r1-go|r2-smoke|r3-probe|r4-requeue|r5-rejected>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:${PATH}"
if [ -f "$REPO_ROOT/backend/.env" ]; then
  set -a; . "$REPO_ROOT/backend/.env"; set +a
fi
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_verify2435}"
export IAM_NAMESPACE="test-ns"
export IAM_SECRET="test-secret-1489"
export TUNELOOP_DB_SIMPLE_PROTOCOL=1
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE" "$(dirname "$ART")"

BUILD="1.0.$(git rev-parse --short HEAD)"
COMMIT="$(git rev-parse HEAD)"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2435/' || true)" ]; then
  DIRTY=true
fi
PROVENANCE="# build=$BUILD commit=$COMMIT dirty=$DIRTY"

case "$MODE" in
  r1-go|r4-requeue|r5-rejected)
    {
      echo "$PROVENANCE"
      ( cd "$REPO_ROOT/backend" && go test ./handlers/ -run "$TEST" -count=1 -v )
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r2-smoke)
    {
      echo "$PROVENANCE"
      ( cd "$REPO_ROOT/frontend-mobile" && npm run smoke )
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r3-probe)
    {
      echo "$PROVENANCE"
      fail=0
      grep -q "未定（待平台核定）" "$REPO_ROOT/frontend-mobile/src/pages-weapp/profile/edit/EditProfile.jsx" \
        && echo "UI_OK: 未定（待平台核定）" || { echo "UI_MISSING"; fail=1; }
      grep -q "ensureSecondDocBatch(tx, batch.UserID)" "$REPO_ROOT/backend/handlers/face_review.go" \
        && echo "CALLSITE_OK: ensureSecondDocBatch(tx, batch.UserID)" || { echo "CALLSITE_MISSING"; fail=1; }
      grep -q "id_photo_other_verified" "$REPO_ROOT/backend/handlers/face_review.go" \
        && echo "SELECT_OK: id_photo_other_verified" || { echo "SELECT_MISSING"; fail=1; }
      [ "$fail" = "0" ] && echo "PROBE-OK" || echo "PROBE-FAIL"
      exit "$fail"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
esac
