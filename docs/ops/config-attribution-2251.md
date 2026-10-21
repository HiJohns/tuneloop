# 商户配置逐行归属报告（#2251）

> 生成时间：2026-10-21 00:44 UTC
> 目的：在执行「按商户 org 改口径」前，先对共享 tenant 的配置行做逐行归属判定，避免无数据依据的回填。
> 查询方式：只读 `SELECT`（未修改任何数据）。

## 数据源与环境

| 环境 | DB | 容器 | 说明 |
|---|---|---|---|
| Dev | `tuneloop_debug` | `jobmaster-postgres` | 本地开发库 |
| Prerelease | `tuneloop_pre_snapshot` | `9a91df0605de_tuneloop-postgres`（cadenza） | 预生产快照库 |
| Production | `tuneloop` | `9a91df0605de_tuneloop-postgres`（cadenza） | 生产库 |

## 商户锚映射（判定依据）

### Prerelease

| 商户 | tenant_id | org_id | 状态 |
|---|---|---|---|
| cadenza | `3cfa99da…`（共享） | `bfce6c6d…` | active |
| 卡丹萨 | `3cfa99da…`（共享） | `bd6bfa4b…` | active |
| tuneloop | `57f05555…` | `3cfa99da…` | active |
| 发发发 | `3cfa99da…`（共享） | `04bfefad…` | inactive |

### Production

| 商户 | tenant_id | org_id | 状态 |
|---|---|---|---|
| cadenza | `3cfa99da…`（共享） | `bfce6c6d…` | active |
| 卡丹萨 | `3cfa99da…`（共享） | `bd6bfa4b…` | active |

### Dev

| 商户 | tenant_id | org_id |
|---|---|---|
| cadenza | `6dd387a5…`（共享） | `8620f5c5…` |
| test / Test DualRole×4 | `6dd387a5…`（共享） | 各自独立 org |

## `merchant_pricing_configs` 逐行判定

### Production（4 行）

| id | tenant_id | 归属判定 | 依据 |
|---|---|---|---|
| `e0860faf…` | `3cfa99da…` | **歧义**（不自动归属） | 同时是 cadenza/卡丹萨 的共享 tenant；种子行（updated_by 空） |
| `6d0df758…` | `bfce6c6d…` | **可判定 → cadenza** | 等于 cadenza.org_id；种子行 |
| `e0cff3e7…` | `bd6bfa4b…` | **可判定 → 卡丹萨** | 等于卡丹萨.org_id；有 updated_by（2026-09-08） |
| `b855a8e1…` | `5ebc2cfa…` | **无归属（孤儿）** | 不匹配任何商户 tenant/org；种子行 |

### Prerelease（4 行）

与 Production 相同的 4 行（id 一致）。`3cfa99da…` 行同时是 tuneloop 的 org_id 与三商户共享 tenant → 仍判**歧义**（若按 tuneloop 归属，需人工确认）。

### Dev（2 行）

| id | tenant_id | 归属判定 | 依据 |
|---|---|---|---|
| `90e97c6c…` | `8620f5c5…` | **可判定 → cadenza** | 等于 dev cadenza.org_id |
| `68a74e49…` | `383c018a…` | **无归属（孤儿）** | 不匹配任何商户 tenant/org |

## 其他配置表

| 表 / key | 数据情况 | 判定 |
|---|---|---|
| `system_settings`（repair_inspection_fee / repair_shipping_fee / repair_gift_points_enabled / site_shipping_fee_*） | 0 行 | 无历史归属问题；新数据按商户 org 写入 |
| `system_settings`（home_menu_config） | 0 行 | 平台级（#1645 已废弃残留）；改为平台 tenant（nil UUID）读写 |
| `system_settings`（平台内容类 12 keys） | 均 `00000000-…` | 已是平台级，无需处理 |
| `system_settings`（upload_image_max_size 等配额） | 按 tenant 多行 | 属**租户配额**（读取路径 `api.go:1108`，不属 #2251 文件清单），保持现状 |
| `points_policies` | 0 行 | 无历史归属问题 |
| `merchants.rebate_opt_in` | 行内布尔列 | 按商户主键读写，无共享问题 |

## 结论与决策

1. **无需数据回填**：所有「可判定」的定价配置行**已经**以商户 org 为 `tenant_id`（dev/prerelease/prod 一致）。
2. 歧义行（`3cfa99da…`）与孤儿行（`5ebc2cfa…` / `383c018a…`）**保持原样**：改口径后对任何商户不可见（安全方向）；如后续需要归属，凭本报告由人工决策后再回填。
3. 代码侧改口径：商户配置（定价 / 维修设置 / 网点运费覆盖 / 乐币策略 / 返点）统一以**商户 org** 为键；平台级 `home_menu_config` 以平台 tenant（nil UUID）读写。
4. 无 schema 变更，无迁移文件。
