#!/usr/bin/env bash
# #2361（#2268）：SettlementExecutor 验收（并发/幂等/部分成功/冲正/净额/迁移）。
# 用法：bash scripts/verify-2361.sh <r1-concurrency|r2-idempotency|r3-partial|r4-reversal|r5-netting|r6-schema>
# 每个模式把带 provenance 头的输出写入 docs/evidence/2361/ 对应产物。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1-concurrency) ART="docs/evidence/2361/R1-concurrency.txt"; TEST='^TestSettlementExecutorConcurrency_2361$' ;;
  r2-idempotency) ART="docs/evidence/2361/R2-idempotency.txt"; TEST='^TestSettlementExecutorIdempotency_2361$' ;;
  r3-partial)     ART="docs/evidence/2361/R3-partial.txt"; TEST='^TestSettlementExecutorPartial_2361$' ;;
  r4-reversal)    ART="docs/evidence/2361/R4-reversal.txt"; TEST='^TestSettlementExecutorReversal_2361$' ;;
  r5-netting)     ART="docs/evidence/2361/R5-netting.txt"; TEST='^TestSettlementExecutorNetting_2361$' ;;
  r6-schema)      ART="docs/evidence/2361/R6-schema.txt" ;;
  *) echo "usage: $0 <r1-concurrency|r2-idempotency|r3-partial|r4-reversal|r5-netting|r6-schema>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
OUT_DIR="${OUT_DIR:-$REPO_ROOT/docs/evidence/2361}"
mkdir -p "$OUT_DIR"
ART="$OUT_DIR/$(basename "$ART")"
export PATH="/usr/local/go/bin:${PATH}"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
if [ -f "$REPO_ROOT/backend/.env" ]; then
  set -a; . "$REPO_ROOT/backend/.env"; set +a
fi
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_verify2361}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TUNELOOP_DB_SIMPLE_PROTOCOL=1
mkdir -p "$TMPDIR" "$GOCACHE" "$(dirname "$ART")"

# 审计 2026-11-20：dirty 必须含 untracked，且不得整体排除证据目录；
# 证据生成前树必须干净（OUT_DIR 指到树外），FIX_COMMIT 显式指向实现提交。
EVAL_AT="$(git rev-parse HEAD)"
COMMIT="$(git rev-parse "${FIX_COMMIT:-HEAD}")" || { echo "FATAL: bad FIX_COMMIT" >&2; exit 3; }
git merge-base --is-ancestor "$COMMIT" "$EVAL_AT" || { echo "FATAL: FIX_COMMIT not ancestor of HEAD" >&2; exit 3; }
BUILD="1.0.$(git rev-parse --short "$COMMIT")"
DIRTY=false
if [ -n "$(git status --porcelain -uall)" ]; then
  DIRTY=true
fi
PROVENANCE="# build=$BUILD commit=$COMMIT dirty=$DIRTY tree=$EVAL_AT"

if [ "$MODE" = "r6-schema" ]; then
  UP="backend/database/migrations/20261117001_settlement_supersedes_2361.up.sql"
  DOWN="backend/database/migrations/20261117001_settlement_supersedes_2361.down.sql"
  {
    echo "$PROVENANCE"
    fail=0
    test -f "$UP" && grep -q "ADD COLUMN IF NOT EXISTS supersedes" "$UP" \
      && grep -q "idx_settlement_supersedes" "$UP" \
      && echo "MIGRATION_UP_OK: $UP" || { echo "MIGRATION_UP_MISSING"; fail=1; }
    test -f "$DOWN" && grep -q "DROP COLUMN IF EXISTS supersedes" "$DOWN" \
      && echo "MIGRATION_DOWN_OK: $DOWN" || { echo "MIGRATION_DOWN_MISSING"; fail=1; }
    grep -q "Supersedes \*string" backend/models/models.go \
      && echo "MODEL_OK: SettlementCalculation.Supersedes" || { echo "MODEL_MISSING"; fail=1; }
    grep -q "20261117001" backend/database/db.go \
      && grep -q '"supersedes"' backend/database/db.go \
      && echo "ARTIFACT_OK: defaultMigrationArtifacts 20261117001 supersedes" || { echo "ARTIFACT_MISSING"; fail=1; }
    if sed -n "/idx_settlement_active_order/,/;/p" backend/database/migrations/20261116001_settlement_ledger_2360.up.sql | grep -q "superseded"; then
      echo "ACTIVE_INDEX_INVALID: superseded must not be in active unique index"; fail=1
    else
      echo "ACTIVE_INDEX_OK: superseded 不参与 active 唯一索引"
    fi
    ( cd "$REPO_ROOT/backend" && go test ./services/ -run '^TestSettlementSupersedesMigration_2361$' -count=1 -v )
    rc=$?
    ( cd "$REPO_ROOT/backend" && go build ./... )
    build_rc=$?
    if [ "$rc" != "0" ] || [ "$build_rc" != "0" ]; then fail=1; fi
    [ "$fail" = "0" ] && echo "SCHEMA_OK: 迁移 + 模型 + 工件登记三处齐全，up/down 可执行"
    exit "$fail"
  } 2>&1 | tee "$ART"
  exit "${PIPESTATUS[0]}"
fi

{
  echo "$PROVENANCE"
  ( cd "$REPO_ROOT/backend" && go test ./services/ -run "$TEST" -count=1 -v )
} 2>&1 | tee "$ART"
exit "${PIPESTATUS[0]}"
