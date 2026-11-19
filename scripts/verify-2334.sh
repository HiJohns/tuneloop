#!/usr/bin/env bash
# #2334 Rev.3：S5 文档镜像 + 本单可达成 gate（R3 不含 handlers 全量）。
# 用法：bash scripts/verify-2334.sh <r1|r2|r3|r4>
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1) ART="docs/evidence/2334/R1-docs.txt" ;;
  r2) ART="docs/evidence/2334/R2-en.txt" ;;
  r3) ART="docs/evidence/2334/R3-gate.txt" ;;
  r4) ART="docs/evidence/2334/R4-stale.txt" ;;
  *) echo "usage: $0 <r1|r2|r3|r4>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:${PATH}"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_verify2334}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
mkdir -p "$TMPDIR" "$GOCACHE" "$(dirname "$ART")"

BUILD="1.0.$(git rev-parse --short HEAD)"
COMMIT="$(git rev-parse HEAD)"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2334/' || true)" ]; then
  DIRTY=true
fi

{
  echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"
  case "$MODE" in
    r1)
      rc=0
      while IFS='|' read -r file key; do
        [ -n "$file" ] || continue
        if grep -Fq -- "$key" "$file"; then
          echo "OK $file :: $key"
        else
          echo "MISS $file :: $key"
          rc=1
        fi
      done < docs/evidence/2334/keys.txt
      exit "$rc"
      ;;
    r2)
      rc=0
      for f in docs/en/api.md docs/en/ui.md docs/en/cases.md; do
        if [ -f "$f" ]; then echo "OK-EN $f"; else echo "MISS-EN $f"; rc=1; fi
      done
      check() { if grep -Eiq -- "$2" "$1"; then echo "OK $1 $3"; else echo "MISS $1 $3"; rc=1; fi; }
      check docs/en/api.md 'credit-assessment' 'credit-assessment'
      check docs/en/api.md 'PAYSCORE_ENABLED' 'config'
      check docs/en/ui.md 'credit assessment' 'entry'
      check docs/en/cases.md 'openBusinessView' 'openBusinessView'
      check docs/en/cases.md 'H5' 'h5'
      exit "$rc"
      ;;
    r3)
      cd "$REPO_ROOT/backend"
      go build ./... || exit 1
      go test ./database/ -count=1 || exit 1
      cd "$REPO_ROOT/frontend-mobile"
      npm run smoke || exit 1
      ;;
    r4)
      rc=0
      for pat in 'deposit_waived=true 即免押' 'PAYSCORE_ENABLED=true 默认' 'pending 可支付' '零押金失败回退全额'; do
        hits=$(grep -rn -- "$pat" docs/spec/features.md docs/spec/api/api.md docs/spec/ui/ui.md docs/topics/wechat/wechat-pay-integration.md docs/cases/h5-weapp-differences.md docs/topics/wechat/wechat-payscore-deposit-waiver.md 2>/dev/null || true)
        if [ -n "$hits" ]; then echo "STALE: $pat"; printf '%s\n' "$hits"; rc=1; else echo "OK no-stale: $pat"; fi
      done
      exit "$rc"
      ;;
  esac
} 2>&1 | tee "$ART"

exit "${PIPESTATUS[0]}"
