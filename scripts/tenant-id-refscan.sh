#!/usr/bin/env bash
# tenant-id-refscan.sh — read-only, DB-free, deterministic tenant_id reference scanner
#
# Issue: #2642 (P4-S1). This tool produces the single decision input for the
# P4 batch (`tenant_id` column/index removal): per-table reference counts, a
# mechanically derived class bucket, and the environment each table lives in.
#
# ---------------------------------------------------------------------------
# Inputs (static repository files only; no DB, no network)
#   docs/ops/tenant-id-p4-tables.txt   authoritative P2 snapshot list (52 rows)
#   docs/ops/tenant-id-backfill-p2.md  cross-check: P2 §3 pre-snapshot tables
#   backend/models/*.go                struct TenantID / OrgID / TableName()
#   backend/database/migrations/*.up.sql  DDL: org_id, DEFAULT, indexes, CREATE TABLE
#   backend/**/*.go                    literal references (sql_refs / field_refs)
#   frontend-pc/src, frontend-mobile/src   tenant_id occurrences (global)
#   docs/**                            docs mentioning a table + tenant_id
#   tmp/refscan-fixture/**/*.go        red/green gate fixture (absent normally)
#
# ---------------------------------------------------------------------------
# Counting rules (mechanical; table-name allowlists are forbidden)
#
#   sql_refs    non-test Go: a line containing `tenant_id` and, within a
#               +/-3 line window, the quoted table name ("<table>").
#               One increment per (tenant_id line, table) pair.
#   field_refs  non-test Go: a line containing `TenantID` and, within a
#               +/-5 line window, the model struct name of that table
#               (covers X.TenantID / X{TenantID: / TenantID:).
#   test_refs   *_test.go: sql_refs + field_refs under the same rules.
#   frontend_refs  GLOBAL count of `tenant_id` in frontend-pc/src +
#               frontend-mobile/src. Not attributed to a table; the value is
#               repeated on every row and must not be summed.
#   doc_refs    per table: number of docs/** files that mention the table and
#               contain `tenant_id` (reference only, never a gate).
#   plumbing_refs  GLOBAL, single token (never per table): tenant_id/TenantID
#               mechanism references inside backend/database/db.go,
#               backend/middleware/*.go and backend/models/*.go. These belong
#               to the tenant plumbing layer, not to any single table
#               (file-level attribution would credit every table with 50+
#               refs; measured example: brand_configs = 0 table-level refs but
#               51 file-level refs).
#
# Classification (fixed four-bucket rule, no per-table overrides)
#   backup            source=backup-only (table name ^bak_)
#   redundant         has_org_id=yes (the table also carries org_id)
#   metadata          has_org_id=no and sql_refs+field_refs == 0
#   anchor-candidate  has_org_id=no and sql_refs+field_refs > 0
#
# Environment (three-environment divergence, prod lacks the 3 bak_ tables)
#   pre             present in the P2 pre-snapshot list
#   pre-only        P2 pre-only (^bak_); missing from prod
#   post-snapshot   table created by an executable migration (*.up.sql) after
#                   the P2 snapshot was taken
#
# ---------------------------------------------------------------------------
# Known blind spots (registered in docs/ops/tenant-id-p4-inventory.md §6)
#   dynamically built column names (fmt.Sprintf), reflection/LookUpField,
#   JSON tags (json:"tenant_id"), DB-side objects (DEFAULT/trigger/view),
#   other repositories. Final authority for column removal is the compiler
#   (delete the field -> go build ./...) plus --assert-no-live-literal.
#
# Modes
#   --tsv (default)                 live rows + "# KEY=VALUE" token block
#   --json                          same content as JSON
#   --table <T> --require-zero      exit 0 only when that table has zero
#                                   sql_refs+field_refs; else exit 1 + hit lines
#   --assert-no-live-literal        whole-repo `tenant_id` literal gate; the
#                                   allowlist is tenant_id_deprecated,
#                                   backend/database/migrations/**, docs/** and
#                                   this tool group. Prints LIVE_LITERAL_BASE;
#                                   exit 1 while the baseline is non-zero.
#   --check-doc <path>              inventory document coverage assertions
#
# Exit codes: 0 ok, 1 gate failed, 2 invalid/damaged input (never downgraded to
# a warning).
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

TABLES_FILE="$ROOT/docs/ops/tenant-id-p4-tables.txt"
P2_DOC="$ROOT/docs/ops/tenant-id-backfill-p2.md"
MODELS_DIR="$ROOT/backend/models"
MIGRATIONS_DIR="$ROOT/backend/database/migrations"
FIXTURE_DIR="$ROOT/tmp/refscan-fixture"
SELF_REL="scripts/tenant-id-refscan.sh"
VERIFY_REL="scripts/verify-2642.sh"
TESTS_REL="scripts/tests/test_tenant_id_refscan.sh"

SNAPSHOT_EXPECTED=52

die() { echo "ERROR:$*" >&2; exit 2; }
usage() { sed -n '2,84p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; }

MODE="tsv"
TARGET_TABLE=""
DOC_PATH=""

while [ $# -gt 0 ]; do
  case "$1" in
    --tsv) MODE="tsv"; shift ;;
    --json) MODE="json"; shift ;;
    --table) [ $# -ge 2 ] || die "--table requires a value"; TARGET_TABLE="$2"; MODE="require-zero"; shift 2 ;;
    --require-zero) MODE="require-zero"; shift ;;
    --assert-no-live-literal) MODE="live-literal"; shift ;;
    --check-doc) [ $# -ge 2 ] || die "--check-doc requires a path"; DOC_PATH="$2"; MODE="check-doc"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

[ -f "$TABLES_FILE" ] || die "table list not found: docs/ops/tenant-id-p4-tables.txt"
[ -d "$MODELS_DIR" ] || die "models directory not found: backend/models"
[ -d "$MIGRATIONS_DIR" ] || die "migrations directory not found: backend/database/migrations"

TMPWORK="$(mktemp -d "${TMPDIR:-/tmp}/refscan.XXXXXX")" || die "cannot create temp dir"
cleanup() { rm -rf "$TMPWORK"; }
trap cleanup EXIT

# ---------------------------------------------------------------------------
# 1. Authoritative table list (single source) + damage checks
# ---------------------------------------------------------------------------
awk -F'|' '
  /^[[:space:]]*($|#)/ { next }
  {
    if (NF != 2) { printf("BADLINE:%d:%s\n", NR, $0); next }
    t = $1; s = $2
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", t)
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", s)
    if (t !~ /^[a-z0-9_]+$/) { printf("BADNAME:%d:%s\n", NR, t); next }
    if (s != "model" && s != "backup-only") { printf("BADSOURCE:%d:%s\n", NR, s); next }
    if (seen[t]++) { printf("DUP:%d:%s\n", NR, t); next }
    printf("%s|%s\n", t, s)
  }
' "$TABLES_FILE" > "$TMPWORK/tables.raw"

if grep -qE '^(BADLINE|BADNAME|BADSOURCE|DUP):' "$TMPWORK/tables.raw"; then
  die "damaged table list: $(grep -m3 -E '^(BADLINE|BADNAME|BADSOURCE|DUP):' "$TMPWORK/tables.raw" | tr '\n' ' ')"
fi

grep -vE '^(BADLINE|BADNAME|BADSOURCE|DUP):' "$TMPWORK/tables.raw" \
  | LC_ALL=C sort > "$TMPWORK/tables.txt"

SNAPSHOT_COUNT="$(wc -l < "$TMPWORK/tables.txt" | tr -d ' ')"
[ "$SNAPSHOT_COUNT" = "$SNAPSHOT_EXPECTED" ] \
  || die "table list must contain $SNAPSHOT_EXPECTED rows (got $SNAPSHOT_COUNT)"

bad_src="$(awk -F'|' '($1 ~ /^bak_/ && $2 != "backup-only") || ($1 !~ /^bak_/ && $2 != "model") { print $1 }' "$TMPWORK/tables.txt")"
[ -z "$bad_src" ] || die "table list source mismatch (bak_* must be backup-only, others model): $bad_src"

cut -d'|' -f1 "$TMPWORK/tables.txt" > "$TMPWORK/tables.only"

# ---------------------------------------------------------------------------
# 2. P2 cross-check: docs/ops/tenant-id-backfill-p2.md §3 must list the same set
# ---------------------------------------------------------------------------
[ -f "$P2_DOC" ] || die "P2 report not found: docs/ops/tenant-id-backfill-p2.md"
awk '
  /^## 3\./ { insec = 1; next }
  /^## 4\./ { insec = 0 }
  insec && /^\|[[:space:]]*`[^`]+`/ {
    line = $0
    sub(/^\|[[:space:]]*`/, "", line)
    sub(/`.*/, "", line)
    print line
  }
' "$P2_DOC" | LC_ALL=C sort > "$TMPWORK/p2.txt"

P2_COUNT="$(wc -l < "$TMPWORK/p2.txt" | tr -d ' ')"
[ "$P2_COUNT" = "$SNAPSHOT_EXPECTED" ] \
  || die "P2 §3 table count mismatch: expected $SNAPSHOT_EXPECTED, got $P2_COUNT"

if ! cmp -s "$TMPWORK/tables.only" "$TMPWORK/p2.txt"; then
  die "table list != P2 §3 table set; diff: $(diff "$TMPWORK/tables.only" "$TMPWORK/p2.txt" | head -6 | tr '\n' ' ')"
fi

# ---------------------------------------------------------------------------
# 3. Models: structs with TenantID -> table name (TableName() or GORM plural)
# ---------------------------------------------------------------------------
MODEL_FILES=()
for f in "$MODELS_DIR"/*.go; do
  [ -f "$f" ] || continue
  case "$f" in *_test.go) continue ;; esac
  MODEL_FILES+=("$f")
done
[ "${#MODEL_FILES[@]}" -gt 0 ] || die "no model files under backend/models"

awk '
  { sub(/\/\/.*$/, "") }
  /^type[[:space:]]+[A-Za-z_][A-Za-z0-9_]*[[:space:]]+struct[[:space:]]*\{/ {
    name = $2
    inb = 1
    body = ""
    next
  }
  inb && /^\}/ {
    if (body ~ /(^|[^A-Za-z0-9_])TenantID([^A-Za-z0-9_]|$)/) {
      printf("S|%s|%d\n", name, (body ~ /(^|[^A-Za-z0-9_])OrgID([^A-Za-z0-9_]|$)/) ? 1 : 0)
    }
    inb = 0
    next
  }
  inb { body = body "\n" $0 }
' "${MODEL_FILES[@]}" > "$TMPWORK/structs.txt"

awk '
  /^func[[:space:]]*\(/ && /\)[[:space:]]*TableName\(\)[[:space:]]*string/ {
    line = $0
    sub(/^func[[:space:]]*\([[:space:]]*\*?/, "", line)
    sub(/\).*/, "", line)
    gsub(/[[:space:]]/, "", line)
    pending = line
    next
  }
  pending != "" && /return[[:space:]]*"/ {
    line = $0
    sub(/.*return[[:space:]]*"/, "", line)
    sub(/".*/, "", line)
    printf("T|%s|%s\n", pending, line)
    pending = ""
  }
' "${MODEL_FILES[@]}" > "$TMPWORK/tableover.txt"

# Tables created by an *executable* migration. The runner consumes *.up.sql; a
# DDL file without that suffix is never applied (measured case:
# 20260404_add_labels_and_metadata.sql creates `labels`, yet
# backend/database/db.go:730 records "ConfirmationSession, Label — no table
# ever created").
awk '
  { sub(/--.*$/, "") }
  /CREATE[[:space:]]+TABLE[[:space:]]+(IF[[:space:]]+NOT[[:space:]]+EXISTS[[:space:]]+)?/ {
    s = $0
    sub(/.*CREATE[[:space:]]+TABLE[[:space:]]+(IF[[:space:]]+NOT[[:space:]]+EXISTS[[:space:]]+)?/, "", s)
    sub(/[[:space:]]*\(.*/, "", s)
    gsub(/"/, "", s)
    if (s ~ /^[a-z0-9_]+$/) print s
  }
' "$MIGRATIONS_DIR"/*.up.sql | LC_ALL=C sort -u > "$TMPWORK/mig.tables"

# Table universe a model may resolve into: P2 snapshot + executable migrations.
# Model -> table resolution does NOT re-implement GORM's inflector. Instead each
# model offers its snake_case candidate set and exactly one candidate must exist
# in the universe; zero candidates means "no table ever created", more than one
# is ambiguous and refuses to run (exit 2). TableName() overrides are
# authoritative and must exist in the universe.
LC_ALL=C sort -u "$TMPWORK/tables.only" "$TMPWORK/mig.tables" > "$TMPWORK/universe.tables"

awk -F'|' -v universe="$TMPWORK/universe.tables" '
  function snake(s,   out, i, c, prev) {
    out = ""
    prev = ""
    for (i = 1; i <= length(s); i++) {
      c = substr(s, i, 1)
      if (c ~ /[A-Z]/ && prev ~ /[a-z0-9]/) out = out "_"
      out = out tolower(c)
      prev = c
    }
    return out
  }
  function addcand(w,   c) {
    ncand++
    cand[ncand] = w
    c = w
    if (c ~ /[^aeiou]y$/) { cand[++ncand] = substr(c, 1, length(c) - 1) "ies" }
    cand[++ncand] = c "s"
    cand[++ncand] = c "es"
  }
  FNR == NR {
    if ($1 == "T") over[$2] = $3
    next
  }
  $1 == "S" {
    m = $2; org = $3
    if (m in over) {
      if (!((over[m]) in known)) {
        printf("BADOVERRIDE|%s|%s|%s\n", m, over[m], "not-created-by-executable-migration")
        next
      }
      printf("OK|%s|%s|%s\n", over[m], m, org)
      next
    }
    base = snake(m)
    delete cand
    ncand = 0
    addcand(base)
    delete hit
    nhit = 0
    for (i = 1; i <= ncand; i++) {
      if ((cand[i]) in known && !((cand[i]) in hit)) { hit[cand[i]] = 1; nhit++; chosen = cand[i] }
    }
    if (nhit == 0) {
      printf("OK|%s|%s|%s\n", "-", m, org)
    } else if (nhit > 1) {
      printf("AMBIGUOUS|%s|%s|%s\n", m, nhit, base)
    } else {
      printf("OK|%s|%s|%s\n", chosen, m, org)
    }
  }
  BEGIN { while ((getline l < universe) > 0) known[l] = 1 }
' "$TMPWORK/tableover.txt" "$TMPWORK/structs.txt" > "$TMPWORK/model-resolve.raw"

if grep -qE '^(AMBIGUOUS|BADOVERRIDE)\|' "$TMPWORK/model-resolve.raw"; then
  die "model->table resolution failed: $(grep -m3 -E '^(AMBIGUOUS|BADOVERRIDE)\|' "$TMPWORK/model-resolve.raw" | tr '\n' ' ')"
fi

awk -F'|' '$1 == "OK" { printf("%s|%s|%s\n", $2, $3, $4) }' "$TMPWORK/model-resolve.raw" \
  | LC_ALL=C sort > "$TMPWORK/models.txt"

MODEL_STRUCTS="$(wc -l < "$TMPWORK/models.txt" | tr -d ' ')"
[ "$MODEL_STRUCTS" = "$SNAPSHOT_EXPECTED" ] \
  || die "model structs with TenantID must be $SNAPSHOT_EXPECTED (got $MODEL_STRUCTS)"

awk -F'|' '$1 != "-" { printf("%s|%s\n", $1, $2) }' "$TMPWORK/models.txt" > "$TMPWORK/table-model.txt"
cut -d'|' -f1 "$TMPWORK/models.txt" | grep -v '^-$' | LC_ALL=C sort -u > "$TMPWORK/model.tables"

: > "$TMPWORK/post.tables"
: > "$TMPWORK/notable.tables"
while IFS= read -r t; do
  [ -n "$t" ] || continue
  grep -Fxq "$t" "$TMPWORK/tables.only" && continue
  printf '%s\n' "$t" >> "$TMPWORK/post.tables"
done < "$TMPWORK/model.tables"

# Models whose table was never created: reported explicitly, never silently
awk -F'|' '$1 == "-" { printf("%s\n", $2) }' "$TMPWORK/models.txt" | LC_ALL=C sort > "$TMPWORK/notable.tables"

MODEL_TABLE_ROWS="$(grep -c -Fxf "$TMPWORK/tables.only" "$TMPWORK/model.tables" || true)"
MODEL_POST_SNAPSHOT="$(wc -l < "$TMPWORK/post.tables" | tr -d ' ')"
MODEL_NO_TABLE="$(wc -l < "$TMPWORK/notable.tables" | tr -d ' ')"

[ "$((MODEL_TABLE_ROWS + MODEL_POST_SNAPSHOT + MODEL_NO_TABLE))" = "$MODEL_STRUCTS" ] \
  || die "model decomposition mismatch: $MODEL_TABLE_ROWS + $MODEL_POST_SNAPSHOT + $MODEL_NO_TABLE != $MODEL_STRUCTS"

while IFS='|' read -r t s; do
  [ "$s" = "model" ] || continue
  grep -Fxq "$t" "$TMPWORK/model.tables" \
    || die "table list row '$t' is marked source=model but no model struct resolves to it"
done < "$TMPWORK/tables.txt"

POST_SNAPSHOT_TABLES="$MODEL_POST_SNAPSHOT"
DROPPED_IN_SNAPSHOT=0

cp "$TMPWORK/tables.txt" "$TMPWORK/live.pairs"
while IFS= read -r t; do
  [ -n "$t" ] || continue
  printf '%s|model\n' "$t" >> "$TMPWORK/live.pairs"
done < "$TMPWORK/post.tables"
LC_ALL=C sort -u "$TMPWORK/live.pairs" > "$TMPWORK/live.txt"
INV_LIVE="$(wc -l < "$TMPWORK/live.txt" | tr -d ' ')"
INV_PRE_ONLY="$(grep -c '|backup-only$' "$TMPWORK/live.txt" || true)"

[ "$INV_LIVE" = "$((SNAPSHOT_EXPECTED + POST_SNAPSHOT_TABLES - DROPPED_IN_SNAPSHOT))" ] \
  || die "INV_LIVE formula violated"

# ---------------------------------------------------------------------------
# 4. org_id / DEFAULT / index facts from executable migrations + gorm tags
# ---------------------------------------------------------------------------
awk '
  function flush_create(   t, b) {
    if (cbuf == "") return
    t = ctable; b = cbuf
    if (b ~ /(^|[(,\n])[[:space:]]*"?org_id"?[[:space:]]/) org[t] = 1
    if (b ~ /"?tenant_id"?[^,)\n]*DEFAULT[[:space:]]+get_default_tenant/) def[t] = "get_default_tenant"
    else if (b ~ /"?tenant_id"?[^,)\n]*DEFAULT[[:space:]]+\x2700000000-0000-0000-0000-000000000000\x27/) def[t] = "zero-uuid"
    seen[t] = 1
    cbuf = ""; ctable = ""
  }
  function flush_alter(   t, b) {
    if (abuf == "") return
    t = atable; b = abuf
    if (b ~ /org_id"?[[:space:]]+(UUID|uuid)/) org[t] = 1
    if (b ~ /tenant_id"?[[:space:]]+(SET[[:space:]]+)?DROP[[:space:]]+DEFAULT/) def[t] = "none"
    else if (b ~ /tenant_id"?[^,;\n]*DEFAULT[[:space:]]+get_default_tenant/) def[t] = "get_default_tenant"
    else if (b ~ /tenant_id"?[^,;\n]*DEFAULT[[:space:]]+\x2700000000-0000-0000-0000-000000000000\x27/) def[t] = "zero-uuid"
    seen[t] = 1
    abuf = ""; atable = ""
  }
  {
    line = $0
    sub(/--.*$/, "", line)
    if (inctx) {
      cbuf = cbuf "\n" line
      if (line ~ /\)[[:space:]]*;/) { inctx = 0; flush_create() }
      next
    }
    if (inalt) {
      abuf = abuf "\n" line
      if (line ~ /;[[:space:]]*$/) { inalt = 0; flush_alter() }
      next
    }
    if (line ~ /CREATE[[:space:]]+TABLE[[:space:]]+/) {
      s = line
      sub(/.*CREATE[[:space:]]+TABLE[[:space:]]+(IF[[:space:]]+NOT[[:space:]]+EXISTS[[:space:]]+)?/, "", s)
      sub(/[[:space:]]*\(.*/, "", s)
      gsub(/"/, "", s)
      if (s ~ /^[a-z0-9_]+$/) {
        ctable = s; cbuf = ""; inctx = 1
        if (line ~ /\)[[:space:]]*;/) { inctx = 0; flush_create() }
      }
      next
    }
    if (line ~ /ALTER[[:space:]]+TABLE[[:space:]]+/) {
      s = line
      sub(/.*ALTER[[:space:]]+TABLE[[:space:]]+(IF[[:space:]]+EXISTS[[:space:]]+)?(ONLY[[:space:]]+)?/, "", s)
      sub(/[[:space:]].*/, "", s)
      gsub(/"/, "", s)
      if (s ~ /^[a-z0-9_]+$/) {
        atable = s; abuf = ""; inalt = 1
        if (line ~ /;[[:space:]]*$/) { inalt = 0; flush_alter() }
      }
      next
    }
  }
  END {
    if (inctx) flush_create()
    if (inalt) flush_alter()
    for (t in seen) printf("T|%s|%d|%s\n", t, (t in org) ? 1 : 0, (t in def) ? def[t] : "none")
  }
' "$MIGRATIONS_DIR"/*.up.sql | LC_ALL=C sort -u > "$TMPWORK/ddl.txt"

awk '
  { sub(/\/\/.*$/, "") }
  /^type[[:space:]]+[A-Za-z_][A-Za-z0-9_]*[[:space:]]+struct[[:space:]]*\{/ { m = $2; next }
  /^\}/ { m = "" }
  m != "" && /TenantID/ {
    line = $0
    while (match(line, /index:[A-Za-z0-9_,]+/)) {
      tag = substr(line, RSTART + 6, RLENGTH - 6)
      n = split(tag, arr, ",")
      for (i = 1; i <= n; i++) if (arr[i] != "") printf("G|%s|%s\n", m, arr[i])
      line = substr(line, RSTART + RLENGTH)
    }
  }
' "${MODEL_FILES[@]}" > "$TMPWORK/gormindex.raw"

: > "$TMPWORK/gorm.index"
while IFS='|' read -r kind model idx; do
  [ "$kind" = "G" ] || continue
  tbl="$(awk -F'|' -v m="$model" '$2 == m { print $1; exit }' "$TMPWORK/models.txt")"
  [ -n "$tbl" ] && printf '%s|%s\n' "$tbl" "$idx" >> "$TMPWORK/gorm.index"
done < "$TMPWORK/gormindex.raw"

awk '
  { sub(/--.*$/, "") }
  /CREATE[[:space:]]+(UNIQUE[[:space:]]+)?INDEX/ {
    line = $0
    sub(/.*[[:space:]]ON[[:space:]]+/, "", line)
    split(line, parts, "(")
    tbl = parts[1]
    gsub(/[[:space:]"]/, "", tbl)
    if (tbl !~ /^[a-z0-9_]+$/) next
    cols = parts[2]
    sub(/\).*/, "", cols)
    if (cols ~ /tenant_id/) {
      nm = $0
      sub(/.*[[:space:]]INDEX[[:space:]]+(IF[[:space:]]+NOT[[:space:]]+EXISTS[[:space:]]+)?/, "", nm)
      sub(/[[:space:]]+ON.*/, "", nm)
      gsub(/"/, "", nm)
      if (nm ~ /^[a-z0-9_]+$/) printf("%s|%s\n", tbl, nm)
    }
  }
' "$MIGRATIONS_DIR"/*.up.sql > "$TMPWORK/ddl.index"

cat "$TMPWORK/gorm.index" "$TMPWORK/ddl.index" | LC_ALL=C sort -u > "$TMPWORK/index.all"

# ---------------------------------------------------------------------------
# 5. Reference counting (sql_refs / field_refs / test_refs)
# ---------------------------------------------------------------------------
: > "$TMPWORK/go.nontest"
find "$ROOT/backend" -name '*.go' ! -name '*_test.go' -type f | LC_ALL=C sort >> "$TMPWORK/go.nontest"
: > "$TMPWORK/go.test"
find "$ROOT/backend" -name '*_test.go' -type f | LC_ALL=C sort >> "$TMPWORK/go.test"
: > "$TMPWORK/go.fixture"
if [ -d "$FIXTURE_DIR" ]; then
  find "$FIXTURE_DIR" -name '*.go' -type f | LC_ALL=C sort >> "$TMPWORK/go.fixture"
fi

awk -F'|' '
  FNR == NR { model[$1] = $2; next }
  { printf("%s|%s\n", $1, ($1 in model) ? model[$1] : "") }
' "$TMPWORK/table-model.txt" "$TMPWORK/live.txt" > "$TMPWORK/count.matrix"

count_refs() { # <files-list> <out> <emit-hits:0|1>
  local list="$1" out="$2" emit="$3"
  : > "$out"
  [ -s "$list" ] || return 0
  local -a files=()
  while IFS= read -r f; do files+=("$f"); done < "$list"
  awk -F'|' -v emit="$emit" '
    FNR == NR {
      t = $1; m = $2
      known[t] = 1
      if (m != "") model_table[m] = t
      next
    }
    function quoted_tables(s,    rest, tok) {
      delete QT
      rest = s
      while (match(rest, /"[^"]*"/)) {
        tok = substr(rest, RSTART + 1, RLENGTH - 2)
        if (tok in known) QT[tok] = 1
        rest = substr(rest, RSTART + RLENGTH)
      }
    }
    function models_on(s,    arr, n, i, a) {
      delete MD
      n = split(s, arr, /[^A-Za-z0-9_]+/)
      for (i = 1; i <= n; i++) {
        a = arr[i]
        if (a in model_table) MD[model_table[a]] = 1
      }
    }
    function flush(    i, j, t) {
      for (i = 1; i <= n; i++) {
        if (buf[i] ~ /tenant_id/) {
          delete hit
          for (j = i - 3; j <= i + 3; j++) {
            if (j < 1 || j > n) continue
            quoted_tables(buf[j])
            for (t in QT) hit[t] = 1
          }
          for (t in hit) {
            sql[t]++
            if (emit) printf("%s:%d:sql_refs:%s:%s\n", lastfile, i, t, buf[i])
          }
        }
        # A struct field declaration is model plumbing, not a usage.
        if (buf[i] ~ /(^|[^A-Za-z0-9_])TenantID([^A-Za-z0-9_]|$)/ && buf[i] !~ /^[[:space:]]*TenantID[[:space:]]+[^:]/) {
          delete hit
          for (j = i - 5; j <= i + 5; j++) {
            if (j < 1 || j > n) continue
            models_on(buf[j])
            for (t in MD) hit[t] = 1
          }
          for (t in hit) {
            field[t]++
            if (emit) printf("%s:%d:field_refs:%s:%s\n", lastfile, i, t, buf[i])
          }
        }
      }
      n = 0
    }
    FILENAME != lastfile {
      if (lastfile != "") flush()
      lastfile = FILENAME
    }
    { buf[++n] = $0 }
    END {
      flush()
      for (t in known) printf("%s|%d|%d\n", t, sql[t] + 0, field[t] + 0)
    }
  ' "$TMPWORK/count.matrix" "${files[@]}" | LC_ALL=C sort > "$out"
}

count_refs "$TMPWORK/go.nontest" "$TMPWORK/count.nontest" 0
if [ -s "$TMPWORK/go.fixture" ]; then
  count_refs "$TMPWORK/go.fixture" "$TMPWORK/count.fixture" 0
  awk -F'|' '
    FNR == NR { s[$1] = $2; f[$1] = $3; next }
    { s[$1] += $2; f[$1] += $3 }
    END { for (t in s) printf("%s|%d|%d\n", t, s[t], f[t]) }
  ' "$TMPWORK/count.nontest" "$TMPWORK/count.fixture" | LC_ALL=C sort > "$TMPWORK/count.nontest.merged"
  mv "$TMPWORK/count.nontest.merged" "$TMPWORK/count.nontest"
fi
count_refs "$TMPWORK/go.test" "$TMPWORK/count.test" 0

# ---------------------------------------------------------------------------
# 6. Global (non-attributed) counters
# ---------------------------------------------------------------------------
FRONTEND_REFS_GLOBAL=0
for d in "$ROOT/frontend-pc/src" "$ROOT/frontend-mobile/src"; do
  [ -d "$d" ] || continue
  c="$(grep -rho 'tenant_id' "$d" 2>/dev/null | wc -l | tr -d ' ')"
  FRONTEND_REFS_GLOBAL=$((FRONTEND_REFS_GLOBAL + c))
done

PLUMBING_REFS=0
for p in "$ROOT/backend/database/db.go"; do
  [ -f "$p" ] || continue
  c="$(grep -o -E 'tenant_id|TenantID' "$p" | wc -l | tr -d ' ')"
  PLUMBING_REFS=$((PLUMBING_REFS + c))
done
for d in "$ROOT/backend/middleware" "$ROOT/backend/models"; do
  [ -d "$d" ] || continue
  c="$(grep -rho -E 'tenant_id|TenantID' --include='*.go' "$d" 2>/dev/null | wc -l | tr -d ' ')"
  PLUMBING_REFS=$((PLUMBING_REFS + c))
done

( cd "$ROOT" && grep -rl 'tenant_id' docs --include='*.md' 2>/dev/null | LC_ALL=C sort ) > "$TMPWORK/doc.candidates" || true
: > "$TMPWORK/doc.refs"
# One pass over the candidate documents instead of 53 x N grep processes: a
# document counts for a table when it mentions the table name.
cut -d'|' -f1 "$TMPWORK/live.txt" | LC_ALL=C sort -u > "$TMPWORK/doc.tables"
if [ -s "$TMPWORK/doc.candidates" ]; then
  local_docs=()
  while IFS= read -r f; do [ -n "$f" ] && local_docs+=("$ROOT/$f"); done < "$TMPWORK/doc.candidates"
  if [ "${#local_docs[@]}" -gt 0 ]; then
    awk '
      FNR == NR { T[$1] = 1; next }
      FILENAME != last {
        if (last != "") for (t in seen) c[t]++
        delete seen
        last = FILENAME
      }
      { for (t in T) if (index($0, t)) seen[t] = 1 }
      END {
        for (t in seen) c[t]++
        for (t in T) printf("%s|%d\n", t, c[t] + 0)
      }
    ' "$TMPWORK/doc.tables" "${local_docs[@]}" | LC_ALL=C sort > "$TMPWORK/doc.refs"
  fi
fi
if [ ! -s "$TMPWORK/doc.refs" ]; then
  awk '{ printf("%s|0\n", $1) }' "$TMPWORK/doc.tables" > "$TMPWORK/doc.refs"
fi

# ---------------------------------------------------------------------------
# 7. Rows
# ---------------------------------------------------------------------------
: > "$TMPWORK/rows.txt"
while IFS='|' read -r t src; do
  model="$(awk -F'|' -v t="$t" '$1 == t { print $2; exit }' "$TMPWORK/table-model.txt")"
  [ -n "$model" ] || model="-"

  case "$src" in
    backup-only) env="pre-only" ;;
    *) env="pre" ;;
  esac
  grep -Fxq "$t" "$TMPWORK/post.tables" && env="post-snapshot"

  has_org="no"
  if [ "$model" != "-" ]; then
    o="$(awk -F'|' -v m="$model" -v t="$t" '$1 == t && $2 == m { print $3; exit }' "$TMPWORK/models.txt")"
    [ "$o" = "1" ] && has_org="yes"
  fi
  if [ "$has_org" = "no" ]; then
    ddl_org="$(awk -F'|' -v t="$t" '$1 == "T" && $2 == t { print $3; exit }' "$TMPWORK/ddl.txt")"
    [ "$ddl_org" = "1" ] && has_org="yes"
  fi
  if [ "$src" = "backup-only" ] && [ "$has_org" = "no" ]; then
    has_org="unknown"
  fi

  sql="$(awk -F'|' -v t="$t" '$1 == t { print $2; exit }' "$TMPWORK/count.nontest")"
  fld="$(awk -F'|' -v t="$t" '$1 == t { print $3; exit }' "$TMPWORK/count.nontest")"
  tst="$(awk -F'|' -v t="$t" '$1 == t { print $2 + $3; exit }' "$TMPWORK/count.test")"
  sql="${sql:-0}"; fld="${fld:-0}"; tst="${tst:-0}"

  doc="$(awk -F'|' -v t="$t" '$1 == t { print $2; exit }' "$TMPWORK/doc.refs")"
  doc="${doc:-0}"

  def="$(awk -F'|' -v t="$t" '$1 == "T" && $2 == t { print $4; exit }' "$TMPWORK/ddl.txt")"
  def="${def:-none}"

  idx="$(awk -F'|' -v t="$t" '$1 == t { print $2 }' "$TMPWORK/index.all" | LC_ALL=C sort -u | paste -sd, -)"
  [ -n "$idx" ] || idx="-"

  if [ "$src" = "backup-only" ]; then
    cls="backup"
  elif [ "$has_org" = "yes" ]; then
    cls="redundant"
  elif [ "$((sql + fld))" -eq 0 ]; then
    cls="metadata"
  else
    cls="anchor-candidate"
  fi

  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$t" "$src" "$model" "$has_org" "$sql" "$fld" "$tst" "$FRONTEND_REFS_GLOBAL" "$doc" "$def" "$idx" "$cls" "$env" \
    >> "$TMPWORK/rows.txt"
done < "$TMPWORK/live.txt"

CLASS_BACKUP="$(awk -F'\t' '$12 == "backup"' "$TMPWORK/rows.txt" | wc -l | tr -d ' ')"
CLASS_REDUNDANT="$(awk -F'\t' '$12 == "redundant"' "$TMPWORK/rows.txt" | wc -l | tr -d ' ')"
CLASS_METADATA="$(awk -F'\t' '$12 == "metadata"' "$TMPWORK/rows.txt" | wc -l | tr -d ' ')"
CLASS_ANCHOR="$(awk -F'\t' '$12 == "anchor-candidate"' "$TMPWORK/rows.txt" | wc -l | tr -d ' ')"
CLASS_SUM=$((CLASS_BACKUP + CLASS_REDUNDANT + CLASS_METADATA + CLASS_ANCHOR))
[ "$CLASS_SUM" = "$INV_LIVE" ] || die "CLASS_SUM_CHECK mismatch: $CLASS_SUM != $INV_LIVE"

SQL_REFS_TOTAL="$(awk -F'\t' '{ s += $5 } END { print s + 0 }' "$TMPWORK/rows.txt")"
FIELD_REFS_TOTAL="$(awk -F'\t' '{ s += $6 } END { print s + 0 }' "$TMPWORK/rows.txt")"

# ---------------------------------------------------------------------------
# 8. Whole-repo literal gate
# ---------------------------------------------------------------------------
live_literal_scan() {
  local total=0
  git -C "$ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1 \
    || die "not a git repository: cannot enumerate tracked files"

  : > "$TMPWORK/live.raw"
  : > "$TMPWORK/live.dep"
  : > "$TMPWORK/live.hits"

  # Tracked files only; the allowlist is expressed as pathspec exclusions so the
  # scan stays a single deterministic git operation instead of N process pairs.
  local -a exclude=(
    ':(exclude)backend/database/migrations/**'
    ':(exclude)docs/**'
    ":(exclude)$SELF_REL"
    ":(exclude)$VERIFY_REL"
    ":(exclude)$TESTS_REL"
  )
  git -C "$ROOT" grep -I -o --full-name 'tenant_id' -- . "${exclude[@]}" > "$TMPWORK/live.raw" 2>/dev/null || true
  git -C "$ROOT" grep -I -o --full-name 'tenant_id_deprecated' -- . "${exclude[@]}" > "$TMPWORK/live.dep" 2>/dev/null || true

  awk -F: '{ c[$1]++ } END { for (f in c) printf("%s|%d\n", f, c[f]) }' "$TMPWORK/live.raw" \
    | LC_ALL=C sort > "$TMPWORK/live.raw.perFile"
  awk -F: '{ c[$1]++ } END { for (f in c) printf("%s|%d\n", f, c[f]) }' "$TMPWORK/live.dep" \
    | LC_ALL=C sort > "$TMPWORK/live.dep.perFile"

  # `tenant_id_deprecated` embeds the literal once, so subtract per file. The
  # raw file is read first: with a single (possibly empty) dep file the
  # `FNR == NR` idiom would otherwise swallow every record.
  awk -F'|' '
    FNR == NR { raw[$1] = $2; next }
    { dep[$1] = $2 }
    END {
      for (f in raw) {
        n = raw[f] - (dep[f] + 0)
        if (n > 0) printf("%s|%d\n", f, n)
      }
    }
  ' "$TMPWORK/live.raw.perFile" "$TMPWORK/live.dep.perFile" | LC_ALL=C sort > "$TMPWORK/live.hits"

  total="$(awk -F'|' '{ s += $2 } END { print s + 0 }' "$TMPWORK/live.hits")"
  printf '%s\n' "$total"
}

emit_tokens() {
  cat <<EOF
# INV_SNAPSHOT=$SNAPSHOT_EXPECTED
# INV_PRE_ONLY=$INV_PRE_ONLY
# INV_LIVE=$INV_LIVE
# POST_SNAPSHOT_TABLES=$POST_SNAPSHOT_TABLES
# DROPPED_IN_SNAPSHOT=$DROPPED_IN_SNAPSHOT
# CLASS_BACKUP=$CLASS_BACKUP
# CLASS_REDUNDANT=$CLASS_REDUNDANT
# CLASS_METADATA=$CLASS_METADATA
# CLASS_ANCHOR=$CLASS_ANCHOR
# CLASS_SUM_CHECK=$CLASS_SUM
# MODEL_STRUCTS=$MODEL_STRUCTS
# MODEL_TABLE_ROWS=$MODEL_TABLE_ROWS
# MODEL_NO_TABLE=$MODEL_NO_TABLE
# MODEL_POST_SNAPSHOT=$MODEL_POST_SNAPSHOT
# PLUMBING_REFS=$PLUMBING_REFS
# FRONTEND_REFS_GLOBAL=$FRONTEND_REFS_GLOBAL
# SQL_REFS_TOTAL=$SQL_REFS_TOTAL
# FIELD_REFS_TOTAL=$FIELD_REFS_TOTAL
# LIVE_LITERAL_BASE=$LIVE_LITERAL_BASE
# DETERMINISTIC=1
EOF
}

# ---------------------------------------------------------------------------
# 9. Modes
# ---------------------------------------------------------------------------
case "$MODE" in
  require-zero)
    [ -n "$TARGET_TABLE" ] || die "--table <T> is required with --require-zero"
    row="$(awk -F'\t' -v t="$TARGET_TABLE" '$1 == t' "$TMPWORK/rows.txt")"
    [ -n "$row" ] || die "unknown table: $TARGET_TABLE (not in live universe)"
    # Zero means zero everywhere: SQL literals + field accesses in non-test Go,
    # the red/green fixture, and *_test.go (test fixtures break the build too).
    read -r r_sql r_fld r_tst <<<"$(printf '%s' "$row" | awk -F'\t' '{ print $5, $6, $7 }')"
    n=$((r_sql + r_fld + r_tst))
    echo "REQUIRE_ZERO_TABLE=$TARGET_TABLE"
    echo "REQUIRE_ZERO_SQL_REFS=$r_sql"
    echo "REQUIRE_ZERO_FIELD_REFS=$r_fld"
    echo "REQUIRE_ZERO_TEST_REFS=$r_tst"
    echo "REQUIRE_ZERO_REFS=$n"
    echo "REQUIRE_ZERO_EXIT=$([ "$n" -eq 0 ] && echo 0 || echo 1)"
    if [ "$n" -eq 0 ]; then
      exit 0
    fi
    count_refs "$TMPWORK/go.nontest" "$TMPWORK/hits.nontest" 1
    : > "$TMPWORK/hits.all"
    cat "$TMPWORK/hits.nontest" >> "$TMPWORK/hits.all"
    if [ -s "$TMPWORK/go.fixture" ]; then
      count_refs "$TMPWORK/go.fixture" "$TMPWORK/hits.fixture" 1
      cat "$TMPWORK/hits.fixture" >> "$TMPWORK/hits.all"
    fi
    count_refs "$TMPWORK/go.test" "$TMPWORK/hits.test" 1
    cat "$TMPWORK/hits.test" >> "$TMPWORK/hits.all"
    grep -F ":$TARGET_TABLE:" "$TMPWORK/hits.all" 2>/dev/null | sed "s#^$ROOT/##" | LC_ALL=C sort | head -200
    exit 1
    ;;

  live-literal)
    LIVE_LITERAL_BASE="$(live_literal_scan)"
    echo "LIVE_LITERAL_BASE=$LIVE_LITERAL_BASE"
    echo "LIVE_LITERAL_ALLOWLIST=tenant_id_deprecated,backend/database/migrations/**,docs/**,$SELF_REL,$VERIFY_REL,$TESTS_REL"
    echo "LIVE_LITERAL_FILES=$(wc -l < "$TMPWORK/live.hits" | tr -d ' ')"
    LC_ALL=C sort -t'|' -k2 "$TMPWORK/live.hits" | awk -F'|' '{ printf("LIVE_LITERAL_FILE=%s|%s\n", $2, $1) }'
    if [ "$LIVE_LITERAL_BASE" -gt 0 ]; then
      echo "LIVE_LITERAL_EXIT=1"
      exit 1
    fi
    echo "LIVE_LITERAL_EXIT=0"
    exit 0
    ;;

  check-doc)
    [ -n "$DOC_PATH" ] || die "--check-doc requires a path"
    doc="$DOC_PATH"
    case "$doc" in
      /*) : ;;
      *) doc="$ROOT/$doc" ;;
    esac
    [ -f "$doc" ] || die "document not found: $DOC_PATH"

    awk '
      function trim(s) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", s); return s }
      # Matrix = the contiguous table block whose header declares table/source/model.
      !inmatrix && /^\|/ && /[|][[:space:]]*table[[:space:]]*\|/ && /[|][[:space:]]*source[[:space:]]*\|/ && /[|][[:space:]]*model[[:space:]]*\|/ {
        inmatrix = 1
        ncell = split($0, cells, "|")
        for (i = 1; i <= ncell; i++) {
          c = trim(cells[i])
          if (c == "class") classcol = i
        }
        next
      }
      inmatrix && !/^\|/ { inmatrix = 0 }
      inmatrix && /^\|/ {
        line = $0
        if (line ~ /^\|[-: ]+\|/) next          # separator row
        ncell = split(line, cells, "|")
        t = cells[2]
        gsub(/[`[:space:]]/, "", t)
        if (t !~ /^[a-z0-9_]+$/) next
        rowcount++
        if (classcol > 0) {
          cv = trim(cells[classcol])
          if (cv == "backup" || cv == "redundant" || cv == "metadata" || cv == "anchor-candidate") classok++
          else badclass = badclass " " t
        }
      }
      END {
        printf("DOCTYPE_TABLES=%d\n", rowcount + 0)
        printf("DOC_CLASS_CELLS=%d\n", classok + 0)
        if (badclass != "") printf("DOC_BAD_CLASS=%s\n", badclass)
      }
    ' "$doc" > "$TMPWORK/doc.stats"

    DOC_TABLES="$(sed -n 's/^DOCTYPE_TABLES=//p' "$TMPWORK/doc.stats")"
    DOC_CLASS_CELLS="$(sed -n 's/^DOC_CLASS_CELLS=//p' "$TMPWORK/doc.stats")"
    DOC_TABLES="${DOC_TABLES:-0}"
    DOC_CLASS_CELLS="${DOC_CLASS_CELLS:-0}"

    DOC_MISSING=0
    missing=""
    while IFS= read -r t; do
      [ -n "$t" ] || continue
      if ! grep -qE "^\|[[:space:]]*\`$t\`" "$doc"; then
        DOC_MISSING=$((DOC_MISSING + 1))
        missing="$missing $t"
      fi
    done < "$TMPWORK/tables.only"
    while IFS= read -r t; do
      [ -n "$t" ] || continue
      if ! grep -qF "\`$t\`" "$doc"; then
        DOC_MISSING=$((DOC_MISSING + 1))
        missing="$missing $t"
      fi
    done < "$TMPWORK/post.tables"

    DOC_LISTS=0
    grep -q '可立即处置' "$doc" && DOC_LISTS=$((DOC_LISTS + 1))
    grep -q '需语义裁定' "$doc" && DOC_LISTS=$((DOC_LISTS + 1))

    DOC_CLASS_COVERED=0
    if [ "$DOC_TABLES" -gt 0 ] && [ "$DOC_CLASS_CELLS" -eq "$DOC_TABLES" ]; then
      DOC_CLASS_COVERED=1
    fi

    DOC_DB_BACKLOG=0
    if grep -q 'information_schema.columns' "$doc" && grep -q 'pg_indexes' "$doc" && grep -q 'pg_constraint' "$doc"; then
      DOC_DB_BACKLOG=1
    fi

    echo "DOC_TABLES=$DOC_TABLES"
    echo "DOC_MISSING=$DOC_MISSING"
    echo "DOC_LISTS=$DOC_LISTS"
    echo "DOC_CLASS_COVERED=$DOC_CLASS_COVERED"
    echo "DOC_DB_BACKLOG=$DOC_DB_BACKLOG"
    [ -n "$missing" ] && echo "DOC_MISSING_TABLES=$missing"

    if [ "$DOC_TABLES" -ne "$SNAPSHOT_EXPECTED" ] || [ "$DOC_MISSING" -ne 0 ] \
       || [ "$DOC_LISTS" -ne 2 ] || [ "$DOC_CLASS_COVERED" -ne 1 ] || [ "$DOC_DB_BACKLOG" -ne 1 ]; then
      exit 1
    fi
    exit 0
    ;;

  json)
    LIVE_LITERAL_BASE="$(live_literal_scan)"
    printf '{\n'
    printf '  "inventory": {\n'
    printf '    "snapshot": %d,\n' "$SNAPSHOT_EXPECTED"
    printf '    "pre_only": %d,\n' "$INV_PRE_ONLY"
    printf '    "live": %d,\n' "$INV_LIVE"
    printf '    "post_snapshot_tables": %d,\n' "$POST_SNAPSHOT_TABLES"
    printf '    "dropped_in_snapshot": %d,\n' "$DROPPED_IN_SNAPSHOT"
    printf '    "model_structs": %d,\n' "$MODEL_STRUCTS"
    printf '    "model_table_rows": %d,\n' "$MODEL_TABLE_ROWS"
    printf '    "model_no_table": %d,\n' "$MODEL_NO_TABLE"
    printf '    "model_post_snapshot": %d,\n' "$MODEL_POST_SNAPSHOT"
    printf '    "plumbing_refs": %d,\n' "$PLUMBING_REFS"
    printf '    "frontend_refs_global": %d,\n' "$FRONTEND_REFS_GLOBAL"
    printf '    "sql_refs_total": %d,\n' "$SQL_REFS_TOTAL"
    printf '    "field_refs_total": %d,\n' "$FIELD_REFS_TOTAL"
    printf '    "live_literal_base": %d,\n' "$LIVE_LITERAL_BASE"
    printf '    "classes": {"backup": %d, "redundant": %d, "metadata": %d, "anchor-candidate": %d}\n' \
      "$CLASS_BACKUP" "$CLASS_REDUNDANT" "$CLASS_METADATA" "$CLASS_ANCHOR"
    printf '  },\n'
    printf '  "tables": [\n'
    awk -F'\t' '
      BEGIN { first = 1 }
      {
        if (!first) printf(",\n")
        first = 0
        idx = $11; gsub(/"/, "", idx)
        printf("    {\"table\":\"%s\",\"source\":\"%s\",\"model\":\"%s\",\"has_org_id\":\"%s\",\"sql_refs\":%s,\"field_refs\":%s,\"test_refs\":%s,\"frontend_refs\":%s,\"doc_refs\":%s,\"db_default\":\"%s\",\"index_names\":\"%s\",\"class\":\"%s\",\"env\":\"%s\"}", \
          $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, idx, $12, $13)
      }
      END { printf("\n") }
    ' "$TMPWORK/rows.txt"
    printf '  ]\n'
    printf '}\n'
    exit 0
    ;;

  tsv|*)
    LIVE_LITERAL_BASE="$(live_literal_scan)"
    printf 'table\tsource\tmodel\thas_org_id\tsql_refs\tfield_refs\ttest_refs\tfrontend_refs\tdoc_refs\tdb_default\tindex_names\tclass\tenv\n'
    cat "$TMPWORK/rows.txt"
    emit_tokens
    ;;
esac
