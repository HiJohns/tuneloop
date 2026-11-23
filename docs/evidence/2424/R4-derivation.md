# build=1.0.59b858e76 commit=59b858e76a67036c14af23bf18b9892dcbf64ee5 dirty=false
# R4-derivation — #2424 Rev.6（Phase B 断言 old→new 逐项推导）

> 生成口径：spec v2.3 §3/§4；券权威 = 逐笔 `order_payment_records.coupon_discount`；
> `usedCoupon = Σ Usage[i]/Days[i] × segmentCoupon[i]`；`R_after_coupon = rentPayable − usedCoupon`；
> `contract_list` = 全期覆盖标价（`Σ tier_segments.subtotal`）。
> 本文件是推导，不是测试输出摘抄；每一项都给出输入 → 公式 → 新值。

## 1. 公式与字段

| 量 | 公式 | 说明 |
|---|---|---|
| `usedCoupon` | `Σ round(segmentCoupon[i] × Usage[i] / Days[i])` | 券按支付段落到实际使用天 |
| `R_after_coupon` | `rentPayable − usedCoupon` | 乐币只走 `p_used`，禁止用段 Rate 反推券 |
| `cash_refundable`（毛额） | `max(0, rent_cash_paid_net − due) + deposit_refund` | 对外展示 |
| `payable_shortfall`（毛额） | `max(0, due − rent_cash_paid_net) + other_fee_shortfall` | 对外展示 |
| `net_cash` | `cash_refundable − payable_shortfall` | **执行口径** |
| `TotalRefund` | `PointsToCash(p_refund) + cash_refundable` | 乐币面值 + 现金毛额 |

## 2. fixture 变更（gross/net 的前提，不是迁就输出）

1. `settlement_1836_test.go::mkOrder`：每笔支付补 `CouponDiscount = 标价日租 × days − 实付`
   （三码场景每笔 36.00 − 0.36 = 35.64）；`orders.coupon_discount` 同步为逐笔合计。
   原 fixture 只有订单级 `coupon_discount`（或 0），逐笔券缺失时 legacy 分摊无法区分「券/乐币」。
2. `反推合同天数` / `1bf456df` 子用例：逐笔支付补 35.64 券；订单券改为 106.92 / 71.28（逐笔合计）。
3. `TestPayableTiers_TruncatedByActualDays` / `TestSettlement_PostDamageStates` / `settle1855Order`：
   两段支付各补 35.64 券 + 补建合同 `rent` 支付流水（账本为唯一事实源，缺合同流水会把未使用续费误判为欠费）。
4. `跨阶-无码-续费跨30天边界`：补建合同租金流水 `1008.00`（原 fixture 只有续费 174.60，ledger net 被覆盖后误判补缴）。

## 3. 断言 old→new（逐文件）

### backend/handlers/settlement_1743_test.go

| 用例/字段 | old | new | 推导 |
|---|---:|---:|---|
| ScenarioB `TotalRefund` | 140 | **240** | 券 0；R1 100，现金 340 → 毛额退 240；物流 100 转补缴 |
| ScenarioB `CashRefundable` | — | **240** | 同上 |
| ScenarioB `PayableShortfall` | 0 | **100** | 押金 0 → 物流全额转补缴 |
| ScenarioB `NetCash` | — | **140** | 240 − 100 |

### backend/handlers/settlement_diff_test.go

| 用例/字段 | old | new | 推导 |
|---|---:|---:|---|
| PointsOverCap `TotalRefund` | 666.67 | **660.00** | 乐币 160 + 现金毛额 500 |
| PointsOverCap `CashRefundable` | 506.67 | **500.00** | 押金 500 全退；租金已足额（R1 2800，现金 2000 + 乐币 1000） |
| PointsOverCap `PayableShortfall` | — | **460.00** | rent_cash_paid_net 1500 − due (2800−840) 1960 = −460 |
| PointsOverCap `NetCash` | — | **40.00** | 500 − 460 |
| PointsOverCap conservation | 666.67 | **660.00** | gift + cash gross |
| PointsWithinCap `CashRefundable` | 666.67 | **500.00** | 押金 500；租金 due 2300 vs 现金 2000 → 缺口 300 |
| PointsWithinCap `PayableShortfall` | — | **300.00** | 2300 − 2000 |
| PointsWithinCap `NetCash` | — | **200.00** | 500 − 300 |
| PointsWithinCap `TotalRefund` | — | **500.00** | 现金毛额（无退点） |

### backend/handlers/settlement_1836_test.go

（场景 1 每笔券 35.64；实租 1 天 → usedCoupon 3564 分，R_after 36 分。）

| 用例/字段 | old | new | 推导 |
|---|---:|---:|---|
| 场景1 `TotalRefund` | 0 | **0.72** | 现金毛额：实付 1.08 − 应收 0.36 |
| 场景1 `PayableShortfall` | 0.28 | **1.00** | 物流 1.00（押金 0） |
| 场景1 `NetCash` | — | **−0.28** | 0.72 − 1.00 |
| 场景1 breakdown `payable_shortfall` | 28 分 | **100 分** | 毛额落库 |
| 场景1 breakdown `cash_refundable_gross` / `net_cash` | — | **72 / −28 分** | 毛额 + 净额键 |
| 场景2A `TotalRefund` | 35.36 | **36.36** | 实付 36.72 − 应收 0.36（合同券 35.64；续1 无码） |
| 场景2A `CashRefundable` / `Payable` / `Net` | — | **36.36 / 1.00 / 35.36** | 物流 1.00 |
| 场景2B `TotalRefund` | 0 | **0.36** | 实租 2 天：R_after 36.36；实付 36.72 |
| 场景2B `PayableShortfall` | 0.64 | **1.00** | 物流 1.00 |
| 场景2B `CashRefundable` / `NetCash` | — | **0.36 / −0.64** | 0.36 − 1.00 |
| 反推合同天数 `PayableShortfall` | 0.28 | **1.00** | 同场景1（券 35.64/笔） |
| 反推合同天数 `TotalRefund` / `NetCash` | 0 / — | **0.72 / −0.28** | 同上 |
| 1bf456df `PayableShortfall` | 0.64 | **1.00** | 物流 1.00 |
| 1bf456df `TotalRefund` / `NetCash` | 0 / — | **0.36 / −0.64** | 实付 0.72 − 应收 0.36 |
| PayableTiers_Truncated `TotalRefund` | 0.35 | **0.36** | 实付 0.72 − 应收 0.36（毛额） |
| PayableTiers_Truncated `Payable` / `Net` | — | **0.01 / 0.35** | 物流 0.01 |
| PostDamageStates `PayableShortfall` | 0.65 | **1.01** | 物流 0.01 + 定损 1.00 |
| PostDamageStates `CashRefundable` / `NetCash` | — | **0.36 / −0.65** | 租金退 0.36 − 费用 1.01 |

### backend/handlers/settlement_1855_test.go

| 用例/字段 | old | new | 推导 |
|---|---:|---:|---|
| waived `PayableShortfall` | 0 | **0.36** | 物流 0.01 + 定损 1.00 − waived 0.65 |
| waived `CashRefundable` / `NetCash` | — | **0.36 / 0** | 租金退 0.36 = 补缴 0.36 |
| waived `TotalRefund` | 0 | **0.36** | 现金毛额 |
| 无 waived `PayableShortfall` | 0.65 | **1.01** | 毛额；净额仍 0.65 |
| 无 waived `NetCash` | — | **−0.65** | discriminator 保持 |
| 落库 breakdown `payable_shortfall` | 0 | **36 分** | 毛额落库；`net_cash=0` 才是闭环判据 |

### backend/handlers/state_machine_e2e_test.go

| 用例/字段 | old | new | 推导 |
|---|---:|---:|---|
| LateReturn 断言基数 | `CashRefundable`（毛额 2220） | **`NetCash`** | 执行按净额；毛额 2220 另加断言 |
| LateReturn 区间 | 1500–2000 | **1500–2000（不变）** | 净额 = 押金退 2220 − 租金缺口 470 = 1750 |

## 4. 未改断言的复核

- `TestComputeSettlement_DamageAccept`：无 `rent_days` → 段模型回退 #1743，spec 因守恒校验失败被跳过 → 旧字段保留，断言不变（500→2500 的既有路径）。
- 其余 70 余个结算/退款用例（`go test` 82 个）未改断言且全绿；其数值由净额守恒或既有 fallback 维持。
- 净额守恒：`net + p_refund = cash_paid + p_used − R_after_coupon − deductions`，R5 三 fixture 均由 `ComputeRefundSpec` 的 `ConservationOK` 强制校验。
