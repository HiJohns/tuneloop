#!/bin/bash
# static-verify.sh — Phase 1 static flow verification (#1563)
#
# Verifies frontend↔backend consistency for a business flow WITHOUT running
# the app: pages registered, components exist, visibility gates satisfiable,
# API params match backend struct tags, response fields exist, navigation
# targets registered.
#
# Usage:
#   bash scripts/static-verify.sh settlement
#   bash scripts/static-verify.sh rent
#
# Exit code: 0 = all checks passed, 1 = at least one failure.

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WEAPP="$REPO_ROOT/frontend-mobile/src/pages-weapp"
H5="$REPO_ROOT/frontend-mobile/src/pages"
BACKEND="$REPO_ROOT/backend"

RED='\033[0;31m'; GREEN='\033[0;32m'; NC='\033[0m'
PASS=0; FAIL=0

pass() { echo -e "  ${GREEN}✅ $1${NC}"; ((PASS++)); }
fail() { echo -e "  ${RED}❌ $1${NC}"; ((FAIL++)); }

check() { # check <desc> <file> <pattern>
  local desc="$1" file="$2" pattern="$3"
  if [ -f "$file" ] && grep -qE "$pattern" "$file" 2>/dev/null; then
    pass "$desc"
  else
    fail "$desc ($file missing pattern: $pattern)"
  fi
}

check_in_routes() { # page registered in app.config.ts
  local page="$1" desc="$2"
  if grep -q "'$page'" "$REPO_ROOT/frontend-mobile/src/app.config.ts"; then
    pass "$desc — $page 已注册"
  else
    fail "$desc — $page 未注册（app.config.ts 缺失）"
  fi
}

verify_settlement() {
  echo "=== 静态流程验证: Settlement（退租结算） ==="

  # ── Step 0: 顾客入口页面 ──────────────────────────────
  check_in_routes "pages-weapp/my-leases/index" "我的租赁页（顾客入口）"
  check_in_routes "pages-weapp/order-detail/index" "订单详情页"
  check_in_routes "pages-weapp/return-settlement/index" "归还结算页"

  # ── Step 1: 顾客发起归还 ──────────────────────────────
  check "MyLeases 列表有归还按钮" "$WEAPP/MyLeases.jsx" "归还乐器|return-confirm|return_confirm"
  check_in_routes "pages-weapp/return-confirm/index" "归还确认页"
  check "OrderDetail 调用 return API" "$WEAPP/order-detail/OrderDetail.jsx" "orders/.*/return|apiFetch.*return"

  # ── Step 3: 员工验收 → PUT /warehouse/orders/:id/return-inspect ──
  check "后端注册 return-inspect 路由" "$BACKEND/handlers/warehouse.go" "return-inspect"
  check "InspectReturn handler 存在" "$BACKEND/handlers/warehouse.go" "func .*InspectReturn"
  check "验收入参含 condition" "$BACKEND/handlers/warehouse.go" '"condition".*binding:"required"'
  check "验收入参含 scan_time" "$BACKEND/handlers/warehouse.go" '"scan_time".*binding:"required"'
  check "验收入参含 damage_amount" "$BACKEND/handlers/warehouse.go" '"damage_amount"'

  # ── Step 4: 结算计算 ──────────────────────────────────
  check "computeSettlement 存在" "$BACKEND/handlers/user_settlement.go" "func computeSettlement"
  check "结算含 cash_refundable" "$BACKEND/handlers/user_settlement.go" '"cash_refundable"'
  check "结算含 actual_rent_days" "$BACKEND/handlers/user_settlement.go" '"actual_rent_days"'
  check "结算含 overdue_fee" "$BACKEND/handlers/user_settlement.go" '"overdue_fee"'

  # ── Step 5: 退款记录 ──────────────────────────────────
  check "executeRefund 存在" "$BACKEND/handlers/user_settlement.go" "func executeRefund"
  check "退款记录模型存在" "$BACKEND/models/models.go" "type OrderRefundRecord"
  check "押金退款标记" "$BACKEND/handlers/user_settlement.go" "deposit_refunded"

  # ── Step 6: 订单状态机 ────────────────────────────────
  check "completed 状态常量" "$BACKEND/models/models.go" "OrderStatusCompleted"
  check "returning 状态常量" "$BACKEND/models/models.go" "OrderStatusReturning"

  # ── Step 7: 前端展示结算结果 ──────────────────────────
  check "订单详情展示退款金额" "$WEAPP/order-detail/OrderDetail.jsx" "cash_refundable|refund_amount|退款"
  check "订单详情展示结算明细" "$WEAPP/order-detail/OrderDetail.jsx" "settlement|actual_rent|结算"
}

verify_rent() {
  echo "=== 静态流程验证: Rent（租赁下单） ==="
  check_in_routes "pages-weapp/home/index" "首页"
  check_in_routes "pages-weapp/detail/index" "乐器详情页"
  check_in_routes "pages-weapp/checkout/index" "结算页"

  check "首页卡片跳详情" "$WEAPP/Home.jsx" "pages-weapp/detail/index"
  check "详情页跳结算" "$WEAPP/Detail.jsx" "pages-weapp/checkout/index"
  check "CreateOrder 接收 instrument_id" "$BACKEND/handlers/user_rental.go" '"instrument_id".*binding:"required"'
  check "CreateOrder 接收 start_date" "$BACKEND/handlers/user_rental.go" '"start_date".*binding:"required"'
  check "CreateOrder 接收 end_date" "$BACKEND/handlers/user_rental.go" '"end_date".*binding:"required"'
  check "CreateOrder 支持免押金" "$BACKEND/handlers/user_rental.go" "deposit_waived"
  check "前端结算页发送免押金" "$WEAPP/Checkout.jsx" "deposit_waived"
  check "switchTab 迁移（首页 tab）" "$WEAPP/Home.jsx" "switchTab"
  check "switchTab 迁移（租赁 tab）" "$WEAPP/MyLeases.jsx" "switchTab"
}

FLOW="${1:-settlement}"
case "$FLOW" in
  settlement) verify_settlement ;;
  rent) verify_rent ;;
  *) echo "Unknown flow: $FLOW (supported: settlement, rent)"; exit 2 ;;
esac

echo ""
echo "=== 结果: $PASS 通过, $FAIL 失败 ==="
[ "$FAIL" -eq 0 ]
