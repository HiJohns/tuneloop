#!/usr/bin/env bash
# verify-2642.sh — evidence generator for #2642 (P4-S1 tenant_id inventory)
#
# Subcommands (each prints one durable artifact including its provenance header):
#   r1  table inventory TSV (+ token block)      -> docs/evidence/2642/R1-inventory.tsv
#   r2  inventory document coverage              -> docs/evidence/2642/R2-doc-coverage.txt
#   r3  determinism (two runs, byte identical)   -> docs/evidence/2642/R3-determinism.txt
#   r4  zero-reference red/green + literal gate  -> docs/evidence/2642/R4-gate-redgreen.txt
#   r5  go build + read-only diff assertions     -> docs/evidence/2642/R5-build.txt
#   evidence  run every Evidence Contract row of the Accepted Plan and write
#             docs/evidence/2642/evidence.json (schema v1)
#
# Provenance (P14): every text artifact starts with
#   # build=1.0.<fix short sha> # commit=<40 hex> # dirty=<true|false>
#   GENERATED_AT_HEAD=<40 hex>
# FIX_COMMIT overrides the anchor (default: HEAD). Evidence must be generated
# *after* the fix commit, from a tree with no tracked modifications.
#
# Heavy command policy: `go build` runs through the host heavy lock
# (~/.config/opencode/scripts/heavy-run.sh) and never wraps a whole batch.
#
# Exit: 0 ok, 1 assertion failed, 2 usage/infrastructure error, 75 lock timeout.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ISSUE=2642
EVID_DIR="$ROOT/docs/evidence/$ISSUE"
SCANNER="$ROOT/scripts/tenant-id-refscan.sh"
TESTS="$ROOT/scripts/tests/test_tenant_id_refscan.sh"
TABLES_FILE="$ROOT/docs/ops/tenant-id-p4-tables.txt"
INVENTORY_DOC_REL="docs/ops/tenant-id-p4-inventory.md"
HEAVY_RUN="${HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"

die() { echo "ERROR:$*" >&2; exit 2; }

usage() {
  sed -n '2,22p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

fix_ref() {
  local ref="${FIX_COMMIT:-HEAD}"
  git -C "$ROOT" rev-parse --verify "${ref}^{commit}" 2>/dev/null || die "cannot resolve FIX_COMMIT=$ref"
}

provenance() {
  local sha short dirty
  sha="$(fix_ref)"
  short="$(git -C "$ROOT" rev-parse --short "$sha")"
  dirty=false
  if [ -n "$(git -C "$ROOT" status --porcelain --untracked-files=no 2>/dev/null)" ]; then
    dirty=true
  fi
  printf '# build=1.0.%s # commit=%s # dirty=%s\n' "$short" "$sha" "$dirty"
  printf 'GENERATED_AT_HEAD=%s\n' "$sha"
}

tables_file_lines() {
  grep -vcE '^[[:space:]]*(#|$)' "$TABLES_FILE" 2>/dev/null || echo 0
}

cmd_r1() {
  provenance
  bash "$SCANNER" --tsv
}

cmd_r2() {
  provenance
  bash "$SCANNER" --check-doc "$INVENTORY_DOC_REL"
}

cmd_r3() {
  provenance
  bash "$TESTS" determinism
}

cmd_r4() {
  provenance
  bash "$TESTS" gate
  local base
  base="$(bash "$SCANNER" --assert-no-live-literal 2>/dev/null | sed -n 's/^LIVE_LITERAL_BASE=//p' | tail -1)"
  printf 'LIVE_LITERAL_BASE=%s\n' "${base:-0}"
}

cmd_r5() {
  provenance
  local go_bin go_build_exit backend_diff frontend_diff lines
  go_bin="$(command -v go 2>/dev/null || true)"
  if [ -z "$go_bin" ] && [ -x /usr/local/go/bin/go ]; then
    go_bin=/usr/local/go/bin/go
    PATH="/usr/local/go/bin:$PATH"
  fi

  go_build_exit=2
  if [ -n "$go_bin" ] && [ -x "$HEAVY_RUN" ]; then
    ( cd "$ROOT/backend" && "$HEAVY_RUN" --scope host -- "$go_bin" build ./... ) >&2
    go_build_exit=$?
  elif [ -n "$go_bin" ]; then
    ( cd "$ROOT/backend" && "$go_bin" build ./... ) >&2
    go_build_exit=$?
  else
    echo "ERROR:go toolchain not found" >&2
  fi

  local base_ref="origin/develop"
  git -C "$ROOT" rev-parse --verify "$base_ref" >/dev/null 2>&1 || base_ref="$(git -C "$ROOT" rev-parse HEAD)"

  backend_diff="$(git -C "$ROOT" diff --name-only "$base_ref"...HEAD -- backend | grep -c . || true)"
  frontend_diff="$( ( git -C "$ROOT" diff --name-only "$base_ref"...HEAD -- frontend-pc frontend-mobile ) | grep -c . || true)"
  lines="$(tables_file_lines)"

  echo "GO_BUILD_EXIT=$go_build_exit"
  echo "BACKEND_DIFF_FILES=${backend_diff:-0}"
  echo "FRONTEND_DIFF_FILES=${frontend_diff:-0}"
  echo "TABLES_FILE_LINES=${lines:-0}"
  echo "BASE_REF=$base_ref"

  local fail=0
  [ "${go_build_exit}" = "0" ] || fail=1
  [ "${backend_diff:-0}" = "0" ] || fail=1
  [ "${frontend_diff:-0}" = "0" ] || fail=1
  [ "${lines:-0}" = "52" ] || fail=1
  return "$fail"
}

# ---------------------------------------------------------------------------
# evidence.json (schema v1)
# ---------------------------------------------------------------------------
contract_rows() {
  awk '
    /^## Evidence Contract/ { insec = 1; next }
    /^### / { if (insec) insec = 0 }
    insec && /^\|[[:space:]]*R[0-9]+[[:space:]]*\|/ {
      n = split($0, c, "|")
      for (i = 1; i <= n; i++) {
        gsub(/^[[:space:]]+|[[:space:]]+$/, "", c[i])
        gsub(/`/, "", c[i])
      }
      if (n >= 8) printf("%s\t%s\t%s\t%s\t%s\t%s\n", c[2], c[3], c[4], c[5], c[6], c[7])
    }
  ' "$EVID_DIR/plan.md"
}

negatives() { # row-id -> machine-checked negative evidence (empty = failed)
  case "$1" in
    R1)
      local out="$EVID_DIR/R1-inventory.tsv" bad=""
      local live snap post drop csum empty enum
      live="$(sed -n 's/^# INV_LIVE=//p' "$out" | tail -1)"
      snap="$(sed -n 's/^# INV_SNAPSHOT=//p' "$out" | tail -1)"
      post="$(sed -n 's/^# POST_SNAPSHOT_TABLES=//p' "$out" | tail -1)"
      drop="$(sed -n 's/^# DROPPED_IN_SNAPSHOT=//p' "$out" | tail -1)"
      csum="$(sed -n 's/^# CLASS_SUM_CHECK=//p' "$out" | tail -1)"
      [ "$live" = "$(( ${snap:-0} + ${post:-0} - ${drop:-0} ))" ] || bad="$bad formula-drift"
      [ "$csum" = "$live" ] || bad="$bad class-sum-mismatch"
      empty="$(awk -F'\t' '$1 != "table" && $1 !~ /^#/ && NF >= 13 { if ($12 == "" || $13 == "") c++ } END { print c + 0 }' "$out")"
      [ "$empty" = "0" ] || bad="$bad empty-class-or-env"
      enum="$(awk -F'\t' '$1 != "table" && $1 !~ /^#/ && NF >= 13 { if ($12 !~ /^(backup|redundant|metadata|anchor-candidate)$/ || $13 !~ /^(pre|pre-only|post-snapshot)$/) c++ } END { print c + 0 }' "$out")"
      [ "$enum" = "0" ] || bad="$bad enum-violation"
      grep -qE 'database/sql|jackc/pgx|gorm\.io' "$SCANNER" && bad="$bad scanner-uses-db"
      echo "formula_ok=1 class_sum_ok=1 enums_nonempty=1 scanner_no_db=1 rejects:$bad"
      [ -z "$bad" ]
      ;;
    R2)
      local out="$EVID_DIR/R2-doc-coverage.txt" bad=""
      local missing lists covered backlog
      missing="$(sed -n 's/^DOC_MISSING=//p' "$out" | tail -1)"
      lists="$(sed -n 's/^DOC_LISTS=//p' "$out" | tail -1)"
      covered="$(sed -n 's/^DOC_CLASS_COVERED=//p' "$out" | tail -1)"
      backlog="$(sed -n 's/^DOC_DB_BACKLOG=//p' "$out" | tail -1)"
      [ "$missing" = "0" ] || bad="$bad missing-tables"
      [ "$lists" = "2" ] || bad="$bad missing-list-section"
      [ "$covered" = "1" ] || bad="$bad missing-class-column"
      [ "$backlog" = "1" ] || bad="$bad missing-ops-template"
      grep -q '^DOC_MISSING_TABLES=' "$out" && bad="$bad unexpected-missing-list"
      echo "no_missing_tables=1 both_lists_present=1 class_covered=1 ops_template_present=1 rejects:$bad"
      [ -z "$bad" ]
      ;;
    R3)
      local out="$EVID_DIR/R3-determinism.txt"
      local diff ts abs
      diff="$(sed -n 's/^DIFF_LINES=//p' "$out" | tail -1)"
      ts="$(sed -n 's/^OUTPUT_HAS_TIMESTAMP=//p' "$out" | tail -1)"
      abs="$(sed -n 's/^OUTPUT_HAS_ABS_PATH=//p' "$out" | tail -1)"
      echo "no_output_jitter=$([ "${diff:-1}" = "0" ] && echo 1) no_timestamp=$([ "${ts:-1}" = "0" ] && echo 1) no_abs_path=$([ "${abs:-1}" = "0" ] && echo 1)"
      [ "${diff:-1}" = "0" ] && [ "${ts:-1}" = "0" ] && [ "${abs:-1}" = "0" ]
      ;;
    R4)
      local out="$EVID_DIR/R4-gate-redgreen.txt" bad=""
      local red green lit cleaned
      red="$(sed -n 's/^GATE_RED_EXIT=//p' "$out" | tail -1)"
      green="$(sed -n 's/^GATE_GREEN_EXIT=//p' "$out" | tail -1)"
      lit="$(sed -n 's/^LIVE_LITERAL_RED_EXIT=//p' "$out" | tail -1)"
      cleaned="$(sed -n 's/^FIXTURE_CLEANED=//p' "$out" | tail -1)"
      [ "$red" = "1" ] || bad="$bad gate-not-red-with-fixture"
      [ "$green" = "0" ] || bad="$bad gate-not-green-after-cleanup"
      [ "$lit" = "1" ] || bad="$bad literal-gate-silently-green"
      [ "$cleaned" = "1" ] || bad="$bad fixture-residue"
      echo "fixture_turns_gate_red=1 cleanup_turns_gate_green=1 literal_gate_rejects=1 fixture_removed=1 rejects:$bad"
      [ -z "$bad" ]
      ;;
    R5)
      local out="$EVID_DIR/R5-build.txt" bad=""
      local gb bd fd tl
      gb="$(sed -n 's/^GO_BUILD_EXIT=//p' "$out" | tail -1)"
      bd="$(sed -n 's/^BACKEND_DIFF_FILES=//p' "$out" | tail -1)"
      fd="$(sed -n 's/^FRONTEND_DIFF_FILES=//p' "$out" | tail -1)"
      tl="$(sed -n 's/^TABLES_FILE_LINES=//p' "$out" | tail -1)"
      [ "$gb" = "0" ] || bad="$bad build-failed"
      [ "$bd" = "0" ] || bad="$bad backend-modified"
      [ "$fd" = "0" ] || bad="$bad frontend-modified"
      [ "$tl" = "52" ] || bad="$bad table-list-line-count"
      echo "build_ok=1 read_only_backend=1 read_only_frontend=1 table_list_rows=52 rejects:$bad"
      [ -z "$bad" ]
      ;;
    *) echo "unknown-row" ; return 1 ;;
  esac
}

cmd_evidence() {
  [ -f "$EVID_DIR/plan.md" ] || die "plan snapshot missing: docs/evidence/$ISSUE/plan.md"
  local rows_file="$EVID_DIR/.rows.jsonl"
  : > "$rows_file"

  local plan_sha build_sha build_short dirty
  plan_sha="$(sha256sum "$EVID_DIR/plan.md" | awk '{print $1}')"
  build_sha="$(fix_ref)"
  build_short="$(git -C "$ROOT" rev-parse --short "$build_sha")"
  dirty=false
  [ -z "$(git -C "$ROOT" status --porcelain --untracked-files=no 2>/dev/null)" ] || dirty=true

  local rc_all=0
  while IFS=$'\t' read -r rid cmd art assert expected negative; do
    [ -n "$rid" ] || continue
    local exit_code=0
    ( cd "$ROOT" && bash -c "$cmd" ) >/dev/null 2>"$EVID_DIR/.$rid.err" || exit_code=$?
    local actual=""
    if [ -f "$ROOT/$art" ]; then
      actual="$(grep -hE '^(# )?(INV_|CLASS_|MODEL_|PLUMBING_|FRONTEND_|SQL_REFS_|FIELD_REFS_|LIVE_LITERAL_|DETERMINISTIC|DOC_|GO_BUILD_|BACKEND_DIFF_|FRONTEND_DIFF_|TABLES_FILE_|GATE_|FIXTURE_)[A-Z_]*=' "$ROOT/$art" \
        | sed 's/^# //' | paste -sd';' -)"
    fi
    [ -n "$actual" ] || actual="NO_TOKENS (exit=$exit_code)"
    local neg_result neg_ok=0
    if neg_result="$(negatives "$rid")"; then neg_ok=1; fi
    local pass=false
    [ "$exit_code" = "0" ] && [ "$neg_ok" = "1" ] && pass=true
    [ "$pass" = "true" ] || rc_all=1

    jq -c -n \
      --arg id "$rid" \
      --arg cmd "$cmd" \
      --argjson exit "$exit_code" \
      --arg art "$art" \
      --arg assert "$assert" \
      --arg actual "$actual" \
      --arg expected "$expected" \
      --arg negative "$negative" \
      --arg negres "$neg_result" \
      --argjson pass "$pass" \
      '{id:$id, command:$cmd, exit:$exit, artifacts:[$art], assert:$assert, actual:$actual, expected:$expected, negative:$negative, negative_result:$negres, pass:$pass}' \
      >> "$rows_file"
  done < <(contract_rows)

  local rows_count
  rows_count="$(jq -s 'length' "$rows_file" 2>/dev/null || echo 0)"
  [ "${rows_count:-0}" -ge 1 ] || die "no Evidence Contract rows found in plan.md"

  jq -n \
    --argjson rows "$(jq -s '.' "$rows_file")" \
    --arg issue "$ISSUE" \
    --arg build "1.0.$build_short" \
    --arg commit "$build_sha" \
    --argjson dirty "$([ "$dirty" = "true" ] && echo true || echo false)" \
    --arg plan_sha "$plan_sha" \
    --arg environment "repo-static(read-only),no-db" \
    --arg tooling_commit "$(git -C "$HOME/.config/opencode" rev-parse HEAD 2>/dev/null || echo '')" \
    '{schema_version:1, issue:($issue|tonumber), build:$build, commit:$commit, dirty:$dirty,
      plan_sha256:$plan_sha, environment:$environment, tooling_commit:$tooling_commit, rows:$rows}' \
    > "$EVID_DIR/evidence.json"

  rm -f "$rows_file" "$EVID_DIR"/.R*.err
  jq -e '.rows | length > 0' "$EVID_DIR/evidence.json" >/dev/null || die "evidence.json row array empty"
  echo "EVIDENCE=docs/evidence/$ISSUE/evidence.json"
  echo "EVIDENCE_ROWS=$rows_count"
  echo "EVIDENCE_PASS=$(jq '[.rows[] | select(.pass == true)] | length' "$EVID_DIR/evidence.json")"
  return "$rc_all"
}

main() {
  case "${1:-}" in
    r1) cmd_r1 ;;
    r2) cmd_r2 ;;
    r3) cmd_r3 ;;
    r4) cmd_r4 ;;
    r5) cmd_r5 ;;
    evidence) cmd_evidence ;;
    -h|--help|"") usage ;;
    *) die "unknown subcommand: $1" ;;
  esac
}

main "$@"
