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
}

case "${MODE%%-*}" in
  r1)
    { echo "$PROV"; backend; go test ./handlers/ -run 'TestRegistrationSecondDocType_Persisted' -count=1 -v; } 2>&1 | tee "$ART"; exit "${PIPESTATUS[0]}" ;;
  r2)
    { echo "$PROV"; backend; go test ./handlers/ -run 'TestUpdateCurrentUser_SecondDocType_2435' -count=1 -v; } 2>&1 | tee "$ART"; exit "${PIPESTATUS[0]}" ;;
  r3)
    { echo "$PROV"; node22; cd "$REPO/frontend-mobile"; npx vitest run --config vitest.config.js scripts/page-smoke/second-doc-type-fallback-2435.test.jsx; } 2>&1 | tee "$ART"; exit "${PIPESTATUS[0]}" ;;
  r4)
    { echo "$PROV"; grep -q 'id_photo_other_type: secondDocType' frontend-mobile/src/pages-weapp/profile/edit/EditProfile.jsx; echo "SAVE_FIELD_OK"; } 2>&1 | tee "$ART"; exit "${PIPESTATUS[0]}" ;;
  r5)
    { echo "$PROV"; grep -q '待审核，用户可自报，审核员可修正' frontend-pc/src/pages/System/UserManagement.jsx; grep -q 'SECOND_DOC_LABELS' frontend-pc/src/pages/System/UserManagement.jsx; echo "PC_DETAIL_OK"; } 2>&1 | tee "$ART"; exit "${PIPESTATUS[0]}" ;;
  r6)
    { echo "$PROV"; backend; go test ./handlers/ -run 'TestUserManagement_SecondDocUnsetFilter_2435' -count=1 -v; } 2>&1 | tee "$ART"; exit "${PIPESTATUS[0]}" ;;
  r7)
    { echo "$PROV"; backend; go test ./handlers/ -run 'TestFaceReview_Registration_AutoEnqueuesSecondDoc_2435|TestRequeueSecondDocBatches' -count=1 -v; } 2>&1 | tee "$ART"; exit "${PIPESTATUS[0]}" ;;
  r8)
    { echo "$PROV"; grep -q '用户自报' docs/cases/id-photos.md; grep -q '待审核' docs/cases/user-management.md; grep -q '四态' docs/spec/ui/ui.md; grep -q 'id_photo_other_type' docs/spec/api/api.md; echo "DOCS_OK"; } 2>&1 | tee "$ART"; exit "${PIPESTATUS[0]}" ;;
  r9|r9-gate-baseline)
    { echo "$PROV"; backend; go build ./... && go test ./handlers/ -count=1 -timeout 10m; } 2>&1 | tee "$ART"; exit "${PIPESTATUS[0]}" ;;
esac
