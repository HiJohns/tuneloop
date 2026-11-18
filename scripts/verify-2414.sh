#!/usr/bin/env bash
# #2414：POST /api/user/rental/calculate 押金与金额单位（#2406 规则 + 复审 F1/A）验收。
# 用法：bash scripts/verify-2414.sh <r1|r2|r3|r4|r5|r6|r7>
# 每个模式把带 provenance 头的输出写入 docs/evidence/2414/ 对应产物。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1) ART="docs/evidence/2414/R1-zero-fallback.txt"; TEST='^TestCalculateRentalDeposit_2406Rule_2414$' ;;
  r2) ART="docs/evidence/2414/R2-instrument.txt"; TEST='^TestCalculateRentalDepositInstrumentOverride_2414$' ;;
  r3) ART="docs/evidence/2414/R3-true-zero.txt"; TEST='^TestCalculateRentalDepositTrueZero_2414$' ;;
  r4) ART="docs/evidence/2414/R4-build.txt"; TEST="" ;;
  r5) ART="docs/evidence/2414/R5-units.txt"; TEST='^TestCalculateRentalUnitConsistency_2414$' ;;
  r6) ART="docs/evidence/2414/R6-doc.txt"; TEST="" ;;
  r7) ART="docs/evidence/2414/R7-callers.txt"; TEST="" ;;
  *) echo "usage: $0 <r1|r2|r3|r4|r5|r6|r7>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

if ! command -v go >/dev/null 2>&1; then
  export PATH="/usr/local/go/bin:${PATH}"
fi
# 构建临时目录固定在仓库 tmp/（系统 /tmp 可能是容量受限的 tmpfs）
mkdir -p "$REPO_ROOT/tmp/gobuild" "$(dirname "$ART")"
export TMPDIR="$REPO_ROOT/tmp/gobuild"
export TUNELOOP_DB_SIMPLE_PROTOCOL="${TUNELOOP_DB_SIMPLE_PROTOCOL:-1}"

BUILD="1.0.$(git rev-parse --short HEAD)"
COMMIT="$(git rev-parse HEAD)"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
  DIRTY=true
fi

run_checks() {
  case "$MODE" in
    r4)
      ( cd backend && go build ./... )
      ;;
    r6)
      # 文档 §8.12：金额字段单位契约 + 一致性约束必须与实现逐字段一致
      SECTION="$(awk '/^### 8\.12 /{f=1} f&&/^### 8\.13 /{exit} f' docs/spec/api/api.md)"
      [ -n "$SECTION" ] || { echo "§8.12 section missing"; return 1; }
      for f in tiers subtotal total_rent member_discount_amount deposit shipping_fee cash_to_pay; do
        printf '%s\n' "$SECTION" | grep -q "$f" || { echo "missing field in §8.12: $f"; return 1; }
      done
      printf '%s\n' "$SECTION" | grep -q '为\*\*分\*\*' || { echo "missing 分 unit contract"; return 1; }
      printf '%s\n' "$SECTION" | grep -q 'cash_to_pay = total_rent + deposit + shipping_fee' || { echo "missing sum invariant"; return 1; }
      printf '%s\n' "$SECTION" | grep -q 'sum(tiers\[\].subtotal) = total_rent' || { echo "missing tiers invariant"; return 1; }
      echo "R6 PASS：§8.12 字段/单位/一致性约束齐全"
      ;;
    r7)
      echo "# 全仓 /rental/calculate 调用方清单（复审 R7）"
      grep -rn "rental/calculate" backend frontend-pc frontend-mobile docs --include='*.go' --include='*.js' --include='*.jsx' --include='*.ts' --include='*.tsx' --include='*.md' 2>/dev/null || true
      echo "# 说明：仓内无前端调用方；仅后端路由注册 + 本 Issue 测试/脚本/证据引用。公开路由保留。"
      ;;
    *)
      ( cd backend && go test ./handlers/ -run "$TEST" -count=1 -v )
      ;;
  esac
}

{
  echo "# build=$BUILD"
  echo "# commit=$COMMIT"
  echo "# dirty=$DIRTY"
  run_checks
} 2>&1 | tee "$ART"

exit "${PIPESTATUS[0]}"
