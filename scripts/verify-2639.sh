#!/usr/bin/env bash
# =============================================================================
# scripts/verify-2639.sh — 证据生成器（#2639 400 族硬编码英文 message 收口）
#
# 口径（docs/spec/api/api.md §1.3 注 #2639）：
#   非绑定失败的 `400xx`（40001/40002/40003）`message` 必须是**中文业务原因**：
#   禁止英文常量（`invalid id` / `status required` / `could not determine scope
#   from path`）与 JSON 字段名（`level_id` / `referral_ratio`）。需点名字段时唯一
#   出口仍是 `backend/bindmsg`。HTTP/code 一律维持修前基线。
#
# 用法：bash scripts/verify-2639.sh <r1..r4>
#   r1 docs/evidence/2639/R1-residual-scan.txt   多行感知扫描（基线 34 → 修后 1 且属 #2604）
#   r2 docs/evidence/2639/R2-handler-messages.txt 33 站点 + 2 绑定变体 = 35（handler 直调）
#   r3 docs/evidence/2639/R3-build-compile.txt   go build / gofmt / 测试包编译
#   r4 docs/evidence/2639/R4-api-doc.txt         api.md §1.3 400 族口径
#
# 每个 artifact 首行 provenance（P14）：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`，
# 其后 `GENERATED_AT_HEAD=<sha>`。锚点 = 本 Issue 的 fix commit（FIX_REF 可覆盖）。
# 重命令一律经 heavy-run.sh 取锁（D9）。handler 测试用一次性 PG（127.0.0.1:15433，
# 由 backend/handlers/main_test.go 按节点克隆模板库），不碰 dev 库。
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

REPO="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not a git repository" >&2; exit 2; }
cd "$REPO" || exit 2

ISSUE=2639
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
API_DOC="docs/spec/api/api.md"

# 10 handler files in the Issue's scope (identical to the #2608 HANDLER_FILES set).
HANDLER_FILES=(
  backend/handlers/discount.go
  backend/handlers/gift_policies.go
  backend/handlers/instrument_promo_override.go
  backend/handlers/membership_benefits.go
  backend/handlers/membership_level.go
  backend/handlers/points_policy.go
  backend/handlers/user_management.go
  backend/handlers/warning.go
  backend/handlers/merchant_rebate.go
  backend/handlers/webhook_config.go
)
FRONTEND_DIRS=(frontend-pc frontend-mobile)
TOUCHED_GO=(
  backend/handlers/discount.go
  backend/handlers/gift_policies.go
  backend/handlers/instrument_promo_override.go
  backend/handlers/membership_benefits.go
  backend/handlers/membership_level.go
  backend/handlers/points_policy.go
  backend/handlers/user_management.go
  backend/handlers/warning.go
  backend/handlers/error_message_2639_test.go
)

# One-time PostgreSQL (#2341 口径): never jobmaster-postgres.
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

# Multiline-aware 400xx message scanner (see R1 of the Accepted Plan).
# Pairs `"code": NNNNN` with `"message": "…"` on the same line or within the next
# 3 lines so a split gin.H is not skipped (the Issue's own grep missed it).
cat > "$WORK/scan400.awk" <<'AWK'
{
  line = $0
  if (match(line, /"code": *[0-9][0-9][0-9][0-9][0-9]/)) {
    code = substr(line, RSTART, RLENGTH)
    sub(/^"code": */, "", code)
    pending = 3
    codeline = FNR
  } else if (pending > 0) {
    pending--
  } else {
    code = ""
    pending = 0
  }
  if (code == "" || code !~ /^400/) next
  mi = index(line, "\"message\":")
  if (mi == 0) next
  rest = substr(line, mi + length("\"message\":"))
  q = index(rest, "\"")
  if (q == 0) next
  rest = substr(rest, q + 1)
  if (rest == "") next
  end = index(rest, "\"")
  body = rest
  if (end > 0) body = substr(rest, 1, end - 1)
  if (index(body, "_") > 0) {
    unders++
    printf "UNDERSCORE %s:%d code=%d\n", FILENAME, FNR, code
  }
  ch = substr(body, 1, 1)
  if (ch !~ /^[A-Za-z]$/) next
  hits++
  if (codeline != FNR) multiline++
  printf "HIT %s:%d code=%d code_line=%d\n", FILENAME, FNR, code, codeline
}
END {
  printf "HITS=%d\n", hits + 0
  printf "MULTILINE=%d\n", multiline + 0
  printf "UNDERSCORE_HITS=%d\n", unders + 0
}
AWK

RC=0
LOG="$WORK/$MODE.log"
: > "$LOG"

case "$MODE" in
  r1)
    NAME="R1-residual-scan.txt"
    BASE="$(git merge-base HEAD origin/develop)"
    BASEDIR="$WORK/baseline"
    mkdir -p "$BASEDIR"
    for f in "${HANDLER_FILES[@]}"; do
      git show "$BASE:$f" > "$BASEDIR/$(basename "$f")" 2>/dev/null || : > "$BASEDIR/$(basename "$f")"
    done

    awk -f "$WORK/scan400.awk" "$BASEDIR"/*.go > "$WORK/baseline.scan" 2>&1
    awk -f "$WORK/scan400.awk" "${HANDLER_FILES[@]/#/}" > "$WORK/postfix.scan" 2>&1
    BASELINE_HITS="$(sed -n 's/^HITS=//p' "$WORK/baseline.scan" | tail -1)"
    BASELINE_MULTILINE="$(sed -n 's/^MULTILINE=//p' "$WORK/baseline.scan" | tail -1)"
    POSTFIX_HITS="$(sed -n 's/^HITS=//p' "$WORK/postfix.scan" | tail -1)"
    UNDERSCORE_HITS="$(sed -n 's/^UNDERSCORE_HITS=//p' "$WORK/postfix.scan" | tail -1)"

    # The single remaining hit must be the #2604 line (explicitly out of scope).
    POSTFIX_HIT_IS_OOS_2604=0
    POSTFIX_HIT_LINE="$(grep '^HIT ' "$WORK/postfix.scan" | head -1 | sed 's|.*/||')"
    if [ "$POSTFIX_HITS" = "1" ] && [ "$POSTFIX_HIT_LINE" = "user_management.go:142 code=40002 code_line=142" ]; then
      POSTFIX_HIT_IS_OOS_2604=1
    fi

    # #2604 owns `"invalid request: " + err.Error()` — must stay byte-identical.
    OOS_PATTERN='"invalid request: " + err.Error()'
    OOS_LINE_BASE="$(grep -Fn -- "$OOS_PATTERN" "$BASEDIR/user_management.go" | head -1)"
    OOS_LINE_NOW="$(grep -Fn -- "$OOS_PATTERN" backend/handlers/user_management.go | head -1)"
    OOS_2604_LINE_UNCHANGED=0
    if [ -n "$OOS_LINE_BASE" ] && [ "$OOS_LINE_BASE" = "$OOS_LINE_NOW" ]; then
      OOS_2604_LINE_UNCHANGED=1
    fi

    OOS_403_404_COUNT="$(count_in '"code": *40[34][0-9]{2}.*"message": *"[A-Za-z]' "${HANDLER_FILES[@]}")"

    FRONTEND_DIFF=0
    if ! git diff --quiet "$BASE" -- "${FRONTEND_DIRS[@]}" 2>/dev/null; then
      FRONTEND_DIFF=1
    fi
    UNTRACKED_FRONTEND="$(git status --porcelain --untracked-files=all -- "${FRONTEND_DIRS[@]}" 2>/dev/null | wc -l | tr -d ' ')"
    if [ "$UNTRACKED_FRONTEND" != "0" ]; then
      FRONTEND_DIFF=1
    fi

    {
      printf '%s\n' "$PROV"
      echo "GENERATED_AT_HEAD=$HEAD_SHA"
      echo "BASELINE_REF=$BASE"
      echo "# 多行感知扫描：10 文件 400xx 分支中 message 以 ASCII 字母起始的点位"
      printf 'BASELINE_400XX_HITS=%s\n' "${BASELINE_HITS:-0}"
      printf 'BASELINE_MULTILINE_HIT=%s\n' "${BASELINE_MULTILINE:-0}"
      printf 'POSTFIX_400XX_HITS=%s\n' "${POSTFIX_HITS:-0}"
      printf 'POSTFIX_HIT_IS_OOS_2604=%s\n' "$POSTFIX_HIT_IS_OOS_2604"
      printf 'OOS_2604_LINE_UNCHANGED=%s\n' "$OOS_2604_LINE_UNCHANGED"
      printf 'UNDERSCORE_400XX_HITS=%s\n' "${UNDERSCORE_HITS:-0}"
      printf 'OOS_403_404_COUNT=%s\n' "$OOS_403_404_COUNT"
      printf 'FRONTEND_DIFF=%s\n' "$FRONTEND_DIFF"
      echo "--- baseline hits (expect 34, one of them multiline) ---"
      grep '^HIT ' "$WORK/baseline.scan" || true
      echo "--- postfix hits (expect only the #2604 line) ---"
      grep '^HIT ' "$WORK/postfix.scan" || true
      echo "--- postfix underscore hits (expect none) ---"
      grep '^UNDERSCORE ' "$WORK/postfix.scan" || true
      echo "--- #2604 out-of-scope line (baseline vs now) ---"
      printf 'BASELINE_LINE=%s\n' "${OOS_LINE_BASE:-<none>}"
      printf 'CURRENT_LINE=%s\n' "${OOS_LINE_NOW:-<none>}"
      printf 'SCAN_EXIT=0\n'
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'BASELINE_400XX_HITS=34' 'BASELINE_MULTILINE_HIT=1' \
      'POSTFIX_400XX_HITS=1' 'POSTFIX_HIT_IS_OOS_2604=1' 'OOS_2604_LINE_UNCHANGED=1' \
      'UNDERSCORE_400XX_HITS=0' 'OOS_403_404_COUNT=16' 'FRONTEND_DIFF=0' 'SCAN_EXIT=0' || RC=1
    ;;

  r2)
    NAME="R2-handler-messages.txt"
    ( cd "$REPO/backend" && heavy go test ./handlers -run '^TestErrorMessages2639$' -count=1 -v -timeout 20m ) > "$WORK/r2.test.log" 2>&1
    TEST_RC=$?
    {
      printf '%s\n' "$PROV"
      echo "GENERATED_AT_HEAD=$HEAD_SHA"
      echo "# 33 站点 + 2 绑定形态变体（损坏体）= 35；handler 直调 + 一次性 PG"
      grep -E 'MSG2639_[A-Z_]+=' "$WORK/r2.test.log" | sed 's/^[[:space:]]*//'
      printf 'GO_TEST_EXIT=%s\n' "$TEST_RC"
      if [ "$TEST_RC" != "0" ]; then
        echo "--- go test output ---"
        tail -60 "$WORK/r2.test.log"
      fi
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'GO_TEST_EXIT=0' \
      'MSG2639_CASES_TOTAL=35' 'MSG2639_CASES_FAIL=0' \
      'MSG2639_ASCII_LEAK=0' 'MSG2639_FIELDNAME_LEAK=0' 'MSG2639_HTTP_CODE_DRIFT=0' \
      'MSG2639_DISCOUNT_EXPIRED=优惠码已过期' \
      'MSG2639_REFERRAL_RANGE=推荐奖励比例必须在 0~1 之间' \
      'MSG2639_OVERRIDE_TYPE=推广类型必须是折扣、返利或租转售' \
      'MSG2639_BIND_CODE=参数不合法：优惠码不能为空' \
      'MSG2639_BIND_STATUS=参数不合法：状态类型不正确' \
      'MSG2639_WARNING_REASON=请求参数有误，请填写预警原因' || RC=1
    ;;

  r3)
    NAME="R3-build-compile.txt"
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
      echo "GOFMT_CMD=gofmt -l <9 changed go files>"
      if [ "$GOFMT_DIFF" = "0" ]; then echo "GOFMT_UNFORMATTED=(none)"; else echo "$GOFMT_OUT"; fi
      printf 'GOFMT_DIFF=%s\n' "$GOFMT_DIFF"
      echo "TEST_COMPILE_CMD=cd backend && go test ./... -run '^\$' -count=1"
      tail -10 "$WORK/compile.out"
      printf 'TEST_COMPILE_EXIT=%s\n' "$COMPILE"
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'GO_BUILD_EXIT=0' 'GOFMT_DIFF=0' 'TEST_COMPILE_EXIT=0' || RC=1
    ;;

  r4)
    NAME="R4-api-doc.txt"
    SECTION="$(sed -n '/^### 1.3 统一响应格式/,/^### 1.4 /p' "$REPO/$API_DOC")"
    API_400_RULE=0
    API_400_NO_ASCII=0
    API_400_BINDMSG_REF=0
    API_2639_REF=0
    case "$SECTION" in *'400 族服务校验文案'*) API_400_RULE=1 ;; esac
    case "$SECTION" in *'禁止'*'英文常量'*) API_400_NO_ASCII=1 ;; esac
    case "$SECTION" in *'backend/bindmsg'*) API_400_BINDMSG_REF=1 ;; esac
    case "$SECTION" in *'#2639'*) API_2639_REF=1 ;; esac
    {
      printf '%s\n' "$PROV"
      echo "GENERATED_AT_HEAD=$HEAD_SHA"
      echo "# docs/spec/api/api.md §1.3：400 族中文业务文案口径 / 禁英文常量与字段名 / bindmsg 唯一出口"
      printf 'API_400_RULE=%s\n' "$API_400_RULE"
      printf 'API_400_NO_ASCII=%s\n' "$API_400_NO_ASCII"
      printf 'API_400_BINDMSG_REF=%s\n' "$API_400_BINDMSG_REF"
      printf 'API_2639_REF=%s\n' "$API_2639_REF"
      printf 'API_SECTION_LINES=%s\n' "$(printf '%s\n' "$SECTION" | wc -l | tr -d ' ')"
      echo "--- §1.3 #2639 paragraph ---"
      printf '%s\n' "$SECTION" | grep -F '400 族服务校验文案' || true
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'API_400_RULE=1' 'API_400_NO_ASCII=1' 'API_400_BINDMSG_REF=1' 'API_2639_REF=1' || RC=1
    ;;
esac

ART="$OUT_DIR/$NAME"
# Documented usage redirects stdout straight into $ART
# (`bash scripts/verify-2639.sh r1 > docs/evidence/2639/R1-*.txt`). Copying
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
