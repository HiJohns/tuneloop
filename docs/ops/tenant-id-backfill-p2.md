# tenant_id 逐表归属盘点与回填决策（P2 / #2254）

> 生成时间：2026-10-21 13:32 UTC
> 方式：只读聚合查询（information_schema + 每表 count FILTER），**未执行任何 DDL/DML**。
> 父任务：#2245（P2）；基线：`docs/ops/tenant-id-audit.md` §A2/§3。

## 1. 数据源与环境

| 环境 | DB | 容器 | 备注 |
|---|---|---|---|
| Prerelease | `tuneloop_pre_snapshot` | `9a91df0605de_tuneloop-postgres`（cadenza） | 52 表全部存在；schema_migrations max=`20261012001` |
| Production | `tuneloop` | `9a91df0605de_tuneloop-postgres`（cadenza） | 49 表（无 `bak_2211_*`）；max=`20261012001` |
| Dev | `tuneloop_debug` | `jobmaster-postgres` | 仅 44/52 表含 tenant_id（本地库未完整迁移）→ 以 pre/prod 为准 |

## 2. 分类口径（含"值碰撞"优先级）

1. `null`：tenant_id 为 NULL
2. `zero`：`00000000-…`（平台/顾客占位，按业务语义判断）
3. `org`：等于某商户 `merchants.org_id` **且该值不是任何共享 tenant**
4. `amb`（**歧义/值碰撞**）：该值既是某商户 org，又是多商户共享 tenant（预生产 `3cfa99da` = tuneloop org + 3 商户共享 tenant）
5. `shared`：是共享 tenant 但不是任何 org
6. `uniq`：唯一归属某商户的 `merchants.tenant_id`（可由该商户 org 推导）
7. `other`：以上皆非
8. `dm/ds/di`：偏差行中可经 `merchant_id` / `site_id` / `instrument_id` 推导到商户 org 的行数

> ⚠️ 关键教训：仅用 `tenant_id IN (SELECT org_id FROM merchants)` 判定会把 `3cfa99da` 误判为已锚定（它是 tuneloop 的 org，同时又是共享 tenant）。本报告已按上表优先级重算（旧基线 §3 的粗判不可直接用于回填）。

## 3. Prerelease 逐表结果（52 表）

| 表 | 行数 | zero | org | amb | shared | uniq | other | 可推导(dm) | 决策 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
| `appeals` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `audit_logs` | 1529 | 30 | 1015 | 348 | 0 | 0 | 136 | 0 | 审计历史记录（操作者当时的 tenant 事实）→ 不回填（回填会篡改审计事实） |
| `bak_2211_order_payment_records` | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | S8 备份表 → P4 删除（不回填） |
| `bak_2211_points_transactions` | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | S8 备份表 → P4 删除（不回填） |
| `bak_2211_users` | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | S8 备份表 → P4 删除（不回填） |
| `banners` | 4 | 0 | 0 | 4 | 0 | 0 | 0 | 0 | 平台级/创建者元数据（tenant 非访问范围）→ 不回填 |
| `brand_configs` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 平台级/创建者元数据（tenant 非访问范围）→ 不回填 |
| `categories` | 32 | 0 | 0 | 32 | 0 | 0 | 0 | 0 | 平台级/创建者元数据（tenant 非访问范围）→ 不回填 |
| `clients` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 平台级/创建者元数据（tenant 非访问范围）→ 不回填 |
| `damage_reports` | 25 | 0 | 25 | 0 | 0 | 0 | 0 | 0 | 已一致（全部商户 org） |
| `deposits` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `electronic_contracts` | 103 | 0 | 103 | 0 | 0 | 0 | 0 | 0 | 已一致（全部商户 org） |
| `forwarding_sessions` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `instrument_loss_records` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `instrument_media` | 2350 | 0 | 2350 | 0 | 0 | 0 | 0 | 0 | 已一致（全部商户 org） |
| `instrument_photo_specs` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 平台级/创建者元数据（tenant 非访问范围）→ 不回填 |
| `instrument_promo_overrides` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `instrument_properties` | 244 | 0 | 244 | 0 | 0 | 0 | 0 | 0 | 已一致（全部商户 org） |
| `instruments` | 218 | 0 | 218 | 0 | 0 | 0 | 0 | 0 | 已一致（全部商户 org） |
| `inventory_transfers` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `invoice_applications` | 2 | 0 | 2 | 0 | 0 | 0 | 0 | 0 | 已一致（全部商户 org） |
| `leases` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `maintenance_session_records` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `maintenance_sessions` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `maintenance_tickets` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `maintenance_workers` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `merchant_members` | 8 | 0 | 0 | 8 | 0 | 0 | 0 | 8 | 由迁移 20261020002 覆盖（待部署） |
| `merchant_pricing_configs` | 4 | 0 | 2 | 1 | 0 | 0 | 1 | 0 | 歧义/孤儿行保持不可见（#2251 决策）→ 不回填 |
| `merchant_settlement_configs` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `merchants` | 4 | 0 | 0 | 3 | 0 | 1 | 0 | 0 | 锚源表（org_id 为归属锚）；tenant 为商户自身/共享 tenant → 不回填 |
| `notifications` | 107 | 25 | 79 | 0 | 0 | 3 | 0 | 0 | 通知元数据；零租户=顾客/平台 → 不回填 |
| `order_payment_records` | 176 | 32 | 144 | 0 | 0 | 0 | 0 | 0 | 零租户=顾客预支付流水（order_id 全 NULL）→ 不回填；org 行已一致 |
| `order_refund_records` | 29 | 0 | 29 | 0 | 0 | 0 | 0 | 0 | 已一致（全部商户 org） |
| `order_status_history` | 162 | 0 | 162 | 0 | 0 | 0 | 0 | 0 | 已一致（全部商户 org） |
| `orders` | 83 | 0 | 83 | 0 | 0 | 0 | 0 | 0 | 已一致（全部商户 org） |
| `ownership_certificates` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `payment_sessions` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `pending_orders` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `points_transactions` | 15 | 15 | 0 | 0 | 0 | 0 | 0 | 0 | 顾客乐币流水（零租户语义正确）→ 不回填 |
| `properties` | 7 | 3 | 0 | 4 | 0 | 0 | 0 | 0 | 平台级/创建者元数据（tenant 非访问范围）→ 不回填 |
| `property_options` | 103 | 35 | 0 | 68 | 0 | 0 | 0 | 0 | 平台级/创建者元数据（tenant 非访问范围）→ 不回填 |
| `repair_requests` | 8 | 0 | 0 | 5 | 0 | 3 | 0 | 0 | 由迁移 20261020003 覆盖（待部署） |
| `roles` | 36 | 0 | 12 | 0 | 0 | 0 | 24 | 0 | 平台级/创建者元数据（tenant 非访问范围）→ 不回填 |
| `settlement_calculations` | 18 | 0 | 18 | 0 | 0 | 0 | 0 | 0 | 已一致（全部商户 org） |
| `site_images` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `site_members` | 7 | 0 | 7 | 0 | 0 | 0 | 0 | 0 | 已一致（全部商户 org） |
| `sites` | 3 | 0 | 2 | 1 | 0 | 0 | 0 | 0 | 已一致（预生产 1 行「北京中转站」tenant=org=3cfa99da，值碰撞非偏差） |
| `staff_invites` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `system_settings` | 21 | 12 | 2 | 1 | 0 | 0 | 6 | 0 | 平台级/创建者元数据（tenant 非访问范围）→ 不回填 |
| `technician_profiles` | 4 | 0 | 1 | 3 | 0 | 0 | 0 | 0 | 由迁移 20261020001 覆盖（待部署） |
| `technicians` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `users` | 53 | 24 | 13 | 13 | 0 | 0 | 3 | 0 | 本地缓存（S1 不可靠）；membership 为权威 → 不回填（P3 停止依赖） |

## 4. Production 逐表结果（49 表）

| 表 | 行数 | zero | org | amb | shared | uniq | other | 可推导(dm) | 决策 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
| `appeals` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `audit_logs` | 1281 | 3 | 846 | 0 | 296 | 0 | 136 | 0 | 审计历史记录（操作者当时的 tenant 事实）→ 不回填（回填会篡改审计事实） |
| `banners` | 3 | 0 | 0 | 0 | 3 | 0 | 0 | 0 | 平台级/创建者元数据（tenant 非访问范围）→ 不回填 |
| `brand_configs` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 平台级/创建者元数据（tenant 非访问范围）→ 不回填 |
| `categories` | 36 | 0 | 0 | 0 | 36 | 0 | 0 | 0 | 平台级/创建者元数据（tenant 非访问范围）→ 不回填 |
| `clients` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 平台级/创建者元数据（tenant 非访问范围）→ 不回填 |
| `damage_reports` | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 已一致（全部商户 org） |
| `deposits` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `electronic_contracts` | 36 | 0 | 36 | 0 | 0 | 0 | 0 | 0 | 已一致（全部商户 org） |
| `forwarding_sessions` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `instrument_loss_records` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `instrument_media` | 2306 | 0 | 2306 | 0 | 0 | 0 | 0 | 0 | 已一致（全部商户 org） |
| `instrument_photo_specs` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 平台级/创建者元数据（tenant 非访问范围）→ 不回填 |
| `instrument_promo_overrides` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `instrument_properties` | 243 | 0 | 243 | 0 | 0 | 0 | 0 | 0 | 已一致（全部商户 org） |
| `instruments` | 209 | 0 | 209 | 0 | 0 | 0 | 0 | 0 | 已一致（全部商户 org） |
| `inventory_transfers` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `invoice_applications` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `leases` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `maintenance_session_records` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `maintenance_sessions` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `maintenance_tickets` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `maintenance_workers` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `merchant_members` | 7 | 0 | 0 | 0 | 7 | 0 | 0 | 7 | 由迁移 20261020002 覆盖（待部署） |
| `merchant_pricing_configs` | 4 | 0 | 2 | 0 | 1 | 0 | 1 | 0 | 歧义/孤儿行保持不可见（#2251 决策）→ 不回填 |
| `merchant_settlement_configs` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `merchants` | 2 | 0 | 0 | 0 | 2 | 0 | 0 | 0 | 锚源表（org_id 为归属锚）；tenant 为商户自身/共享 tenant → 不回填 |
| `notifications` | 6 | 2 | 4 | 0 | 0 | 0 | 0 | 0 | 通知元数据；零租户=顾客/平台 → 不回填 |
| `order_payment_records` | 31 | 3 | 28 | 0 | 0 | 0 | 0 | 0 | 零租户=顾客预支付流水（order_id 全 NULL）→ 不回填；org 行已一致 |
| `order_refund_records` | 12 | 0 | 12 | 0 | 0 | 0 | 0 | 0 | 已一致（全部商户 org） |
| `order_status_history` | 39 | 0 | 39 | 0 | 0 | 0 | 0 | 0 | 已一致（全部商户 org） |
| `orders` | 25 | 0 | 25 | 0 | 0 | 0 | 0 | 0 | 已一致（全部商户 org） |
| `ownership_certificates` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `payment_sessions` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `pending_orders` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `points_transactions` | 3 | 3 | 0 | 0 | 0 | 0 | 0 | 0 | 顾客乐币流水（零租户语义正确）→ 不回填 |
| `properties` | 7 | 3 | 0 | 0 | 4 | 0 | 0 | 0 | 平台级/创建者元数据（tenant 非访问范围）→ 不回填 |
| `property_options` | 103 | 35 | 0 | 0 | 68 | 0 | 0 | 0 | 平台级/创建者元数据（tenant 非访问范围）→ 不回填 |
| `repair_requests` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 由迁移 20261020003 覆盖（待部署） |
| `roles` | 32 | 0 | 8 | 0 | 0 | 0 | 24 | 0 | 平台级/创建者元数据（tenant 非访问范围）→ 不回填 |
| `settlement_calculations` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `site_images` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `site_members` | 6 | 0 | 6 | 0 | 0 | 0 | 0 | 0 | 已一致（全部商户 org） |
| `sites` | 2 | 0 | 2 | 0 | 0 | 0 | 0 | 0 | 已一致（预生产 1 行「北京中转站」tenant=org=3cfa99da，值碰撞非偏差） |
| `staff_invites` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `system_settings` | 26 | 17 | 2 | 0 | 1 | 0 | 6 | 0 | 平台级/创建者元数据（tenant 非访问范围）→ 不回填 |
| `technician_profiles` | 2 | 0 | 2 | 0 | 0 | 0 | 0 | 0 | 由迁移 20261020001 覆盖（待部署） |
| `technicians` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 空表 |
| `users` | 35 | 12 | 8 | 0 | 12 | 0 | 3 | 0 | 本地缓存（S1 不可靠）；membership 为权威 → 不回填（P3 停止依赖） |

## 5. 偏差明细与既有迁移覆盖（真值核对，2026-10-21）

| 表 | 环境 | 偏差行 | 推导方式 | 覆盖迁移 | 核对结果 |
|---|---|---:|---|---|---|
| `merchant_members` | prod 7（shared=7） | 7 | `merchant_id → merchants.org_id` | `20261020002` | **7/7 可推导**（pre 8 行同样可推导，待 002 应用） |
| `technician_profiles` | pre 3（amb=3） | 3 | `user_id → users.tenant_id → merchants.org_id` | `20261020001` | **3/3 可推导**（prod 已为 org，无需处理） |
| `repair_requests`（type=service） | pre 8（amb=5, uniq=3） | 8 | `technician_id → users.tenant_id → merchants.org_id` | `20261020003` | **8/8 可推导**（prod 0 行） |
| `sites` | pre 1（amb=1） | 0 | `org_id=3cfa99da` 属 tuneloop 商户 | — | 值碰撞；无偏差（保持） |
| `users` | pre 13 amb / prod 12 shared | — | membership 仅 pre 1 行可推导（小林→卡丹萨 org） | — | 缓存不可靠（S1）→ **不回填**，P3 停止依赖 |
| `merchant_pricing_configs` | pre 1 amb + 1 other；prod 1 shared + 1 other | — | 无（种子/孤儿行） | — | 保持不可见（#2251） |

## 6. 结论

1. **无需新增回填迁移**：所有"可推导偏差"均已被既有三个迁移覆盖（`20261020001` / `20261020002` / `20261020003`）；其余偏差属平台级元数据 / 审计历史 / 缓存语义 / 顾客零租户 / 无归属行，按规则**不猜测回填**。
2. **不新增工件断言**：`defaultMigrationArtifacts` 仅支持表/列存在性校验（`verifyArtifacts`），三个迁移均为**纯数据 UPDATE**、无 schema 工件；为其登记伪表名会误报。执行正确性改由本报告 §5 的推导计数 + 部署后复查 SQL 保证（见 §7）。
3. 已知待办：三个迁移在 pre/prod 尚未应用（max=`20261012001`），随下次发布由 bootstrap 执行；执行属**部署侧动作，需用户授权**。

## 7. 部署侧（需授权，不在本提交执行）

```sql
-- 部署后复核（只读）
-- 1) prod merchant_members 应无 shared 行：
SELECT count(*) FROM merchant_members mm WHERE NOT EXISTS (SELECT 1 FROM merchants m WHERE m.org_id=mm.tenant_id);
-- 2) pre technician_profiles 应全部 org：
SELECT count(*) FROM technician_profiles tp WHERE NOT EXISTS (SELECT 1 FROM merchants m WHERE m.org_id=tp.tenant_id);
-- 3) pre repair_requests(service) 应全部 org：
SELECT count(*) FROM repair_requests rr WHERE rr.type='service' AND NOT EXISTS (SELECT 1 FROM merchants m WHERE m.org_id=rr.tenant_id);
```

- 执行人：发布负责人；环境：cadenza 预生产 → 生产；动作：随 `make release` 部署后核查启动日志 `Successfully applied migrations` + 上述三条 SQL 期望值 = 0。

## 8. 复跑方法

```bash
# 生成 + 采集（只读）
python3 tmp/audit2.py tmp/pre-tables.txt tmp/pre-cols.txt > tmp/pre-audit2.sql
ssh cadenza "docker exec -i 9a91df0605de_tuneloop-postgres psql -U tuneloop_user -d tuneloop_pre_snapshot -At -F'|'" < tmp/pre-audit2.sql
```

## 9. 回填行级 preview（只读，验证"回填后值"正确）

> 从每个回填目标表抽 3 行，展示「当前 tenant → 推导 org」，证明回填目标值唯一且业务归属正确（未执行写操作）。

### `merchant_members`（pre；迁移 20261020002）

| id | user_id | 当前 tenant | 推导 org | 商户 |
|---|---|---|---|---|
| `b1609555…` | `04bbe692…` | `3cfa99da…` | `bfce6c6d…` | cadenza |
| `4eb0584b…` | `7f8085be…` | `3cfa99da…` | `bd6bfa4b…` | 卡丹萨 |
| `e168782d…` | `df016bf8…` | `3cfa99da…` | `bfce6c6d…` | cadenza |

### `technician_profiles`（pre；迁移 20261020001）

| id | 当前 tenant | 推导 org | 商户 |
|---|---|---|---|
| `60b3ede9…` | `3cfa99da…` | `bd6bfa4b…` | 卡丹萨 |
| `baeab991…` | `3cfa99da…` | `bd6bfa4b…` | 卡丹萨 |
| `3d771e64…`（南哥） | `bd6bfa4b…`（已一致） | `bd6bfa4b…` | 卡丹萨 |

### `repair_requests`（pre，type=service；迁移 20261020003）

| id | 当前 tenant | 推导 org | 商户 | 状态 |
|---|---|---|---|---|
| `0f9bcfe2…` | `57f05555…` | `bd6bfa4b…` | 卡丹萨 | pending_quote |
| `fbaa38e7…` | `57f05555…` | `bd6bfa4b…` | 卡丹萨 | pending_quote |
| `73b237ad…` | `3cfa99da…` | `bd6bfa4b…` | 卡丹萨 | closed |

**关系断言**：三表推导链均经 `merchant_id` / `user_id→users.tenant_id` / `technician_id→users.tenant_id`，与 `merchants.org_id` 一一对应（每行 derived_org 非空且唯一）；`repair_requests` 的 order/instrument 链未受影响（本迁移只改 tenant_id 列）。

### SQL 类型安全（EXPLAIN，只读）

```
Update on merchant_members mm  (cost=1.07..2.18 rows=3)
   Join Filter: (mm.tenant_id IS DISTINCT FROM m.org_id)
Update on technician_profiles tp  (cost=5.28..17.07 rows=0)
Update on repair_requests rr  (cost=8.30..12.28 rows=0)   # #2248 已修正 uuid 类型比较
```
