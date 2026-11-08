# total_spending 存量 dry-run（#2280）

> 环境：预生产快照库 `tuneloop_pre_snapshot`（只读）；执行时间 2026-10-24。
> **本报告不执行任何 UPDATE**；回填交 #2283（需用户书面授权）。

## 1. SQL（只读）

```sql
SELECT u.id, u.email, u.total_spending, u.total_spending*100 AS corrected,
       COALESCE(SUM(s.actual_rent_amount),0) AS rent_sum
FROM users u
LEFT JOIN orders o ON o.user_id = u.id
LEFT JOIN settlements s ON s.order_id = o.id
WHERE u.total_spending > 0
GROUP BY u.id, u.email, u.total_spending
ORDER BY u.total_spending DESC;
```

## 2. 结果（4 用户）

| user id | email | 当前 total_spending | ×100 目标 | rent_sum（分） | 判定 |
|---|---|---:|---:|---:|---|
| `c04d4512…` | — | 22,200 | 2,220,000 | 0 | **异常**：无结算但有消费，需人工核对 |
| `3332ac1f…` | linwx1978@163.com | 220 | 22,000 | 22,000 | ✅ 与 rent_sum 一致（应为 22,000 分） |
| `fe6fd4fa…` | — | 67 | 6,700 | 6,700 | ✅ 与 rent_sum 一致（应为 6,700 分） |
| `17ff916f…` | — | 1 | 100 | 1 | **异常**：×100 与 rent_sum 不一致（缺历史订单/结算），人工核对 |

统计：4 用户；明确 100× 低估 2 户；异常 2 户（不自动回填）。

## 3. 回填脚本草案（#2283 执行，不在此运行）

```sql
-- 幂等：仅当值未变化时更新；逐用户执行并记录 old→new
UPDATE users SET total_spending = total_spending * 100, updated_at = NOW()
WHERE id = $1 AND total_spending = $2;
```

- 仅对「有结算且 `rent_sum = total_spending*100`」的用户执行；
- 异常用户（无结算 / 不一致）先人工核对，不回填；
- 执行后复跑 §1 SQL，校验 `corrected = rent_sum`（或逐单说明）。

## 4. 关联

- 代码修复：#2280（`models.FromYuan(spendingBasis)`，红绿通过）。
- 回填执行：#2283 Batch2（依赖本修复上线 + 用户授权）。
