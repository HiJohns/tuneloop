#!/usr/bin/env bash
# =============================================================================
# scripts/verify-2477.sh — evidence harness for #2477 (R1–R6)
#
# 口径（Accepted Plan v2 / Decision Lock D1–D5）：
#   R1 归一化纯函数：trim / 全角空白(U+3000) / 制表 / 空串 / 不折叠大小写
#   R2 CreateSite 以归一化串写 IAM 与本地两处；裁 B 不预检 → IAM_CALLS=1
#   R3 归一化后为空 → 400/40002 + 中文，且在调用 IAM 之前返回（IAM_CALLS=0）
#   R4 UpdateSite 共用同一归一化函数：全角空格变体视为未改名（IAM_CALLS=0），
#      真实改名时两处同串
#   R5 只读近似重名清单：READONLY=1 且能输出「归一化同串」分组
#   R6 修后树 go build + gofmt 干净
#
# 用法：bash scripts/verify-2477.sh <r1|r2|r3|r4|r5|r6>
# 产物：docs/evidence/2477/R*-*.txt（首行 provenance）
# =============================================================================
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r2|r3|r4|r5|r6) ;;
  *) echo "usage: $0 <r1|r2|r3|r4|r5|r6>" >&2; exit 2 ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2477
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"

export PATH="/usr/local/go/bin:$PATH"
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_local}"
export TMPDIR="${TMPDIR:-$REPO/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"
HEAVY_RUN="${HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"

HEAD_SHA="$(git rev-parse HEAD)"
if [ -z "${FIX_REF:-}" ]; then
  FIX_REF="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
fi
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR")" ]; then
  DIRTY=true
fi
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"

BODY="$(mktemp "$REPO/tmp/verify-$ISSUE-$ROW.XXXXXX")" || exit 2
trap 'rm -f "$BODY"' EXIT
RC=0
emit() { printf '%s\n' "$*" | tee -a "$BODY"; }
require() { grep -Fq -- "$1" "$BODY" || { echo "ASSERT_FAIL: missing [$1]" >&2; RC=1; }; }
finish() { # artifact-name
  local name="$1"
  {
    printf '%s\n' "$PROV"
    cat "$BODY"
  } > "$OUT_DIR/$name"
  cat "$OUT_DIR/$name"
  [ "$RC" = "0" ] || return 1
}
heavy() { if [ -x "$HEAVY_RUN" ]; then bash "$HEAVY_RUN" --timeout 3600 -- "$@"; else "$@"; fi; }
run_test() { # selector logfile
  ( cd "$REPO/backend" && heavy go test ./handlers -run "$1" -count=1 -v -timeout 10m ) > "$2" 2>&1
  local rc=$?
  grep -E '^(=== RUN|--- PASS|--- FAIL|ok|FAIL|PASS)|^ *(NORM_|IAM_|LOCAL_|NAMES_|HTTP=|CODE=|MSG_|RENAME_)' "$2" | tee -a "$BODY" >/dev/null
  return $rc
}

case "$ROW" in
r1)
  if ! run_test '^TestNormalizeSiteName2477$' "$TMPDIR/r1-$ISSUE.log"; then
    emit "TEST_EXIT=1"
    RC=1
  fi
  require "NORM_TRIM=1"
  require "NORM_FULLWIDTH=1"
  require "NORM_TAB=1"
  require "NORM_EMPTY=1"
  require "NORM_CASE_KEPT=1"
  finish "R1-normalize.txt"
  ;;
r2)
  if ! run_test '^TestCreateSiteNormalizedName2477$' "$TMPDIR/r2-$ISSUE.log"; then
    emit "TEST_EXIT=1"
    RC=1
  fi
  require "IAM_NAME_TRIMMED=1"
  require "LOCAL_NAME_TRIMMED=1"
  require "NAMES_EQUAL=1"
  require "IAM_CALLS=1"
  finish "R2-create.txt"
  ;;
r3)
  if ! run_test '^TestCreateSiteEmptyName2477$' "$TMPDIR/r3-$ISSUE.log"; then
    emit "TEST_EXIT=1"
    RC=1
  fi
  require "HTTP=400"
  require "CODE=40002"
  require "MSG_ZH=1"
  require "IAM_CALLS=0"
  finish "R3-empty.txt"
  ;;
r4)
  if ! run_test '^(TestUpdateSiteNormalizedNoop2477|TestUpdateSiteNormalizedRename2477)$' "$TMPDIR/r4-$ISSUE.log"; then
    emit "TEST_EXIT=1"
    RC=1
  fi
  require "IAM_CALLS=0"
  require "NAMES_EQUAL=1"
  require "RENAME_IAM_NAME=新 店"
  require "RENAME_LOCAL_NAME=新 店"
  finish "R4-update.txt"
  ;;
r5)
  # 正例：同租户三种「视觉同串」（无空格 / 半角空格 / 全角空格 U+3000）+ 另一租户单行。
  # 期望：只有 tenant-a 构成 1 组 3 行冲突，tenant-b 的单行不报。
  FIXTURE="$TMPDIR/r5-$ISSUE.tsv"
  printf 'tenant-a\t测试网点A\ntenant-a\t测试网点 A\ntenant-a\t测试网点\u3000A\ntenant-b\t测试网点A\n' > "$FIXTURE"
  bash "$REPO/scripts/site-name-similar-2477.sh" --source "$FIXTURE" 2>&1 | tee -a "$BODY" >/dev/null
  require "READONLY=1"
  require "COLLIDING_GROUPS=1"
  require "COLLIDING_ROWS=3"
  require "GROUP tenant=tenant-a"
  rm -f "$FIXTURE"

  # 负例：全部互不相同的名称不得被误报为冲突（防止归一化过宽/分组逻辑写错）。
  NEG_FIXTURE="$TMPDIR/r5-neg-$ISSUE.tsv"
  printf 'tenant-a\t测试网点A\ntenant-a\t测试网点B\ntenant-b\t测试网点A\n' > "$NEG_FIXTURE"
  NEG_OUT="$(bash "$REPO/scripts/site-name-similar-2477.sh" --source "$NEG_FIXTURE" 2>&1)"
  printf '%s\n' "$NEG_OUT" | tee -a "$BODY" >/dev/null
  if printf '%s\n' "$NEG_OUT" | grep -Fq "NO_COLLISION=1"; then
    emit "NEGATIVE_NO_COLLISION=1"
  else
    emit "NEGATIVE_NO_COLLISION=0"
    RC=1
  fi
  rm -f "$NEG_FIXTURE"
  finish "R5-similar.txt"
  ;;
r6)
  BUILD_RC=0
  ( cd "$REPO/backend" && heavy go build ./... ) >> "$BODY" 2>&1 || BUILD_RC=1
  if [ "$BUILD_RC" = "0" ]; then emit "GO_BUILD_OK=1"; else emit "GO_BUILD_OK=0"; RC=1; fi
  FMT_DIFF="$(cd "$REPO" && gofmt -l \
    backend/handlers/site.go \
    backend/handlers/site_name_2477.go \
    backend/handlers/site_name_2477_test.go 2>/dev/null | wc -l | tr -d ' ')"
  emit "GOFMT_FILES=$FMT_DIFF"
  if [ "$FMT_DIFF" = "0" ]; then emit "GOFMT_DIFF=0"; else emit "GOFMT_DIFF=1"; RC=1; fi
  finish "R6-build.txt"
  ;;
esac

exit "$RC"
