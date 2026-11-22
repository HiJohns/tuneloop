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
  r5|r5-provenance) ART="docs/evidence/2334/R5-provenance.txt" ;;
  *) echo "usage: $0 <r1|r2|r3|r4|r5-provenance>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:${PATH}"
OUT_DIR="${OUT_DIR:-$REPO_ROOT/docs/evidence/2334}"
mkdir -p "$OUT_DIR" "$REPO_ROOT/tmp"
ART="$OUT_DIR/$(basename "$ART")"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"
set -a; . /home/coder/tuneloop/backend/.env; set +a

FIX_REF="${FIX_REF:-b15232591}"
FIX_SHA="$(git rev-parse "$FIX_REF")"; FIX_SHORT="$(git rev-parse --short "$FIX_REF")"
HEAD_SHA="$(git rev-parse --short HEAD)"
BUILD="1.0.$FIX_SHORT"
COMMIT="$FIX_SHA"
DIRTY=false
if [ -n "$(git status --porcelain -uall)" ]; then
  DIRTY=true
fi

{
  echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY tree=$HEAD_SHA"
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
      echo "BUILD_OK"
      go test ./database/ -count=1 || exit 1
      echo "DATABASE_OK"
      cd "$REPO_ROOT/frontend-mobile"
      npm run smoke
      src=$?
      echo "SMOKE_EXIT=$src"
      [ "$src" = "0" ] && echo "SMOKE_OK"
      exit "$src"
      ;;
    r5)
      rc=0
      status="$(git status --porcelain -uall || true)"
      if [ -n "$status" ]; then echo "FAIL: worktree dirty"; printf '%s\n' "$status"; rc=1; else echo "STATUS_CLEAN"; fi
      for f in R1-docs R2-en R3-gate R4-stale; do
        p="$OUT_DIR/$f.txt"
        if [ ! -f "$p" ]; then echo "FAIL: missing $p"; rc=1; continue; fi
        first="$(head -1 "$p")"
        case "$first" in
          "# build=$BUILD commit=$FIX_SHA dirty=false "*) echo "OK $f" ;;
          *) echo "FAIL: provenance mismatch in $f: $first"; rc=1 ;;
        esac
      done
      ev="$OUT_DIR/evidence.json"
      if [ ! -f "$ev" ]; then echo "FAIL: evidence.json missing"; rc=1; else
        ev_build="$(jq -r '.build // empty' "$ev")"; ev_commit="$(jq -r '.commit // empty' "$ev")"
        ev_dirty="$(jq -r 'if has("dirty") then (.dirty|tostring) else "" end' "$ev")"
        [ "$ev_build" = "$BUILD" ] || { echo "FAIL: evidence build=$ev_build"; rc=1; }
        [ "$ev_commit" = "$FIX_SHA" ] || { echo "FAIL: evidence commit=$ev_commit"; rc=1; }
        [ "$ev_dirty" = "false" ] || { echo "FAIL: evidence dirty=$ev_dirty"; rc=1; }
      fi
      [ "$rc" = "0" ] && echo "PROVENANCE_OK"
      exit "$rc"
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
