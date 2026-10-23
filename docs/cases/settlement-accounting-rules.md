# 结算记账口径裁定（#2268 P1）

> 依据：`docs/ops/settlement-reconciliation-2268.md`（预生产快照只读对账，2026-10-23）与 develop 代码事实。
> 标记：**[已由代码事实确定]** = 与现有代码/数据一致，可直接作为基线；**[待用户确认]** = 产品口径选择，确认后另立执行 Issue（P2–P5）。

## 0. 统一符号

- `Paid` = `Σ order_payment_records(type='payment', status='paid').amount`
- `Refunded` = `Σ order_refund_records(status='refunded').amount`
- `CashPaid` = `orders.cash_paid`
- `RentPayable` = 应收租金（结算重算值）；`CashBasis` = 现金计费基数（元）
- 守恒式（P0 已用其判差异）：**`Paid − Refunded = CashPaid + 未回写项`**；目标状态为 `Paid − Refunded = CashPaid`。

## 1. `cash_paid` 语义

- **裁定**：`cash_paid` = **现金净实收**（含押金与运费；不含乐币/赠点；**扣除已退款**）。
- 依据：创建/回调路径均按「金额−乐币」写入；P0 发现 5 单退款未回写，说明目标语义应为净额。
- 状态：**[待用户确认]**（确认「净额」后，P2a 需在退款成功路径同步扣减）。

## 2. `record.Amount` 与乐币

- **裁定**：prepay 时已扣乐币/优惠；回调**不得**再从 `cash_paid` 扣 gift。乐币只体现在 `gift_points_used` 与 `point_batch_consumptions`（FIFO 留痕）。
- 依据：`rent` 路径 `cash_paid = record.Amount` 覆盖写，实际未重复扣；但 `renewal/damage/shortfall/loss` 不覆盖，存在重复扣风险（C2）。
- 状态：**[已由代码事实确定]**（rent 路径）；其余路径 **[待用户确认]** 修复方式（P2b）。

## 3. 续租计价版本

- **裁定**：`orders.pricing_breakdown` 视为**创建时快照，只读**；续租写**追加式** `term_version`（天数/档位/金额/支付记录 ID），结算按版本区间求和。
- 依据：当前续租重写 `pricing_breakdown`，导致历史区间不可复算。
- 状态：**[待用户确认]**（P4）。

## 4. 补缴闭环（`payment_shortfall`）

- **裁定**：到账 → `cash_paid += record.Amount`；关闭 pending 记录；触发结算重算与 `executeRefund`（含超收退款）；幂等键 `(order_id, order_type, out_trade_no)`。
- 依据：`wechatpay_callback.go:357-392` 已有分支但缺累加/退款触发；P0 实证 `YL20260914-003` 差 5 分。
- 状态：**[待用户确认]**（P2a 执行）。

## 5. `total_spending` 单位

- **裁定**：统一写 **Cents**（`models.FromYuan(CashBasis)`）；存量修复须先 dry-run 报告并经用户书面授权。
- 依据：P0 实证 `user fe6fd4fa… total_spending=67` 而 `rent_sum=6,700` 分（¥67）→ 低 100 倍；前端 `/100` 展示。
- 状态：**[已由代码事实确定]**（缺陷）；存量修复 **[待用户确认]**（P2c/P5）。

## 6. 返点（介绍人 / 消费返点）

逐项裁定：

| 项 | 裁定 | 状态 |
|---|---|---|
| 基数 | 以**实收现金** `CashBasis` 为基数（与 `total_spending` 同源），而非应收 `RentPayable` | [待用户确认] |
| 时点 | 结算成功时发放（非支付到账时） | [待用户确认] |
| 幂等 | 同一订单结算重试不得重复发放（唯一键 `order_id + source`） | [待用户确认] |
| 等级取值 | 按下单时会员等级 or 结算时等级 | [待用户确认] |
| 0 现金订单 | 仍发返点？（乐币全额支付） | [待用户确认] |

## 7. 逾期费口径

- **裁定**：以退库检查 `damage_reports.overdue_fee` 为准（一次性），不按日累加。
- 依据：`user_settlement.go:723-727` 读取 `DamageReport.OverdueFee`；P0 场景 5 逾期为 0。
- 状态：**[已由代码事实确定]**。

## 8. 待用户确认清单（P2 启动前置）

1. `cash_paid` 采用**净额**语义（§1）。
2. 非 rent 路径的乐币处理方式（§2）。
3. 续租 `term_version` 方案（§3）。
4. 补缴闭环含「超收退款」（§4）。
5. `total_spending` 存量修复授权（§5）。
6. 返点五项口径（§6）。
7. 是否按 P2a→P2b→P2c→P3→P4→P5 顺序拆 Issue（§9）。

## 9. 后续拆分（P1 确认后建 Issue，Parent: #2268）

- P2a 补缴入账 + 结算重算/退款 + 幂等
- P2b 乐币二次扣减（renewal/damage/shortfall/loss 端到端矩阵）
- P2c `total_spending` 单位 + 存量修复评估
- P3 单一账本（`order_payment_records` + `order_refund_records` 为唯一事实源，聚合可重算）
- P4 快照不可变 / 续租 `term_version` / 结算纯函数 + 幂等
- P5 历史数据回填（用户书面授权，先 dry-run）

每个执行 Issue 的验收：组合矩阵行级断言 `Paid − Refunded = CashPaid`（缺陷版 FAIL → 修复版 PASS）+ ≥3 笔真实订单 replay。
