# TC-1552 Review — Gift points 订单完成奖励

> TC Issue: #1552 | 生成日期: 2026-08-07 | 类型: 后端测试

## 1. 目标

验证订单完成时 loyalty 积分奖励按实际租金 × 会员等级比率入账。

## 2. 验证方式

- [x] 后端测试: `go test -run TestExecuteRefund_LoyaltyPoints ./handlers/ -v`

## 3. 流程步骤与证据

### Step: executeRefund → 积分奖励

**代码证据**:
- `backend/handlers/user_settlement.go:371-396` — executeRefund 结算后 loyaltyPoints = rentPayable × selfRatio → promo_points + PointsTransaction(type=loyalty)
- `backend/services/membership_engine.go:77-87` — GetGiftRatios(levelID) → SelfSpendRatio

**测试证据**:
```
--- PASS: TestExecuteRefund_LoyaltyPoints (1.10s)
  断言: loyalty 积分按 ratio × rentPayable 入账
  断言: PointsTransaction(type=loyalty) 落库
```

## 4. 结论

| 检查项 | 结果 | 证据 |
|--------|:---:|------|
| 会员等级比率读取 | ✅ | GetGiftRatios |
| 积分 = rentPayable × ratio | ✅ | loyaltyPoints |
| promo_points 入账 | ✅ | gorm.Expr("promo_points + ?") |
| PointsTransaction 记录 | ✅ | type=loyalty |

**总判定**: ✅ 通过

---

*Model: deepseek/deepseek-v4-flash*
