# TC-1551 Review — Gift points 注册奖励

> TC Issue: #1551 | 生成日期: 2026-08-07 | 类型: 后端测试

## 1. 目标

验证注册奖励积分流程：#1551 "Gift points — registration bonus"
- 注册赠点：`membership_gift_points`（默认 99）→ 新用户 promo_points + PointsTransaction(type=registration)
- 介绍人奖励：referrer 的 `MembershipGiftRatio.ReferralRegPoints` → referrer promo_points + PointsTransaction(type=referral_reg)

## 2. 验证方式

- [x] 后端测试: `go test -run TestGiftRegistration ./handlers/ -v`
- [ ] 静态验证: 不适用
- [ ] 真机验证: 不适用

## 3. 流程步骤与证据

### Step 1: 注册（带 ref）→ 双奖励

**代码证据**:
- `backend/handlers/auth.go:371-385` — 注册赠点：`membership_gift_points` setting（默认 99）→ promo_points + PointsTransaction(registration)
- `backend/handlers/auth.go:400-415` — 介绍人奖励：referrer MembershipLevelID → GetGiftRatios → ReferralRegPoints → promo_points + PointsTransaction(referral_reg)

**测试证据**:
```
--- PASS: TestGiftRegistration/register_with_ref_credits_both
  断言: 新用户 promo_points = 99（注册赠点）
  断言: PointsTransaction(type=registration, amount=99) 1 条
  断言: referrer promo_points = 100+50 = 150（介绍奖励）
  断言: PointsTransaction(type=referral_reg, amount=50) 1 条
  断言: Referral 行创建（referrer_id + ref_code）
```

### Step 2: 注册（无 ref）→ 仅注册赠点

**测试证据**:
```
--- PASS: TestGiftRegistration/register_without_ref_credits_only_gift
  断言: 新用户 promo_points = 99（仅注册赠点）
  断言: referrer promo_points 保持 150（无 ref 不触发介绍奖励）
```

## 4. 结论

| 检查项 | 结果 | 证据位置 |
|--------|:---:|---------|
| 注册赠点 99 入账新用户 | ✅ | Step 1/2 |
| 注册 PointsTransaction 落库 | ✅ | Step 1 |
| 介绍人奖励 50 入账 | ✅ | Step 1 |
| 介绍 PointsTransaction 落库 | ✅ | Step 1 |
| Referral 关系行创建 | ✅ | Step 1 |
| 无 ref 不触发介绍奖励 | ✅ | Step 2 |

**总判定**: ✅ 通过

**测试输出汇总**:
```
--- PASS: TestGiftRegistration (1.10s)
```

## 5. 已知局限

- 依赖 IAM mock（`newRegisterMockServer`）——真实 IAM 注册链路需预生产验证
- 忠诚度奖励（SelfSpendRatio，订单完成时）属 #1552，由 `TestExecuteRefund_LoyaltyPoints` 覆盖，非本 TC 范围

---

*Model: deepseek/deepseek-v4-flash*
