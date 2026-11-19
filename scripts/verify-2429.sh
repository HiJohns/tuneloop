#!/usr/bin/env bash
# #2429：编辑员工字段规则 + 站点幂等（不误触 IAM 重绑）+ 编辑页按 id 加载。
set -euo pipefail
MODE="${1:-}"
case "$MODE" in r1-go|r2-pc-build|r3-probe) ;; *) echo "usage: $0 {r1-go|r2-pc-build|r3-probe}" >&2; exit 2 ;; esac
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:${PATH}"
echo "# build=1.0.$(git rev-parse --short HEAD) commit=$(git rev-parse HEAD) dirty=$([ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2429/' || true)" ] && echo true || echo false)"
case "$MODE" in
  r1-go)
    export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}" POSTGRES_PORT="${POSTGRES_PORT:-5432}" \
           POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}" POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}" \
           TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_verify2429}" IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}" IAM_SECRET="${IAM_SECRET:-test-secret}"
    cd "$REPO_ROOT/backend"
    go test ./handlers/ -run '2429' -count=1 -v
    ;;
  r2-pc-build)
    cd "$REPO_ROOT/frontend-pc"
    export NODE_OPTIONS="--max-old-space-size=2048"
    npm run build
    ;;
  r3-probe)
    ! grep -q "siteIDChanged := true" "$REPO_ROOT/backend/handlers/user_staff.go"
    grep -q "currentSiteID != newSiteIDStr" "$REPO_ROOT/backend/handlers/user_staff.go"
    grep -q "func (h \*UserStaffHandler) GetStaffUser" "$REPO_ROOT/backend/handlers/user_staff.go"
    grep -q 'authRequired.GET("/admin/staff/:id"' "$REPO_ROOT/backend/main.go"
    grep -q "staffApi.getUser(id)" "$REPO_ROOT/frontend-pc/src/pages/StaffEdit.jsx"
    grep -q "AutoComplete" "$REPO_ROOT/frontend-pc/src/pages/StaffEdit.jsx"
    grep -q 'navigate(`/staff/${record.id}/edit`' "$REPO_ROOT/frontend-pc/src/pages/StaffManagement.jsx"
    ! grep -q "title=\"编辑员工\"" "$REPO_ROOT/frontend-pc/src/pages/StaffManagement.jsx"
    echo "PROBE-OK: site-change idempotent + edit page loads by id + direct-staff field rules"
    ;;
esac
