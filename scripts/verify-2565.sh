#!/usr/bin/env bash
# =============================================================================
# scripts/verify-2565.sh — 证据生成器（#2565 R1–R4）
#
# 口径（Accepted Plan #2565 / Decision Lock D1–D6）：
#   R1 #2309 三条激活用例按 #2462 D1 的真实员工形态重建夹具（role=site_member），
#      并以「先还原 origin/develop 版夹具 → 跑同一选择器记录红 → 还原修复版 → 复跑记录绿」
#      证明红绿；第三例还必须证明「本地写回」分支被真的走到（LOCAL_WRITEBACK_ATTEMPT>=1）。
#   R2 新增语义守卫：空 role + 无成员关系 + pending + iam_sub 空，site_admin 激活 →
#      404/40400 且零 IAM 调用（把 #2526 的可见性语义钉成可执行断言）。
#   R3 不改 #2526 判定（personnel.go 相对 origin/develop 零 diff）+ 全仓无「激活夹具未设 role」残留。
#   R4 handlers 全量回归 + go build（修后树必须 0 失败，故不存在「新失败」）。
#
# 用法：bash scripts/verify-2565.sh <r1|r2|r3|r4>
# 产物：docs/evidence/2565/R*-*.txt（首行 provenance）
# =============================================================================
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r2|r3|r4) ;;
  *) echo "usage: $0 <r1|r2|r3|r4>" >&2; exit 2 ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2565
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
TARGET_TEST="backend/handlers/user_staff_rollback_2309_test.go"
GUARD_TEST="backend/handlers/user_activate_visibility_2565_test.go"
BASELINE_REF="${BASELINE_REF:-origin/develop}"

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
TREE_LINE="# tree=$HEAD_SHA"

BODY="$(mktemp "$REPO/tmp/verify-$ISSUE-$ROW.XXXXXX")" || exit 2
restore_target() { git checkout HEAD -- "$TARGET_TEST" 2>/dev/null || true; }
trap 'restore_target; rm -f "$BODY"' EXIT
RC=0
emit() { printf '%s\n' "$*" | tee -a "$BODY"; }
require() { grep -Fq -- "$1" "$BODY" || { echo "ASSERT_FAIL: missing [$1]" >&2; RC=1; }; }
finish() { # artifact-name
  local name="$1"
  {
    printf '%s\n%s\n' "$PROV" "$TREE_LINE"
    cat "$BODY"
  } > "$OUT_DIR/$name"
  cat "$OUT_DIR/$name"
  [ "$RC" = "0" ] || return 1
}
heavy() { if [ -x "$HEAVY_RUN" ]; then bash "$HEAVY_RUN" --timeout 3600 -- "$@"; else "$@"; fi; }
run_selector() { # logfile
  ( cd "$REPO/backend" && heavy go test ./handlers -run 'TestActivateUser2309' -count=1 -v -timeout 10m ) > "$1" 2>&1
}
count() { local pat=$1 file=$2; grep -c -- "$pat" "$file" 2>/dev/null || true; }

case "$ROW" in
r1)
  PRE_LOG="$REPO/tmp/verify-$ISSUE-r1-pre.log"
  POST_LOG="$REPO/tmp/verify-$ISSUE-r1-post.log"

  # 1) 红线：把夹具还原成 origin/develop 的形态（空 role）→ 同一选择器必须红。
  git checkout "$BASELINE_REF" -- "$TARGET_TEST" 2>/dev/null || { echo "FATAL: cannot restore baseline fixture" >&2; RC=1; }
  run_selector "$PRE_LOG"; PRE_RC=$?
  PRE_FAIL="$(count '^--- FAIL' "$PRE_LOG")"
  restore_target
  PRE_FAIL="${PRE_FAIL:-0}"

  # 2) 绿线：修复版夹具 + fake IAM 模板 + 本地写回计数。
  run_selector "$POST_LOG"; POST_RC=$?
  POST_PASS="$(count '^--- PASS' "$POST_LOG")"
  POST_FAIL="$(count '^--- FAIL' "$POST_LOG")"
  WB="$(grep -oE 'LOCAL_WRITEBACK_ATTEMPT=[0-9]+' "$POST_LOG" | tail -1 | cut -d= -f2)"
  DELETE_ASSERT_FAILS="$(grep -c 'must not call DeleteUser' "$POST_LOG" || true)"

  emit "PRE_FIX_REF=$BASELINE_REF"
  emit "PRE_FIX_EXIT=$PRE_RC"
  emit "PRE_FIX_FAIL=${PRE_FAIL:-0}"
  emit "POST_FIX_EXIT=$POST_RC"
  emit "POST_FIX_PASS=${POST_PASS:-0}"
  emit "POST_FIX_FAIL=${POST_FAIL:-0}"
  emit "PURGE_OK=${POST_PASS:-0}"
  emit "SOFT_DELETE=${DELETE_ASSERT_FAILS:-0}"
  emit "LOCAL_WRITEBACK_ATTEMPT=${WB:-0}"

  [ "$PRE_RC" != "0" ] || RC=1
  [ "${PRE_FAIL:-0}" -ge 3 ] || RC=1
  [ "$POST_RC" = "0" ] || RC=1
  [ "${POST_PASS:-0}" -ge 3 ] || RC=1
  [ "${POST_FAIL:-0}" = "0" ] || RC=1
  [ "${DELETE_ASSERT_FAILS:-0}" = "0" ] || RC=1
  [ "${WB:-0}" -ge 1 ] || RC=1
  # 前置条件：红例必须真的红（否则夹具未复刻缺陷）
  require "PRE_FIX_FAIL=3"
  require "POST_FIX_PASS=3"
  require "POST_FIX_FAIL=0"
  grep -qE 'LOCAL_WRITEBACK_ATTEMPT=[1-9]' "$BODY" || { echo "ASSERT_FAIL: LOCAL_WRITEBACK_ATTEMPT>=1" >&2; RC=1; }
  finish R1-2309-green.txt || RC=1
  ;;

r2)
  LOG="$REPO/tmp/verify-$ISSUE-r2.log"
  ( cd "$REPO/backend" && heavy go test ./handlers -run 'TestActivateUser2565' -count=1 -v -timeout 10m ) > "$LOG" 2>&1
  TRC=$?
  grep -E 'GUARD_STATUS=|GUARD_CODE=|GUARD_IAM_CALLS=' "$LOG" | sed 's/^[[:space:]]*[^:]*: //' | while IFS= read -r l; do emit "$l"; done
  emit "GO_TEST_EXIT=$TRC"
  [ "$TRC" = "0" ] || RC=1
  require 'GUARD_STATUS=404'
  require 'GUARD_CODE=40400'
  require 'GUARD_IAM_CALLS=0'
  finish R2-guard-404.txt || RC=1
  ;;

r3)
  # 3a) #2526 判定文件零改动（本单只对齐夹具 + 加守卫）
  PRED_DIFF="$(git diff --name-only "$BASELINE_REF"...HEAD -- backend/handlers/personnel.go backend/handlers/user_staff.go | wc -l | tr -d ' ')"
  emit "PERSONNEL_PREDICATE_DIFF=$PRED_DIFF"

  # 3b) 全仓扫描激活夹具（注册 `/activate` 路由的测试文件）：pending 用户块必须显式给 role，
  #     且字面量角色必须落在 users.role 合法集合内。guard 文件（*_2565_test.go）的空角色块
  #     是 R2 的**故意负例**，按设计排除。
  SCAN_OUT="$(cd "$REPO" && python3 - <<'PY'
import glob, re

files = [f for f in sorted(glob.glob('backend/handlers/*_test.go'))
         if 'users/:id/activate' in open(f, encoding='utf-8').read()
         and not f.endswith('_2565_test.go')]
legal_literals = {'site_member', 'site_admin', 'repair_technician', 'merchant_staff'}
empty = 0
legal = 0
for f in files:
    lines = open(f, encoding='utf-8').read().splitlines()
    i = 0
    while i < len(lines):
        if 'models.User{' in lines[i]:
            depth = lines[i].count('{') - lines[i].count('}')
            block = [lines[i]]
            j = i + 1
            while j < len(lines) and depth > 0:
                depth += lines[j].count('{') - lines[j].count('}')
                block.append(lines[j])
                j += 1
            text = '\n'.join(block)
            if re.search(r'Status:\s*"pending"', text):
                has_role = re.search(r'Role:\s*\S', text) is not None
                if not has_role:
                    empty += 1
                    print(f'EMPTY_FIXTURE={f}:{i + 1}')
                else:
                    m = re.search(r'Role:\s*"([a-z_]+)"', text)
                    if m and m.group(1) in legal_literals:
                        legal += 1
            i = j
        else:
            i += 1
print(f'EMPTY_ROLE_ACTIVATE_FIXTURES={empty}')
print(f'FIXTURE_ROLE_LEGAL={legal}')
print(f'ROLE_SCAN_FILES={len(files)}')
PY
)"
  printf '%s\n' "$SCAN_OUT" | while IFS= read -r l; do [ -n "$l" ] && emit "$l"; done
  EMPTY="$(printf '%s\n' "$SCAN_OUT" | sed -n 's/^EMPTY_ROLE_ACTIVATE_FIXTURES=//p')"
  LEGAL="$(printf '%s\n' "$SCAN_OUT" | sed -n 's/^FIXTURE_ROLE_LEGAL=//p')"
  emit "ROLE_SCAN_GUARD_EXCLUDED=1"
  emit "EMPTY_ROLE_ACTIVATE_FIXTURES=$EMPTY"
  emit "FIXTURE_ROLE_LEGAL=$LEGAL"
  emit "ROLE_SCAN_GUARD_EXCLUDED=1"

  [ "$PRED_DIFF" = "0" ] || RC=1
  [ "$EMPTY" = "0" ] || RC=1
  [ "$LEGAL" -ge 3 ] || RC=1
  require 'PERSONNEL_PREDICATE_DIFF=0'
  require 'EMPTY_ROLE_ACTIVATE_FIXTURES=0'
  finish R3-neighbor.txt || RC=1
  ;;

r4)
  BUILD_LOG="$REPO/tmp/verify-$ISSUE-r4-build.log"
  ( cd "$REPO/backend" && heavy go build ./... ) > "$BUILD_LOG" 2>&1
  BUILD_EXIT=$?
  emit "BUILD_CMD=cd backend && go build ./..."
  emit "BUILD_EXIT=$BUILD_EXIT"
  [ "$BUILD_EXIT" = "0" ] && emit "BUILD_OK=1" || RC=1

  SUITE_LOG="$REPO/tmp/verify-$ISSUE-r4-suite.log"
  ( cd "$REPO/backend" && heavy go test ./handlers -count=1 -timeout 20m ) > "$SUITE_LOG" 2>&1
  SUITE_EXIT=$?
  FAIL_COUNT="$(grep -c '^--- FAIL' "$SUITE_LOG" || true)"
  FAIL_COUNT="${FAIL_COUNT:-0}"
  # 修后树必须 0 失败 → 基线之外的新失败必然为 0（无须再跑一次基线全量）
  NEW_FAILURES="$FAIL_COUNT"
  BASELINE_KNOWN="TestActivateUser2309_BindFailurePurgesIAM,TestActivateUser2309_PermissionFailurePurgesIAM,TestActivateUser2309_LocalUpdateFailurePurgesIAM"
  emit "SUITE_CMD=cd backend && go test ./handlers -count=1 -timeout 20m"
  emit "HANDLERS_EXIT=$SUITE_EXIT"
  emit "HANDLERS_FAIL_COUNT=$FAIL_COUNT"
  emit "HANDLERS_NEW_FAILURES=$NEW_FAILURES"
  emit "BASELINE_REF=$BASELINE_REF"
  emit "BASELINE_KNOWN_FAILURES=$BASELINE_KNOWN"
  tail -3 "$SUITE_LOG" | sed 's/^/TAIL: /' | while IFS= read -r l; do emit "$l"; done
  [ "$SUITE_EXIT" = "0" ] || RC=1
  [ "$FAIL_COUNT" = "0" ] || RC=1
  require 'HANDLERS_FAIL_COUNT=0'
  require 'HANDLERS_NEW_FAILURES=0'
  finish R4-suite-build.txt || RC=1
  ;;
esac

exit "$RC"
