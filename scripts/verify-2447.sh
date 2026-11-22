#!/usr/bin/env bash
# #2447: gate site-management hint by permission.
set -uo pipefail
MODE="${1:-}"
case "$MODE" in r1) NAME="R1-manual.md";; r2) NAME="R2-build.txt";; r3) NAME="R3-static.txt";; *) echo "usage: $0 <r1|r2|r3>" >&2; exit 2;; esac
REPO="$(git rev-parse --show-toplevel)"; cd "$REPO"
OUT="${OUT_DIR:-$REPO/docs/evidence/2447}"; mkdir -p "$OUT"
ART="$OUT/$NAME"
FIX_REF="${FIX_REF:-HEAD}"
FIX_SHA="$(git rev-parse "$FIX_REF")"; FIX_SHORT="$(git rev-parse --short "$FIX_REF")"
BUILD="1.0.$FIX_SHORT"; COMMIT="$FIX_SHA"
DIRTY=false; [ -n "$(git status --porcelain --untracked-files=no)" ] && DIRTY=true
case "$MODE" in
 r1)
   # An Owner-signed receipt wins: keep it untouched and pass.
   if [ -f "$ART" ] && grep -q 'OWNER_PASS' "$ART"; then cat "$ART"; exit 0; fi
   VERSION="$(curl -fsS "${API_CONFIG_URL:-https://preweb.cadenzayueqi.com/api/config}" 2>/dev/null | sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"
   {
     echo "# executor=user"
     echo "# version=${VERSION:-unknown}"
     echo "# issue=2447"
     echo "# accounts: site_admin (expect hidden), merchant_admin (expect visible)"
     echo "OWNER_PENDING"
     echo "回执待补：site_admin 无提示/无「前往网点管理」；merchant_admin 可见；填写结论后改为 OWNER_PASS。"
   } | tee "$ART"
   exit 1
   ;;
 r2)
   { echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"; cd "$REPO/frontend-pc"; npx eslint src/pages/StaffManagement.jsx && npm run build && echo "BUILD_OK" && echo "ESLINT_OK"; } 2>&1 | tee "$ART"; exit "${PIPESTATUS[0]}";;
 r3)
   {
     echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"
     fail=0
     grep -Fq 'checkPermission(' frontend-pc/src/pages/StaffManagement.jsx || { echo "MISS checkPermission"; fail=1; }
     grep -Fq "{canManageSites && (" frontend-pc/src/pages/StaffManagement.jsx || { echo "MISS canManageSites guard"; fail=1; }
     grep -Fq '前往网点管理' frontend-pc/src/pages/StaffManagement.jsx || { echo "MISS alert content"; fail=1; }
     if grep -Fq '前往网点管理' frontend-pc/src/pages/StaffManagement.jsx && \
        ! awk '/前往网点管理/{found=NR} /canManageSites && \(/{guard=NR} END{exit !(found>0 && guard>0 && guard<=found)}' frontend-pc/src/pages/StaffManagement.jsx; then
       echo "MISS guard precedes alert"; fail=1
     fi
     grep -Fq "sysPermBits: [SysPermBits.organization_view]" frontend-pc/src/pages/StaffManagement.jsx || { echo "MISS organization_view bit"; fail=1; }
     grep -Fq "cusPermCodes: ['instrument:create', 'instrument:read']" frontend-pc/src/pages/StaffManagement.jsx || { echo "MISS instrument perms"; fail=1; }
     [ "$fail" = "0" ] && echo "STATIC_OK" || echo "STATIC_FAIL"
     exit "$fail"
   } 2>&1 | tee "$ART"; exit "${PIPESTATUS[0]}";;
esac
