# 维修服务单逐行归属报告（#2248 M2）

> 生成时间：2026-10-21（UTC）
> 目的：在切换「商户 org 锚定」写读口径前，对 `repair_requests(type='service')` 存量行做逐行归属推导，区分可判定 / 歧义 / 无归属。
> 查询方式：只读 `SELECT` / `EXPLAIN`（未修改任何数据）。

## 数据源与环境

| 环境 | DB | 容器 | 已应用迁移 |
|---|---|---|---|
| Prerelease | `tuneloop_pre_snapshot` | `9a91df0605de_tuneloop-postgres`（cadenza） | max=`20261012001`（#2248 迁移尚未应用） |
| Production | `tuneloop` | `9a91df0605de_tuneloop-postgres`（cadenza） | max=`20261012001`（同上） |
| Dev | `tuneloop_debug` | `jobmaster-postgres` | 缺 `repair_requests` 表（本地库未迁移，N/A） |

## 计数结论

| 环境 | 总行数 | 可推导（technician） | 可推导（site） | 歧义 | 无归属 |
|---|---:|---:|---:|---:|---:|
| Production | 0 | 0 | 0 | 0 | 0 |
| Prerelease | 8 | 8 | 0（site_id 全为 NULL） | 0 | 0 |
| Dev | N/A | N/A | N/A | N/A | N/A |

## 逐行明细（Prerelease，8 行）

推导规则：`technician_id = users.id` → `users.tenant_id` → `merchants.org_id`（M2 步骤 1）。

| repair_request id | 当前 tenant_id | technician_id | 推导商户 org | 判定 |
|---|---|---|---|---|
| `3aca4e54…` | `3cfa99da…` | `f658d512…` | `bd6bfa4b…`（卡丹萨） | 可判定 |
| `086cfdd5…` | `3cfa99da…` | `f658d512…` | `bd6bfa4b…`（卡丹萨） | 可判定 |
| `73b237ad…` | `3cfa99da…` | `f658d512…` | `bd6bfa4b…`（卡丹萨） | 可判定 |
| `f0f902c9…` | `3cfa99da…` | `f658d512…` | `bd6bfa4b…`（卡丹萨） | 可判定 |
| `113dca89…` | `3cfa99da…` | `f658d512…` | `bd6bfa4b…`（卡丹萨） | 可判定 |
| `fbaa38e7…` | `57f05555…` | `7f8085be…` | `bd6bfa4b…`（卡丹萨） | 可判定 |
| `3ef6741a…` | `57f05555…` | `f658d512…` | `bd6bfa4b…`（卡丹萨） | 可判定 |
| `0f9bcfe2…` | `57f05555…` | `7f8085be…` | `bd6bfa4b…`（卡丹萨） | 可判定 |

> 说明：8 行维修师均属卡丹萨（`users.tenant_id = bd6bfa4b`），故统一归属卡丹萨 org；当前 tenant 的两种历史值（`3cfa99da` = 旧「根租户」规范化的共享值 / `57f05555` = tuneloop 商户 tenant）均不再作为归属依据。

## 迁移 SQL 缺陷与修正（阻断级）

审计返工时发现 `20261020003` 迁移 up SQL 存在**类型错误**（在任何库上都会 FATAL，阻断启动）：

1. `rr.technician_id <> ''` —— `technician_id` 为 `uuid`，与 `''` 比较 → `invalid input syntax for type uuid: ""`；
2. `rr.technician_id = u.id::text` —— `uuid = text` 无比较算子；
3. `rr.site_id ~ '^[0-9a-fA-F-]{36}$'` —— `uuid ~ text` 无正则算子。

已修正为 uuid 原生等值比较，并将 `<>` 改为 `IS DISTINCT FROM`（同时覆盖 tenant_id 为 NULL 的待归属行）。修正后在本报告环境执行 `EXPLAIN`（只读，不执行）验证两条 UPDATE 均可正常生成计划：

```
Update on repair_requests rr  (cost=8.30..12.28 rows=0 width=0)
  ... Hash Cond: (u.id = rr.technician_id) ...
Update on repair_requests rr  (cost=0.00..…) ...
  ... Join Filter: (rr.site_id = s.id) ...
```

同批迁移（`20261020001` technician_profiles、`20261020002` merchant_members）也以 `EXPLAIN` 复核通过（uuid = uuid）。

## 决策

- 8 行全部可判定 → 迁移会自动回填为卡丹萨 org（`20261020003`，幂等）。
- 无歧义/无归属行；迁移后不存在「对任何商户不可见」的 service 行。
- 部署动作（在预生产/生产执行迁移）仍需用户授权，属部署侧待办。
