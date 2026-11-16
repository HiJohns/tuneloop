# 结算退款算法规格 v2.3（定稿 · 评审 4 项必修 + Q16/Q17 决策）

> v2.2 → v2.3（业务决策）：**Q16 优惠券在前、乐币上限按「券后实际租金」计算**（覆盖评审建议的「按券前标价」）；
> **Q17 支付分配账本列入本期**。据此：`cap` 基数改为 `R_after_coupon_cents`，守恒式去掉券项（券已体现在实收与券后租金中）。
>
> v2.1 → v2.2（按 2026-11 评审）：① 优惠券与乐币基数拆分 + 守恒式补券项；② 争议金额从实际扣款隔离；
> ③ `failed_retryable` 纳入订单级唯一约束 + 预生成稳定幂等键；④ 租金现金实收改由**支付分配账本**读取（禁止反推）。
> 附带：T8 夹具显式化、T23–T26 新增、整数溢出校验、价格规则快照、费用分类分配。
> 状态：**定稿 v2.3 —— Q1–Q17 全部确认（Q16 券在前、上限按券后租金；Q17 分配账本列入本期）。**

---

## 0. 单位与类型（P0，已定）

| 量 | 单位/类型 |
|---|---|
| 现金 | `*_cents int64` |
| 乐币 | `*_points int64`，1 点 = 100 分 |
| 比例 | 整型 `ratio_num/ratio_den`（30% → 3/10），禁止浮点 |
| 天数 | `int` |

转换唯一出口（业务代码禁止自行 ×100 / ÷100）：

```go
func PointsFromStoredCents(v int64) (int64, error) // gift_points_used 以 Cents 存储（900 点 = 90000）
func StoredCentsFromPoints(p int64) int64
func PointsToCashCents(p int64) int64
```

**溢出与合法性校验（评审技术建议）**：

```text
ratio_den > 0；0 <= ratio_num <= ratio_den
金额/点数/天数 >= 0
cap 计算：先算 R_actual_list_cents * ratio_num，乘法前检查 <= MaxInt64/ratio_num
rent_cash_due：p_consumed_points * 100 前检查 <= MaxInt64/100
错误：ErrSettlementInvalidInput / ErrSettlementOverflow
```

---

## 1. 原则（评审确认）

| # | 原则 |
|---|---|
| P1 | 押金与租金完全分离：押金不参与租金折算、不作乐币基数 |
| P2 | 应付租金按**标价**与实际天数重算，禁止用实付/标价倒推日租 |
| P3 | 乐币是有上限的租金优惠权益；比例取**下单快照**，不溯既往 |
| P4 | 乐币只退超额：`p_consumed = min(p_used, cap)`；少用不补、不折现 |
| P5 | 额外费用独立核算（逾期/定损/物流）；可扣押金，不足补缴；押金不得抵租金缺口 |
| P6 | **争议金额隔离**：申诉中的金额挂起，不进入本期扣款与退款计算 |

---

## 2. 租金的三层金额（P0，评审新增）

```text
R_actual_list_cents   = 实际天数对应的标价租金（阶梯重算）
coupon_discount_cents = 优惠券抵扣（>=0）
R_after_coupon_cents  = R_actual_list_cents - coupon_discount_cents   // 必须 >= 0
```

**Q16 决策（2026-11）：优惠券在前、乐币在后，乐币上限基数为 `R_after_coupon_cents`（券后实际租金）。**
即：先扣券，再按券后租金计算可用乐币上限与消耗。

---

## 3. 分账户算法（含券与争议）

```text
# 乐币
cap_points        = floor(R_after_coupon_cents * ratio_num / (100 * ratio_den))   // Q16：券后租金为基数
p_consumed_points = min(p_used_points, cap_points)
p_refund_points   = p_used_points - p_consumed_points

# 租金账户（现金，分）
rent_cash_paid_net_cents   = 分配账本读取（见 §3.1）
rent_cash_due_cents        = R_after_coupon_cents - PointsToCashCents(p_consumed_points)
rent_balance_cents         = rent_cash_paid_net_cents - rent_cash_due_cents
rent_refund_cents          = max(0, rent_balance_cents)
rent_shortfall_cents       = max(0, -rent_balance_cents)

# 额外费用（分类 + 争议隔离）
deductions_total_cents      = overdue_penalty_cents + damage_fee_cents + logistics_fee_cents
undisputed_deductions_cents = deductions_total_cents - disputed_amount_cents   // 见 §8
deposit_deductions_cents    = min(deposit_held_cents, undisputed_deductions_cents)
deposit_refund_cents        = deposit_held_cents - deposit_deductions_cents
other_fee_shortfall_cents   = max(0, undisputed_deductions_cents - deposit_held_cents)

# 汇总（执行层）
cash_refundable_cents = rent_refund_cents + deposit_refund_cents
cash_payable_cents    = rent_shortfall_cents + other_fee_shortfall_cents
net_cash_cents        = cash_refundable_cents - cash_payable_cents
```

### 3.1 租金/押金现金实收：只能从分配账本读取（P1，评审新增）

```text
rent_cash_paid_net_cents          // 租金现金净实收
deposit_cash_collected_net_cents  // 押金累计净收款
deposit_held_cents                // 当前持有押金（未退未扣）
```

- **禁止**用 `cash_paid_net - deposit_held` 反推（押金被扣用后该式失真）；
- 短期无法改数据模型时，必须显式限制：仅当「无押金扣用 + 历史退款分配记录完整」才允许回退公式，并打 `legacy_derived_rent_cash` 标记，逐单审计。

> **Q17 决策（2026-11）：支付分配账本列入本期并强制实现**（落地见 #2360；支付成功/退款回调写账本，结算只读账本）。

#### 3.1.1 为什么必须用账本（示例）

以「租金 ¥1000 + 押金 ¥1000」为例：

| 时点 | 事实 | 反推公式的错误 | 账本读取 |
|---|---|---|---|
| 下单支付 | 实收 ¥2000（租金 1000 + 押金 1000） | — | 租金实收 1000 / 押金收 1000 |
| 归还定损，从押金扣 ¥300 | 当前持有押金 = ¥700 | `2000 − 700 = 1300`（**错**：租金实际只收了 1000） | 租金实收仍为 1000；押金已用 300 |
| 结算退款 | 应退押金 ¥700，租金按实际租期另算 | 反推会把多出的 300 当作「少收的租金」，导致租金/押金账户串账 | 分账户各自结算，互不影响 |

结论：`rent_cash_paid_net_cents` 只能来自分配账本（每笔支付/退款按 `rent_cents / deposit_cents / fee_cents` 落一行），这也是 T26 用例守护的不变量——**押金扣用前后，租金现金净实收不得变化**。

---

## 4. 守恒式（含券，单位为分）

```text
net_cash_cents + PointsToCashCents(p_refund_points)
  = cash_paid_cents + PointsToCashCents(p_used_points)
    - R_after_coupon_cents - deductions_total_cents

# 说明：优惠券不是支付，已体现在「cash_paid 减少」与「R_after_coupon 减少」两侧，
# 故等式不再单列 coupon 项（v2.2 的写法在 Q16 券前基数下才成立）。
```

作为独立 invariant（`ErrSettlementInvariantViolation`）；多次结算/冲正/争议挂起按 §9 版本机制处理。

---

## 5. 推演（合同 ¥100/天 × 30 天，押金 ¥500，ratio=3/10，无券无争议）

| # | 场景 | cash_paid(分) | p_used | 天数 | R_list(分) | cap | p_refund | rent 退/补(分) | 押金退(分) | 净现金(分) | 退点 |
|---|---|---|---|---|---|---|---|---|---|---|---|
| T1 | 正常履约 30 天 | 260000 | 900 | 30 | 300000 | 900 | 0 | 0 | 50000 | +50000 | 0 |
| T2 | 提前 28 天 | 260000 | 900 | 28 | 280000 | 840 | 60 | +14000 | 50000 | +64000 | 60 |
| T3 | 提前 28 天，少用点 | 300000 | 500 | 28 | 280000 | 840 | 0 | +20000 | 50000 | +70000 | 0 |
| T4 | 历史少收 + 超用点 | 200000 | 1000 | 28 | 280000 | 840 | 160 | **−46000** | 50000 | +4000 | 160 |
| T5 | T2 + 定损 300 | 260000 | 900 | 28 | 280000 | 840 | 60 | +14000 | 20000 | +34000 | 60 |
| T6 | T2 + 定损 800 | 260000 | 900 | 28 | 280000 | 840 | 60 | +14000 | 0 | −16000 | 60 |
| T7 | 当日取消（未交付/未起租/无取消费） | 260000 | 900 | 0 | 0 | 0 | 900 | +210000 | 50000 | +260000 | 900 |
| T8 | 续费 +30 天，实际 45 天 | 560000 | 900 | 45 | 450000 | 1350 | 0 | +150000 | 50000 | +200000 | 0 |
| T9 | 零押金乐器（T2 条件） | 210000 | 900 | 28 | 280000 | 840 | 60 | +14000 | 0 | +14000 | 60 |
| T10 | 差额押金 ¥30（T2 条件） | 213000 | 900 | 28 | 280000 | 840 | 60 | +14000 | 3000 | +17000 | 60 |

**T8 夹具必须显式写出（评审要求）**：

```text
original_term_days=30,  original_rent_cents=300000
renewal_term_days=30,   renewal_rent_cents=300000
total_contract_days=60, total_contract_rent_cents=600000
actual_days=45,         R_actual_list_cents=450000
deposit_policy=carry_over
```

**新增用例**：

| # | 场景 | 期望要点 |
|---|---|---|
| T23 | 券 30000 分 + 乐币 900 点，28 天 | `R_after_coupon=250000`；**cap=750（按券后租金）**；`p_refund=150`；rent 现金实收 160000 vs 应付 175000 → **补缴 15000**；押金退 50000；**净现金 +35000**；守恒：35000+15000=300000−250000 ✓ |
| T24 | 定损 300 分处于申诉 + 逾期 100 分 | 只扣 `undisputed=10000`；`disputed=30000` 挂起；押金退 `40000`；申诉结案后补结算 |
| T25 | 首次结算 `failed_retryable` 后再发起 | 订单级唯一阻挡第二版本；必须先冲正/恢复同一 settlement |
| T26 | 押金已被扣 30000 后再结算 | `rent_cash_paid_net` 从分配账本读取，不得因 `deposit_held` 变化而改变 |

---

## 6. Q8：足额收款（三个现金口径）

| 概念 | 用途 |
|---|---|
| `initial_cash_collected_cents` | 下单校验足额：`= R_after_coupon_cents − PointsToCashCents(p_used) + deposit_cents`（券在前，Q16） |
| `cash_paid_net_cents` | 结算时现金净实收（含押金） |
| `deposit_held_cents` | 结算时持有押金 |

---

## 7. Q11：比例来源与冻结

```text
订单快照 points_policy_snapshot → 有则严格使用
缺失 → 订单创建日生效的历史政策版本
仍不可追溯 → legacy_policy_unresolved（默认：不产生乐币退款，人工确认）
结算开始即冻结比例与输入，重试复用；不得重新读取最新政策
```

---

## 8. 争议金额隔离（P0，评审新增）

```text
deductions_total_cents       = overdue_penalty + damage + logistics
disputed_amount_cents        = 申诉中（appeal status in pending/appealing）的金额合计
undisputed_deductions_cents  = deductions_total_cents - disputed_amount_cents
```

规则：
- 仅 `undisputed` 参与 `deposit_deductions` / `other_fee_shortfall`；
- `disputed_amount_cents` 单独记录 `dispute_status` / `dispute_reference_id`（appeal id）；
- **争议金额不得视为已收取**；申诉结案后生成**补充结算**（新的 settlement 版本，冲正/差额调整）；
- 若申诉结果支持用户，则争议金额不扣；若支持平台，则在补充结算中扣除。

---

## 9. 幂等、并发与状态机（P1，含 `failed_retryable` 修正）

1. **订单级唯一最终结算**：

```sql
UNIQUE (order_id) WHERE status IN
  ('confirmed','processing','succeeded','partially_succeeded','failed_retryable')
```

- `failed_retryable` 未冲正/取消前，**不得**创建新结算版本；
2. **订单锁**：计算与执行均在订单锁内；
3. **版本化**：`settlement_version` 递增；计算快照不可变；已执行只能冲正/差额调整；
4. **状态机**：`calculated → confirmed → processing → succeeded | partially_succeeded | failed_retryable`；
5. **幂等键**：为每个资金动作**预生成** `settlement_id + action_type + source_transaction_id`（不依赖渠道返回的 channel_txn_id），
   若渠道支持幂等请求则透传该键；
6. 部分成功：按子动作重试未完成项，禁止整单重放。

---

## 10. 逾期三变量（Q14 已确认）

| 变量 | 账户 | 乐币基数 |
|---|---|---|
| `contract_rent_actual` | 租金 | 计入 |
| `overdue_rent` | 租金 | **计入**（Q14 已确认） |
| `overdue_penalty` | 额外费用 | 不计入 |

---

## 11. 续费与价格规则快照（Q15 已确认）

```text
original_term_days, original_rent_cents
renewal_term_days,  renewal_rent_cents
total_contract_days, total_contract_rent_cents
actual_days
deposit_policy = carry_over            // Q15：沿用原押金；档位调整仅收差额
pricing_policy_snapshot, pricing_policy_version   // 评审技术建议：冻结阶梯价格规则
```

`R_actual_list_cents` 按 `term_versions` 的阶梯与天数重算，必须使用合同创建/续费时冻结的价格规则。

---

## 12. `SettlementResult` 与账本字段

```text
settlement_id, settlement_version, status, calculated_at, algorithm_version
ratio_num, ratio_den, ratio_source(snapshot|history|unresolved)

R_actual_list_cents, coupon_discount_cents, R_after_coupon_cents, actual_days
rent_cash_paid_net_cents, rent_cash_due_cents, rent_refund_cents, rent_shortfall_cents
deposit_cash_collected_net_cents, deposit_held_cents, deposit_deductions_cents, deposit_refund_cents
deductions_total_cents, disputed_amount_cents, undisputed_deductions_cents, other_fee_shortfall_cents
overdue_penalty_cents, damage_fee_cents, logistics_fee_cents
gift_points_used, gift_points_cap, gift_points_consumed, gift_points_refunded
cash_refundable_cents, cash_payable_cents, net_cash_cents
```

账本子动作：`action_type`（rent_refund/deposit_refund/fee_charge/points_restore）、`payment_channel`、
`channel_txn_id`、`original_txn_id`、`executed_amount_cents|points`、`retry_count`、`status`、`idempotency_key`、`created_at`。

---

## 13. 架构

| 模块 | 职责 |
|---|---|
| `SettlementCalculator` | 纯函数：输入订单/合同/标价/券/支付分配/乐币/押金/费用/争议 → §12 结果；不写库、不调支付 |
| `SettlementExecutor` | 订单锁、唯一约束、持久化、净额或分笔执行、乐币恢复、退款、重试、冲正、审计 |

---

## 14. 测试矩阵

T1–T10（§5）+ T11–T22（v2.1）+ **T23–T26（§5 新增）**；断言要求：

- T4/T6 分账户逐项断言（非仅净额）；
- T7 仅限未交付/未起租/无取消费；
- T8 必须显式夹具（§5）；
- T15 每类费用原始金额 + 凭证；
- T23 券 + 乐币叠加/互斥（按 Q16）；
- T24 争议隔离；
- T25 `failed_retryable` 唯一性；
- T26 押金扣用后租金现金来源正确。

---

## 15. 迁移与发布

1. 历史已结算不重算，保留 `algorithm_version`；
2. `legacy_underpaid` / `legacy_policy_unresolved` / `legacy_derived_rent_cash` 显式标记；
3. 预生产回归：足额 / 历史少收 / 零押金 / 差额 / 续费 / 逾期 / 定损 / 券 / 乐币少用 / 乐币超用 / 争议 / 部分失败。

---

## 16. 决策清单（Q1–Q17，全部已确认）

| # | 问题 | 结论 |
|---|---|---|
| Q1 | 乐币基数 | ✅ **券后实际租金 `R_after_coupon_cents`**（随 Q16 更新；原「券前标价」作废） |
| Q2 | 取整 | ✅ `floor`，整数点 |
| Q3 | 精度 | ✅ 整数分 |
| Q4 | 押金扣款顺序 | ✅ 逾期 → 定损 → 物流（分类记账 + 凭证） |
| Q5 | 押金抵租金 | ✅ 不允许 |
| Q6 | 续费结算 | ✅ 同一合同累计（`term_versions`）；独立合同分别结算 |
| Q7 | 历史订单 | ✅ 不重算，保留 `algorithm_version` |
| Q8 | 新单收款 | ✅ 必须足额（§6 三口径） |
| Q9 | 乐币退款 | ✅ 原交易冲正 + 保存批次分配明细（不得仅凭 FIFO 反推） |
| Q10 | 提前取消 | ✅ 三档（未交付 / 已交付未起租 / 已起租）；T7 仅覆盖第一档 |
| Q11 | 比例来源 | ✅ 下单快照（三级 fallback + 结算冻结） |
| Q12 | 阶梯重算 | ✅ 按实际天数重算阶梯段 |
| Q13 | 净额执行 | ✅ 账务分账户；执行按渠道/资金来源/协议条件净额或分笔 |
| Q14 | `overdue_rent` | ✅ 计入乐币基数；`overdue_penalty` 不计 |
| Q15 | 续费押金 | ✅ 沿用原押金，不重复收取（档位调整仅收差额） |
| Q16 | 券 × 乐币 | ✅ 可叠加，**券在前**；上限按券后实际租金 |
| Q17 | 支付分配账本 | ✅ 列入本期（§3.1 强制从账本读取） |

---

*v2.3 by deepseek/deepseek-flash（评审 4 项必修 + Q16/Q17 业务决策）*
