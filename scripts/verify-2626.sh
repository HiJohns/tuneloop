#!/usr/bin/env bash
# #2626 证据生成器 —— 用户角色权威与术语收敛（IAM 权威 + 一人多角色 + IAM 术语）。
#
# 用法：bash scripts/verify-2626.sh <r1|r2|r3|r4|r5|r6>
#   r1  docs/evidence/2626/R1-derive.txt          DeriveRoles 表驱动（4 组 + 边界）
#   r2  docs/evidence/2626/R2-write-sites.txt     三处写侧收口（静态）
#   r3  docs/evidence/2626/R3-mirror-column.txt   迁移文本 + 工件登记 + 模型字段（静态）
#   r4  docs/evidence/2626/R4-pc-derive.txt       前端集合判据（node --test）
#   r5  docs/evidence/2626/R5-terms.txt           文档/注释术语扫描
#   r6  docs/evidence/2626/R6-recon.txt           不引入单值判据（收敛）
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<FIX_REF 40 位> # dirty=<bool>`，
# 次行 `# tree=<HEAD sha>`。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4|r5|r6) ;;
  *)
    echo "usage: $0 <r1|r2|r3|r4|r5|r6>" >&2
    exit 2
    ;;
esac

REPO="$(git rev-parse --show-toplevel)" || exit 2
cd "$REPO" || exit 2
mkdir -p "$REPO/tmp"
ISSUE=2626
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
export PATH="/usr/local/go/bin:$PATH"
HEAD_SHA="$(git rev-parse HEAD)"
if [ -z "${FIX_REF:-}" ]; then
  FIX_REF="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
fi
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor of HEAD" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
if [ -z "${PRE_REF:-}" ]; then
  PRE_REF="$(git rev-parse "$(git log --format=%H --grep="^fix(issue-$ISSUE)" | tail -1)^" 2>/dev/null || true)"
fi
[ -n "$PRE_REF" ] || { echo "FATAL: cannot resolve PRE_REF" >&2; exit 3; }
DIRTY=false
[ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR" ':(exclude)tmp')" ] && DIRTY=true
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
TREE_LINE="# tree=$HEAD_SHA"

HANDLERS=("backend/handlers/iam_callback.go" "backend/handlers/merchant_member.go" "backend/handlers/user_staff.go")
MIG_UP="backend/database/migrations/20261128001_users_roles_mirror_2626.up.sql"
MIG_DOWN="backend/database/migrations/20261128001_users_roles_mirror_2626.down.sql"
HEAVY_RUN="${OPENCODE_HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"

LOG="$REPO/tmp/verify-2626-${MODE}.log"
: > "$LOG"
RC=0
NAME=""

emit() { printf '%s\n' "$*" >> "$LOG"; }
require() { grep -Fq -- "$1" "$LOG" || { echo "ASSERT_FAIL: missing [$1]" >&2; RC=1; }; }
provenance() { printf '%s\n%s\n' "$PROV" "$TREE_LINE" > "$LOG"; }

case "$MODE" in
  r1)
    NAME="R1-derive.txt"
    provenance
    R1_LOG="$REPO/tmp/verify-2626-r1-gotest.log"
    ( cd "$REPO/backend" && go test ./services -run 'TestDeriveRoles2626' -v -count=1 ) > "$R1_LOG" 2>&1
    GO_RC=$?
    emit "GO_TEST_CMD=cd backend && go test ./services -run TestDeriveRoles2626 -v"
    emit "GO_TEST_EXIT=$GO_RC"
    grep -E '^DERIVE_' "$R1_LOG" | sed 's/^/GOTEST: /' >> "$LOG"
    grep -E '^(=== RUN|--- (PASS|FAIL))' "$R1_LOG" | sed 's/^/GOTEST: /' >> "$LOG"
    DERIVE_SITE_ADMIN="$(sed -n 's/^DERIVE_PRIMARY_SITE_ADMIN=//p' "$R1_LOG" | head -1)"
    DERIVE_MULTI="$(sed -n 's/^DERIVE_PRIMARY_MULTI=//p' "$R1_LOG" | head -1)"
    DERIVE_ALL_LEN="$(sed -n 's/^DERIVE_ALL_LEN=//p' "$R1_LOG" | head -1)"
    DERIVE_PLATFORM="$(sed -n 's/^DERIVE_NAMESPACE_ADMIN_KEPT=//p' "$R1_LOG" | head -1)"
    emit "DERIVE_PRIMARY_SITE_ADMIN=${DERIVE_SITE_ADMIN:-missing}"
    emit "DERIVE_PRIMARY_MULTI=${DERIVE_MULTI:-missing}"
    emit "DERIVE_ALL_LEN=${DERIVE_ALL_LEN:-missing}"
    emit "DERIVE_NAMESPACE_ADMIN_KEPT=${DERIVE_PLATFORM:-missing}"
    emit "EXIT=$GO_RC"
    [ "$GO_RC" = "0" ] || { echo "ASSERT_FAIL: go test failed" >&2; RC=1; }
    require 'DERIVE_PRIMARY_SITE_ADMIN=site_admin'
    require 'DERIVE_PRIMARY_MULTI=merchant_staff'
    require 'DERIVE_ALL_LEN=2'
    require 'DERIVE_NAMESPACE_ADMIN_KEPT=1'
    require 'EXIT=0'
    ;;

  r2)
    NAME="R2-write-sites.txt"
    provenance
    sites=0
    inline=0
    delegated=0
    for f in "${HANDLERS[@]}"; do
      [ -f "$f" ] || { emit "FATAL: missing $f"; RC=1; continue; }
      derive_calls="$(grep -c 'services\.DeriveRoles(' "$f" || true)"
      mirror_writes="$(grep -cE '(mirror\.Primary|derived\.Primary)' "$f" || true)"
      mirror_all="$(grep -cE '(mirror\.All|derived\.All)' "$f" || true)"
      literals="$(grep -cE '(Role:|\bRole\s*=\s*)["'"'"'](site_member|site_admin|merchant_staff|merchant_admin|repair_technician)["'"'"']' "$f" || true)"
      emit "FILE=$f derive_calls=$derive_calls mirror_primary=$mirror_writes mirror_all=$mirror_all inline_literals=$literals"
      [ "${derive_calls:-0}" -gt 0 ] && sites=$((sites + 1))
      if [ "${derive_calls:-0}" -gt 0 ] && [ "${mirror_writes:-0}" -gt 0 ] && [ "${mirror_all:-0}" -gt 0 ]; then
        delegated=$((delegated + 1))
      fi
      inline=$((inline + ${literals:-0}))
    done
    emit "WRITE_SITES=$sites"
    emit "WRITE_INLINE_LITERAL=$inline"
    if [ "$delegated" = "3" ]; then emit "MIRROR_WRITE_DELEGATED=1"; else emit "MIRROR_WRITE_DELEGATED=0"; fi
    require 'WRITE_SITES=3'
    require 'WRITE_INLINE_LITERAL=0'
    require 'MIRROR_WRITE_DELEGATED=1'
    ;;

  r3)
    NAME="R3-mirror-column.txt"
    provenance
    COLUMN=0; DOWN=0; NOGUESS=0; ARTIFACT=0; MODEL=0
    [ -f "$MIG_UP" ] && grep -qiE 'ADD COLUMN IF NOT EXISTS roles JSONB NOT NULL DEFAULT' "$MIG_UP" && COLUMN=1
    [ -f "$MIG_DOWN" ] && grep -qiE 'DROP COLUMN IF EXISTS roles' "$MIG_DOWN" && DOWN=1
    # 迁移内不得写猜测值：不允许任何 UPDATE/INSERT 角色回填。
    if [ -f "$MIG_UP" ] && ! grep -qiE '^[[:space:]]*(UPDATE|INSERT)[[:space:]]' "$MIG_UP"; then NOGUESS=1; fi
    grep -q '20261128001, "users", "roles"' backend/database/db.go && ARTIFACT=1
    grep -qE 'Roles[[:space:]]+\[\]string' backend/models/models.go && MODEL=1
    emit "MIG_UP=$MIG_UP"
    emit "MIG_DOWN=$MIG_DOWN"
    emit "COLUMN_ROLES_EXISTS=$COLUMN"
    emit "BACKFILL_NO_GUESS=$NOGUESS"
    emit "DOWN_REMOVES_COLUMN=$DOWN"
    emit "ARTIFACT_REGISTERED=$ARTIFACT"
    emit "MODEL_FIELD_EXISTS=$MODEL"
    emit "--- up.sql ---"
    sed 's/^/UP: /' "$MIG_UP" >> "$LOG"
    emit "--- artifact registration ---"
    grep -n '20261128001' backend/database/db.go | sed 's/^/ARTIFACT: /' >> "$LOG"
    require 'COLUMN_ROLES_EXISTS=1'
    require 'BACKFILL_NO_GUESS=1'
    require 'DOWN_REMOVES_COLUMN=1'
    require 'ARTIFACT_REGISTERED=1'
    require 'MODEL_FIELD_EXISTS=1'
    ;;

  r4)
    NAME="R4-pc-derive.txt"
    provenance
    TEST_LOG="$REPO/tmp/verify-2626-r4-node.log"
    ( cd "$REPO" && node --test frontend-pc/tests/staffForm.test.mjs ) > "$TEST_LOG" 2>&1
    NODE_RC=$?
    emit "TEST_CMD=node --test frontend-pc/tests/staffForm.test.mjs"
    emit "NODE_TEST_EXIT=$NODE_RC"
    grep -E '^# (tests|pass|fail)' "$TEST_LOG" | sed 's/^/NODE: /' >> "$LOG"
    USES_SET=0
    if grep -qE 'Array\.isArray\(user\?\.roles\)' frontend-pc/src/utils/staffForm.js && grep -q 'roleSet(' frontend-pc/src/utils/staffForm.js; then USES_SET=1; fi
    READS_SINGLE="$(grep -cE '\buser\??\.role\b' frontend-pc/src/utils/staffForm.js || true)"
    emit "PC_USES_ROLES_SET=$USES_SET"
    emit "PC_READS_USERS_ROLE=${READS_SINGLE:-0}"
    [ "$NODE_RC" = "0" ] || { echo "ASSERT_FAIL: node --test failed" >&2; RC=1; }
    require 'PC_USES_ROLES_SET=1'
    require 'PC_READS_USERS_ROLE=0'
    ;;

  r5)
    NAME="R5-terms.txt"
    provenance
    DOC_ROLE="$(grep -c '域角色' docs/spec/permissions.md docs/spec/api/api.md 2>/dev/null | awk -F: '{s+=$2} END {print s+0}')"
    TERM_OK=1
    for term in '组织角色' '功能角色' '组织级别'; do
      grep -q "$term" docs/spec/permissions.md || TERM_OK=0
    done
    ROLES_FIELD=0
    grep -q 'roles\[\]' docs/spec/api/api.md && ROLES_FIELD=1
    emit "DOC_TERM_DOMAIN_ROLE=${DOC_ROLE:-0}"
    emit "DOC_TERM_SET_OK=$TERM_OK"
    emit "DOC_ROLES_FIELD=$ROLES_FIELD"
    emit "--- three-term definition ---"
    grep -nE '组织角色|功能角色|组织级别' docs/spec/permissions.md | head -8 | sed 's/^/DOC: /' >> "$LOG"
    require 'DOC_TERM_DOMAIN_ROLE=0'
    require 'DOC_TERM_SET_OK=1'
    require 'DOC_ROLES_FIELD=1'
    ;;

  r6)
    NAME="R6-recon.txt"
    provenance
    # 单值判据的定义（收敛口径）：直接用 `.role` / `.Role` 与**业务角色字面量**做等值判断，
    # 而不是读 roles[] 集合。空值检查（`req.Role != ""`）不算判据。
    ROLE_LITERAL_RE='\.(role|Role)\s*(===|!==|==|!=)\s*["'"'"'](merchant_admin|site_admin|merchant_staff|site_member|repair_technician|namespace_admin|sys_admin)["'"'"']'
    SINGLE_FRONTEND="$(grep -cE "$ROLE_LITERAL_RE" frontend-pc/src/utils/staffForm.js || true)"
    SINGLE_BACKEND=0
    for f in "${HANDLERS[@]}"; do
      c="$(grep -cE "$ROLE_LITERAL_RE" "$f" || true)"
      SINGLE_BACKEND=$((SINGLE_BACKEND + ${c:-0}))
      grep -nE "$ROLE_LITERAL_RE" "$f" | sed "s|^|PREEXISTING: $f:|" >> "$LOG"
    done
    # 本单 diff 新增行里不得出现新的单值判据。
    ADDED_SINGLE="$(git diff "$PRE_REF"..HEAD -- "${HANDLERS[@]}" frontend-pc/src/utils/staffForm.js 2>/dev/null \
      | grep -E '^\+' | grep -vE '^\+\+\+' | grep -cE "$ROLE_LITERAL_RE" || true)"
    emit "PRE_REF=$PRE_REF"
    emit "SINGLE_VALUE_FRONTEND=${SINGLE_FRONTEND:-0}"
    emit "PREEXISTING_SINGLE_VALUE_JUDGES=${SINGLE_BACKEND:-0}"
    emit "SINGLE_VALUE_JUDGE_RESIDUAL=${ADDED_SINGLE:-0}"
    emit "PREEXISTING_SCOPE=与镜像写入无关的历史判据（D7 三步走的第二步按批次迁移，本单不扩面）"
    if [ "${SINGLE_FRONTEND:-0}" = "0" ] && [ "${ADDED_SINGLE:-0}" = "0" ]; then emit "RECON_SINGLE_ONLY=0"; else emit "RECON_SINGLE_ONLY=1"; fi
    grep -nE 'roleSet|primaryRole|roles\b' frontend-pc/src/utils/staffForm.js | head -6 | sed 's/^/JUDGE: /' >> "$LOG"
    require 'RECON_SINGLE_ONLY=0'
    require 'SINGLE_VALUE_JUDGE_RESIDUAL=0'
    ;;
esac

if [ "$RC" != "0" ]; then
  echo "RESULT: FAIL ($MODE)" >> "$LOG"
else
  echo "RESULT: PASS ($MODE)" >> "$LOG"
fi
if [ -n "$NAME" ]; then
  cp "$LOG" "$OUT_DIR/$NAME"
  echo "ARTIFACT=$OUT_DIR/$NAME" >&2
fi
tail -1 "$LOG" >&2
[ "$RC" = "0" ] || exit 1
exit 0
