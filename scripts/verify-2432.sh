#!/usr/bin/env bash
# #2432：地址管理按钮内联尺寸 + 同类清单 + 真机 R5。
set -uo pipefail
MODE="${1:-}"
case "$MODE" in
  r1) ART="docs/evidence/2432/R1-address.txt" ;;
  r2) ART="docs/evidence/2432/R2-styles.txt" ;;
  r3) ART="docs/evidence/2432/R3-smoke.txt" ;;
  r4) ART="docs/evidence/2432/R4-inventory.md" ;;
  r5) ART="docs/evidence/2432/R5-manual.md" ;;
  *) echo "usage: $0 <r1|r2|r3|r4|r5>" >&2; exit 2 ;;
esac
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
BUILD="1.0.$(git rev-parse --short HEAD)"; COMMIT="$(git rev-parse HEAD)"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2432/' || true)" ]; then DIRTY=true; fi
PROVENANCE="# build=$BUILD commit=$COMMIT dirty=$DIRTY"
mkdir -p "$(dirname "$ART")"

case "$MODE" in
  r1)
    {
      echo "$PROVENANCE"
      fail=0
      grep -q "secondaryButtonStyle()" frontend-mobile/src/components/AddressManager.jsx && echo "SECONDARY_OK" || { echo "SECONDARY_MISSING"; fail=1; }
      grep -q "secondaryButtonStyle('danger')" frontend-mobile/src/components/AddressManager.jsx && echo "DANGER_OK" || { echo "DANGER_MISSING"; fail=1; }
      grep -q "primaryButtonStyle()" frontend-mobile/src/components/AddressManager.jsx && echo "PRIMARY_OK" || { echo "PRIMARY_MISSING"; fail=1; }
      if grep -q "py-1.5 bg-zinc-100" frontend-mobile/src/components/AddressManager.jsx; then echo "OLD_CLASS_PRESENT"; fail=1; else echo "OLD_CLASS_GONE"; fi
      exit "$fail"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r2)
    { echo "$PROVENANCE"; ( cd frontend-mobile && node --test src/utils/buttonStyles.test.js ); } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r3)
    { echo "$PROVENANCE"; ( cd frontend-mobile && npm run smoke ) && test -f frontend-mobile/dist-weapp/app.js && echo "WEAPP_BUILD_OK"; } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r4)
    {
      echo "$PROVENANCE"
      echo "# 未内联高度的 Taro Button 清单（className 含 py- 且同行无 style={{）"
      grep -rn "<Button" frontend-mobile/src --include='*.jsx' | grep "py-" | grep -v "style={{" || true
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r5)
    if tail -5 docs/evidence/2432/R5-manual.md 2>/dev/null | grep -q '^RESULT: PASS'; then echo "MANUAL_OK"; exit 0; fi
    echo "MANUAL_PENDING: docs/evidence/2432/R5-manual.md"; exit 1
    ;;
esac
