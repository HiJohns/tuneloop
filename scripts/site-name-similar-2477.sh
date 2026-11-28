#!/usr/bin/env bash
# #2477 — read-only inventory of site (网点) names that collide *after* the
# #2477 normalization (trim + whitespace folding, incl. U+3000 full-width space).
#
# Purpose: the duplicate-name authority is IAM (Owner ruling B, 2026-11-26), so
# this script NEVER participates in any decision and NEVER writes. It only lists
# the existing rows that would normalize to the same string inside one tenant,
# for ops to eyeball before deciding whether to rename anything.
#
# Usage:
#   scripts/site-name-similar-2477.sh                     # query the DB (SELECT only)
#   scripts/site-name-similar-2477.sh --source <file.tsv> # offline rows (tests / evidence)
#
# TSV row layouts accepted by --source:
#   <tenant_id>\t<name>
#   <id>\t<tenant_id>\t<name>
#
# Exit codes: 0 = ok (even when nothing collides), 2 = usage, 3 = no reader
# available (no --source and no psql). Never exits non-zero because of a hit.

set -uo pipefail

READONLY=1

SOURCE=""
while [ $# -gt 0 ]; do
  case "$1" in
    --source)
      [ $# -ge 2 ] || { echo "ERROR:--source requires a file" >&2; exit 2; }
      SOURCE="$2"
      shift 2
      ;;
    --source=*)
      SOURCE="${1#--source=}"
      shift
      ;;
    -h|--help)
      sed -n '2,20p' "$0"
      exit 0
      ;;
    *)
      echo "ERROR:unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

work_file=""
cleanup() {
  [ -n "$work_file" ] && rm -f "$work_file"
}
trap cleanup EXIT

if [ -n "$SOURCE" ]; then
  [ -r "$SOURCE" ] || { echo "ERROR:source not readable: $SOURCE" >&2; exit 2; }
  work_file="$SOURCE"
else
  # Read-only pull. Only SELECT is ever issued: this script must be safe to run
  # against a live environment (Owner authorizes any cleanup separately).
  PSQL_BIN="${PSQL_BIN:-psql}"
  if ! command -v "$PSQL_BIN" >/dev/null 2>&1; then
    echo "ERROR:no reader available: pass --source <tsv> or set PSQL_BIN" >&2
    exit 3
  fi
  work_file="$(mktemp "${TMPDIR:-/tmp}/site-name-similar-2477.XXXXXX")"
  if ! "$PSQL_BIN" \
      --no-psqlrc --quiet --tuples-only --no-align --field-separator=$'\t' \
      -v ON_ERROR_STOP=1 \
      -c "SELECT id, tenant_id, name FROM sites WHERE deleted_at IS NULL ORDER BY tenant_id, name" \
      >"$work_file"; then
    echo "ERROR:read-only SELECT failed (no data written)" >&2
    exit 3
  fi
fi

PYTHONIOENCODING=utf-8 python3 - "$work_file" <<'PY'
import collections
import re
import sys

# Whitespace folding must match backend/handlers/site_name_2477.go: every run of
# Unicode whitespace (incl. U+3000) collapses to one ASCII space, then trim.
_WS = re.compile(r"\s+", re.UNICODE)


def normalize(name: str) -> str:
    return _WS.sub(" ", name).strip()


rows = []
with open(sys.argv[1], encoding="utf-8") as handle:
    for raw in handle:
        line = raw.rstrip("\n")
        if not line.strip():
            continue
        parts = line.split("\t")
        if len(parts) >= 3:
            row_id, tenant_id, name = parts[0], parts[1], parts[2]
        elif len(parts) == 2:
            row_id, tenant_id, name = "", parts[0], parts[1]
        else:
            continue
        rows.append((row_id.strip(), tenant_id.strip(), name))

groups = collections.defaultdict(list)
for row_id, tenant_id, name in rows:
    if not tenant_id:
        continue
    groups[(tenant_id, normalize(name))].append((row_id, name))

collisions = {key: items for key, items in groups.items() if len(items) > 1}

print("READONLY=1")
print("TOTAL_ROWS=%d" % len(rows))
print("COLLIDING_GROUPS=%d" % len(collisions))
print("COLLIDING_ROWS=%d" % sum(len(v) for v in collisions.values()))

for (tenant_id, key), items in sorted(collisions.items()):
    print("GROUP tenant=%s key=%r count=%d" % (tenant_id, key, len(items)))
    for row_id, name in sorted(items, key=lambda pair: pair[1]):
        print("  ROW id=%s name=%r" % (row_id or "-", name))

if not collisions:
    print("NO_COLLISION=1")
PY
