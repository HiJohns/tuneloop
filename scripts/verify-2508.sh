#!/usr/bin/env bash
# #2508 verification harness — one subcommand per Evidence Contract row.
#
#   bash scripts/verify-2508.sh r1   # ID-domain root cause (old predicate 0 rows / new 2 rows)
#   bash scripts/verify-2508.sh r2   # site_admin list main symptom
#   bash scripts/verify-2508.sh r3   # cross-org isolation (negative)
#   bash scripts/verify-2508.sh r4   # old keying gone (negative)
#   bash scripts/verify-2508.sh r5   # multi-site org + site_id intersection
#   bash scripts/verify-2508.sh r6   # personnel suite regression
#   bash scripts/verify-2508.sh r7   # go build + same-class ID-domain scan
#
# Every run writes docs/evidence/2508/R<row>-*.txt whose first line is the
# provenance header consumed by scripts/evidence-artifacts.sh (P14):
#   # build=1.0.<short-sha> # commit=<full-sha> # dirty=<true|false>
#
# TUNELOOP_TEST_ALLOW_SKIP is intentionally NOT set: a skipped DB test is a red
# (the fixture must run against the real Postgres test database).
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r2|r3|r4|r5|r6|r7) ;;
  *) echo "usage: $0 r1|r2|r3|r4|r5|r6|r7" >&2; exit 2 ;;
esac

ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not a git repository" >&2; exit 2; }
cd "$ROOT" || exit 2

export PATH="/usr/local/go/bin:${PATH}"
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_DB="${TUNELOOP_DB:-tuneloop_test}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TMPDIR="${TMPDIR:-$ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE" docs/evidence/2508

HEAD_SHA="$(git rev-parse HEAD)"
SHORT_SHA="$(git rev-parse --short HEAD)"
BUILD="1.0.${SHORT_SHA}"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence')" ]; then
  DIRTY=true
fi
PROVENANCE="# build=${BUILD} # commit=${HEAD_SHA} # dirty=${DIRTY}"

fail() { echo "R${ROW#r} FAIL: $*" >&2; exit 1; }

# need <artifact> <literal marker>
need() {
  grep -qF -- "$2" "$1" || fail "missing marker '$2' in $1"
}

OUT=""

case "$ROW" in
  r1)
    OUT="docs/evidence/2508/R1-id-domain.txt"
    {
      echo "$PROVENANCE"
      echo "== R1 ID-domain root cause (fixture: sites.id != sites.org_id) =="
      ( cd backend && go test ./handlers -run '^TestPersonnel2508_IDDomainRootCause$' -count=1 -v -timeout 10m )
    } > "$OUT" 2>&1
    RC=$?
    [ "$RC" -eq 0 ] || fail "go test exit=$RC (see $OUT)"
    need "$OUT" "--- PASS: TestPersonnel2508_IDDomainRootCause"
    need "$OUT" "BUGGY_ROWS=0"
    need "$OUT" "FIXED_ROWS=2"
    need "$OUT" "RED_CONFIRMED=1"
    ;;

  r2)
    OUT="docs/evidence/2508/R2-site-view.txt"
    {
      echo "$PROVENANCE"
      echo "== R2 site_admin list (actor oid = sites.org_id) =="
      ( cd backend && go test ./handlers -run '^TestPersonnel2508_SiteView$' -count=1 -v -timeout 10m )
    } > "$OUT" 2>&1
    RC=$?
    [ "$RC" -eq 0 ] || fail "go test exit=$RC (see $OUT)"
    need "$OUT" "--- PASS: TestPersonnel2508_SiteView"
    need "$OUT" "SITE_VIEW_ROWS=2"
    need "$OUT" "HAS_SITE_A_MEMBER=1"
    need "$OUT" "HAS_ZERO_TENANT=1"
    need "$OUT" "LEAK_SITE_B=0"
    need "$OUT" "LEAK_TECH=0"
    need "$OUT" "LEAK_PLATFORM=0"
    need "$OUT" "LEAK_OTHER_TENANT=0"
    ;;

  r3)
    OUT="docs/evidence/2508/R3-isolation.txt"
    {
      echo "$PROVENANCE"
      echo "== R3 cross-org isolation (org A vs org B actors) =="
      ( cd backend && go test ./handlers -run '^TestPersonnel2508_CrossOrgIsolation$' -count=1 -v -timeout 10m )
    } > "$OUT" 2>&1
    RC=$?
    [ "$RC" -eq 0 ] || fail "go test exit=$RC (see $OUT)"
    need "$OUT" "--- PASS: TestPersonnel2508_CrossOrgIsolation"
    need "$OUT" "A_ONLY=1"
    need "$OUT" "B_ONLY=1"
    need "$OUT" "CROSS_LEAK=0"
    ;;

  r4)
    OUT="docs/evidence/2508/R4-old-behavior-gone.txt"
    {
      echo "$PROVENANCE"
      echo "== R4 old keying gone (actor oid = sites.id) =="
      ( cd backend && go test ./handlers -run '^TestPersonnel2508_OldKeyingGone$' -count=1 -v -timeout 10m )
    } > "$OUT" 2>&1
    RC=$?
    [ "$RC" -eq 0 ] || fail "go test exit=$RC (see $OUT)"
    need "$OUT" "--- PASS: TestPersonnel2508_OldKeyingGone"
    need "$OUT" "OLD_KEYING_ROWS=0"
    need "$OUT" "STATUS=200"
    ;;

  r5)
    OUT="docs/evidence/2508/R5-multisite-filter.txt"
    {
      echo "$PROVENANCE"
      echo "== R5 multi-site org (sites.org_id has 2 sites) + site_id intersection =="
      ( cd backend && go test ./handlers -run '^TestPersonnel2508_MultiSiteFilter$' -count=1 -v -timeout 10m )
    } > "$OUT" 2>&1
    RC=$?
    [ "$RC" -eq 0 ] || fail "go test exit=$RC (see $OUT)"
    need "$OUT" "--- PASS: TestPersonnel2508_MultiSiteFilter"
    need "$OUT" "MULTI_ROWS=3"
    need "$OUT" "FILTER_A_ROWS=2"
    need "$OUT" "FILTER_B_ROWS=0"
    need "$OUT" "INVALID_SITE_CODE=40002"
    ;;

  r6)
    OUT="docs/evidence/2508/R6-regression.txt"
    {
      echo "$PROVENANCE"
      echo "== R6 personnel suite regression (2065/2067/2072/2427/2508) =="
      ( cd backend && go test ./handlers -run 'TestPersonnel' -count=1 -v -timeout 15m )
    } > "$OUT" 2>&1
    RC=$?
    FAIL_COUNT="$(grep -c '^--- FAIL' "$OUT")"
    PASS_COUNT="$(grep -c '^--- PASS' "$OUT")"
    REGRESSION_PASS=0
    if [ "$RC" -eq 0 ] && [ "$FAIL_COUNT" -eq 0 ] && [ "$PASS_COUNT" -gt 0 ]; then
      REGRESSION_PASS=1
    fi
    {
      echo "SUMMARY: REGRESSION_PASS=$REGRESSION_PASS FAIL_COUNT=$FAIL_COUNT PASS_COUNT=$PASS_COUNT"
    } >> "$OUT"
    need "$OUT" "--- PASS: TestPersonnel2065_SiteView"
    need "$OUT" "REGRESSION_PASS=1"
    need "$OUT" "FAIL_COUNT=0"
    [ "$REGRESSION_PASS" -eq 1 ] || fail "regression run not green (rc=$RC, fail=$FAIL_COUNT)"
    ;;

  r7)
    OUT="docs/evidence/2508/R7-build-scan.txt"
    {
      echo "$PROVENANCE"
      echo "== R7 go build + same-class ID-domain scan =="
      ( cd backend && go build ./... )
      echo "BUILD_RC=$?"
      echo "-- scan: personnel.go still keys site_members.site_id off the org ID? (expect 0) --"
      PERSONNEL_SITE_ID_MATCH="$(grep -c "sm\.site_id = ? AND sm\.status = 'active'" backend/handlers/personnel.go || true)"
      echo "PERSONNEL_SITE_ID_MATCH=${PERSONNEL_SITE_ID_MATCH:-0}"
      echo "-- same-class defects registered in #2509 (S2..S5, unchanged by this Issue) --"
      S2="$(grep -c 'CurrentSiteID.String() != oid' backend/handlers/instrument_loss.go || true)"
      S3="$(grep -c '"current_site_id"\] = order\.OrgID' backend/handlers/warehouse.go || true)"
      S4="$(grep -cE 'else if in[a-z]*\.SiteID != nil' backend/handlers/user_rental.go || true)"
      S5="$(grep -cE 'site_id = \?.*", orgID\)' backend/handlers/repair_service.go || true)"
      echo "S2=${S2:-0} S3=${S3:-0} S4=${S4:-0} S5=${S5:-0}"
      echo "-- ID-domain source of truth --"
      echo "docs/cases/cases.md:289 $(grep -n 'org_id' docs/cases/cases.md | sed -n '1p')"
    } > "$OUT" 2>&1

    NEED_BUILD="$(sed -n 's/^BUILD_RC=\([0-9][0-9]*\)$/\1/p' "$OUT" | head -1)"
    NEED_MATCH="$(sed -n 's/^PERSONNEL_SITE_ID_MATCH=\([0-9][0-9]*\)$/\1/p' "$OUT" | head -1)"
    NEED_S2="$(sed -n 's/^S2=\([0-9][0-9]*\) S3=\([0-9][0-9]*\) S4=\([0-9][0-9]*\) S5=\([0-9][0-9]*\)$/\1/p' "$OUT" | head -1)"
    NEED_S3="$(sed -n 's/^S2=\([0-9][0-9]*\) S3=\([0-9][0-9]*\) S4=\([0-9][0-9]*\) S5=\([0-9][0-9]*\)$/\2/p' "$OUT" | head -1)"
    NEED_S4="$(sed -n 's/^S2=\([0-9][0-9]*\) S3=\([0-9][0-9]*\) S4=\([0-9][0-9]*\) S5=\([0-9][0-9]*\)$/\3/p' "$OUT" | head -1)"
    NEED_S5="$(sed -n 's/^S2=\([0-9][0-9]*\) S3=\([0-9][0-9]*\) S4=\([0-9][0-9]*\) S5=\([0-9][0-9]*\)$/\4/p' "$OUT" | head -1)"
    [ "$NEED_BUILD" = "0" ] || fail "go build failed (BUILD_RC=${NEED_BUILD:-missing})"
    [ "$NEED_MATCH" = "0" ] || fail "personnel.go still keys site_id off the org ID (${NEED_MATCH:-missing})"
    [ "$NEED_S2" = "1" ] || fail "S2 count drifted (${NEED_S2:-missing} != 1)"
    [ "$NEED_S3" = "2" ] || fail "S3 count drifted (${NEED_S3:-missing} != 2)"
    [ "$NEED_S4" = "2" ] || fail "S4 count drifted (${NEED_S4:-missing} != 2)"
    [ "$NEED_S5" = "2" ] || fail "S5 count drifted (${NEED_S5:-missing} != 2)"
    ;;
esac

echo "R${ROW#r} PASS: $OUT"
