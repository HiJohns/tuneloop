# 结算退款算法规格 v2（草案 · 按评审意见修订）

> v1 → v2 变更：采用**分账户结算**（租金账户 / 押金账户 / 额外费用账户），不再用单一 `cash_refund` 净额公式；
> 乐币规则按评审 §3.1 固定；补 T11–T22 覆盖与持久化/幂等要求。
> 来源：#2339 结算三例失败；关联 `docs/cases/settlement-accounting-rules.md`（#2268 P1，§1 `cash_paid` 含押金）。
> 评审：2026-11-15 专家评审报告（有条件通过，v1 需修订后再开发）。
> 状态：**草案 v2，Q1–Q13 未确认前不得据此改码**。

---

## 0. v1 → v2 变更对照（评审意见吸收）

| 评审意见 | v2 处理 |
|---|---|
| 现金模型需拆租金/押金/费用三账户 | §3 全面改为分账户公式，`cash_refundable`/`cash_payable` 由账户结果汇总 |
| 乐币公式 `p_consumed = min(p_used, cap)` | §2.3 采用；退款只针对超额部分 |
| 金额统一整数分、乐币整数点 | §2.1 明确单位；§6 结构体字段全部整数 |
| 结算结果结构化 + `algorithm_version` | §6 `SettlementResult` 字段表 |
| 幂等与账本（退款/退点/押金分别记账） | §7 幂等键、持久化后执行、失败可恢复 |
| 历史订单不重算 | §8 迁移策略 |
| 补 T11–T22 | §5.2 覆盖矩阵 |
| 乐币比例快照 vs 最新 | Q11（建议：下单快照） |
| 阶梯租金提前归还 | Q12（建议：按实际天数重算阶梯段） |

---

## 1. 业务原则（评审确认）

| # | 原则 |
|---|---|
| P1 | **押金与租金完全分离**：押金是保证金，不是收入、不是折扣；不参与租金折算，不作乐币基数 |
| P2 | **实际租金按标价与实际计费天数重算**：禁止用「现金实付 ÷ 合同标价」倒推日租 |
| P3 | **乐币是有比例上限的租金优惠权益**：比例来自 `gift_policies.pay_ratio`（可配置，禁止硬编码）；支付端按合同租金定额度，结算端按实际租金重算 |
| P4 | **乐币只退超额**：`p_consumed = min(p_used, cap)`；少用不补发、不折现 |
| P5 | **额外费用独立核算**：逾期 / 定损 / 物流，可扣押金，不足走补缴；**押金不得抵扣租金缺口** |

---

## 2. 符号、单位与乐币算法

### 2.1 单位

- 金额：**整数分**（`int64`），禁止浮点累加；
- 乐币：**整数点**，1 点 = 1 元（100 分），与 `orders.gift_points_used`（Cents 存储）同口径；
- 比例：有理数（如 30% → `0.3`），来自快照，禁止运行时改算历史单。

### 2.2 符号

| 符号 | 含义 | 来源 |
|---|---|---|
| `cash_paid` | 现金净实收（**含押金**、不含乐币） | `orders.cash_paid` |
| `deposit_collected` | 押金实收（全额档或差额档） | `orders.deposit` |
| `p_used` | 下单时实际使用乐币 | `orders.gift_points_used` |
| `ratio` | 乐币抵扣比例（**下单快照**） | `orders.points_policy_snapshot` |
| `R_list` | 合同标价租金 | `pricing_breakdown` / `term_versions` |
| `actualDays` | 实际计费天数 | 结算入参 |
| `R_actual` | 实际标价租金（按同一阶梯规则重算） | 计算值 |
| `deductions` | 额外费用合计 = 逾期 + 定损 + 物流 | `damage_reports` 等 |

### 2.3 乐币

```text
cap        = floor(R_actual × ratio)          // 分→点：R_actual 先转点（÷100）再乘比例
p_consumed = min(p_used, cap)
p_refund   = p_used − p_consumed              // = max(0, p_used − cap)
```

---

## 3. 分账户结算（v2 核心）

### 3.1 租金账户（现金部分）

```text
rent_cash_paid     = cash_paid − deposit_collected
rent_cash_due      = R_actual − p_consumed        // 乐币按点折元抵扣租金
rent_cash_balance  = rent_cash_paid − rent_cash_due
rent_cash_refund   = max(0, rent_cash_balance)
rent_cash_shortfall= max(0, −rent_cash_balance)
```

### 3.2 押金账户

```text
deposit_deductions  = min(deposit_collected, deductions)   // 扣款顺序见 Q4
deposit_refund      = deposit_collected − deposit_deductions
other_fee_shortfall = max(0, deductions − deposit_collected)
```

### 3.3 汇总

```text
cash_refundable = rent_cash_refund + deposit_refund
cash_payable    = rent_cash_shortfall + other_fee_shortfall
net_cash        = cash_refundable − cash_payable      // 仅用于「一次性收/付」展示与执行
```

> **净额抵销只发生在展示/执行层**：核对与审计始终以分账户结果为准（评审 §4.2）。

### 3.4 守恒式（实现必须满足）

```text
net_cash + p_refund = cash_paid + p_used − R_actual − deductions
```

---

## 4. 推演（合同 ¥100/天 × 30 天，押金 ¥500）

### 4.1 统一使用配置比例 `ratio = 0.3`（推荐作为验收口径）

| # | 场景 | cash_paid | p_used | 天数 | R_actual | cap | p_refund | rent 退款(+) / 补缴(−) | 押金退 | 净现金 |
|---|---|---|---|---|---|---|---|---|---|---|
| T1 | 正常履约 30 天 | 2600 | 900 | 30 | 3000 | 900 | 0 | 0 | 500 | 退 500 |
| T2 | 提前 28 天 | 2600 | 900 | 28 | 2800 | 840 | 60 | +140 | 500 | 退 640（另退 60 点） |
| T3 | 提前 28 天，少用点 | 3000 | 500 | 28 | 2800 | 840 | 0 | +200 | 500 | 退 700 |
| T4 | 提前 28 天，历史少收 + 超用点 | 2000 | 1000 | 28 | 2800 | 840 | 160 | **−460** | 500 | 净退 40（另退 160 点） |
| T5 | T2 + 定损 300 | 2600 | 900 | 28 | 2800 | 840 | 60 | +140 | 200 | 退 340（另退 60 点） |
| T6 | T2 + 定损 800 | 2600 | 900 | 28 | 2800 | 840 | 60 | +140 | 0（不足 300） | 净补 160（另退 60 点） |
| T7 | 当日取消（未计费） | 2600 | 900 | 0 | 0 | 0 | 900 | +2100 | 500 | 退 2600（另退 900 点） |
| T8 | 续费 +30 天，实际 45 天 | 5600 | 900 | 45 | 4500 | 1350 | 0 | +1500 | 500 | 退 2000 |
| T9 | 零押金乐器（T2 条件） | 2100 | 900 | 28 | 2800 | 840 | 60 | +140 | 0 | 退 140（另退 60 点） |
| T10 | 差额押金 ¥30（T2 条件） | 2130 | 900 | 28 | 2800 | 840 | 60 | +140 | 30 | 退 170（另退 60 点） |

> 说明：
> 1. 本表统一使用配置比例 `ratio = 0.3`（合同上限 = floor(3000×0.3) = 900 点）。T1–T3、T5–T10 均**足额收款**：
>    `cash_paid = R_list − p_used + deposit`（Q8）。
> 2. **T4 是历史特例**（`cash_paid=2000` 且 `p_used=1000`）：既少收 500，又超出合同上限 900 → 结算同时出现
>    「租金现金补缴 460」与「押金退还 500」，净退 40 + 退点 160 = **合计 200**（正是 #2339 测试期望的总额）。
> 3. 评审报告 §5 使用的是 `ratio = 1/3`，同一算法；换算示例：截止 28 天时 cap=933、退点 67、
>    rent 退款 133、押金 500 → 现金 633（对应评审表第 2 行）。

### 4.2 评审报告表格的两处口径待澄清

| 位置 | 现象 | 建议 |
|---|---|---|
| 评审 §5 第 4 行（cash 2000 / p 1000） | 结论「租金现金补缴 **460**」用的是 `ratio=0.3`（cap 840 → due 1960 → 差 460），同表其余行用「1/3」（cap 933 → 退 67） | 全表统一为配置值（建议 0.3），或在表头标注每行比例 |
| 评审 §5 定损 800 行 | 结论「补缴 300」未体现租金退款 40 的抵销 → 若净额执行应为「净补 260」 | v2 采用分账户明细 + `net_cash` 展示；需确认执行时是否净额抵销（Q13） |

---

## 5. 测试覆盖

### 5.1 结算三例（#2339 验收口径）

| 测试 | 期望（v2，ratio=0.3） |
|---|---|
| `TestRefundDiff_PointsOverCap` | T4 口径：rent 补缴 460 + 押金退 500 → **净退 40 现金 + 退 160 点 = 200**（与现有测试总额一致，但必须按分账户展示；现实现 666.67/506.67 仍错） |
| `TestRefundDiff_PointsWithinCap` | 按 T3 口径（净退 700） |
| `TestSettlementFlow` | 按 §3 重算（押金单列 + 标价计费） |

> 关键：该夹具 `cash_paid=2000` 是**历史少收**（租金现金 1500 < 应付 1960），因此正确结果不是 v1 写的「退 200」，
> 而是「租金补缴 460 + 押金退 500 → 净退 40」并按 160 点退乐币；总额 200 与测试一致。

### 5.2 补充覆盖（评审 T11–T22）

| 编号 | 场景 | 断言要点 |
|---|---|---|
| T11 | 合同期正常归还 | 押金全额退、乐币消耗 = min(p_used, cap) |
| T12 | 逾期归还 | 逾期租金（按合同天数）+ 逾期费，两者不混 |
| T13 | 定损 < 押金 | `deposit_refund = deposit − deductions` |
| T14 | 定损 > 押金 | `other_fee_shortfall` 进入 `cash_payable` |
| T15 | 多费用并发 | 扣款顺序（逾期→定损→物流）与合计 |
| T16 | 乐币少用 | `p_refund = 0`，不折现 |
| T17 | 乐币超用 | `p_refund = p_used − cap` |
| T18 | 续费后提前归还 | 按 `term_versions` 累计 `R_actual` |
| T19 | 跨阶梯档位 | 阶梯段按实际天数重算 |
| T20 | 重复提交结算 | 幂等：不重复退款/退点 |
| T21 | 已结算再次结算 | 返回既有结果 |
| T22 | 并发结算 | 唯一键 + 事务，无重复扣款 |

---

## 6. `SettlementResult`（结构化输出，整数分/点）

```text
rent_list, rent_actual
deposit_collected, deposit_deductions, deposit_refund
gift_points_used, gift_points_cap, gift_points_consumed, gift_points_refunded
rent_cash_paid, rent_cash_due, rent_cash_refund, rent_cash_shortfall
other_fee_total, other_fee_shortfall
cash_refundable, cash_payable, net_cash
algorithm_version, ratio_snapshot, actual_days
```

持久化建议：`settlement_calculations` 增加 `algorithm_version` / `ratio_snapshot` / 分账户字段（或 JSON 明细），历史行保留原版本号，不重算。

---

## 7. 幂等与账本

- 每类资金动作独立记账：租金退款、押金退款、额外费用补缴、乐币恢复；
- 记录字段：`order_id` / `settlement_id` / `type` / `amount|points` / `source_txn_id` / `idempotency_key` / `status` / `created_at`；
- 流程：**先持久化结算结果 → 再执行外部退款/退点**，失败可重试（状态机 `pending → succeeded/failed`）；
- 幂等键：`(order_id, settlement_id, type)`；并发由唯一键保证（沿用 #2282 模式）。

---

## 8. 迁移与发布

1. **历史已结算订单不重算**；保留 `algorithm_version` 与原始明细；
2. 新算法仅对新结算生效；如需修正历史单，另立数据修复 Issue（dry-run → 授权 → 分批）；
3. 预生产回归场景：足额单 / 历史少收单 / 零押金 / 差额押金 / 续费 / 逾期 / 定损 / 券 / 乐币少用 / 乐币超用。

---

## 9. 待确认（Q1–Q13）

| # | 问题 | 建议 | 状态 |
|---|---|---|---|
| Q1 | 乐币上限基数 | 实际标价租金（不含押金、不含券） | ⬜ |
| Q2 | 乐币取整 | `floor`，整数点 | ⬜ |
| Q3 | 现金精度 | 整数分，展示两位小数 | ⬜ |
| Q4 | 押金扣款顺序 | 逾期 → 定损 → 物流（产品政策最终确认） | ⬜ |
| Q5 | 押金是否抵扣租金缺口 | **不允许**（独立账户） | ⬜ |
| Q6 | 续费结算 | 同一合同累计；独立合同分别结算 | ⬜ |
| Q7 | 历史订单 | 不自动重算 | ⬜ |
| Q8 | 新单收款 | 必须足额：`cash_paid = R_list − p_used + deposit` | ⬜ |
| Q9 | 乐币退款属性 | 原交易冲正（恢复到原批次，FIFO 反向） | ⬜ |
| Q10 | 提前取消 | 未交付 / 已交付未起租 / 已起租 三档分别定义 | ⬜ |
| Q11 | 乐币比例取值 | **下单快照**（`points_policy_snapshot`），不随政策变更 | ⬜ |
| Q12 | 阶梯租金提前归还 | 按实际天数重算阶梯段（复用 `ComputeTierSegments`） | ⬜ |
| Q13 | 退款执行是否净额抵销 | 账单分账户明细；执行按 `net_cash` 一次性收付 | ⬜ |

---

## 10. 与现行实现的差异（改造范围）

| 位置 | 现行 | v2 |
|---|---|---|
| `user_settlement.go` | 单一 `paidTotal`（含押金）与 `discountedDue` 求净额 | 拆 §3 三账户 |
| `segment_settlement.go` | `rate = paid/original` 折后日租 | 按标价日租；paid 仅用于账户核对 |
| 支付端乐币上限 | 按「支付金额（含押金）×ratio」 | 按「合同标价租金 ×ratio」 |
| 结算落库 | 单一 refund 金额 | 分账户明细 + `algorithm_version` + 幂等键 |
| 退款执行 | 一次性现金退款 | 现金退款 / 押金退款 / 乐币恢复分别记账，可重试 |

*Draft v2 by deepseek/deepseek-flash, 2026-11-16（按 2026-11-15 评审报告修订）*
