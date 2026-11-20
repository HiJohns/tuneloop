#!/usr/bin/env bash
# #2446：P2 合并后设置用户名/密码 — 证据生成。
# 用法：bash scripts/verify-2446.sh <r1-backend|r2-user-me|r3-credentials-smoke|r4-entry|r5-weapp-build|r6-prerelease|r7-owner>
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1-backend) ART_NAME="R1-backend.txt" ;;
  r2-user-me) ART_NAME="R2-user-me.txt" ;;
  r3-credentials-smoke) ART_NAME="R3-credentials-smoke.txt" ;;
  r4-entry) ART_NAME="R4-entry.txt" ;;
  r5-weapp-build) ART_NAME="R5-weapp-build.txt" ;;
  r6-prerelease) ART_NAME="R6-prerelease.txt" ;;
  r7-owner) ART_NAME="R7-owner.md" ;;
  *) echo "usage: $0 <r1-backend|r2-user-me|r3-credentials-smoke|r4-entry|r5-weapp-build|r6-prerelease|r7-owner>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
OUT_DIR="${OUT_DIR:-$REPO_ROOT/docs/evidence/2446}"
mkdir -p "$OUT_DIR"
ART="$OUT_DIR/$ART_NAME"
BUILD="1.0.$(git rev-parse --short HEAD)"
COMMIT="$(git rev-parse HEAD)"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
  DIRTY=true
fi
PROVENANCE="# build=$BUILD commit=$COMMIT dirty=$DIRTY"

run_node22() {
  if [ -s "$HOME/.nvm/nvm.sh" ]; then
    # shellcheck disable=SC1090
    source "$HOME/.nvm/nvm.sh"
    nvm use 22 >/dev/null 2>&1 || true
  fi
}

case "$MODE" in
  r1-backend)
    {
      echo "$PROVENANCE"
      cd "$REPO_ROOT/backend"
      set -a; . ./.env; set +a
      export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2446_evidence}" \
             IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}" \
             IAM_SECRET="${IAM_SECRET:-test-secret-2446}" \
             TUNELOOP_DB_SIMPLE_PROTOCOL=1
      go test ./handlers/ -run 'TestUpdateCurrentUser_2446|TestGetCurrentUser_2446' -count=1 -v
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r2-user-me)
    {
      echo "$PROVENANCE"
      cd "$REPO_ROOT/backend"
      set -a; . ./.env; set +a
      export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2446_evidence}" \
             IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}" \
             IAM_SECRET="${IAM_SECRET:-test-secret-2446}" \
             TUNELOOP_DB_SIMPLE_PROTOCOL=1
      go test ./handlers/ -run 'TestGetCurrentUser_2446' -count=1 -v
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r3-credentials-smoke)
    {
      echo "$PROVENANCE"
      run_node22
      cd "$REPO_ROOT/frontend-mobile"
      npx vitest run --config vitest.config.js scripts/page-smoke/credentials-2446.test.jsx
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r4-entry)
    {
      echo "$PROVENANCE"
      fail=0
      grep -q '设置登录账号' frontend-mobile/src/pages-weapp/bind/Bind.jsx || { echo 'ENTRY_FAIL: Bind CTA missing'; fail=1; }
      grep -q 'needs_credentials' frontend-mobile/src/pages-weapp/Profile.jsx || { echo 'ENTRY_FAIL: Profile gate missing'; fail=1; }
      grep -q 'pages-weapp/credentials/index' frontend-mobile/src/app.config.ts || { echo 'ENTRY_FAIL: page not registered'; fail=1; }
      [ "$fail" = "0" ] && echo 'ENTRY_OK' || echo 'ENTRY_FAIL'
      exit "$fail"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r5-weapp-build)
    {
      echo "$PROVENANCE"
      run_node22
      cd "$REPO_ROOT/frontend-mobile"
      npm run build:weapp
      rc=$?
      [ "$rc" = "0" ] || exit "$rc"
      hits=$(grep -rlE '<(div|span|img|button|input|textarea|p|select|a)\b' dist-weapp --include='*.wxml' | grep -v '/base.wxml$' || true)
      if [ -n "$hits" ]; then echo "PROBE-FAIL:"; echo "$hits"; exit 1; fi
      echo "PROBE-OK: no native HTML interactive tags in dist-weapp wxml"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r6-prerelease)
    {
      echo "$PROVENANCE"
      ssh cadenza 'systemctl is-active tuneloop-pre; systemctl show tuneloop-pre -p NRestarts; curl -s localhost:5563/api/config | grep -o "\"version\":\"[^\"]*\""' 2>&1
      rc=$?
      [ "$rc" = "0" ] || exit "$rc"
      echo "PRERELEASE_OK"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r7-owner)
    {
      echo "$PROVENANCE"
      if [ -f "$OUT_DIR/R7-owner.md" ] && grep -q 'OWNER_PASS' "$OUT_DIR/R7-owner.md"; then
        echo "OWNER_PASS"
        exit 0
      fi
      echo "OWNER_PENDING"
      exit 1
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
esac
