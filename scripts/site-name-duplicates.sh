#!/usr/bin/env bash
# #2455 存量同商户网点重名清单（只读报告，供 Owner 决定改名/合并，不静默改名）。
#
# 用途：迁移 20261123002 在同商户存在同名网点时会 fail-fast 并报错；
#       本脚本给出**完整清单**（tenant / lower(name) / site ids），供 ops 按
#       Owner 授权逐条处理（D3：方向 IAM→本地，执行与回执属部署侧）。
#
# 只读：仅 SELECT，无任何写库语句；不提供写模式（无 --apply）。
#
# 用法：
#   bash scripts/site-name-duplicates.sh --report                  # 只读查库
#   bash scripts/site-name-duplicates.sh --report --fixture <tsv>  # 用 fixture（可测，不触库）
#   bash scripts/site-name-duplicates.sh --dsn "postgres://..."    # 显式 DSN
#
# fixture 行格式（TSV，3 列）：tenant_id <TAB> name <TAB> id
# 只有 deleted_at IS NULL 的行参与判定（与唯一索引谓词一致）。
#
# 输出（机器可判定）：
#   DUP_GROUPS=<组数>                  同商户 lower(name) 重复的组数
#   DUP tenant_id=… name=… ids=… count=…  每组明细（ids 升序逗号分隔）
#   READONLY=1 / DB_WRITES=0 / RESULT: OK
set -uo pipefail

MODE="report"
FIXTURE=""
DSN="${SITE_NAME_DUP_DSN:-${DATABASE_URL:-}}"
THRESHOLD=2

while [ $# -gt 0 ]; do
  case "$1" in
    --report) MODE="report"; shift ;;
    --fixture) FIXTURE="${2:-}"; shift 2 ;;
    --dsn) DSN="${2:-}"; shift 2 ;;
    --threshold) THRESHOLD="${2:-2}"; shift 2 ;;
    -h|--help) sed -n '2,22p' "$0"; exit 0 ;;
    *)
      echo "usage: $0 --report [--fixture <tsv>] [--dsn <dsn>]" >&2
      exit 2 ;;
  esac
done

[ "$MODE" = "report" ] || { echo "unsupported mode: $MODE" >&2; exit 2; }
[[ "$THRESHOLD" =~ ^[0-9]+$ ]] || { echo "invalid --threshold: $THRESHOLD" >&2; exit 2; }

rows_file=""
cleanup() { [ -n "$rows_file" ] && rm -f "$rows_file"; }
trap cleanup EXIT
rows_file="$(mktemp "${TMPDIR:-/tmp}/site-name-dup-2455.XXXXXX")" || exit 2

# ---------------------------------------------------------------- data source
if [ -n "$FIXTURE" ]; then
  [ -f "$FIXTURE" ] || { echo "fixture not found: $FIXTURE" >&2; exit 2; }
  awk -F'\t' 'NF >= 3 && $1 !~ /^#/ && $1 != "" { print $1 "\t" $2 "\t" $3 }' \
    "$FIXTURE" > "$rows_file" || exit 2
else
  command -v psql >/dev/null 2>&1 || {
    echo "psql not found; use --fixture or install psql" >&2
    exit 2
  }
  # 只读查询：软删除外的全部网点行，分组口径与唯一索引一致（tenant_id + lower(name)）。
  query="SELECT tenant_id::text, name, id::text FROM sites WHERE deleted_at IS NULL"
  if [ -n "$DSN" ]; then
    psql "$DSN" -v ON_ERROR_STOP=1 -At -F $'\t' -c "$query" > "$rows_file" || exit 2
  else
    PGPASSWORD="${POSTGRES_PASSWORD:-}" psql \
      -h "${POSTGRES_HOST:-127.0.0.1}" -p "${POSTGRES_PORT:-5432}" \
      -U "${POSTGRES_USER:-tuneloop_user}" -d "${POSTGRES_DB:-tuneloop}" \
      -v ON_ERROR_STOP=1 -At -F $'\t' -c "$query" > "$rows_file" || exit 2
  fi
fi

# ---------------------------------------------- deterministic grouping (POSIX awk)
report="$(awk -F'\t' '{ print tolower($1) "\t" tolower($2) "\t" $3 }' "$rows_file" \
  | LC_ALL=C sort -t $'\t' -k1,1 -k2,2 -k3,3 \
  | awk -F'\t' -v threshold="$THRESHOLD" '
      function flush() {
        if (prev != "" && n >= threshold) {
          groups++
          lines[groups] = "DUP tenant_id=" prev_t " name=" prev_n " ids=" ids " count=" n
        }
      }
      {
        key = $1 "\t" $2
        if (key != prev) {
          flush()
          prev = key; prev_t = $1; prev_n = $2; ids = $3; n = 1
        } else {
          ids = ids "," $3; n++
        }
      }
      END {
        flush()
        print "DUP_GROUPS=" (groups + 0)
        for (i = 1; i <= groups; i++) print lines[i]
      }')" || { echo "grouping failed" >&2; exit 2; }

printf '%s\n' "$report"
echo "READONLY=1"
echo "DB_WRITES=0"
echo "THRESHOLD=$THRESHOLD"
echo "RESULT: OK"
