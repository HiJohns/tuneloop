# build=1.0.b70f96eb3 commit=b70f96eb3b80b4d18a873ef2dd4eaa95c1bf7add dirty=false
# R9 — #2339 R4 退款断言逐值推导（rent / deposit / points）

> 权威裁定：#2339 Rev.5 D1 —— 本单以**当前实现（#1743/#1836 段模型）**为准并逐值推导；
> spec v2.3 分账户差异记录在本文件末尾，归属 #2359–#2363，由 follow-up **#2417** 收敛。
> 引用代码：`backend/handlers/user_settlement.go::computeSettlement`、
> `backend/handlers/segment_settlement.go`、`backend/handlers/settlement_diff_test.go`（R4 fixture）。

## 0. Fixture 与字段来源（R4：TestRefundDiff_PointsOverCap）

| 输入 | 值 | 来源 |
|---|---|---|
| CashPaid | 2000.00（200000 分） | `orders.cash_paid`（含押金，L-06 生产契约） |
| GiftPointsUsed | 1000 点（100000 分存储） | `orders.gift_points_used`（`models.Cents`，1 点 = 100 分） |
| Deposit | 500.00（50000 分） | `orders.deposit` |
| 实际天数 | 28 | StartDate 2026-07-01，ReturnedAt 2026-07-28，`CalculateLeaseDays` |
| 定价 | 100 元/天 × 30 天，无阶梯/优惠 | `pricing_breakdown`（base_daily_rent=10000 分，tier_segments 30 天 rate=10000，discount=1） |
| pay_ratio | 0.3 | `users.membership_level_id=1` → `gift_policies.pay_ratio` |

## 1. Rent 推导（段模型 #1836）

```text
totalRentPaid = (CashPaid + PrepaidPointsUsed + GiftPointsUsed − Deposit) / 100
              = (200000 + 0 + 100000 − 50000) / 100 = 2500.00
paidTotal     = totalRentPaid + Deposit = 2500 + 500 = 3000.00
contractDays  = pricing_breakdown.rent_days − Σ续费 days = 30 − 0 = 30
段原价         = 30 天 × 100.00 = 3000.00
段折扣率       = contractRent / 段原价 = 2500 / 3000 = 0.833333…
discountedRent = round(28 天 × 100.00 × 0.833333…) = 2333.33
discountedDue  = discountedRent + 物流 0 = 2333.33
净额           = discountedDue + 逾期 0 + 定损 0 − paidTotal = 2333.33 − 3000 = −666.67
→ TotalRefund  = 666.67（应退）
→ PayableShortfall = 0
CashBasis      = rentPayable − effectiveGift = 2800 − 840 = 1960.00
rentPayable    = 28 天 × 100.00 = 2800.00（tier 截断，非折后）
```

## 2. Deposit 推导

```text
押金扣项 = 逾期 0 + 定损 0 + 物流 0 = 0
RemainingDeposit = Deposit − 押金扣项 = 500.00（全额退回，含在 TotalRefund 内）
DepositDeductedOverdue = 0；DamageDeducted = 0
```

## 3. Points 推导

```text
a0（已用点） = GiftPointsUsed = 1000
a1（退款时上限）= floor(rentPayable × pay_ratio) = floor(2800 × 0.3) = 840
a1 < a0 → 退回 (a0 − a1) = 160 点
cashRefundable = TotalRefund − 退回点面值 = 666.67 − 160 = 506.67
```

> 断言映射：`GiftPointsRefunded=160`、`TotalRefund=666.67`、`CashRefundable=506.67`、
> `CashBasis=1960`，与 `settlement_diff_test.go::TestRefundDiff_PointsOverCap` 逐值相等。

### 3.1 第二用例（TestRefundDiff_PointsWithinCap，pay_ratio=0.5）

```text
CashPaid=2500、GiftPointsUsed=500 点、Deposit=500 → totalRentPaid=2500、paidTotal=3000
段折扣率 = 2500/3000；discountedRent = 2333.33；TotalRefund = 666.67
a1 = floor(2800 × 0.5) = 1400 ≥ a0=500 → 退回点 0
CashRefundable = 666.67；CashBasis = 2800 − 500 = 2300
```

## 4. 与 spec v2.3 分账户公式的差异（不属本单）

以同一 fixture（券后租金 2800、乐币 1000 点、押金 500、租金现金实收 1500、无额外费用）按
`docs/cases/settlement-refund-spec-v2.md` §3/§4 推导：

```text
cap           = floor(2800 × 0.3) = 840
p_consumed    = min(1000, 840) = 840；p_refund = 160
rent_cash_due = 2800 − 840×1 = 1960
rent_balance  = 1500 − 1960 = −460 → rent_shortfall 460 / rent_refund 0
deposit_refund= 500（无扣项）
cash_refundable（毛额）= 500；payable_shortfall = 460
net cash      = 40（= 500 − 460）
TotalRefund（#2417 lock 兼容口径）= gift 160 + cash 毛额 500 = 660
```

| 口径 | TotalRefund | CashRefundable | 净现金 | 退点 |
|---|---:|---:|---:|---:|
| 当前实现（R4 断言，段模型） | 666.67 | 506.67 | 506.67（无补缴） | 160 |
| spec v2.3 分账户 | 660 | 500 | **net cash 40** | 160 |
| 差异 | +6.67 | +6.67 | +466.67 | 0 |

**归属与跟踪**：差异来自段模型对「租金+乐币」按支付比例整体摊薄（#1743/#1836），
不是本单（Group B triage）可修改的范围。语义收敛归 **#2359**（计算器）、**#2360**（模型/账本）、
**#2361**（执行器）、**#2362**（乐币）、**#2363**（回归）；**#2417**（Parent #2268）跟踪决策与文档。
截至本文件生成：#2423 S1 纯函数已落地（fix `fc8cbc14c`），#2424 S2 切换口径在 `status:analysis`
（执行器毛/净语义缺口待 Architect 锁定）；本单不改 `user_settlement.go` / `segment_settlement.go`。

## 5. 押金语义（#2406 对齐）

`instruments.deposit` 带 `default:0`，读取后恒为非 nil；`resolveDepositTier` 仅 `>0` 视为显式覆盖，
`=0` 回落定价押金（#2406 裁定）。本 fixture 的押金 500 来自定价结果，与 #2406 口径一致；
R6 的 SettlementFlow 断言 `deposit from instrument pricing = 50000 分`。
