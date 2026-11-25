#!/usr/bin/env bash
# #2511 verification harness — one subcommand per Evidence Contract row.
#
#   bash scripts/verify-2511.sh r1    # InspectReturn fallback writes sites.id (red-green)
#   bash scripts/verify-2511.sh r2    # AssessDamage fallback writes sites.id
#   bash scripts/verify-2511.sh r3    # unresolvable org keeps the previous value
#   bash scripts/verify-2511.sh r4    # resolver determinism + primary branch unchanged
#   bash scripts/verify-2511.sh r5    # same-class scan + read-only stock SQL
#   bash scripts/verify-2511.sh r6    # build + targeted regression
#   bash scripts/verify-2511.sh gate  # full gate entry (r6)
#
# Capture a row into its artifact:
#   bash scripts/verify-2511.sh r1 > docs/evidence/2511/R1-inspect-return.txt
#
# Every artifact starts with the provenance header consumed by
# scripts/evidence-artifacts.sh (P14):
#   # build=1.0.<short-sha> # commit=<full-sha> # dirty=<true|false>
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r2|r3|r4|r5|r6|gate) ;;
  *) echo "usage: $0 r1|r2|r3|r4|r5|r6|gate" >&2; exit 2 ;;
esac

ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not a git repository" >&2; exit 2; }
cd "$ROOT" || exit 2

if ! command -v go >/dev/null 2>&1 && [ -x /usr/local/go/bin/go ]; then
  export PATH="/usr/local/go/bin:$PATH"
fi

HEAD_SHA="$(git rev-parse HEAD)"
SHORT_SHA="$(git rev-parse --short HEAD)"
BUILD="1.0.${SHORT_SHA}"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence')" ]; then
  DIRTY=true
fi
echo "# build=${BUILD} # commit=${HEAD_SHA} # dirty=${DIRTY}"

fail() { echo "R${ROW#r}_FAIL: $*" >&2; exit 1; }

HEAVY="$HOME/.config/opencode/scripts/heavy-run.sh"
WAREHOUSE="backend/handlers/warehouse.go"

heavy() { bash "$HEAVY" -- "$@"; }

go_test_row() { # test-name → runs one backend evidence test, echoes its token lines
  local name="$1" out rc
  out="$(mktemp)"
  ( cd backend && heavy go test ./handlers -run "${name}" -count=1 -v ) > "$out" 2>&1
  rc=$?
  grep -E '^(CURRENT_SITE_ID|OLD_FALLBACK_IS_ORG|RED_CONFIRMED|NOT_ORG|KEPT_ORIGINAL|NO_CROSS_DOMAIN_WRITE|EARLIEST_FALLBACK|ANCHOR_PREFERRED|EMPTY_FOR_UNKNOWN|PRIMARY_EQUALS_OPERATOR_SITE|PRIMARY_NOT_ORG)=' "$out" || true
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

case "$ROW" in
  r1)
    echo "== R1 InspectReturn fallback writes sites.id =="
    OUT="$(mktemp)"; go_test_row TestInspectReturn2511_FallbackWritesSiteNotOrg > "$OUT" 2>&1
    cat "$OUT"
    need_token "CURRENT_SITE_ID=aID" "$OUT"
    need_token "OLD_FALLBACK_IS_ORG=1" "$OUT"
    need_token "RED_CONFIRMED=1" "$OUT"
    rm -f "$OUT"
    echo "SITE_ID_IS=aID"
    echo "R1_OK"
    ;;

  r2)
    echo "== R2 AssessDamage fallback writes sites.id =="
    OUT="$(mktemp)"; go_test_row TestAssessDamage2511_FallbackWritesSiteNotOrg > "$OUT" 2>&1
    cat "$OUT"
    need_token "CURRENT_SITE_ID=aID" "$OUT"
    need_token "NOT_ORG=1" "$OUT"
    rm -f "$OUT"
    echo "R2_OK"
    ;;

  r3)
    echo "== R3 unresolvable org keeps the previous value =="
    OUT="$(mktemp)"; go_test_row TestInspectReturn2511_UnresolvableKeepsValue > "$OUT" 2>&1
    cat "$OUT"
    need_token "CURRENT_SITE_ID=aID" "$OUT"
    need_token "KEPT_ORIGINAL=1" "$OUT"
    need_token "NO_CROSS_DOMAIN_WRITE=1" "$OUT"
    rm -f "$OUT"
    echo "R3_OK"
    ;;

  r4)
    echo "== R4 resolver determinism + primary branch unchanged =="
    OUT="$(mktemp)"; go_test_row TestResolveSiteIDByOrg2511_Deterministic > "$OUT" 2>&1
    cat "$OUT"
    need_token "ANCHOR_PREFERRED=1" "$OUT"
    need_token "EARLIEST_FALLBACK=1" "$OUT"
    need_token "EMPTY_FOR_UNKNOWN=1" "$OUT"
    need_token "PRIMARY_EQUALS_OPERATOR_SITE=1" "$OUT"
    need_token "PRIMARY_NOT_ORG=1" "$OUT"
    rm -f "$OUT"
    echo "R4_OK"
    ;;

  r5)
    echo "== R5 same-class scan + read-only stock SQL =="
    [ -f "$WAREHOUSE" ] || fail "missing $WAREHOUSE"

    # Defect signature: cross-domain write of the order's org into current_site_id.
    ORG_WRITTEN_TO_SITE="$(grep -cE 'current_site_id"\] = order\.OrgID' "$WAREHOUSE" || true)"
    # The resolver must be the only fallback and must be used by both entry points.
    RESOLVER_CALLS="$(grep -c 'resolveSiteIDByOrg(db, order.OrgID)' "$WAREHOUSE" || true)"
    RESOLVER_DEF="$(grep -c '^func resolveSiteIDByOrg(' "$WAREHOUSE" || true)"

    # Neighbour scan: does the assessment handler have the same defect?
    ASSESSMENT_HITS="$(grep -rnE 'current_site_id"\] = .*OrgID' backend/handlers/assessment*.go 2>/dev/null | wc -l || true)"

    echo "ORG_WRITTEN_TO_SITE=${ORG_WRITTEN_TO_SITE}"
    echo "RESOLVER_CALLS=${RESOLVER_CALLS}"
    echo "RESOLVER_DEF=${RESOLVER_DEF}"
    echo "ASSESSMENT_SAME_CLASS_HITS=${ASSESSMENT_HITS}"

    # Read-only stock scan SQL (D3: never executed here; data repair needs Owner approval).
    echo "--- stock scan SQL (read-only, NOT executed) ---"
    echo "SQL_PRESENT=1"
    cat <<'SQL'
SELECT i.id, i.sn, i.tenant_id, i.current_site_id
FROM instruments i
WHERE i.current_site_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM sites s WHERE s.id = i.current_site_id);
-- Rows above have a cross-domain value in instruments.current_site_id.
-- Fixing them requires Owner approval (AGENTS.md red line: no DB writes by AI).
SQL

    [ "${ORG_WRITTEN_TO_SITE:-1}" = "0" ] || fail "warehouse.go still writes order.OrgID into current_site_id"
    [ "${RESOLVER_CALLS:-0}" = "2" ] || fail "expected 2 resolver call sites, got ${RESOLVER_CALLS:-0}"
    [ "${RESOLVER_DEF:-0}" = "1" ] || fail "expected exactly one resolveSiteIDByOrg definition"
    echo "R5_OK"
    ;;

  r6|gate)
    echo "== R6 build + targeted regression =="
    TMP_BUILD="$(mktemp)"
    ( cd backend && heavy go build ./... ) > "$TMP_BUILD" 2>&1
    BUILD_RC=$?
    tail -5 "$TMP_BUILD"
    rm -f "$TMP_BUILD"

    TMP_REG="$(mktemp)"
    ( cd backend && heavy go test ./handlers -count=1 -v \
      -run 'TestInspectReturn2511|TestAssessDamage2511|TestResolveSiteIDByOrg2511|TestResolveOperatorSiteID|TestInspectReturn_|TestAssessDamage_|TestSubmitAssessment_' \
      ) > "$TMP_REG" 2>&1
    REG_RC=$?
    FAIL_N="$(grep -c '^--- FAIL' "$TMP_REG" || true)"
    PASS_N="$(grep -cE '^--- PASS' "$TMP_REG" || true)"
    grep -E '^--- (PASS|FAIL)' "$TMP_REG" | tail -12 || true
    tail -3 "$TMP_REG"
    rm -f "$TMP_REG"

    echo "BUILD_RC=${BUILD_RC}"
    echo "REG_RC=${REG_RC}"
    echo "REG_PASS_COUNT=${PASS_N}"
    echo "REG_FAIL_COUNT=${FAIL_N}"

    [ "$BUILD_RC" -eq 0 ] || fail "go build ./... failed (rc=${BUILD_RC})"
    [ "$REG_RC" -eq 0 ] || fail "targeted regression failed (rc=${REG_RC}, fails=${FAIL_N})"
    echo "R6_OK"
    ;;
esac
