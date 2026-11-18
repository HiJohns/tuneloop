#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT"

if [ -n "$(git status --porcelain -- . ':(exclude)docs/evidence/2409' ':(exclude)tmp')" ]; then
  echo "DIRTY_TREE: evidence must be generated from a clean fix worktree" >&2
  exit 1
fi

HEAD_SHA="$(git rev-parse HEAD)"
HEAD_SHORT="$(git rev-parse --short=7 HEAD)"
echo "PROVENANCE build=bind-openid-2409-${HEAD_SHORT} commit=${HEAD_SHA} dirty=false"

cmd="${1:-}"
case "$cmd" in
  r1-success)
    cd "$ROOT/frontend-mobile"
    npm run smoke -- scripts/page-smoke/bind-wx-accounts.test.jsx -t "uses wx-accounts openid"
    ;;
  r2-failure)
    cd "$ROOT/frontend-mobile"
    npm run smoke -- scripts/page-smoke/bind-wx-accounts.test.jsx -t "shows failure"
    ;;
  r3-doc)
    for f in docs/cases/cases.md docs/spec/api/api.md; do
      grep -q 'wx-accounts' "$ROOT/$f"
      grep -q 'openid' "$ROOT/$f"
      echo "$f: wx-accounts=ok openid=ok"
    done
    ;;
  r4-lint)
    cd "$ROOT/frontend-mobile"
    ./node_modules/.bin/eslint src/pages-weapp/bind/Bind.jsx
    ;;
  r5-build)
    cd "$ROOT/frontend-mobile"
    npm run build:weapp
    probe="$(grep -rlE '<(div|span|img|button|input|textarea|p|select|a)\b' dist-weapp --include='*.wxml' | grep -v '/base.wxml$' || true)"
    if [ -n "$probe" ]; then
      echo "WXML_PROBE: FAIL"
      printf '%s\n' "$probe"
      exit 1
    fi
    echo "WXML_PROBE: PASS"
    ;;
  *)
    echo "usage: $0 r1-success|r2-failure|r3-doc|r4-lint|r5-build" >&2
    exit 2
    ;;
esac
