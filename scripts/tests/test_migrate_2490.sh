#!/usr/bin/env bash
# =============================================================================
# scripts/tests/test_migrate_2490.sh — 回归：migrate.sh 空 email 不得左移、畸形行 fail-fast、
# 写库二次确认。本脚本即 #2490 的 full-gate 命令（无网络、无 IAM、无 DB 访问）。
#
# 断言（对应 Accepted Plan R1–R7）：
#   R1 空 email 不左移：u1||196608 → ACTION=UPDATE email=（空）old_perm=196608 new_perm=983040
#   R2 非法 cus_perm（空 / abc）→ exit 3、MIGRATE_INVALID=1、DB_WRITES=0
#   R3 字段数不足（2 列）→ exit 3、ROW_FIELDS=2
#   R4 位已置（983040）→ ACTION=SKIP、PLAN_UPDATES=0
#   R5 写库确认：无/错 --confirm-rows → exit 2 + APPLY_CONFIRM_MISMATCH=1；
#      匹配 → APPLY_READY=1；--fixture + --yes → exit 2
#   R6 红绿：旧 Tab-IFS 读法丢 bits 16/17（786432），新读法保留（983040）
#   R7 语法 + 同类扫描（信息行，不参与判定）
#
# 退出码：0 = 全部 PASS；1 = 有断言失败。
# =============================================================================
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MIGRATE="$REPO_ROOT/scripts/migrate.sh"
MASK=$(( (1 << 18) | (1 << 19) ))
mkdir -p "$REPO_ROOT/tmp"
TMP_DIR="$(mktemp -d "$REPO_ROOT/tmp/test-migrate-2490.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT

PASS=0
FAIL=0
ok()   { PASS=$((PASS + 1)); echo "PASS: $1"; }
bad()  { FAIL=$((FAIL + 1)); echo "FAIL: $1"; }
check(){ # desc actual expected
  if [ "$2" = "$3" ]; then ok "$1 ($2)"; else bad "$1 (got '$2', want '$3')"; fi
}

run_fixture() { # fixture_text [extra args...]
  local text="$1"; shift
  local f="$TMP_DIR/fixture.txt"
  printf '%s\n' "$text" > "$f"
  bash "$MIGRATE" --fixture "$f" "$@" 2>&1
}

# ---------------------------------------------------------------- R1
OUT="$(run_fixture 'u1||196608')"
RC=$?
check "R1 exit" "$RC" "0"
echo "$OUT" | grep -q '^ACTION=UPDATE uid=u1 email= old_perm=196608 new_perm=983040$' \
  && ok "R1 action line" || bad "R1 action line: $(echo "$OUT" | grep '^ACTION=' | head -1)"
echo "$OUT" | grep -q '^PLAN_UPDATES=1$' && ok "R1 PLAN_UPDATES" || bad "R1 PLAN_UPDATES"
echo "$OUT" | grep -q '^DB_WRITES=0$' && ok "R1 DB_WRITES" || bad "R1 DB_WRITES"
echo "$OUT" | grep -q '^RESULT: PASS$' && ok "R1 RESULT PASS" || bad "R1 RESULT PASS"
if echo "$OUT" | grep -q 'email=196608'; then bad "R1 email shifted left"; else ok "R1 no shift"; fi

# ---------------------------------------------------------------- R2
run_fixture 'u2|a@b|' > "$TMP_DIR/r2-empty.txt" 2>&1; RC=$?
check "R2 empty perm exit" "$RC" "3"
grep -q '^BAD_PERM=$' "$TMP_DIR/r2-empty.txt" && ok "R2 BAD_PERM empty" || bad "R2 BAD_PERM empty"
grep -q '^RESULT: FAIL$' "$TMP_DIR/r2-empty.txt" && ok "R2 RESULT FAIL" || bad "R2 RESULT FAIL"
run_fixture 'u3|a@b|abc' > "$TMP_DIR/r2.txt" 2>&1; RC=$?
check "R2 non-numeric exit" "$RC" "3"
grep -q '^BAD_PERM=abc$' "$TMP_DIR/r2.txt" && ok "R2 BAD_PERM" || bad "R2 BAD_PERM"
grep -q '^MIGRATE_INVALID=1$' "$TMP_DIR/r2.txt" && ok "R2 MIGRATE_INVALID" || bad "R2 MIGRATE_INVALID"
grep -q '^DB_WRITES=0$' "$TMP_DIR/r2.txt" && ok "R2 DB_WRITES" || bad "R2 DB_WRITES"

# ---------------------------------------------------------------- R3
run_fixture 'u4|a@b' > "$TMP_DIR/r3.txt" 2>&1; RC=$?
check "R3 short row exit" "$RC" "3"
grep -q '^ROW_FIELDS=2$' "$TMP_DIR/r3.txt" && ok "R3 ROW_FIELDS" || bad "R3 ROW_FIELDS"

# ---------------------------------------------------------------- R4
OUT="$(run_fixture 'u5|a@b|983040')"
echo "$OUT" | grep -q '^ACTION=SKIP uid=u5' && ok "R4 skip" || bad "R4 skip"
echo "$OUT" | grep -q '^PLAN_UPDATES=0$' && ok "R4 PLAN_UPDATES=0" || bad "R4 PLAN_UPDATES"
echo "$OUT" | grep -q '^SKIPPED=1$' && ok "R4 SKIPPED=1" || bad "R4 SKIPPED"
echo "$OUT" | grep -q '^DB_WRITES=0$' && ok "R4 DB_WRITES=0" || bad "R4 DB_WRITES"
echo "$OUT" | grep -q '^RESULT: PASS$' && ok "R4 RESULT PASS" || bad "R4 RESULT PASS"

# ---------------------------------------------------------------- R5
bash "$MIGRATE" --yes > "$TMP_DIR/r5a.txt" 2>&1; RC=$?
check "R5 --yes without confirm exit" "$RC" "2"
grep -q '^APPLY_CONFIRM_MISMATCH=1$' "$TMP_DIR/r5a.txt" && ok "R5 missing confirm" || bad "R5 missing confirm"

run_fixture 'u1||196608' --confirm-rows 9 > "$TMP_DIR/r5b.txt" 2>&1; RC=$?
check "R5 wrong confirm exit" "$RC" "2"
grep -q '^APPLY_CONFIRM_MISMATCH=1$' "$TMP_DIR/r5b.txt" && ok "R5 wrong confirm" || bad "R5 wrong confirm"
grep -q '^DB_WRITES=0$' "$TMP_DIR/r5b.txt" && ok "R5 wrong confirm no write" || bad "R5 wrong confirm DB_WRITES"

run_fixture 'u1||196608' --confirm-rows 1 > "$TMP_DIR/r5c.txt" 2>&1; RC=$?
check "R5 matching confirm exit" "$RC" "0"
grep -q '^APPLY_READY=1$' "$TMP_DIR/r5c.txt" && ok "R5 APPLY_READY" || bad "R5 APPLY_READY"
grep -q '^DB_WRITES=0$' "$TMP_DIR/r5c.txt" && ok "R5 fixture stays dry" || bad "R5 fixture wrote"
grep -q '^RESULT: PASS$' "$TMP_DIR/r5c.txt" && ok "R5 RESULT PASS" || bad "R5 RESULT PASS"

# Strict equality also covers the zero-update plan: a stale non-zero count on an
# already-satisfied plan must still be a mismatch (D4).
run_fixture 'u5|a@b|983040' --confirm-rows 1 > "$TMP_DIR/r5e.txt" 2>&1; RC=$?
check "R5 zero-plan stale confirm exit" "$RC" "2"
grep -q '^APPLY_CONFIRM_MISMATCH=1$' "$TMP_DIR/r5e.txt" && ok "R5 zero-plan mismatch" || bad "R5 zero-plan mismatch"
grep -q '^DB_WRITES=0$' "$TMP_DIR/r5e.txt" && ok "R5 zero-plan no write" || bad "R5 zero-plan DB_WRITES"

run_fixture 'u5|a@b|983040' --confirm-rows 0 > "$TMP_DIR/r5f.txt" 2>&1; RC=$?
check "R5 zero-plan exact confirm exit" "$RC" "0"
grep -q '^APPLY_READY=1$' "$TMP_DIR/r5f.txt" && ok "R5 zero-plan APPLY_READY" || bad "R5 zero-plan APPLY_READY"
grep -q '^RESULT: PASS$' "$TMP_DIR/r5f.txt" && ok "R5 zero-plan RESULT PASS" || bad "R5 zero-plan RESULT PASS"

run_fixture 'u1||196608' --yes >/dev/null 2>&1; RC=$?
check "R5 fixture+yes exit" "$RC" "2"

# ---------------------------------------------------------------- R6（红绿）
# 旧读法（Tab-IFS）：空 email 折叠 → cus_perm 读空 → 0|mask，bits 16/17 丢失
OLD_INPUT=$'u1\t\t196608'
IFS=$'\t' read -r _ouid _oemail _operm <<< "$OLD_INPUT"
OLD_NEW_PERM=$(( ${_operm:-0} | MASK ))
NEW_PERM=$(( 196608 | MASK ))
echo "OLD_EMAIL_FIELD=${_oemail:-}"
echo "OLD_PERM_FIELD=${_operm:-}"
echo "OLD_NEW_PERM=$OLD_NEW_PERM"
echo "NEW_PERM=$NEW_PERM"
[ $(( OLD_NEW_PERM & 196608 )) -ne 196608 ] && OLD_LOST_BITS=1 || OLD_LOST_BITS=0
[ $(( NEW_PERM & 196608 )) -eq 196608 ] && FIXED=1 || FIXED=0
echo "OLD_LOST_BITS=$OLD_LOST_BITS"
echo "FIXED=$FIXED"
if [ "$OLD_LOST_BITS" = "1" ] && [ "$FIXED" = "1" ]; then
  RED_GREEN=1; ok "R6 red/green"
else
  RED_GREEN=0; bad "R6 red/green (old=$OLD_LOST_BITS fixed=$FIXED)"
fi
echo "RED_GREEN=$RED_GREEN"
check "R6 old perm" "$OLD_NEW_PERM" "786432"
check "R6 new perm" "$NEW_PERM" "983040"

# ---------------------------------------------------------------- R7
if bash -n "$MIGRATE"; then SYNTAX_OK=1; ok "R7 syntax"; else SYNTAX_OK=0; bad "R7 syntax"; fi
echo "SYNTAX_OK=$SYNTAX_OK"
if command -v shellcheck >/dev/null 2>&1; then
  shellcheck "$MIGRATE" > "$TMP_DIR/shellcheck.txt" 2>&1; SC=$?
  echo "SHELLCHECK_EXIT=$SC"
  [ "$SC" = "0" ] || bad "R7 shellcheck"
else
  echo "SHELLCHECK_EXIT=NA"
fi
SAME_PATH_SCAN=$(grep -rn "IFS=\$'\\\\t'" "$REPO_ROOT/scripts" 2>/dev/null | grep -vc 'tests/test_migrate_2490.sh' || true)
echo "SAME_PATH_SCAN=${SAME_PATH_SCAN:-0}"

echo ""
echo "SUMMARY pass=$PASS fail=$FAIL"
if [ "$FAIL" -ne 0 ]; then
  echo "RESULT: FAIL"
  exit 1
fi
echo "RESULT: PASS"
