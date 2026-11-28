#!/usr/bin/env bash
# verify-2647.sh — Evidence Contract 执行器（#2647）。
#
# 用法：bash scripts/verify-2647.sh <r2|r7|r8|r9|all>
#
# 每行产出 docs/evidence/2647/<artifact>，首行为 provenance：
#   # build=1.0.<short> # commit=<full> # dirty=false
# 并在 artifact 内追加可 grep 的 TOKEN=VALUE 断言行。
#
# 环境（可用环境变量覆盖）：POSTGRES_HOST=127.0.0.1 POSTGRES_PORT=15433
# POSTGRES_USER=tuneloop_user POSTGRES_PASSWORD=trust
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT" || exit 2

ISSUE=2647
EDIR="docs/evidence/$ISSUE"
COMMIT="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
[ -n "$COMMIT" ] || COMMIT="$(git rev-parse HEAD)"
SHORT="$(git rev-parse --short=7 "$COMMIT")"
BUILD="1.0.$SHORT"

export PATH="/usr/local/go/bin:$PATH"
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TMPDIR="${TMPDIR:-$ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$ROOT/tmp/gocache}"
mkdir -p "$EDIR" "$TMPDIR" "$GOCACHE" "$ROOT/tmp/evidence-$ISSUE"

HEAVY_RUN="$HOME/.config/opencode/scripts/heavy-run.sh"

run_heavy() {
  if [ -x "$HEAVY_RUN" ]; then
    bash "$HEAVY_RUN" --scope host --timeout "${HEAVY_LOCK_TIMEOUT:-1800}" -- "$@"
  else
    "$@"
  fi
}

dirty_flag() {
  local d
  d="$(git status --porcelain -- ':!docs/evidence' ':!tmp' 2>/dev/null)"
  if [ -n "$d" ]; then echo "true"; else echo "false"; fi
}

provenance() {
  printf '# build=%s # commit=%s # dirty=%s' "$BUILD" "$COMMIT" "$(dirty_flag)"
}

header() {
  local artifact="$1"
  mkdir -p "$(dirname "$artifact")"
  provenance > "$artifact"
  printf '\n' >> "$artifact"
}

# extract_tokens <log>
extract_tokens() {
  local log="$1"
  grep -E '[A-Z][A-Z0-9_]+=' "$log" \
    | sed -E 's/^.*\.go:[0-9]+: //' \
    | grep -E '^[A-Z][A-Z0-9_]+=' \
    | awk '!seen[$0]++'
}

# ---------------------------------------------------------------- R2 (static)
# 写点口径（D1）：6 个在册写点 = iam_callback(1) + merchant_member(1) + user_staff
# CreateUser(1) + user_staff UpdateUser(1) + merchant(1) + iam_proxy SyncUsers(1)。
# allowlist（D1）= site.go ×2 + site_member_common.go ×1 建号初始化写点。
row_r2() {
  local artifact="$EDIR/R2-write-sites.txt"
  header "$artifact"
  local rc=0 sites=0 inline=0 assign_without_mirror=0 allowlist_entries=0 allowlist_unlinked=0
  local f

  for f in backend/handlers/iam_callback.go backend/handlers/merchant_member.go \
           backend/handlers/user_staff.go backend/handlers/merchant.go backend/handlers/iam_proxy.go; do
    local derived struct_writes map_writes
    derived="$(grep -c 'DeriveRoles(' "$f" || true)"
    # 结构体字面量写（Create 走 serializer）：Roles: <derived expr>
    struct_writes="$(grep -cE '^\s*Roles:\s+' "$f" || true)"
    # map 写：必须经 setUserRolesMirror（唯一 jsonb 出口，R7）
    map_writes="$(grep -cE 'setUserRolesMirror\(' "$f" || true)"
    echo "FILE=$f derive_calls=$derived struct_roles=$struct_writes mirror_expr_calls=$map_writes" >> "$artifact"
    # D1 写点口径 = 文件级：该文件至少有一个「经派生的镜像写」（map 形态或结构体派生值）
    if grep -qE 'setUserRolesMirror\(|Roles:\s+(mirror|derived|info)\.(All|all)' "$f"; then
      sites=$((sites + 1))
    fi
  done
  # user_staff.go 有两个在册写点（CreateUser 结构体 + UpdateUser map）→ 补 1 点
  if grep -q 'updates\["role"\] =' backend/handlers/user_staff.go; then
    sites=$((sites + 1))
  fi
  [ "$sites" -ge 6 ] || { echo "ASSERT_FAIL: WRITE_SITES=$sites (<6)" >> "$artifact"; rc=1; }

  # 内联角色字面量：5 个在册文件里 **users 镜像写**不得出现硬编码角色字面量
  inline="$(grep -nE 'Roles:\s*\[\]string\{?"(site_member|site_admin|merchant_staff|repair_technician|merchant_admin)"' \
      backend/handlers/iam_callback.go backend/handlers/merchant_member.go \
      backend/handlers/user_staff.go backend/handlers/merchant.go backend/handlers/iam_proxy.go \
      2>/dev/null | wc -l | tr -d ' ')"
  [ "$inline" = "0" ] || { echo "ASSERT_FAIL: WRITE_INLINE_LITERAL=$inline" >> "$artifact"; rc=1; }

  # 改角色点：每个 updates["role"] = 必须有伴随镜像写（R8 的结构性保证）
  local in_assign_block
  in_assign_block="$(grep -c 'updates\["role"\] =' backend/handlers/user_staff.go || true)"
  local mirrored
  mirrored="$(grep -c 'setUserRolesMirror(updates' backend/handlers/user_staff.go || true)"
  if [ "$in_assign_block" -gt 0 ] && [ "$mirrored" -eq 0 ]; then
    assign_without_mirror=1
    echo "ASSERT_FAIL: ROLE_ASSIGN_WITHOUT_MIRROR=1（updates[\"role\"] 无伴随镜像写）" >> "$artifact"
    rc=1
  fi
  echo "ROLE_ASSIGN_SITES=$in_assign_block" >> "$artifact"
  echo "MIRROR_WRITES_IN_USER_STAFF=$mirrored" >> "$artifact"

  # allowlist：3 个建号初始化点（D1），必须真实存在且未被当成在册写点
  for f in backend/handlers/site.go backend/handlers/site_member_common.go; do
    local hits
    hits="$(grep -cE 'Role:\s+"(site_admin|site_member)"' "$f" || true)"
    if [ "$hits" -gt 0 ]; then
      allowlist_entries=$((allowlist_entries + hits))
      echo "ALLOWLIST=$f hits=$hits reason=建号初始化（D1）ref=#2626 D1/D7" >> "$artifact"
    else
      allowlist_unlinked=$((allowlist_unlinked + 1))
      echo "ASSERT_FAIL: ALLOWLIST_UNLINKED=$f" >> "$artifact"
      rc=1
    fi
  done
  [ "$allowlist_entries" = "3" ] || { echo "ASSERT_FAIL: ALLOWLIST_ENTRIES=$allowlist_entries" >> "$artifact"; rc=1; }

  # 负例自检：变量形态（updates["role"] = roleVar，无镜像）必须被判定为「缺镜像」
  local negfile="$ROOT/tmp/evidence-$ISSUE/negative-var-2647.go"
  cat > "$negfile" <<'GO'
package handlers
func synthetic2647(updates map[string]interface{}, roleVar string) {
	updates["role"] = roleVar
}
GO
  local neg_assign neg_mirror neg_control=0
  neg_assign="$(grep -c 'updates\["role"\] =' "$negfile" || true)"
  neg_mirror="$(grep -c 'setUserRolesMirror(updates' "$negfile" || true)"
  if [ "$neg_assign" -gt 0 ] && [ "$neg_mirror" -eq 0 ]; then neg_control=1; fi
  [ "$neg_control" = "1" ] || { echo "ASSERT_FAIL: R2_NEGATIVE_CONTROL_VAR=0" >> "$artifact"; rc=1; }

  {
    echo "WRITE_SITES=$sites"
    echo "WRITE_INLINE_LITERAL=$inline"
    echo "ROLE_ASSIGN_WITHOUT_MIRROR=$assign_without_mirror"
    echo "R2_NEGATIVE_CONTROL_VAR=$neg_control"
    echo "ALLOWLIST_ENTRIES=$allowlist_entries"
    echo "ALLOWLIST_UNLINKED=$allowlist_unlinked"
  } >> "$artifact"
  return "$rc"
}

# ---------------------------------------------------------------- R7 (jsonb)
row_r7() {
  local artifact="$EDIR/R7-users-roles-jsonb.txt" log="$ROOT/tmp/evidence-$ISSUE/r7.log"
  header "$artifact"
  : > "$log"
  ( cd backend && run_heavy go test ./handlers -run 'TestUserRolesMirrorJSONB_2647' -count=1 -v ) > "$log" 2>&1
  local rc=$?
  cat "$log" >> "$artifact"
  extract_tokens "$log" >> "$artifact"

  # 静态：4 个 map 写点都必须经 userRolesExpr（唯一 jsonb 出口），不得残留裸切片
  local jsonb_sites raw_sites
  jsonb_sites="$(grep -cE 'setUserRolesMirror\(' \
      backend/handlers/iam_callback.go backend/handlers/merchant.go \
      backend/handlers/user_staff.go backend/handlers/iam_proxy.go | awk -F: '{s+=$2} END {print s+0}')"
  raw_sites="$(grep -nE 'updates\["roles"\]\s*=\s*[A-Za-z_.]+\.All' \
      backend/handlers/iam_callback.go backend/handlers/merchant.go \
      backend/handlers/user_staff.go backend/handlers/iam_proxy.go 2>/dev/null | wc -l | tr -d ' ')"
  local expr_ok=0 raw_ok=1
  [ "$jsonb_sites" -ge 4 ] && expr_ok=1
  [ "$raw_sites" = "0" ] || raw_ok=0
  [ "$expr_ok" = "1" ] || { echo "ASSERT_FAIL: USERS_MIRROR_JSONB_EXPR=0 (sites=$jsonb_sites)" >> "$artifact"; rc=1; }
  [ "$raw_ok" = "1" ] || { echo "ASSERT_FAIL: USERS_MIRROR_RAW_SLICE=$raw_sites" >> "$artifact"; rc=1; }
  {
    echo "USERS_MIRROR_JSONB_EXPR=$expr_ok"
    echo "USERS_MIRROR_RAW_SLICE=$raw_sites"
  } >> "$artifact"

  grep -Fq 'JSONB_TYPE_IS_ARRAY=1' "$artifact" || return 1
  grep -Fq 'JSONB_IS_NULL=0' "$artifact" || return 1
  grep -Fq 'RAW_SLICE_WRITE_ERROR=1' "$artifact" || return 1
  return "$rc"
}

# ---------------------------------------------------------------- R8 / R9
row_r8() {
  local artifact="$EDIR/R8-role-change-mirror.txt" log="$ROOT/tmp/evidence-$ISSUE/r8.log"
  header "$artifact"
  ( cd backend && run_heavy go test ./handlers -run 'TestUpdateUserRoleMirror_2647' -count=1 -v ) > "$log" 2>&1
  local rc=$?
  cat "$log" >> "$artifact"
  extract_tokens "$log" >> "$artifact"
  grep -Fq 'HTTP=200' "$artifact" || return 1
  grep -Fq 'ROLE_ASSIGN_WITHOUT_MIRROR=0' "$artifact" || return 1
  grep -Fq 'MIRROR_PRIMARY=site_admin' "$artifact" || return 1
  grep -Fq 'MIRROR_ALL_CONTAINS_SITE_ADMIN=1' "$artifact" || return 1
  grep -Fq 'MIRROR_ALL_CONTAINS_OLD=1' "$artifact" || return 1
  grep -Fq 'READBACK_PREFERS_MIRROR=1' "$artifact" || return 1
  return "$rc"
}

row_r9() {
  local artifact="$EDIR/R9-mirror-source.txt" log="$ROOT/tmp/evidence-$ISSUE/r9.log"
  header "$artifact"
  ( cd backend && run_heavy go test ./handlers -run 'TestSyncUsersMirrorNonDegrade_2647' -count=1 -v ) > "$log" 2>&1
  local rc=$?
  cat "$log" >> "$artifact"
  extract_tokens "$log" >> "$artifact"

  # 静态：镜像只能由 DeriveRoles 产出，且 handler 目录无第二处 users.roles 赋值来源
  local second=0 derive_calls
  derive_calls="$(grep -rc 'services.DeriveRoles(' backend/handlers/*.go | awk -F: '{s+=$2} END {print s+0}')"
  # 除 userRolesExpr/setUserRolesMirror 与结构体字段外，不得再有 `Updates(...角色切片...)` 直写
  second="$(grep -rnE '"(roles)"\s*:\s*[A-Za-z_.]+\.All' backend/handlers/*.go | grep -v '_test.go' | wc -l | tr -d ' ')"
  [ "$second" = "0" ] || { echo "ASSERT_FAIL: WRITE_SIDE_SECOND_ASSIGNMENT=$second" >> "$artifact"; rc=1; }
  {
    echo "DERIVE_CALLS_TOTAL=$derive_calls"
    echo "WRITE_SIDE_SECOND_ASSIGNMENT=$second"
  } >> "$artifact"

  grep -Fq 'MIRROR_SOURCE_DERIVE_ONLY=1' "$artifact" || return 1
  grep -Fq 'SYNC_SKIPS_NONEMPTY_MIRROR_OVERWRITE=1' "$artifact" || return 1
  grep -Fq 'SYNC_DEGRADES_NONEMPTY_MIRROR=0' "$artifact" || return 1
  return "$rc"
}

ROW="${1:-}"
case "$ROW" in
  r2) row_r2 ;;
  r7) row_r7 ;;
  r8) row_r8 ;;
  r9) row_r9 ;;
  all)
    FAIL=0
    for r in r2 r7 r8 r9; do
      if "row_$r"; then echo "$r: PASS"; else echo "$r: FAIL"; FAIL=1; fi
    done
    exit "$FAIL"
    ;;
  *)
    echo "usage: $0 <r2|r7|r8|r9|all>" >&2
    exit 2 ;;
esac

rc=$?
if [ "$rc" = "0" ]; then echo "$ROW: PASS"; else echo "$ROW: FAIL"; fi
exit "$rc"
