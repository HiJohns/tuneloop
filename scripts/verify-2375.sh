#!/usr/bin/env bash
# #2375 Rev.3：角色分配/变更 + 非阻断告警 + 角色列。
# 用法：bash scripts/verify-2375.sh <r1-role-errors|r2-role-update|r3-forbidden|r4-list-role|r5-pending|r6-iam-params|r7-role-column>
set -uo pipefail
MODE="${1:-}"
case "$MODE" in
  r1-role-errors) ART="docs/evidence/2375/R1-role-errors.txt"; TEST='^TestCreateUserRoleErrors_2375$' ;;
  r2-role-update) ART="docs/evidence/2375/R2-role-update.txt"; TEST='^TestUpdateUserRole_2375$' ;;
  r3-forbidden) ART="docs/evidence/2375/R3-forbidden.txt"; TEST='^TestUpdateUserRoleForbidden_2375$' ;;
  r4-list-role) ART="docs/evidence/2375/R4-list-role.txt"; TEST="" ;;
  r5-pending) ART="docs/evidence/2375/R5-update-pending.txt"; TEST='^TestUpdateUserPendingRoleErrors_2375$' ;;
  r6-iam-params) ART="docs/evidence/2375/R6-iam-mapping.txt"; TEST='^TestUpdateUserRoleIAMPayload_2375$' ;;
  r7-role-column) ART="docs/evidence/2375/R7-role-column.md"; TEST="" ;;
  *) echo "usage: $0 <r1-role-errors|r2-role-update|r3-forbidden|r4-list-role|r5-pending|r6-iam-params|r7-role-column>" >&2; exit 2 ;;
esac
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:${PATH}"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
# DB/租户参数统一取 backend/.env（本机实际端口），仅当未显式注入时生效。
if [ -f "$REPO_ROOT/backend/.env" ]; then
  set -a; . "$REPO_ROOT/backend/.env"; set +a
fi
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-25432}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_verify2375}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
mkdir -p "$TMPDIR" "$GOCACHE" "$(dirname "$ART")"

ISSUE=2375
EVIDENCE_DIR="docs/evidence/$ISSUE"
HEAD_SHA="$(git rev-parse HEAD)"
# provenance 锚定本单 fix commit：默认取最早提到本单的提交（实现提交），可用 FIX_REF 覆盖。
FIX_REF="${FIX_REF:-$(git log --format=%H --grep="issue-$ISSUE" --reverse | head -1)}"
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
# dirty 只看源码/文档树：本脚本自身写出的 evidence 产物不计入。
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR" || true)" ]; then DIRTY=true; fi
# pre-ready P14 要求的首行格式（严格匹配，行内不得有其它字段）
BUILD="1.0.$FIX_SHORT"
COMMIT="$FIX_SHA"
PROV="# build=$BUILD # commit=$COMMIT # dirty=$DIRTY"

if [ "$MODE" = "r7-role-column" ]; then
  f="$ART"
  head -n 1 "$f" | grep -qF -- "$PROV" || { echo "PROV_MISSING: $f"; exit 1; }
  if grep -q 'executor: user' "$f" && grep -q 'RESULT: PASS' "$f"; then echo "MANUAL_OK: $f"; exit 0; fi
  echo "MANUAL_PENDING: $f（Owner 角色列回执未签）"; exit 1
fi

{
  echo "$PROV"
  case "$MODE" in
    r1-role-errors|r2-role-update|r3-forbidden|r5-pending|r6-iam-params)
      cd "$REPO_ROOT/backend"
      go test ./handlers/ -run "$TEST" -count=1 -v
      ;;
    r4-list-role)
      cd "$REPO_ROOT/frontend-pc"
      if [ ! -d node_modules ]; then
        ln -s "$(git rev-parse --path-format=absolute --git-common-dir)/../frontend-pc/node_modules" node_modules
      fi
      npm run build
      ;;
  esac
} 2>&1 | tee "$ART"
exit "${PIPESTATUS[0]}"
