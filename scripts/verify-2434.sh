#!/usr/bin/env bash
# #2434：扫码直达页返回必须有兜底（safeBack），不得裸用 Taro.navigateBack()。
set -euo pipefail
MODE="${1:-}"
case "$MODE" in r1-unit|r2-probe|r3-smoke) ;; *) echo "usage: $0 {r1-unit|r2-probe|r3-smoke}" >&2; exit 2 ;; esac
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
echo "# build=1.0.$(git rev-parse --short HEAD) commit=$(git rev-parse HEAD) dirty=$([ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2434/' || true)" ] && echo true || echo false)"
cd frontend-mobile
case "$MODE" in
  r1-unit) npx vitest run --config vitest.config.js scripts/page-smoke/back-nav-2434.test.jsx ;;
  r2-probe)
    # 负例：扫码/分享直达页仍裸用 Taro.navigateBack() → FAIL
    hits=$(grep -rn "Taro.navigateBack()" src/pages-weapp/bind src/pages-weapp/login src/pages-weapp/profile-complete src/pages-weapp/account-select 2>/dev/null || true)
    if [ -n "$hits" ]; then echo "FAIL: bare navigateBack in scan-prone pages" >&2; echo "$hits" >&2; exit 1; fi
    grep -q "safeBack" src/platform/index.weapp.js
    grep -q "safeBack" src/platform/browser.js
    echo "PROBE-OK: scan-prone pages use navigation.safeBack()"
    ;;
  r3-smoke) npm run smoke ;;
esac
