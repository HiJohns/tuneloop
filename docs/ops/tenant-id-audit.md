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

## A1 隔离机制层审核结论（middleware / database）
| 位置 | 机制 | 判定 | 结论 |
|---|---|---|---|
| `middleware/iam.go:272,372` | JWT claims → context `tenant_id`（`tid`，回退 `oid`） | **S2 高危** | `tid` 对商户账号 = 商户 org；对平台管理员 = 命名空间主组织；同一字段承载两种语义 |
| `database/db.go` `addTenantScope` | Query/Delete 自动追加 `WHERE tenant_id = ctx` | **S5 高危** | 单值 tenant 无法表达商户维度；多商户共享 tenant 时会放大范围 |
| `database/db.go` `setTenantIDFromContext` | Create/Update 自动填充 tenant/org | **S4 高危** | 写入用“操作者上下文”而非数据归属（维修师建档案例） |
| `database.IdentityCtx` | 清空 tenant 以绕过自动范围 | **绕行信号** | 非测试使用 23 处（repair_service 12 / admin_2129 5 / user_staff 2 / db 2 / role 1 / order 1） |
| `user_management.go` / `settings.go` / `gift_policies.go` / `role.go` / `face_review.go` | 显式清空 tenant 以查全量 | **S5/S6** | 平台级功能被迫绕开自动范围（注释自述“否则查询永假/被过滤”） |

**A1 结论**：自动租户范围只能作为**粗筛**，不能承担商户隔离；归属必须由商户 org / membership 显式表达。`IdentityCtx` 的 23 处绕行是 P3 重构优先清单。

## A2 模型层分类（51 个含 TenantID 的模型）
分类：**P**=平台级共享（tenant 仅创建者元数据，S6）；**M**=商户级（应锚商户 org）；**N**=命名空间/审计级。

| 模型 | 分类 | 有 OrgID | 判定 / 目标 |
|---|:---:|:---:|---|
| `Category` | **P** | — | AGENTS 明确：平台级共享（tenant 为创建者元数据） |
| `Property` | **P** | — | AGENTS 明确：平台级共享（tenant 为创建者元数据） |
| `PropertyOption` | **P** | — | AGENTS 明确：平台级共享（tenant 为创建者元数据） |
| `Banner` | **P** | — | 平台级轮播（全站配置） |
| `BrandConfig` | **P** | — | 品牌字典（平台级） |
| `InstrumentPhotoSpec` | **P** | — | 拍照规范（平台级） |
| `Label` | **P** | — | 已豁免/废弃模型（AGENTS 清单） |
| `Role` | **P** | — | IAM 角色模板缓存（平台级） |
| `SystemSetting` | **P** | — | 系统设置（平台级） |
| `Client` | **P** | — | IAM 客户端配置（平台级） |
| `Merchant` | **M** | ✓ | 自身即商户定义：`tenant_id` 为共享 tenant，商户身份 = `org_id` → S2 |
| `Site` | **M** | ✓ | 商户网点：`tenant_id` 应=商户 org（历史混用） |
| `SiteImage` | **M** | — | 商户级：应锚商户 org（审计目标） |
| `SiteMember` | **M** | ✓ | 会员关系：归属应看 `site_id`→站点商户，而非 tenant |
| `MerchantMember` | **M** | ✓ | 商户成员：`tenant_id` 应=商户 org（身份锚） |
| `StaffInvite` | **M** | ✓ | 邀请：`tenant_id`/`org_id` 双写，接受时绑定商户 org |
| `Technician` | **M** | ✓ | 商户级：应锚商户 org（审计目标） |
| `TechnicianProfile` | **M** | — | **S4 实例**：写入用创建者 tid（#2247）→ 改为商户 org |
| `MaintenanceWorker` | **M** | ✓ | 商户级：应锚商户 org（审计目标） |
| `MerchantPricingConfig` | **M** | — | 商户级：应锚商户 org（审计目标） |
| `MerchantSettlementConfig` | **M** | — | 商户级：应锚商户 org（审计目标） |
| `Instrument` | **M** | ✓ | 商户级：应锚商户 org（审计目标） |
| `InstrumentMedia` | **M** | ✓ | 商户级：应锚商户 org（审计目标） |
| `InstrumentProperty` | **M** | — | 商户级：应锚商户 org（审计目标） |
| `InstrumentLossRecord` | **M** | — | 商户级：应锚商户 org（审计目标） |
| `InventoryTransfer` | **M** | ✓ | 商户级：应锚商户 org（审计目标） |
| `OwnershipCertificate` | **M** | ✓ | 商户级：应锚商户 org（审计目标） |
| `Order` | **M** | ✓ | 商户级：应锚商户 org（审计目标） |
| `OrderStatusHistory` | **M** | ✓ | 商户级：应锚商户 org（审计目标） |
| `PendingOrder` | **M** | — | 商户级：应锚商户 org（审计目标） |
| `OrderPaymentRecord` | **M** | ✓ | 商户级：应锚商户 org（审计目标） |
| `OrderRefundRecord` | **M** | — | 商户级：应锚商户 org（审计目标） |
| `PaymentSession` | **M** | — | 商户级：应锚商户 org（审计目标） |
| `SettlementCalculation` | **M** | — | 商户级：应锚商户 org（审计目标） |
| `Lease` | **M** | — | 商户级：应锚商户 org（审计目标） |
| `Deposit` | **M** | — | 商户级：应锚商户 org（审计目标） |
| `RepairRequest` | **M** | — | **S2/S3 实例**：#2134 双值过滤跨商户（#2248）→ 商户 org |
| `MaintenanceTicket` | **M** | ✓ | 商户级：应锚商户 org（审计目标） |
| `MaintenanceSession` | **M** | ✓ | 商户级：应锚商户 org（审计目标） |
| `MaintenanceSessionRecord` | **M** | — | 商户级：应锚商户 org（审计目标） |
| `ForwardingSession` | **M** | ✓ | 商户级：应锚商户 org（审计目标） |
| `ElectronicContract` | **M** | ✓ | 商户级：应锚商户 org（审计目标） |
| `DamageReport` | **M** | ✓ | 商户级：应锚商户 org（审计目标） |
| `Appeal` | **M** | ✓ | 商户级：应锚商户 org（审计目标） |
| `InvoiceApplication` | **M** | — | 商户级：应锚商户 org（审计目标） |
| `Notification` | **M** | ✓ | 按用户归属；tenant 仅粗筛 |
| `PointsTransaction` | **M** | — | 商户级：应锚商户 org（审计目标） |
| `ConfirmationSession` | **M** | ✓ | 商户级：应锚商户 org（审计目标） |
| `InstrumentPromoOverride` | **M** | — | 商户级：应锚商户 org（审计目标） |
| `User` | **N** | ✓ | **S1**：本地缓存，禁止用于归属判断（api.md:5900）→ 仅作展示缓存 |
| `AuditLog` | **N** | ✓ | 审计：tenant/org 仅记录上下文，不参与隔离 |

**A2 结论**：51 个模型中平台级 10、命名空间/审计级 2、商户级 39。商户级中 `User / Merchant / TechnicianProfile / RepairRequest / SiteMember / MerchantMember` 属 S1–S4 必修；其余为 P2 数据核对对象。

> 状态：A1/A2 完成；A3（handlers 分域）、A4（services/前端/文档）待做。

## A3-1 users / auth 域审核结论

| 文件 | 命中模式 | 判定 | 目标改法 |
|---|---|---|---|
| `user_staff.go` | S2/S4 | ListUsers/CreateUser 查重与写入均用 JWT `tid`；**L445 `WHERE tenant_id=? ORDER BY created_at LIMIT 1` 任取商户**（共享 tenant 时必错） | 解析商户 org 后使用；商户查找改由 `oid` 精确匹配；写入用数据归属 |
| `user_rental.go` | 正面样例 / S7 | 游客/顾客从 **instrument 推导 tenant/org**（正确做法）；订单写入 `nilUUID` 仅顾客占位 | 作为 P1 参照模式；后续统一 nilUUID 语义 |
| `member_invite_2114.go` | S2/S4 | 邀请/审批全链路 `site/merchant/invite.tenant_id = tid`，写入用 tid | 商户 org 锚定；邀请记录写入由目标商户推导 |
| `merchant_member.go` | S2/S4 | 商户校验/成员写入均 `tenant_id = tid` | 同上；`merchant_id` 精确校验 |
| `staff_invite.go` | S2/S4 | 列表/发送/接受用 JWT tid 或 invite.tenant；接受时写 `site_members.tenant_id = invite.TenantID` | 邀请归属改为商户 org（发送时落库） |
| `site_member.go` | S2 | 全部按 `site_members.tenant_id = tid` 过滤 | 归属应由 `site_id → sites` + 商户 org 推导 |
| `user_management.go` | 平台级豁免 | 显式清空 tenant 查全量（注释已说明） | 保留（S6 合法例外） |
| `auth.go` | S4/S7 | 注册/推荐写入 `TenantID: tenantID`（上下文可能为空 → 零 UUID） | 注册流程明确零 UUID 语义；不用于归属 |
| `iam_proxy.go` | **S1**/S2/S4 | **L619 `WHERE iam_sub=? AND tenant_id=?` 更新本地缓存**（iam-notes:65 的“路人”根因同类）；L760 `TenantID: userOrgID` 混写 | 移除 tenant 过滤（按 iam_sub 唯一）；org 写入 org 字段 |
| `confirmation_session.go` | S2 | 会话创建/校验按 `tenant_id = tid` | 会话归属改商户 org / 用户 id |

**A3-1 结论**
- 最高危：`user_staff.go:445`（共享 tenant 下任取商户）、`iam_proxy.go:619`（S1 缓存过滤导致静默 0 行更新）
- 共性：**写入一律用操作者 `tid`**（S4），商户身份未参与
- 正面样例：`user_rental.go` 从 instrument 推导归属（数据归属优先）

> 状态：A3-1 完成；A3-2（orders/payments/settlements/invoices）、A3-3（instruments/inventory/media）、A3-4（repair）、A3-5（其余）待做。
