# TC-1554 Review — Discount code 应用与使用追踪

> TC Issue: #1554 | 生成日期: 2026-08-07 | 类型: 后端测试

## 1. 目标

验证折扣码完整流程：#1554 "Discount code — apply + usage tracking"
- 有效码应用 → 租金折扣正确
- 使用追踪 → usage_count 递增
- 无效/过期/用尽码 → 拒绝（不生效）

## 2. 验证方式

- [x] 后端测试: `go test -run TestDiscountCodeFlow ./handlers/ -v`
- [ ] 静态验证: 不适用（后端专用，无前端链路变更）
- [ ] 真机验证: 不适用

## 3. 流程步骤与证据

### Step 1: 有效码应用折扣

**代码证据**:
- `backend/services/rent_calculator.go:156` — `CalculatePricingBreakdown` 将 `policy.RentDiscount` 乘入 tier Discount 链
- `backend/handlers/user_rental.go:434-456` — CreateOrder 先算 breakdown，`rentSubtotal = pricingBreakdown.TotalAmount`（折扣后租金）
- `backend/services/rent_calculator.go:292` — `resolveDiscountPolicy` 校验激活/次数/有效期

**测试证据**:
```
--- PASS: TestDiscountCodeFlow/valid_code_applies_discount
  断言: CashPaid = 1400（折扣后租金 900 + 押金 500）
  断言: tier_segments[0].discount ≈ 0.9（含折扣码因子）
```

### Step 2: 使用追踪（usage_count 递增）

**代码证据**: `backend/handlers/user_rental.go:556-572` — CreateOrder 创建 `DiscountCodeUsage` + `usage_count+1`

**测试证据**:
```
--- PASS: TestDiscountCodeFlow/usage_tracking
  断言: usage_count = 2（两单后）
  断言: discount_code_usages 行数 = 2
```

### Step 3: 用尽码拒绝

**代码证据**: `backend/services/rent_calculator.go:297-299` — `MaxUses > 0 && UsageCount >= MaxUses → nil`

**测试证据**:
```
--- PASS: TestDiscountCodeFlow/max_uses_exhausted
  断言: 第 3 单无新 usage 行（仍 2 行）
  断言: CashPaid = 1500（全价）
```

### Step 4: 无效码拒绝

**代码证据**: `backend/services/rent_calculator.go:294-296` — code 不存在 → nil

**测试证据**:
```
--- PASS: TestDiscountCodeFlow/invalid_code_ignored
  断言: CashPaid = 1500（全价，码被忽略）
```

### Step 5: 过期码拒绝

**代码证据**: `backend/services/rent_calculator.go:300-302` — ExpiresAt 已过 → nil

**测试证据**:
```
--- PASS: TestDiscountCodeFlow/expired_code_ignored
  断言: CashPaid = 1500（全价，码被忽略）
```

## 4. 结论

| 检查项 | 结果 | 证据位置 |
|--------|:---:|---------|
| 有效码 10% 折扣生效于实收金额 | ✅ | Step 1（CashPaid 1400） |
| 折扣进入 pricing_breakdown tier | ✅ | Step 1（discount 0.9） |
| usage_count 递增 | ✅ | Step 2（0→2） |
| DiscountCodeUsage 落库 | ✅ | Step 2（2 行） |
| 用尽码拒绝 | ✅ | Step 3 |
| 无效码拒绝 | ✅ | Step 4 |
| 过期码拒绝 | ✅ | Step 5 |

**总判定**: ✅ 通过

**测试输出汇总**:
```
--- PASS: TestDiscountCodeFlow (1.13s)
```

## 5. 已知局限

- 本 TC 测试暴露并修复了**生产 Bug**：原 CreateOrder `totalAmount` 未采用折扣后租金（`dailyRent × days` 全价），折扣仅显示在 breakdown —— 已修复为 `rentSubtotal = pricingBreakdown.TotalAmount`（#1569 comment 记录）
- 折扣上限 `MaxAmount` 未在测试中覆盖（fixture 未设 cap）——如实现存在可补测
- 预存失败 `TestCalculatePricing_Deposit`（services 层，与本次无关，clean main 确认）

---

*Model: deepseek/deepseek-v4-flash*
