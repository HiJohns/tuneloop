#!/usr/bin/env bash
# #2445 Rev.2 — 扫码合并（Bind UI / 本地同步 / token / IAM / weapp / builds / gate / docs）
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1-bind-ui) ART_NAME="R1-bind-ui.txt" ;;
  r2-local-sync) ART_NAME="R2-local-sync.txt" ;;
  r3-token) ART_NAME="R3-token.txt" ;;
  r4-iam) ART_NAME="R4-iam-merge.txt" ;;
  r5-iam-password) ART_NAME="R5-iam-password.txt" ;;
  r6-weapp-probe) ART_NAME="R6-weapp-probe.txt" ;;
  r7-build) ART_NAME="R7-build.txt" ;;
  r8-gate-baseline) ART_NAME="R8-full-gate.txt" ;;
  r9-docs) ART_NAME="R9-docs.txt" ;;
  *) echo "usage: $0 <r1-bind-ui|r2-local-sync|r3-token|r4-iam|r5-iam-password|r6-weapp-probe|r7-build|r8-gate-baseline|r9-docs>" >&2; exit 2 ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO"
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/2445}"
mkdir -p "$OUT_DIR" "$REPO/tmp"
ART="$OUT_DIR/$ART_NAME"

FIX_REF="${FIX_REF:-89327c9c1}"
FIX_SHA="$(git rev-parse "$FIX_REF")"
FIX_SHORT="$(git rev-parse --short "$FIX_REF")"
HEAD_SHA="$(git rev-parse --short HEAD)"
BUILD="1.0.$FIX_SHORT"
DIRTY=false
[ -n "$(git status --porcelain -uall)" ] && DIRTY=true
PROV="# build=$BUILD commit=$FIX_SHA dirty=$DIRTY tree=$HEAD_SHA"

backend() {
  cd "$REPO/backend"
  set -a
  # shellcheck disable=SC1091
  . ./.env
  set +a
  export TMPDIR="${TMPDIR:-$REPO/tmp/gobuild}" GOCACHE="${GOCACHE:-$REPO/tmp/gocache}" \
         TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2445_vr}" \
         IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}" IAM_SECRET="${IAM_SECRET:-test-secret-2445}"
  mkdir -p "$TMPDIR" "$GOCACHE"
}

node22() {
  if [ -s "$HOME/.nvm/nvm.sh" ]; then
    # shellcheck disable=SC1090
    . "$HOME/.nvm/nvm.sh"
    nvm use 22 >/dev/null 2>&1 || true
  fi
}

case "$MODE" in
  r1-bind-ui)
    {
      echo "$PROV"
      cd "$REPO/frontend-mobile"
      npx vitest run --config vitest.config.js scripts/page-smoke/bind-needs-merge.test.jsx
      rc=$?
      echo "R1_EXIT=$rc"
      exit "$rc"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r2-local-sync)
    {
      echo "$PROV"
      backend
      go test ./handlers/ -run '^TestMigrateMergedAccountLocal_2445$' -count=1 -v
      rc=$?
      echo "R2_EXIT=$rc"
      exit "$rc"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r3-token)
    {
      echo "$PROV"
      backend
      go test ./handlers/ -run '^(TestBindMergeFlow_2445|TestPollBindToken_States|TestConfirmBind_NeedsMerge)$' -count=1 -v
      rc=$?
      echo "R3_EXIT=$rc"
      exit "$rc"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r4-iam|r5-iam-password)
    {
      echo "$PROV repo=beaconiam iam_commit=f7ccfcd"
      IAM_REPO="${IAM_REPO:-/home/coder/beaconiam}"
      IAM_COMMIT="$(git -C "$IAM_REPO" rev-parse f7ccfcd 2>/dev/null || true)"
      if [ -z "$IAM_COMMIT" ]; then
        echo "FAIL: beaconiam f7ccfcd not resolvable"
        exit 1
      fi
      echo "IAM_REPO=$IAM_REPO IAM_HEAD=$(git -C "$IAM_REPO" rev-parse --short HEAD)"
      cd "$IAM_REPO"
      export TMPDIR="${TMPDIR:-$REPO/tmp/gobuild}" GOCACHE="${GOCACHE:-$REPO/tmp/gocache}"
      go test ./internal/auth -run '2445' -count=1 -v
      rc=$?
      echo "IAM_EXIT=$rc"
      exit "$rc"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r6-weapp-probe)
    {
      echo "$PROV"
      node22
      cd "$REPO/frontend-mobile"
      npm run build:weapp
      rc=$?
      echo "WEAPP_BUILD_EXIT=$rc"
      [ "$rc" = "0" ] || exit 1
      if [ ! -f dist-weapp/pages-weapp/bind/index.js ]; then
        echo "FAIL: bind page not compiled"
        exit 1
      fi
      hits="$(grep -rlE '<(div|span|img|button|input|textarea|p|select|a)\b' dist-weapp --include='*.wxml' | grep -v '/base.wxml$' || true)"
      if [ -n "$hits" ]; then
        echo "FAIL: native HTML tags in weapp output:"
        printf '%s\n' "$hits"
        exit 1
      fi
      echo "BIND_PAGE=dist-weapp/pages-weapp/bind/index.js"
      echo "PROBE-OK"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r7-build)
    {
      echo "$PROV"
      cd "$REPO/frontend-pc"
      npm run build
      pc=$?
      echo "PC_BUILD_EXIT=$pc"
      node22
      cd "$REPO/frontend-mobile"
      npm run build:weapp
      mob=$?
      echo "WEAPP_BUILD_EXIT=$mob"
      [ "$pc" = "0" ] && [ "$mob" = "0" ] && echo "BUILD_OK"
      exit "$(( pc != 0 || mob != 0 ))"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r8-gate-baseline)
    {
      echo "$PROV"
      backend
      echo "BUILD_CMD=go build ./..."
      go build ./...
      brc=$?
      echo "BUILD_EXIT=$brc"
      [ "$brc" = "0" ] && echo "BUILD_OK"
      LOG="$REPO/tmp/2445-full-gate.log"
      echo "FULL_SUITE_CMD=go test ./... -count=1 -timeout 20m"
      go test ./... -count=1 -timeout 20m > "$LOG" 2>&1
      trc=$?
      cat "$LOG"
      fails="$(grep -c '^--- FAIL' "$LOG" || true)"
      echo "FULL_SUITE_EXIT=$trc FAIL_COUNT=${fails:-0}"
      # 既有基线包（非本单）：middleware / service（单数）；必须在报告中点名
      bad="$(grep -E '^FAIL[[:space:]]+tuneloop-backend/' "$LOG" | awk '{print $2}' | grep -vE 'tuneloop-backend/(middleware|service)$' || true)"
      baseline="$(grep -E '^FAIL[[:space:]]+tuneloop-backend/' "$LOG" | awk '{print $2}' | grep -E 'tuneloop-backend/(middleware|service)$' || true)"
      if [ -n "$baseline" ]; then
        echo "BASELINE_FAIL:"
        printf '%s\n' "$baseline"
      fi
      if [ -n "$bad" ]; then
        echo "FAIL: non-baseline package failures:"
        printf '%s\n' "$bad"
        exit 1
      fi
      [ "$brc" = "0" ] || exit 1
      echo "GATE_BASELINE_OK"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r9-docs)
    {
      echo "$PROV"
      fail=0
      for spec in \
        'docs/spec/api/api.md:merge_ticket' \
        'docs/spec/api/api.md:wx-merge-confirm' \
        'docs/spec/api/api.md:wechat-bind/merge' \
        'docs/spec/api/api.md:merged' \
        'docs/spec/ui/ui.md:merged' \
        'docs/cases/account-select.md:合并' \
        'docs/cases/cases.md:2445'; do
        f="${spec%%:*}"; key="${spec#*:}"
        if grep -Fq -- "$key" "$f"; then
          echo "OK $f :: $key"
        else
          echo "MISS $f :: $key"
          fail=1
        fi
      done
      if grep -Fq '我的 → 编辑资料 → 微信绑定' docs/spec/ui/ui.md; then
        echo "MISS ui.md still has old 编辑资料 guide"
        fail=1
      fi
      [ "$fail" = "0" ] && echo "DOCS_OK"
      exit "$fail"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
esac
