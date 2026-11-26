#!/usr/bin/env bash
# #2547 verification harness — one subcommand per Evidence Contract row.
#
#   bash scripts/verify-2547.sh r1   # 建站创建 IAM branch 子组织 + 失败不留站点
#   bash scripts/verify-2547.sh r2   # 成员 bind/role/unbind 的 org 段 = sites.org_id
#   bash scripts/verify-2547.sh r3   # 中转成员识别改为站点维度（列表 + by-id）
#   bash scripts/verify-2547.sh r4   # 存量回填 dry-run 0 写入 + apply 幂等
#   bash scripts/verify-2547.sh r5   # 四份文档口径同步（静态）
#   bash scripts/verify-2547.sh r6   # 构建 + gofmt + 定向回归
#
# Capture a row into its artifact:
#   bash scripts/verify-2547.sh r1 > docs/evidence/2547/R1-create-org.txt
#
# Provenance header (consumed by scripts/evidence-artifacts.sh P14):
#   # build=1.0.<short-sha> # commit=<full-sha> # dirty=<true|false>
#   GENERATED_AT_HEAD=<full-sha>
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r2|r3|r4|r5|r6) ;;
  *) echo "usage: $0 r1|r2|r3|r4|r5|r6" >&2; exit 2 ;;
esac

ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not a git repository" >&2; exit 2; }
cd "$ROOT" || exit 2

if ! command -v go >/dev/null 2>&1 && [ -x /usr/local/go/bin/go ]; then
  export PATH="/usr/local/go/bin:$PATH"
fi

# Backend tests need the Postgres test cluster.
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_DB_SIMPLE_PROTOCOL="${TUNELOOP_DB_SIMPLE_PROTOCOL:-1}"
export TMPDIR="${TMPDIR:-$ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"

FIX_SHA="${FIX_COMMIT:-$(git log --format=%H --grep='^fix(issue-2547)' -1 2>/dev/null)}"
ANCHOR_SHA="${FIX_SHA:-$(git rev-parse HEAD)}"
SHORT_SHA="$(git rev-parse --short "$ANCHOR_SHA")"
BUILD="1.0.${SHORT_SHA}"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence' ':!tmp')" ]; then
  DIRTY=true
fi
echo "# build=${BUILD} # commit=${ANCHOR_SHA} # dirty=${DIRTY}"
echo "GENERATED_AT_HEAD=$(git rev-parse HEAD)"

fail() {
  echo "FAIL(${ROW}): $*" >&2
  exit 1
}

HEAVY="$HOME/.config/opencode/scripts/heavy-run.sh"
heavy() { bash "$HEAVY" -- "$@"; }

# run_row <pkg> <test-regex> <expected-token>...
run_row() {
  local pkg="$1" name="$2"; shift 2
  local out; out="$(mktemp)"
  ( cd backend && heavy go test "$pkg" -run "${name}" -count=1 -v ) > "$out" 2>&1
  local rc=$?
  grep -E '^(MEMBER_|ADMIN_|LEGACY_|CUSTOMER_|UNKNOWN_|ILLEGAL_|PERM_|CUS_|BIND_|BUSINESS_|CREATE_|NO_SITE|DUPLICATE_|SYSTEM_|STAFF_|DRYRUN_|APPLY_|SECOND_|ROLE_|UNBIND_|MEMBER_|ROLE_TEMPLATE|NO_ORG)' "$out" || true
  grep -E '^--- (PASS|FAIL)' "$out" || true
  if [ "$rc" -ne 0 ]; then
    echo "--- go test output (tail) ---"
    tail -40 "$out"
    rm -f "$out"
    fail "go test ${pkg} ${name} exit=${rc}"
  fi
  local tok
  for tok in "$@"; do
    if ! grep -qx -- "$tok" "$out"; then
      echo "--- go test output (tail) ---"
      tail -20 "$out"
      rm -f "$out"
      fail "missing expected token '$tok'"
    fi
  done
  rm -f "$out"
}

case "$ROW" in
  r1)
    echo "== R1 建站：IAM branch 子组织 + 失败不留站点 =="
    run_row ./handlers 'TestCreateAdminTransitSiteCreatesBranchOrg2547|TestCreateAdminTransitSiteFailuresLeaveNoSite2547' \
      "CREATE_SITE_ORG=1" \
      "CREATE_PARENT_IS_NAMESPACE=1" \
      "CREATE_TYPE_BRANCH=1" \
      "CREATE_ADMIN_INFO_ABSENT=1" \
      "NO_SITE_ON_IAM_FAILURE=1" \
      "NO_SITE_ON_NO_PRIMARY=1" \
      "NO_SITE_ON_EMPTY_TENANT=1" \
      "DUPLICATE_NAME_409=1"
    echo "R1_OK"
    ;;

  r2)
    echo "== R2 成员：IAM org 段 = sites.org_id =="
    run_row ./handlers 'TestTransitSiteMemberIAMOrgIsSiteOrg2547' \
      "BIND_ORG_IS_SITE_ORG=1" \
      "BIND_ORG_NOT_TENANT=1" \
      "ROLE_ORG_IS_SITE_ORG=1" \
      "UNBIND_ORG_IS_SITE_ORG=1" \
      "MEMBER_TENANT_IS_LOCAL_TENANT=1" \
      "ROLE_TEMPLATE_NS_SCOPED=1" \
      "NO_ORG_TEMPLATE_CREATE=1"
    echo "R2_OK"
    ;;

  r3)
    echo "== R3 中转成员识别：站点维度（列表 + by-id） =="
    run_row ./handlers 'TestPersonnelSystemViewTransitBySiteOrg2547' \
      "SYSTEM_TRANSIT_VISIBLE=1" \
      "SYSTEM_NON_TRANSIT_PLATFORM_SITE_HIDDEN=1" \
      "SYSTEM_FOREIGN_TENANT_TRANSIT_HIDDEN=1" \
      "STAFF_DETAIL_TRANSIT_VISIBLE=1"
    echo "R3_OK"
    ;;

  r4)
    echo "== R4 存量回填：dry-run 0 写入 + apply 幂等 =="
    run_row ./services 'TestTransitSiteOrgBackfill2547' \
      "DRYRUN_ROWS=2" \
      "DRYRUN_WRITES=0" \
      "DRYRUN_IAM_CREATES=0" \
      "APPLY_UPDATED=2" \
      "APPLY_SKIPPED_MODERN=1" \
      "APPLY_IAM_CREATES=2" \
      "SECOND_RUN_UPDATED=0"
    echo "R4_OK"
    ;;

  r5)
    echo "== R5 文档口径同步 =="
    API_DOC="docs/spec/api/api.md"
    CASES_DOC="docs/cases/transit.md"
    AUDIT_DOC="docs/ops/tenant-id-audit.md"
    DB_DOC="docs/spec/database/database.md"
    for f in "$API_DOC" "$CASES_DOC" "$AUDIT_DOC" "$DB_DOC"; do
      [ -f "$f" ] || fail "missing $f"
    done

    # api.md：中转判据 = 站点维度（type + tenant_id）+ branch 子组织说明
    API_TRANSIT_RULE=$(grep -c "sites.type='transit' AND s.tenant_id" "$API_DOC" || true)
    # cases/transit.md：归属改为命名空间组织下的直营分支子组织
    CASES_TRANSIT_ORG=$(grep -c "直营分支子组织" "$CASES_DOC" || true)
    # 审计表：tenant=平台租户；org=中转子组织
    AUDIT_DOC_UPDATED=$(grep -c "org=中转子组织（#2547" "$AUDIT_DOC" || true)
    # database.md：sites.org_id 补中转同规则
    DB_DOC_TRANSIT=$(grep -c "中转网点同规则" "$DB_DOC" || true)
    # 旧判据残留：`s.tenant_id = ? OR s.org_id = ?` / 「平台顶层组织（租户=平台根）」
    OLD_ORG_EQUALS_TENANT_RULE=$((
      $(grep -c "s.tenant_id = ? OR s.org_id = ?" "$API_DOC" || true) +
      $(grep -c "平台顶层组织（租户=平台根）" "$CASES_DOC" || true) +
      $(grep -c "直辖平台顶层组织，tenant=org=平台租户" "$AUDIT_DOC" || true)
    ))

    echo "API_TRANSIT_RULE=${API_TRANSIT_RULE}"
    echo "CASES_TRANSIT_ORG=${CASES_TRANSIT_ORG}"
    echo "AUDIT_DOC_UPDATED=${AUDIT_DOC_UPDATED}"
    echo "DB_DOC_TRANSIT=${DB_DOC_TRANSIT}"
    echo "OLD_ORG_EQUALS_TENANT_RULE=${OLD_ORG_EQUALS_TENANT_RULE}"

    [ "${API_TRANSIT_RULE:-0}" -ge 1 ] || fail "api.md 未写站点维度中转判据"
    [ "${CASES_TRANSIT_ORG:-0}" -ge 1 ] || fail "cases/transit.md 未写直营分支子组织归属"
    [ "${AUDIT_DOC_UPDATED:-0}" -ge 1 ] || fail "审计表未更新中转子组织口径"
    [ "${DB_DOC_TRANSIT:-0}" -ge 1 ] || fail "database.md 未补中转同规则"
    [ "${OLD_ORG_EQUALS_TENANT_RULE:-0}" = "0" ] || fail "旧判据仍有残留"
    echo "R5_OK"
    ;;

  r6)
    echo "== R6 构建 + gofmt + 定向回归 =="
    BUILD_OUT="$(mktemp)"
    ( cd backend && heavy go build ./... ) > "$BUILD_OUT" 2>&1
    BUILD_RC=$?
    tail -5 "$BUILD_OUT"
    rm -f "$BUILD_OUT"

    GOFMT_DIFF=$(
      ( cd backend && gofmt -l \
          handlers/transit_site_admin.go \
          handlers/personnel.go \
          services/iam_client.go \
          services/transit_site_org_backfill.go \
          tools/backfill_transit_site_orgs/main.go ) | wc -l | tr -d ' '
    )

    TRANSIT_OUT="$(mktemp)"
    ( cd backend && heavy go test ./handlers -run \
        'TestCreateAdminTransitSite|TestTransitSiteMember|TestPersonnelSystemViewTransitBySiteOrg2547' \
        -count=1 ) > "$TRANSIT_OUT" 2>&1
    TRANSIT_RC=$?
    tail -6 "$TRANSIT_OUT"
    rm -f "$TRANSIT_OUT"

    PERSONNEL_OUT="$(mktemp)"
    ( cd backend && heavy go test ./handlers -run 'TestPersonnel2065|TestPersonnel2526|TestPersonnel2427' -count=1 ) > "$PERSONNEL_OUT" 2>&1
    PERSONNEL_RC=$?
    tail -6 "$PERSONNEL_OUT"
    rm -f "$PERSONNEL_OUT"

    SERVICES_OUT="$(mktemp)"
    ( cd backend && heavy go test ./services -run 'TestTransitSiteOrgBackfill2547|TestRoleTemplate' -count=1 ) > "$SERVICES_OUT" 2>&1
    SERVICES_RC=$?
    tail -6 "$SERVICES_OUT"
    rm -f "$SERVICES_OUT"

    echo "BUILD_RC=${BUILD_RC}"
    echo "GOFMT_DIFF=${GOFMT_DIFF}"
    echo "TRANSIT_SUITE_OK=$([ "$TRANSIT_RC" = "0" ] && echo 1 || echo 0)"
    echo "PERSONNEL_SUITE_OK=$([ "$PERSONNEL_RC" = "0" ] && echo 1 || echo 0)"
    echo "SERVICES_SUITE_OK=$([ "$SERVICES_RC" = "0" ] && echo 1 || echo 0)"

    [ "$BUILD_RC" = "0" ] || fail "go build ./... 失败"
    [ "$GOFMT_DIFF" = "0" ] || fail "gofmt 未通过（$GOFMT_DIFF 个文件）"
    [ "$TRANSIT_RC" = "0" ] || fail "中转套件失败"
    [ "$PERSONNEL_RC" = "0" ] || fail "人员套件失败"
    [ "$SERVICES_RC" = "0" ] || fail "服务套件失败"
    echo "R6_OK"
    ;;
esac
