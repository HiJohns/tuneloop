#!/usr/bin/env bash
# #2540 verification harness — one subcommand per Evidence Contract row.
#
#   bash scripts/verify-2540.sh r1   # 激活：关系角色 + 权限模板同源（site_member/site_admin/legacy/customer/unknown/platform）
#   bash scripts/verify-2540.sh r2   # 批量导入：关系角色经映射 + 白名单兜底
#   bash scripts/verify-2540.sh r3   # 静态：不得再把业务角色当 IAM 关系角色 / 不得硬编码 site_member 模板
#   bash scripts/verify-2540.sh r4   # 文档：activate + 批量导入 + 商户管理员 OWNER 口径
#   bash scripts/verify-2540.sh r5   # 构建 + 定向测试 + 全量基线差异（Rev.3 / 审计 Bug 2）
#   bash scripts/verify-2540.sh r6   # 商户管理员确认：绑定 OWNER
#
# Capture a row into its artifact:
#   bash scripts/verify-2540.sh r1 > docs/evidence/2540/R1-activate-role.txt
#
# Every artifact starts with the provenance header consumed by
# scripts/evidence-artifacts.sh (P14):
#   # build=1.0.<short-sha> # commit=<full-sha> # dirty=<true|false>
# FIX_COMMIT overrides the anchor so a re-run after the evidence commit keeps
# pointing at the fix commit.
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

# Backend tests need the Postgres test cluster (docs/ops/test-db-lifecycle.md).
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_DB_SIMPLE_PROTOCOL="${TUNELOOP_DB_SIMPLE_PROTOCOL:-1}"
export TMPDIR="${TMPDIR:-$ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"

FIX_SHA="${FIX_COMMIT:-$(git log --format=%H --grep='^fix(issue-2540)' -1 2>/dev/null)}"
ANCHOR_SHA="${FIX_SHA:-$(git rev-parse HEAD)}"
SHORT_SHA="$(git rev-parse --short "$ANCHOR_SHA")"
BUILD="1.0.${SHORT_SHA}"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence' ':!tmp')" ]; then
  DIRTY=true
fi
echo "# build=${BUILD} # commit=${ANCHOR_SHA} # dirty=${DIRTY}"
# 计划 ### Artifact Provenance：首行之后再锚定一次生成本产物时的 HEAD（可不同于 fix commit）。
echo "GENERATED_AT_HEAD=$(git rev-parse HEAD)"

fail() {
  echo "FAIL(${ROW}): $*" >&2
  exit 1
}

HEAVY="$HOME/.config/opencode/scripts/heavy-run.sh"
heavy() { bash "$HEAVY" -- "$@"; }

run_row() { # package test-name expected-token...
  local pkg="$1" name="$2"; shift 2
  local out; out="$(mktemp)"
  # 必须在本层检查 rc：函数内 fail 的 stderr 会随重定向落入临时文件而丢失。
  ( cd backend && heavy go test "$pkg" -run "${name}" -count=1 -v ) > "$out" 2>&1
  local rc=$?
  grep -E '^(MEMBER_|ADMIN_|LEGACY_|CUSTOMER_|UNKNOWN_|PLATFORM_|ILLEGAL_|PERM_|CUS_|BIND_|BUSINESS_|NON_|ROLE_TEMPLATE_)' "$out" || true
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
    echo "== R1 激活：关系角色 + 权限模板 + 功能角色模板同源 =="
    run_row ./handlers TestActivateUser2540_RoleMapping \
      "MEMBER_BIND_ROLE=STAFF" \
      "MEMBER_ROLE_TEMPLATE=site_member" \
      "ADMIN_BIND_ROLE=ADMIN" \
      "ADMIN_ROLE_TEMPLATE=site_admin" \
      "LEGACY_BIND_ROLE=STAFF" \
      "LEGACY_ROLE_TEMPLATE=site_member" \
      "CUSTOMER_BIND_ROLE=STAFF" \
      "UNKNOWN_BIND_ROLE=STAFF" \
      "PLATFORM_BIND_ROLE=STAFF" \
      "PLATFORM_ROLE_TEMPLATE=site_member" \
      "ILLEGAL_ROLE_COUNT=0" \
      "ILLEGAL_TEMPLATE_COUNT=0" \
      "ADMIN_PERM_TEMPLATE=site_admin" \
      "PERM_TEMPLATE=site_member" \
      "CUS_PERM_MATCH=1" \
      "ROLE_TEMPLATE_MISMATCH=0"
    echo "R1_OK"
    ;;

  r2)
    echo "== R2 批量导入：关系角色映射 + 白名单兜底 =="
    run_row ./services TestBulkImportIAMRole2540 \
      "BIND_ROLES=STAFF,ADMIN" \
      "LEGACY_BIND_ROLE=STAFF" \
      "BUSINESS_ROLE_AS_IAM=0"
    echo "R2_OK"
    ;;

  r3)
    echo "== R3 静态：业务角色不得作为 IAM 关系角色 =="
    STAFF_SRC="backend/handlers/user_staff.go"
    BULK_SRC="backend/services/bulk_import_accounts.go"
    [ -f "$STAFF_SRC" ] || fail "missing $STAFF_SRC"
    [ -f "$BULK_SRC" ] || fail "missing $BULK_SRC"

    # BindUserToOrganization 的第三参数不得来自 GetBusinessRole（仅允许 toIAMRole/Normalize）。
    BIND_WITH_BUSINESS_ROLE=$(
      grep -rn 'BindUserToOrganization(' "$STAFF_SRC" "$BULK_SRC" \
        | grep -c 'GetBusinessRole' || true
    )
    # 激活路径不得再硬编码 site_member 模板（改由 roleTemplate 变量）。
    HARDCODED_SITE_MEMBER=$(
      grep -n 'AllRoleTemplates\["site_member"\]' "$STAFF_SRC" \
        | awk -F: '$1 >= 600 && $1 <= 760' \
        | wc -l | tr -d ' '
    )
    # 关系角色赋值不得来自 GetBusinessRole（业务展示角色），只能经 ToIAMRole/Normalize。
    GETBUSINESSROLE_AS_IAMROLE=$(
      grep -hE -c 'iamRole := (services\.)?GetBusinessRole\(' "$STAFF_SRC" "$BULK_SRC" \
        | awk '{s+=$1} END {print s+0}'
    )
    # 映射下沉：services 必须有唯一实现，handlers.toIAMRole 必须是薄封装。
    SERVICE_MAPPING="$(grep -c '^func ToIAMRole(' backend/services/role_templates.go || true)"
    HANDLER_THIN="$(grep -c 'return services.ToIAMRole(tuneloopCode)' backend/handlers/permission_manage.go || true)"
    NORMALIZE="$(grep -c '^func NormalizeIAMRelationRole(' backend/services/role_templates.go || true)"

    echo "BIND_WITH_BUSINESS_ROLE=${BIND_WITH_BUSINESS_ROLE}"
    echo "GETBUSINESSROLE_AS_IAMROLE=${GETBUSINESSROLE_AS_IAMROLE}"
    echo "HARDCODED_SITE_MEMBER=${HARDCODED_SITE_MEMBER}"
    echo "SERVICE_TOIAMROLE=${SERVICE_MAPPING}"
    echo "HANDLER_THIN_WRAPPER=${HANDLER_THIN}"
    echo "NORMALIZE_WHITELIST=${NORMALIZE}"

    [ "$BIND_WITH_BUSINESS_ROLE" = "0" ] || fail "BindUserToOrganization 第三参数仍来自 GetBusinessRole"
    [ "$GETBUSINESSROLE_AS_IAMROLE" = "0" ] || fail "iamRole 仍直接取自 GetBusinessRole（业务角色当 IAM 角色）"
    [ "$HARDCODED_SITE_MEMBER" = "0" ] || fail "ActivateUser 仍硬编码 AllRoleTemplates[\"site_member\"]"
    [ "$SERVICE_MAPPING" = "1" ] || fail "services.ToIAMRole 必须恰好一个实现"
    [ "$HANDLER_THIN" = "1" ] || fail "handlers.toIAMRole 必须是 services.ToIAMRole 的薄封装"
    [ "$NORMALIZE" = "1" ] || fail "services.NormalizeIAMRelationRole 必须存在（白名单兜底）"
    echo "R3_OK"
    ;;

  r4)
    echo "== R4 文档：关系角色口径 =="
    DOC="docs/spec/api/api.md"
    [ -f "$DOC" ] || fail "missing $DOC"
    DOC_TOIAMROLE=$(grep -c 'ToIAMRole' "$DOC" || true)
    DOC_STAFF_FALLBACK=$(grep -c 'NormalizeIAMRelationRole' "$DOC" || true)
    DOC_ACTIVATE_SECTION=$(grep -cE '^#### 10\.18\.[0-9]+ 激活员工' "$DOC" || true)
    DOC_MERCHANT_OWNER=$(grep -c '商户管理员.*OWNER' "$DOC" || true)
    DOC_ROLE_TEMPLATE=$(grep -c '功能角色模板 | 同样来自' "$DOC" || true)
    echo "DOC_TOIAMROLE=${DOC_TOIAMROLE}"
    echo "DOC_STAFF_FALLBACK=${DOC_STAFF_FALLBACK}"
    echo "DOC_ACTIVATE_SECTION=${DOC_ACTIVATE_SECTION}"
    echo "DOC_MERCHANT_OWNER=${DOC_MERCHANT_OWNER}"
    echo "DOC_ROLE_TEMPLATE=${DOC_ROLE_TEMPLATE}"
    [ "${DOC_TOIAMROLE:-0}" -ge 1 ] || fail "docs 未写 ToIAMRole"
    [ "${DOC_STAFF_FALLBACK:-0}" -ge 1 ] || fail "docs 未写 NormalizeIAMRelationRole 兜底"
    [ "${DOC_ACTIVATE_SECTION:-0}" -ge 1 ] || fail "docs 缺 activate 口径小节"
    [ "${DOC_MERCHANT_OWNER:-0}" -ge 1 ] || fail "docs 缺商户管理员绑定 OWNER 口径"
    [ "${DOC_ROLE_TEMPLATE:-0}" -ge 1 ] || fail "docs 缺「功能角色模板同样来自 roleTemplate」口径"
    echo "R4_OK"
    ;;

  r5)
    echo "== R5 构建 + 定向测试 + 全量基线差异 =="
    BUILD_OUT="$(mktemp)"
    ( cd backend && heavy go build ./... ) > "$BUILD_OUT" 2>&1
    BUILD_RC=$?
    tail -5 "$BUILD_OUT"
    rm -f "$BUILD_OUT"

    TEST_OUT="$(mktemp)"
    ( cd backend && heavy go test ./handlers ./services \
        -run 'TestActivateUser2540|TestResolveActivationRoleTemplate2540|TestBulkImportIAMRole2540|TestToIAMRoleMapping2540|TestMerchantAdminConfirmBindsOwner2540|TestUpdateUserRoleForbidden_2375|TestSiteTransfer_' \
        -count=1 ) > "$TEST_OUT" 2>&1
    TEST_RC=$?
    tail -10 "$TEST_OUT"
    rm -f "$TEST_OUT"

    GO_TEST_OK=0
    [ "$TEST_RC" = "0" ] && GO_TEST_OK=1
    BUILD_OK=0
    [ "$BUILD_RC" = "0" ] && BUILD_OK=1
    echo "GO_TEST_OK=${GO_TEST_OK}"
    echo "BUILD_OK=${BUILD_OK}"
    [ "$BUILD_OK" = "1" ] || fail "go build ./... 失败"
    [ "$GO_TEST_OK" = "1" ] || fail "定向测试失败"

    # Rev.3 / 审计 Bug 2：仓库根无 go.mod → pre-ready P5 判 N/A；本行改用
    # 「全量 go test ./... 相对基线（origin/develop）的失败集差异」证明未引入新增失败。
    # 基线里同集失败属测试集群环境耦合（缺迁移/共享库），不属本单范围。
    BASELINE_REF="${BASELINE_REF:-origin/develop}"
    BASELINE_DIR="$ROOT/tmp/full-gate-baseline"
    cleanup_baseline() {
      if git -C "$ROOT" worktree list --porcelain | grep -qx "worktree ${BASELINE_DIR}"; then
        git -C "$ROOT" worktree remove --force "$BASELINE_DIR" >/dev/null 2>&1 || true
      fi
      rm -rf "$BASELINE_DIR"
    }
    trap cleanup_baseline EXIT
    cleanup_baseline
    git -C "$ROOT" worktree add --detach "$BASELINE_DIR" "$BASELINE_REF" >/dev/null 2>&1 \
      || fail "无法创建基线 worktree（${BASELINE_REF}）；用 BASELINE_REF=<ref> 覆盖"
    BASELINE_SHA="$(git -C "$ROOT" rev-parse "${BASELINE_REF}^{commit}")"

    # go test -json：按 (package, test) 归一化失败身份，避免只数条数。
    extract_failures() {
      grep '^{' "$1" | jq -r 'select(.Action == "fail") | "\(.Package)::\(.Test // "PKG")"' | LC_ALL=C sort -u
    }

    FULL_GATE_OUT="$(mktemp)"
    ( cd backend && heavy go test -json ./... -count=1 ) > "$FULL_GATE_OUT" 2>&1
    FULL_GATE_EXIT=$?
    CURRENT_FAILURES="$(mktemp)"
    extract_failures "$FULL_GATE_OUT" > "$CURRENT_FAILURES"

    BASE_GATE_OUT="$(mktemp)"
    ( cd "$BASELINE_DIR/backend" && heavy go test -json ./... -count=1 ) > "$BASE_GATE_OUT" 2>&1
    BASELINE_GATE_EXIT=$?
    BASELINE_FAILURES="$(mktemp)"
    extract_failures "$BASE_GATE_OUT" > "$BASELINE_FAILURES"

    NEW_FAILURES="$(mktemp)"
    LC_ALL=C comm -13 "$BASELINE_FAILURES" "$CURRENT_FAILURES" > "$NEW_FAILURES"
    NEW_COUNT="$(wc -l < "$NEW_FAILURES" | tr -d ' ')"

    echo "BASELINE_REF=${BASELINE_REF}"
    echo "BASELINE_SHA=${BASELINE_SHA}"
    echo "BASELINE_GATE_EXIT=${BASELINE_GATE_EXIT}"
    echo "FULL_GATE_EXIT=${FULL_GATE_EXIT}"
    echo "FULL_GATE_CURRENT_FAILURES=$(wc -l < "$CURRENT_FAILURES" | tr -d ' ')"
    echo "FULL_GATE_BASELINE_FAILURES=$(wc -l < "$BASELINE_FAILURES" | tr -d ' ')"
    echo "FULL_GATE_NEW_FAILURES=${NEW_COUNT}"
    if [ -s "$NEW_FAILURES" ]; then
      echo "--- 新增失败（基线中没有，本单引入） ---"
      cat "$NEW_FAILURES"
    fi
    rm -f "$FULL_GATE_OUT" "$BASE_GATE_OUT" "$CURRENT_FAILURES" "$BASELINE_FAILURES" "$NEW_FAILURES"

    # 计划 R5 登记 FULL_GATE_EXIT=1（既有环境耦合失败）；全绿使该口径失真 → 回 Architect。
    [ "$FULL_GATE_EXIT" = "1" ] || fail "全量 gate 退出码与计划登记值不一致（计划 FULL_GATE_EXIT=1，实际 ${FULL_GATE_EXIT}）→ 回 Architect 更新 R5"
    [ "$NEW_COUNT" = "0" ] || fail "全量 gate 出现 ${NEW_COUNT} 个基线中不存在的失败（见上）"
    echo "R5_OK"
    ;;

  r6)
    echo "== R6 商户管理员确认：关系角色 OWNER =="
    run_row ./handlers TestMerchantAdminConfirmBindsOwner2540 \
      "BIND_ROLE=OWNER" \
      "PERM_TEMPLATE=merchant_admin" \
      "CUS_PERM_MATCH=1" \
      "NON_OWNER_ROLE_COUNT=0"
    # 负例（静态）：商户管理员确认路径不得再写字面量 "admin"/"ADMIN" 作为关系角色。
    MERCHANT_LITERAL_ADMIN=$(
      grep -n 'BindUserToOrganization(' backend/handlers/confirmation_session.go \
        | grep -ci '"admin"\|"ADMIN"' || true
    )
    echo "MERCHANT_LITERAL_ADMIN=${MERCHANT_LITERAL_ADMIN}"
    [ "$MERCHANT_LITERAL_ADMIN" = "0" ] || fail "商户管理员确认仍用字面量 admin/ADMIN 绑定关系角色"
    echo "R6_OK"
    ;;
esac
