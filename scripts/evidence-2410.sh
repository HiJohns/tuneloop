#!/usr/bin/env bash
# #2410：PC 扫码绑定接入 #2228（ConfirmBind → IAM auth/wx-bind）证据生成器。
# 用法：bash scripts/evidence-2410.sh <r1-wxbind|r2-needs-merge|r3-update-guard|r4-bind-ui|r5-doc|r6-go-build|r7-weapp-build>
#
# 按 Accepted Plan D6/Evidence Environment：在 tmp/wt-2410（detach 到 HEAD）的 clean
# worktree 中执行；node_modules 以 symlink 复用；结束时删除 worktree。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1-wxbind|r2-needs-merge|r3-update-guard|r4-bind-ui|r5-doc|r6-go-build|r7-weapp-build) ;;
  *) echo "usage: $0 <r1-wxbind|r2-needs-merge|r3-update-guard|r4-bind-ui|r5-doc|r6-go-build|r7-weapp-build>" >&2; exit 2 ;;
esac

ROOT="$(git rev-parse --show-toplevel)"
WT="$ROOT/tmp/wt-2410"
HEAD_FULL="$(git rev-parse HEAD)"
HEAD_SHORT="$(git rev-parse --short HEAD)"

cleanup() {
  git -C "$ROOT" worktree remove --force "$WT" >/dev/null 2>&1 || true
  git -C "$ROOT" worktree prune >/dev/null 2>&1 || true
}
trap cleanup EXIT

git -C "$ROOT" worktree remove --force "$WT" >/dev/null 2>&1 || true
git -C "$ROOT" worktree add --detach "$WT" "$HEAD_FULL" >/dev/null 2>&1 || {
  echo "FATAL: cannot create evidence worktree at $HEAD_FULL" >&2
  exit 1
}

if [ -e "$ROOT/frontend-mobile/node_modules" ] && [ ! -e "$WT/frontend-mobile/node_modules" ]; then
  ln -s "$ROOT/frontend-mobile/node_modules" "$WT/frontend-mobile/node_modules"
fi
if [ -e "$ROOT/frontend-mobile/.env.local" ] && [ ! -e "$WT/frontend-mobile/.env.local" ]; then
  ln -s "$ROOT/frontend-mobile/.env.local" "$WT/frontend-mobile/.env.local"
fi

export PATH="/usr/local/go/bin:${PATH}"
export TMPDIR="${TMPDIR:-$ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$ROOT/tmp/gocache}"
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_verify2410}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
mkdir -p "$TMPDIR" "$GOCACHE"

DIRTY=false
if [ -n "$(git -C "$WT" status --porcelain --untracked-files=no)" ]; then
  DIRTY=true
fi
echo "# build=1.0.${HEAD_SHORT} commit=${HEAD_FULL} dirty=${DIRTY} PROVENANCE=pc-qr-wxbind-2410-${HEAD_SHORT}"

run_go_test() { # package, test regex
  ( cd "$WT/backend" && go test "./$1/" -run "$2" -count=1 -v )
}

case "$MODE" in
  r1-wxbind)
    run_go_test handlers '^TestConfirmBind_Success$'
    ;;
  r2-needs-merge)
    run_go_test handlers '^TestConfirmBind_NeedsMerge$'
    ;;
  r3-update-guard)
    run_go_test services '^TestIAMClient_RejectsDirectWxOpenid$'
    ;;
  r4-bind-ui)
    ( cd "$WT/frontend-mobile" && npx vitest run --config vitest.config.js scripts/page-smoke/bind-needs-merge.test.jsx )
    ;;
  r5-doc)
    rc=0
    for f in docs/spec/api/api.md docs/spec/ui/ui.md docs/cases/cases.md; do
      if ! grep -q "needs_merge" "$WT/$f" || ! grep -q "exchange_token" "$WT/$f"; then
        echo "DOC_MISSING: $f (needs_merge/exchange_token)" >&2
        rc=1
      else
        echo "DOC_OK: $f"
      fi
    done
    exit "$rc"
    ;;
  r6-go-build)
    ( cd "$WT/backend" && go build ./... )
    ;;
  r7-weapp-build)
    (
      cd "$WT/frontend-mobile" || exit 1
      . "$HOME/.nvm/nvm.sh" >/dev/null 2>&1
      nvm use 22 >/dev/null 2>&1
      npm run build:weapp
    ) || exit 1
    HITS="$(grep -rlE '<(div|span|img|button|input|textarea|p|select|a)\b' "$WT/frontend-mobile/dist-weapp" --include='*.wxml' 2>/dev/null | grep -v '/base.wxml$' || true)"
    if [ -n "$HITS" ]; then
      echo "WXML_PROBE FAIL:" >&2
      printf '%s\n' "$HITS" >&2
      exit 1
    fi
    echo "WXML_PROBE PASS"
    ;;
esac
