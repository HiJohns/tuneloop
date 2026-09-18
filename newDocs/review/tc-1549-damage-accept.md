# TC-1549 Review — Damage flow 接受

> TC Issue: #1549 | 生成日期: 2026-08-07 | 类型: 后端测试

## 1. 目标

验证乐器归还时验收发现损坏（damaged），顾客接受定损，进入维修状态。

## 2. 验证方式

- [x] 后端测试: `go test -run TestScenarioA_DamageVariant ./handlers/ -v`
- [x] `go test -run TestComputeSettlement_DamageAccept ./handlers/ -v`

## 3. 流程步骤与证据

### Step 1: 验收 → damaged

**代码证据**: `backend/handlers/warehouse.go:420-422` — condition=damaged → status=pending_damage_response; 乐器 → maintenance

**测试证据**:
```
--- PASS: TestScenarioA_DamageVariant (1.12s)
  子测试: A7_InspectDamaged, A8_AcceptDamage
```

### Step 2: 客户接受定损 → 结算 + 退款

**代码证据**: `backend/handlers/order.go:1393` AcceptDamage → 结算 executeRefund; DamageReport.deposit_deducted

**测试证据**:
```
--- PASS: TestComputeSettlement_DamageAccept (0.01s)
  断言: damage_deducted 从押金扣除
```

## 4. 结论

| 检查项 | 结果 | 证据 |
|--------|:---:|------|
| 验收损坏 → 状态 pending_damage_response | ✅ | A7 |
| 乐器进入维修 | ✅ | StockStatusMaintenance |
| 客户接受定损 | ✅ | A8 |
| 损坏赔偿从押金扣除 | ✅ | deposit_deducted |
| 结算退款正确 | ✅ | ComputeSettlement |

**总判定**: ✅ 通过

---

*Model: deepseek/deepseek-v4-flash*
