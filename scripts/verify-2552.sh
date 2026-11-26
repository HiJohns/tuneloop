#!/usr/bin/env bash
# #2552 verification harness — one subcommand per Evidence Contract row.
#
#   bash scripts/verify-2552.sh r1   # D2：成员 tenant 列 = 本地租户；IAM org = 站点 org（bind/role/unbind）；
#                                    #     重复添加不增行（Rev.2：R1b 已并入 R1）
#   bash scripts/verify-2552.sh r1b  # 别名：仅跑 role/unbind 两路径（Rev.2 起 R1 已包含）
#   bash scripts/verify-2552.sh r2   # 中转成员识别改为站点维度（列表 + by-id + 负例）
#   bash scripts/verify-2552.sh r3   # 建站：type=branch + 父组织=命名空间主组织 + 错误映射 + 补偿
#   bash scripts/verify-2552.sh r4   # 回填工具：dry-run 0 写入 + apply 幂等 + members 阶段
#   bash scripts/verify-2552.sh r5   # 四份文档口径同步（结构断言）
#   bash scripts/verify-2552.sh r6   # 构建 + gofmt + 定向回归
#   bash scripts/verify-2552.sh r7   # plan 四源 FRESH + plan.md 被 git 跟踪
#
# Capture a row into its artifact:
#   bash scripts/verify-2552.sh r1 > docs/evidence/2552/R1-member-tenant.txt
#
# Every artifact starts with the provenance header consumed by
# scripts/evidence-artifacts.sh (P14):
#   # build=1.0.<short-sha> # commit=<full-sha> # dirty=<true|false>
# The anchor is the issue's fix commit (FIX_COMMIT overrides), never HEAD after
# the evidence commit, so a later re-run keeps pointing at the same source.
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r1b|r2|r3|r4|r5|r6|r7|gate) ;;
  *) echo "usage: $0 r1|r1b|r2|r3|r4|r5|r6|r7|gate" >&2; exit 2 ;;
esac

ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not a git repository" >&2; exit 2; }
cd "$ROOT" || exit 2

if ! command -v go >/dev/null 2>&1 && [ -x /usr/local/go/bin/go ]; then
  export PATH="/usr/local/go/bin:$PATH"
fi

# Backend integration tests use the isolated test database created/cloned by
# backend/handlers/main_test.go (see docs/ops/test-db-lifecycle.md).
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test}"
export TMPDIR="${TMPDIR:-$ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"

FIX_SHA="${FIX_COMMIT:-$(git log --format=%H --grep='^fix(issue-2552)' -1 2>/dev/null)}"
ANCHOR_SHA="${FIX_SHA:-$(git rev-parse HEAD)}"
SHORT_SHA="$(git rev-parse --short "$ANCHOR_SHA")"
BUILD="1.0.${SHORT_SHA}"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence' ':!tmp')" ]; then
  DIRTY=true
fi
echo "# build=${BUILD} # commit=${ANCHOR_SHA} # dirty=${DIRTY}"

fail() {
  echo "FAIL(${ROW}): $*" >&2
  exit 1
}

HEAVY="$HOME/.config/opencode/scripts/heavy-run.sh"
heavy() { bash "$HEAVY" -- "$@"; }

TOKEN_RE='^(MEMBER_|USER_|CREATE_|DUPLICATE_NAME_|NO_SITE_|NO_DUPLICATE_|PRIMARY_ORG_|ORPHAN_|SITE_ROWS_|SYSTEM_VIEW_|BYID_|NEG_|CLAIM_|OTHER_TENANT_|STORE_MEMBER_|TRANSIT_MEMBER_|DRYRUN_|APPLY_|SECOND_APPLY_|MEMBERS_|IAM_FAILURE_|UPDATE_FAILURE_|--- (PASS|FAIL))'

go_test_row() { # pkg test-pattern...
  local pkg="$1" pattern="$2" out rc
  out="$(mktemp)"
  ( cd backend && heavy go test "$pkg" -run "$pattern" -count=1 -v ) > "$out" 2>&1
  rc=$?
  grep -E "$TOKEN_RE" "$out" || true
  if [ "$rc" -ne 0 ]; then
    echo "--- go test output (tail) ---"
    tail -40 "$out"
    rm -f "$out"
    fail "go test ${pkg} -run ${pattern} exit=${rc}"
  fi
  rm -f "$out"
}

run_row() { # pkg pattern expected-token...
  local pkg="$1" pattern="$2"; shift 2
  local out; out="$(mktemp)"
  go_test_row "$pkg" "$pattern" > "$out" 2>&1
  cat "$out"
  local tok
  for tok in "$@"; do
    grep -qx -- "$tok" "$out" || fail "missing expected token '$tok'"
  done
  rm -f "$out"
}

REG_PATTERN='TestTransit|TestCreateAdminTransitSiteValidation|TestPersonnel2065|TestPersonnel2508|TestPersonnelTransitPredicate2552|TestStaffDetail2526|TestListControlledSitesExcludesTransit'

case "$ROW" in
  r1)
    echo "== R1 D2：成员 tenant 列 = 本地租户；IAM org = 站点 org（bind/role/unbind）；重复添加不增行 =="
    # 现象级差异（实施前先跑）：第 4 参数必须是 site.TenantID，且不得再有 bindOrgID 回退。
    BIND_ORG_AS_TENANT="$(grep -c 'addMembersCore(c, db, site, bindOrgID' backend/handlers/transit_site_admin.go || true)"
    LOCAL_TENANT_ARG="$(grep -c 'addMembersCore(c, db, site, site.TenantID' backend/handlers/transit_site_admin.go || true)"
    echo "BIND_ORG_AS_TENANT=${BIND_ORG_AS_TENANT}"
    echo "LOCAL_TENANT_ARG=${LOCAL_TENANT_ARG}"
    [ "${BIND_ORG_AS_TENANT:-0}" = "0" ] || fail "AddTransitSiteMember 仍把 org 当本地租户作用域"
    [ "${LOCAL_TENANT_ARG:-0}" = "1" ] || fail "AddTransitSiteMember 未传 site.TenantID"

    run_row ./handlers 'TestTransitMemberScope2552' \
      "MEMBER_ROWS=1" \
      "MEMBER_TENANT_IS_LOCAL_TENANT=1" \
      "MEMBER_IAM_ORG_IS_SITE_ORG=1" \
      "NO_DUPLICATE_MEMBER_ROW=1" \
      "USER_TENANT_IS_LOCAL_TENANT=1"
    # Rev.2（Round 1 审计 Bug 1）：bind / role / unbind 三条 IAM 路径的 org 段都必须
    # 等于 sites.org_id；夹具 seedTransitSite2552 保证 site.OrgID ≠ site.TenantID。
    run_row ./handlers 'TestTransitMemberOrgPaths2552_RoleAndUnbind' \
      "MEMBER_ROLE_ORG_IS_SITE_ORG=1" \
      "MEMBER_UNBIND_ORG_IS_SITE_ORG=1"
    echo "R1_OK"
    ;;

  r1b)
    echo "== R1b（已并入 R1，Rev.2）：role/unbind 两路径 org 段 = 站点组织（sites.org_id） =="
    # 审计 Round 1 Bug 1：计划 P1-1 要求锁定三条路径，原实现只锁了 bind。
    run_row ./handlers 'TestTransitMemberOrgPaths2552_RoleAndUnbind' \
      "MEMBER_ROLE_ORG_IS_SITE_ORG=1" \
      "MEMBER_UNBIND_ORG_IS_SITE_ORG=1"
    echo "R1b_OK"
    ;;

  r2)
    echo "== R2 中转成员识别改为站点维度（列表 + by-id + 负例） =="
    # 判据守卫必须只看**中转**谓词：personnel.go 其他分支正常使用 `sites.org_id`
    # （site_admin 视图/site 成员判定），不能用全文件 grep 误伤。
    ORG_IN_TRANSIT="$(grep -E 's\.type = \?' backend/handlers/personnel.go | grep -c 'org_id' || true)"
    TENANT_RULE="$(grep -cE 's\.type = \?.*s\.tenant_id = \?' backend/handlers/personnel.go || true)"
    echo "OLD_ORG_EQUALS_TENANT_RULE=${ORG_IN_TRANSIT:-0}"
    echo "TRANSIT_TENANT_RULE_SITES=${TENANT_RULE:-0}"
    [ "${ORG_IN_TRANSIT:-0}" = "0" ] || fail "中转判据仍以 org_id 比较"
    [ "${TENANT_RULE:-0}" -ge 2 ] || fail "中转判据未改为站点维度 s.tenant_id（system view + by-id 各一处）"

    run_row ./handlers 'TestPersonnelTransitPredicate2552' \
      "SYSTEM_VIEW_TRANSIT=1" \
      "BYID_TRANSIT=200" \
      "NEG_STORE_SAME_TENANT=0" \
      "NEG_OTHER_TENANT_TRANSIT=0" \
      "BYID_STORE_SAME_TENANT=200" \
      "BYID_STORE_NO_TID=404" \
      "CLAIM_SYSTEM_VIEW_TRANSIT=1" \
      "CLAIM_NEG_STORE_SAME_TENANT=0" \
      "CLAIM_NEG_OTHER_TENANT_TRANSIT=0" \
      "OTHER_TENANT_TRANSIT_MEMBERSHIP=0" \
      "STORE_MEMBER_TRANSIT_MEMBERSHIP=0" \
      "TRANSIT_MEMBER_TRANSIT_MEMBERSHIP=1"
    echo "R2_OK"
    ;;

  r3)
    echo "== R3 建站契约：type=branch / parent=主组织 / 错误映射 / 孤儿补偿 =="
    run_row ./handlers 'TestTransitSiteCreate2552' \
      "CREATE_TYPE_BRANCH=1" \
      "CREATE_PARENT_IS_NAMESPACE=1" \
      "DUPLICATE_NAME_409=1" \
      "NO_SITE_ON_EMPTY_TENANT=1" \
      "PRIMARY_ORG_UNRESOLVED=50000" \
      "ORPHAN_ORG_COMPENSATED=1" \
      "SITE_ROWS_ON_FAILURE=0"
    echo "R3_OK"
    ;;

  r4)
    echo "== R4 回填工具：dry-run 0 写入 + apply 幂等 + members 阶段 =="
    ls backend/services/transit_site_org_backfill.go backend/tools/backfill_transit_site_orgs/main.go >/dev/null 2>&1 \
      || fail "回填服务或 CLI 缺失"
    # 不挂启动路径：只能由 CLI 调用。
    WIRED="$(grep -rn 'ApplyTransitSiteOrgBackfill' backend --include='*.go' | grep -v '_test.go' | grep -v 'services/transit_site_org_backfill.go' | grep -vc 'tools/backfill_transit_site_orgs/main.go' || true)"
    echo "BACKFILL_WIRED_TO_STARTUP=${WIRED:-0}"
    [ "${WIRED:-0}" = "0" ] || fail "回填工具被挂到了启动路径（应仅 CLI 调用）"

    run_row ./services 'TestTransitBackfill2552' \
      "DRYRUN_WRITES=0" \
      "DRYRUN_IAM_CREATES=0" \
      "APPLY_CREATED=2" \
      "APPLY_UPDATED=2" \
      "SECOND_APPLY_UPDATED=0" \
      "MEMBERS_REBIND=1" \
      "IAM_FAILURE_EXPLICIT=1" \
      "UPDATE_FAILURE_COMPENSATED=1"
    echo "R4_OK"
    ;;

  r5)
    echo "== R5 四份文档口径同步 =="
    API="docs/spec/api/api.md"
    CASE="docs/cases/transit.md"
    AUDIT="docs/ops/tenant-id-audit.md"
    DB="docs/spec/database/database.md"

    DOC_API_TRANSIT_TENANT=0
    if grep -q "sites.tenant_id = 平台租户" "$API" && grep -q "branch" "$API"; then DOC_API_TRANSIT_TENANT=1; fi
    DOC_CASE_TRANSIT_ORG=0
    if grep -q "kind=branch" "$CASE" && grep -q "sites.tenant_id" "$CASE"; then DOC_CASE_TRANSIT_ORG=1; fi
    DOC_OPS_AUDIT_ROW=0
    if grep -q "branch\` 子组织" "$AUDIT" || grep -q "branch. 子组织" "$AUDIT"; then DOC_OPS_AUDIT_ROW=1; fi
    DOC_DB_SITES_ORG=0
    if grep -q "branch\` 子组织" "$DB" && grep -q "禁止互相替代" "$DB"; then DOC_DB_SITES_ORG=1; fi

    echo "DOC_API_TRANSIT_TENANT=${DOC_API_TRANSIT_TENANT}"
    echo "DOC_CASE_TRANSIT_ORG=${DOC_CASE_TRANSIT_ORG}"
    echo "DOC_OPS_AUDIT_ROW=${DOC_OPS_AUDIT_ROW}"
    echo "DOC_DB_SITES_ORG=${DOC_DB_SITES_ORG}"
    [ "$DOC_API_TRANSIT_TENANT" -eq 1 ] || fail "$API 未同步中转判据/子组织口径"
    [ "$DOC_CASE_TRANSIT_ORG" -eq 1 ] || fail "$CASE 未同步归属口径"
    [ "$DOC_OPS_AUDIT_ROW" -eq 1 ] || fail "$AUDIT 未同步 transit_site_admin 行"
    [ "$DOC_DB_SITES_ORG" -eq 1 ] || fail "$DB 未同步 sites.org_id 语义"
    echo "R5_OK"
    ;;

  r6|gate)
    echo "== R6 构建 + gofmt + 定向回归 =="
    BUILD_OUT="$(mktemp)"
    ( cd backend && heavy go build ./... ) > "$BUILD_OUT" 2>&1
    BUILD_RC=$?
    tail -5 "$BUILD_OUT"
    rm -f "$BUILD_OUT"

    GOFMT_OUT="$(mktemp)"
    ( cd backend && gofmt -l handlers/transit_site_admin.go handlers/personnel.go services/iam_client.go \
        services/transit_site_org_backfill.go tools/backfill_transit_site_orgs/main.go \
        handlers/transit_site_admin_2552_test.go handlers/personnel_transit_2552_test.go \
        services/transit_site_org_backfill_2552_test.go ) > "$GOFMT_OUT" 2>&1
    GOFMT_DIFF="$(grep -c . "$GOFMT_OUT" || true)"
    cat "$GOFMT_OUT"
    rm -f "$GOFMT_OUT"

    REG_OUT="$(mktemp)"
    ( cd backend && heavy go test ./handlers -count=1 -v -run "$REG_PATTERN" ) > "$REG_OUT" 2>&1
    REG_RC=$?
    FAIL_COUNT="$(grep -c '^--- FAIL' "$REG_OUT" || true)"
    PASS_COUNT="$(grep -c '^--- PASS' "$REG_OUT" || true)"
    grep -E '^--- (PASS|FAIL)' "$REG_OUT" | tail -40 || true
    tail -3 "$REG_OUT"
    REGRESSION_PASS=0
    if [ "$REG_RC" -eq 0 ] && [ "${FAIL_COUNT:-1}" = "0" ] && [ "${PASS_COUNT:-0}" -gt 0 ]; then REGRESSION_PASS=1; fi
    HAVE_TRANSIT="$(grep -c -- '--- PASS: TestTransitSiteMemberIAMSync' "$REG_OUT" || true)"
    HAVE_PERSONNEL="$(grep -c -- '--- PASS: TestPersonnel2065_SystemView' "$REG_OUT" || true)"
    rm -f "$REG_OUT"

    echo "BUILD_RC=${BUILD_RC}"
    echo "GOFMT_DIFF=${GOFMT_DIFF:-0}"
    echo "REGRESSION_RC=${REG_RC}"
    echo "REGRESSION_PASS=${REGRESSION_PASS}"
    echo "FAIL_COUNT=${FAIL_COUNT:-0}"
    echo "REGRESSION_PASS_COUNT=${PASS_COUNT:-0}"
    echo "EXISTING_TRANSIT_IAM_PASS=${HAVE_TRANSIT:-0}"
    echo "EXISTING_PERSONNEL_SYSTEM_VIEW_PASS=${HAVE_PERSONNEL:-0}"

    [ "$BUILD_RC" -eq 0 ] || fail "go build ./... failed (rc=${BUILD_RC})"
    [ "${GOFMT_DIFF:-1}" = "0" ] || fail "gofmt diff present (${GOFMT_DIFF:-?} files)"
    [ "$REGRESSION_PASS" -eq 1 ] || fail "regression suite failed (rc=${REG_RC}, fails=${FAIL_COUNT:-?})"
    [ "${HAVE_TRANSIT:-0}" -ge 1 ] || fail "existing transit member IAM case did not PASS"
    [ "${HAVE_PERSONNEL:-0}" -ge 1 ] || fail "existing personnel system view case did not PASS"
    echo "R6_OK"
    ;;

  r7)
    echo "== R7 plan 四源 FRESH + plan.md 被 git 跟踪 =="
    # 计划口径（R7 命令列）：只比对最新 Accepted Plan 评论 ↔ plan.md（--fresh）。
    # 四源（含 gate / evidence.json）一致性由 pre-ready gate 的 P15 独立校验，
    # 避免 R7 自身产物形成循环依赖（D6：合并后 origin/develop 可读性归 #2554）。
    PS_OUT="$(bash "$HOME/.config/opencode/scripts/plan-source.sh" --issue 2552 --fresh 2>&1)"
    PS_RC=$?
    echo "$PS_OUT"
    PLAN_SOURCE_FRESH=E_PLAN_STALE
    case "$PS_OUT" in *FRESH*) PLAN_SOURCE_FRESH=PASS ;; esac
    PLAN_MD_TRACKED=0
    if git ls-files --error-unmatch docs/evidence/2552/plan.md >/dev/null 2>&1; then PLAN_MD_TRACKED=1; fi
    echo "PLAN_SOURCE_FRESH=${PLAN_SOURCE_FRESH}"
    echo "PLAN_MD_TRACKED=${PLAN_MD_TRACKED}"
    echo "PLAN_SOURCE_RC=${PS_RC}"
    [ "$PLAN_SOURCE_FRESH" = "PASS" ] || fail "plan-source.sh 四源不一致（${PS_OUT}）"
    [ "$PLAN_MD_TRACKED" = "1" ] || fail "docs/evidence/2552/plan.md 未被 git 跟踪"
    echo "R7_OK"
    ;;
esac
