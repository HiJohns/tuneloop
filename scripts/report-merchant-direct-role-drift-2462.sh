#!/usr/bin/env bash
# #2462 存量漂移清单：merchant_members.role = site_member 且无 site_members 行的直属成员。
#
# 只读（默认）：输出 ACTION=align / ACTION=none，不做任何写库。
# --apply 必须显式 Owner 授权：OWNER_APPROVED_MERCHANT_DIRECT_ROLE=1，否则 exit 2。
#
# 用法：
#   bash scripts/report-merchant-direct-role-drift-2462.sh [--report|--apply]
#   bash scripts/report-merchant-direct-role-drift-2462.sh --fixture <tsv>
#
# fixture 行格式（TSV，5 列）：
#   user_id <TAB> mm_role <TAB> users_role <TAB> merchant_id <TAB> has_site_member(0|1)
set -uo pipefail

MODE="report"
FIXTURE=""
while [ $# -gt 0 ]; do
  case "$1" in
    --report) MODE="report"; shift ;;
    --apply) MODE="apply"; shift ;;
    --fixture) FIXTURE="${2:-}"; shift 2 ;;
    -h|--help)
      sed -n '2,14p' "$0"
      exit 0
      ;;
    *)
      echo "usage: $0 [--report|--apply] [--fixture <tsv>]" >&2
      exit 2
      ;;
  esac
done

TARGET_ROLE="merchant_staff"
DB_WRITES=0

if [ "$MODE" = "apply" ]; then
  if [ "${OWNER_APPROVED_MERCHANT_DIRECT_ROLE:-}" != "1" ]; then
    echo "UNAUTHORIZED_APPLY=1"
    echo "DB_WRITES=0"
    echo "READONLY=1"
    echo "RESULT: FAIL (owner approval required: OWNER_APPROVED_MERCHANT_DIRECT_ROLE=1)"
    exit 2
  fi
fi

# 数据来源：fixture（可测，不触库）或真实只读查询。
rows_file=""
cleanup() { [ -n "$rows_file" ] && rm -f "$rows_file"; }
trap cleanup EXIT

if [ -n "$FIXTURE" ]; then
  [ -f "$FIXTURE" ] || { echo "fixture not found: $FIXTURE" >&2; exit 2; }
  rows_file="$(mktemp "${TMPDIR:-/tmp}/drift-2462.XXXXXX")" || exit 2
  cp "$FIXTURE" "$rows_file"
else
  command -v psql >/dev/null 2>&1 || { echo "psql not found; use --fixture or install psql" >&2; exit 2; }
  rows_file="$(mktemp "${TMPDIR:-/tmp}/drift-2462.XXXXXX")" || exit 2
  PGPASSWORD="${POSTGRES_PASSWORD:-}" psql \
    -h "${POSTGRES_HOST:-127.0.0.1}" -p "${POSTGRES_PORT:-5432}" \
    -U "${POSTGRES_USER:-tuneloop_user}" -d "${POSTGRES_DB:-tuneloop}" \
    -At -F $'\t' -c "
      SELECT mm.user_id::text,
             mm.role,
             COALESCE(u.role, ''),
             mm.merchant_id::text,
             CASE WHEN EXISTS (SELECT 1 FROM site_members sm WHERE sm.user_id = mm.user_id) THEN '1' ELSE '0' END
      FROM merchant_members mm
      LEFT JOIN users u ON u.id = mm.user_id
      ORDER BY mm.user_id
    " > "$rows_file" || { echo "drift query failed" >&2; exit 2; }
fi

align_rows=0
none_rows=0
while IFS=$'\t' read -r user_id mm_role users_role merchant_id has_site_member; do
  [ -n "${user_id:-}" ] || continue
  if [ "${mm_role:-}" = "site_member" ] && [ "${has_site_member:-0}" = "0" ]; then
    align_rows=$((align_rows + 1))
    echo "ACTION=align user_id=${user_id} mm_role=${mm_role} users_role=${users_role:-} merchant_id=${merchant_id:-}"
    if [ "$MODE" = "apply" ]; then
      if PGPASSWORD="${POSTGRES_PASSWORD:-}" psql \
        -h "${POSTGRES_HOST:-127.0.0.1}" -p "${POSTGRES_PORT:-5432}" \
        -U "${POSTGRES_USER:-tuneloop_user}" -d "${POSTGRES_DB:-tuneloop}" \
        -q -c "UPDATE merchant_members SET role = '${TARGET_ROLE}', updated_at = now() WHERE user_id = '${user_id}' AND role = 'site_member'" >/dev/null; then
        DB_WRITES=$((DB_WRITES + 1))
      else
        echo "APPLY_FAILED user_id=${user_id}" >&2
      fi
    fi
  else
    none_rows=$((none_rows + 1))
    echo "ACTION=none user_id=${user_id} mm_role=${mm_role} users_role=${users_role:-} merchant_id=${merchant_id:-}"
  fi
done < "$rows_file"

echo "ALIGN_ROWS=${align_rows}"
echo "NONE_ROWS=${none_rows}"
echo "DB_WRITES=${DB_WRITES}"
if [ "$MODE" = "report" ] || [ "$DB_WRITES" -eq 0 ]; then
  echo "READONLY=1"
else
  echo "READONLY=0"
fi
echo "RESULT: PASS"
