#!/usr/bin/env bash
# #2455 R5 fixture 测试：存量同商户网点重名清单脚本。
#
# 覆盖：
#   - 干净 fixture（3 行不同名 / 跨商户同名）→ DUP_GROUPS=0
#   - 重复 fixture（同商户 2 行「卡丹萨乐器」+ 跨商户 1 行同名）→ 恰好 1 组、2 个 id
#   - 只读：脚本源码不得出现任何写库语句（READONLY=1）
#
# 用法：bash scripts/tests/test_site_name_duplicates.sh
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/scripts/site-name-duplicates.sh"

[ -x "$SCRIPT" ] || { echo "FAIL: script not executable: $SCRIPT" >&2; exit 1; }

WORK="$(mktemp -d "${TMPDIR:-/tmp}/site-name-dup-2455-test.XXXXXX")" || exit 1
trap 'rm -rf "$WORK"' EXIT

FAIL=0

# 干净 fixture：同商户两行不同名 + 另一商户一行与原商户同名（跨商户同名合法）。
cat > "$WORK/clean.tsv" <<'EOF'
aaaaaaaa-1111-4111-8111-111111111111	海淀店	11111111-1111-4111-8111-111111111111
aaaaaaaa-1111-4111-8111-111111111111	卡丹萨西铁营	22222222-2222-4222-8222-222222222222
bbbbbbbb-1111-4111-8111-111111111111	卡丹萨乐器	33333333-3333-4333-8333-333333333333
EOF

# 重复 fixture：同商户 2 行「卡丹萨乐器」+ 另一商户 1 行同名（只算同商户那组）。
cat > "$WORK/dup.tsv" <<'EOF'
aaaaaaaa-1111-4111-8111-111111111111	卡丹萨乐器	11111111-1111-4111-8111-111111111111
aaaaaaaa-1111-4111-8111-111111111111	卡丹萨乐器	22222222-2222-4222-8222-222222222222
bbbbbbbb-1111-4111-8111-111111111111	卡丹萨乐器	33333333-3333-4333-8333-333333333333
EOF

# ---------- 干净 fixture ----------
CLEAN_OUT="$(bash "$SCRIPT" --report --fixture "$WORK/clean.tsv" 2>&1)"
CLEAN_RC=$?
echo "$CLEAN_OUT"
[ "$CLEAN_RC" = "0" ] || { echo "FAIL: clean fixture rc=$CLEAN_RC" >&2; FAIL=1; }
grep -Fq 'DUP_GROUPS=0' <<<"$CLEAN_OUT" || { echo "FAIL: clean fixture must report DUP_GROUPS=0" >&2; FAIL=1; }
grep -Fq 'READONLY=1' <<<"$CLEAN_OUT" || { echo "FAIL: READONLY != 1 (report must not write)" >&2; FAIL=1; }
grep -Fq 'DB_WRITES=0' <<<"$CLEAN_OUT" || { echo "FAIL: DB_WRITES != 0 in report mode" >&2; FAIL=1; }

# ---------- 重复 fixture ----------
DUP_OUT="$(bash "$SCRIPT" --report --fixture "$WORK/dup.tsv" 2>&1)"
DUP_RC=$?
echo "$DUP_OUT"
[ "$DUP_RC" = "0" ] || { echo "FAIL: dup fixture rc=$DUP_RC" >&2; FAIL=1; }
grep -Fq 'DUP_GROUPS=1' <<<"$DUP_OUT" || { echo "FAIL: dup fixture must report exactly 1 group" >&2; FAIL=1; }
DUP_IDS_COUNT=$(grep -F 'DUP tenant_id=' <<<"$DUP_OUT" | head -1 | sed -n 's/.*ids=\([^ ][^ ]*\) count=.*/\1/p' | tr ',' '\n' | grep -c .)
[ "$DUP_IDS_COUNT" = "2" ] || { echo "FAIL: dup group must list exactly 2 site ids (got $DUP_IDS_COUNT)" >&2; FAIL=1; }
grep -Fq 'count=2' <<<"$DUP_OUT" || { echo "FAIL: dup group count must be 2" >&2; FAIL=1; }
grep -Fq '11111111-1111-4111-8111-111111111111' <<<"$DUP_OUT" || { echo "FAIL: dup group must list the first id" >&2; FAIL=1; }
grep -Fq '22222222-2222-4222-8222-222222222222' <<<"$DUP_OUT" || { echo "FAIL: dup group must list the second id" >&2; FAIL=1; }
if grep -Fq 'bbbbbbbb-1111-4111-8111-111111111111' <<<"$DUP_OUT"; then
  echo "FAIL: cross-tenant same-name site must not be reported as a duplicate" >&2
  FAIL=1
fi

# ---------- 只读断言（源码级） ----------
WRITE_HITS=$(grep -icE '\b(UPDATE|DELETE|INSERT[[:space:]]+INTO|ALTER|CREATE[[:space:]]+(UNIQUE[[:space:]]+)?(INDEX|TABLE)|TRUNCATE|DROP[[:space:]]+(INDEX|TABLE))\b' "$SCRIPT" || true)
[ "$WRITE_HITS" = "0" ] || { echo "FAIL: script must not contain write statements (hits=$WRITE_HITS)" >&2; FAIL=1; }
grep -Fq 'ROLE="apply"\|--apply)' "$SCRIPT" && { echo "FAIL: script must not expose a write mode" >&2; FAIL=1; }

if [ "$FAIL" = "0" ]; then
  echo "CLEAN_DUP_GROUPS=0"
  echo "FIXTURE_DUP_GROUPS=1"
  echo "FIXTURE_IDS=2"
  echo "READONLY=1"
  echo "RESULT: PASS"
  exit 0
fi
echo "RESULT: FAIL"
exit 1
