# TC-1546 Review — Settlement 正常退租退款

> TC Issue: #1546 | 生成日期: 2026-08-07 | 类型: 后端测试

## 1. 目标

验证完整租赁闭环 + 正常退租结算退款：CreateOrder → Pay → Ship → Delivery → Return → InspectReturn(good) → 结算 + 退款。

## 2. 验证方式

- [x] 后端测试: `go test -run TestSettlementFlow ./handlers/ -v`
- [ ] 静态验证: 不适用
- [ ] 真机验证: 不适用

## 3. 流程步骤与证据

### Step 1-6: 完整闭环 → 结算

**代码证据**:
- `backend/handlers/user_settlement.go:247-356` — executeRefund: 结算创建 + 退款
- `backend/handlers/user_settlement.go:487-646` — computeSettlement: 租金计算 + 退款公式
- `backend/handlers/warehouse.go:323-489` — InspectReturn: 验收 → executeRefund

**测试证据**:
```
--- PASS: TestSettlementFlow (1.60s)
  断言: deposit=500, CashPaid=3500, actualDays=31, rent=3000
  断言: CashRefundable=500, refundRecord.status=refunded, amount=500
  断言: deposit_refunded=true, instrument=available
```

### Step 7: 幂等性

**测试证据**:
```
  断言: 二次 inspect 返回 400（已完成订单不可再验收）
  断言: Settlement 仅 1 条, OrderRefundRecord 仅 1 条
```

## 4. 结论

| 检查项 | 结果 | 证据 |
|--------|:---:|------|
| 租金计算（30 天 × 100 = 3000） | ✅ | TestSettlementFlow |
| 押金退还（500） | ✅ | CashRefundable=500 |
| 退款记录创建 | ✅ | status=refunded, amount=500 |
| 乐器归还可用 | ✅ | StockStatusAvailable |
| 幂等性（不可重复验收） | ✅ | 400 BadRequest |
| 退款不重复 | ✅ | 1 条 settlement + 1 条 refund |

**总判定**: ✅ 通过

## 5. 已知局限

- 依赖真实 PostgreSQL test DB（`tuneloop_test`）
- Mock 支付模式（WECHAT_PAY_MOCK_MODE=true）

---

*Model: deepseek/deepseek-v4-flash*
