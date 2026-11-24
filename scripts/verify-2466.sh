#!/usr/bin/env bash
# #2466 证据驱动：R1–R8 定向用例、R9 gate/plan-source、R10 构建与回归。
set -uo pipefail
MODE="${1:-}"
case "$MODE" in
  r1) NAME="R1-same-org.txt"; TEST='^TestSiteTransfer_SameOrg_LocalOnly_2466$'; MARK='SITE_ID=B ORG_UNCHANGED=1 IAM_CALLS=0' ;;
  r2) NAME="R2-cross-org.txt"; TEST='^TestSiteTransfer_CrossOrg_Staff_2466$'; MARK='BIND_ROLE=STAFF' ;;
  r3) NAME="R3-admin.txt"; TEST='^TestSiteTransfer_CrossOrg_Admin_2466$'; MARK='BIND_ROLE=ADMIN' ;;
  r4) NAME="R4-existing-target.txt"; TEST='^TestSiteTransfer_TargetAlreadyMember_2466$'; MARK='ROWS_AFTER=1' ;;
  r5) NAME="R5-new-target.txt"; TEST='^TestSiteTransfer_NewTarget_2466$'; MARK='ROWS_AFTER=2' ;;
  r6) NAME="R6-iam-fail.txt"; TEST='^TestSiteTransfer_IAMBindFail_NoHalfState_2466$'; MARK='STATUS=502' ;;
  r7) NAME="R7-template-fail.txt"; TEST='^TestSiteTransfer_TemplateFail_Explicit_2466$'; MARK='MSG_CONTAINS=切换网点失败' ;;
  r8) NAME="R8-readback.txt"; TEST='^TestSiteTransfer_Readback_2466$'; MARK='LIST_SITE_MATCH=1' ;;
  r9) NAME="R9-gate.txt" ;;
  r10) NAME="R10-build.txt" ;;
  *) echo "usage: $0 <r1..r10>" >&2; exit 2 ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO"
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/2466}"; mkdir -p "$OUT_DIR" "$REPO/tmp/gobuild" "$REPO/tmp/gocache"
ART="$OUT_DIR/$NAME"
export PATH="/usr/local/go/bin:${PATH}"
export TMPDIR="${TMPDIR:-$REPO/tmp/gobuild}" GOCACHE="${GOCACHE:-$REPO/tmp/gocache}"
if [ -f "$REPO/backend/.env" ]; then set -a; . "$REPO/backend/.env"; set +a; fi
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2466}" IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}" IAM_SECRET="${IAM_SECRET:-test-secret-1489}" TUNELOOP_DB_SIMPLE_PROTOCOL=1

HEAD_SHA="$(git rev-parse HEAD)"
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
DIRTY=false; [ -n "$(git status --porcelain -uall)" ] && DIRTY=true
PROV="# build=1.0.$FIX_SHORT commit=$FIX_SHA dirty=$DIRTY tree=$HEAD_SHA"

case "$MODE" in
  r1|r2|r3|r4|r5|r6|r7|r8)
    { echo "$PROV"; ( cd backend && go test ./handlers -run "$TEST" -count=1 -v ); } 2>&1 | tee "$ART"
    rc=${PIPESTATUS[0]}
    grep -qF -- "$MARK" "$ART" || { echo "MARK_MISSING[$MARK]"; rc=1; }
    exit "$rc" ;;
  r10)
    { echo "$PROV"; echo "BUILD_CMD=go build ./..."; ( cd backend && go build ./... ); build_rc=$?; echo "BUILD_EXIT=$build_rc"
      ( cd backend && go test ./handlers -run '_2466$' -count=1 -v ) 2>&1 | tee /tmp/opencode/2466-r10-tests.log
      test_rc=${PIPESTATUS[0]}
      fails=$(grep -c '^--- FAIL' /tmp/opencode/2466-r10-tests.log || true)
      echo "TESTS_FAIL=$fails"
      [ "$build_rc" = "0" ] && [ "$test_rc" = "0" ] && [ "$fails" = "0" ] && echo "BUILD_OK" || echo "BUILD_FAIL"
    } 2>&1 | tee "$ART"
    grep -q '^BUILD_OK$' "$ART"; exit $? ;;
  r9)
    { echo "$PROV"
      fail=0
      GATE="$REPO/docs/evidence/2466/pre-ready-2466.json"
      if [ -f "$GATE" ] && jq -e '.result == "PASS"' "$GATE" >/dev/null 2>&1; then
        echo "GATE_RESULT=PASS"
      else
        echo "GATE_RESULT=MISSING_OR_FAIL"; fail=1
      fi
      p5=$(jq -r '.checks[]? | select(.id=="P5") | .status' "$GATE" 2>/dev/null | head -1)
      echo "P5=$p5"
      [ -n "$p5" ] && [ "$p5" != "N/A" ] || { echo "P5_IS_NA"; fail=1; }
      bash "$HOME/.config/opencode/scripts/plan-source.sh" --issue 2466 2>&1 | tail -5 | tee /tmp/opencode/2466-r9-plan-source.log
      grep -q 'PLAN_SOURCE: PASS' /tmp/opencode/2466-r9-plan-source.log || { echo "PLAN_SOURCE=FAIL"; fail=1; }
      if [ -f "$REPO/docs/evidence/2466/full-gate.log" ] && grep -q 'FULL_SUITE_EXIT=0' "$REPO/docs/evidence/2466/full-gate.log"; then
        echo "FULL_SUITE_EXIT=0"
      else
        echo "FULL_SUITE_EXIT=MISSING"; fail=1
      fi
      [ "$fail" = "0" ] && echo "GATE_OK" || echo "GATE_FAIL"
    } 2>&1 | tee "$ART"
    grep -q '^GATE_OK$' "$ART"; exit $? ;;
esac
