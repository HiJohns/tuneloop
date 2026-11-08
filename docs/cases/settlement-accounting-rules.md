# 结算记账口径裁定（#2268 P1）

> 依据：`docs/ops/settlement-reconciliation-2268.md`（预生产快照只读对账，2026-10-23）与 develop 代码事实。
> 标记：**[已由代码事实确定]** = 与现有代码/数据一致；**[已确认 2026-10-24]** = 用户已书面确认，可直接作为执行口径；**[待用户确认]** = 尚未确认。
>
> **确认记录**：2026-10-24 用户确认 §1–§8 全部口径（净额、乐币不双扣、term_version、补缴含超收退款、Cents + dry-run 分批、返点五项、逾期费、P2a→P5 顺序），执行 Issue 见 §9（#2278–#2283）。

## 0. 统一符号

- `Paid` = `Σ order_payment_records(type='payment', status='paid').amount`
- `Refunded` = `Σ order_refund_records(status='refunded').amount`
- `CashPaid` = `orders.cash_paid`
- `RentPayable` = 应收租金（结算重算值）；`CashBasis` = 现金计费基数（元）
- 守恒式（P0 已用其判差异）：**`Paid − Refunded = CashPaid + 未回写项`**；目标状态为 `Paid − Refunded = CashPaid`。

## 1. `cash_paid` 语义

- **裁定**：`cash_paid` = **现金净实收**（含押金与运费；不含乐币/赠点；**扣除已退款**）。
- 依据：创建/回调路径均按「金额−乐币」写入；P0 发现 5 单退款未回写，说明目标语义应为净额。
- 状态：**[已确认 2026-10-24]**（采用净额；P2a 在退款成功路径同步扣减）。

## 2. `record.Amount` 与乐币

- **裁定**：prepay 时已扣乐币/优惠；回调**不得**再从 `cash_paid` 扣 gift。乐币只体现在 `gift_points_used` 与 `point_batch_consumptions`（FIFO 留痕）。
- 依据：`rent` 路径 `cash_paid = record.Amount` 覆盖写，实际未重复扣；但 `renewal/damage/shortfall/loss` 不覆盖，存在重复扣风险（C2）。
- 状态：**[已确认 2026-10-24]**：所有路径统一「预付/扣款时已扣乐币，支付回调不再重复扣减」；rent 路径为现状基线，renewal/damage/shortfall/loss 按 P2b 修复。

## 3. 续租计价版本

- **裁定**：`orders.pricing_breakdown` 视为**创建时快照，只读**；续租写**追加式** `term_version`（天数/档位/金额/支付记录 ID），结算按版本区间求和。
- 依据：当前续租重写 `pricing_breakdown`，导致历史区间不可复算。
- 状态：**[已确认 2026-10-24 / 已实现]**：
  - 续租仅追加 `order_term_versions`（`renewal.go` 不再改写 `pricing_breakdown`）；
  - `computeSettlement` 有版本时按版本合计覆盖天数/合同天数，日均 = Σtotal/Σdays；无版本回退只读快照；
  - 损失/定损日租读取同步版本加权日均；
  - 守卫：`TestOrderTermVersion_MultiRenewalImmutable_2282`（30+30+30，`pricing_breakdown` 逐字节不变）、`TestRenewal1997_*`。

## 3.1 结算幂等（#2282 P4b，2026-10-26 补齐）

- `settlement_calculations.ledger_version` = 账本条目数（paid + refunded）；唯一键 `(order_id, trigger, ledger_version)`。
- 重复 confirm/preview（账本未变）→ `ON CONFLICT DO NOTHING` 并返回既有 calculation，仅 1 次退款/返点；并发由唯一键兜底。
- 守卫：`TestSettlementCalculation_LedgerVersion_2282`（版本递增/既有行返回）、`TestSettlementCalculation_ConcurrentIdempotent_2282`（10 并发仅 1 行）。

## 4. 补缴闭环（`payment_shortfall`）

- **裁定**：到账 → `cash_paid += record.Amount`；关闭 pending 记录；触发结算重算与 `executeRefund`（含超收退款）；幂等键 `(order_id, order_type, out_trade_no)`。
- 依据：`wechatpay_callback.go:357-392` 已有分支但缺累加/退款触发；P0 实证 `YL20260914-003` 差 5 分。
- 状态：**[已确认 2026-10-24]**（P2a 执行；幂等键 + 超收退款）。

## 5. `total_spending` 单位

- **裁定**：统一写 **Cents**（`models.FromYuan(CashBasis)`）；存量修复须先 dry-run 报告并经用户书面授权。
- 依据：P0 实证 `user fe6fd4fa… total_spending=67` 而 `rent_sum=6,700` 分（¥67）→ 低 100 倍；前端 `/100` 展示。
- 状态：**[已确认 2026-10-24]**：单位统一 Cents；存量修复按「先 dry-run 评估 → 书面授权 → 分批执行」（P2c 出评估，P5 执行）。

## 6. 返点（介绍人 / 消费返点）

逐项裁定：

| 项 | 裁定 | 状态 |
|---|---|---|
| 基数 | 以**实收现金** `CashBasis` 为基数（与 `total_spending` 同源），而非应收 `RentPayable` | [已确认 2026-10-24] |
| 时点 | 结算成功时发放（非支付到账时） | [已确认 2026-10-24] |
| 幂等 | 同一订单结算重试不得重复发放（唯一键 `order_id + source`） | [已确认 2026-10-24] |
| 等级取值 | **下单时**会员等级（非结算时） | [已确认 2026-10-24] |
| 0 现金订单 | **不发/发 0**（乐币全额支付无现金毛利） | [已确认 2026-10-24 / 已实现] |

实现：`referral_commissions` 唯一键 `(order_id)` 幂等；`orders.referral_level_id` 快照；`result.CashBasis <= 0` 直接跳过。守卫：`TestReferralCommission_Idempotent_2282` / `_LevelSnapshot_2282` / `TestMatrix_ReferralOnZeroCashOrder_2268`。

## 7. 逾期费口径

- **裁定**：以退库检查 `damage_reports.overdue_fee` 为准（一次性），不按日累加。
- 依据：`user_settlement.go:723-727` 读取 `DamageReport.OverdueFee`；P0 场景 5 逾期为 0。
- 状态：**[已由代码事实确定]**。

## 8. 确认清单（2026-10-24 已全部确认）

1. ✅ `cash_paid` 采用**净额**语义（§1）。
2. ✅ 非 rent 路径：回调不重复扣乐币（§2）。
3. ✅ 续租 `term_version` 追加式、快照不可变（§3）。
4. ✅ 补缴闭环含「超收退款」+ 幂等（§4）。
5. ✅ `total_spending` 统一 Cents；存量 dry-run → 书面授权 → 分批（§5）。
6. ✅ 返点五项口径（§6，见上表）。
7. ✅ 按 P2a→P2b→P2c→P3→P4→P5 顺序拆 Issue（§9）。

## 9. 后续执行 Issue（已建，Parent: #2268）

- **#2278** P2a 补缴入账 + 退款回写 + 结算重算/退款 + 幂等
- **#2279** P2b 乐币二次扣减（renewal/damage/shortfall/loss 端到端矩阵）
- **#2280** P2c `total_spending` 单位 + 存量修复评估
- **#2281** P3 单一账本（`order_payment_records` + `order_refund_records` 为唯一事实源，聚合可重算）
- **#2282** P4 快照不可变 / 续租 `term_version` / 结算纯函数 + 幂等
- **#2283** P5 历史数据回填（用户书面授权，先 dry-run）

每个执行 Issue 的验收：组合矩阵行级断言 `Paid − Refunded = CashPaid`（缺陷版 FAIL → 修复版 PASS）+ ≥3 笔真实订单 replay。
