#!/usr/bin/env bash
# #2462 R7 fixture 测试：存量漂移清单脚本（只读默认 + 未授权 --apply 必须 exit 2）。
#
# 用法：bash scripts/tests/test_merchant_direct_role_drift_2462.sh
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/scripts/report-merchant-direct-role-drift-2462.sh"

[ -x "$SCRIPT" ] || { echo "FAIL: script not executable: $SCRIPT" >&2; exit 1; }

WORK="$(mktemp -d "${TMPDIR:-/tmp}/drift-2462-test.XXXXXX")" || exit 1
trap 'rm -rf "$WORK"' EXIT

# 3 行 fixture：2 行漂移（mm=site_member 且无 site_members / mm=site_member 无网点行）
# + 1 行正常（mm=merchant_staff）。
cat > "$WORK/fixture.tsv" <<'EOF'
11111111-1111-4111-8111-111111111111	site_member	site_member	aaaaaaaa-1111-4111-8111-111111111111	0
22222222-2222-4222-8222-222222222222	site_member	site_member	aaaaaaaa-1111-4111-8111-111111111111	0
33333333-3333-4333-8333-333333333333	merchant_staff	merchant_staff	aaaaaaaa-1111-4111-8111-111111111111	0
EOF

FAIL=0

REPORT_OUT="$(bash "$SCRIPT" --fixture "$WORK/fixture.tsv" 2>&1)"
REPORT_RC=$?
echo "$REPORT_OUT"
[ "$REPORT_RC" = "0" ] || { echo "FAIL: report rc=$REPORT_RC" >&2; FAIL=1; }
grep -Fq 'ALIGN_ROWS=2' <<<"$REPORT_OUT" || { echo "FAIL: ALIGN_ROWS != 2" >&2; FAIL=1; }
grep -Fq 'NONE_ROWS=1' <<<"$REPORT_OUT" || { echo "FAIL: NONE_ROWS != 1" >&2; FAIL=1; }
grep -Fq 'READONLY=1' <<<"$REPORT_OUT" || { echo "FAIL: READONLY != 1 (report must not write)" >&2; FAIL=1; }
grep -Fq 'DB_WRITES=0' <<<"$REPORT_OUT" || { echo "FAIL: DB_WRITES != 0 in report mode" >&2; FAIL=1; }

# 未授权 --apply：必须 exit 2 且不写库
APPLY_OUT="$(bash "$SCRIPT" --apply --fixture "$WORK/fixture.tsv" 2>&1)"
APPLY_RC=$?
echo "$APPLY_OUT"
[ "$APPLY_RC" = "2" ] || { echo "FAIL: unauthorized apply rc=$APPLY_RC (want 2)" >&2; FAIL=1; }
grep -Fq 'UNAUTHORIZED_APPLY=1' <<<"$APPLY_OUT" || { echo "FAIL: missing UNAUTHORIZED_APPLY=1" >&2; FAIL=1; }
grep -Fq 'DB_WRITES=0' <<<"$APPLY_OUT" || { echo "FAIL: unauthorized apply attempted writes" >&2; FAIL=1; }
if grep -Fq 'READONLY=0' <<<"$APPLY_OUT"; then
  echo "FAIL: unauthorized apply must stay read-only" >&2
  FAIL=1
fi

if [ "$FAIL" = "0" ]; then
  echo "ALIGN_ROWS=2"
  echo "NONE_ROWS=1"
  echo "READONLY=1"
  echo "UNAUTHORIZED_APPLY_EXIT=${APPLY_RC}"
  echo "DB_WRITES=0"
  echo "RESULT: PASS"
  exit 0
fi
echo "RESULT: FAIL"
exit 1
