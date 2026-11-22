#!/usr/bin/env bash
# #2435 Rev.3 evidence driver.
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r1-registration-type) NAME="R1-registration-type.txt" ;;
  r2|r2-put-type) NAME="R2-put-type.txt" ;;
  r3|r3-render) NAME="R3-render.txt" ;;
  r4|r4-save) NAME="R4-save.txt" ;;
  r5|r5-pc-detail) NAME="R5-pc-detail.txt" ;;
  r6|r6-filter) NAME="R6-filter.txt" ;;
  r7|r7-review) NAME="R7-review.txt" ;;
  r8|r8-docs) NAME="R8-docs.txt" ;;
  r9|r9-gate-baseline) NAME="R9-full-gate.txt" ;;
  *) echo "usage: $0 <r1..r9>" >&2; exit 2 ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO"
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/2435}"
mkdir -p "$OUT_DIR" "$REPO/tmp/gobuild" "$REPO/tmp/gocache"
ART="$OUT_DIR/$NAME"
FIX_REF="${FIX_REF:-HEAD}"
FIX_SHA="$(git rev-parse "$FIX_REF")"; FIX_SHORT="$(git rev-parse --short "$FIX_REF")"
HEAD_SHA="$(git rev-parse --short HEAD)"
BUILD="1.0.$FIX_SHORT"
COMMIT="$FIX_SHA"
DIRTY=false
[ -n "$(git status --porcelain -uall)" ] && DIRTY=true
PROV="# build=$BUILD commit=$COMMIT dirty=$DIRTY tree=$HEAD_SHA"

node22() {
  if [ -s "$HOME/.nvm/nvm.sh" ]; then
    # shellcheck disable=SC1090
    source "$HOME/.nvm/nvm.sh"; nvm use 22 >/dev/null 2>&1 || true
  fi
}
backend() {
  cd "$REPO/backend"
  export TMPDIR="${TMPDIR:-$REPO/tmp/gobuild}" GOCACHE="${GOCACHE:-$REPO/tmp/gocache}"
  set -a; . ./.env; set +a
  export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2435_vr}" \
         IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}" IAM_SECRET="${IAM_SECRET:-test-secret-2435}"
}

case "${MODE%%-*}" in
  r1)
    { echo "$PROV"; backend; go test ./handlers/ -run 'TestRegistrationSecondDocType_Persisted' -count=1 -v; } 2>&1 | tee "$ART"; exit "${PIPESTATUS[0]}" ;;
  r2)
    { echo "$PROV"; backend; go test ./handlers/ -run 'TestUpdateCurrentUser_SecondDocType_2435' -count=1 -v; } 2>&1 | tee "$ART"; exit "${PIPESTATUS[0]}" ;;
  r3)
    { echo "$PROV"; node22; cd "$REPO/frontend-mobile"; npx vitest run --config vitest.config.js scripts/page-smoke/second-doc-type-fallback-2435.test.jsx -t 'R3'; } 2>&1 | tee "$ART"
    fail=0
    for marker in R3_STATE_1_OK R3_STATE_2_OK R3_STATE_3_OK R3_STATE_4_OK; do
      if grep -q "$marker" "$ART"; then echo "R3_MARKER_OK $marker"; else echo "R3_MARKER_MISSING $marker"; fail=1; fi
    done
    [ "$fail" -eq 0 ] && echo "R3_RENDER_OK" || echo "R3_RENDER_FAIL"
    exit "$fail" ;;
  r4)
    { echo "$PROV"; node22; cd "$REPO/frontend-mobile"; npx vitest run --config vitest.config.js scripts/page-smoke/second-doc-type-fallback-2435.test.jsx -t 'R4'; } 2>&1 | tee "$ART"
    fail=0
    for marker in R4_BODY_WITH_TYPE_OK R4_NO_TYPE_OMITTED_OK R4_NO_PHOTO_OMITTED_OK; do
      if grep -q "$marker" "$ART"; then echo "R4_MARKER_OK $marker"; else echo "R4_MARKER_MISSING $marker"; fail=1; fi
    done
    [ "$fail" -eq 0 ] && echo "R4_SAVE_OK" || echo "R4_SAVE_FAIL"
    exit "$fail" ;;
  r5)
    {
      echo "$PROV"
      cd "$REPO/frontend-pc"
      f=src/pages/System/UserManagement.jsx
      fail=0
      grep -q "SECOND_DOC_LABELS" "$f" && echo "LABELS_MAP_OK" || { echo "LABELS_MAP_MISSING"; fail=1; }
      for pair in "student: '学生证'" "teacher: '教职工证'" "work: '工作证'" "other: '其他'"; do
        if grep -Fq "$pair" "$f"; then echo "LABEL_OK $pair"; else echo "LABEL_MISSING $pair"; fail=1; fi
      done
      grep -q "idPhotos.other &&" "$f" && echo "OTHER_GATE_OK" || { echo "OTHER_GATE_MISSING"; fail=1; }
      grep -Fq "类型未定：待审核，用户可自报，审核员可修正" "$f" && echo "PENDING_TEXT_OK" || { echo "PENDING_TEXT_MISSING"; fail=1; }
      grep -q "readOnly" "$f" && echo "READONLY_OK" || { echo "READONLY_MISSING"; fail=1; }
      npm run build
      rc=$?
      echo "PC_BUILD_EXIT=$rc"
      [ "$rc" = "0" ] && echo "PC_BUILD_OK" || fail=1
      [ "$fail" = "0" ] && echo "R5_STRUCTURE_BUILD_OK"
      exit "$fail"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}" ;;
  r6)
    { echo "$PROV"; backend; go test ./handlers/ -run 'TestUserManagement_SecondDocUnsetFilter_2435' -count=1 -v; } 2>&1 | tee "$ART"; exit "${PIPESTATUS[0]}" ;;
  r7)
    { echo "$PROV"; backend; go test ./handlers/ -run 'TestFaceReview_Registration_AutoEnqueuesSecondDoc_2435|TestRequeueSecondDocBatches' -count=1 -v; } 2>&1 | tee "$ART"; exit "${PIPESTATUS[0]}" ;;
  r8)
    { echo "$PROV"; grep -q '用户自报' docs/cases/id-photos.md; grep -q '待审核' docs/cases/user-management.md; grep -q '四态' docs/spec/ui/ui.md; grep -q 'id_photo_other_type' docs/spec/api/api.md; echo "DOCS_OK"; } 2>&1 | tee "$ART"; exit "${PIPESTATUS[0]}" ;;
  r9|r9-gate-baseline)
    { echo "$PROV"; backend; go build ./... && go test ./handlers/ -count=1 -timeout 15m; } 2>&1 | tee "$ART"; exit "${PIPESTATUS[0]}" ;;
esac
