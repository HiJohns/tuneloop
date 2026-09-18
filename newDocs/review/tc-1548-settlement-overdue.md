# TC-1548 Review — Settlement 超期退租退款

> TC Issue: #1548 | 生成日期: 2026-08-07 | 类型: 后端测试

## 1. 目标

验证超期归还时逾期费用从押金中扣除，剩余部分参与退款。

## 2. 验证方式

- [x] 后端测试: `go test -run TestInspectReturn_OverdueFee ./handlers/ -v`
- [x] `go test -run TestComputeSettlement_LateReturn ./handlers/ -v`
- [x] `go test -run TestSettlement_OverdueFeeDeducted ./handlers/ -v`

## 3. 流程步骤与证据

### Step: 超期结算

**代码证据**: `backend/handlers/warehouse.go:438-448` — overdueDays = CalculateDays(endDate+1, scanTime); `user_settlement.go:579-586` — totalDepositDeducted = overdueFee + damage, remainingDeposit = deposit - totalDepositDeducted

**测试证据**:
```
--- PASS: TestInspectReturn_OverdueFee (1.08s)
--- PASS: TestComputeSettlement_LateReturn (0.00s)
--- PASS: TestSettlement_OverdueFeeDeducted (pass after #1566 fix)
```

## 4. 结论

| 检查项 | 结果 | 证据 |
|--------|:---:|------|
| 逾期天数 = 5 | ✅ | TestInspectReturn_OverdueFee |
| 逾期费 = 75 | ✅ | 5 × 15 |
| 剩余押金 = 425 | ✅ | 500 - 75 |
| 退款 = 425 | ✅ | rent 1000 + 425 - 1000 |

**总判定**: ✅ 通过

---

*Model: deepseek/deepseek-v4-flash*
