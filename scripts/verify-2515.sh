#!/usr/bin/env bash
# verify-2515.sh — Evidence Contract runner for #2515
#   (GET /instrument-loss 未按 oid 隔离：网点账号可见本商户全部网点丢失台账)
#
# Usage:
#   bash scripts/verify-2515.sh r1 … r6 | gate
#
# Each row prints a provenance header, runs the row's commands, writes the
# artifact under docs/evidence/2515/ and fails (exit 1) when any expected
# assertion token is missing. Heavy commands go through scripts/heavy-run.sh.
set -uo pipefail

ROW="${1:-}"
ISSUE=2515
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/docs/evidence/$ISSUE"
HEAVY_RUN="${HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"
LOCK_DIR="${HEAVY_LOCK_DIR:-/tmp/opencode/locks}"

mkdir -p "$OUT" "$ROOT/tmp"
cd "$ROOT" || exit 2

if [ -f "$ROOT/backend/.env" ]; then
  set -a
  # shellcheck disable=SC1091
  . "$ROOT/backend/.env"
  set +a
fi
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-5432}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export TUNELOOP_DB="${TUNELOOP_DB:-tuneloop_debug}"

# r1–r5 需要 Postgres 测试库；r6 / gate 只做构建与静态扫描（P5 在干净 worktree 内调用
# gate，那里没有被 gitignore 的 backend/.env，不能要求 DB 凭据）。
case "$ROW" in
  r1|r2|r3|r4|r5)
    if [ -z "${POSTGRES_PASSWORD:-}" ]; then
      echo "ERROR: POSTGRES_PASSWORD is empty (backend/.env missing?)" >&2
      exit 2
    fi
    ;;
esac

# 证据锚定本单 fix commit（plan Artifact Provenance：证据收尾提交不改变锚点）。
COMMIT="$(git log --format=%H --grep='^fix(issue-2515)' -1)"
if [ -z "$COMMIT" ]; then
  COMMIT="$(git rev-parse HEAD)"
fi
SHORT="$(git rev-parse --short "$COMMIT")"
BUILD="1.0.$SHORT"
DIRTY="false"
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/' || true)" ]; then
  DIRTY="true"
fi
PROVENANCE="# build=$BUILD # commit=$COMMIT # dirty=$DIRTY"

FAILURES=0

heavy() {
  if [ -x "$HEAVY_RUN" ]; then
    bash "$HEAVY_RUN" --dir "$LOCK_DIR" -- "$@"
  else
    "$@"
  fi
}

assert_grep() { # artifact token...
  local artifact="$1"; shift
  local missing=0
  for token in "$@"; do
    if ! grep -qF -- "$token" "$artifact"; then
      echo "MISSING: $token (in ${artifact#$ROOT/})" >&2
      missing=1
      FAILURES=$((FAILURES + 1))
    fi
  done
  if [ "$missing" = "0" ]; then
    echo "ASSERT_OK: ${artifact#$ROOT/} ($# tokens)"
  fi
}

# assert_absent：反向断言（token 不得出现）。用于证明测试未依赖 GORM 自动租户作用域。
assert_absent() { # artifact token...
  local artifact="$1"; shift
  local hit=0
  for token in "$@"; do
    if grep -qF -- "$token" "$artifact"; then
      echo "UNEXPECTED: $token (in ${artifact#$ROOT/})" >&2
      hit=1
      FAILURES=$((FAILURES + 1))
    fi
  done
  if [ "$hit" = "0" ]; then
    echo "ASSERT_ABSENT_OK: ${artifact#$ROOT/} ($# tokens)"
  fi
}

# list_block 抽出 List handler 的函数体，供文本级断言使用（R6）。
list_block() {
  awk '/^func \(h \*InstrumentLossHandler\) List\(/{f=1} f{print} f&&/^}/{exit}' "$ROOT/backend/handlers/instrument_loss.go"
}

go_row() { # artifact testregex ok-token
  local artifact="$1" regex="$2" ok="$3"
  {
    echo "$PROVENANCE"
    echo "# CMD go test -count=1 -v -run '$regex' ./handlers/"
    ( cd "$ROOT/backend" && heavy go test -count=1 -v -run "$regex" ./handlers/ )
  } >>"$artifact" 2>&1
  local rc=$?
  echo "GO_TEST_RC=$rc" >>"$artifact"
  if [ "$rc" = "0" ]; then
    echo "$ok" >>"$artifact"
  fi
}

row_r1() {
  local a="$OUT/R1-site-scope.txt"; : >"$a"
  # R1 = site_member 隔离 + 红证据；R1a = site_admin（同持 instrument:read）同口径。
  go_row "$a" '^TestInstrumentLoss2515_(SiteScope|SiteAdminScope)$' 'R1_OK'
  assert_grep "$a" 'R1_OK' 'CROSS_SITE_LEAK=0' 'OLD_PREDICATE_LEAK=2' 'RED_CONFIRMED=1' \
    'SITE_ADMIN_ROWS=1' 'SITE_ADMIN_CROSS_SITE_LEAK=0'
  # 测试路由已挂 middleware.ExplicitTenantScope()（与生产一致）→ 请求期不得出现自动租户注入日志。
  assert_absent "$a" '[TenantScope][observe]'
}

row_r2() {
  local a="$OUT/R2-merchant-wide.txt"; : >"$a"
  go_row "$a" '^TestInstrumentLoss2515_MerchantAndPlatform$' 'R2_OK'
  assert_grep "$a" 'R2_OK' 'MERCHANT_SEES=2' 'PLATFORM_SEES=2'
}

row_r3() {
  local a="$OUT/R3-multisite.txt"; : >"$a"
  go_row "$a" '^TestInstrumentLoss2515_MultiSiteStaff$' 'R3_OK'
  assert_grep "$a" 'R3_OK' 'MULTISITE_SEES=2'
}

row_r4() {
  local a="$OUT/R4-negative.txt"; : >"$a"
  go_row "$a" '^TestInstrumentLoss2515_NegativeCrossSiteInstrument$' 'R4_OK'
  assert_grep "$a" 'R4_OK' 'CROSS_INSTRUMENT_ROWS=0' 'DRIFT_SCOPE=0' 'SAME_SITE_ROWS=1' 'NO_DRIFT_PROOF=1'
}

row_r5() {
  local a="$OUT/R5-regression.txt"
  {
    echo "$PROVENANCE"
    echo "# R5 既有 TestInstrumentLoss_* 套件回归"
  } >"$a"
  ( cd "$ROOT/backend" && heavy go test ./handlers/ -run 'TestInstrumentLoss' -count=1 -v ) >"$ROOT/tmp/r5-2515.log" 2>&1
  local rc=$?
  local fail_count
  fail_count=$(grep -c '^--- FAIL' "$ROOT/tmp/r5-2515.log" 2>/dev/null || true)
  fail_count=${fail_count:-0}
  grep -E '^--- (PASS|FAIL)|^ok ' "$ROOT/tmp/r5-2515.log" >>"$a" 2>/dev/null || true
  echo "REGRESSION_RC=$rc" >>"$a"
  echo "FAIL_COUNT=$fail_count" >>"$a"
  if [ "$rc" = "0" ] && [ "$fail_count" = "0" ]; then
    echo "REGRESSION_PASS=1" >>"$a"
    echo "R5_OK" >>"$a"
  else
    echo "REGRESSION_PASS=0" >>"$a"
  fi
  assert_grep "$a" 'R5_OK' 'REGRESSION_PASS=1' 'FAIL_COUNT=0'
}

row_r6() {
  local a="$OUT/R6-build-scan.txt"
  {
    echo "$PROVENANCE"
    echo "# R6 go build ./... + List 同类扫描"
  } >"$a"

  ( cd "$ROOT/backend" && heavy go build ./... ) >>"$a" 2>&1
  local build_rc=$?
  echo "BUILD_RC=$build_rc" >>"$a"

  local block
  block="$(list_block)"

  local tenant_only=1
  if printf '%s' "$block" | grep -q 'instrument_loss_records.tenant_id' &&
     ! printf '%s' "$block" | grep -q 'query := db.Where("tenant_id = ?", tid)'; then
    tenant_only=0
  fi
  echo "LIST_TENANT_ONLY=$tenant_only" >>"$a"

  local uses_scope=0
  printf '%s' "$block" | grep -q 'applyInstrumentListScope(db, ctx, visible)' && uses_scope=1
  echo "LIST_USES_SHARED_SCOPE=$uses_scope" >>"$a"

  local parity=0
  if printf '%s' "$block" | grep -q 'middleware.BusinessRoleSiteAdmin' &&
     printf '%s' "$block" | grep -q 'middleware.BusinessRoleSiteMember'; then
    parity=1
  fi
  echo "ROLE_GATE_PARITY=$parity" >>"$a"

  # 注释里的「不使用 current_site_id」不算代码引用，只看非注释行。
  local drift=0
  if printf '%s' "$block" | grep -v '^[[:space:]]*//' | grep -q 'current_site_id'; then
    drift=1
  fi
  echo "CURRENT_SITE_ID_IN_LIST=$drift" >>"$a"

  if [ "$build_rc" = "0" ] && [ "$tenant_only" = "0" ] && [ "$uses_scope" = "1" ] &&
     [ "$parity" = "1" ] && [ "$drift" = "0" ]; then
    echo "R6_OK" >>"$a"
  fi
  assert_grep "$a" 'R6_OK' 'BUILD_RC=0' 'LIST_TENANT_ONLY=0' 'LIST_USES_SHARED_SCOPE=1' 'ROLE_GATE_PARITY=1' 'CURRENT_SITE_ID_IN_LIST=0'
}

row_gate() {
  ( cd "$ROOT/backend" && heavy go build ./... )
}

case "$ROW" in
  r1) row_r1 ;;
  r2) row_r2 ;;
  r3) row_r3 ;;
  r4) row_r4 ;;
  r5) row_r5 ;;
  r6) row_r6 ;;
  gate) row_gate ;;
  *)
    echo "usage: bash scripts/verify-2515.sh {r1|r2|r3|r4|r5|r6|gate}" >&2
    exit 2
    ;;
esac

if [ "$FAILURES" != "0" ]; then
  echo "RESULT: FAIL ($FAILURES assertion(s))"
  exit 1
fi
echo "RESULT: OK ($ROW)"
