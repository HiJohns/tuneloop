# `tenant_id` P4 清点与逐表分类（#2642 / P4-S1，只读）

> 父任务：#2256（P4）。本单**只读**：不执行任何 DDL/DML，不改列名，不改业务代码。
> 生成器：`scripts/tenant-id-refscan.sh`（无 DB、无网络、确定性）；证据：`docs/evidence/2642/`。
> 权威表清单：`docs/ops/tenant-id-p4-tables.txt`（52 行，单一来源）。

## 1. 口径与规则

### 1.1 双口径（为什么不是单一 52）

| 口径 | 值 | 含义 |
|---|---:|---|
| `INV_SNAPSHOT` | 52 | P2 预生产快照表数（`docs/ops/tenant-id-backfill-p2.md` §3，含 3 张 `bak_2211_*`） |
| `INV_PRE_ONLY` | 3 | 仅存在于预生产（prod 无 `bak_2211_*`） |
| `POST_SNAPSHOT_TABLES` | 1 | 快照之后由可执行迁移新建且带 `tenant_id` 的表（`order_payment_allocations`，`20261116001`） |
| `INV_LIVE` | 53 | 当前 live 全量 = `INV_SNAPSHOT + POST_SNAPSHOT_TABLES - DROPPED_IN_SNAPSHOT` |

TSV 以 **live** 为准，`env ∈ {pre, pre-only, post-snapshot}`；文档矩阵按计划列出 52 行快照表，post-snapshot 表在 §3 单列。

### 1.2 模型三方校验

| 项 | 值 | 判据 |
|---|---:|---|
| `MODEL_STRUCTS` | 52 | `backend/models/*.go` 中声明 `TenantID` 的 struct |
| `MODEL_TABLE_ROWS` | 49 | 解析到的表出现在 P2 快照清单中 |
| `MODEL_NO_TABLE` | 2 | `Label` / `ConfirmationSession`：从未建表（见 §1.4） |
| `MODEL_POST_SNAPSHOT` | 1 | `OrderPaymentAllocation`（`order_payment_allocations`） |

校验恒等式：`MODEL_STRUCTS = MODEL_TABLE_ROWS + MODEL_NO_TABLE + MODEL_POST_SNAPSHOT`（52 = 49 + 2 + 1）。
计划 Rev.2 备注写作 `52 = 49 + 1 + 2`；按仓库实测，无表模型是 **2** 个（`Label`、`ConfirmationSession`）、post-snapshot 是 **1** 个，恒等式成立但分量对调。依据见 §1.4，属事实校准，不改变任何断言口径。

### 1.3 计数规则（机械，禁止表名白名单）

| 列 | 规则 |
|---|---|
| `sql_refs` | 非测试 Go：含 `tenant_id` 的行 ±3 行窗口内出现**带引号的整表名**（`"<table>"`） |
| `field_refs` | 非测试 Go：含 `TenantID` 的行（排除 struct 字段声明行）±5 行窗口内出现该表模型 struct 名 |
| `test_refs` | `*_test.go` 中上述两类规则合计 |
| `frontend_refs` | **全局**：`frontend-pc/src` + `frontend-mobile/src` 中 `tenant_id` 出现次数（34）；不归属任何表，每行重复展示，**不可求和** |
| `doc_refs` | 每表：`docs/**` 中同时提到该表且含 `tenant_id` 的文件数（仅参考，不作门禁） |
| `plumbing_refs` | **全局 token**（155）：`backend/database/db.go`、`backend/middleware/*`、`backend/models/*` 的租户机制引用，不归属任何表 |

### 1.4 为什么不用文件级归属（实测反例）

`backend/database/db.go` 的 `LookUpField("TenantID")` / `SetTenantID` / `addTenantScope` 与 middleware claim 传递属于通用租户机制，被「文件级提及」规则计入**每一张表**。实测：`brand_configs` 文件级 51 处，表级 0 处（本表 `sql_refs=0`、`field_refs=0`、`test_refs=0`）。若按文件级归属，52 张表会全部得到 50+ 引用，分类四桶完全失效。因此把该层单独汇总为 `plumbing_refs`（当前 155）。

同样，模型 struct 自身的 `TenantID` 字段声明行属于模型 plumbing（不是使用点），计数时排除——否则每张表都会因自己的声明得到 1 次 `field_refs`，`metadata` 桶恒空。

`Label`（`labels`）与 `ConfirmationSession`（`confirmation_sessions`）判定为「无表」的依据：`backend/database/db.go:730` 明确记录 “ConfirmationSession, Label — no table ever created”；`labels` 只出现在 `backend/database/migrations/20260404_add_labels_and_metadata.sql`，而该文件**没有 `.up.sql` 后缀**（迁移器只消费 `*.up.sql`），从未被执行。`order_payment_allocations` 则由 `20261116001_settlement_ledger_2360.up.sql` 正规建表，故列为 post-snapshot。

非 `TableName()` 覆写的模型按「候选集 + 表全集」解析：snake_case 基形及其复数候选必须**恰好一个**落在（P2 快照 ∪ 可执行迁移建表）全集内；0 个即判「无表」，多个则 `exit 2` 拒绝运行——不重写 GORM 复数规则，也不维护表名白名单。

### 1.5 分类四桶（固定规则）

| class | 判定 |
|---|---|
| `backup` | `source=backup-only`（表名 `^bak_`） |
| `redundant` | 表同时存在 `org_id` 列（模型 `OrgID` 字段或可执行迁移 DDL 可判） |
| `metadata` | 无 `org_id` 且 `sql_refs + field_refs + test_refs == 0` |
| `anchor-candidate` | 无 `org_id` 且存在引用，需语义裁定 |

## 2. 矩阵（52 张 P2 快照表）

| table | source | model | has_org_id | sql_refs | field_refs | test_refs | frontend_refs | doc_refs | db_default | index_names | class | env |
|---|---|---|---|---:|---:|---:|---:|---:|---|---|---|---|
| `appeals` | model | Appeal | yes | 0 | 4 | 10 | 34 | 11 | none | idx_appeals_tenant | redundant | pre |
| `audit_logs` | model | AuditLog | yes | 0 | 7 | 4 | 34 | 9 | none | idx_audit_logs_tenant_id | redundant | pre |
| `bak_2211_order_payment_records` | backup-only | - | unknown | 0 | 0 | 0 | 34 | 3 | none | - | backup | pre-only |
| `bak_2211_points_transactions` | backup-only | - | unknown | 0 | 0 | 0 | 34 | 3 | none | - | backup | pre-only |
| `bak_2211_users` | backup-only | - | unknown | 0 | 0 | 0 | 34 | 4 | none | - | backup | pre-only |
| `banners` | model | Banner | no | 0 | 1 | 2 | 34 | 5 | none | idx_banners_tenant | anchor-candidate | pre |
| `brand_configs` | model | BrandConfig | no | 0 | 0 | 0 | 34 | 5 | none | idx_brand_configs_tenant | metadata | pre |
| `categories` | model | Category | no | 0 | 4 | 14 | 34 | 11 | zero-uuid | idx_categories_tenant | anchor-candidate | pre |
| `clients` | model | Client | no | 0 | 0 | 0 | 34 | 8 | zero-uuid | idx_clients_tenant | metadata | pre |
| `damage_reports` | model | DamageReport | yes | 0 | 6 | 26 | 34 | 8 | none | idx_damage_reports_tenant | redundant | pre |
| `deposits` | model | Deposit | no | 0 | 4 | 70 | 34 | 5 | none | idx_deposits_tenant | anchor-candidate | pre |
| `electronic_contracts` | model | ElectronicContract | yes | 0 | 2 | 1 | 34 | 4 | none | idx_electronic_contracts_tenant_id | redundant | pre |
| `forwarding_sessions` | model | ForwardingSession | yes | 0 | 2 | 2 | 34 | 4 | none | idx_fs_tenant | redundant | pre |
| `instrument_loss_records` | model | InstrumentLossRecord | no | 0 | 1 | 0 | 34 | 3 | none | idx_instrument_loss_tenant | anchor-candidate | pre |
| `instrument_media` | model | InstrumentMedia | yes | 0 | 12 | 4 | 34 | 10 | none | idx_media_tenant | redundant | pre |
| `instrument_photo_specs` | model | InstrumentPhotoSpec | no | 0 | 0 | 0 | 34 | 3 | none | idx_instrument_photo_specs_tenant_id | metadata | pre |
| `instrument_promo_overrides` | model | InstrumentPromoOverride | no | 0 | 2 | 7 | 34 | 4 | none | - | anchor-candidate | pre |
| `instrument_properties` | model | InstrumentProperty | no | 0 | 1 | 1 | 34 | 6 | none | idx_instrument_properties_property_name | anchor-candidate | pre |
| `instruments` | model | Instrument | yes | 0 | 10 | 230 | 34 | 23 | none | idx_instruments_tenant | redundant | pre |
| `inventory_transfers` | model | InventoryTransfer | yes | 0 | 0 | 0 | 34 | 5 | zero-uuid | idx_inventory_transfers_tenant | redundant | pre |
| `invoice_applications` | model | InvoiceApplication | no | 0 | 2 | 3 | 34 | 5 | none | idx_invoice_app_tenant | anchor-candidate | pre |
| `leases` | model | Lease | no | 0 | 1 | 2 | 34 | 7 | none | idx_leases_tenant | anchor-candidate | pre |
| `maintenance_session_records` | model | MaintenanceSessionRecord | no | 0 | 0 | 1 | 34 | 4 | none | idx_maintenance_session_records_tenant_id | metadata | pre |
| `maintenance_sessions` | model | MaintenanceSession | yes | 0 | 0 | 1 | 34 | 4 | none | idx_maintenance_sessions_tenant_id | redundant | pre |
| `maintenance_tickets` | model | MaintenanceTicket | yes | 0 | 0 | 0 | 34 | 8 | zero-uuid | idx_maintenance_tenant | redundant | pre |
| `maintenance_workers` | model | MaintenanceWorker | yes | 0 | 0 | 1 | 34 | 4 | none | idx_maintenance_workers_tenant_id | redundant | pre |
| `merchant_members` | model | MerchantMember | no | 5 | 4 | 51 | 34 | 17 | none | idx_mm_tenant | anchor-candidate | pre |
| `merchant_pricing_configs` | model | MerchantPricingConfig | no | 0 | 5 | 2 | 34 | 7 | none | - | anchor-candidate | pre |
| `merchant_settlement_configs` | model | MerchantSettlementConfig | no | 1 | 1 | 1 | 34 | 4 | none | idx_settlement_configs_tenant | anchor-candidate | pre |
| `merchants` | model | Merchant | yes | 4 | 27 | 117 | 34 | 23 | none | idx_merchants_tenant | redundant | pre |
| `notifications` | model | Notification | yes | 0 | 20 | 6 | 34 | 7 | none | idx_notifications_tenant | redundant | pre |
| `order_payment_records` | model | OrderPaymentRecord | yes | 0 | 9 | 126 | 34 | 8 | none | idx_payment_records_tenant,idx_payment_tenant | redundant | pre |
| `order_refund_records` | model | OrderRefundRecord | no | 0 | 6 | 21 | 34 | 7 | none | - | anchor-candidate | pre |
| `order_status_history` | model | OrderStatusHistory | yes | 0 | 15 | 5 | 34 | 6 | none | idx_order_status_history_tenant_id | redundant | pre |
| `orders` | model | Order | yes | 7 | 16 | 290 | 34 | 21 | zero-uuid | idx_orders_tenant | redundant | pre |
| `ownership_certificates` | model | OwnershipCertificate | yes | 0 | 0 | 0 | 34 | 6 | zero-uuid | idx_certificates_tenant | redundant | pre |
| `payment_sessions` | model | PaymentSession | no | 0 | 0 | 7 | 34 | 4 | none | idx_payment_sessions_tenant | metadata | pre |
| `pending_orders` | model | PendingOrder | no | 0 | 3 | 5 | 34 | 5 | none | - | anchor-candidate | pre |
| `points_transactions` | model | PointsTransaction | no | 0 | 9 | 0 | 34 | 7 | none | idx_points_transactions_tenant_id | anchor-candidate | pre |
| `properties` | model | Property | no | 0 | 1 | 2 | 34 | 9 | none | idx_properties_tenant | anchor-candidate | pre |
| `property_options` | model | PropertyOption | no | 0 | 5 | 0 | 34 | 4 | none | idx_property_options_property_name | anchor-candidate | pre |
| `repair_requests` | model | RepairRequest | no | 1 | 5 | 49 | 34 | 11 | none | idx_repair_requests_tenant | anchor-candidate | pre |
| `roles` | model | Role | no | 2 | 49 | 439 | 34 | 19 | none | - | anchor-candidate | pre |
| `settlement_calculations` | model | SettlementCalculation | no | 0 | 3 | 0 | 34 | 5 | none | idx_sc_tenant_id | anchor-candidate | pre |
| `site_images` | model | SiteImage | no | 0 | 0 | 0 | 34 | 5 | zero-uuid | idx_site_images_tenant | metadata | pre |
| `site_members` | model | SiteMember | no | 4 | 16 | 125 | 34 | 31 | none | idx_site_members_tenant | anchor-candidate | pre |
| `sites` | model | Site | yes | 1 | 15 | 174 | 34 | 33 | zero-uuid | idx_sites_tenant,idx_sites_tenant_id | redundant | pre |
| `staff_invites` | model | StaffInvite | yes | 0 | 4 | 25 | 34 | 6 | none | idx_staff_invites_tenant,idx_staff_invites_tenant_status | redundant | pre |
| `system_settings` | model | SystemSetting | no | 0 | 12 | 8 | 34 | 8 | none | idx_settings_tenant | anchor-candidate | pre |
| `technician_profiles` | model | TechnicianProfile | no | 0 | 9 | 38 | 34 | 14 | none | idx_technician_profiles_tenant,idx_technician_profiles_user_tenant | anchor-candidate | pre |
| `technicians` | model | Technician | yes | 0 | 1 | 0 | 34 | 8 | zero-uuid | idx_technicians_tenant | redundant | pre |
| `users` | model | User | yes | 2 | 38 | 444 | 34 | 36 | none | idx_users_tenant,idx_users_wx_unionid_tenant_unique | redundant | pre |

计数 token（与 `docs/evidence/2642/R1-inventory.tsv` 逐字一致）：`INV_SNAPSHOT=52`、`INV_PRE_ONLY=3`、`INV_LIVE=53`、`CLASS_BACKUP=3`、`CLASS_REDUNDANT=21`、`CLASS_METADATA=7`、`CLASS_ANCHOR=22`、`CLASS_SUM_CHECK=53`。

## 3. post-snapshot 表（不在 52 行快照方案内，但属 live）

| table | model | has_org_id | sql_refs | field_refs | test_refs | class | env | 说明 |
|---|---|---|---:|---:|---:|---|---|---|
| `order_payment_allocations` | OrderPaymentAllocation | no | 0 | 0 | 1 | metadata | post-snapshot | `20261116001_settlement_ledger_2360.up.sql` 建表（#2360 结算台账），P2 快照早于该迁移 |

## 4. 可立即处置清单（P4-S2 批 1 输入）

规则（计划口径）：`backup` ∪ `metadata` ∪ (`redundant` ∧ `sql_refs + field_refs == 0`)。共 **13** 张。

注意：`class` 与「可立即处置」按计划口径只看 `sql_refs + field_refs`；但 `*_test.go` 的引用同样会阻塞编译，因此**处置批次内必须把 `test_refs` 一并清零**（`--table <T> --require-zero` 已按 `sql+field+test` 判定）。下表 `test_refs > 0` 的行在 DROP 前必须先清理测试夹具。

| table | class | env | sql_refs | field_refs | test_refs | 处置建议 |
|---|---|---|---:|---:|---:|---|
| `bak_2211_order_payment_records` | backup | pre-only | 0 | 0 | 0 | 直接 DROP（S8 备份表，prod 无此表） |
| `bak_2211_points_transactions` | backup | pre-only | 0 | 0 | 0 | 直接 DROP（S8 备份表，prod 无此表） |
| `bak_2211_users` | backup | pre-only | 0 | 0 | 0 | 直接 DROP（S8 备份表，prod 无此表） |
| `brand_configs` | metadata | pre | 0 | 0 | 0 | DROP COLUMN + 删模型字段（字面量引用为零，编译器兜底） |
| `clients` | metadata | pre | 0 | 0 | 0 | DROP COLUMN + 删模型字段（字面量引用为零，编译器兜底） |
| `instrument_photo_specs` | metadata | pre | 0 | 0 | 0 | DROP COLUMN + 删模型字段（字面量引用为零，编译器兜底） |
| `inventory_transfers` | redundant | pre | 0 | 0 | 0 | DROP COLUMN + 删模型字段（有 org_id 承载归属，引用为零） |
| `maintenance_session_records` | metadata | pre | 0 | 0 | 1 | DROP COLUMN + 删模型字段（字面量引用为零，编译器兜底）；先清理 1 处测试引用 |
| `maintenance_tickets` | redundant | pre | 0 | 0 | 0 | DROP COLUMN + 删模型字段（有 org_id 承载归属，引用为零） |
| `order_payment_allocations` | metadata | post-snapshot | 0 | 0 | 1 | DROP COLUMN + 删模型字段（字面量引用为零，编译器兜底）；先清理 1 处测试引用 |
| `ownership_certificates` | redundant | pre | 0 | 0 | 0 | DROP COLUMN + 删模型字段（有 org_id 承载归属，引用为零） |
| `payment_sessions` | metadata | pre | 0 | 0 | 7 | DROP COLUMN + 删模型字段（字面量引用为零，编译器兜底）；先清理 7 处测试引用 |
| `site_images` | metadata | pre | 0 | 0 | 0 | DROP COLUMN + 删模型字段（字面量引用为零，编译器兜底） |

## 5. 需语义裁定清单（anchor-candidate，本单不裁定）

规则：无 `org_id` 且存在引用 → 必须先由 Owner/Architect 裁定语义，再由后续批次处置。共 **22** 张。

| table | env | sql_refs | field_refs | test_refs | 语义候选 | 依据 |
|---|---|---:|---:|---:|---|---|
| `banners` | pre | 0 | 1 | 2 | 创建者元数据（平台级） | P2 §3「平台级/创建者元数据→不回填」 |
| `categories` | pre | 0 | 4 | 14 | 创建者元数据（平台级） | P2 §3「平台级/创建者元数据→不回填」 |
| `deposits` | pre | 0 | 4 | 70 | 无数据（空表） | P2 §3 空表；字段见 models.Deposit |
| `instrument_loss_records` | pre | 0 | 1 | 0 | 无数据（空表） | P2 §3 空表；#1948 丢失台账 |
| `instrument_promo_overrides` | pre | 0 | 2 | 7 | 无数据（空表） | P2 §3 空表 |
| `instrument_properties` | pre | 0 | 1 | 1 | 商户 org 锚 | P2 §3「已一致（全部商户 org）」 |
| `invoice_applications` | pre | 0 | 2 | 3 | 商户 org 锚 | P2 §3「已一致（全部商户 org）」 |
| `leases` | pre | 0 | 1 | 2 | 无数据（空表） | P2 §3 空表 |
| `merchant_members` | pre | 5 | 4 | 51 | 商户 org 锚 | P2 §3 由迁移 20261020002 回填为 merchants.org_id |
| `merchant_pricing_configs` | pre | 0 | 5 | 2 | 无归属（孤儿行，不猜测） | P2 §3「歧义/孤儿行保持不可见（#2251）」 |
| `merchant_settlement_configs` | pre | 1 | 1 | 1 | 无数据（空表） | P2 §3 空表 |
| `order_refund_records` | pre | 0 | 6 | 21 | 商户 org 锚 | P2 §3「已一致（全部商户 org）」 |
| `pending_orders` | pre | 0 | 3 | 5 | 无数据（空表） | P2 §3 空表 |
| `points_transactions` | pre | 0 | 9 | 0 | 顾客零租户（00000000…） | P2 §3「顾客乐币流水（零租户语义正确）」 |
| `properties` | pre | 0 | 1 | 2 | 创建者元数据（平台级） | P2 §3「平台级/创建者元数据→不回填」 |
| `property_options` | pre | 0 | 5 | 0 | 创建者元数据（平台级） | P2 §3「平台级/创建者元数据→不回填」 |
| `repair_requests` | pre | 1 | 5 | 49 | 商户 org 锚 | P2 §3 由迁移 20261020003 回填为商户 org |
| `roles` | pre | 2 | 49 | 439 | 创建者元数据（平台级） | P2 §3「平台级/创建者元数据→不回填」 |
| `settlement_calculations` | pre | 0 | 3 | 0 | 商户 org 锚 | P2 §3「已一致（全部商户 org）」 |
| `site_members` | pre | 4 | 16 | 125 | 商户 org 锚 / 平台租户（中转网点本地租户作用域） | P2 §3「已一致」；中转网点成员行必须=平台租户（#2552/#2554、AGENTS.md 隔离规则 8） |
| `system_settings` | pre | 0 | 12 | 8 | 创建者元数据（平台级） | P2 §3「平台级/创建者元数据→不回填」 |
| `technician_profiles` | pre | 0 | 9 | 38 | 商户 org 锚 | P2 §3 由迁移 20261020001 回填为商户 org |

## 6. DB 侧待实测项（ops 只读，本单不执行）

静态扫描只能看到「代码/DDL 里写了什么」。以下三项必须由 ops 在 dev / pre(`tuneloop_pre_snapshot`) / prod(`tuneloop`) 只读查询补全——三环境表数与索引名可能不同（P2 §1：pre 52 / prod 49 / dev 44）：

- `tenant_id` 列与默认值（`db_default` 列的权威来源）
- 索引名集合（含 GORM 运行时创建、静态迁移中不存在的索引）
- `tenant_id` 参与的唯一约束 / 外键 / 触发器

```sql
-- 1) 每表 tenant_id 列与默认值（三环境分别执行）
SELECT table_name, column_name, data_type, is_nullable, column_default
FROM information_schema.columns
WHERE column_name = 'tenant_id' AND table_schema = 'public'
ORDER BY table_name;

-- 2) 索引：哪些索引包含 tenant_id
SELECT tablename, indexname, indexdef
FROM pg_indexes
WHERE schemaname = 'public' AND indexdef ILIKE '%tenant_id%'
ORDER BY tablename, indexname;

-- 3) 唯一约束 / 外键 / 检查约束中是否涉及 tenant_id
SELECT conrelid::regclass AS table_name, conname, contype, pg_get_constraintdef(oid) AS definition
FROM pg_constraint
WHERE pg_get_constraintdef(oid) ILIKE '%tenant_id%'
ORDER BY 1, 2;
```

回执要求（部署侧，需用户授权）：把三份查询输出贴回本 Issue；与本文件 `index_names` / `db_default` 列不一致时，以 DB 实测为准，并在 P4-S2 计划中更新。

## 7. 口径校验（抽样 5 张表，独立实现交叉验证）

扫描器计数与一份独立 Python 实现（同样窗口规则，不复用脚本代码）逐表比对：

| table | 扫描器 sql/field | 独立实现 sql/field | 一致 |
|---|---|---|---|
| `brand_configs` | 0/0 | 0/0 | ✅ |
| `orders` | 7/16 | 7/16 | ✅ |
| `site_members` | 4/16 | 4/16 | ✅ |
| `merchant_members` | 5/4 | 5/4 | ✅ |
| `bak_2211_users` | 0/0 | 0/0 | ✅ |

说明：`brand_configs` 表级 0 引用而文件级 51 处（§1.4），是本单拒绝文件级归属的实测反例。

## 8. 扫描盲区登记

| 盲区 | 影响 | 兜底 |
|---|---|---|
| 动态拼列名（`fmt.Sprintf("... %s", col)`） | 字面量扫描看不到 | 编译器：删除字段后 `go build ./...` 枚举全部字段级依赖 |
| 反射字段名（`LookUpField("TenantID")`） | 同上 | 同上 + `plumbing_refs` 单列监控 |
| JSON tag（`json:"tenant_id"`） | 契约外溢面（前端/文档） | `frontend_refs` 全局计数 + `--assert-no-live-literal` |
| DB 侧 DEFAULT / 触发器 / 视图 | 列级语义 | §6 ops 只读模板（本单不连库） |
| 其他仓库（beaconiam 等） | 跨仓引用 | 不扫描；跨仓依赖按 `external:` 依赖登记 |

最终放行权威不是本矩阵，而是：字段级 = 编译器（`go build ./...`）+ 字面量级 = `scripts/tenant-id-refscan.sh --assert-no-live-literal`（当前基线 `LIVE_LITERAL_BASE=1031`，负例口径：> 0 即拒绝）。本矩阵只用于**排序与分批**，不作为 #2643 的放行依据。

## 9. 复跑

```bash
bash scripts/tenant-id-refscan.sh --tsv            # 53 行 + token 段
bash scripts/tenant-id-refscan.sh --json           # 同上 JSON
bash scripts/tenant-id-refscan.sh --table brand_configs --require-zero
bash scripts/tenant-id-refscan.sh --assert-no-live-literal
bash scripts/tenant-id-refscan.sh --check-doc docs/ops/tenant-id-p4-inventory.md
bash scripts/tests/test_tenant_id_refscan.sh determinism
bash scripts/tests/test_tenant_id_refscan.sh gate
```
