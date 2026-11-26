#!/usr/bin/env bash
# #2532 verification harness — one subcommand per Evidence Contract row.
#
#   bash scripts/verify-2532.sh r1    # SubmitAssessment fallback writes sites.id (red-green)
#   bash scripts/verify-2532.sh r2    # unresolvable org keeps the previous value
#   bash scripts/verify-2532.sh r3    # primary branch unchanged
#   bash scripts/verify-2532.sh r4    # same-class scan + build + targeted regression
#   bash scripts/verify-2532.sh gate  # gate entry (build + targeted regression)
#   bash scripts/verify-2532.sh red   # R1 red run: revert the fix, expect the R1 test to FAIL
#   bash scripts/verify-2532.sh ledger # derive build/commit/dirty + R5 actual into evidence.json
#
# Capture a row into its artifact:
#   bash scripts/verify-2532.sh r1 > docs/evidence/2532/R1-assessment-fallback.txt
#   bash scripts/verify-2532.sh red > docs/evidence/2532/R1-red-prefix.txt
#
# Every row artifact starts with the provenance header consumed by
# scripts/evidence-artifacts.sh (P14):
#   # build=1.0.<short-sha> # commit=<full-sha> # dirty=<true|false>
# The `red` artifact is not part of the Evidence Contract artifact set (it
# documents the pre-fix failure), so it carries its own pre-fix provenance.
#
# `ledger` is the only writer of evidence.json's build/commit/dirty and of the
# R5 `actual` cell: both are derived from the latest `fix(issue-2532)` commit
# and from the committed gate report, never typed by hand.
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r2|r3|r4|gate|red|ledger) ;;
  *) echo "usage: $0 r1|r2|r3|r4|gate|red|ledger" >&2; exit 2 ;;
esac

ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not a git repository" >&2; exit 2; }
cd "$ROOT" || exit 2

if ! command -v go >/dev/null 2>&1 && [ -x /usr/local/go/bin/go ]; then
  export PATH="/usr/local/go/bin:$PATH"
fi

# Backend integration tests need the Postgres test database
# (docs/ops/test-db-lifecycle.md). Overridable for other hosts.
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_local}"
export TMPDIR="${TMPDIR:-$ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"

# Provenance anchors the issue's fix commit, not HEAD: the evidence bundle is
# committed after the fix, and the plan requires the artifacts to keep pointing
# at the fix commit so a later re-run (e.g. /fixreview) does not silently move
# the anchor. Override with FIX_COMMIT=<sha> if needed.
FIX_SHA="${FIX_COMMIT:-$(git log --format=%H --grep='^fix(issue-2532)' -1 2>/dev/null)}"
ANCHOR_SHA="${FIX_SHA:-$(git rev-parse HEAD)}"
SHORT_SHA="$(git rev-parse --short "$ANCHOR_SHA")"
BUILD="1.0.${SHORT_SHA}"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence' ':!tmp')" ]; then
  DIRTY=true
fi
case "$ROW" in
  r1|r2|r3|r4|gate) echo "# build=${BUILD} # commit=${ANCHOR_SHA} # dirty=${DIRTY}" ;;
esac

fail() {
  case "$ROW" in
    r1|r2|r3|r4) echo "R${ROW#r}_FAIL: $*" >&2 ;;
    *) echo "FAIL(${ROW:-?}): $*" >&2 ;;
  esac
  exit 1
}

HEAVY="$HOME/.config/opencode/scripts/heavy-run.sh"
ASSESSMENT="backend/handlers/assessment.go"

heavy() { bash "$HEAVY" -- "$@"; }

go_test_row() { # test-name → runs one backend evidence test, echoes its token lines
  local name="$1" out rc
  out="$(mktemp)"
  ( cd backend && heavy go test ./handlers -run "${name}" -count=1 -v ) > "$out" 2>&1
  rc=$?
  grep -E '^(STATUS|OPERATOR_SITE|RESOLVED_IS_SITE|CURRENT_IS_SITE|CURRENT_NOT_ORG|OLD_VALUE_IS_ORG|RED_CONFIRMED|KEPT_ORIGINAL|NO_CROSS_DOMAIN_WRITE|RESOLVED_EMPTY|OLD_WOULD_WRITE_ORG|PRIMARY_EQUALS_OPERATOR_SITE|PRIMARY_NOT_ORG)=' "$out" || true
  grep -E '^--- (PASS|FAIL)' "$out" || true
  if [ "$rc" -ne 0 ]; then
    echo "--- go test output (tail) ---"
    tail -25 "$out"
    rm -f "$out"
    fail "go test ${name} exit=${rc}"
  fi
  rm -f "$out"
}

need_token() { # token file
  grep -qx -- "$1" "$2" || fail "missing expected token '$1'"
}

# ledger_r5_actual prints the truthful R5 `actual` cell derived from the gate
# report (never hand-written).
ledger_r5_actual() { # gate-report
  local report="$1"
  jq -r --arg src "$report" '
    "\(.overall)：overall=\(.overall) / plan_sha256=\(.plan_sha256) / provenance.build=\(.provenance.build) / provenance.commit=\(.provenance.commit) / dirty=\(.provenance.dirty) / P12=\(([.checks[] | select(.id == "P12")][0].status) // "missing") / P13=\(([.checks[] | select(.id == "P13")][0].status) // "missing")（source: \($src)，ledger 派生）"
  ' "$report"
}

case "$ROW" in
  r1)
    echo "== R1 SubmitAssessment fallback writes sites.id (red-green) =="
    OUT="$(mktemp)"; go_test_row TestSubmitAssessment2532_FallbackWritesSiteNotOrg > "$OUT" 2>&1
    cat "$OUT"
    need_token "STATUS=200" "$OUT"
    need_token "OPERATOR_SITE=none" "$OUT"
    need_token "RESOLVED_IS_SITE=1" "$OUT"
    need_token "CURRENT_IS_SITE=1" "$OUT"
    need_token "CURRENT_NOT_ORG=1" "$OUT"
    need_token "OLD_VALUE_IS_ORG=1" "$OUT"
    need_token "RED_CONFIRMED=1" "$OUT"
    rm -f "$OUT"
    echo "R1_OK"
    ;;

  r2)
    echo "== R2 unresolvable org keeps the previous value =="
    OUT="$(mktemp)"; go_test_row TestSubmitAssessment2532_UnresolvableKeepsValue > "$OUT" 2>&1
    cat "$OUT"
    need_token "STATUS=200" "$OUT"
    need_token "KEPT_ORIGINAL=1" "$OUT"
    need_token "NO_CROSS_DOMAIN_WRITE=1" "$OUT"
    need_token "RESOLVED_EMPTY=1" "$OUT"
    need_token "OLD_WOULD_WRITE_ORG=1" "$OUT"
    rm -f "$OUT"
    echo "R2_OK"
    ;;

  r3)
    echo "== R3 primary branch unchanged =="
    OUT="$(mktemp)"; go_test_row TestSubmitAssessment2532_PrimaryBranchUnchanged > "$OUT" 2>&1
    cat "$OUT"
    need_token "STATUS=200" "$OUT"
    need_token "PRIMARY_EQUALS_OPERATOR_SITE=1" "$OUT"
    need_token "PRIMARY_NOT_ORG=1" "$OUT"
    rm -f "$OUT"
    echo "R3_OK"
    ;;

  r4)
    echo "== R4 same-class scan + build + targeted regression =="
    [ -f "$ASSESSMENT" ] || fail "missing $ASSESSMENT"

    # Defect signature: cross-domain write of the order's org into current_site_id.
    ASSESSMENT_ORG_ASSIGN="$(grep -cE 'current_site_id"\] = .*OrgID' "$ASSESSMENT" || true)"
    TREE_ORG_ASSIGN="$(grep -rhE 'current_site_id"\] = .*OrgID' backend/handlers/ 2>/dev/null | wc -l | tr -d ' ')"
    # The shared resolver (#2511) must be consumed here, never re-implemented.
    RESOLVER_CALL="$(grep -c 'resolveSiteIDByOrg(h.db, order.OrgID)' "$ASSESSMENT" || true)"
    LOCAL_RESOLVER_DEF="$(grep -cE '^func resolve(Site|Operator)' "$ASSESSMENT" || true)"
    WARN_LINE_PRESENT="$(grep -c '#2532: no site resolved' "$ASSESSMENT" || true)"

    TMP_BUILD="$(mktemp)"
    ( cd backend && heavy go build ./... ) > "$TMP_BUILD" 2>&1
    BUILD_RC=$?
    tail -5 "$TMP_BUILD"
    rm -f "$TMP_BUILD"

    TMP_REG="$(mktemp)"
    ( cd backend && heavy go test ./handlers -count=1 -v \
      -run 'TestSubmitAssessment2532|TestSubmitAssessment_Damaged_SetsCurrentSiteID|TestResolveOperatorSiteID|TestInspectReturn_|TestAssessDamage_|TestInspectReturn2511|TestAssessDamage2511|TestResolveSiteIDByOrg2511' \
      ) > "$TMP_REG" 2>&1
    REG_RC=$?
    REG_FAIL="$(grep -c '^--- FAIL' "$TMP_REG" || true)"
    REG_PASS="$(grep -cE '^--- PASS' "$TMP_REG" || true)"
    grep -E '^--- (PASS|FAIL)' "$TMP_REG" | tail -16 || true
    tail -3 "$TMP_REG"
    rm -f "$TMP_REG"

    echo "ASSESSMENT_ORG_ASSIGN=${ASSESSMENT_ORG_ASSIGN:-0}"
    echo "TREE_ORG_ASSIGN=${TREE_ORG_ASSIGN:-0}"
    echo "RESOLVER_CALL=${RESOLVER_CALL:-0}"
    echo "LOCAL_RESOLVER_DEF=${LOCAL_RESOLVER_DEF:-0}"
    echo "WARN_LINE_PRESENT=${WARN_LINE_PRESENT:-0}"
    echo "BUILD_RC=${BUILD_RC}"
    echo "REG_RC=${REG_RC}"
    echo "REG_PASS_COUNT=${REG_PASS:-0}"
    echo "REG_FAIL=${REG_FAIL:-0}"

    [ "${ASSESSMENT_ORG_ASSIGN:-1}" = "0" ] || fail "assessment.go still writes order.OrgID into current_site_id"
    [ "${TREE_ORG_ASSIGN:-1}" = "0" ] || fail "another handler still writes order.OrgID into current_site_id (#2511 not integrated?)"
    [ "${RESOLVER_CALL:-0}" = "1" ] || fail "expected exactly one resolveSiteIDByOrg call site, got ${RESOLVER_CALL:-0}"
    [ "${LOCAL_RESOLVER_DEF:-1}" = "0" ] || fail "assessment.go must not define its own resolver"
    [ "${WARN_LINE_PRESENT:-0}" = "1" ] || fail "missing #2532 WARN line for the unresolvable branch"
    [ "$BUILD_RC" -eq 0 ] || fail "go build ./... failed (rc=${BUILD_RC})"
    [ "$REG_RC" -eq 0 ] || fail "targeted regression failed (rc=${REG_RC}, fails=${REG_FAIL:-0})"
    [ "${REG_FAIL:-1}" = "0" ] || fail "targeted regression reported FAIL lines (${REG_FAIL})"
    echo "R4_OK"
    ;;

  gate)
    echo "== gate: build + targeted regression =="
    TMP_BUILD="$(mktemp)"
    ( cd backend && heavy go build ./... ) > "$TMP_BUILD" 2>&1
    BUILD_RC=$?
    rm -f "$TMP_BUILD"

    TMP_REG="$(mktemp)"
    ( cd backend && heavy go test ./handlers -count=1 \
      -run 'TestSubmitAssessment2532|TestSubmitAssessment_Damaged_SetsCurrentSiteID|TestResolveOperatorSiteID|TestInspectReturn_|TestAssessDamage_|TestInspectReturn2511|TestAssessDamage2511|TestResolveSiteIDByOrg2511' \
      ) > "$TMP_REG" 2>&1
    REG_RC=$?
    REG_FAIL="$(grep -c '^--- FAIL' "$TMP_REG" || true)"
    tail -3 "$TMP_REG"
    rm -f "$TMP_REG"

    echo "BUILD_RC=${BUILD_RC}"
    echo "REG_RC=${REG_RC}"
    echo "REG_FAIL=${REG_FAIL:-0}"
    [ "$BUILD_RC" -eq 0 ] || fail "go build ./... failed (rc=${BUILD_RC})"
    [ "$REG_RC" -eq 0 ] || fail "targeted regression failed (rc=${REG_RC})"
    echo "GATE_OK"
    ;;

  red)
    # Pre-fix red run: the R1 assertion must FAIL against the pre-fix
    # expression. The fix commit's parent holds the pre-fix handler, so this is
    # the same tree as "stash the fix away and re-run R1" — but reproducible and
    # durably capturable.
    [ -n "$FIX_SHA" ] || fail "cannot locate the fix(issue-2532) commit"
    PRE_SHA="$(git rev-parse "${FIX_SHA}^")"
    PRE_SHORT="$(git rev-parse --short "$PRE_SHA")"
    echo "# build=1.0.${PRE_SHORT} # commit=${PRE_SHA} # dirty=false"
    echo "# red-run: pre-fix source ${ASSESSMENT} @ ${PRE_SHA} (fix ${FIX_SHA} reverted in the working tree for this run)"
    echo "== R1 red run: pre-fix SubmitAssessment fallback writes order.OrgID =="

    BACKUP="$(mktemp)"
    cp "$ASSESSMENT" "$BACKUP" || fail "cannot back up $ASSESSMENT"
    restore_assessment() { cp "$BACKUP" "$ASSESSMENT"; rm -f "$BACKUP"; }
    trap 'restore_assessment' EXIT INT TERM
    git show "${PRE_SHA}:${ASSESSMENT}" > "$ASSESSMENT" || {
      restore_assessment; trap - EXIT INT TERM
      fail "cannot materialize the pre-fix source"
    }

    OUT="$(mktemp)"
    ( cd backend && heavy go test ./handlers -run TestSubmitAssessment2532_FallbackWritesSiteNotOrg -count=1 -v ) > "$OUT" 2>&1
    RED_RC=$?
    grep -E '^(STATUS|OPERATOR_SITE|RESOLVED_IS_SITE|CURRENT_IS_SITE|CURRENT_NOT_ORG|OLD_VALUE_IS_ORG|RED_CONFIRMED)=' "$OUT" || true
    grep -E '^--- (PASS|FAIL)' "$OUT" || true
    echo "--- go test output (tail) ---"
    tail -15 "$OUT"
    rm -f "$OUT"

    restore_assessment
    trap - EXIT INT TERM

    echo "RED_EXIT=${RED_RC}"
    [ "$RED_RC" -ne 0 ] || fail "pre-fix source unexpectedly passed the R1 assertion"
    echo "RED_OK"
    ;;

  ledger)
    # Single writer for evidence.json provenance + the R5 `actual` cell.
    echo "== ledger: derive build/commit/dirty + R5 actual into evidence.json =="
    LEDGER_FILE="docs/evidence/2532/evidence.json"
    GATE_REPORT="docs/evidence/2532/pre-ready-2532.json"
    [ -f "$LEDGER_FILE" ] || fail "missing $LEDGER_FILE"
    command -v jq >/dev/null 2>&1 || fail "jq is required"
    [ -f "$GATE_REPORT" ] || fail "missing gate report $GATE_REPORT; run the gate first"
    jq -e . "$GATE_REPORT" >/dev/null 2>&1 || fail "gate report is not valid JSON: $GATE_REPORT"

    GATE_OVERALL="$(jq -r '.overall // empty' "$GATE_REPORT")"
    [ "$GATE_OVERALL" = "PASS" ] || fail "gate report overall=${GATE_OVERALL:-missing}; the ledger only records a PASS gate"
    GATE_COMMIT="$(jq -r '.provenance.commit // empty' "$GATE_REPORT")"
    [ "$GATE_COMMIT" = "$ANCHOR_SHA" ] || fail "gate report provenance.commit=${GATE_COMMIT:-missing} does not match the fix commit ${ANCHOR_SHA}"
    R5_COUNT="$(jq -r '[.rows[] | select(.id == "R5")] | length' "$LEDGER_FILE")"
    [ "$R5_COUNT" = "1" ] || fail "expected exactly one R5 row in $LEDGER_FILE, got ${R5_COUNT:-0}"

    R5_ACTUAL="$(ledger_r5_actual "$GATE_REPORT")" || fail "cannot derive the R5 actual from $GATE_REPORT"
    [ -n "$R5_ACTUAL" ] || fail "derived R5 actual is empty"

    TMP_LEDGER="$(mktemp)"
    if ! jq --arg build "$BUILD" --arg commit "$ANCHOR_SHA" --arg actual "$R5_ACTUAL" \
         --argjson dirty "$([ "$DIRTY" = "true" ] && echo true || echo false)" \
         '.build = $build
          | .commit = $commit
          | .dirty = $dirty
          | .rows = (.rows | map(if .id == "R5" then .actual = $actual else . end))' \
         "$LEDGER_FILE" > "$TMP_LEDGER"; then
      rm -f "$TMP_LEDGER"
      fail "jq rewrite failed"
    fi
    mv "$TMP_LEDGER" "$LEDGER_FILE"

    echo "build=${BUILD}"
    echo "commit=${ANCHOR_SHA}"
    echo "dirty=${DIRTY}"
    echo "R5_ACTUAL=${R5_ACTUAL}"
    echo "ROWS=$(jq -r '.rows | length' "$LEDGER_FILE") PASS_ROWS=$(jq -r '[.rows[] | select(.pass == true)] | length' "$LEDGER_FILE")"
    echo "LEDGER_OK"
    ;;
esac
