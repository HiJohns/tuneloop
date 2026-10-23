# 结算对账与根因校正（#2268 P0）

> 环境：预生产快照库 `tuneloop_pre_snapshot`（容器 `9a91df0605de_tuneloop-postgres`）
> 执行时间：2026-10-23 03:1x–03:2x UTC（北京时间 11:1x–11:2x）
> 方式：**只读 SQL（仅 SELECT）**；执行前后关键表行数一致（见 §5）。

## 1. 口径与根因校正（以 develop 代码为准）

| # | Issue 原文判断 | 代码事实（develop） | 正确结论 |
|---|---|---|---|
| C1 | `applySideEffects` 无 `payment_shortfall` 分支，补缴后 settlement 仍 pending | `wechatpay_callback.go:357-392` 已有 `case "payment_shortfall"`：关闭 pending 补缴记录、订单 completed、`settlements.refund_status=completed`；但**未累加 `cash_paid`**、**未调用 `executeRefund`** | 缺陷＝「补缴金额未入账 + 不触发结算重算/退款」，不是「无分支/卡 pending」 |
| C2 | `deductPointsFromRecord` 重复扣乐币 | 首付 `rent` 路径随后用 `cash_paid = record.Amount` **绝对覆盖**（`:233-235`），不会重复扣；风险在**不覆盖**的 `renewal`/`damage`/`payment_shortfall`/`loss` | 须按 `order_type` 做端到端矩阵；直接调 helper 不能证明生产路径 |
| C3 | 返点/消费额只是风险 | `user_settlement.go:261-266/577-582` 将 `CashBasis`（**元**）累加进 `users.total_spending`（**Cents/bigint**）；前端 `/100` 展示 | **确定性单位缺陷**（低 100 倍），列入 P4 |
| C4（本次新增） | 计划中的账本公式只统计 `order_payment_records.type='refund'` | 实际退款流水在 **`order_refund_records`**（`payment_record_id` 关联，`status='refunded'`）；`order_payment_records` 仅 `type='payment'` | 三源对账必须并入退款表，否则所有退款单会被误判「缺流水」 |

## 2. 对账口径

- **账本现金** = `Σ order_payment_records(type='payment', status='paid').amount − Σ order_refund_records(status='refunded').amount`
- **订单聚合** = `orders.cash_paid`（本项目经验证为 **Cents**；不含乐币/点数）
- **结算** = `settlements.actual_rent_amount / gift_points_refunded / cash_refundable / refund_status`
- **差异** = `账本现金 − orders.cash_paid`

## 3. 候选集与总量

| 指标 | 值 |
|---|---:|
| orders | 83 |
| order_payment_records | 176（paid 86 / closed 80 / failed 4 / pending 6） |
| order_refund_records | 29（refunded 23 / pending 3 / failed 3） |
| settlements | 21 |
| 候选单（≥2 笔支付 / renewal / shortfall / damage / 退款 / 乐币） | **19** |
| 有差异单 | **9** |
| 差异绝对值合计 | **877,097 分（¥8,770.97）** |

SQL（候选 + 三源）原文见 §6。

## 4. 逐单差异

| 订单号 | 状态 | cash_paid | 账本现金 | 差异(分) | 分类 | 证据 |
|---|---|---:|---:|---:|---|---|
| YL20261020-019 | completed | 627,000 | 1,000 | **−626,000** | 退款未回写聚合 | refunded 626,000；`cash_paid` 未减 |
| YL20260910-002 | completed | 0 | 210,000 | **+210,000** | 定损款未入账 | `damage/payment/paid` 200,000＋押金 10,000；`damage_reports.damage_amount=200,000` |
| YL20261020-010 | completed | 40,740 | 100 | **−40,640** | 退款未回写聚合 | refunded 40,640 |
| YL20260822-001 | completed | 4 | 299 | **+295** | 租金未落入聚合（疑似覆盖） | `rent/payment/paid` 301；结算 actual_rent=1；需 P2 复现 |
| YL20260908-002 | completed | 72 | 172 | **+100** | 定损款未入账 | `damage/payment/paid` 100；`damage_amount=100` |
| YL20260910-001 | completed | 72 | 38 | **−34** | 退款未回写聚合 | refunded 34 |
| YL20261020-018 | completed | 31 | 11 | **−20** | 退款未回写聚合 | refunded 20 |
| YL20260914-003 | completed | 6 | 11 | **+5** | 补缴款未入账 | `payment_shortfall/payment/paid` 5 |
| YL20260914-004 | completed | 7 | 4 | **−3** | 退款未回写聚合 | refunded 3 |
| YL20261020-017 | completed | 2 | 2 | 0 | 一致 | shortfall 记录金额 0 |
| YL20261020-016 | returning | 2 | 2 | 0 | 一致（补缴 pending） | 4 笔 shortfall 金额 0/closed |
| YL20261020-008 | completed | 0 | 0 | 0 | 一致 | shortfall 金额 0 |
| YL20261020-003 | returning | 0 | 0 | 0 | 一致 | shortfall pending 金额 0 |
| YL20260914-001 | expired | 4 | 4 | 0 | 一致 | shortfall pending 金额 0 |
| YL20260908-001 | completed | 72 | 72 | 0 | 一致 | — |
| YL20260906-003 | completed | 108 | 108 | 0 | 一致 | — |
| YL20260827-001 | completed | 2 | 2 | 0 | 一致 | — |
| YL20260825-002 | completed | 3 | 3 | 0 | 一致 | — |
| YL20260811-006 | completed | 5 | 5 | 0 | 一致（退款 failed） | `refund_status=failed`，无退款流水 |

### 分类汇总

| 分类 | 单数 | 金额（分） | 代表 |
|---|---:|---:|---|
| 退款已执行但未回写 `cash_paid` | 5 | 666,697 | 019 / 010 / 0910-001 / 018 / 0914-004 |
| 定损款未累加 `cash_paid` | 2 | 210,100 | 0910-002 / 0908-002 |
| 补缴款未累加 `cash_paid` | 1 | 5 | 0914-003 |
| 租金未落入聚合（疑似覆盖） | 1 | 295 | 0822-001 |
| 一致 | 10 | 0 | — |

> `gift_points_used / prepaid_points_used > 0` 的订单在快照中为 **0** 单 → C2 的乐币路径未被真实数据覆盖，必须靠 P2b 端到端矩阵。

## 5. 只读证明

执行前后（同一会话内）行数快照一致：

| 表 | 执行前 | 执行后 |
|---|---:|---:|
| orders | 83 | 83 |
| order_payment_records | 176 | 176 |
| order_refund_records | 29 | 29 |
| settlements | 21 | 21 |
| schema_migrations | 1 | 1 |

全部语句为 `SELECT`；无 DDL/DML。SQL 原文见 §6。

## 6. SQL 原文（可复制）

```sql
-- 候选 + 三源（仅 SELECT）
WITH pay AS (
  SELECT order_id,
    COUNT(*) FILTER (WHERE type='payment' AND status='paid') AS pay_cnt,
    COALESCE(SUM(CASE WHEN type='payment' AND status='paid' THEN amount ELSE 0 END),0) AS paid_sum,
    COALESCE(SUM(CASE WHEN type='payment' AND status='paid' AND order_type='renewal' THEN amount ELSE 0 END),0) AS renewal_paid,
    COALESCE(SUM(CASE WHEN type='payment' AND status='paid' AND order_type='payment_shortfall' THEN amount ELSE 0 END),0) AS shortfall_paid,
    COALESCE(SUM(CASE WHEN type='payment' AND status='paid' AND order_type='damage' THEN amount ELSE 0 END),0) AS damage_paid,
    COUNT(*) FILTER (WHERE order_type='payment_shortfall') AS shortfall_cnt
  FROM order_payment_records GROUP BY order_id
), ref AS (
  SELECT pr.order_id, COALESCE(SUM(CASE WHEN rr.status='refunded' THEN rr.amount ELSE 0 END),0) AS refunded_sum,
         COUNT(rr.id) AS refund_cnt
  FROM order_refund_records rr JOIN order_payment_records pr ON pr.id=rr.payment_record_id
  GROUP BY pr.order_id
)
SELECT o.order_no, o.status, o.cash_paid,
       COALESCE(p.pay_cnt,0), COALESCE(p.paid_sum,0), COALESCE(f.refunded_sum,0), COALESCE(f.refund_cnt,0),
       COALESCE(p.paid_sum,0)-COALESCE(f.refunded_sum,0) AS ledger_cash,
       COALESCE(p.paid_sum,0)-COALESCE(f.refunded_sum,0)-o.cash_paid AS diff,
       COALESCE(p.renewal_paid,0), COALESCE(p.shortfall_paid,0), COALESCE(p.damage_paid,0),
       s.cash_refundable, s.refund_status
FROM orders o
LEFT JOIN pay p ON p.order_id=o.id
LEFT JOIN ref f ON f.order_id=o.id
LEFT JOIN LATERAL (SELECT * FROM settlements s0 WHERE s0.order_id=o.id ORDER BY s0.created_at DESC LIMIT 1) s ON true
WHERE COALESCE(p.pay_cnt,0)>=2 OR COALESCE(p.renewal_paid,0)>0 OR COALESCE(p.shortfall_cnt,0)>0
   OR COALESCE(p.damage_paid,0)>0 OR COALESCE(f.refund_cnt,0)>0
   OR o.gift_points_used>0 OR o.prepaid_points_used>0
ORDER BY ABS(COALESCE(p.paid_sum,0)-COALESCE(f.refunded_sum,0)-o.cash_paid) DESC;
```

```sql
-- total_spending 量级证据（C3）
SELECT u.id, u.total_spending,
       COALESCE(SUM(s.actual_rent_amount),0) AS rent_sum
FROM users u
LEFT JOIN orders o ON o.user_id=u.id
LEFT JOIN settlements s ON s.order_id=o.id
WHERE u.total_spending>0 GROUP BY u.id,u.total_spending ORDER BY u.total_spending DESC;
-- 结果示例：user fe6fd4fa… total_spending=67；rent_sum=6700（分）
-- ⇒ total_spending 写入的是「元」数值，同列语义为 Cents → 低 100 倍（67 vs ¥67.00）
```

## 7. 无法归因 / 说明

- `YL20260822-001`（+295）：租金支付 301 分、结算 `actual_rent_amount=1`、失败续租 5,000 不计数；`cash_paid=4` 与任一支付记录都不相等，疑似被结算/回调覆盖，需 P2 用订单 replay 复现。
- `YL20261020-019` 结算 `actual_rent_amount=900`，与 `cash_paid=627,000` 差异巨大（大概率测试单），不改变「退款未回写」结论。
- 退款 `failed` 单（如 `YL20260811-006`）账本与聚合一致，但存在「结算 refund_status=failed 且无 refunded 流水」的运营待办。
