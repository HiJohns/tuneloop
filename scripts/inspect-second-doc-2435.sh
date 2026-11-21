#!/usr/bin/env bash
# #2435: list accounts that have a second-doc photo but no type.
# Usage: bash scripts/inspect-second-doc-2435.sh [--fail-if-any]
# Requires psql + POSTGRES_* / TUNELOOP_DB (or .env).
set -uo pipefail
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
set -a; [ -f backend/.env ] && . backend/.env; set +a
DB="${TUNELOOP_DB:-tuneloop_debug}"
SQL="SELECT id, username, name, phone, id_photo_other FROM users WHERE COALESCE(id_photo_other,'') <> '' AND COALESCE(id_photo_other_type,'') = '' ORDER BY created_at DESC;"
if ! command -v psql >/dev/null 2>&1; then
  echo "psql not found; run against $DB:" >&2
  echo "$SQL" >&2
  exit 2
fi
ROWS=$(PGPASSWORD="${POSTGRES_PASSWORD:-}" psql -h "${POSTGRES_HOST:-127.0.0.1}" -p "${POSTGRES_PORT:-5432}" \
  -U "${POSTGRES_USER:-tuneloop_user}" -d "$DB" -Atc "$SQL")
[ -n "$ROWS" ] && printf '%s\n' "$ROWS"
if [ "${1:-}" = "--fail-if-any" ] && [ -n "$ROWS" ]; then
  echo "SECOND_DOC_UNSET_COUNT=$(printf '%s\n' "$ROWS" | wc -l)" >&2
  exit 1
fi
exit 0
