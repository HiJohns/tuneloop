#!/usr/bin/env bash
# #2431：weapp 端 dialog 必须降级原生（App 层 DialogHost 不可见 → Promise 永不 resolve）。
set -euo pipefail
MODE="${1:-}"
case "$MODE" in
  r1-unit|r2-probe|r3-smoke|r4-build) ;;
  *) echo "usage: $0 {r1-unit|r2-probe|r3-smoke|r4-build}" >&2; exit 2 ;;
esac
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"

BUILD="1.0.$(git rev-parse --short HEAD)"
COMMIT="$(git rev-parse HEAD)"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2431/' || true)" ]; then
  DIRTY=true
fi
echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"

FE=frontend-mobile

case "$MODE" in
  r1-unit)
    cd "$FE"
    npx vitest run --config vitest.config.js scripts/page-smoke/dialog-fallback-2431.test.jsx
    ;;
  r2-probe)
    # 负例：若 weapp platform 仍以 isDialogHostMounted() 单独决定入队 → FAIL
    if grep -nE '^\s*if \(isDialogHostMounted\(\)\)' "$FE/src/platform/index.weapp.js"; then
      echo "FAIL: weapp still gates AppDialog on isDialogHostMounted() alone" >&2
      exit 1
    fi
    grep -q "shouldUseAppDialog" "$FE/src/platform/index.weapp.js"
    grep -q "APP_DIALOG_ON_WEAPP = false" "$FE/src/utils/dialogFallback.mjs"
    grep -q "confirmText" "$FE/src/utils/dialogFallback.mjs"
    echo "PROBE-OK: weapp dialog → native fallback (custom confirm/cancel text preserved)"
    ;;
  r3-smoke)
    cd "$FE"
    npm run smoke
    ;;
  r4-build)
    cd "$FE"
    npm run build:weapp
    test -f dist-weapp/app.js
    echo "BUILD-OK: dist-weapp/app.js present"
    ;;
esac
