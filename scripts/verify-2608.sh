#!/usr/bin/env bash
# =============================================================================
# scripts/verify-2608.sh — 证据生成器（#2608 绑定失败裸回传 err.Error() 收口）
#
# 口径（docs/spec/api/api.md §1.3 注 #2603 / #2608）：
#   `c.ShouldBindJSON` 失败 → `message` 必须由 `backend/bindmsg` 生成
#   （`Message(err)`，同名异义字段用 `MessageScoped(<scope>, err)`），
#   禁止把 `err.Error()` 的 Go/JSON 原文拼进响应体。
#
# 用法：bash scripts/verify-2608.sh <r1..r5>
#   r1 docs/evidence/2608/R1-bindmsg-unit.txt     称呼表 116 条 + 作用域 + 缩写复数 + 9 条模板
#   r2 docs/evidence/2608/R2-handler-messages.txt 14 端点 × 2 用例 = 28（handler 直调）
#   r3 docs/evidence/2608/R3-residual-scan.txt    收口扫描（14 处全替换、非绑定分支与同类不外溢）
#   r4 docs/evidence/2608/R4-build-compile.txt    go build / gofmt / 测试包编译
#   r5 docs/evidence/2608/R5-api-doc.txt          api.md §1.3 作用域口径 + 唯一出口
#
# 每个 artifact 首行 provenance（P14）：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`，
# 其后 `GENERATED_AT_HEAD=<sha>`。锚点 = 本 Issue 的 fix commit（FIX_REF 可覆盖）。
# 重命令一律经 heavy-run.sh 取锁（D10）。handler 测试用一次性 PG（127.0.0.1:15433，
# 库名由 TUNELOOP_TEST_DB 决定），不碰 dev 库。
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

ISSUE=2608
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
BINDMSG_GO="backend/bindmsg/bindmsg.go"
BINDMSG_TEST="backend/bindmsg/bindmsg_test.go"
HANDLER_TEST="backend/handlers/bind_error_2608_test.go"
API_DOC="docs/spec/api/api.md"

# One-time PostgreSQL (#2341口径): never jobmaster-postgres.
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_local}"
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

# The 10 handler files whose 14 binding branches收口 by this Issue.
HANDLER_FILES=(
  backend/handlers/discount.go
  backend/handlers/gift_policies.go
  backend/handlers/instrument_promo_override.go
  backend/handlers/membership_benefits.go
  backend/handlers/membership_level.go
  backend/handlers/merchant_rebate.go
  backend/handlers/points_policy.go
  backend/handlers/user_management.go
  backend/handlers/warning.go
  backend/handlers/webhook_config.go
)
TOUCHED_GO=("$BINDMSG_GO" "$BINDMSG_TEST" "$HANDLER_TEST" "${HANDLER_FILES[@]}")

case "$MODE" in
  r1)
    NAME="R1-bindmsg-unit.txt"
    ( cd "$REPO/backend" && heavy go test ./bindmsg -run 'TestBindErrorMessage2603|TestBindFieldLabelsCoverage2603|TestBindMessageScoped2608|TestCamelToSnakeAcronymPlural2608|TestBindFieldLabelsCoverage2608' -count=1 -v -timeout 10m ) > "$WORK/r1.test.log" 2>&1
    TEST_RC=$?
    {
      printf '%s\n' "$PROV"
      echo "GENERATED_AT_HEAD=$HEAD_SHA"
      echo "# backend/bindmsg：称呼表 116 条 + 作用域称呼 + 缩写复数还原 + #2603 的 9 条模板"
      grep -E 'BMSG_[A-Z]+=|LABEL_[A-Z_]+=|BIND2608_[A-Z_]+=' "$WORK/r1.test.log" | sed 's/^[[:space:]]*//'
      printf 'BINDMSG_UNIT_EXIT=%s\n' "$TEST_RC"
      if [ "$TEST_RC" != "0" ]; then
        echo "--- go test output ---"
        tail -40 "$WORK/r1.test.log"
      fi
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'BINDMSG_UNIT_EXIT=0' \
      'BMSG_REQUIRED=参数不合法：名称不能为空' \
      'BIND2608_SCOPED_MSG=参数不合法：权益项类型不正确' \
      'BIND2608_UNSCOPED_MSG=参数不合法：乐器列表类型不正确' \
      'BIND2608_SCOPED_UNKNOWN=参数不合法：乐器列表类型不正确' \
      'BIND2608_IDS_LABEL=编号列表' \
      'BIND2608_GUARANTOR_IDS_LABEL=担保人列表' \
      'BIND2608_OPT_IN_LABEL=返点开关' \
      'BIND2608_LEVEL_ID_LABEL=会员等级' \
      'LABEL_ASCII_LEAK=0' 'LABEL_COVERAGE_MISSING=0' || RC=1
    ;;

  r2)
    NAME="R2-handler-messages.txt"
    ( cd "$REPO/backend" && heavy go test ./handlers -run '^TestBindErrorMessages2608$' -count=1 -v -timeout 20m ) > "$WORK/r2.test.log" 2>&1
    TEST_RC=$?
    {
      printf '%s\n' "$PROV"
      echo "GENERATED_AT_HEAD=$HEAD_SHA"
      echo "# 14 个绑定分支 × 2 用例（损坏体 / 缺必填或类型不符）= 28；handler 直调（一次性 PG，14 分支均在 DB 访问前返回）"
      grep -E 'BIND2608_[A-Z_]+=' "$WORK/r2.test.log" | sed 's/^[[:space:]]*//'
      printf 'GO_TEST_EXIT=%s\n' "$TEST_RC"
      if [ "$TEST_RC" != "0" ]; then
        echo "--- go test output ---"
        tail -60 "$WORK/r2.test.log"
      fi
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'GO_TEST_EXIT=0' \
      'BIND2608_CASES_TOTAL=28' 'BIND2608_CASES_FAIL=0' \
      'BIND2608_ASCII_LEAK=0' 'BIND2608_RAW_PHRASE_LEAK=0' \
      'BIND2608_DISCOUNT_MSG=参数不合法：名称不能为空' \
      'BIND2608_DISCOUNTCODE_MSG=参数不合法：优惠码不能为空' \
      'BIND2608_POINTS_MSG=参数不合法：有效天数类型不正确' \
      'BIND2608_BENEFITS_MSG=参数不合法：权益项类型不正确' \
      'BIND2608_REBATE_MSG=参数不合法：返点开关类型不正确' \
      'BIND2608_SYNTAX_MSG=请求体格式不正确，请检查后重试' || RC=1
    ;;

  r3)
    NAME="R3-residual-scan.txt"
    # 绑定分支内残留的原文（`-A1` 探针；多行写法由 BARE_ERR_TOTAL 精确增量兜底）
    BINDING_BRANCH_RAW="$(grep -A1 'if err := c.ShouldBindJSON' backend/handlers/*.go 2>/dev/null | grep -cE '"message": err\.Error\(\)')"
    BINDING_BRANCH_RAW="${BINDING_BRANCH_RAW:-0}"
    # 全目录剩余裸 `"message": err.Error()`（修前 63 → 修后 49）
    BARE_ERR_TOTAL="$(count_in '"message": err\.Error\(\)' backend/handlers/*.go)"
    # 本单 10 个目标文件内剩余裸 err.Error() 处数（修前 45 → 修后 31 = 29×50000 + 2×40002 服务校验）
    BARE_ERR_TOUCHED_FILES="$(count_in '"message": err\.Error\(\)' "${HANDLER_FILES[@]}")"
    BARE_ERR_TOUCHED_FILE_COUNT="$(grep -lE '"message": err\.Error\(\)' "${HANDLER_FILES[@]}" 2>/dev/null | wc -l | tr -d ' ')"
    # 目标文件内 40002 服务校验分支（D8：文案已是中文，不动）
    BARE_40002_SERVICE_RAW="$(count_in '"code": 40002, "message": err\.Error\(\)' "${HANDLER_FILES[@]}")"
    BARE_ERR_OCCURRENCE_FILES="$(grep -lE '"message": err\.Error\(\)' backend/handlers/*.go 2>/dev/null | wc -l | tr -d ' ')"
    BINDMSG_CALL_SITES="$(count_in 'bindmsg\.Message\(err\)' backend/handlers/*.go)"
    SCOPED_CALL_SITES="$(count_in 'bindmsg\.MessageScoped\(' backend/handlers/*.go)"
    BINDMSG_HANDLER_FILES="$(grep -lE 'bindmsg\.Message(Scoped)?\(' backend/handlers/*.go 2>/dev/null | wc -l | tr -d ' ')"
    OUT_OF_SCOPE_INVALID_REQUEST="$(count_in '"invalid request: " \+ err\.Error\(\)' backend/handlers/*.go)"
    OUT_OF_SCOPE_INVALID_PARAMETERS="$(count_in '"invalid parameters: " \+ err\.Error\(\)' backend/handlers/*.go)"
    {
      printf '%s\n' "$PROV"
      echo "GENERATED_AT_HEAD=$HEAD_SHA"
      echo "# 收口扫描：14 处绑定分支全部收敛到 bindmsg；非绑定分支（50000 DB / 40002 服务校验）与同类前缀族不得外溢"
      printf 'BINDING_BRANCH_RAW=%s\n' "$BINDING_BRANCH_RAW"
      printf 'BARE_ERR_TOTAL=%s\n' "$BARE_ERR_TOTAL"
      printf 'BARE_ERR_TOUCHED_FILES=%s\n' "$BARE_ERR_TOUCHED_FILES"
      printf 'BARE_ERR_TOUCHED_FILE_COUNT=%s\n' "$BARE_ERR_TOUCHED_FILE_COUNT"
      printf 'BARE_40002_SERVICE_RAW=%s\n' "$BARE_40002_SERVICE_RAW"
      printf 'BARE_ERR_OCCURRENCE_FILES=%s\n' "$BARE_ERR_OCCURRENCE_FILES"
      printf 'BINDMSG_CALL_SITES=%s\n' "$BINDMSG_CALL_SITES"
      printf 'SCOPED_CALL_SITES=%s\n' "$SCOPED_CALL_SITES"
      printf 'BINDMSG_HANDLER_FILES=%s\n' "$BINDMSG_HANDLER_FILES"
      printf 'OUT_OF_SCOPE_INVALID_REQUEST=%s\n' "$OUT_OF_SCOPE_INVALID_REQUEST"
      printf 'OUT_OF_SCOPE_INVALID_PARAMETERS=%s\n' "$OUT_OF_SCOPE_INVALID_PARAMETERS"
      echo "--- per-file bare err.Error() in the 10 touched files (0 = fully converged) ---"
      (
        cd "$REPO" || exit 2
        for f in "${HANDLER_FILES[@]}"; do
          printf '%s=%s\n' "$f" "$(grep -cE '"message": err\.Error\(\)' "$f" 2>/dev/null || :)"
        done
      )
      echo "--- per-file bindmsg exit calls (Message / MessageScoped) ---"
      (
        cd "$REPO" || exit 2
        for f in "${HANDLER_FILES[@]}"; do
          printf '%s=%s\n' "$f" "$(grep -cE 'bindmsg\.Message(Scoped)?\(' "$f" 2>/dev/null || :)"
        done
      )
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'BINDING_BRANCH_RAW=0' 'BARE_ERR_TOTAL=49' \
      'BARE_ERR_TOUCHED_FILES=31' 'BARE_40002_SERVICE_RAW=2' \
      'BINDMSG_CALL_SITES=41' 'SCOPED_CALL_SITES=1' \
      'BINDMSG_HANDLER_FILES=23' \
      'OUT_OF_SCOPE_INVALID_REQUEST=25' 'OUT_OF_SCOPE_INVALID_PARAMETERS=0' || RC=1
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
      echo "GOFMT_CMD=gofmt -l <13 changed go files>"
      if [ "$GOFMT_DIFF" = "0" ]; then echo "GOFMT_UNFORMATTED=(none)"; else echo "$GOFMT_OUT"; fi
      printf 'GOFMT_DIFF=%s\n' "$GOFMT_DIFF"
      echo "TEST_COMPILE_CMD=cd backend && go test ./... -run '^\$' -count=1"
      tail -10 "$WORK/compile.out"
      printf 'TEST_COMPILE_EXIT=%s\n' "$COMPILE"
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'GO_BUILD_EXIT=0' 'GOFMT_DIFF=0' 'TEST_COMPILE_EXIT=0' || RC=1
    ;;

  r5)
    NAME="R5-api-doc.txt"
    SECTION="$(sed -n '/^### 1.3 统一响应格式/,/^### 1.4 /p' "$REPO/$API_DOC")"
    API_BIND_RULE=0
    API_SCOPED_NOTE=0
    API_BINDMSG_REF=0
    API_FORBID_RAW=0
    SCOPED_HAS_EXIT=0
    SCOPED_HAS_ITEM=0
    case "$SECTION" in *'请求体绑定失败'*) API_BIND_RULE=1 ;; esac
    case "$SECTION" in *'MessageScoped'*) SCOPED_HAS_EXIT=1 ;; esac
    case "$SECTION" in *'权益项'*) SCOPED_HAS_ITEM=1 ;; esac
    [ "$SCOPED_HAS_EXIT" = "1" ] && [ "$SCOPED_HAS_ITEM" = "1" ] && API_SCOPED_NOTE=1
    case "$SECTION" in *'backend/bindmsg'*) API_BINDMSG_REF=1 ;; esac
    case "$SECTION" in *'禁止'*'err.Error()'*) API_FORBID_RAW=1 ;; esac
    {
      printf '%s\n' "$PROV"
      echo "GENERATED_AT_HEAD=$HEAD_SHA"
      echo "# docs/spec/api/api.md §1.3：绑定失败口径 / 同名异义作用域 / 禁原文 / 引用唯一出口"
      printf 'API_BIND_RULE=%s\n' "$API_BIND_RULE"
      printf 'API_SCOPED_NOTE=%s\n' "$API_SCOPED_NOTE"
      printf 'API_BINDMSG_REF=%s\n' "$API_BINDMSG_REF"
      printf 'API_FORBID_RAW=%s\n' "$API_FORBID_RAW"
      printf 'API_SECTION_LINES=%s\n' "$(printf '%s\n' "$SECTION" | wc -l | tr -d ' ')"
      echo "--- §1.3 binding note ---"
      printf '%s\n' "$SECTION" | grep -F '请求体绑定失败'
      echo "--- §1.3 scoped note ---"
      printf '%s\n' "$SECTION" | grep -F '同名异义字段的作用域'
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'API_BIND_RULE=1' 'API_SCOPED_NOTE=1' 'API_BINDMSG_REF=1' 'API_FORBID_RAW=1' || RC=1
    ;;
esac

ART="$OUT_DIR/$NAME"
# Documented usage redirects stdout straight into $ART
# (`bash scripts/verify-2608.sh r1 > docs/evidence/2608/R1-*.txt`). Copying
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
