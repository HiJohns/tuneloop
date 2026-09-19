# TC-1556 Review — Shipping fee 员工发货

> TC Issue: #1556 | 生成日期: 2026-08-07 | 类型: 后端测试

## 1. 目标

验证员工发货时填写物流费存入 order.shipping_fee，不向顾客收费，结算退款时扣除。

## 2. 验证方式

- [x] 后端测试: `go test -run TestUpdateShipping ./handlers/ -v`
- [x] 流程内含: `TestSettlementFlow`（UpdateShipping 调用证实链路）
- [x] 前端静态: #1570 已完成物流费显示控制（Checkout 不显示，OrderDetail 按状态门控）

## 3. 流程步骤与证据

### Step: 发货填写物流费

**代码证据**: `backend/handlers/warehouse.go:113-165` — UpdateShipping 接收 shipping_fee；line 40-41 `updateFields["shipping_fee"] = req.ShippingFee`

**测试证据**:
```
--- PASS: TestUpdateShipping (0.32s)
  断言: shipping_fee 写入 order
```

### Step: 结算时扣除物流费

**代码证据**: `backend/handlers/user_settlement.go:568` — totalRentPaid = CashPaid - Deposit - ShippingFee

**前端证据**: `frontend-mobile/src/pages-weapp/Checkout.jsx` (commit `035d8870`) — 下单不显示物流费

## 4. 结论

| 检查项 | 结果 | 证据 |
|--------|:---:|------|
| 发货填费 | ✅ | UpdateShipping 接收 shipping_fee |
| 不收费（下单无物流费） | ✅ | Checkout.jsx totalAmount 无 fee |
| 结算扣物流费 | ✅ | totalRentPaid 减 ShippingFee |
| 发货后显示物流费 | ✅ | OrderDetail showShippingFee 门控 |

**总判定**: ✅ 通过

---

*Model: deepseek/deepseek-v4-flash*
