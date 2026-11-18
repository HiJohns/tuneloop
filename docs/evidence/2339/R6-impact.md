# R6-impact — `resolveDepositTier` 押金语义影响面

## 1. 裁定依据（#2406，2026-11-17）

`docs/cases/lease.md:334`（接口文档同步）：

> `instruments.deposit > 0` 为**显式配置**，优先于定价；`instruments.deposit = 0` 表示**未配置**，
> 押金取定价结果（含非零小数额，如 ¥0.01）。**零押金档仅当定价押金 = 0 时可用**，
> 不得凭信用免掉全额押金；确认页（pricing-v2）与下单校验（`resolveDepositTier`）必须同源。

因此本单的 `>0` 判断不是自创口径，而是执行 #2406 的裁定；`DepositTierZero` 仍然保留，
但只在**定价押金 = 0** 时产生（不再由 `instruments.deposit = 0` 触发）。

## 2. `instruments.deposit = 0` 存量行数（只读查询）

```sql
SELECT count(*) AS total,
       count(*) FILTER (WHERE deposit = 0)   AS zero_deposit,
       count(*) FILTER (WHERE deposit IS NULL) AS null_deposit
FROM instruments;
```

| 环境 | DB | total | deposit = 0 | deposit IS NULL |
|---|---|---:|---:|---:|
| Dev | `tuneloop_debug`（127.0.0.1:25432） | 1 | 1 | 0 |
| Prerelease | `tuneloop_pre_snapshot` | 218 | 1 | 0 |
| Production | `tuneloop` | 214 | **214** | 0 |

> 生产 214 台乐器当前全部 `deposit = 0`。按 #2406 口径，这些是「未配置」，
> 新订单押金应取定价结果，而不是继续按零押金下单。

## 3. 改动前后行为

| 场景 | 改动前（`deposit != nil` 即覆盖） | 改动后（`deposit > 0` 才覆盖） |
|---|---|---|
| `instruments.deposit = 500`，定价押金 500 | 500 | 500（不变） |
| `instruments.deposit = 0`，定价押金 500 | **0（DepositTierZero）** | **500（定价押金）** |
| `instruments.deposit = 0`，定价押金 0 | 0（DepositTierZero） | 0（DepositTierZero，保留） |
| `instruments.deposit = NULL`，定价押金 500 | 500（nil 不覆盖） | 500（不变） |

DB 读取后 `instruments.deposit` 因 `default:0` 恒为非 nil，所以「改动前」对
`deposit = 0` 的行一律走零押金档。

## 4. 影响范围

- **已有订单**：`orders.deposit` 在下单时已落库，结算/退款读取订单自身字段，**不受影响**。
- **新订单**（`CreateOrder` / `BatchCreateOrder`）：`deposit = 0` 的乐器将从「零押金」变为
  「定价模板押金」；若定价模板押金也为 0，仍为零押金档。
- **确认页一致性**：pricing-v2 已按 #2406 先落地「0 = 未配置」；本改动使下单校验与确认页同源。
- **发布提示**：生产 214/214 乐器会进入上述新行为，发布说明需注明「未配置押金的乐器新订单将按定价收取押金」。

## 5. 与审计 F2 的对应

审计要求「列出存量行数 + 改动后这些订单押金如何计算；或回滚」。本文件给出三项：
① 三环境存量行数；② 改动前后对照表；③ 新/旧订单影响面。语义依据为 #2406 裁定与
`docs/cases/lease.md:334`，非测试夹具驱动。
