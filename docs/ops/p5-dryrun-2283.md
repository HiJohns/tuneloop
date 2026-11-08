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

## 3. 执行前置（已满足）

- **#2278 / #2280 已 PASS/done**；
- 用户**逐批书面授权**（脚本强制 `--i-have-written-authorization`，缺失 exit 2）；
- 建议低峰窗口；执行前再跑 dry-run 快照（旧值映射）。
- 回滚：脚本使用 `WHERE cash_paid = old` / `total_spending = old`，可按批次反向 UPDATE。

## 3.1 脚本收尾（2026-10-26）

- `--apply` 必须同时带 `--i-have-written-authorization` + `--batch 1|2`；缺授权 → **exit 2**，仅执行 COUNT：
  - 预生产快照计数：`batch1=6`、`batch2=2`。
- Batch1 apply：沿用安全阀（6 单）；执行前写 `tmp/fix-2268-batch1-<ts>.tsv`（id/old/new）。
- Batch2 apply：规则 `total_spending > 0 AND rent_sum = total_spending * 100`；
  - 预生产 dry-run：可回填 2 户（`3332ac1f…` 220→22000、`fe6fd4fa…` 67→6700）；异常 2 户（`c04d4512…`、`17ff916f…`）不命中、保持不变。
  - 单事务；执行前写 `tmp/fix-2268-batch2-<ts>.tsv`（TSV：id/old/new）。
- dry-run 无 UPDATE：`bash -x ... --batch 1 2>trace`，`grep '^+ .*UPDATE' trace` = **0**。
- 本地夹具验证（临时库，已销毁）：
  - Batch1 `8000→6000`，`UPDATE 1`；再次执行 `UPDATE 0`（自然幂等）。
  - Batch2 `220→22000`，`UPDATE 1`；异常用户不变；再次执行 `UPDATE 0`。
  - 按映射反向 UPDATE 后恢复 `8000 / 220`。

### Batch2 回滚示例
```sql
-- map(TSV): id	old	new
UPDATE users SET total_spending = 220 WHERE id = '3332ac1f-…' AND total_spending = 22000;
UPDATE users SET total_spending = 67  WHERE id = 'fe6fd4fa-…' AND total_spending = 6700;
```

### 执行命令（ops，需授权）
```bash
# 1) 重跑 dry-run 并保存快照
PSQL_CMD=<target-psql> bash scripts/fix-2268/backfill.sh --batch 1
PSQL_CMD=<target-psql> bash scripts/fix-2268/backfill.sh --batch 2

# 2) 逐批授权执行
PSQL_CMD=<target-psql> bash scripts/fix-2268/backfill.sh --apply --batch 1 --i-have-written-authorization
PSQL_CMD=<target-psql> bash scripts/fix-2268/backfill.sh --apply --batch 2 --i-have-written-authorization
```

## 4. 只读证明

- Batch1/2 均为 `SELECT`；apply 代码路径未触发；
- 行数/金额统计来自当前快照；未执行 DDL/DML。
