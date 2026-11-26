#!/usr/bin/env bash
# =============================================================================
# scripts/verify-2587.sh — 证据生成器（#2587 乐器只读子端点补 instrument:read 守卫）
#
# 口径（docs/spec/permissions.md §3.2.1 / §3.2.3）：
#   7 个只读子端点 = instrument:read；legacy POST /:id/photos/upload = instrument:media_upload。
#
# 用法：bash scripts/verify-2587.sh <r1..r4>
#   r1 docs/evidence/2587/R1-read-endpoints-403.txt      无对应位 → 8/8 403/40305
#   r2 docs/evidence/2587/R2-authorized-passthrough.txt  有对应位 → 8/8 非 403 且非 5xx
#   r3 docs/evidence/2587/R3-route-wiring.txt            路由结构断言 + fix^ 红样
#   r4 docs/evidence/2587/R4-build-lint.txt              go build + 全包编译冒烟 + 定向测试
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`；另起一行 GENERATED_AT_HEAD。
# 重命令一律经 heavy-run.sh 取锁（D6）。
# =============================================================================
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4) ;;
  *)
    echo "usage: $0 <r1..r4>" >&2
    exit 2
    ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2587
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
MAIN_GO="backend/main.go"

export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"

HEAD_SHA="$(git rev-parse HEAD)"
if [ -z "${FIX_REF:-}" ]; then
  FIX_REF="$(git log --format=%H --grep="^fix(issue-$ISSUE)" --reverse | head -1 2>/dev/null || true)"
fi
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor of HEAD" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
DIRTY=false
[ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR")" ] && DIRTY=true
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
HEAVY_RUN="${HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"
LOCK_DIR="${HEAVY_LOCK_DIR:-/tmp/opencode/locks}"
WORK="$(mktemp -d "$REPO/tmp/verify-$ISSUE-XXXXXX")" || exit 2
trap 'rm -rf "$WORK"' EXIT

heavy() {
  if [ -x "$HEAVY_RUN" ]; then bash "$HEAVY_RUN" --dir "$LOCK_DIR" -- "$@"; else "$@"; fi
}

require_tokens() { # log tokens...
  local log="$1"; shift
  local missing="" tok
  for tok in "$@"; do
    grep -Fq -- "$tok" "$log" || missing="$missing [$tok]"
  done
  if [ -n "$missing" ]; then
    echo "ASSERT_FAIL: missing tokens:$missing" >&2
    return 1
  fi
}

go_test_2587() { # pkg, regex, out
  ( cd "$REPO/backend" && heavy go test "$1" -run "$2" -count=1 -v -timeout 10m ) > "$3" 2>&1
  return $?
}

# 8 条端点（method:path）
ROUTES_2587=(
  "GET:/instruments/filter-options"
  "GET:/instruments/check"
  "GET:/instruments/:id/photos/latest"
  "GET:/instruments/:id/media"
  "GET:/instruments/:id/activity-log"
  "GET:/instruments/:id/pricing"
  "GET:/instruments/:id/pricing-v2"
  "POST:/instruments/:id/photos/upload"
)

NAME=""
RC=0
LOG="$WORK/$MODE.log"
: > "$LOG"

case "$MODE" in
  r1)
    NAME="R1-read-endpoints-403.txt"
    go_test_2587 ./handlers 'TestInstrumentReadEndpointsPerm2587' "$WORK/r1.test.log"; TEST_RC=$?
    {
      printf '%s\n' "$PROV"
      printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
      echo "# 8 端点 × 无对应位（CusPerm=1<<5）→ 403/40305"
      printf 'GO_TEST_EXIT=%s\n' "$TEST_RC"
      grep -E 'READ_ENDPOINTS[A-Z0-9_]*=' "$WORK/r1.test.log" | sed 's/^ *//'
      [ "$TEST_RC" = "0" ] && echo "R1_OK" || echo "R1_FAIL"
    } > "$LOG" 2>&1
    grep -q '^R1_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'GO_TEST_EXIT=0' 'READ_ENDPOINTS=8' 'READ_ENDPOINTS_403=8' 'READ_ENDPOINTS_40305=8' || RC=1
    ;;
  r2)
    NAME="R2-authorized-passthrough.txt"
    go_test_2587 ./handlers 'TestAuthorizedPassthrough2587' "$WORK/r2.test.log"; TEST_RC=$?
    {
      printf '%s\n' "$PROV"
      printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
      echo "# 8 端点 × 有对应位（read=1<<1 + media_upload=1<<15）→ 非 403 且非 5xx"
      printf 'GO_TEST_EXIT=%s\n' "$TEST_RC"
      grep -E 'AUTHORIZED_[A-Z0-9_]*=' "$WORK/r2.test.log" | sed 's/^ *//'
      [ "$TEST_RC" = "0" ] && echo "R2_OK" || echo "R2_FAIL"
    } > "$LOG" 2>&1
    grep -q '^R2_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'GO_TEST_EXIT=0' 'AUTHORIZED_ENDPOINTS=8' 'AUTHORIZED_NOT_403=8' 'AUTHORIZED_NOT_5XX=8' || RC=1
    ;;
  r3)
    NAME="R3-route-wiring.txt"
    go_test_2587 . 'TestInstrumentReadRoutes2587' "$WORK/r3.test.log"; TEST_RC=$?
    git show "$FIX_SHA^:$MAIN_GO" > "$WORK/main.before.go" 2>/dev/null || {
      echo "FATAL: cannot read pre-fix main.go" >&2; exit 3; }
    RED_UNGUARDED=0
    for entry in "${ROUTES_2587[@]}"; do
      m="${entry%%:*}"; p="${entry#*:}"
      line="$(grep -F "authRequired.$m(\"$p\"" "$WORK/main.before.go" | head -1)"
      case "$line" in
        *RequireCusPerm*) ;;
        *) RED_UNGUARDED=$((RED_UNGUARDED + 1)) ;;
      esac
    done
    {
      printf '%s\n' "$PROV"
      printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
      echo "# 路由注册行结构断言（backend/instrument_read_guard_2587_test.go）+ fix^ 红样"
      printf 'GO_TEST_EXIT=%s\n' "$TEST_RC"
      grep -E 'ROUTE_[A-Z0-9_]+=' "$WORK/r3.test.log" | sed 's/^ *//'
      printf 'RED_PRE_FIX_UNGUARDED=%s\n' "$RED_UNGUARDED"
      [ "$TEST_RC" = "0" ] && echo "R3_OK" || echo "R3_FAIL"
    } > "$LOG" 2>&1
    grep -q '^R3_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'GO_TEST_EXIT=0' 'ROUTE_READ_GUARDED=7' 'ROUTE_UPLOAD_GUARDED=1' \
      'ROUTE_INSTRUMENT_READ_UNGUARDED=0' 'RED_PRE_FIX_UNGUARDED=8' || RC=1
    ;;
  r4)
    NAME="R4-build-lint.txt"
    {
      printf '%s\n' "$PROV"
      printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
      echo "GO_BUILD_CMD=cd backend && go build ./..."
      ( cd "$REPO/backend" && heavy go build ./... ) > "$WORK/gobuild.out" 2>&1
      GOB=$?
      tail -3 "$WORK/gobuild.out"
      printf 'GO_BUILD_EXIT=%s\n' "$GOB"
      echo "GO_COMPILE_SMOKE_CMD=cd backend && go test ./... -run '^$' -count=1"
      ( cd "$REPO/backend" && heavy go test ./... -run '^$' -count=1 ) > "$WORK/gosmoke.out" 2>&1
      GOS=$?
      tail -3 "$WORK/gosmoke.out"
      printf 'GO_COMPILE_SMOKE_EXIT=%s\n' "$GOS"
      echo "GO_TEST_2587_CMD=cd backend && go test ./handlers . -run 'TestInstrumentReadEndpointsPerm2587|TestAuthorizedPassthrough2587|TestInstrumentReadRoutes2587' -count=1"
      ( cd "$REPO/backend" && heavy go test ./handlers . -run 'TestInstrumentReadEndpointsPerm2587|TestAuthorizedPassthrough2587|TestInstrumentReadRoutes2587' -count=1 ) > "$WORK/gotest.out" 2>&1
      GOT=$?
      tail -3 "$WORK/gotest.out"
      printf 'GO_TEST_2587_EXIT=%s\n' "$GOT"
      if [ "$GOB" = "0" ] && [ "$GOS" = "0" ] && [ "$GOT" = "0" ]; then
        echo "GO_BUILD_OK=1"
        echo "GO_COMPILE_SMOKE_OK=1"
        echo "GO_TEST_2587_OK=1"
        echo "R4_OK"
      else
        echo "GO_BUILD_OK=0"
        echo "R4_FAIL"
      fi
    } > "$LOG" 2>&1
    grep -q '^R4_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'GO_BUILD_OK=1' 'GO_COMPILE_SMOKE_OK=1' 'GO_TEST_2587_OK=1' || RC=1
    ;;
esac

ART="$OUT_DIR/$NAME"
cp "$LOG" "$ART"
echo "ARTIFACT=$ART"
if [ "$RC" != "0" ]; then
  echo "RESULT: FAIL ($MODE)" >&2
  exit 1
fi
echo "RESULT: PASS ($MODE)"
