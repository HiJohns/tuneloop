#!/usr/bin/env bash
# #2437：页面级 DialogHost + 队列归属 + 超时降级 + 开关；真机 R6 Owner。
set -uo pipefail
MODE="${1:-}"
case "$MODE" in
  r1) ART="docs/evidence/2437/R1-host.txt" ;;
  r2) ART="docs/evidence/2437/R2-queue.txt" ;;
  r3) ART="docs/evidence/2437/R3-timeout.txt" ;;
  r4) ART="docs/evidence/2437/R4-switch.txt" ;;
  r5) ART="docs/evidence/2437/R5-smoke.txt" ;;
  r6) ART="docs/evidence/2437/R6-manual.md" ;;
  *) echo "usage: $0 <r1|r2|r3|r4|r5|r6>" >&2; exit 2 ;;
esac
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
BUILD="1.0.$(git rev-parse --short HEAD)"; COMMIT="$(git rev-parse HEAD)"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2437/' || true)" ]; then DIRTY=true; fi
PROVENANCE="# build=$BUILD commit=$COMMIT dirty=$DIRTY"
mkdir -p "$(dirname "$ART")"

case "$MODE" in
  r1)
    {
      echo "$PROVENANCE"
      fail=0
      missing=$(grep -L "components/PageShell" frontend-mobile/src/pages-weapp/*/index.tsx || true)
      if [ -n "$missing" ]; then echo "MISSING PageShell: $missing"; fail=1; else echo "SHELLS_OK: all pages-weapp index.tsx use PageShell"; fi
      grep -q "DialogHost" frontend-mobile/src/components/PageShell.jsx && echo "PAGESHELL_HOST_OK" || { echo "PAGESHELL_HOST_MISSING"; fail=1; }
      test -f frontend-mobile/dist-weapp/app.js && echo "WEAPP_BUILD_OK" || { echo "WEAPP_BUILD_MISSING"; fail=1; }
      exit "$fail"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r2)
    { echo "$PROVENANCE"; ( cd frontend-mobile && npx vitest run --config vitest.config.js scripts/page-smoke/dialog-host-2437.test.jsx -t 'pickHostRoute' ); } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r3)
    { echo "$PROVENANCE"; ( cd frontend-mobile && npx vitest run --config vitest.config.js scripts/page-smoke/dialog-host-2437.test.jsx -t '宿主未消费' ); } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r4)
    { echo "$PROVENANCE"; ( cd frontend-mobile && npx vitest run --config vitest.config.js scripts/page-smoke/dialog-host-2437.test.jsx -t 'weapp 开关' ); } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r5)
    { echo "$PROVENANCE"; ( cd frontend-mobile && npm run smoke ) && test -f frontend-mobile/dist-weapp/app.js && echo "WEAPP_BUILD_OK"; } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r6)
    if tail -5 docs/evidence/2437/R6-manual.md 2>/dev/null | grep -q '^RESULT: PASS'; then echo "MANUAL_OK"; exit 0; fi
    echo "MANUAL_PENDING: docs/evidence/2437/R6-manual.md"; exit 1
    ;;
esac
