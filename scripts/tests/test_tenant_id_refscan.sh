#!/usr/bin/env bash
# test_tenant_id_refscan.sh — red/green + determinism regression for the #2642 scanner
#
# Usage:
#   bash scripts/tests/test_tenant_id_refscan.sh determinism
#   bash scripts/tests/test_tenant_id_refscan.sh gate
#
# determinism
#   Runs `tenant-id-refscan.sh --tsv` twice on the same tree and asserts:
#     DETERMINISTIC=1  DIFF_LINES=0
#     OUTPUT_HAS_TIMESTAMP=0  OUTPUT_HAS_ABS_PATH=0
#
# gate
#   Builds a throw-away Go fixture under tmp/refscan-fixture/ that references a
#   real table with zero current references (brand_configs) plus a `tenant_id`
#   literal, then asserts:
#     GATE_RED_EXIT=1    fixture present  -> --require-zero fails
#     GATE_GREEN_EXIT=0  fixture removed  -> --require-zero passes
#     LIVE_LITERAL_RED_EXIT=1            whole-repo literal gate is still red
#     FIXTURE_CLEANED=1                  no fixture residue in git status
#
# Exit: 0 all assertions held, 1 assertion failed, 2 infrastructure/usage error.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
SCANNER="$ROOT/scripts/tenant-id-refscan.sh"
FIXTURE_DIR="$ROOT/tmp/refscan-fixture"
FIXTURE_FILE="$FIXTURE_DIR/fixture_refs.go"
FIXTURE_TABLE="brand_configs"

MODE="${1:-}"
case "$MODE" in
  determinism|gate) : ;;
  *) echo "Usage: $0 determinism|gate" >&2; exit 2 ;;
esac

[ -x "$SCANNER" ] || [ -f "$SCANNER" ] || { echo "ERROR:scanner missing: scripts/tenant-id-refscan.sh" >&2; exit 2; }

WORK="$(mktemp -d "${TMPDIR:-/tmp}/refscan-test.XXXXXX")" || { echo "ERROR:temp dir" >&2; exit 2; }

cleanup() {
  rm -rf "$FIXTURE_DIR"
  rm -rf "$WORK"
}
trap cleanup EXIT

fail=0
note() { printf '%s\n' "$*"; }

if [ "$MODE" = "determinism" ]; then
  bash "$SCANNER" --tsv > "$WORK/a.tsv" 2> "$WORK/a.err"
  rc_a=$?
  bash "$SCANNER" --tsv > "$WORK/b.tsv" 2> "$WORK/b.err"
  rc_b=$?
  if [ "$rc_a" -ne 0 ] || [ "$rc_b" -ne 0 ]; then
    note "SCANNER_EXIT=$rc_a/$rc_b"
    cat "$WORK/a.err" "$WORK/b.err"
    exit 1
  fi

  diff_lines="$(diff "$WORK/a.tsv" "$WORK/b.tsv" | grep -cE '^[<>]' || true)"
  diff_lines="${diff_lines:-0}"

  has_ts=0
  if grep -qE '[0-9]{4}-[0-9]{2}-[0-9]{2}[T ][0-9]{2}:[0-9]{2}:[0-9]{2}' "$WORK/a.tsv"; then
    has_ts=1
  fi
  if grep -qE '(^|[^0-9])(0[0-9]|1[0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9]' "$WORK/a.tsv"; then
    has_ts=1
  fi

  has_abs=0
  if grep -qF "$ROOT" "$WORK/a.tsv"; then
    has_abs=1
  fi
  if grep -qE '(^|[[:space:]])/(home|root|tmp|usr|var)/' "$WORK/a.tsv"; then
    has_abs=1
  fi

  deterministic=0
  if [ "$diff_lines" -eq 0 ] && [ "$has_ts" -eq 0 ] && [ "$has_abs" -eq 0 ]; then
    deterministic=1
  fi

  note "RUNS=2"
  note "SCANNER_EXIT=$rc_a"
  note "DIFF_LINES=$diff_lines"
  note "OUTPUT_HAS_TIMESTAMP=$has_ts"
  note "OUTPUT_HAS_ABS_PATH=$has_abs"
  note "DETERMINISTIC=$deterministic"

  [ "$deterministic" -eq 1 ] || fail=1
  [ "$fail" -eq 0 ] && note "RESULT=PASS" || note "RESULT=FAIL"
  exit "$fail"
fi

# ---------------------------------------------------------------- gate mode
mkdir -p "$FIXTURE_DIR"
cat > "$FIXTURE_FILE" <<'EOF'
package refscanfixture

// Red/green probe for scripts/tenant-id-refscan.sh --table <T> --require-zero.
// It is created only for the duration of test_tenant_id_refscan.sh gate and
// removed afterwards; it never reaches a commit. The quoted exact table name is
// what the scanner attributes (same shape as db.Table("orders") in the code).
const redFixtureTable = "brand_configs"
const redFixtureSQL = "SELECT * FROM brand_configs WHERE tenant_id = ?"
EOF

red_out="$(bash "$SCANNER" --table "$FIXTURE_TABLE" --require-zero 2>&1)"
red_rc=$?
red_refs="$(printf '%s\n' "$red_out" | sed -n 's/^REQUIRE_ZERO_REFS=//p' | tail -1)"

rm -rf "$FIXTURE_DIR"
green_out="$(bash "$SCANNER" --table "$FIXTURE_TABLE" --require-zero 2>&1)"
green_rc=$?
green_refs="$(printf '%s\n' "$green_out" | sed -n 's/^REQUIRE_ZERO_REFS=//p' | tail -1)"

live_out="$(bash "$SCANNER" --assert-no-live-literal 2>&1)"
live_rc=$?
live_base="$(printf '%s\n' "$live_out" | sed -n 's/^LIVE_LITERAL_BASE=//p' | tail -1)"

residue="$(git -C "$ROOT" status --porcelain -- tmp/refscan-fixture 2>/dev/null | wc -l | tr -d ' ')"
fixture_cleaned=1
[ "$residue" = "0" ] || fixture_cleaned=0

note "FIXTURE_TABLE=$FIXTURE_TABLE"
note "FIXTURE_RED_REFS=${red_refs:-0}"
note "FIXTURE_GREEN_REFS=${green_refs:-0}"
note "GATE_RED_EXIT=$red_rc"
note "GATE_GREEN_EXIT=$green_rc"
note "LIVE_LITERAL_BASE=${live_base:-0}"
note "LIVE_LITERAL_RED_EXIT=$live_rc"
note "FIXTURE_CLEANED=$fixture_cleaned"

[ "$red_rc" -eq 1 ] || fail=1
[ "$green_rc" -eq 0 ] || fail=1
[ "$live_rc" -eq 1 ] || fail=1
[ "$fixture_cleaned" -eq 1 ] || fail=1
[ "${red_refs:-0}" -ge 1 ] 2>/dev/null || fail=1
[ "${green_refs:-1}" -eq 0 ] 2>/dev/null || fail=1

[ "$fail" -eq 0 ] && note "RESULT=PASS" || note "RESULT=FAIL"
exit "$fail"
