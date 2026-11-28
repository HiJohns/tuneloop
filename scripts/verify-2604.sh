#!/usr/bin/env bash
# =============================================================================
# scripts/verify-2604.sh — 证据生成器（#2604 绑定失败文案收口第二批）
#
# 口径（承 #2603，docs/spec/api/api.md §1.3）：
#   `c.ShouldBindJSON` 失败 → `message` 必须由 `backend/bindmsg.Message(err[, scope])`
#   生成（中文点名「哪一项不能为空 / 格式不对」，或「请求体格式不正确，请检查后重试」），
#   禁止把 `err.Error()` 的 Go/JSON 原文拼进响应体。
#
# 用法：bash scripts/verify-2604.sh <r0..r4>
#   r0 docs/evidence/2604/R0-red-baseline.txt    修前基线静态计数（base vs head）
#   r1 docs/evidence/2604/R1-bindmsg-unit.txt    56 条称呼 + 2 作用域 + 复数缩写 + dive 下标
#   r2 docs/evidence/2604/R2-handler-messages.txt 25 端点 × 2 用例 = 50（handler 直调）
#   r3 docs/evidence/2604/R3-residual-scan.txt   收口扫描（25 处全替换、同类不外溢）
#   r4 docs/evidence/2604/R4-build-compile.txt   go build / gofmt / 测试包编译
#
# 每个 artifact 首行 provenance（P14）：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`，
# 其后 `GENERATED_AT_HEAD=<sha>`。锚点 = 本 Issue 的 fix commit（FIX_REF 可覆盖）。
# 重命令一律经 heavy-run.sh 取锁（D11）。handler 测试用一次性 PG（127.0.0.1:15433，
# 库名由 backend/handlers/main_test.go 按节点生成），不碰 dev 库。
# =============================================================================
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r0|r1|r2|r3|r4) ;;
  *)
    echo "usage: $0 <r0..r4>" >&2
    exit 2
    ;;
esac

REPO="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not a git repository" >&2; exit 2; }
cd "$REPO" || exit 2

ISSUE=2604
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
BINDMSG_GO="backend/bindmsg/bindmsg.go"
BINDMSG_TEST="backend/bindmsg/bindmsg_2604_test.go"
HANDLER_TEST="backend/handlers/bind_error_2604_test.go"

# 本单 25 个绑定点所在的 17 个 handler 文件。
HANDLER_FILES=(
  backend/handlers/auth.go
  backend/handlers/billing.go
  backend/handlers/confirmation_session.go
  backend/handlers/face_verify.go
  backend/handlers/guarantor.go
  backend/handlers/instrument.go
  backend/handlers/instrument_media.go
  backend/handlers/permission_manage.go
  backend/handlers/platform_staff.go
  backend/handlers/pricing.go
  backend/handlers/registration_session.go
  backend/handlers/role_manage.go
  backend/handlers/smtp_config.go
  backend/handlers/user_address.go
  backend/handlers/user_management.go
  backend/handlers/user_management_idcard.go
  backend/handlers/user_onboarding.go
)

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

count_at() { # <rev> <pattern> <paths...>
  local rev="$1" pat="$2"; shift 2
  git grep -cE -- "$pat" "$rev" -- "$@" 2>/dev/null | awk -F: '{s+=$NF} END {print s+0}'
}

RC=0
LOG="$WORK/$MODE.log"
: > "$LOG"

case "$MODE" in
  r0)
    NAME="R0-red-baseline.txt"
    BASE="$(git merge-base origin/develop "$HEAD_SHA")"
    BASE_SITES="$(count_at "$BASE" '"invalid request: " \+ err\.Error\(\)' "${HANDLER_FILES[@]}")"
    BASE_FILES="$(git grep -lE '"invalid request: " \+ err\.Error\(\)' "$BASE" -- "${HANDLER_FILES[@]}" 2>/dev/null | wc -l | tr -d ' ')"
    BASE_ASCII_MSGS="$(count_at "$BASE" '"message": "invalid request: "' "${HANDLER_FILES[@]}")"
    HEAD_SITES="$(count_in '"invalid request: " \+ err\.Error\(\)' "${HANDLER_FILES[@]}")"
    {
      printf '%s\n' "$PROV"
      echo "GENERATED_AT_HEAD=$HEAD_SHA"
      echo "# 修前基线（base = merge-base origin/develop HEAD）：25 处 / 17 文件直出解析层原文"
      printf 'BASE_COMMIT=%s\n' "$BASE"
      printf 'RED_BASE_SITES=%s\n' "$BASE_SITES"
      printf 'RED_BASE_FILES=%s\n' "$BASE_FILES"
      printf 'RED_BASE_ASCII_MESSAGES=%s\n' "$BASE_ASCII_MSGS"
      printf 'RED_HEAD_SITES=%s\n' "$HEAD_SITES"
      echo "--- per-file base counts ---"
      for f in "${HANDLER_FILES[@]}"; do
        n="$(git grep -c '"invalid request: " + err.Error()' "$BASE" -- "$f" 2>/dev/null | awk -F: '{print $NF}')"
        printf '%s=%s\n' "$f" "${n:-0}"
      done
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'RED_BASE_SITES=25' 'RED_BASE_FILES=17' \
      'RED_BASE_ASCII_MESSAGES=25' 'RED_HEAD_SITES=0' || RC=1
    ;;

  r1)
    NAME="R1-bindmsg-unit.txt"
    ( cd "$REPO/backend" && heavy go test ./bindmsg -run 'TestBindMsg.*2604' -count=1 -v -timeout 10m ) > "$WORK/r1.test.log" 2>&1
    TEST_RC=$?
    {
      printf '%s\n' "$PROV"
      echo "GENERATED_AT_HEAD=$HEAD_SHA"
      echo "# backend/bindmsg：56 条新称呼 + 2 作用域 + 复数缩写 + dive 下标归一"
      grep -E 'LABEL_[A-Z0-9_]+=|SCOPE_[A-Z0-9_]+=|CAMEL_[A-Z0-9_]+=|DIVE_[A-Z0-9_]+=' "$WORK/r1.test.log" | sed 's/^[[:space:]]*//'
      printf 'BINDMSG_UNIT_EXIT=%s\n' "$TEST_RC"
      if [ "$TEST_RC" != "0" ]; then
        echo "--- go test output ---"
        tail -40 "$WORK/r1.test.log"
      fi
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'BINDMSG_UNIT_EXIT=0' \
      'LABEL_NEW_2604=56' 'LABEL_ASCII_LEAK=0' 'LABEL_COVERAGE_MISSING=0' \
      'SCOPE_PERSON_NAME=姓名不能为空' 'SCOPE_ROLE_CODE=角色编码不能为空' \
      'SCOPE_DEFAULT_CODE=优惠码不能为空' \
      'CAMEL_IDS=ids' 'CAMEL_USER_IDS=user_ids' \
      'DIVE_INDEX_LABEL=乐器列表' || RC=1
    ;;

  r2)
    NAME="R2-handler-messages.txt"
    ( cd "$REPO/backend" && heavy go test ./handlers -run '^TestBindErrorMessages2604$' -count=1 -v -timeout 20m ) > "$WORK/r2.test.log" 2>&1
    TEST_RC=$?
    {
      printf '%s\n' "$PROV"
      echo "GENERATED_AT_HEAD=$HEAD_SHA"
      echo "# 25 个绑定点 × 2 用例（损坏体 / 缺必填或类型不符）= 50；handler 直调 + 一次性 PG"
      grep -E 'BIND2604_[A-Z_]+=' "$WORK/r2.test.log" | sed 's/^[[:space:]]*//'
      printf 'GO_TEST_EXIT=%s\n' "$TEST_RC"
      if [ "$TEST_RC" != "0" ]; then
        echo "--- go test output ---"
        tail -60 "$WORK/r2.test.log"
      fi
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'GO_TEST_EXIT=0' \
      'BIND2604_CASES_TOTAL=50' 'BIND2604_CASES_FAIL=0' \
      'BIND2604_ASCII_LEAK=0' 'BIND2604_RAW_PHRASE_LEAK=0' \
      'BIND2604_ROLE_CODE_MSG=参数不合法：角色编码不能为空' \
      'BIND2604_PERSON_NAME_MSG=参数不合法：姓名不能为空' \
      'BIND2604_IDS_MSG=参数不合法：乐器列表不能为空' \
      'BIND2604_SYNTAX_MSG=请求体格式不正确，请检查后重试' || RC=1
    ;;

  r3)
    NAME="R3-residual-scan.txt"
    BASE="$(git merge-base origin/develop "$HEAD_SHA")"
    RESIDUAL_REQUEST="$(count_in '"invalid request: " \+ err\.Error\(\)' "${HANDLER_FILES[@]}")"
    RESIDUAL_PARAMETERS="$(count_in '"invalid parameters: " \+ err\.Error\(\)' backend/handlers/*.go)"
    SITES_NOW="$(count_in 'bindmsg\.Message\(err' "${HANDLER_FILES[@]}")"
    SITES_BASE="$(count_at "$BASE" 'bindmsg\.Message\(err' "${HANDLER_FILES[@]}")"
    FILES_NOW="$(grep -lE 'bindmsg\.Message\(err' "${HANDLER_FILES[@]}" 2>/dev/null | wc -l | tr -d ' ')"
    OUT_BASE=0
    OUT_NOW=0
    for pat in 'invalid request body: " \+ err' 'Invalid request data: " \+ err' \
               'Invalid request: " \+ err' 'Invalid input: " \+ err' \
               'invalid [a-z]+ format: " \+ err'; do
      b="$(count_at "$BASE" "$pat" backend/handlers/*.go)"
      n="$(count_in "$pat" backend/handlers/*.go)"
      OUT_BASE=$((OUT_BASE + b))
      OUT_NOW=$((OUT_NOW + n))
    done
    OUT_OF_SCOPE_UNCHANGED=0
    [ "$OUT_BASE" = "$OUT_NOW" ] && OUT_OF_SCOPE_UNCHANGED=1
    {
      printf '%s\n' "$PROV"
      echo "GENERATED_AT_HEAD=$HEAD_SHA"
      echo "# 收口扫描：25 处全部收敛为 bindmsg；#2605 的 28 处同类字面量不得外溢"
      printf 'RESIDUAL_INVALID_REQUEST_RAW=%s\n' "$RESIDUAL_REQUEST"
      printf 'RESIDUAL_INVALID_PARAMETERS_RAW=%s\n' "$RESIDUAL_PARAMETERS"
      # 替换点数 = 本批新增调用点（instrument.go 在 #2603 已有 1 处，不计入本单）
      printf 'SITES_REPLACED_2604=%s\n' "$((SITES_NOW - SITES_BASE))"
      printf 'HANDLER_FILES_2604=%s\n' "$FILES_NOW"
      printf 'BINDMSG_SITES_BASE=%s\n' "$SITES_BASE"
      printf 'BINDMSG_DELTA_2604=%s\n' "$((SITES_NOW - SITES_BASE))"
      printf 'OUT_OF_SCOPE_BASE=%s\n' "$OUT_BASE"
      printf 'OUT_OF_SCOPE_HEAD=%s\n' "$OUT_NOW"
      printf 'OUT_OF_SCOPE_UNCHANGED=%s\n' "$OUT_OF_SCOPE_UNCHANGED"
      echo "--- per-file bindmsg.Message counts (head) ---"
      for f in "${HANDLER_FILES[@]}"; do
        printf '%s=%s\n' "$f" "$(grep -cE 'bindmsg\.Message\(err' "$f" 2>/dev/null || echo 0)"
      done
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'RESIDUAL_INVALID_REQUEST_RAW=0' 'RESIDUAL_INVALID_PARAMETERS_RAW=0' \
      'SITES_REPLACED_2604=25' 'HANDLER_FILES_2604=17' \
      'BINDMSG_DELTA_2604=25' 'OUT_OF_SCOPE_UNCHANGED=1' || RC=1
    ;;

  r4)
    NAME="R4-build-compile.txt"
    ( cd "$REPO/backend" && heavy go build ./... ) > "$WORK/build.out" 2>&1
    BUILD=$?
    GOFMT_OUT="$(cd "$REPO" && gofmt -l "$BINDMSG_GO" "$BINDMSG_TEST" "$HANDLER_TEST" "${HANDLER_FILES[@]}" 2>&1)"
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
      echo "GOFMT_CMD=gofmt -l <20 changed go files>"
      if [ "$GOFMT_DIFF" = "0" ]; then echo "GOFMT_UNFORMATTED=(none)"; else echo "$GOFMT_OUT"; fi
      printf 'GOFMT_DIFF=%s\n' "$GOFMT_DIFF"
      echo "TEST_COMPILE_CMD=cd backend && go test ./... -run '^\$' -count=1"
      tail -10 "$WORK/compile.out"
      printf 'TEST_COMPILE_EXIT=%s\n' "$COMPILE"
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'GO_BUILD_EXIT=0' 'GOFMT_DIFF=0' 'TEST_COMPILE_EXIT=0' || RC=1
    ;;
esac

ART="$OUT_DIR/$NAME"
# Documented usage redirects stdout straight into $ART
# (`bash scripts/verify-2604.sh r1 > docs/evidence/2604/R1-*.txt`). Copying
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
