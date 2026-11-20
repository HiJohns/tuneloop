#!/usr/bin/env bash
# #2388 Rev.4：创建员工成功反馈的证据收尾（R1/R2 人工回执 + R3 静态 + R4 构建）。
# 用法：bash scripts/verify-2388.sh <r1|r2|r3|r4>
# R3/R4 写入 OUT_DIR（默认 docs/evidence/2388）；隔离树生成时把 OUT_DIR 指到树外，
# 使 provenance 的 dirty=false 为真实快照（dirty 判定含 untracked）。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4) ;;
  *) echo "usage: $0 <r1|r2|r3|r4>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
OUT_DIR="${OUT_DIR:-$REPO_ROOT/docs/evidence/2388}"
mkdir -p "$OUT_DIR"

BUILD="1.0.$(git rev-parse --short HEAD)"
COMMIT="$(git rev-parse HEAD)"
DIRTY=false
if [ -n "$(git status --porcelain -uall)" ]; then
  DIRTY=true
fi
PROVENANCE="# build=$BUILD commit=$COMMIT dirty=$DIRTY"

case "$MODE" in
  r1)
    f="$REPO_ROOT/docs/evidence/2388/R1-manual.md"
    [ -f "$f" ] || { echo "R1_FAIL: 缺少 $f"; exit 1; }
    grep -q '^RESULT: PASS$' "$f" || { echo "R1_FAIL: 回执非 PASS"; exit 1; }
    echo "R1_OK: Owner 回执 RESULT: PASS"
    ;;
  r2)
    f="$REPO_ROOT/docs/evidence/2388/R2-auto.md"
    [ -f "$f" ] || { echo "R2_FAIL: 缺少 $f"; exit 1; }
    grep -q '^RESULT: PASS$' "$f" || { echo "R2_FAIL: 回执非 PASS"; exit 1; }
    echo "R2_OK: Owner 回执 RESULT: PASS"
    ;;
  r3)
    {
      echo "$PROVENANCE"
      echo "## resolveCreateFeedback 四种 kind（frontend-pc/src/utils/createFeedback.js）"
      fail=0
      for kind in password_auto password_manual invite blocked; do
        if grep -q "kind: '$kind'" frontend-pc/src/utils/createFeedback.js; then
          echo "KIND_OK: $kind"
        else
          echo "KIND_FAIL: $kind"; fail=1
        fi
      done
      [ "$fail" = "0" ] && echo "STATIC_OK" || echo "STATIC_FAIL"
    } 2>&1 | tee "$OUT_DIR/R3-static.txt"
    grep -q "STATIC_OK" "$OUT_DIR/R3-static.txt"
    exit $?
    ;;
  r4)
    {
      echo "$PROVENANCE"
      echo "## frontend-pc build"
      echo "BUILD_CMD=npm run build"
      ( cd frontend-pc && NODE_OPTIONS="--max-old-space-size=2048" npm run build )
      rc=$?
      echo "BUILD_EXIT=$rc"
      [ "$rc" = "0" ] && echo "BUILD_OK" || echo "BUILD_FAIL"
    } 2>&1 | tee "$OUT_DIR/R4-build.txt"
    grep -q "BUILD_OK" "$OUT_DIR/R4-build.txt"
    exit $?
    ;;
esac
