# 结算退款算法规格 v2.1（草案 · 评审 P0/P1 修正）

> v2 → v2.1 变更（按 2026-11 评审意见）：
> **P0** ① 全链路分/点单位显式化（整型分子分母比例，禁止浮点）；② 守恒式改为「分」；③ Q8 足额收款拆三个现金口径。
> **P1** ④ Q11 快照缺失的三级来源与冻结；⑤ Q13 净额执行增加渠道前置条件；⑥ 订单级并发与 `partially_succeeded` 状态机；
> ⑦ 逾期三变量（`contract_rent_actual` / `overdue_rent` / `overdue_penalty`）；⑧ 续费完整性字段与押金口径。
> 架构：**`SettlementCalculator`（纯计算）+ `SettlementExecutor`（锁/落库/执行/重试/审计）** 两模块分离。
> 状态：**草案 v2.1 —— Q8/Q11/Q13/Q14/Q15 已确认；Q1–Q7/Q9/Q10/Q12 按建议值（评审确认后定稿）。**

---

## 0. 评审意见吸收对照

| 评审项 | v2.1 处理 |
|---|---|
| P0-1 单位混用（分/点） | §2 全量改为显式 `*_cents` / `*_points`；比例用 `ratio_num/ratio_den`；给出三个转换函数与 DB 字段换算 |
| P0-2 守恒式单位错误 | §4 改为 `net_cash_cents + p_refund_points*100 = ...`，并给出 invariant 代码 |
| P0-3 Q8 校验单位与口径 | §6 拆 `initial_cash_collected_cents` / `cash_paid_net_cents` / `deposit_held_cents` |
| P1-4 Q11 fallback | §7 三级来源 + 本次结算冻结比例，重试复用 |
| P1-5 Q13 执行边界 | §8 增加「渠道/资金来源/协议」前置条件；否则按原交易分别执行，账务仍等于 `net_cash` |
| P1-6 幂等/并发 | §9 订单级唯一最终结算 + 锁 + 版本 + `partially_succeeded` + 冲正 |
| P1-7 逾期规则 | §10 三变量拆分；**Q14 已确认**：`overdue_rent` 计入乐币基数、`overdue_penalty` 不计 |
| P1-8 续费完整性 | §11 补 `original/renewal` 天数与金额；**Q15 已确认**：续费沿用原押金 |
| P2 细节 | §12 结果/账本字段；§5 T4/T6 分账户断言；§5 T7 条件；§14 T15 凭证；§15 券测试保留（系统有 `/coupons/validate`） |
| 日期一致性 | 全文改为「2026-11 评审确认」，不再写具体日 |

---

## 1. 业务原则（评审确认）

| # | 原则 |
|---|---|
| P1 | **押金与租金完全分离**：押金是保证金，不是收入/折扣，不参与租金折算、不作乐币基数 |
| P2 | **实际租金按标价与实际计费天数重算**：禁止用「现金实付 ÷ 合同标价」倒推日租 |
| P3 | **乐币是有比例上限的租金优惠权益**：比例来自下单快照（`points_policy_snapshot`），结算按实际租金重算上限 |
| P4 | **乐币只退超额**：`p_consumed = min(p_used, cap)`；少用不补发、不折现 |
| P5 | **额外费用独立核算**：逾期 / 定损 / 物流；可扣押金，不足走补缴；押金不得抵扣租金缺口 |

---

## 2. 单位、类型与换算（P0）

### 2.1 单位契约

| 量 | 单位 | 类型/字段 |
|---|---|---|
| 现金 | **分（cents）** | `*_cents int64` |
| 乐币 | **点（points）**，1 点 = 1 元 = 100 分 | `*_points int64` |
| 比例 | 整数分子/分母（如 30% → `3/10`） | `ratio_num/ratio_den int64` |
| 天数 | 整数天 | `int` |

> 禁止在计算链路出现 `float64` 金额/比例；展示层再转元与两位小数。

### 2.2 强制转换函数（唯一出口）

```go
// PointsFromStoredCents: orders.gift_points_used 以 Cents 存储（900 点 → 90000 Cents）
func PointsFromStoredCents(v int64) (int64, error) {
    if v%100 != 0 { return 0, ErrPointsNotWhole }
    return v / 100, nil
}
func StoredCentsFromPoints(p int64) int64 { return p * 100 } // 写库用
func PointsToCashCents(p int64) int64     { return p * 100 } // 点 → 分
```

- 业务代码**禁止**直接把 `gift_points_used` 当点数使用；
- 禁止自行 `*100` / `/100`（除上述三函数外）。

### 2.3 比例

- 来源：`orders.points_policy_snapshot`（见 §7）；
- 存储：`ratio_num` / `ratio_den`（如 `3/10`）；**不得**存 `0.3` 浮点后回算。

---

## 3. 分账户算法（全部显式单位）

```text
输入：
  R_actual_cents            // 实际标价租金（阶梯重算）
  cash_paid_cents           // 现金净实收（含押金）
  deposit_held_cents        // 结算时实际持有的押金
  p_used_points             // 已用乐币（由 gift_points_used Cents 换算）
  ratio_num, ratio_den      // 下单快照比例
  deductions_cents          // 逾期费 + 定损 + 物流

① 乐币
  cap_points        = floor( R_actual_cents * ratio_num / (100 * ratio_den) )
  p_consumed_points = min(p_used_points, cap_points)
  p_refund_points   = p_used_points - p_consumed_points

② 租金账户（现金，分）
  rent_cash_paid_cents  = cash_paid_cents - deposit_held_cents
  rent_cash_due_cents   = R_actual_cents - PointsToCashCents(p_consumed_points)
  rent_balance_cents    = rent_cash_paid_cents - rent_cash_due_cents
  rent_refund_cents     = max(0, rent_balance_cents)
  rent_shortfall_cents  = max(0, -rent_balance_cents)

③ 押金账户（分）
  deposit_deductions_cents  = min(deposit_held_cents, deductions_cents)
  deposit_refund_cents      = deposit_held_cents - deposit_deductions_cents
  other_fee_shortfall_cents = max(0, deductions_cents - deposit_held_cents)

④ 汇总（执行层）
  cash_refundable_cents = rent_refund_cents + deposit_refund_cents
  cash_payable_cents    = rent_shortfall_cents + other_fee_shortfall_cents
  net_cash_cents        = cash_refundable_cents - cash_payable_cents
```

> 校验示例（ratio=3/10，R=280000 分）：`cap = 280000*3/(100*10) = 840` 点；
> `rent_cash_due = 280000 − 84000 = 196000` 分（¥1960）。

---

## 4. 守恒式（以分为单位，P0）

```text
net_cash_cents + PointsToCashCents(p_refund_points)
  = cash_paid_cents + PointsToCashCents(p_used_points) - R_actual_cents - deductions_cents
```

实现为独立 invariant 检查（不得只在文档里）：

```go
if netCashCents+pointsToCashCents(pRefundPoints) !=
   cashPaidCents+pointsToCashCents(pUsedPoints)-actualRentCents-deductionsCents {
    return ErrSettlementInvariantViolation
}
```

> 注意：真实实现还需处理「此前已退款/已补缴」「争议挂起」「部分执行」——上述等式是**首次最终结算**的基准，
> 历史/多次结算必须在 §9 的版本与冲正机制下计算，不可直接套用。

---

## 5. 推演（合同 ¥100/天 × 30 天，押金 ¥500，ratio=3/10）

| # | 场景 | cash_paid(分) | p_used(点) | 天数 | R_actual(分) | cap | p_refund | rent 退/补(分) | 押金退(分) | 净现金(分) | 乐币退(点) |
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

**断言要求（评审 P2）**：
- T4 必须分别断言 `rent_shortfall=46000` / `deposit_refund=50000` / `p_refund=160` / `net_cash=4000`，**不得只断净额**；
- T6 必须分别断言 `rent_refund=14000` / `other_fee_shortfall=30000` / `p_refund=60` / `net_cash=−16000`；
- T7 仅适用于「未交付、未起租、无取消费用」；已交付/已起租见 Q10。

---

## 6. Q8：足额收款校验（三个现金口径，P0）

| 概念 | 含义 | 用途 |
|---|---|---|
| `initial_cash_collected_cents` | **下单时**首次实收现金（含押金） | 校验足额：`= R_list_cents − PointsToCashCents(p_used) + deposit_cents` |
| `cash_paid_net_cents` | 截至**结算时**现金净实收（扣已退款） | 租金账户计算用 |
| `deposit_held_cents` | 结算时实际持有的押金（可能因部分退还而小于初始） | 押金账户计算用 |

规则：
- 新单（`algorithm_version>=2`）必须满足 `initial_cash_collected_cents` 等式，否则**拒绝下单/支付**；
- 不得用 `cash_paid_net_cents` 与初始应收比较（会被历史退款污染）；
- 历史单不满足 → 标 `legacy_underpaid`，按实际 `cash_paid_net_cents` 结算（T4 类）。

---

## 7. Q11：比例来源与冻结（P1）

```text
1) orders.points_policy_snapshot 存在 → 严格使用（ratio_num/ratio_den 直接取自快照）
2) 快照缺失 → 查「订单创建日期当时生效」的历史政策版本
3) 仍不可追溯 → 状态 legacy_policy_unresolved，按显式兼容规则处理（默认：不产生乐币退款，需人工确认）
4) 一旦本次结算开始计算，比例与输入快照写入 settlement_calculations，重试必须复用，不得重新读取最新政策
```

---

## 8. Q13：净额执行的前置条件（P1）

| 层 | 规则 |
|---|---|
| 账务核算 | **永远分账户**（租金 / 押金 / 费用），不受执行方式影响 |
| 用户结算 | 计算 `net_cash_cents` 作为「一次收付」的目标金额 |
| 实际支付 | **仅当** ①支付渠道支持（同源可退/可收）②资金来源允许 ③协议授权 时，才执行一次净额收付；否则按**原交易/原渠道分别执行**（如原路退租金 + 新单收补缴），执行完毕后账务合计必须等于 `net_cash_cents` |

> 行业对照仅作设计参考，不构成合同授权或渠道可行性的结论；最终以微信支付规则与《租用服务协议》条款为准。

---

## 9. 幂等、并发与状态机（P1）

1. **订单级唯一最终结算**：`unique(order_id) where status in ('confirmed','processing','succeeded','partially_succeeded')`；
   同一订单不得存在两个有效最终结算版本。
2. **订单级锁**：计算与执行均在 `SELECT ... FOR UPDATE` 或分布式锁内进行。
3. **版本化**：`settlement_version` 单调递增；计算快照不可变；已执行版本只能通过**冲正/差额调整**修订。
4. **状态机**：

```text
calculated → confirmed → processing → succeeded
                                 ├── partially_succeeded
                                 └── failed_retryable → processing ...
```

5. `partially_succeeded`（如现金退款成功、乐币恢复失败）：**禁止整单重放**；按子动作状态机重试未完成项。
6. 幂等键：`(order_id, settlement_id, action_type, channel_txn_id?)`；子动作各自唯一。

---

## 10. 逾期：三个变量（P1，评审 §7）

| 变量 | 含义 | 账户 | 乐币基数 |
|---|---|---|---|
| `contract_rent_actual` | 合同期内实际天数租金（标价） | 租金账户 | 计入 |
| `overdue_rent` | 逾期期间按日租产生的**额外租金** | 租金账户 | **计入（Q14 已确认）** |
| `overdue_penalty` | 逾期**罚金**（违约性质） | 额外费用账户 | 不计入 |

- 示例：合同 30 天 / 实际 35 天 / 日租 100 / 罚金 50 → 方案 A：租金 3500 + 罚金 250；方案 B：租金 3000 + 罚金 250。
  **Q14 已确认：`overdue_rent` 计入乐币基数（属租金），`overdue_penalty` 不计入。** 采用方案 A 的租金口径。

---

## 11. 续费完整性（P1，评审 §8）

续费单必须落库并可复算：

```text
original_term_days, original_rent_cents
renewal_term_days,  renewal_rent_cents        （可多次，按 term_versions 累加）
total_contract_days, total_contract_rent_cents
deposit_policy: carry_over            // Q15 已确认：沿用原押金（仅档位调整时收差额）
actual_days
```

- `R_actual` = 按 `term_versions` 的阶梯与天数重算（非按日均摊）；
- **Q15 已确认：续费沿用原押金，不重复收取**；若档位调整（如补差价升级差额档），仅收差额部分并更新 `deposit_held`。

---

## 12. `SettlementResult`（P2 补充字段）

```text
settlement_id, settlement_version, status, calculated_at, algorithm_version
ratio_num, ratio_den, ratio_source(snapshot|history|unresolved)

R_list_cents, R_actual_cents, actual_days
deposit_collected_cents, deposit_held_cents, deposit_deductions_cents, deposit_refund_cents
gift_points_used, gift_points_cap, gift_points_consumed, gift_points_refunded
rent_cash_paid_cents, rent_cash_due_cents, rent_refund_cents, rent_shortfall_cents
other_fee_total_cents, other_fee_shortfall_cents
cash_refundable_cents, cash_payable_cents, net_cash_cents
```

账本子动作字段：`action_type`（rent_refund/deposit_refund/fee_charge/points_restore）、`payment_channel`、
`channel_txn_id`、`original_txn_id`、`executed_amount_cents|points`、`retry_count`、`status`、`created_at`。

---

## 13. 架构（评审建议）

| 模块 | 职责 | 禁止 |
|---|---|---|
| `SettlementCalculator` | 纯函数：输入订单/合同/标价/支付/乐币/押金/费用 → 输出 §12 结构 | 不改余额、不调支付、不写库 |
| `SettlementExecutor` | 订单锁、结算持久化、净额或分笔执行、乐币恢复、退款、重试、审计 | 不做金额计算 |

---

## 14. 测试矩阵（P2 细节）

| 编号 | 场景 | 断言要点 |
|---|---|---|
| T1–T10 | §5 | 分账户逐项断言 + 守恒式（分） |
| T11 | 正常履约 | 押金全额退、乐币消耗 = min |
| T12 | 逾期 | `contract_rent_actual` / `overdue_rent` / `overdue_penalty` 三值分别断言（Q14 确定后） |
| T13 | 定损 < 押金 | `deposit_refund = deposit − deductions` |
| T14 | 定损 > 押金 | `other_fee_shortfall` 进入 `cash_payable` |
| T15 | 多费用并发 | 扣款顺序 + **每类费用原始金额与凭证**（`damage_reports` / 物流单） |
| T16 | 乐币少用 | `p_refund=0`，不折现 |
| T17 | 乐币超用 | `p_refund=p_used−cap` |
| T18 | 续费后提前归还 | §11 字段齐全 + `R_actual` 按版本重算 |
| T19 | 跨阶梯 | 阶梯段按实际天数重算 |
| T20 | 重复结算请求 | 订单级唯一 + 幂等 |
| T21 | 已结算再结算 | 返回既有版本 / 冲正 |
| T22 | 并发结算 | 锁 + 唯一键 + `partially_succeeded` 恢复 |

> 优惠券：系统存在 `/coupons/validate` 与 prepay `coupon_code`，**保留**券场景回归，并要求券后租金作为 `R_actual`（券与乐币上限关系见 Q1）。

---

## 15. 迁移与发布

1. 历史已结算订单**不重算**，保留 `algorithm_version`；
2. 新算法仅对新结算生效；`legacy_underpaid` / `legacy_policy_unresolved` 明确标记；
3. 预生产回归：足额单 / 历史少收 / 零押金 / 差额押金 / 续费 / 逾期 / 定损 / 优惠券 / 乐币少用 / 乐币超用。

---

## 16. 待确认（Q1–Q15）

| # | 问题 | 建议 |
|---|---|---|
| Q1 | 乐币基数 | 实际标价租金（不含押金、不含券） |
| Q2 | 取整 | `floor`，整数点 |
| Q3 | 精度 | 整数分 |
| Q4 | 押金扣款顺序 | 逾期 → 定损 → 物流 |
| Q5 | 押金抵租金 | 不允许 |
| Q6 | 续费结算 | 同一合同累计；独立合同分别结算 |
| Q7 | 历史订单 | 不重算 |
| Q8 | 新单收款 | ✅ 已确认：必须足额（§6 三个口径） |
| Q9 | 乐币退款 | 原交易冲正；**保存每次消费的批次分配明细**，不得仅凭 FIFO 反推 |
| Q10 | 提前取消 | 未交付 / 已交付未起租 / 已起租 三档；T7 仅覆盖第一档 |
| Q11 | 比例来源 | ✅ 已确认：下单快照（§7 三级 fallback + 冻结） |
| Q12 | 阶梯重算 | 按实际天数重算阶梯段 |
| Q13 | 净额执行 | ✅ 已确认：账务分账户，执行按渠道条件净额或分笔（§8） |
| Q14 | `overdue_rent` 是否计入乐币基数 | ✅ 已确认：**计入**（属租金）；`overdue_penalty` 不计 |
| Q15 | 续费押金 | ✅ 已确认：**沿用原押金**，不重复收取；档位调整仅收差额 |

---

## 17. 与现行实现的差异（改造范围）

| 位置 | 现行 | v2.1 |
|---|---|---|
| `user_settlement.go` | 单一 `paidTotal`（含押金）与 `discountedDue` 求净额 | `SettlementCalculator` 分账户（分/点显式） |
| `segment_settlement.go` | `rate = paid/original` 折后日租 | 标价日租；paid 仅用于账户核对 |
| 支付端乐币上限 | 按「支付金额（含押金）×ratio」浮点 | 按「合同标价租金 × 整型比例」，点/分显式 |
| 结算落库 | 单一 refund 金额 | §12 结构 + 版本 + 子动作账本 + 幂等 |
| 退款执行 | 一次性现金退款 | `SettlementExecutor`：锁 → 落库 → 净额或分笔 → 重试 → 审计 |

*v2.1 draft by deepseek/deepseek-flash（按 2026-11 评审 P0/P1 修正）*
