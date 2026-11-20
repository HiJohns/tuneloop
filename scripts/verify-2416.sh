#!/usr/bin/env bash
# #2416 S2：直连 showModal 清零 / H5 alert 回退 / smoke / 真机回执。
# 用法：bash scripts/verify-2416.sh <r1|r2|r3|r4>
set -uo pipefail
MODE="${1:-}"
case "$MODE" in
  r1) ART="docs/evidence/2416/R1-probe.txt" ;;
  r2) ART="docs/evidence/2416/R2-h5-probe.txt" ;;
  r3) ART="docs/evidence/2416/R3-smoke.txt" ;;
  r4) ART="" ;;
  *) echo "usage: $0 <r1|r2|r3|r4>" >&2; exit 2 ;;
esac
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
mkdir -p docs/evidence/2416
BUILD="1.0.$(git rev-parse --short HEAD)"; COMMIT="$(git rev-parse HEAD)"; DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2416/' || true)" ]; then DIRTY=true; fi

if [ "$MODE" = "r4" ]; then
  f="docs/evidence/2416/R4-manual.md"
  if grep -q 'executor: user' "$f" && tail -5 "$f" | grep -q '^RESULT: PASS'; then
    echo "MANUAL_OK: $f"; exit 0
  fi
  echo "MANUAL_PENDING: $f（Owner 真机回执未签）"; exit 1
fi

{
  echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"
  case "$MODE" in
    r1)
      hits=$(grep -rn "Taro\.showModal(" frontend-mobile/src || true)
      count=$(printf '%s\n' "$hits" | grep -c "Taro\.showModal(" || true)
      echo "count=$count"
      printf '%s\n' "$hits"
      if [ "$count" -le 1 ] && printf '%s\n' "$hits" | grep -q "frontend-mobile/src/platform/"; then
        echo "PROBE PASS: only platform fallback remains"
      else
        echo "PROBE FAIL: business call sites remain or count > 1"; exit 1
      fi
      ;;
    r2)
      alert=$(grep -rn "window\.alert(" frontend-mobile/src || true)
      confirm=$(grep -rn "window\.confirm(" frontend-mobile/src || true)
      ac=$(printf '%s\n' "$alert" | grep -c "window\.alert(" || true)
      cc=$(printf '%s\n' "$confirm" | grep -c "window\.confirm(" || true)
      echo "window.alert count=$ac"; printf '%s\n' "$alert"
      echo "window.confirm count=$cc"; printf '%s\n' "$confirm"
      if [ "$ac" -le 1 ] && [ "$cc" -le 1 ]; then echo "H5 PROBE PASS"; else echo "H5 PROBE FAIL"; exit 1; fi
      ;;
    r3)
      cd "$REPO_ROOT/frontend-mobile"
      npm run smoke
      ;;
  esac
} 2>&1 | tee "$ART"
exit "${PIPESTATUS[0]}"
