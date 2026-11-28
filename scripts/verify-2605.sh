#!/usr/bin/env bash
# #2605 证据生成器 —— 其余 22 处绑定失败字面量收口（bindmsg + 27 称呼 + 2 作用域 + error 键移除）。
#
# 用法：bash scripts/verify-2605.sh <r0|r1|r2|r3|r4>
#   r0  docs/evidence/2605/R0-red-baseline.txt     修前红样基线（merge-base origin/develop vs HEAD）
#   r1  docs/evidence/2605/R1-bindmsg-unit.txt     bindmsg 27 称呼 + 2 作用域 + 53 字段覆盖
#   r2  docs/evidence/2605/R2-handler-messages.txt 22 端点 × 2 用例契约（一次性 PG 15433）
#   r3  docs/evidence/2605/R3-residual-scan.txt    残余扫描 / 替换点 / 文件数 / 调用点增量
#   r4  docs/evidence/2605/R4-build-compile.txt    构建 / gofmt / 测试包编译
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<FIX_REF 40 位> # dirty=<bool>`，随后 GENERATED_AT_HEAD=<sha>。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r0|r1|r2|r3|r4) ;;
  *)
    echo "usage: $0 <r0|r1|r2|r3|r4>" >&2
    exit 2
    ;;
esac

REPO="$(git rev-parse --show-toplevel)" || exit 2
cd "$REPO" || exit 2
mkdir -p "$REPO/tmp"
ISSUE=2605
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
HEAD_SHA="$(git rev-parse HEAD)"
if [ -z "${FIX_REF:-}" ]; then
  FIX_REF="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
fi
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor of HEAD" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
DIRTY=false
[ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR" ':(exclude)tmp')" ] && DIRTY=true
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
HEAD_LINE="GENERATED_AT_HEAD=$HEAD_SHA"

export PATH="/usr/local/go/bin:$PATH"
HEAVY_RUN="${OPENCODE_HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"
BASE_REF="$(git merge-base origin/develop HEAD)"
# 本单叠加在依赖分支 #2604 之上；「修前」调用点基线取分支起点（= origin/issue-2604 尖端）。
BRANCH_BASE="$(git merge-base "${BRANCH_BASE_REF:-origin/issue-2604}" HEAD 2>/dev/null || true)"
[ -n "$BRANCH_BASE" ] || BRANCH_BASE="$BASE_REF"
PG_ENV=(POSTGRES_HOST=127.0.0.1 POSTGRES_PORT=15433 POSTGRES_USER=tuneloop_user POSTGRES_PASSWORD=trust TUNELOOP_TEST_DB=tuneloop_test_2605 IAM_NAMESPACE=test-ns IAM_SECRET=test-secret-1489)

FILES_2605=(
  backend/handlers/iam_orphan.go
  backend/handlers/user_staff.go
  backend/handlers/api.go
  backend/handlers/lease.go
  backend/handlers/merchant.go
  backend/handlers/merchant_member.go
  backend/handlers/setup.go
  backend/handlers/site_member.go
  backend/handlers/transit_site_admin.go
)

RAW_PATTERNS=(
  '"invalid request body: " *\+ *err\.Error\(\)'
  '"Invalid request data: " *\+ *err\.Error\(\)'
  '"Invalid request: " *\+ *err\.Error\(\)'
  '"Invalid input: " *\+ *err\.Error\(\)'
)

count_raw_in_tree() {
  local total=0
  local pat
  for pat in "${RAW_PATTERNS[@]}"; do
    total=$((total + $(grep -rEc "$pat" backend/handlers/*.go 2>/dev/null | awk -F: '{s+=$2} END {print s+0}')))
  done
  printf '%s' "$total"
}

count_raw_at_ref() { # ref
  local ref=$1 total=0 f
  local file
  for file in "${FILES_2605[@]}"; do
    git show "$ref:$file" > "$REPO/tmp/verify-2605-base-file.go" 2>/dev/null || continue
    for pat in "${RAW_PATTERNS[@]}"; do
      total=$((total + $(grep -cE "$pat" "$REPO/tmp/verify-2605-base-file.go" || true)))
    done
  done
  printf '%s' "$total"
}

count_raw_files_at_ref() { # ref
  local ref=$1 matched=0
  local file
  for file in "${FILES_2605[@]}"; do
    git show "$ref:$file" > "$REPO/tmp/verify-2605-base-file.go" 2>/dev/null || continue
    local hit=0
    for pat in "${RAW_PATTERNS[@]}"; do
      if grep -qE "$pat" "$REPO/tmp/verify-2605-base-file.go"; then hit=1; fi
    done
    matched=$((matched + hit))
  done
  printf '%s' "$matched"
}

count_bindmsg_calls_at_ref() { # ref
  local ref=$1
  git grep -ho 'bindmsg\.Message' "$ref" -- 'backend/handlers/*.go' ':(exclude)backend/handlers/*_test.go' 2>/dev/null | wc -l | tr -d ' '
}

count_bindmsg_calls_in_tree() {
  grep -rho 'bindmsg\.Message' $(ls backend/handlers/*.go | grep -v '_test\.go$') 2>/dev/null | wc -l | tr -d ' '
}

LOG="$REPO/tmp/verify-2605-${MODE}.log"
: > "$LOG"
RC=0
NAME=""

emit() { printf '%s\n' "$*" >> "$LOG"; }
require() { grep -Fq -- "$1" "$LOG" || { echo "ASSERT_FAIL: missing [$1]" >&2; RC=1; }; }
provenance() { printf '%s\n%s\n' "$PROV" "$HEAD_LINE" > "$LOG"; }

case "$MODE" in
  r0)
    NAME="R0-red-baseline.txt"
    provenance
    BASE_SITES="$(count_raw_at_ref "$BASE_REF")"
    BASE_FILES="$(count_raw_files_at_ref "$BASE_REF")"
    HEAD_SITES="$(count_raw_in_tree)"
    emit "BASE_REF=$BASE_REF"
    emit "BASE_SCOPE=merge-base(origin/develop,HEAD)"
    emit "RED_BASE_SITES=$BASE_SITES"
    emit "RED_BASE_FILES=$BASE_FILES"
    emit "RED_BASE_ASCII_MESSAGES=$BASE_SITES"
    emit "RED_HEAD_SITES=$HEAD_SITES"
    emit "--- per-file base counts ---"
    for file in "${FILES_2605[@]}"; do
      git show "$BASE_REF:$file" > "$REPO/tmp/verify-2605-base-file.go" 2>/dev/null || continue
      n=0
      for pat in "${RAW_PATTERNS[@]}"; do
        n=$((n + $(grep -cE "$pat" "$REPO/tmp/verify-2605-base-file.go" || true)))
      done
      emit "BASE_SITE_FILE=$file count=$n"
    done
    require 'RED_BASE_SITES=22'
    require 'RED_BASE_FILES=9'
    require 'RED_BASE_ASCII_MESSAGES=22'
    require 'RED_HEAD_SITES=0'
    ;;

  r1)
    NAME="R1-bindmsg-unit.txt"
    provenance
    TEST_LOG="$REPO/tmp/verify-2605-r1-gotest.log"
    "$HEAVY_RUN" --timeout 1800 -- bash -c "cd '$REPO/backend' && go test ./bindmsg -run '^(TestBindMsgLabels2605|TestBindMsgScope2605|TestBindMsgCoverage2605)$' -count=1 -v" > "$TEST_LOG" 2>&1
    GO_RC=$?
    emit "GO_TEST_CMD=cd backend && go test ./bindmsg -run '^(TestBindMsgLabels2605|TestBindMsgScope2605|TestBindMsgCoverage2605)$' -count=1 -v"
    emit "BINDMSG_UNIT_EXIT=$GO_RC"
    grep -E 'LABEL_|SCOPE_|^(--- (PASS|FAIL)|ok|FAIL)' "$TEST_LOG" | sed 's/^/GOTEST: /' >> "$LOG"
    for tok in LABEL_NEW_2605 LABEL_ASCII_LEAK LABEL_COVERAGE_MISSING SCOPE_WX_CODE SCOPE_CATEGORY_ITEMS SCOPE_DEFAULT_CODE SCOPE_DEFAULT_ITEMS; do
      val="$(sed -n "s/.*${tok}=//p" "$TEST_LOG" | head -1)"
      emit "${tok}=${val:-missing}"
    done
    [ "$GO_RC" = "0" ] || { echo "ASSERT_FAIL: bindmsg unit tests failed" >&2; RC=1; }
    require 'BINDMSG_UNIT_EXIT=0'
    require 'LABEL_NEW_2605=27'
    require 'LABEL_ASCII_LEAK=0'
    require 'LABEL_COVERAGE_MISSING=0'
    require 'SCOPE_WX_CODE=微信授权码'
    require 'SCOPE_CATEGORY_ITEMS=分类列表'
    require 'SCOPE_DEFAULT_CODE=优惠码'
    require 'SCOPE_DEFAULT_ITEMS=乐器列表'
    ;;

  r2)
    NAME="R2-handler-messages.txt"
    provenance
    TEST_LOG="$REPO/tmp/verify-2605-r2-gotest.log"
    "$HEAVY_RUN" --timeout 1800 -- env "${PG_ENV[@]}" bash -c "cd '$REPO/backend' && go test ./handlers -run '^TestBindErrorMessages2605$' -count=1 -v" > "$TEST_LOG" 2>&1
    GO_RC=$?
    emit "GO_TEST_CMD=cd backend && go test ./handlers -run '^TestBindErrorMessages2605$' -count=1 -v (PG 15433/tuneloop_test_2605)"
    emit "GO_TEST_EXIT=$GO_RC"
    grep -E 'BIND2605_|^(--- (PASS|FAIL)|ok|FAIL)' "$TEST_LOG" | sed 's/^/GOTEST: /' >> "$LOG"
    for tok in BIND2605_CASES_TOTAL BIND2605_CASES_FAIL BIND2605_ASCII_LEAK BIND2605_RAW_PHRASE_LEAK BIND2605_ERROR_KEY_LEAK BIND2605_WX_CODE_MSG BIND2605_CATEGORY_ITEMS_MSG BIND2605_LEASE_MSG BIND2605_SYNTAX_MSG; do
      val="$(sed -n "s/.*${tok}=//p" "$TEST_LOG" | head -1)"
      emit "${tok}=${val:-missing}"
    done
    [ "$GO_RC" = "0" ] || { echo "ASSERT_FAIL: handler contract tests failed" >&2; RC=1; }
    require 'GO_TEST_EXIT=0'
    require 'BIND2605_CASES_TOTAL=44'
    require 'BIND2605_CASES_FAIL=0'
    require 'BIND2605_ASCII_LEAK=0'
    require 'BIND2605_RAW_PHRASE_LEAK=0'
    require 'BIND2605_ERROR_KEY_LEAK=0'
    require 'BIND2605_WX_CODE_MSG=参数不合法：微信授权码不能为空'
    require 'BIND2605_CATEGORY_ITEMS_MSG=参数不合法：分类列表不能为空'
    require 'BIND2605_LEASE_MSG=参数不合法：乐器不能为空'
    require 'BIND2605_SYNTAX_MSG=请求体格式不正确，请检查后重试'
    ;;

  r3)
    NAME="R3-residual-scan.txt"
    provenance
    RB="$(grep -rEc '"invalid request body: " *\+ *err\.Error\(\)' backend/handlers/*.go 2>/dev/null | awk -F: '{s+=$2} END {print s+0}')"
    IRD="$(grep -rEc '"Invalid request data: " *\+ *err\.Error\(\)' backend/handlers/*.go 2>/dev/null | awk -F: '{s+=$2} END {print s+0}')"
    IR="$(grep -rEc '"Invalid request: " *\+ *err\.Error\(\)' backend/handlers/*.go 2>/dev/null | awk -F: '{s+=$2} END {print s+0}')"
    II="$(grep -rEc '"Invalid input: " *\+ *err\.Error\(\)' backend/handlers/*.go 2>/dev/null | awk -F: '{s+=$2} END {print s+0}')"
    REPLACED=0
    MATCHED_FILES=0
    for file in "${FILES_2605[@]}"; do
      n="$(grep -c 'bindmsg\.Message(err' "$file" 2>/dev/null || true)"
      REPLACED=$((REPLACED + n))
      [ "$n" -gt 0 ] && MATCHED_FILES=$((MATCHED_FILES + 1))
    done
    BASE_CALLS="$(count_bindmsg_calls_at_ref "$BRANCH_BASE")"
    HEAD_CALLS="$(count_bindmsg_calls_in_tree)"
    DELTA=$((HEAD_CALLS - BASE_CALLS))
    BASE_MARSHAL="$(git show "$BASE_REF:backend/handlers/instrument.go" 2>/dev/null | grep -cE '"invalid (images|specifications|pricing) format: "' || true)"
    HEAD_MARSHAL="$(grep -cE '"invalid (images|specifications|pricing) format: "' backend/handlers/instrument.go 2>/dev/null || true)"
    OUT_OF_SCOPE=0
    [ "$HEAD_MARSHAL" -le "$BASE_MARSHAL" ] && OUT_OF_SCOPE=1
    emit "RESIDUAL_RB_RAW=$RB"
    emit "RESIDUAL_IRD_RAW=$IRD"
    emit "RESIDUAL_IR_RAW=$IR"
    emit "RESIDUAL_II_RAW=$II"
    emit "SITES_REPLACED_2605=$REPLACED"
    emit "HANDLER_FILES_2605=$MATCHED_FILES"
    emit "BINDMSG_BASE_2605=$BASE_CALLS"
    emit "BINDMSG_HEAD_2605=$HEAD_CALLS"
    emit "BINDMSG_DELTA_2605=$DELTA"
    emit "BRANCH_BASE_REF=$BRANCH_BASE"
    emit "INSTRUMENT_MARSHAL_BASE=$BASE_MARSHAL"
    emit "INSTRUMENT_MARSHAL_HEAD=$HEAD_MARSHAL"
    emit "OUT_OF_SCOPE_NOT_INCREASED=$OUT_OF_SCOPE"
    emit "--- per-file replaced sites ---"
    for file in "${FILES_2605[@]}"; do
      emit "REPLACED_FILE=$file count=$(grep -c 'bindmsg\.Message(err' "$file" 2>/dev/null || true)"
    done
    require 'RESIDUAL_RB_RAW=0'
    require 'RESIDUAL_IRD_RAW=0'
    require 'RESIDUAL_IR_RAW=0'
    require 'RESIDUAL_II_RAW=0'
    require 'SITES_REPLACED_2605=22'
    require 'HANDLER_FILES_2605=9'
    require 'BINDMSG_DELTA_2605=22'
    require 'OUT_OF_SCOPE_NOT_INCREASED=1'
    ;;

  r4)
    NAME="R4-build-compile.txt"
    provenance
    BUILD_LOG="$REPO/tmp/verify-2605-r4-build.log"
    "$HEAVY_RUN" --timeout 1800 -- bash -c "cd '$REPO/backend' && go build ./..." > "$BUILD_LOG" 2>&1
    BUILD_RC=$?
    GOFMT_DIFF="$(gofmt -l backend/bindmsg/bindmsg.go backend/bindmsg/bindmsg_2605_test.go backend/handlers/api.go backend/handlers/iam_orphan.go backend/handlers/lease.go backend/handlers/merchant.go backend/handlers/merchant_member.go backend/handlers/setup.go backend/handlers/site_member.go backend/handlers/transit_site_admin.go backend/handlers/user_staff.go backend/handlers/bind_error_2605_test.go 2>/dev/null | wc -l | tr -d ' ')"
    COMPILE_LOG="$REPO/tmp/verify-2605-r4-compile.log"
    "$HEAVY_RUN" --timeout 1800 -- env "${PG_ENV[@]}" bash -c "cd '$REPO/backend' && go test ./handlers -run '^$' -count=1" > "$COMPILE_LOG" 2>&1
    COMPILE_RC=$?
    emit "GO_BUILD_CMD=cd backend && go build ./..."
    emit "GO_BUILD_EXIT=$BUILD_RC"
    tail -3 "$BUILD_LOG" | sed 's/^/BUILD_TAIL: /' >> "$LOG"
    emit "GOFMT_DIFF=$GOFMT_DIFF"
    emit "TEST_COMPILE_EXIT=$COMPILE_RC"
    tail -3 "$COMPILE_LOG" | sed 's/^/COMPILE_TAIL: /' >> "$LOG"
    [ "$BUILD_RC" = "0" ] || { echo "ASSERT_FAIL: go build failed" >&2; RC=1; }
    [ "$GOFMT_DIFF" = "0" ] || { echo "ASSERT_FAIL: gofmt diff" >&2; RC=1; }
    [ "$COMPILE_RC" = "0" ] || { echo "ASSERT_FAIL: handlers compile failed" >&2; RC=1; }
    require 'GO_BUILD_EXIT=0'
    require 'GOFMT_DIFF=0'
    require 'TEST_COMPILE_EXIT=0'
    ;;
esac

if [ "$RC" != "0" ]; then
  echo "RESULT: FAIL ($MODE)" >> "$LOG"
else
  echo "RESULT: PASS ($MODE)" >> "$LOG"
fi
if [ -n "$NAME" ]; then
  cp "$LOG" "$OUT_DIR/$NAME"
  echo "ARTIFACT=$OUT_DIR/$NAME" >&2
fi
tail -1 "$LOG" >&2
[ "$RC" = "0" ] || exit 1
exit 0
