#!/usr/bin/env bash
# #2271：3-seed shuffle 回归；失败集合必须等于非 shuffle 基线（NEW/MISSING 空）。
# 用法：
#   OPENCODE_NODE_ID=<node> scripts/test-shuffle.sh [--update-baseline] [seed...]
#   TUNELOOP_TEST_DB=<db>  scripts/test-shuffle.sh [seed...]
# 基线默认 tmp/test-baseline.txt；缺失或 --update-baseline 时自动跑非 shuffle 全量生成。
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE="${BASE:-$ROOT/tmp/test-baseline.txt}"
OUT_DIR="${OUT_DIR:-$ROOT/tmp/shuffle-2271}"
UPDATE_BASELINE=0
if [ "${1:-}" = "--update-baseline" ]; then
  UPDATE_BASELINE=1
  shift
fi

# 多节点安全：必须显式节点/库，禁止默认落到其他节点的测试库（#2288）。
if [ -z "${TUNELOOP_TEST_DB:-}" ] && [ -z "${OPENCODE_NODE_ID:-}" ]; then
  echo "ERROR: 请设置 OPENCODE_NODE_ID（推荐）或 TUNELOOP_TEST_DB；本脚本拒绝使用默认测试库。" >&2
  exit 2
fi

if [ $# -gt 0 ]; then
  SEEDS=("$@")
else
  SEEDS=(20261023 42 7777)
fi

mkdir -p "$OUT_DIR"
run_full() {
  local log="$1"
  (cd "$ROOT/backend" && go test ./handlers -count=1 -v -timeout 35m) >"$log" 2>&1 || true
  grep '^--- FAIL' "$log" | sed 's/.*FAIL: //;s/ (.*//' | sort -u
}

if [ "$UPDATE_BASELINE" = "1" ] || [ ! -f "$BASE" ]; then
  echo "== 生成非 shuffle 基线 -> $BASE"
  run_full "$OUT_DIR/baseline.log" > "$BASE"
  echo "baseline_failed=$(wc -l < "$BASE")"
fi
sort -u "$BASE" > "$OUT_DIR/baseline.sorted"

rc=0
for seed in "${SEEDS[@]}"; do
  log="$OUT_DIR/seed-$seed.log"
  echo "== shuffle seed=$seed -> $log"
  (cd "$ROOT/backend" && GOGC=20 GOMAXPROCS=1 GOFLAGS=-p=1 \
    go test ./handlers -count=1 -v -shuffle="$seed" -timeout 35m) >"$log" 2>&1 || true
  grep '^--- FAIL' "$log" | sed 's/.*FAIL: //;s/ (.*//' | sort -u > "$OUT_DIR/seed-$seed.fail"
  echo "failed=$(wc -l < "$OUT_DIR/seed-$seed.fail")"
  echo "NEW:"; comm -23 "$OUT_DIR/seed-$seed.fail" "$OUT_DIR/baseline.sorted"
  echo "MISSING:"; comm -13 "$OUT_DIR/seed-$seed.fail" "$OUT_DIR/baseline.sorted"
  if [ -n "$(comm -23 "$OUT_DIR/seed-$seed.fail" "$OUT_DIR/baseline.sorted")" ]; then
    rc=1
  fi
done
exit $rc
