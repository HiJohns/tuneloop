#!/usr/bin/env bash
# #2606 证据生成器 —— instrument.go 400 族收口（A 类 2 处走 bindmsg、B 类 6 处固定中文常量）。
#
# 用法：bash scripts/verify-2606.sh <r1|r2|r3|r4>
#   r1  docs/evidence/2606/R1-static-scan.txt       8 处收口静态扫描（含 base 对照 + code 计数）
#   r2  docs/evidence/2606/R2-handler-messages.txt  A 类 2 端点 4 例绑定失败契约（一次性 PG 15433）
#   r3  docs/evidence/2606/R3-marshal-unreachable.txt B 类不可达见证（10 语料 + 2 负例）
#   r4  docs/evidence/2606/R4-build-compile.txt     构建 / gofmt / 编译 / bindmsg 单测 / 乐器定向测试
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<FIX_REF 40 位> # dirty=<bool>`，随后 GENERATED_AT_HEAD=<sha>。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4) ;;
  *)
    echo "usage: $0 <r1|r2|r3|r4>" >&2
    exit 2
    ;;
esac

REPO="$(git rev-parse --show-toplevel)" || exit 2
cd "$REPO" || exit 2
mkdir -p "$REPO/tmp"
ISSUE=2606
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
INSTRUMENT="backend/handlers/instrument.go"
BASE_REF="$(git merge-base origin/develop HEAD)"
PG_ENV=(POSTGRES_HOST=127.0.0.1 POSTGRES_PORT=15433 POSTGRES_USER=tuneloop_user POSTGRES_PASSWORD=trust TUNELOOP_TEST_DB=tuneloop_test_2606 IAM_NAMESPACE=test-ns IAM_SECRET=test-secret-1489)

LOG="$REPO/tmp/verify-2606-${MODE}.log"
: > "$LOG"
RC=0
NAME=""

emit() { printf '%s\n' "$*" >> "$LOG"; }
require() { grep -Fq -- "$1" "$LOG" || { echo "ASSERT_FAIL: missing [$1]" >&2; RC=1; }; }
provenance() { printf '%s\n%s\n' "$PROV" "$HEAD_LINE" > "$LOG"; }

case "$MODE" in
  r1)
    NAME="R1-static-scan.txt"
    provenance
    BASE_FILE="$REPO/tmp/verify-2606-base-instrument.go"
    git show "${BASE_REF}:${INSTRUMENT}" > "$BASE_FILE" 2>/dev/null || { echo "FATAL: base file unreadable" >&2; exit 2; }
    BARE_SITES="$(grep -c '"message": err.Error()' "$INSTRUMENT" || true)"
    BARE_BASE="$(grep -c '"message": err.Error()' "$BASE_FILE" || true)"
    MARSHAL_SITES="$(grep -cE '"invalid (images|specifications|pricing) format: "' "$INSTRUMENT" || true)"
    MARSHAL_BASE="$(grep -cE '"invalid (images|specifications|pricing) format: "' "$BASE_FILE" || true)"
    IMAGES="$(grep -c '参数不合法：图片格式不正确' "$INSTRUMENT" || true)"
    SPECS="$(grep -c '参数不合法：规格格式不正确' "$INSTRUMENT" || true)"
    PRICING="$(grep -c '参数不合法：价格格式不正确' "$INSTRUMENT" || true)"
    C40003="$(grep -cE '"code":[[:space:]]*40003' "$INSTRUMENT" || true)"
    C40004="$(grep -cE '"code":[[:space:]]*40004' "$INSTRUMENT" || true)"
    C40005="$(grep -cE '"code":[[:space:]]*40005' "$INSTRUMENT" || true)"
    ASCII_LEAK=0
    for msg in '参数不合法：图片格式不正确' '参数不合法：规格格式不正确' '参数不合法：价格格式不正确'; do
      if printf '%s' "$msg" | grep -q '[A-Za-z]'; then ASCII_LEAK=$((ASCII_LEAK + 1)); fi
      grep -Fq "\"$msg\"" "$INSTRUMENT" || ASCII_LEAK=$((ASCII_LEAK + 1))
    done
    BINDMSG_CALLS="$(grep -c 'bindmsg\.Message(err)' "$INSTRUMENT" || true)"
    emit "BASE_REF=$BASE_REF"
    emit "BASE_REF_FILE=$INSTRUMENT"
    emit "BARE_BIND_LEAK_BASE=$BARE_BASE"
    emit "BARE_BIND_LEAK_SITES=$BARE_SITES"
    emit "MARSHAL_LEAK_BASE=$MARSHAL_BASE"
    emit "MARSHAL_LEAK_SITES=$MARSHAL_SITES"
    emit "IMAGES_INVALID_MSG_COUNT=$IMAGES"
    emit "SPECS_INVALID_MSG_COUNT=$SPECS"
    emit "PRICING_INVALID_MSG_COUNT=$PRICING"
    emit "CODE_40003_COUNT=$C40003"
    emit "CODE_40004_COUNT=$C40004"
    emit "CODE_40005_COUNT=$C40005"
    emit "BINDMSG_MESSAGE_SITES=$BINDMSG_CALLS"
    emit "MSG_ASCII_LEAK=$ASCII_LEAK"
    emit "--- target sites ---"
    grep -nE '"message": (err\.Error\(\)|bindmsg\.Message\(err\)|"参数不合法：)' "$INSTRUMENT" | sed 's/^/SITE: /' >> "$LOG"
    require 'BARE_BIND_LEAK_BASE=2'
    require 'BARE_BIND_LEAK_SITES=0'
    require 'MARSHAL_LEAK_BASE=6'
    require 'MARSHAL_LEAK_SITES=0'
    require 'IMAGES_INVALID_MSG_COUNT=2'
    require 'SPECS_INVALID_MSG_COUNT=2'
    require 'PRICING_INVALID_MSG_COUNT=2'
    require 'CODE_40003_COUNT=3'
    require 'CODE_40004_COUNT=2'
    require 'CODE_40005_COUNT=2'
    require 'MSG_ASCII_LEAK=0'
    ;;

  r2)
    NAME="R2-handler-messages.txt"
    provenance
    TEST_LOG="$REPO/tmp/verify-2606-r2-gotest.log"
    "$HEAVY_RUN" --timeout 1800 -- env "${PG_ENV[@]}" bash -c "cd '$REPO/backend' && go test ./handlers -run '^TestBindErrorMessages2606$' -count=1 -v" > "$TEST_LOG" 2>&1
    GO_RC=$?
    emit "GO_TEST_CMD=cd backend && go test ./handlers -run '^TestBindErrorMessages2606$' -count=1 -v (PG 15433/tuneloop_test_2606)"
    emit "GO_TEST_EXIT=$GO_RC"
    grep -E 'BIND2606_|^(--- (PASS|FAIL)|ok|FAIL)' "$TEST_LOG" | sed 's/^/GOTEST: /' >> "$LOG"
    for tok in BIND2606_CASES_TOTAL BIND2606_CASES_FAIL BIND2606_ASCII_LEAK BIND2606_RAW_PHRASE_LEAK BIND2606_SYNTAX_MSG BIND2606_CATEGORY_MSG BIND2606_STATUS_TYPE_MSG; do
      val="$(sed -n "s/.*${tok}=//p" "$TEST_LOG" | head -1)"
      emit "${tok}=${val:-missing}"
    done
    [ "$GO_RC" = "0" ] || { echo "ASSERT_FAIL: handler test failed" >&2; RC=1; }
    require 'GO_TEST_EXIT=0'
    require 'BIND2606_CASES_TOTAL=4'
    require 'BIND2606_CASES_FAIL=0'
    require 'BIND2606_ASCII_LEAK=0'
    require 'BIND2606_RAW_PHRASE_LEAK=0'
    require 'BIND2606_CATEGORY_MSG=参数不合法：分类不能为空'
    require 'BIND2606_STATUS_TYPE_MSG=参数不合法：状态类型不正确'
    require 'BIND2606_SYNTAX_MSG=请求体格式不正确，请检查后重试'
    ;;

  r3)
    NAME="R3-marshal-unreachable.txt"
    provenance
    TEST_LOG="$REPO/tmp/verify-2606-r3-gotest.log"
    "$HEAVY_RUN" --timeout 1800 -- env "${PG_ENV[@]}" bash -c "cd '$REPO/backend' && go test ./handlers -run '^TestInstrumentMarshalUnreachable2606$' -count=1 -v" > "$TEST_LOG" 2>&1
    GO_RC=$?
    emit "GO_TEST_CMD=cd backend && go test ./handlers -run '^TestInstrumentMarshalUnreachable2606\$' -count=1 -v"
    emit "GO_TEST_EXIT=$GO_RC"
    grep -E 'MARSHAL_|^(--- (PASS|FAIL)|ok|FAIL)' "$TEST_LOG" | sed 's/^/GOTEST: /' >> "$LOG"
    for tok in MARSHAL_ROUNDTRIP_FAIL MARSHAL_CORPUS_CASES MARSHAL_NONJSON_FAILS; do
      val="$(sed -n "s/.*${tok}=//p" "$TEST_LOG" | head -1)"
      emit "${tok}=${val:-missing}"
    done
    [ "$GO_RC" = "0" ] || { echo "ASSERT_FAIL: marshal witness test failed" >&2; RC=1; }
    require 'GO_TEST_EXIT=0'
    require 'MARSHAL_ROUNDTRIP_FAIL=0'
    require 'MARSHAL_CORPUS_CASES=10'
    require 'MARSHAL_NONJSON_FAILS=2'
    ;;

  r4)
    NAME="R4-build-compile.txt"
    provenance
    BUILD_LOG="$REPO/tmp/verify-2606-r4-build.log"
    "$HEAVY_RUN" --timeout 1800 -- bash -c "cd '$REPO/backend' && go build ./..." > "$BUILD_LOG" 2>&1
    BUILD_RC=$?
    GOFMT_DIFF="$(gofmt -l backend/bindmsg/bindmsg.go backend/bindmsg/bindmsg_2606_test.go backend/handlers/instrument.go backend/handlers/instrument_400_messages_2606_test.go 2>/dev/null | wc -l | tr -d ' ')"
    COMPILE_LOG="$REPO/tmp/verify-2606-r4-compile.log"
    "$HEAVY_RUN" --timeout 1800 -- env "${PG_ENV[@]}" bash -c "cd '$REPO/backend' && go test ./handlers -run '^$' -count=1" > "$COMPILE_LOG" 2>&1
    COMPILE_RC=$?
    BINDMSG_LOG="$REPO/tmp/verify-2606-r4-bindmsg.log"
    "$HEAVY_RUN" --timeout 1800 -- bash -c "cd '$REPO/backend' && go test ./bindmsg -count=1" > "$BINDMSG_LOG" 2>&1
    BINDMSG_RC=$?
    INSTR_LOG="$REPO/tmp/verify-2606-r4-instrument.log"
    "$HEAVY_RUN" --timeout 1800 -- env "${PG_ENV[@]}" bash -c "cd '$REPO/backend' && go test ./handlers -run '^(TestCreateInstrument|TestInstrumentCRUD)' -count=1" > "$INSTR_LOG" 2>&1
    INSTR_RC=$?
    emit "GO_BUILD_CMD=cd backend && go build ./..."
    emit "GO_BUILD_EXIT=$BUILD_RC"
    tail -3 "$BUILD_LOG" | sed 's/^/BUILD_TAIL: /' >> "$LOG"
    emit "GOFMT_DIFF=$GOFMT_DIFF"
    emit "TEST_COMPILE_EXIT=$COMPILE_RC"
    tail -3 "$COMPILE_LOG" | sed 's/^/COMPILE_TAIL: /' >> "$LOG"
    emit "BINDMSG_UNIT_EXIT=$BINDMSG_RC"
    tail -3 "$BINDMSG_LOG" | sed 's/^/BINDMSG_TAIL: /' >> "$LOG"
    emit "INSTRUMENT_TESTS_EXIT=$INSTR_RC"
    tail -5 "$INSTR_LOG" | sed 's/^/INSTR_TAIL: /' >> "$LOG"
    [ "$BUILD_RC" = "0" ] || { echo "ASSERT_FAIL: go build failed" >&2; RC=1; }
    [ "$GOFMT_DIFF" = "0" ] || { echo "ASSERT_FAIL: gofmt diff" >&2; RC=1; }
    [ "$COMPILE_RC" = "0" ] || { echo "ASSERT_FAIL: handlers compile failed" >&2; RC=1; }
    [ "$BINDMSG_RC" = "0" ] || { echo "ASSERT_FAIL: bindmsg tests failed" >&2; RC=1; }
    [ "$INSTR_RC" = "0" ] || { echo "ASSERT_FAIL: instrument tests failed" >&2; RC=1; }
    require 'GO_BUILD_EXIT=0'
    require 'GOFMT_DIFF=0'
    require 'TEST_COMPILE_EXIT=0'
    require 'BINDMSG_UNIT_EXIT=0'
    require 'INSTRUMENT_TESTS_EXIT=0'
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
