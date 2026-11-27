#!/usr/bin/env bash
# =============================================================================
# scripts/verify-2603.sh — 证据生成器（#2603 绑定失败文案中文化）
#
# 口径（docs/spec/api/api.md §1.3 注 #2603）：
#   `c.ShouldBindJSON` 失败 → `message` 必须由 `backend/bindmsg.Message(err)` 生成
#   （中文点名「哪一项不能为空 / 格式不对」，或「请求体格式不正确，请检查后重试」），
#   禁止把 `err.Error()` 的 Go/JSON 原文拼进响应体。
#
# 用法：bash scripts/verify-2603.sh <r1..r5>
#   r1 docs/evidence/2603/R1-bindmsg-unit.txt     9 条文案模板 + 称呼表覆盖（无 ASCII）
#   r2 docs/evidence/2603/R2-handler-messages.txt 26 端点 × 2 用例 = 52（handler 直调）
#   r3 docs/evidence/2603/R3-residual-scan.txt    收口扫描（28 处全替换、同类不外溢）
#   r4 docs/evidence/2603/R4-build-compile.txt    go build / gofmt / 测试包编译
#   r5 docs/evidence/2603/R5-api-doc.txt          api.md §1.3 绑定失败口径
#
# 每个 artifact 首行 provenance（P14）：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`，
# 其后 `GENERATED_AT_HEAD=<sha>`。锚点 = 本 Issue 的 fix commit（FIX_REF 可覆盖）。
# 重命令一律经 heavy-run.sh 取锁（D10）。handler 测试用一次性 PG（127.0.0.1:15433，
# 库名由 TUNELOOP_TEST_DB 或 backend/handlers/main_test.go 按节点生成），不碰 dev 库。
# =============================================================================
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4|r5) ;;
  *)
    echo "usage: $0 <r1..r5>" >&2
    exit 2
    ;;
esac

REPO="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not a git repository" >&2; exit 2; }
cd "$REPO" || exit 2

ISSUE=2603
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
BINDMSG_GO="backend/bindmsg/bindmsg.go"
BINDMSG_TEST="backend/bindmsg/bindmsg_test.go"
HANDLER_TEST="backend/handlers/bind_error_2603_test.go"
API_DOC="docs/spec/api/api.md"

# One-time PostgreSQL (#2341口径): never jobmaster-postgres.
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_DB_SIMPLE_PROTOCOL="${TUNELOOP_DB_SIMPLE_PROTOCOL:-1}"
if ! command -v go >/dev/null 2>&1 && [ -x /usr/local/go/bin/go ]; then
  export PATH="/usr/local/go/bin:$PATH"
fi
TMPDIR="${TMPDIR:-$REPO/tmp/gobuild}"
GOCACHE="${GOCACHE:-$REPO/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"
export TMPDIR GOCACHE

HEAD_SHA="$(git rev-parse HEAD)"
if [ -z "${FIX_REF:-}" ]; then
  FIX_REF="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
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
trap '[ -n "${KEEP_WORK:-}" ] || rm -rf "$WORK"' EXIT

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

count_in() { # <pattern> <files...> → 命中行数（0 也输出 0）
  local pat="$1"; shift
  grep -cE -- "$pat" "$@" 2>/dev/null | awk -F: '{s+=$NF} END {print s+0}'
}

RC=0
LOG="$WORK/$MODE.log"
: > "$LOG"

# Handler binding points收口 by this Issue (26 + the 2 site.go seed points).
HANDLER_FILES=(
  backend/handlers/site.go
  backend/handlers/property.go
  backend/handlers/warehouse.go
  backend/handlers/wechatpay_prepay.go
  backend/handlers/renewal.go
  backend/handlers/mall_image.go
  backend/handlers/inventory.go
  backend/handlers/iam_proxy.go
  backend/handlers/banner.go
  backend/handlers/appeal.go
  backend/handlers/payment_calculate.go
  backend/handlers/instrument.go
  backend/handlers/coupon_pricing.go
)
TOUCHED_GO=("$BINDMSG_GO" "$BINDMSG_TEST" "$HANDLER_TEST" "${HANDLER_FILES[@]}")

case "$MODE" in
  r1)
    NAME="R1-bindmsg-unit.txt"
    ( cd "$REPO/backend" && heavy go test ./bindmsg -run 'TestBindErrorMessage2603|TestBindFieldLabelsCoverage2603' -count=1 -v -timeout 10m ) > "$WORK/r1.test.log" 2>&1
    TEST_RC=$?
    {
      printf '%s\n' "$PROV"
      echo "GENERATED_AT_HEAD=$HEAD_SHA"
      echo "# backend/bindmsg：9 条文案模板 + 称呼表覆盖 26 端点字段全集（无 ASCII）"
      grep -E 'BMSG_[A-Z]+=|LABEL_[A-Z_]+=' "$WORK/r1.test.log" | sed 's/^[[:space:]]*//'
      printf 'BINDMSG_UNIT_EXIT=%s\n' "$TEST_RC"
      if [ "$TEST_RC" != "0" ]; then
        echo "--- go test output ---"
        tail -40 "$WORK/r1.test.log"
      fi
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'BINDMSG_UNIT_EXIT=0' \
      'BMSG_REQUIRED=参数不合法：名称不能为空' \
      'BMSG_EMAIL=参数不合法：邮箱格式不正确' \
      'BMSG_UUID=参数不合法：编号不是合法的唯一标识' \
      'BMSG_RANGE=参数不合法：数量取值不合法' \
      'BMSG_ONEOF=参数不合法：状态取值不在允许范围内' \
      'BMSG_TYPE=参数不合法：纬度类型不正确' \
      'BMSG_SYNTAX=请求体格式不正确，请检查后重试' \
      'BMSG_FALLBACK=参数不合法，请检查后重试' \
      'LABEL_ASCII_LEAK=0' 'LABEL_COVERAGE_MISSING=0' || RC=1
    ;;

  r2)
    NAME="R2-handler-messages.txt"
    ( cd "$REPO/backend" && heavy go test ./handlers -run '^TestBindErrorMessages2603$' -count=1 -v -timeout 20m ) > "$WORK/r2.test.log" 2>&1
    TEST_RC=$?
    {
      printf '%s\n' "$PROV"
      echo "GENERATED_AT_HEAD=$HEAD_SHA"
      echo "# 26 个绑定点 × 2 用例（损坏体 / 缺必填或类型不符）= 52；handler 直调 + 一次性 PG"
      grep -E 'BIND2603_[A-Z_]+=' "$WORK/r2.test.log" | sed 's/^[[:space:]]*//'
      printf 'GO_TEST_EXIT=%s\n' "$TEST_RC"
      if [ "$TEST_RC" != "0" ]; then
        echo "--- go test output ---"
        tail -60 "$WORK/r2.test.log"
      fi
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'GO_TEST_EXIT=0' \
      'BIND2603_CASES_TOTAL=52' 'BIND2603_CASES_FAIL=0' \
      'BIND2603_ASCII_LEAK=0' 'BIND2603_RAW_PHRASE_LEAK=0' \
      'BIND2603_SITE_MSG=参数不合法：名称不能为空' \
      'BIND2603_PROPERTY_MSG=参数不合法：属性类型不能为空' \
      'BIND2603_WAREHOUSE_MSG=参数不合法：物流单号不能为空' \
      'BIND2603_COUPON_MSG=参数不合法：优惠码不能为空' \
      'BIND2603_SYNTAX_MSG=请求体格式不正确，请检查后重试' || RC=1
    ;;

  r3)
    NAME="R3-residual-scan.txt"
    RESIDUAL_BIND_RAW="$(count_in 'invalid parameters: " \+ err\.Error\(\)' "${HANDLER_FILES[@]}")"
    RESIDUAL_INVALID_PREFIX="$(count_in '"invalid parameters: "' "${HANDLER_FILES[@]}")"
    BINDMSG_CALL_SITES="$(count_in 'bindmsg\.Message\(err\)' "${HANDLER_FILES[@]}")"
    BINDMSG_HANDLER_FILES="$(grep -lE 'bindmsg\.Message\(err\)' "${HANDLER_FILES[@]}" 2>/dev/null | wc -l | tr -d ' ')"
    OUT_OF_SCOPE_INVALID_REQUEST="$(count_in '"invalid request: " \+ err\.Error\(\)' backend/handlers/*.go)"
    {
      printf '%s\n' "$PROV"
      echo "GENERATED_AT_HEAD=$HEAD_SHA"
      echo "# 收口扫描：28 处（site.go 2 + 其余 12 文件 26）全部收敛；同类 25 处不得外溢"
      printf 'RESIDUAL_BIND_RAW=%s\n' "$RESIDUAL_BIND_RAW"
      printf 'RESIDUAL_INVALID_PREFIX=%s\n' "$RESIDUAL_INVALID_PREFIX"
      printf 'BINDMSG_CALL_SITES=%s\n' "$BINDMSG_CALL_SITES"
      printf 'BINDMSG_HANDLER_FILES=%s\n' "$BINDMSG_HANDLER_FILES"
      printf 'OUT_OF_SCOPE_INVALID_REQUEST=%s\n' "$OUT_OF_SCOPE_INVALID_REQUEST"
      echo "--- per-file bindmsg.Message(err) counts ---"
      (
        cd "$REPO" || exit 2
        for f in "${HANDLER_FILES[@]}"; do
          printf '%s=%s\n' "$f" "$(grep -cE 'bindmsg\.Message\(err\)' "$f" 2>/dev/null || echo 0)"
        done
      )
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'RESIDUAL_BIND_RAW=0' 'RESIDUAL_INVALID_PREFIX=0' \
      'BINDMSG_CALL_SITES=28' 'BINDMSG_HANDLER_FILES=13' \
      'OUT_OF_SCOPE_INVALID_REQUEST=25' || RC=1
    ;;

  r4)
    NAME="R4-build-compile.txt"
    ( cd "$REPO/backend" && heavy go build ./... ) > "$WORK/build.out" 2>&1
    BUILD=$?
    GOFMT_OUT="$(cd "$REPO" && gofmt -l "${TOUCHED_GO[@]}" 2>&1)"
    GOFMT_DIFF=0
    [ -n "$GOFMT_OUT" ] && GOFMT_DIFF=1
    ( cd "$REPO/backend" && heavy go test ./... -run '^$' -count=1 -timeout 30m ) > "$WORK/compile.out" 2>&1
    COMPILE=$?
    {
      printf '%s\n' "$PROV"
      echo "GENERATED_AT_HEAD=$HEAD_SHA"
      echo "BUILD_CMD=cd backend && go build ./..."
      tail -5 "$WORK/build.out"
      printf 'GO_BUILD_EXIT=%s\n' "$BUILD"
      echo "GOFMT_CMD=gofmt -l <28 changed go files>"
      if [ "$GOFMT_DIFF" = "0" ]; then echo "GOFMT_UNFORMATTED=(none)"; else echo "$GOFMT_OUT"; fi
      printf 'GOFMT_DIFF=%s\n' "$GOFMT_DIFF"
      echo "TEST_COMPILE_CMD=cd backend && go test ./... -run '^\$' -count=1"
      grep -E '^(FAIL|ok|---)' "$WORK/compile.out" | grep -c '^FAIL' > "$WORK/compile_fail.count"
      tail -10 "$WORK/compile.out"
      printf 'TEST_COMPILE_EXIT=%s\n' "$COMPILE"
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'GO_BUILD_EXIT=0' 'GOFMT_DIFF=0' 'TEST_COMPILE_EXIT=0' || RC=1
    ;;

  r5)
    NAME="R5-api-doc.txt"
    SECTION="$(sed -n '/^### 1.3 统一响应格式/,/^### 1.4 /p' "$REPO/$API_DOC")"
    API_BIND_RULE=0
    API_FORBID_RAW=0
    API_BINDMSG_REF=0
    case "$SECTION" in *'请求体绑定失败'*) API_BIND_RULE=1 ;; esac
    case "$SECTION" in *'禁止'*'err.Error()'*) API_FORBID_RAW=1 ;; esac
    case "$SECTION" in *'backend/bindmsg'*) API_BINDMSG_REF=1 ;; esac
    {
      printf '%s\n' "$PROV"
      echo "GENERATED_AT_HEAD=$HEAD_SHA"
      echo "# docs/spec/api/api.md §1.3：绑定失败口径 / 禁原文 / 引用唯一出口"
      printf 'API_BIND_RULE=%s\n' "$API_BIND_RULE"
      printf 'API_FORBID_RAW=%s\n' "$API_FORBID_RAW"
      printf 'API_BINDMSG_REF=%s\n' "$API_BINDMSG_REF"
      printf 'API_SECTION_LINES=%s\n' "$(printf '%s\n' "$SECTION" | wc -l | tr -d ' ')"
      echo "--- §1.3 binding note ---"
      printf '%s\n' "$SECTION" | grep -F '请求体绑定失败'
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'API_BIND_RULE=1' 'API_FORBID_RAW=1' 'API_BINDMSG_REF=1' || RC=1
    ;;
esac

ART="$OUT_DIR/$NAME"
# Documented usage redirects stdout straight into $ART
# (`bash scripts/verify-2603.sh r1 > docs/evidence/2603/R1-*.txt`). Copying
# then would race with the shell's write offset and corrupt the artifact, so
# only copy when stdout is a different file.
STDOUT_PATH="$(readlink /proc/self/fd/1 2>/dev/null || true)"
ART_PATH="$(readlink -f "$ART" 2>/dev/null || true)"
if [ -z "$STDOUT_PATH" ] || [ "$STDOUT_PATH" != "$ART_PATH" ]; then
  cp "$LOG" "$ART"
fi
cat "$LOG"
echo "ARTIFACT=$ART" >&2
if [ "$RC" != "0" ]; then
  echo "RESULT: FAIL ($MODE)" >&2
  exit 1
fi
echo "RESULT: PASS ($MODE)" >&2
