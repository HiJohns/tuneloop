# P5 dry-run 报告（#2283）

> 环境：预生产快照库 `tuneloop_pre_snapshot`（**只读**）；时间 2026-10-24。
> 工具：`scripts/fix-2268/backfill.sh`（默认 dry-run）。**未执行任何 UPDATE**。

## 1. Batch1：`orders.cash_paid` 差异

命令：
```bash
PSQL_CMD=<pre-prod psql wrapper> bash scripts/fix-2268/backfill.sh --batch 1
```

结果：**56 单差异**，分类：

| 分类 | 单数 | 处置 |
|---|---:|---|
| `refund_not_written`（退款未回写） | 5 | 自动回填（账本净额，旧值条件） |
| `shortfall`（补缴未入账） | 1 | 自动回填 |
| `damage`（定损未入账） | 2 | 人工复算后回填（不自动） |
| `other`（其中 48 单 `ledger_cash=0`） | 48 | **禁止自动**：支付流水已不在快照（历史归档/清理），回填会清零 `cash_paid`；需人工核对原始流水 |

自动执行范围（脚本安全阀）：`ledger_cash > 0` 且（退款未回写 或 补缴未入账）且 `damage_paid=0` → **6 单**（已用只读 count 校验）。

## 2. Batch2：`users.total_spending`

| user id | 当前 | ×100 | rent_sum | 判定 |
|---|---:|---:|---:|---|
| `c04d4512…` | 22,200 | 2,220,000 | 0 | 异常（无结算）→ 不回填 |
| `3332ac1f…` | 220 | 22,000 | 22,000 | 可回填 |
| `fe6fd4fa…` | 67 | 6,700 | 6,700 | 可回填 |
| `17ff916f…` | 1 | 100 | 1 | 异常（与账本不一致）→ 人工 |

## 3. 执行前置（未满足）

- **#2278/#2280 需审计 PASS**（当前 `ready`，未 PASS）；
- 用户**逐批书面授权**；
- 建议低峰窗口；执行前再跑 dry-run 快照（旧值映射）。
- 回滚：脚本使用 `WHERE cash_paid = old` / `total_spending = old`，可按批次反向 UPDATE。

## 4. 只读证明

- Batch1/2 均为 `SELECT`；apply 代码路径未触发；
- 行数/金额统计来自当前快照；未执行 DDL/DML。
