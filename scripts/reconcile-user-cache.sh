#!/usr/bin/env bash
# #2623 (L1) 用户缓存只读对账：把 tuneloop 本地 users 缓存与 IAM 权威关系比对，
# 输出 6 类差异分类 + 机器可读 token（供 #2624 修复、#2625 例行消费）。
#
# 只读：本脚本不包含任何写入语句；真实库模式在连接后立即
# `SET default_transaction_read_only = on`，输出恒为 DB_WRITES=0。
#
# 用法：
#   bash scripts/reconcile-user-cache.sh [--json] [--limit N]
#   bash scripts/reconcile-user-cache.sh --fixture <dir>          # iam.tsv + local.tsv
#   bash scripts/reconcile-user-cache.sh --iam-fixture <tsv> --local-fixture <tsv>
#
# 真实库 env（脚本内零凭据）：
#   IAM 侧：BEACONIAM_DB_HOST / BEACONIAM_DB_PORT / BEACONIAM_DB_USER /
#           BEACONIAM_DB_PASSWORD / BEACONIAM_DB_NAME（或 BEACONIAM_DB）
#   本地侧：TUNELOOP_DB_HOST / TUNELOOP_DB_PORT / TUNELOOP_DB_USER /
#           TUNELOOP_DB_PASSWORD / TUNELOOP_DB_NAME（或 TUNELOOP_DB）
#   两库可同名不同 host —— 分别配置，绝不假设同一连接。
#
# fixture 输入（TSV，# 注释行与空行忽略）：
#   iam.tsv   6 列：user_id  org_id  role  functional_roles(逗号分隔或空)  is_active(0|1)  org_parent_id(空=根)
#   local.tsv 6 列：iam_sub  user_id  tenant_id  org_id  role  status
#
# 退出码：0 = 比对完成（有差异也算 0）；2 = 参数/连接/env 错误；3 = 输入损坏。
set -uo pipefail

ZERO_UUID="00000000-0000-0000-0000-000000000000"
MAX_ORG_HOPS=64
DEFAULT_LIMIT=20

READONLY_GUARD="SET default_transaction_read_only = on"

MODE_JSON=0
LIMIT="$DEFAULT_LIMIT"
IAM_FIXTURE=""
LOCAL_FIXTURE=""
FIXTURE_DIR=""

usage() {
  sed -n '2,30p' "$0"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --json) MODE_JSON=1; shift ;;
    --limit) LIMIT="${2:-}"; shift 2 ;;
    --fixture) FIXTURE_DIR="${2:-}"; shift 2 ;;
    --iam-fixture) IAM_FIXTURE="${2:-}"; shift 2 ;;
    --local-fixture) LOCAL_FIXTURE="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "RESULT: FAIL (unknown argument: $1)" >&2; exit 2 ;;
  esac
done

case "$LIMIT" in
  ''|*[!0-9]*) echo "RESULT: FAIL (--limit must be a non-negative integer)" >&2; exit 2 ;;
esac
if [ -n "$FIXTURE_DIR" ]; then
  [ -n "$IAM_FIXTURE" ] || IAM_FIXTURE="$FIXTURE_DIR/iam.tsv"
  [ -n "$LOCAL_FIXTURE" ] || LOCAL_FIXTURE="$FIXTURE_DIR/local.tsv"
fi

RECONCILE_DATE="$(date +%F)"
DB_WRITES=0
EXIT_CODE=0

# --------------------------------------------------------------- fail helpers
# 失败输出只打印一次（stdout）：消费方与测试按行匹配，重复行会让计数断言失真。
fail_with() { # code message
  local code="$1"; shift
  echo "RECONCILE_DATE=$RECONCILE_DATE"
  echo "DB_WRITES=$DB_WRITES"
  echo "RESULT: FAIL ($*)"
  exit "$code"
}

bad_input() { # message
  echo "FIXTURE_INVALID=1"
  fail_with 3 "$@"
}

# ------------------------------------------------------------------- env / db
iam_env_value() { # suffix
  local suffix="$1"
  case "$suffix" in
    NAME)
      printf '%s' "${BEACONIAM_DB_NAME:-${BEACONIAM_DB:-}}"
      ;;
    *)
      local upper="BEACONIAM_DB_$suffix"
      printf '%s' "${!upper:-}"
      ;;
  esac
}

local_env_value() { # suffix
  local suffix="$1"
  case "$suffix" in
    NAME)
      printf '%s' "${TUNELOOP_DB_NAME:-${TUNELOOP_DB:-}}"
      ;;
    *)
      local upper="TUNELOOP_DB_$suffix"
      printf '%s' "${!upper:-}"
      ;;
  esac
}

psql_select() { # host port user password db sql -> rows on stdout (tab separated)
  local host="$1" port="$2" user="$3" password="$4" db="$5" sql="$6"
  PGOPTIONS="-c default_transaction_read_only=on" \
  PGPASSWORD="$password" \
  psql -X -q -A -t -F $'\t' -v ON_ERROR_STOP=1 \
       -h "$host" -p "$port" -U "$user" -d "$db" \
       -c "$READONLY_GUARD" -c "$sql" 2>&1
}

# --------------------------------------------------------------- parsing input
declare -A ORG_PARENT=()
declare -A ORG_PARENT_CONFLICT=()
declare -A REL_BY_USER=()
declare -A IAM_USER=()
declare -A LOCAL_BY_SUB=()
declare -A LOCAL_DUP=()

IAM_USERS_KNOWN=0
LOCAL_ROWS_TOTAL=0
IAM_SUB_EMPTY=0

load_org_parent() { # org_id parent_id
  local org="$1" parent="$2"
  [ -n "$org" ] || return 0
  if [ -n "${ORG_PARENT[$org]+set}" ]; then
    [ "${ORG_PARENT[$org]}" = "$parent" ] || ORG_PARENT_CONFLICT["$org"]=1
  else
    ORG_PARENT["$org"]="$parent"
  fi
}

load_iam_line() { # line source_label
  # 逐字段用 cut -f（分隔符语义），不能用 IFS=<TAB> read：
  # TAB 属 IFS 空白，空字段会被折叠导致字段左移（#2484 同类陷阱）。
  local line="$1" label="$2" user org role froles active parent fields
  fields="$(printf '%s' "$line" | awk -F'\t' '{print NF}')"
  if [ "$fields" != "6" ]; then
    echo "ROW_FIELDS=$fields"
    bad_input "malformed $label row: expected 6 tab-separated fields, got $fields"
  fi
  user="$(printf '%s' "$line" | cut -f1)"
  org="$(printf '%s' "$line" | cut -f2)"
  role="$(printf '%s' "$line" | cut -f3)"
  froles="$(printf '%s' "$line" | cut -f4)"
  active="$(printf '%s' "$line" | cut -f5)"
  parent="$(printf '%s' "$line" | cut -f6)"
  if [ -z "$user" ] || [ -z "$org" ]; then
    bad_input "malformed $label row: empty user_id or org_id"
  fi
  case "$active" in
    0|1) ;;
    *) bad_input "malformed $label row: is_active must be 0|1 (got '$active')" ;;
  esac
  if ! printf '%s' "$role" | grep -qE '^[A-Za-z][A-Za-z0-9_]*$'; then
    bad_input "malformed $label row: role must be a non-empty identifier (got '$role')"
  fi
  [ "$active" = "1" ] || return 0
  IAM_USER["$user"]=1
  REL_BY_USER["$user"]="${REL_BY_USER[$user]:-}${org}|${role}|${froles}"$'\n'
  load_org_parent "$org" "$parent"
}

load_local_line() { # line source_label
  local line="$1" label="$2" sub uid tenant org role status email fields
  fields="$(printf '%s' "$line" | awk -F'\t' '{print NF}')"
  case "$fields" in
    6) email="" ;;
    7) email="$(printf '%s' "$line" | awk -F'\t' '{print $7}')" ;;
    *)
      echo "ROW_FIELDS=$fields"
      bad_input "malformed $label row: expected 6 or 7 tab-separated fields, got $fields"
      ;;
  esac
  sub="$(printf '%s' "$line" | cut -f1)"
  uid="$(printf '%s' "$line" | cut -f2)"
  tenant="$(printf '%s' "$line" | cut -f3)"
  org="$(printf '%s' "$line" | cut -f4)"
  role="$(printf '%s' "$line" | cut -f5)"
  status="$(printf '%s' "$line" | cut -f6)"
  if [ -z "$sub" ]; then
    IAM_SUB_EMPTY=$((IAM_SUB_EMPTY + 1))
    return 0
  fi
  [ -n "$tenant" ] || bad_input "malformed $label row: empty tenant_id for iam_sub '$sub'"
  [ -n "$org" ] || bad_input "malformed $label row: empty org_id for iam_sub '$sub'"
  LOCAL_ROWS_TOTAL=$((LOCAL_ROWS_TOTAL + 1))
  if [ -n "${LOCAL_BY_SUB[$sub]+set}" ]; then
    LOCAL_DUP["$sub"]=1
    return 0
  fi
  LOCAL_BY_SUB["$sub"]="$uid|$tenant|$org|$role|$status|$email"
}

read_tsv() { # file label loader
  local file="$1" label="$2" loader="$3" line
  [ -n "$file" ] || fail_with 2 "missing $label input"
  [ -f "$file" ] || fail_with 2 "$label input not found: $file"
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      ''|'#'*) continue ;;
    esac
    "$loader" "$line" "$label"
  done < "$file"
}

# ------------------------------------------------------------- source loading
# 两侧可独立取源：fixture（离线复跑）或 live（只读查询）。
# 混合模式（IAM fixture + 本地真库）用于一次性 PG 验证，与 #2623 R3 一致。
RECONCILE_SOURCE="live"
IAM_SOURCE="live"
LOCAL_SOURCE="live"
[ -n "$IAM_FIXTURE" ] && IAM_SOURCE="fixture"
[ -n "$LOCAL_FIXTURE" ] && LOCAL_SOURCE="fixture"
[ "$LOCAL_SOURCE" = "live" ] || RECONCILE_SOURCE="fixture"

need_env() { # value name
  [ -n "$1" ] || fail_with 2 "missing env $2"
}

if [ "$IAM_SOURCE" = "fixture" ]; then
  read_tsv "$IAM_FIXTURE" iam load_iam_line
else
  IAM_HOST="$(iam_env_value HOST)"; [ -n "$IAM_HOST" ] || IAM_HOST="127.0.0.1"
  IAM_PORT="$(iam_env_value PORT)"; [ -n "$IAM_PORT" ] || IAM_PORT="5432"
  IAM_USER="$(iam_env_value USER)"; [ -n "$IAM_USER" ] || IAM_USER="beaconiam_user"
  IAM_PASSWORD="$(iam_env_value PASSWORD)"
  IAM_DB="$(iam_env_value NAME)"
  need_env "$IAM_DB" "BEACONIAM_DB_NAME"

  IAM_REL_SQL="SELECT r.user_id::text, r.org_id::text, r.role,
       COALESCE((SELECT string_agg(v, ',' ORDER BY v)
                 FROM jsonb_array_elements_text(r.functional_roles) AS t(v)), ''),
       '1', COALESCE(o.parent_id::text, '')
  FROM user_org_relations r
  LEFT JOIN organizations o ON o.id = r.org_id
  WHERE r.is_active = true
  ORDER BY r.user_id, r.org_id"

  IAM_ORG_SQL="SELECT id::text, COALESCE(parent_id::text, '')
  FROM organizations
  ORDER BY id"

  IAM_USERS_SQL="SELECT id::text FROM users ORDER BY id"

  IAM_ROWS="$(psql_select "$IAM_HOST" "$IAM_PORT" "$IAM_USER" "$IAM_PASSWORD" "$IAM_DB" "$IAM_REL_SQL")" ||
    fail_with 2 "iam connection or query failed (BEACONIAM_DB_HOST=$IAM_HOST DB=$IAM_DB)"
  if printf '%s' "$IAM_ROWS" | grep -q '^ERROR:'; then
    fail_with 2 "iam query rejected: $(printf '%s' "$IAM_ROWS" | head -n 1)"
  fi
  IAM_ORG_ROWS="$(psql_select "$IAM_HOST" "$IAM_PORT" "$IAM_USER" "$IAM_PASSWORD" "$IAM_DB" "$IAM_ORG_SQL")" ||
    fail_with 2 "iam organization query failed"
  if printf '%s' "$IAM_ORG_ROWS" | grep -q '^ERROR:'; then
    fail_with 2 "iam organization query rejected: $(printf '%s' "$IAM_ORG_ROWS" | head -n 1)"
  fi
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in ''|'#'*) continue ;; esac
    load_org_parent "${line%%$'\t'*}" "${line#*$'\t'}"
  done <<< "$IAM_ORG_ROWS"

  IAM_USERS_KNOWN=1
  IAM_USER_ROWS="$(psql_select "$IAM_HOST" "$IAM_PORT" "$IAM_USER" "$IAM_PASSWORD" "$IAM_DB" "$IAM_USERS_SQL")" ||
    fail_with 2 "iam users query failed"
  while IFS= read -r line || [ -n "$line" ]; do
    [ -n "$line" ] || continue
    IAM_USER["$line"]=1
  done <<< "$IAM_USER_ROWS"

  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in ''|'#'*) continue ;; esac
    load_iam_line "$line" iam
  done <<< "$IAM_ROWS"
fi

if [ "$LOCAL_SOURCE" = "fixture" ]; then
  read_tsv "$LOCAL_FIXTURE" local load_local_line
else
  LOCAL_HOST="$(local_env_value HOST)"; [ -n "$LOCAL_HOST" ] || LOCAL_HOST="127.0.0.1"
  LOCAL_PORT="$(local_env_value PORT)"; [ -n "$LOCAL_PORT" ] || LOCAL_PORT="5432"
  LOCAL_USER="$(local_env_value USER)"; [ -n "$LOCAL_USER" ] || LOCAL_USER="tuneloop_user"
  LOCAL_PASSWORD="$(local_env_value PASSWORD)"
  LOCAL_DB="$(local_env_value NAME)"
  need_env "$LOCAL_DB" "TUNELOOP_DB_NAME"

  LOCAL_SQL="SELECT COALESCE(iam_sub, ''), id::text, tenant_id::text, org_id::text,
       COALESCE(role, ''), COALESCE(status, ''), COALESCE(email, '')
  FROM users
  WHERE deleted_at IS NULL
  ORDER BY COALESCE(iam_sub, ''), id"

  LOCAL_ROWS="$(psql_select "$LOCAL_HOST" "$LOCAL_PORT" "$LOCAL_USER" "$LOCAL_PASSWORD" "$LOCAL_DB" "$LOCAL_SQL")" ||
    fail_with 2 "local connection or query failed (TUNELOOP_DB_HOST=$LOCAL_HOST DB=$LOCAL_DB)"
  if printf '%s' "$LOCAL_ROWS" | grep -q '^ERROR:'; then
    fail_with 2 "local query rejected: $(printf '%s' "$LOCAL_ROWS" | head -n 1)"
  fi
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in ''|'#'*) continue ;; esac
    load_local_line "$line" local
  done <<< "$LOCAL_ROWS"
fi

# --------------------------------------------------------------- judgment core
# 与 beaconiam resolveUserOrg 对齐：role='member' 为零权限影子关系，不参与上下文推导。
resolve_root_org() { # org -> prints root org ("" when unresolved)
  local cur="$1" hops=0
  local -A seen=()
  while [ -n "$cur" ]; do
    if [ -n "${ORG_PARENT_CONFLICT[$cur]+set}" ]; then
      printf '%s' ""
      return 0
    fi
    if [ -n "${seen[$cur]+set}" ]; then
      printf '%s' ""
      return 0
    fi
    seen["$cur"]=1
    hops=$((hops + 1))
    if [ "$hops" -gt "$MAX_ORG_HOPS" ]; then
      printf '%s' ""
      return 0
    fi
    local parent="${ORG_PARENT[$cur]:-}"
    if [ -z "$parent" ]; then
      printf '%s' "$cur"
      return 0
    fi
    cur="$parent"
  done
  printf '%s' ""
}

# ToIAMRole 的正向原像（backend/services/role_templates.go:129-156，只读引用）：
#   merchant_admin→OWNER、site_admin→ADMIN、site_member→STAFF、
#   merchant_staff→STAFF、repair_technician→repair_technician
iam_role_of_template() { # template code -> IAM relation role ("" when not mapped)
  case "$1" in
    merchant_admin) printf 'OWNER' ;;
    site_admin) printf 'ADMIN' ;;
    site_member|merchant_staff) printf 'STAFF' ;;
    repair_technician) printf 'repair_technician' ;;
    *) printf '' ;;
  esac
}

allowed_local_roles() { # iam_role froles -> space separated allowlist
  local role="$1" froles="$2" allowed="" code item mapped
  case "$role" in
    OWNER) allowed="merchant_admin" ;;
    ADMIN) allowed="site_admin merchant_admin" ;;
    STAFF) allowed="site_member merchant_staff" ;;
    repair_technician) allowed="repair_technician" ;;
    *) allowed="" ;;
  esac
  [ -n "$allowed" ] || { printf '%s' ""; return 0; }
  local IFS=','
  for item in $froles; do
    code="$(printf '%s' "$item" | tr -d ' ')"
    [ -n "$code" ] || continue
    mapped="$(iam_role_of_template "$code")"
    [ "$mapped" = "$role" ] || continue
    case " $allowed " in
      *" $code "*) ;;
      *) allowed="$allowed $code" ;;
    esac
  done
  printf '%s' "$allowed"
}

declare -A COUNT=(
  [TENANT_MISMATCH]=0 [ORG_MISMATCH]=0 [NIL_UUID]=0 [MISSING_LOCAL]=0
  [ORPHAN_LOCAL]=0 [ROLE_MISMATCH]=0 [ROLE_UNVERIFIED]=0
  [CONTEXT_AMBIGUOUS]=0 [MEMBER_ONLY]=0 [IAM_MISSING]=0 [ORG_UNRESOLVED]=0
)
DETAILS=()

add_detail() { # category sub local_desc iam_desc
  DETAILS+=("$2"$'\t'"$1"$'\t'"$3"$'\t'"$4")
}

classify_subject() { # sub
  local sub="$1" rels org role froles loginable=0 rel_org="" rel_role="" rel_froles=""
  local local_row="" local_uid="" local_tenant="" local_org="" local_role="" local_status="" local_email=""
  local expect_org="" expect_tenant="" allowed=""
  rels="${REL_BY_USER[$sub]:-}"
  while IFS='|' read -r org role froles; do
    [ -n "$org" ] || continue
    if [ "$role" != "member" ]; then
      loginable=$((loginable + 1))
      rel_org="$org"; rel_role="$role"; rel_froles="$froles"
    fi
  done <<< "$rels"

  if [ -n "${LOCAL_BY_SUB[$sub]+set}" ]; then
    local_row="${LOCAL_BY_SUB[$sub]}"
    IFS='|' read -r local_uid local_tenant local_org local_role local_status local_email <<< "$local_row"
  fi

  if [ "$loginable" -ge 2 ]; then
    COUNT[CONTEXT_AMBIGUOUS]=$((COUNT[CONTEXT_AMBIGUOUS] + 1))
    add_detail CONTEXT_AMBIGUOUS "$sub" \
      "tenant=${local_tenant:-<none>} org=${local_org:-<none>} role=${local_role:-<none>}" \
      "loginable=${loginable}（多上下文，登录时由用户选择，本地只能存一个）"
    return 0
  fi

  if [ "$loginable" -eq 1 ]; then
    expect_org="$rel_org"
    expect_tenant="$(resolve_root_org "$expect_org")"
    if [ -z "$expect_tenant" ]; then
      COUNT[ORG_UNRESOLVED]=$((COUNT[ORG_UNRESOLVED] + 1))
      add_detail ORG_UNRESOLVED "$sub" "org=${local_org:-<none>}" "org=${expect_org}（父链成环/冲突，无法求根）"
      return 0
    fi
    if [ -z "$local_row" ]; then
      COUNT[MISSING_LOCAL]=$((COUNT[MISSING_LOCAL] + 1))
      add_detail MISSING_LOCAL "$sub" "<absent>" \
        "tenant=$expect_tenant org=$expect_org role=$rel_role"
      return 0
    fi
    if [ "$local_tenant" = "$ZERO_UUID" ] && [ "$local_org" = "$ZERO_UUID" ]; then
      COUNT[NIL_UUID]=$((COUNT[NIL_UUID] + 1))
      add_detail NIL_UUID "$sub" "tenant=$local_tenant org=$local_org role=$local_role" \
        "tenant=$expect_tenant org=$expect_org role=$rel_role"
      return 0
    fi
    if [ "$local_tenant" != "$expect_tenant" ]; then
      COUNT[TENANT_MISMATCH]=$((COUNT[TENANT_MISMATCH] + 1))
      add_detail TENANT_MISMATCH "$sub" "tenant=$local_tenant org=$local_org role=$local_role" \
        "tenant=$expect_tenant org=$expect_org role=$rel_role"
      return 0
    fi
    if [ "$local_org" != "$expect_org" ]; then
      COUNT[ORG_MISMATCH]=$((COUNT[ORG_MISMATCH] + 1))
      add_detail ORG_MISMATCH "$sub" "tenant=$local_tenant org=$local_org role=$local_role" \
        "tenant=$expect_tenant org=$expect_org role=$rel_role"
      return 0
    fi
    allowed="$(allowed_local_roles "$rel_role" "$rel_froles")"
    if [ -z "$allowed" ] || [ -z "$local_role" ]; then
      COUNT[ROLE_UNVERIFIED]=$((COUNT[ROLE_UNVERIFIED] + 1))
      add_detail ROLE_UNVERIFIED "$sub" "role=${local_role:-<empty>}" \
        "iam_role=$rel_role（白名单外/无法唯一确定）"
      return 0
    fi
    case " $allowed " in
      *" $local_role "*) return 0 ;;
      *)
        COUNT[ROLE_MISMATCH]=$((COUNT[ROLE_MISMATCH] + 1))
        add_detail ROLE_MISMATCH "$sub" "role=$local_role" \
          "iam_role=$rel_role allow={${allowed# }}"
        return 0
        ;;
    esac
  fi

  # loginable == 0
  if [ -n "$rels" ]; then
    COUNT[MEMBER_ONLY]=$((COUNT[MEMBER_ONLY] + 1))
    add_detail MEMBER_ONLY "$sub" "tenant=${local_tenant:-<none>} org=${local_org:-<none>}" \
      "仅 member 影子关系（与 resolveUserOrg 回退语义一致，不判差异）"
    return 0
  fi

  if [ "$IAM_USERS_KNOWN" = "1" ] && [ -z "${IAM_USER[$sub]+set}" ]; then
    COUNT[IAM_MISSING]=$((COUNT[IAM_MISSING] + 1))
    add_detail IAM_MISSING "$sub" "tenant=$local_tenant org=$local_org role=$local_role" \
      "IAM 无该 user_id"
    return 0
  fi

  if [ -z "$local_row" ]; then
    return 0
  fi
  if [ "$local_tenant" = "$ZERO_UUID" ] && [ "$local_org" = "$ZERO_UUID" ]; then
    return 0
  fi
  COUNT[ORPHAN_LOCAL]=$((COUNT[ORPHAN_LOCAL] + 1))
  add_detail ORPHAN_LOCAL "$sub" "tenant=$local_tenant org=$local_org role=$local_role" \
    "<no active relation>"
  return 0
}

scheduled_keys="$( { printf '%s\n' "${!REL_BY_USER[@]}"; printf '%s\n' "${!LOCAL_BY_SUB[@]}"; } | sed '/^$/d' | LC_ALL=C sort -u)"
while IFS= read -r subject; do
  [ -n "$subject" ] || continue
  classify_subject "$subject"
done <<< "$scheduled_keys"

# ------------------------------------------------------------------- reporting
USERS_LOCAL="$LOCAL_ROWS_TOTAL"
USERS_IAM_ACTIVE=0
for subject in "${!REL_BY_USER[@]}"; do
  [ -n "$subject" ] && USERS_IAM_ACTIVE=$((USERS_IAM_ACTIVE + 1))
done

sort_details() {
  if [ "${#DETAILS[@]}" -eq 0 ]; then
    return 0
  fi
  printf '%s\n' "${DETAILS[@]}" | LC_ALL=C sort -t$'\t' -k1,1
}

TOKEN_ORDER=(
  RECONCILE_DATE RECONCILE_SOURCE USERS_LOCAL USERS_IAM_ACTIVE
  TENANT_MISMATCH ORG_MISMATCH NIL_UUID MISSING_LOCAL ORPHAN_LOCAL
  ROLE_MISMATCH ROLE_UNVERIFIED CONTEXT_AMBIGUOUS MEMBER_ONLY
  IAM_MISSING ORG_UNRESOLVED ROLE_REPAIR_SUPPORTED DB_WRITES
)

token_value() { # name
  case "$1" in
    RECONCILE_DATE) printf '%s' "$RECONCILE_DATE" ;;
    RECONCILE_SOURCE) printf '%s' "$RECONCILE_SOURCE" ;;
    USERS_LOCAL) printf '%s' "$USERS_LOCAL" ;;
    USERS_IAM_ACTIVE) printf '%s' "$USERS_IAM_ACTIVE" ;;
    ROLE_REPAIR_SUPPORTED) printf '0' ;;
    DB_WRITES) printf '%s' "$DB_WRITES" ;;
    *) printf '%s' "${COUNT[$1]:-0}" ;;
  esac
}

if [ "$MODE_JSON" = "1" ]; then
  rows_json="[]"
  if [ "${#DETAILS[@]}" -gt 0 ]; then
    rows_json="$(sort_details | awk -F'\t' -v limit="$LIMIT" '
      BEGIN { n = 0 }
      {
        if (limit > 0 && n >= limit) exit
        n++
        gsub(/\\/, "\\\\", $0)
        printf "%s\t%s\t%s\t%s\n", $1, $2, $3, $4
      }' | jq -R -s 'split("\n") | map(select(length > 0)) | map(split("\t") | {
          iam_sub: .[0], category: .[1], local: .[2], iam: .[3],
          email: ((.[4] // "") | if . == "" then "" else . end)
        })')"
  fi
  tokens_json="$(for name in "${TOKEN_ORDER[@]}"; do
      jq -n --arg k "$name" --arg v "$(token_value "$name")" '{($k): $v}'
    done | jq -s 'add')"
  email_map="$(for sub in "${!LOCAL_BY_SUB[@]}"; do
      row="${LOCAL_BY_SUB[$sub]}"
      printf '%s\t%s\n' "$sub" "$(printf '%s' "$row" | awk -F'|' '{print $6}')"
    done | LC_ALL=C sort | jq -R -s 'split("\n") | map(select(length > 0)) | map(split("\t") | {(.[0]): (.[1] // "")}) | add // {}')"
  jq -n --argjson tokens "$tokens_json" --argjson rows "$rows_json" --argjson emails "$email_map" \
    '{tokens: $tokens, rows: ($rows | map(. + {email: ($emails[.iam_sub] // .email)}))}'
  exit "$EXIT_CODE"
fi

for name in "${TOKEN_ORDER[@]}"; do
  printf '%s=%s\n' "$name" "$(token_value "$name")"
done
printf 'DETAIL_COUNT=%s\n' "${#DETAILS[@]}"
if [ "${#DETAILS[@]}" -gt 0 ]; then
  sort_details | while IFS=$'\t' read -r sub category local_desc iam_desc; do
    printf 'DETAIL iam_sub=%s category=%s local[%s] iam[%s]\n' "$sub" "$category" "$local_desc" "$iam_desc"
  done
fi
echo "RESULT: PASS"
exit "$EXIT_CODE"
