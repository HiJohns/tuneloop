#!/usr/bin/env bash
# #2516 verification harness — one subcommand per Evidence Contract row.
#
#   bash scripts/verify-2516.sh r1   # apiResult terminal contract (node --test)
#   bash scripts/verify-2516.sh r2   # tenant page static structure
#   bash scripts/verify-2516.sh r3   # dead tenant write helpers removed
#   bash scripts/verify-2516.sh r4   # /system path contract scan (frontend vs backend)
#   bash scripts/verify-2516.sh r5   # docs sync (api.md §12.2 / ui.md §5)
#   bash scripts/verify-2516.sh r6   # frontend build + eslint no-undef
#
# Each row prints to stdout; capture it into the artifact path from the plan:
#   bash scripts/verify-2516.sh r1 > docs/evidence/2516/R1-api-result-unit.txt
#
# Every artifact starts with the provenance header consumed by
# scripts/evidence-artifacts.sh (P14):
#   # build=1.0.<short-sha> # commit=<full-sha> # dirty=<true|false>
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r2|r3|r4|r5|r6) ;;
  *) echo "usage: $0 r1|r2|r3|r4|r5|r6" >&2; exit 2 ;;
esac

ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not a git repository" >&2; exit 2; }
cd "$ROOT" || exit 2

HEAD_SHA="$(git rev-parse HEAD)"
SHORT_SHA="$(git rev-parse --short HEAD)"
BUILD="1.0.${SHORT_SHA}"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence')" ]; then
  DIRTY=true
fi
echo "# build=${BUILD} # commit=${HEAD_SHA} # dirty=${DIRTY}"

fail() { echo "R${ROW#r}_FAIL: $*" >&2; exit 1; }

PAGE="frontend-pc/src/pages/TenantManagement.jsx"
API="frontend-pc/src/services/api.js"

case "$ROW" in
  r1)
    echo "== R1 apiResult terminal contract (node --test) =="
    OUT="$(mktemp)"
    ( cd frontend-pc && node --test tests/apiResult.test.mjs ) > "$OUT" 2>&1
    RC=$?
    cat "$OUT"
    PASS_N="$(sed -n 's/^# pass \([0-9][0-9]*\)$/\1/p' "$OUT" | tail -1)"
    FAIL_N="$(sed -n 's/^# fail \([0-9][0-9]*\)$/\1/p' "$OUT" | tail -1)"
    rm -f "$OUT"
    echo "RESULT: PASS ${PASS_N:-0} FAIL ${FAIL_N:-0}"
    [ "$RC" -eq 0 ] || fail "node --test exit=$RC"
    [ "${FAIL_N:-1}" = "0" ] || fail "node --test reported failures (fail=${FAIL_N:-missing})"
    [ "${PASS_N:-0}" -ge 1 ] || fail "node --test ran no passing test"
    ;;

  r2)
    echo "== R2 tenant page static structure ($PAGE) =="
    [ -f "$PAGE" ] || fail "page not found: $PAGE"
    # 写入口 / 创建 Modal 必须清零（负例：残留 api.post('/system/tenants') 或 Modal）
    WRITE_ENTRY="$(grep -cE "api\.post\('/system/tenants'|handleCreateTenant|>创建租户<" "$PAGE")"
    CREATE_MODAL="$(grep -c 'Modal' "$PAGE")"
    # 读路径必须终判 code，失败分支给出后端 message
    CODE_GUARD=0
    grep -q 'isApiOk(res)' "$PAGE" && CODE_GUARD=1
    FAIL_BRANCH_MSG=0
    grep -q 'message.error(apiFailureMessage(' "$PAGE" && FAIL_BRANCH_MSG=1
    # 列表字段对齐真实响应：租户ID 用 id，状态按 record.status 动态渲染
    COLUMN_ID=0
    grep -q "dataIndex: 'id'" "$PAGE" && COLUMN_ID=1
    COLUMN_STATUS_DYNAMIC=0
    if grep -q "dataIndex: 'status'" "$PAGE" && \
       grep -q "active: { text: '启用'" "$PAGE" && \
       grep -q "inactive: { text: '停用'" "$PAGE" && \
       grep -q "text: '未知'" "$PAGE"; then
      COLUMN_STATUS_DYNAMIC=1
    fi
    # 只读说明必须存在
    READONLY_HINT=0
    grep -q '租户随商户创建自动生成' "$PAGE" && grep -q '<Alert' "$PAGE" && READONLY_HINT=1

    echo "WRITE_ENTRY=$WRITE_ENTRY"
    echo "CREATE_MODAL=$CREATE_MODAL"
    echo "CODE_GUARD=$CODE_GUARD"
    echo "FAIL_BRANCH_MSG=$FAIL_BRANCH_MSG"
    echo "COLUMN_ID=$COLUMN_ID"
    echo "COLUMN_STATUS_DYNAMIC=$COLUMN_STATUS_DYNAMIC"
    echo "READONLY_HINT=$READONLY_HINT"

    [ "$WRITE_ENTRY" = "0" ] || fail "write entry still present (WRITE_ENTRY=$WRITE_ENTRY)"
    [ "$CREATE_MODAL" = "0" ] || fail "create modal still present (CREATE_MODAL=$CREATE_MODAL)"
    [ "$CODE_GUARD" = "1" ] || fail "read path does not judge code (CODE_GUARD=0)"
    [ "$FAIL_BRANCH_MSG" = "1" ] || fail "failure branch does not surface backend message"
    [ "$COLUMN_ID" = "1" ] || fail "tenant id column not mapped to 'id'"
    [ "$COLUMN_STATUS_DYNAMIC" = "1" ] || fail "status column is not rendered from record.status"
    [ "$READONLY_HINT" = "1" ] || fail "read-only hint missing"
    echo "R2_OK"
    ;;

  r3)
    echo "== R3 dead tenant write helpers removed ($API) =="
    [ -f "$API" ] || fail "api client not found: $API"
    TENANT_HELPERS="$(grep -cE '\b(createTenant|getTenant|updateTenant|deleteTenant)\b' "$API")"
    GET_TENANTS="$(grep -c "getTenants: () => api.get('/system/tenants')" "$API")"
    # 全仓引用扫描：被删 helper 不得仍有消费方
    REPO_REFS="$(grep -rnE 'iamAdminApi\.(createTenant|getTenant|updateTenant|deleteTenant)\b' frontend-pc/src | wc -l | tr -d ' ')"
    echo "TENANT_HELPERS=$TENANT_HELPERS"
    echo "GET_TENANTS=$GET_TENANTS"
    echo "TENANT_HELPER_REFS=$REPO_REFS"
    [ "$TENANT_HELPERS" = "0" ] || fail "tenant write helper still defined (TENANT_HELPERS=$TENANT_HELPERS)"
    [ "$GET_TENANTS" = "1" ] || fail "getTenants must stay defined exactly once (GET_TENANTS=$GET_TENANTS)"
    [ "$REPO_REFS" = "0" ] || fail "removed helpers still referenced (TENANT_HELPER_REFS=$REPO_REFS)"
    echo "R3_OK"
    ;;

  r4)
    echo "== R4 /system path contract scan (frontend pages vs backend routes) =="
    # 前端页面里的 /system 写调用：tenant 侧必须为 0；client 侧残留在 #2517 allowlist 内。
    TENANT_WRITE_MISMATCH="$(grep -rhE "api\.(post|put|delete)\(\s*[\`']/system/tenants" frontend-pc/src/pages | wc -l | tr -d ' ')"
    RESIDUAL_LIST="$(grep -rhE "api\.(post|put|delete)\(\s*[\`']/system/clients" frontend-pc/src/pages || true)"
    RESIDUAL_COUNT="$(printf '%s\n' "$RESIDUAL_LIST" | sed '/^$/d' | wc -l | tr -d ' ')"
    # #2517 声明的残留 = ClientManagement.jsx 的 3 处 client 写调用
    RESIDUAL_OUTSIDE_ALLOWLIST="$(grep -rnE "api\.(post|put|delete)\(\s*[\`']/system/clients" frontend-pc/src/pages \
      | grep -v 'ClientManagement.jsx' | wc -l | tr -d ' ')"
    # 后端：/system 只有 GET 路由，没有任何写路由
    BACKEND_SYSTEM_GET="$(grep -cE '\.GET\("/system/(clients|tenants)"' backend/main.go)"
    BACKEND_SYSTEM_WRITE="$(grep -cE '\.(POST|PUT|DELETE|PATCH)\("/system/' backend/main.go)"
    echo "TENANT_WRITE_MISMATCH=$TENANT_WRITE_MISMATCH"
    echo "RESIDUAL_CLIENT_WRITES=$RESIDUAL_COUNT (allowlist: ClientManagement.jsx ×3, owner #2517)"
    echo "RESIDUAL_OUTSIDE_ALLOWLIST=$RESIDUAL_OUTSIDE_ALLOWLIST"
    echo "BACKEND_SYSTEM_GET_ROUTES=$BACKEND_SYSTEM_GET"
    echo "BACKEND_SYSTEM_WRITE_ROUTES=$BACKEND_SYSTEM_WRITE"
    echo "-- residual set (#2517 allowlist) --"
    grep -rnE "api\.(post|put|delete)\(\s*[\`']/system/clients" frontend-pc/src/pages || true
    [ "$TENANT_WRITE_MISMATCH" = "0" ] || fail "tenant write call still present (TENANT_WRITE_MISMATCH=$TENANT_WRITE_MISMATCH)"
    [ "$RESIDUAL_OUTSIDE_ALLOWLIST" = "0" ] || fail "unexpected /system write outside #2517 allowlist ($RESIDUAL_OUTSIDE_ALLOWLIST)"
    [ "$BACKEND_SYSTEM_GET" = "2" ] || fail "backend /system GET routes drifted (BACKEND_SYSTEM_GET_ROUTES=$BACKEND_SYSTEM_GET)"
    [ "$BACKEND_SYSTEM_WRITE" = "0" ] || fail "backend gained a /system write route (BACKEND_SYSTEM_WRITE_ROUTES=$BACKEND_SYSTEM_WRITE)"
    echo "R4_OK"
    ;;

  r5)
    echo "== R5 docs sync (api.md §12.2 / ui.md §5) =="
    API_DOC_STALE="$(grep -c 'data\.tenants\|tenant_id.*tenant_name\|org_count' docs/spec/api/api.md)"
    API_DOC_NOWRITE=0
    grep -q '无\*\* POST/PUT/DELETE /api/system/tenants' docs/spec/api/api.md && API_DOC_NOWRITE=1
    API_DOC_FIELDS=0
    grep -q '"page_size": 20' docs/spec/api/api.md && grep -q '"list": \[' docs/spec/api/api.md && API_DOC_FIELDS=1
    UI_DOC_READONLY=0
    grep -q '租户管理 (`/system/tenants`)' docs/spec/ui/ui.md && \
      grep -q '只读列表' docs/spec/ui/ui.md && UI_DOC_READONLY=1
    echo "API_DOC_STALE=$API_DOC_STALE"
    echo "API_DOC_NOWRITE=$API_DOC_NOWRITE"
    echo "API_DOC_FIELDS=$API_DOC_FIELDS"
    echo "UI_DOC_READONLY=$UI_DOC_READONLY"
    [ "$API_DOC_STALE" = "0" ] || fail "api.md still documents the old tenants[] shape (API_DOC_STALE=$API_DOC_STALE)"
    [ "$API_DOC_NOWRITE" = "1" ] || fail "api.md does not state the absence of write endpoints"
    [ "$API_DOC_FIELDS" = "1" ] || fail "api.md response sample missing list/page_size"
    [ "$UI_DOC_READONLY" = "1" ] || fail "ui.md tenant entry is missing the read-only semantics"
    echo "R5_OK"
    ;;

  r6)
    echo "== R6 frontend build + eslint no-undef =="
    ( cd frontend-pc && bash "$HOME/.config/opencode/scripts/heavy-run.sh" -- npm run build ) 2>&1 | tail -25
    BUILD_EXIT="${PIPESTATUS[0]}"
    ( cd frontend-pc && npx eslint src/pages/TenantManagement.jsx src/utils/apiResult.js src/services/api.js tests/apiResult.test.mjs ) > /tmp/eslint-2516.log 2>&1
    ESLINT_RC=$?
    cat /tmp/eslint-2516.log
    ESLINT_NO_UNDEF="$(grep -c 'no-undef' /tmp/eslint-2516.log)"
    rm -f /tmp/eslint-2516.log
    echo "BUILD_EXIT=$BUILD_EXIT"
    echo "ESLINT_RC=$ESLINT_RC"
    echo "ESLINT_NO_UNDEF=$ESLINT_NO_UNDEF"
    [ "$BUILD_EXIT" = "0" ] || fail "build failed (BUILD_EXIT=$BUILD_EXIT)"
    [ "$ESLINT_NO_UNDEF" = "0" ] || fail "eslint reported no-undef (ESLINT_NO_UNDEF=$ESLINT_NO_UNDEF)"
    [ "$ESLINT_RC" = "0" ] || fail "eslint exit=$ESLINT_RC"
    echo "R6_OK"
    ;;
esac
