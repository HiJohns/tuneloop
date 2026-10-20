# tenant_id 全量审计清单（可疑代码基线）

> 原则：**所有使用 `tenant_id` 的代码默认可疑**；归属权威 = membership（`merchant_members` / `site_members` / `technician_profiles` 等），`users.tenant_id` 与 `merchants.tenant_id` 均不作商户身份。
> 生成方式：全仓 grep + 预生产 `information_schema`（2026-10-20）。

## 1. 量化

| 层 | 引用数 |
|---|---:|
| 后端 Go（handlers/services/middleware/database/models） | 2232 |
| 前端 JS/JSX | 26 |
| 文档 | 216 |
| 含 `tenant_id` 的表 | 52（预生产） |

文件级明细：`docs/ops/tenant-id-audit-files.csv`（layer,file,refs）。

## 2. 后端热点文件（Top 20）

| 文件 | 引用数 |
|---|---:|
| `backend/handlers/repair_service.go` | 51 |
| `backend/handlers/order_test.go` | 44 |
| `backend/handlers/instrument_create_test.go` | 33 |
| `backend/handlers/instrument_batch_import_test.go` | 28 |
| `backend/handlers/refund_appeal_notify_test.go:		TenantID` | 16 |
| `backend/handlers/state_machine_e2e_test.go:		TenantID` | 15 |
| `backend/handlers/confirmation_session_test.go` | 13 |
| `backend/handlers/merchant_setup_test.go:		TenantID` | 13 |
| `backend/handlers/iam_integration_test.go:		TenantID` | 12 |
| `backend/handlers/site_test.go` | 12 |
| `backend/handlers/merchant_member.go` | 12 |
| `backend/handlers/forwarding_session_1934_test.go` | 11 |
| `backend/handlers/invoice_test.go:		TenantID` | 11 |
| `backend/handlers/user_rental.go` | 11 |
| `backend/database/db.go` | 11 |
| `backend/handlers/iam_integration_test.go` | 10 |
| `backend/handlers/invoice.go` | 10 |
| `backend/handlers/repair_service_tenant_2134_test.go` | 9 |
| `backend/handlers/order.go` | 9 |
| `backend/handlers/instrument_media.go:	if err ` | 9 |

## 3. 表清单（含 `tenant_id`，预生产）

| 表 | 同时有 org_id | 约行数 | 判定（待审） |
|---|:---:|---:|---|
| `appeals` | 是 | -1 | ☐ |
| `audit_logs` | 是 | 1349 | ☐ |
| `bak_2211_order_payment_records` | 是 | -1 | ☐ |
| `bak_2211_points_transactions` | 否 | -1 | ☐ |
| `bak_2211_users` | 是 | -1 | ☐ |
| `banners` | 否 | 4 | ☐ |
| `brand_configs` | 否 | -1 | ☐ |
| `categories` | 否 | 32 | ☐ |
| `clients` | 否 | -1 | ☐ |
| `damage_reports` | 是 | 5 | ☐ |
| `deposits` | 否 | -1 | ☐ |
| `electronic_contracts` | 是 | 56 | ☐ |
| `forwarding_sessions` | 是 | -1 | ☐ |
| `instrument_loss_records` | 否 | -1 | ☐ |
| `instrument_media` | 是 | 2312 | ☐ |
| `instrument_photo_specs` | 否 | -1 | ☐ |
| `instrument_promo_overrides` | 否 | -1 | ☐ |
| `instrument_properties` | 否 | 242 | ☐ |
| `instruments` | 是 | 218 | ☐ |
| `inventory_transfers` | 是 | -1 | ☐ |
| `invoice_applications` | 否 | -1 | ☐ |
| `leases` | 否 | -1 | ☐ |
| `maintenance_session_records` | 否 | -1 | ☐ |
| `maintenance_sessions` | 是 | -1 | ☐ |
| `maintenance_tickets` | 是 | -1 | ☐ |
| `maintenance_workers` | 是 | -1 | ☐ |
| `merchant_members` | 否 | 8 | ☐ |
| `merchant_pricing_configs` | 否 | 4 | ☐ |
| `merchant_settlement_configs` | 否 | -1 | ☐ |
| `merchants` | 是 | 3 | ☐ |
| `notifications` | 是 | 80 | ☐ |
| `order_payment_records` | 是 | 161 | ☐ |
| `order_refund_records` | 否 | 26 | ☐ |
| `order_status_history` | 是 | 129 | ☐ |
| `orders` | 是 | 82 | ☐ |
| `ownership_certificates` | 是 | -1 | ☐ |
| `payment_sessions` | 否 | -1 | ☐ |
| `pending_orders` | 否 | -1 | ☐ |
| `points_transactions` | 否 | 3 | ☐ |
| `properties` | 否 | 7 | ☐ |
| `property_options` | 否 | 103 | ☐ |
| `repair_requests` | 否 | -1 | ☐ |
| `roles` | 否 | 32 | ☐ |
| `settlement_calculations` | 否 | -1 | ☐ |
| `site_images` | 否 | -1 | ☐ |
| `site_members` | 否 | 7 | ☐ |
| `sites` | 是 | 2 | ☐ |
| `staff_invites` | 是 | -1 | ☐ |
| `system_settings` | 否 | 21 | ☐ |
| `technician_profiles` | 否 | -1 | ☐ |
| `technicians` | 是 | -1 | ☐ |
| `users` | 是 | 52 | ☐ |

## 4. 可疑模式清单（审核判据）

| # | 模式 | 风险 |
|---|---|---|
| S1 | 用 `users.tenant_id` 做归属/权限判断 | 本地缓存不可靠（api.md:5900） |
| S2 | 把 JWT `tid` 直接当商户范围 | 多商户共用 tenant（3cfa99da）→ 跨商户泄漏 |
| S3 | `tenant_id IN {tid, oid}` 双值过滤 | 多商户共用 tenant 时命中其他商户（#2134 风险） |
| S4 | 写入时用创建者 `tid` 而非数据归属 | 平台管理员建商户数据 → 锚错（维修师案例） |
| S5 | GORM `addTenantScope` 自动范围 | 单值 tenant 无法表达商户维度 |
| S6 | 平台级表（categories/properties/banners 等）把 `tenant_id` 当访问范围 | 应仅为创建者元数据 |
| S7 | 顾客零 UUID tenant（00000000） | 语义占位，非真实归属 |
| S8 | `bak_*` 备份表残留 `tenant_id` | 无业务意义，删列前先清理 |

## 5. 审核方法（逐文件）

1. 打开文件 → 定位每处 `tenant_id/TenantID` → 判定属于：归属判断 / 粗筛 / 元数据 / 写入赋值 / 测试夹具
2. 归属判断一律改为：解析商户（由 JWT `oid/tid` → `merchants`）→ 以 **商户 org** 或 membership 定位
3. 写入一律由**数据归属**推导（非创建者上下文）
4. 每个目录补隔离测试（跨商户 × 跨网点 × 顾客）

## 6. 阶段计划

| 阶段 | 内容 | 出口条件 |
|---|---|---|
| P0 | 定标准（归属模型 + 审计判据 + 新代码禁令） | 本文档评审通过 |
| P1 | 修已知缺陷（维修师归属、平台管理员越权、#2134 重叠） | 红绿测试 + 隔离矩阵 |
| P2 | 逐表数据核对与回填 | 校验 SQL 全绿 |
| P3 | 代码停用 `tenant_id`（GORM 回调/context/查询） | 全量测试 |
| P4 | 删列/删索引（先 rename deprecated） | 迁移 up/down + 预生产验证 |
| P5 | 清理文档/前端/备份表 | 验收报告 |

> 说明：本次仅产出审计基线，不做代码重构。
