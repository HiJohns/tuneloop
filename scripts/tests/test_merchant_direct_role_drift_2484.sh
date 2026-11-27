#!/usr/bin/env bash
# #2484 回归测试：漂移清单脚本的空字段 / 畸形行 / --apply 二次确认。
#
# 运行：bash scripts/tests/test_merchant_direct_role_drift_2484.sh
#
# 覆盖（对应 Accepted Plan R1–R5）：
#   R1 空 users_role + has_site_member=1 → ACTION=none（旧 Tab 版会左移成 ACTION=align）
#   R3 字段数 ≠ 5 或 has_site_member 非 0|1 → FIXTURE_INVALID=1 / exit 3 / 不写库
#   R4 --apply 未给或不匹配 --confirm-rows → exit 2 / DB_WRITES=0；匹配 → APPLY_READY=1
#   R5 真实形态快照（5 漂移 + 4 双归属）→ ALIGN_ROWS=5 / NONE_ROWS=4
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/scripts/report-merchant-direct-role-drift-2462.sh"

[ -x "$SCRIPT" ] || { echo "FAIL: script not executable: $SCRIPT" >&2; exit 1; }

WORK="$(mktemp -d "${TMPDIR:-/tmp}/drift-2484-test.XXXXXX")" || exit 1
trap 'rm -rf "$WORK"' EXIT

FAIL=0
fail() { echo "FAIL: $*" >&2; FAIL=1; }

# --- R1：空 users_role（存量 users.role=''）+ has_site_member=1 → ACTION=none ----
cat > "$WORK/empty-users-role.psv" <<'EOF'
33333333-3333-4333-8333-333333333333|site_member||aaaaaaaa-1111-4111-8111-111111111111|1
EOF
OUT="$(bash "$SCRIPT" --fixture "$WORK/empty-users-role.psv" 2>&1)"; RC=$?
echo "$OUT"
[ "$RC" = "0" ] || fail "R1 rc=$RC (want 0)"
grep -Fq 'ACTION=none user_id=33333333-3333-4333-8333-333333333333' <<<"$OUT" || fail "R1: 空 users_role 行必须 ACTION=none"
grep -Fq 'merchant_id=aaaaaaaa-1111-4111-8111-111111111111' <<<"$OUT" || fail "R1: merchant_id 字段左移"
grep -Fq 'ALIGN_ROWS=0' <<<"$OUT" || fail "R1: ALIGN_ROWS 应为 0（伪阳性）"
grep -Fq 'NONE_ROWS=1' <<<"$OUT" || fail "R1: NONE_ROWS 应为 1"

# --- R1b：空 merchant_id 仍保持字段对齐 --------------------------------------
cat > "$WORK/empty-merchant.psv" <<'EOF'
44444444-4444-4444-8444-444444444444|site_member|site_member||0
EOF
OUT="$(bash "$SCRIPT" --fixture "$WORK/empty-merchant.psv" 2>&1)"; RC=$?
echo "$OUT"
[ "$RC" = "0" ] || fail "R1b rc=$RC (want 0)"
grep -Fq 'ACTION=align user_id=44444444-4444-4444-8444-444444444444' <<<"$OUT" || fail "R1b: 应识别为 align"
grep -Fq 'ALIGN_ROWS=1' <<<"$OUT" || fail "R1b: ALIGN_ROWS 应为 1"

# --- R3：字段数 ≠ 5 → exit 3 + FIXTURE_INVALID ------------------------------
cat > "$WORK/short-row.psv" <<'EOF'
55555555-5555-4555-8555-555555555555|site_member|site_member|cccccccc-1111-4111-8111-111111111111
EOF
OUT="$(bash "$SCRIPT" --fixture "$WORK/short-row.psv" 2>&1)"; RC=$?
echo "$OUT"
[ "$RC" = "3" ] || fail "R3 rc=$RC (want 3)"
grep -Fq 'FIXTURE_INVALID=1' <<<"$OUT" || fail "R3: missing FIXTURE_INVALID=1"
grep -Fq 'BAD_ROW=1' <<<"$OUT" || fail "R3: missing BAD_ROW=1"
grep -Fq 'DB_WRITES=0' <<<"$OUT" || fail "R3: 畸形行不得写库"

# --- R3b：has_site_member 非法枚举 → exit 3 ---------------------------------
cat > "$WORK/bad-enum.psv" <<'EOF'
66666666-6666-4666-8666-666666666666|site_member|site_member|cccccccc-1111-4111-8111-111111111111|2
EOF
OUT="$(bash "$SCRIPT" --fixture "$WORK/bad-enum.psv" 2>&1)"; RC=$?
echo "$OUT"
[ "$RC" = "3" ] || fail "R3b rc=$RC (want 3)"
grep -Fq 'FIXTURE_INVALID=1' <<<"$OUT" || fail "R3b: missing FIXTURE_INVALID=1"

# --- R4：--apply 未给 --confirm-rows → exit 2 + 不写库 ----------------------
cat > "$WORK/one-align.psv" <<'EOF'
77777777-7777-4777-8777-777777777777|site_member|site_member|cccccccc-1111-4111-8111-111111111111|0
EOF
OUT="$(OWNER_APPROVED_MERCHANT_DIRECT_ROLE=1 bash "$SCRIPT" --apply --fixture "$WORK/one-align.psv" 2>&1)"; RC=$?
echo "$OUT"
[ "$RC" = "2" ] || fail "R4 rc=$RC (want 2)"
grep -Fq 'APPLY_CONFIRM_MISMATCH=1' <<<"$OUT" || fail "R4: missing APPLY_CONFIRM_MISMATCH=1"
grep -Fq 'DB_WRITES=0' <<<"$OUT" || fail "R4: 未确认不得写库"

# --- R4b：--confirm-rows 不匹配 → exit 2 ------------------------------------
OUT="$(OWNER_APPROVED_MERCHANT_DIRECT_ROLE=1 bash "$SCRIPT" --apply --confirm-rows 0 --fixture "$WORK/one-align.psv" 2>&1)"; RC=$?
echo "$OUT"
[ "$RC" = "2" ] || fail "R4b rc=$RC (want 2)"
grep -Fq 'APPLY_CONFIRM_MISMATCH=1' <<<"$OUT" || fail "R4b: missing APPLY_CONFIRM_MISMATCH=1"

# --- R4c：--confirm-rows 匹配 → APPLY_READY=1（本机无 psql 时写库失败是允许的） --
OUT="$(OWNER_APPROVED_MERCHANT_DIRECT_ROLE=1 bash "$SCRIPT" --apply --confirm-rows 1 --fixture "$WORK/one-align.psv" 2>&1)"; RC=$?
echo "$OUT"
grep -Fq 'APPLY_READY=1' <<<"$OUT" || fail "R4c: missing APPLY_READY=1"

# --- R5：真实形态快照（5 漂移 + 4 双归属，含空 users_role）→ 5/4 ------------
cat > "$WORK/snapshot.psv" <<'EOF'
10101010-0000-4000-8000-000000000001|site_member||cccccccc-1111-4111-8111-111111111111|0
10101010-0000-4000-8000-000000000002|site_member|site_member|cccccccc-1111-4111-8111-111111111111|0
10101010-0000-4000-8000-000000000003|site_member||cccccccc-1111-4111-8111-111111111111|0
10101010-0000-4000-8000-000000000004|site_member|site_member||0
10101010-0000-4000-8000-000000000005|site_member||cccccccc-1111-4111-8111-111111111111|0
10101010-0000-4000-8000-000000000006|site_member||cccccccc-1111-4111-8111-111111111111|1
10101010-0000-4000-8000-000000000007|site_member|site_member|cccccccc-1111-4111-8111-111111111111|1
10101010-0000-4000-8000-000000000008|site_member|||1
10101010-0000-4000-8000-000000000009|merchant_staff|merchant_staff|cccccccc-1111-4111-8111-111111111111|0
EOF
OUT="$(bash "$SCRIPT" --fixture "$WORK/snapshot.psv" 2>&1)"; RC=$?
echo "$OUT"
[ "$RC" = "0" ] || fail "R5 rc=$RC (want 0)"
grep -Fq 'ALIGN_ROWS=5' <<<"$OUT" || fail "R5: ALIGN_ROWS 应为 5（伪阳性必须为 0）"
grep -Fq 'NONE_ROWS=4' <<<"$OUT" || fail "R5: NONE_ROWS 应为 4"
grep -Fq 'READONLY=1' <<<"$OUT" || fail "R5: report 模式必须只读"
grep -Fq 'DB_WRITES=0' <<<"$OUT" || fail "R5: report 模式不得写库"

echo "TESTS_FAIL=${FAIL}"
[ "$FAIL" = "0" ] || { echo "RESULT: FAIL"; exit 1; }
echo "SKIPPED=0"
echo "RESULT: PASS"
