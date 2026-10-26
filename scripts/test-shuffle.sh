#!/usr/bin/env bash
# #2271：3-seed shuffle 回归；失败集合必须等于非 shuffle 基线（NEW/MISSING 空）。
# 用法：BASE=tmp/test-baseline.txt scripts/test-shuffle.sh [seed...]
# 基线：backend 下 `go test ./handlers -count=1 -v` 的 `--- FAIL` 名单（每行一个）。
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE="${BASE:-$ROOT/tmp/test-baseline.txt}"
OUT_DIR="${OUT_DIR:-$ROOT/tmp/shuffle-2271}"
SEEDS=("${@:-20261023 42 7777}")
if [ ! -f "$BASE" ]; then
  echo "ERROR: baseline not found: $BASE" >&2
  exit 2
fi
mkdir -p "$OUT_DIR"
sort -u "$BASE" > "$OUT_DIR/baseline.sorted"
rc=0
for seed in ${SEEDS[@]}; do
  log="$OUT_DIR/seed-$seed.log"
  echo "== shuffle seed=$seed -> $log"
  (cd "$ROOT/backend" && OPENCODE_NODE_ID="${OPENCODE_NODE_ID:-node-2}" \
    GOGC=20 GOMAXPROCS=1 GOFLAGS=-p=1 go test ./handlers -count=1 -v -shuffle="$seed" -timeout 30m) >"$log" 2>&1 || true
  grep '^--- FAIL' "$log" | sed 's/.*FAIL: //;s/ (.*//' | sort -u > "$OUT_DIR/seed-$seed.fail"
  echo "failed=$(wc -l < "$OUT_DIR/seed-$seed.fail")"
  echo "NEW:"; comm -23 "$OUT_DIR/seed-$seed.fail" "$OUT_DIR/baseline.sorted"
  echo "MISSING:"; comm -13 "$OUT_DIR/seed-$seed.fail" "$OUT_DIR/baseline.sorted"
  if [ -n "$(comm -23 "$OUT_DIR/seed-$seed.fail" "$OUT_DIR/baseline.sorted")" ]; then
    rc=1
  fi
done
exit $rc
