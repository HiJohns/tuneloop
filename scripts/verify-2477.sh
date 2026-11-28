#!/usr/bin/env bash
# =============================================================================
# scripts/verify-2477.sh — evidence harness for #2477 (R1–R6)
#
# 口径（Accepted Plan v2 / Decision Lock D1–D5）：
#   R1 归一化纯函数：trim / 全角空白(U+3000) / 制表 / 空串 / 不折叠大小写（含红绿样本）
#   R2 CreateSite 以归一化串写 IAM 与本地两处；裁 B 不预检 → IAM_CALLS=1
#   R3 归一化后为空 → 400/40002 + 中文，且在调用 IAM 之前返回（IAM_CALLS=0）
#   R4 UpdateSite 共用同一归一化函数：全角空格变体视为未改名（IAM_CALLS=0），
#      真实改名时两处同串
#   R5 只读近似重名清单：READONLY=1 且能输出「归一化同串」分组（含负例）
#   R6 修后树 go build + gofmt 干净
#
# 用法：bash scripts/verify-2477.sh <r1|r2|r3|r4|r5|r6>
# 输出：全部写 stdout（首行为 provenance），由 Evidence Contract 的重定向落盘：
#       bash scripts/verify-2477.sh r1 > docs/evidence/2477/R1-normalize.txt
# 退出码：0 = 全部断言通过；1 = 断言失败
# =============================================================================
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r2|r3|r4|r5|r6) ;;
  *) echo "usage: $0 <r1|r2|r3|r4|r5|r6>" >&2; exit 2 ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2477

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
if [ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)docs/evidence/$ISSUE")" ]; then
  DIRTY=true
fi
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"

RC=0
require() { grep -Fq -- "$1" "$LOG" || { echo "ASSERT_FAIL: missing [$1]" >&2; RC=1; }; }
heavy() { if [ -x "$HEAVY_RUN" ]; then bash "$HEAVY_RUN" --timeout 3600 -- "$@"; else "$@"; fi; }

# run_test <selector> <logfile>: run the selector, echo its assertion tokens.
run_test() {
  LOG="$2"
  ( cd "$REPO/backend" && heavy go test ./handlers -run "$1" -count=1 -v -timeout 10m ) > "$LOG" 2>&1
  local rc=$?
  # t.Logf lines are emitted as "    file_test.go:NN: TOKEN=VALUE", so the token
  # match must not be anchored at the start of the line.
  grep -E '^(=== RUN|--- PASS|--- FAIL|ok|FAIL|PASS)|(NORM_|IAM_|LOCAL_|NAMES_|MSG_|RENAME_|RED_|GREEN_)[A-Z0-9_]*=|(HTTP|CODE)=[0-9]+' "$LOG" || true
  return $rc
}

printf '%s\n' "$PROV"

case "$ROW" in
r1)
  run_test '^(TestNormalizeSiteName2477|TestTrimSpaceMissesWhitespaceVariants2477)$' "$TMPDIR/r1-$ISSUE.log" || RC=1
  require "NORM_TRIM=1"
  require "NORM_FULLWIDTH=1"
  require "NORM_TAB=1"
  require "NORM_EMPTY=1"
  require "NORM_CASE_KEPT=1"
  # 红绿：修复前的 TrimSpace 口径下两串不同（缺陷成立），修复后归一化到同串。
  require "RED_OLD_TRIMSPACE_DISTINCT=1"
  require "GREEN_NORMALIZED_SAME=1"
  ;;
r2)
  run_test '^TestCreateSiteNormalizedName2477$' "$TMPDIR/r2-$ISSUE.log" || RC=1
  require "IAM_NAME_TRIMMED=1"
  require "LOCAL_NAME_TRIMMED=1"
  require "NAMES_EQUAL=1"
  require "IAM_CALLS=1"
  ;;
r3)
  run_test '^TestCreateSiteEmptyName2477$' "$TMPDIR/r3-$ISSUE.log" || RC=1
  require "HTTP=400"
  require "CODE=40002"
  require "MSG_ZH=1"
  require "IAM_CALLS=0"
  ;;
r4)
  run_test '^(TestUpdateSiteNormalizedNoop2477|TestUpdateSiteNormalizedRename2477)$' "$TMPDIR/r4-$ISSUE.log" || RC=1
  require "IAM_CALLS=0"
  require "NAMES_EQUAL=1"
  require "RENAME_IAM_NAME=新 店"
  require "RENAME_LOCAL_NAME=新 店"
  ;;
r5)
  # 正例：同租户三种「空白变体」（单个半角空格 / 全角空格 U+3000 / 连续两个半角空格）
  # 必须折叠成同一个 key；无空格的「测试网点A」不属于该组（D2 只折叠连续空白）；
  # 另一租户的同名行不报（唯一性 scope = 租户内）。
  FIXTURE="$TMPDIR/r5-$ISSUE.tsv"
  printf 'tenant-a\t测试网点 A\ntenant-a\t测试网点\u3000A\ntenant-a\t测试网点  A\ntenant-a\t测试网点A\ntenant-b\t测试网点 A\n' > "$FIXTURE"
  LOG="$TMPDIR/r5-$ISSUE.log"
  bash "$REPO/scripts/site-name-similar-2477.sh" --source "$FIXTURE" > "$LOG" 2>&1
  cat "$LOG"
  require "READONLY=1"
  require "COLLIDING_GROUPS=1"
  require "COLLIDING_ROWS=3"
  require "GROUP tenant=tenant-a"
  rm -f "$FIXTURE"

  # 负例：互不相同的名称不得被误报为冲突（防止归一化过宽/分组逻辑写错）。
  NEG_FIXTURE="$TMPDIR/r5-neg-$ISSUE.tsv"
  printf 'tenant-a\t测试网点A\ntenant-a\t测试网点B\ntenant-b\t测试网点A\n' > "$NEG_FIXTURE"
  NEG_OUT="$(bash "$REPO/scripts/site-name-similar-2477.sh" --source "$NEG_FIXTURE" 2>&1)"
  printf '%s\n' "$NEG_OUT"
  rm -f "$NEG_FIXTURE"
  if printf '%s\n' "$NEG_OUT" | grep -Fq "NO_COLLISION=1"; then
    echo "NEGATIVE_NO_COLLISION=1"
  else
    echo "NEGATIVE_NO_COLLISION=0"
    RC=1
  fi
  ;;
r6)
  BUILD_RC=0
  ( cd "$REPO/backend" && heavy go build ./... ) 2>&1 || BUILD_RC=1
  if [ "$BUILD_RC" = "0" ]; then echo "GO_BUILD_OK=1"; else echo "GO_BUILD_OK=0"; RC=1; fi
  FMT_DIFF="$(cd "$REPO" && gofmt -l \
    backend/handlers/site.go \
    backend/handlers/site_name_2477.go \
    backend/handlers/site_name_2477_test.go 2>/dev/null | wc -l | tr -d ' ')"
  echo "GOFMT_FILES=$FMT_DIFF"
  if [ "$FMT_DIFF" = "0" ]; then echo "GOFMT_DIFF=0"; else echo "GOFMT_DIFF=1"; RC=1; fi
  ;;
esac

exit "$RC"
