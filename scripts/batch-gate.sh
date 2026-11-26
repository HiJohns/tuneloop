#!/usr/bin/env bash
# batch-gate.sh — 批次级全量 gate：整批只跑一次全量测试，供各 Issue 计划引用。
#
# 背景（Owner 2026-11-26）：逐 Issue 跑全量 `go test ./...`（且 P5 还要跑
# baseline 第二遍）耗时 10–30 分钟/单，严重压吞吐。改为：
#   - 单 Issue gate：`go build ./...` + 定向测试 + 编译冒烟（≈3–4 分钟）
#   - 批次 gate：一批合并到集成分支后跑一次全量，产出本文件；计划用
#     `### Batch Gate Reference` 引用它，pre-ready 的 P5 直接采信（不再重跑）。
#
# Usage:
#   bash scripts/batch-gate.sh [--batch <id>] [--cmd <cmd>] [--timeout <dur>] [--dry-run]
#
# 产物：docs/evidence/batch/<id>/full-gate.txt（首行 provenance）
# 输出：可直接粘贴进计划的 `### Batch Gate Reference` 块
set -uo pipefail

BATCH=""
CMD='cd backend && go build ./... && go test ./... -count=1 -timeout 20m -p 2'
TIMEOUT="20m"
BASELINE=""
DRY=0

while [ $# -gt 0 ]; do
  case "$1" in
    --batch) BATCH="${2:-}"; shift 2 ;;
    --cmd) CMD="${2:-}"; shift 2 ;;
    --timeout) TIMEOUT="${2:-}"; shift 2 ;;
    --baseline) BASELINE="${2:-}"; shift 2 ;;
    --dry-run) DRY=1; shift ;;
    -h|--help) sed -n '2,20p' "$0"; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not a git repo" >&2; exit 2; }
cd "$REPO_ROOT"

SHA="$(git rev-parse HEAD)"
SHORT="$(git rev-parse --short=9 HEAD)"
[ -n "$BATCH" ] || BATCH="$(date -u +%Y%m%d-%H%M)-$SHORT"
OUT_DIR="docs/evidence/batch/$BATCH"
OUT_FILE="$OUT_DIR/full-gate.txt"

if [ "$DRY" = "1" ]; then
  echo "DRY_RUN=1"
  echo "COMMAND=$CMD"
  echo "HEAD=$SHA"
  echo "ARTIFACT=$OUT_FILE"
  exit 0
fi

DIRTY="$(git status --porcelain | grep -vE '^\?\?' | head -1 || true)"
if [ -n "$DIRTY" ]; then
  echo "working tree not clean: $DIRTY" >&2
  exit 2
fi

mkdir -p "$OUT_DIR"
LOG="$OUT_DIR/full-gate.log"
START=$(date +%s)
timeout "$TIMEOUT" bash -c "$CMD" >"$LOG" 2>&1
RC=$?
DUR=$(( $(date +%s) - START ))

fail_set() {
  {
    grep -E '^--- FAIL:' "$1" 2>/dev/null | sed -E 's/^--- FAIL: ([^ ]+).*/TEST:\1/'
    grep -E '^FAIL[[:space:]]' "$1" 2>/dev/null | sed -E 's/^FAIL[[:space:]]+([^[:space:]]+).*/PKG:\1/'
    grep -E '^panic:' "$1" 2>/dev/null | sed 's/^/PANIC:/'
  } | sort -u
}

BASE_FAILS=0
NEW_FAILS=""
if [ -n "$BASELINE" ]; then
  BASE_WT="$(mktemp -d)"
  if git worktree add --detach "$BASE_WT" "$BASELINE" >/dev/null 2>&1; then
    ( cd "$BASE_WT" && timeout "$TIMEOUT" bash -c "$CMD" ) >"$OUT_DIR/full-gate-baseline.log" 2>&1
    BASE_FAILS=$(fail_set "$OUT_DIR/full-gate-baseline.log" | wc -l | tr -d ' ')
    NEW_FAILS=$(comm -23 <(fail_set "$LOG") <(fail_set "$OUT_DIR/full-gate-baseline.log") || true)
    git worktree remove --force "$BASE_WT" >/dev/null 2>&1 || rm -rf "$BASE_WT"
  else
    rm -rf "$BASE_WT"
    echo "WARN: cannot create baseline worktree for $BASELINE; baseline diff skipped" >&2
  fi
fi

FAILS=$(grep -cE '^(--- FAIL|FAIL[[:space:]])' "$LOG" 2>/dev/null || true)
PANICS=$(grep -c '^panic:' "$LOG" 2>/dev/null || true)
[ -n "$FAILS" ] || FAILS=0
[ -n "$PANICS" ] || PANICS=0

{
  printf '# build=1.0.%s # commit=%s # dirty=false\n' "$SHORT" "$SHA"
  printf 'GENERATED_AT_HEAD=%s\n' "$SHA"
  printf 'BATCH=%s\n' "$BATCH"
  printf 'COMMAND=%s\n' "$CMD"
  printf 'FULL_SUITE_EXIT=%s\n' "$RC"
  printf 'FAIL_COUNT=%s\n' "$FAILS"
  printf 'PANIC_COUNT=%s\n' "$PANICS"
  printf 'DURATION_SEC=%s\n' "$DUR"
  printf 'TIMEOUT=%s\n' "$TIMEOUT"
  if [ -n "$BASELINE" ]; then
    printf 'BASELINE=%s\n' "$BASELINE"
    printf 'BASELINE_FAIL_COUNT=%s\n' "$BASE_FAILS"
    printf 'FULL_GATE_NEW_FAILURES=%s\n' "$(printf '%s' "$NEW_FAILS" | grep -c . || true)"
    printf 'NEW_PANIC_COUNT=%s\n' "$(printf '%s' "$NEW_FAILS" | grep -c '^PANIC:' || true)"
    printf 'NEW_FAILURES=%s\n' "$(printf '%s' "$NEW_FAILS" | tr '\n' ',' )"
  fi
} > "$OUT_FILE"

cat "$OUT_FILE"
echo
echo '--- paste into the plan ---'
cat <<EOF
### Batch Gate Reference

- batch: $BATCH
- source: $OUT_FILE
- source_tree: $SHA
- result: FULL_SUITE_EXIT=$RC FAIL_COUNT=$FAILS PANIC_COUNT=$PANICS${BASELINE:+ FULL_GATE_NEW_FAILURES=$(printf '%s' "$NEW_FAILS" | grep -c . || true) NEW_PANIC_COUNT=$(printf '%s' "$NEW_FAILS" | grep -c '^PANIC:' || true) BASELINE_FAIL_COUNT=$BASE_FAILS}
- timeout: $TIMEOUT
- marker: BATCH_GATE_REF_OK
EOF

if [ "$RC" != "0" ]; then
  if [ -n "$BASELINE" ] && [ -z "$(printf '%s' "$NEW_FAILS" | grep . || true)" ]; then
    echo "GATE PASS (baseline-diff: no new failures; rc=$RC is pre-existing)" >&2
    exit 0
  fi
  echo
  echo "GATE FAILED (rc=$RC): see $LOG" >&2
  exit "$RC"
fi
