# R4-derivation — 666.67 / 506.67 的逐段推导与规格对照

> 结论先行：`666.67` / `506.67` **只能**由当前实现（#1836 段级折后模型）推出；
> 按 `docs/cases/settlement-refund-spec-v2.md`（v2.3，L-06 v2）分账户公式，
> 同一夹具应为 **net cash 40 / gift 160**（over-cap）与 **net cash 200 / gift 0**（within-cap）。
> 两者冲突，需 Architect 裁定（见 §4）。本文件不把实现输出当作验收真值。

## 1. 夹具（`TestRefundDiff_PointsOverCap`，`settlement_diff_test.go:12`）

| 字段 | 值 |
|---|---|
| `pricing_breakdown` | `base_daily_rent=10000` 分，`rent_days=30`，`tier_segments=[{days:30, rate:10000, discount:1, subtotal:300000}]` |
| `cash_paid` | 2000 元（含押金 500） |
| `deposit` | 500 元 |
| `gift_points_used` | 1000 元（= 1000 点，存 100000 分） |
| 租期 | 2026-07-01 → 2026-07-28 12:00，`CalculateLeaseDays` = **28 天** |
| 会员比例 | 0.3（`gift_policies.pay_ratio`） |

## 2. 当前实现（`computeSettlement` → #1836 段级折后模型）

```
totalRentPaid   = cash 2000 + gift 1000 − deposit 500            = 2500
合同原价         = 30 天 × 100 元/天                              = 3000
段折扣率         = 2500 / 3000                                    = 5/6
折后租金应收      = round(28 天 × 100 × 5/6)                       = 2333.33
paidTotal       = totalRentPaid + deposit                        = 3000
net             = 2333.33 − 3000                                   = −666.67
totalRefund     = 666.67
a1              = floor(2800 × 0.3)                              = 840
a0              = 1000
giftRefunded    = 1000 − 840                                      = 160
cashRefundable  = 666.67 − 160                                    = 506.67
```

实现位置：`backend/handlers/user_settlement.go:636`（`computeSettlement`）、
`backend/handlers/segment_settlement.go:120`（`makePaidSegments`）、
`segment_settlement.go:57`（`computeDiscountedSettlement`）。

`TestRefundDiff_PointsWithinCap` 同夹具（`cash_paid=2500`、`gift=500`、`pay_ratio=0.5`）：
段模型同样得 `2333.33` 应收、`666.67` 退款；`a1=1400 ≥ a0=500` → gift 不退，`cashRefundable=666.67`。

## 3. 权威规格（`docs/cases/settlement-refund-spec-v2.md` §3，L-06 v2）

Over-cap 夹具（cash 2000 / deposit 500 / gift 1000 / R1=2800 / ratio=0.3）：

```
R_after_coupon      = 2800（无券）
cap_points          = floor(2800 × 3/10)                    = 840
p_consumed          = min(1000, 840)                        = 840
p_refund            = 1000 − 840                            = 160 点
rent_cash_paid_net  = 2000 − 500（押金分离）                 = 1500
rent_cash_due       = 2800 − PointsToCash(840)              = 1960
rent_balance        = 1500 − 1960                            = −460
rent_refund         = max(0, −460)                           = 0
rent_shortfall      = 460
deposit_refund      = 500（无逾期/定损/物流）
cash_refundable     = 0 + 500                                = 500
cash_payable        = 460
net_cash            = 500 − 460                              = 40
总经济退款           = gift 160 + net cash 40                 = 200 = R0(3000) − R1(2800)
```

Within-cap 夹具（cash 2500 / deposit 500 / gift 500 / ratio=0.5）：

```
cap_points          = floor(2800 × 0.5)                      = 1400 ≥ 500 → p_refund = 0
rent_cash_paid_net  = 2000
rent_cash_due       = 2800 − 500                             = 2300
rent_balance        = 2000 − 2300                            = −300
rent_refund         = 0；cash_refundable = 500；cash_payable = 300
net_cash            = 200
```

> 修复前断言 `TotalRefund=200 / CashRefundable=40`（over-cap）与 `CashRefundable=200`
> （within-cap）**与 spec v2 的净额一致**；修复后改成 `666.67 / 506.67 / 666.67`
> 与 spec 不一致，且差异全部来自「段模型把乐币当成整单折扣，而不是结算时按 R1 重算
> cap/补缴」。

## 4. 冲突结论与待裁定项（Architect）

| 口径 | 来源 | over-cap | within-cap |
|---|---|---|---|
| 段级折后模型（当前产品代码） | #1836（代码注释称 2026-09-07 业务确认），`segment_settlement.go` | total 666.67 / cash 506.67 | cash 666.67 |
| 分账户规格 v2.3 | `docs/cases/settlement-refund-spec-v2.md` + `docs/cases/lease.md` L-06 v2 | net cash 40 / gift 160 | net cash 200 |
| #2339 Accepted Plan | Input Truth「结算口径以现有实现为准（不是 #2268 尚未落地的 spec）」 | 段模型（666.67） | 段模型（666.67） |

Accepted Plan 与审计 F3 直接矛盾：

- Plan 说「以现有实现为准，不是 #2268 尚未落地的 spec」；审计说 spec v2 是权威规格，
  666.67 无文档依据、属「改断言迁就实现」。
- 本单 Out of Scope 明确排除「#2268 结算重构（#2359–#2363）的实现」。

按 Plan D1 / D5（结算语义冲突 → 回 Architect，禁止改断言迁就实现）与 AGENTS §2.5，
本项**不由 Developer 自行裁定**。需要 Architect 在以下路径中选一：

1. **维持当前产品口径**：在 `docs/cases/` 正式文档化 #1836 段级折后模型（含本例推导），
   并更新 `settlement-refund-spec-v2.md` 的适用范围/优先级；#2339 保留 666.67 断言。
2. **对齐 spec v2**：将「结算按 R1 分账户重算」纳入 #2359–#2363 的实现范围，
   #2339 的本组断言在实现落地前保持旧值（或标记为依赖 #2359–#2363 的 blocked 行），
   不得以现状输出为真值。

在裁定前，#2339 不进入 ready；本文件与 R5/R6 前缀证据随 Issue 一起交回 Architect。
