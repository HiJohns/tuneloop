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

## A3-2 orders / payments / settlements / invoices 域审核结论

### 数据锚实测（预生产，已核实）

| 表 | 实际 tenant_id 值 | 锚类型 |
|---|---|---|
| `orders`（83 行） | `bd6bfa4b`（卡丹萨 org） | ✅ 商户 org |
| `instruments`（218 行） | `bd6bfa4b` | ✅ 商户 org |
| `merchant_members`（8 行） | `3cfa99da`（共享 tenant） | ❌ 与代码 `= tid(商户org)` 不匹配 |
| `site_members`（7 行） | `bfce6c6d`（cadenza org）/ `bd6bfa4b`（卡丹萨 org） | ⚠️ 站点所属商户 org |
| `technician_profiles`（4 行） | `3cfa99da` ×3 / `bd6bfa4b` ×1 | ❌ 混用（#2247） |

**结论：同一系统内同时存在 3 种锚（商户 org / 共享 tenant / 站点商户 org），这就是 `tenant_id` 不可信的直接证据。**

### 文件判定

| 文件 | 命中模式 | 判定 | 说明 / 目标 |
|---|---|---|---|
| `order.go` | S2 | 列表按 `tenant_id = tid` 过滤；L711 商户名按 `order.tenant_id` 反查（无唯一性） | 订单已锚商户 org → 商户管理员可用；反查需按 org 精确 |
| `invoice.go` | S2/S3 | `tenant_id IN ?` 分组 + 按 tenant 反查商户 | 订单 org 锚下可用，但分组/反查需改为 org |
| `warehouse.go` | S2 | 订单操作 `id=? AND tenant_id=tid` | 商户 org 下可用；需补网点角色用例 |
| `merchant_invoice.go` | S2/S4 | 列表/详情 `= tid`；写 `TenantID: tid`；商户反查按 tid | 改商户 org |
| `billing.go` | S2/S4 | `orders.tenant_id = tid`；写 `TenantID: tid` | 读取改 org；写入取数据归属 |
| `merchant_order.go` | S2 | `orders.tenant_id = tid` | 同上 |
| `pending_order.go` | **S1 邻接** | 写入 `TenantID: u.TenantID`（用户缓存） | 用户缓存不可靠 → 改订单商户 org |
| `user_settlement.go` | ✅ 正面 | 全部 `order.TenantID` | 数据归属优先（保持） |
| `wechatpay_prepay.go` / `renewal.go` | ✅ 正面 | 顾客空 tid → **从 order 推导** | 保持，作为 P1 参照 |
| `warehouse.go` 写入 | S4 | 记录 `TenantID: tid` | 改订单归属 |

**A3-2 结论**
- 订单/乐器数据已按**商户 org** 锚定，是现有最一致的部分
- **高危**：`merchant_members` 数据锚（共享 tenant）与代码过滤（商户 org）不一致 → 成员列表/角色校验可能空/失败（建议独立验证 Issue）
- 写路径仍存在 `TenantID: tid`（S4）与用户缓存（S1 邻接）
- 正面：prepay/renewal 从订单推导；settlement 全部用 order.TenantID

> 状态：A3-2 完成；A3-3（instruments/inventory/media）、A3-4（repair）、A3-5（其余）待做。

## A3-3 instruments / inventory / media 域

| 文件 | 模式 | 判定 |
|---|---|---|
| `instrument.go` / `instrument_media.go` / `inventory.go` / `api.go` / `instrument_scrap.go` | S2/S4 | 乐器已锚商户 org，`tenant_id=tid` 对商户管理员可用；写入用 tid（S4） |
| **`instrument_batch_import.go`** | **S6/S2** | 属性/选项查询 `WHERE tenant_id=tid` 命中**平台级表**（实测 properties=`3cfa99da`/零 UUID）→ 商户管理员（org）**查不到平台属性**（疑似批量导入下拉为空） |
| `instrument_loss.go` | ✅ | 写入用 `inst/order.TenantID`（数据归属） |
| `instrument_promo_override.go` | S4 | 覆盖配置写 `tid` → 应商户 org |
| `public.go` | S2/S6 | 覆盖查询用 `merchant.OrgID`（✅）；`system_settings` 按 tenant 读写（平台级，S6） |

## A3-4 repair 域

| 文件 | 模式 | 判定 |
|---|---|---|
| **`repair_service.go`** | **S2** | L586 `WHERE tenant_id=? OR org_id=? ORDER BY created_at LIMIT 1` **任取商户**（共享 tenant 必错）；IdentityCtx 12 处；`tenant_id IN ?` 范围 |
| `repair_service_admin_2129.go` | **S3** | #2134 双值过滤 → #2248 |
| `repair_request.go` | ✅/S2 | 写入 `req.TenantID`（数据归属 ✅）；列表按 tid（S2） |
| `repair_admin.go` | S2/S4 | 列表 tid；写入 tid |
| `repair_config.go` | **S6/S2** | 配置存 `system_settings` 按 tenant → 共享 tenant 的商户**共用同一份配置** |

## A3-5 其余域

| 文件 | 模式 | 判定 |
|---|---|---|
| `appeal.go` | S2/S4 | 全链路 `id=? AND tenant_id=tid`；写入 tid |
| `merchant.go` / `site.go` | S2 | 商户/网点列表按 `merchant/site.tenant_id=tid`；网点写入 tid（S4） |
| `pricing.go` / `merchant_rebate.go` / `points_policy.go` | **S6/S2** | 商户配置按 tenant 存取 → 共享 tenant 时**跨商户共用配置** |
| `forwarding_session.go` / `transit_site_admin.go` / `transit_order.go` | S2/S4 | 中转单按 tid 过滤/写入 |
| `property.go` / `label.go` | S6 | 平台级资源写入**操作者 tenant** → properties 出现 `3cfa99da` 与零 UUID 混杂 |
| `settings.go` / `role_manage.go` / `permission_manage.go` / `audit_log.go` / `banner.go` / `wechatpay_admin.go` / `webhook_config.go` / `smtp_config.go` / `system.go` | S6/平台级 | 已显式清空 tenant 或仅记录；保留但纳入 P4 字段清理 |
| `gift_policies.go` / `face_review.go` | S5/S6 | 显式清空 tenant 绕行（合法例外，P3 统一机制） |
| `auto_confirm.go` / `reserved_order_scheduler.go` / `logistics_monitor.go` | ✅ | 由数据行（order）驱动 tenant（数据归属） |
| `quote.go` / `repair.go` / `instrument_import.go` / `bulk_import.go` / `user_onboarding.go` / `registration_*` / `pending_order.go` | S4/S2 | 写入用上下文 tenant 或用户缓存（S1 邻接） |
| `iam_callback.go` | ✅ | 补建用户写零 UUID（明确占位语义） |

## A4 services / frontend / docs

- **services**：`iam_bootstrap.go`(10)、`email.go`(10)、`webhook.go`(8)、`point_batches.go`(5) 等按 tenant 循环/查询 → 与 P2/P3 一并处理（非本阶段改造）
- **frontend 26 处**：`tenant_id` 仅在少数请求/展示 payload 中 → 随 API 口径调整同步
- **docs 216 处**：集中更新 `api.md`（租户口径）、`cases/organization.md`、`user-management.md`（归属锚）→ P5

## A3/A4 总结：必修清单（按优先级）

| 优先级 | 位置 | 问题 | 关联 |
|---|---|---|---|
| P0 | `user_staff.go:445` | 共享 tenant 下任取商户 | #2247 |
| P0 | `repair_service.go:586` | 同上（OR tenant/org + LIMIT 1） | #2248 |
| P0 | `iam_proxy.go:619` | `users.tenant_id` 过滤 → 静默 0 行更新（S1） | 新 Issue |
| P1 | `merchant_members` 数据锚（共享 tenant）≠ 代码（商户 org） | 成员/角色校验可能失败 | 新 Issue |
| P1 | `technician_profiles` 混用 | 商户管理员不可见 | #2247 |
| P1 | `#2134` 双值过滤 | 跨商户重叠 | #2248 |
| P1 | `instrument_batch_import` / `pricing` / `repair_config` / `settings` | 平台/商户配置按 tenant → 共享/查空 | P2 数据核对 |
| P2 | S4 写入（`TenantID: tid`）全域 | 改数据归属 | P3 |

> 覆盖率：`docs/ops/tenant-id-audit-files.csv` 中全部 backend handler 文件已归类（详细 40 个 + 同类归组 40 个）；无未分类文件。

## A3-6 补充判定（返工补全 15 个未判定文件）

> 来源：#2246 审计（覆盖率实测 139 个 backend 文件中 15 个非测试源文件未判定）。判定口径同 §5。

| 文件 | refs | 使用形态 | 模式 | 判定 / 目标改法 |
|---|---:|---|---|---|
| `backend/handlers/activity_log.go` | 2 | 乐器活动日志：orders / order_status_history / instrument_media 按 `tenant_id = tid` | S2 | **可保留**：三类数据均已锚商户 org，商户管理员 tid=org 一致 |
| `backend/handlers/personnel.go` | 6 | 商户全员视图按 tid 聚合 `sites.tenant_id` / `merchant_members.tenant_id` / `technician_profiles.tenant_id`；平台分支按 `rootOrgID` 查中转网点 | S2 | **可保留**：三表均已锚商户 org；平台分支为平台语义 |
| `backend/handlers/site_member_common.go` | 1 | 网点成员写入 `TenantID = tenantID`（JWT tid）；P1 成员判定用 `site_members.tenant_id` | S4 | **可保留（调用方锚）**：网点管理员 tid=商户 org；若出现共享 tenant JWT，需改 `site.TenantID`（记录备查） |
| `backend/handlers/technician_profile.go` | 2 | List/Create 已按商户 org（#2247）；Get/Update/Delete 仍 `tenant_id = tid`；审计日志 tenant 为元数据 | S2（残留） | **部分可保留**：Get/Update/Delete 待与 List 同口径（目标 `merchantScopeIDs2247`），已登记为后续统一项 |
| `backend/handlers/user_management_idcard.go` | 2 | 通知使用 `user.TenantID`（本地缓存） | S1 邻接 | **可保留（仅通知元数据）**：平台级审核操作；租户不参与权限判定 |
| `backend/models/models.go` | 1 | 模型字段定义（TenantID/OrgID） | 元数据 | **可保留**：模型分类见 §A2（P 10 / N 2 / M 39） |
| `backend/scripts/migrate_media_keys.go` | 1 | 一次性迁移工具输出 TenantID | 工具 | **可保留**：非运行时路径 |
| `backend/services/audit_query.go` | 3 | 审计日志查询按 tenant 过滤（平台角色走全量） | S2 | **可保留**：`audit_logs.tenant_id` = 操作者 org |
| `backend/services/audit_writer.go` | 1 | 审计写入 TenantID（调用方显式传入） | 元数据 | **可保留** |
| `backend/services/bulk_import_accounts.go` | 2 | 批量建号：users/sites 查询与写入使用 `tenantID` 参数 | S4 | **可保留（调用方锚）**：调用方（bulk_import.go）传操作者 tid，商户管理员=org；平台导入为平台上下文 |
| `backend/services/bulk_import_orgs.go` | 1 | 批量建组织：sites 查询与写入使用 `tenantID` 参数 | S4 | 同上 |
| `backend/services/collection_reminder_scheduler.go` | 2 | 通知使用 `order.TenantID`（数据归属） | 数据 | **可保留**：订单已锚商户 org |
| `backend/services/notification.go` | 1 | 通知按显式 `tenantID` 写入 / 查管理员 | 元数据 | **可保留**：显式传参，不读取操作者上下文 |
| `backend/services/rent_calculator.go` | 1 | 促销计划匹配 `plan.ScopeID == tenantID`（tenant 来自订单/乐器） | 数据 | **可保留**：tenant 为数据归属（org 锚）；`scope_id` 语义同 #2251（商户 org） |
| `backend/testutil/role_factory.go` | 3 | 测试夹具注入 JWT tenant/org/user/role | 夹具 | **可保留**：仅测试 |

## A3-7 测试文件同组清单（59 个）

> 分组规则：按被覆盖业务域归组；测试夹具（`TestActor` / `SiteMember` / `Merchant`，见 `testutil/role_factory.go`）统一视为「夹具组」，其 tenant/org 注入用于构造跨范围用例，不逐断言判定。以下文件均视为**已判定（同组）**。

- **A. 订单 / 租赁 / 结算 / 财务（17）**：`merchant_order_test.go`、`order_delivery_address_2010_test.go`、`pending_order_test.go`、`user_rental_test.go`、`user_rental_1762_test.go`、`lease_early_return_test.go`、`renewal_test.go`、`renewal_1781_test.go`、`settlement_1743_test.go`、`settlement_1764_test.go`、`settlement_flow_test.go`、`shortfall_1746_test.go`、`payment_calculate_1758_test.go`、`prepay_coupon_snapshot_test.go`、`promo_points_1757_test.go`、`promo_override_1863_test.go`、`warehouse_test.go`
- **B. 维修 / 乐器 / 库存 / 媒体（13）**：`repair_request_test.go`、`repair_service_test.go`、`repair_service_reassign_2229_test.go`、`repair_service_receive_2116_test.go`、`inspect_return_1801_test.go`、`inspect_return_shortfall_test.go`、`instrument_import_test.go`、`instrument_props_1896_test.go`、`instrument_sort_1797_test.go`、`instrument_update_test.go`、`inventory_test.go`、`mall_image_test.go`、`media_cleanup_test.go`
- **C. 人员 / 商户 / 网点 / 邀请（13）**：`merchant_member_test.go`、`personnel_2065_test.go`、`member_invite_2114_test.go`、`member_invite_2114_iam_test.go`、`staff_invite_test.go`、`platform_staff_1795_test.go`、`transit_site_admin_test.go`、`create_user_2068_test.go`、`user_staff_test.go`、`update_current_user_test.go`、`user_address_test.go`、`user_me_tenant_name_2117_test.go`、`weapp_staff_flow_test.go`
- **D. 会员 / 乐币 / 积分（2）**：`point_settings_test.go`、`point_batches_test.go`
- **E. 识别 / 认证 / 权限（6）**：`face_verify_test.go`、`face_review_1791_test.go`、`auth_test.go`、`iam_proxy_test.go`、`permissions_1964_test.go`、`guarantor_ocr_test.go`
- **F. 平台 / 系统 / 审计 / 内容（8）**：`api_test.go`、`appeal_test.go`、`audit_jsonb_2062_test.go`、`banner_test.go`、`cents_migration_test.go`、`dashboard_stats_2005_test.go`、`notification_test.go`、`webhook_1909_test.go`

## A5 隔离测试矩阵（跨商户 / 跨网点 / 顾客）

> 用途：P1/P3 执行 Issue 的回归对照组。状态：✅=已有测试；⬜=待补（随对应执行 Issue 落地）。

| 用例 | 角色 | 场景 | 断言（行级） | 状态 / 落地 |
|---|---|---|---|---|
| ISO-01 | 商户 A 管理员 | 维修服务单列表（共享 tenant） | 仅见 A 单；B 单不可见 | ✅ `TestListMerchantRepairServices_SharedTenantNoLeak_2248` |
| ISO-02 | 商户 A 管理员 | 新建维修服务单 → 列表 → 员工可操作 | `tenant_id == A.org`；A 列表可见；staffAllowed=true | ✅ `TestCreateRepairService_WriteAnchoredToMerchantOrg_2248` |
| ISO-03 | 商户 B 管理员 | 删除 A 的直属成员 | 403；A 成员行保留；IAM 解绑未被调用 | ✅ `TestRemoveMember_SharedTenant_OrgAnchorAndIAMUnbind_2250` |
| ISO-04 | 商户 A 管理员 | 删除本商户成员（共享 tenant） | 200；本地行删除；IAM 解绑调用 `org=A.org` | ✅ 同上 |
| ISO-05 | 商户 A/B 管理员 | 定价配置读取/写入（共享 tenant） | 各自 `tenant_id == 本商户 org`；互不覆盖 | ✅ `TestMerchantPricingConfig_SharedTenantIsolation_2251` |
| ISO-06 | 商户 A/B 管理员 | 维修设置（检查费/运费/赠点开关） | A 写入值 B 不可见 | ✅ `TestRepairSettings_SharedTenantIsolation_2251` |
| ISO-07 | 商户 A/B 管理员 | 乐币策略列表/创建/跨商户更新 | scope_id=A.org；B 列表为空；B 改 A 策略 404 | ✅ `TestPointsPolicies_SharedTenantIsolation_2251` |
| ISO-08 | 商户 A/B 管理员 | 返点开关读取/写入 | B 不读 A 写入；仅本商户行更新 | ✅ `TestMerchantRebate_SharedTenantIsolation_2251` |
| ISO-09 | 商户 A/B 管理员 | 平台级 home_menu_config 写入/读取 | 存储于平台 tenant（nil UUID）；两端读取一致 | ✅ `TestHomeMenuConfig_PlatformLevel_2251` |
| ISO-10 | 平台管理员 | 创建商户员工/维修师 | 403 / code 40310 | ✅ `TestCreateUser_PlatformAdminRejected_2247` |
| ISO-11 | 商户管理员 | 创建维修师 | `technician_profiles.tenant_id == 商户 org` | ✅ `TestCreateUser_MerchantAdminProfileAnchored_2247` |
| ISO-12 | 商户管理员 | 关联非 active / 非本商户用户为维修师 | 400 | ✅ `TestTechnicianProfileCreate_InactiveUserRejected_2247` |
| ISO-13 | 商户 A 管理员 | 跨商户维修服务单详情 / 转派 / 收货 / 结算 | 404 / 403；状态与金额不变 | ✅ 列表隔离 `TestRepairService_LeafOrgScope_Isolation_2134`；转派跨商户目标 40003 `TestReassignRepairService_2229_Validation`；跨网点操作 403 `TestAcceptRepair_OtherSite_Forbidden` |
| ISO-14 | 网点管理员 | 跨网点订单/乐器详情 | 跨商户 → 404/403；本商户 org 锚可读 | ✅ 自动化：`TestIsolationMatrix_OrderDetail_CrossMerchant_ISO14`（站点级列表隔离由 #1911/#2116 覆盖） |
| ISO-15 | 顾客（零租户） | 下单/读取/我的订单 | 不因 tid 为空被拒；数据落商户 org；跨顾客 404 | ✅ 自动化：`TestIsolationMatrix_CustomerZeroTenant_Chain_ISO15`（拍付/收货动作由 `prepay_2110`/`#2116` 组覆盖，wxpay 真链路列部署侧走查） |
| ISO-16 | 顾客 | 发票申请：伪造 `tenant_id=B`；跨商户订单合并 | 伪造 → 400 且无申请行；不传 → 落订单商户 org；跨商户合并 → 400 | ✅ `TestInvoiceSubmit_TenantDerivedFromOrders_2262` |
| ISO-17 | 商户 A/B 管理员 | 共享 tenant 下各自 webhook 配置 | A/B 互不覆盖；旧 tenant 键在 scoped ctx 下只读回退可用 | ✅ `TestMerchantWebhook_LegacyKeyFallbackUnderScopedCtx_2262`、`TestMerchantWebhook_SharedTenantIsolation_2262`（#2262 返工） |
| ISO-18 | 商户/网点管理员、顾客 | B3 显式模式下的 repair / orders-instruments / membership 路由 | 跨商户 404/空；本商户 200；on/off 等价 | 🟡 **待 #2261 审计 PASS**（返工 `e80de659`：8 条已守卫路由；其余回滚）。测试：`TestRepairDomain_ExplicitScopeIsolation_2261`、`TestOrdersInstrumentsDomain_ExplicitScopeIsolation_2261`、`TestMembershipDomain_SitesExplicitScope_2261`、`TestOrdersByTradeNo_ExplicitScopeIsolation_2261`、`TestOrdersInstrumentsDelete_ExplicitScope_2261` |

## A6 覆盖率校验（可复跑）

```bash
bash scripts/tenant-audit-coverage.sh
# 输出示例：backend files: 139 | covered: 139 | uncovered: 0
```

校验标准：`docs/ops/tenant-id-audit-files.csv` 中每个 backend `.go` 文件必须以文件名出现在本文档（§A1–A4 或 §A3-6/A3-7 分组清单）。未覆盖 → 脚本 exit 1 并列出文件。

| 指标 | 数值 |
|---|---|
| CSV backend 文件总数 | 139 |
| 已判定 | 139 |
| 未判定 | 0 |
| 测试文件同组映射 | 59 / 59 |

## A3-8 显式判定归一表（必修 / 可保留）

> 来源：#2246 审计意见「A3 表格判定栏以叙述为主，缺显式二选一」。本表对 A3-1~A3-5 全部文件给出显式归类；「必修」= 命中 S1–S4/S6 且目标改法已登记（括号内为落地 Issue / 批次）。

| 判定 | 文件（目标改法落地） |
|---|---|
| **必修** | `user_staff.go`（#2247 已修）、`member_invite_2114.go`（P3）、`merchant_member.go`（#2250 已修）、`staff_invite.go`（P3）、`site_member.go`（P3）、`iam_proxy.go`（#2249 已修）、`confirmation_session.go`（P3）、`invoice.go`（P2）、`warehouse.go`（写入 S4，P3）、`merchant_invoice.go`（P2/P3）、`billing.go`（P2/P3）、`pending_order.go`（S1 邻接，P3）、`instrument_batch_import.go`（#2251 已修）、`instrument_promo_override.go`（P2/P3）、`public.go`（#2251 已修）、`repair_service.go`（#2248 已修）、`repair_service_admin_2129.go`（#2248 已修）、`repair_admin.go`（P3）、`repair_config.go`（#2251 已修）、`appeal.go`（P3）、`merchant.go` / `site.go`（P3）、`pricing.go` / `merchant_rebate.go` / `points_policy.go`（#2251 已修）、`forwarding_session.go` / `transit_site_admin.go` / `transit_order.go`（P3）、`property.go` / `label.go`（S6 写入，P3） |
| **可保留** | `user_rental.go`（正面：从 instrument 推导）、`user_management.go`（平台级豁免）、`auth.go`（零 UUID 占位语义，P4 统一）、`order.go` / `merchant_order.go`（读取按订单 org 一致；反查优化归 P2）、`user_settlement.go` / `wechatpay_prepay.go` / `renewal.go`（正面：数据归属）、`instrument.go` / `instrument_media.go` / `inventory.go` / `api.go` / `instrument_scrap.go`（读取可保留；写入 S4 已并入 P3）、`instrument_loss.go`（数据归属）、`repair_request.go`（写入数据归属；列表 S2 归 P3）、`settings.go` / `role_manage.go` / `permission_manage.go` / `audit_log.go` / `banner.go` / `wechatpay_admin.go` / `webhook_config.go` / `smtp_config.go` / `system.go`（平台级显式清空 tenant）、`gift_policies.go` / `face_review.go`（合法例外）、`auto_confirm.go` / `reserved_order_scheduler.go` / `logistics_monitor.go`（数据驱动）、`quote.go` / `repair.go` / `instrument_import.go` / `bulk_import.go` / `user_onboarding.go` / `registration_*`（调用方锚；S1 邻接随 P3）、`iam_callback.go`（零 UUID 明确占位） |

> 判定以本表逐项为准；同一文件若读取可保留、写入必修，已在该文件条目注明（如 `warehouse.go`、`instrument.go`）。

## A7 P3 执行进度（分批，每批独立可回滚）

> 依据 #2255 计划「分批提交、禁止与 bug fix 混提交」。批次状态供审计跟踪。
> 拆分记录（#2255 评论 2026-10-22）：B0→#2259 / B2b→#2260 / B3→#2261 / B4B5→#2262 / B6→#2263；P3 本体转为集成父。

| 批次 | 内容 | 状态 | 验证 |
|---|---|---|---|
| B0 | **隔离守卫测试网**：ISO-14/15 自动化（守卫网先于回调改造存在） | ✅（#2259） | `TestIsolationMatrix_*` ×2 全绿
| B1 | **商户归属解析统一**：`resolveOperatorMerchantConfig`（#2251）改为复用 `resolveOperatorMerchant2247`（#2247）——单一实现（org 优先 → 唯一 tenant → 共享不解析） | ✅ 已完成 | `TestResolveMerchantOrg_Precedence_2255`（org 优先/唯一 tenant/共享拒绝/未知身份 4 例）+ #2247/#2251 原测试回归 |
| B2a | **S4 写入数据归属（缓存表）**：`pending_order` 的 tenant 改为乐器所属商户 org（原用 `users.tenant_id` 缓存，顾客零租户/共享 tenant 均不可靠） | ✅ 已完成（`pending_order`） | 红绿：`TestCreatePendingOrder_TenantFromInstrument_2255`（父提交=零租户/共享 → 修复=商户 org）+ 无乐器回退平台零 UUID 负例 |
| B2b | **S4 写入数据归属（逐点显式化）** | 🟡 部分（2 点）：乐器促销覆盖 → 乐器商户 org（+跨商户 404）；商户结算配置 → 商户 org（修 computePaymentDeadline 口径） | 红绿：`TestPromoOverride_WriteAnchoredToInstrumentOrg_2260` / `TestSettlementConfig_AnchoredToMerchantOrg_2260`（父提交红）；**回调判定**：`setTenantIDFromContext` 现状只在字段零值时填充 → 显式归属点改造后自动不介入（守卫式 no-op 成立） |
### B2b S4 逐点判定表（2026-10-22）

| S4 点 | 判定 | 证据/理由 |
|---|---|---|
| `pending_order.go` 创建缓存单 | ✅ 已修（B2a） | 顾客零租户/共享 tenant → 乐器商户 org（红绿 ×2） |
| `instrument_promo_override.go` 读写 | ✅ 已修 | 操作者 tid → 乐器商户 org；平台管理员 uuid 空串 500 修复；跨商户 404（红绿） |
| `billing.go` 结算配置 Get/Upsert | ✅ 已修 | 共享 `merchants.tenant_id` → 商户 org；修复 `computePaymentDeadline` 读不到配置（红绿） |
| `warehouse.go` 7 处（状态历史/定损/媒体/通知） | ⚪ 不适用（门控等价） | 每个 handler 先以 `orders.tenant_id = 操作者 tid` 取单（warehouse.go:152/186/193…）→ 写入时 `tid` 恒等于 `order.TenantID`，改造无可观测差异（无法红绿，禁造假通过测试） |
| `appeal.go` 8 处 | ⚪ 不适用（门控等价） | 同上：申诉单/订单经 tenant 门控加载 |
| `quote/repair/instrument_import/bulk_import` 写入 | ⚪ 不适用（调用方锚） | 调用方传入的 tenant 即操作者商户 org（#2247/#2251 已统一解析），无独立偏差 |
| `site.go` `CreateSite` 写入 | ⚠️ **待决策**（跟踪 Issue #2266） | 平台管理员（SysPermOrganizationCreate）创建网点时请求体**无目标商户**；锚只能取操作者 tid（平台/空）→ 数据归属不明 |
| `repair_admin.go` `CreateAppeal` | ✅ 已修 | 归属 = 被申诉对象（repair_request/order/damage）的 tenant；`org_id/site_id` 空串改 Omit（同根 22P02）。红绿：父提交 500 → 修复 200/org 锚 + 未知对象→平台零 UUID |
| `invoice.go` | ⚪ 已正确 | 写入取 `order.TenantID` / `group.TenantID`（数据归属） |
| `merchant_invoice.go` | ⚪ 不适用 | 写入为**通知**元数据（target=申请用户；tenant 仅路由） |
| `merchant.go` | ⚪ 不适用（S2 读取归 P3） | 商户定义行 `merchant.go:354` 的 `tenant_id` = 创建者/平台共享 tenant（A2：Merchant 的 tenant 为共享值，身份 = `org_id = iamOrgID`）；`initSystemRoles`（:719）用新商户 tenant 建角色。本批 S4 不适用，列表读取按 tenant 的 S2 归 P3 |
| `forwarding_session.go` | ⚪ 不适用（调用方数据锚） | `createForwardingSession`（:82）由调用方传 tenant：`user_rental.go:704` 传 `instrument.TenantID`；`merchant_id` 优先操作者 org、无 org 时回退该 tenant（:85-89）。写入值来自订单/乐器数据归属，非创建者上下文 |
| `label.go` | ⚪ 不适用（P 表 / S6） | `label.go:75` 写入 `tenant_id = 操作者 tid`，但 Label 属 A2 的 **P（平台级）** 表，tenant 仅创建者元数据；按 tenant 做访问范围属 S6，归 P3/P5 统一，不属本批 S4 |
| A3-8 其余必修（前批已修） | ✅ 已修（非本批） | `user_staff.go`(#2247)、`merchant_member.go`(#2250)、`iam_proxy.go`(#2249)、`repair_service.go`/`repair_service_admin_2129.go`(#2248)、`instrument_batch_import.go`/`public.go`/`pricing.go`/`merchant_rebate.go`/`points_policy.go`(#2251) |
| `transit_site_admin.go` | ⚪ 不适用 | 中转网点按 #1935 直辖平台顶层组织，tenant=org=平台租户（注释明确） |
| `transit_order.go` | ⚪ 不适用 | 中转域操作行，锚 = 平台顶层组织（同 #1935）；未来若允许商户级操作需复核 |
| `property.go` | ⚪ 不适用 | 平台级资源（P 表），tenant = 创建者元数据（设计如此） |
| `confirmation_session.go` | ⚪ 不适用 | 会话归属 user_id，tenant 仅上下文元数据 |
| `staff_invite.go` / `site_member.go` / `member_invite_2114.go` | ⚪ 不适用 | 写入前经 `hasSiteAccess`（sites.tenant_id = 操作者 tid）→ 与 site.TenantID 恒等 |
| `repair_config.go` | ✅ 已修（#2251） | 维修设置锚商户 org / 平台 nil UUID |
| B3 | 读回调逐域显式化（opt-in 等价迁移） | 🟡 批次1 ✅ 开关+观察；批次2 本地统计（9 表）✅；**批次3 返工后仅保留 8 条已核查+已守卫路由**（§A7-B3）；其余原批次路由**已回滚**（repair 组、orders/instruments 详情与写删、membership 商户/站点写等），由 **#2276** 跟踪逐路由补过滤+守卫后再切；#2269 判定有误已回滚（`e8a64aa4`）；观察期实采 12 表（真实口径：sites.tenant_id=商户 org、merchants.tenant_id=共享 tenant/org_id=商户 org） | `TestExplicitTenantScopeOnly_2261`、`TestRepairDomain_...`、`TestOrdersInstrumentsDomain_...`、`TestMembershipDomain_Sites...`、`TestTechnicianProfilesDomain_...`、`TestOrdersByTradeNo_ExplicitScopeIsolation_2261`、`TestOrdersInstrumentsDelete_ExplicitScope_2261`；全量 10 failed / NEW 空。预生产观察日志已核对（8 条切域无注入）。 |
| B4 | A3-8 必修清单（P2/P3 批 25 文件）口径改造 | 🟡 逐文件判定完成；本轮修 `invoice.go`（归属由订单派生）+ `member_invite_2114.go`（org 锚） | 见 §A7-B4 |
| B5 | 前端/services tenant payload 收敛 | 🟡 盘点 88 行（63/21/4）→ `docs/ops/tenant-payload-audit-2262.md`；改 `webhook.go`（org 键 + 旧键只读回退）、`InvoiceConfirm` 移除 payload | 与 API 口径同步 |
| B6 | 隔离矩阵补齐（ISO-14~18） | 🟡 自动化部分：ISO-16/17 ✅（#2262 返工）；**ISO-18 待 #2261 审计 PASS**；预生产走查待最终关闭时 waitForConfirm | 行级断言


## §A7-B4 逐文件判定（#2262）

| 文件 | 判定 | 动作/证据 |
|---|---|---|
| `user_staff.go` | 已修 #2247 | 仅复核 |
| `merchant_member.go` | 已修 #2250 | 仅复核 |
| `iam_proxy.go` | 已修 #2249 | 仅复核 |
| `instrument_batch_import.go` / `public.go` | 已修 #2251 | 仅复核 |
| `repair_service.go` / `repair_service_admin_2129.go` | 已修 #2248 | 仅复核 |
| `repair_config.go` / `pricing.go` / `merchant_rebate.go` / `points_policy.go` | 已修 #2251 | 仅复核 |
| `pending_order.go` | 已修 B2a | 仅复核 |
| `instrument_promo_override.go` | 已修 B2b | 仅复核 |
| `billing.go` | 已修 B2b + `86774cf1` | 仅复核（#2261 显式范围） |
| `repair_admin.go`（CreateAppeal） | 已修 B2b | 仅复核 |
| `warehouse.go` | 写入经 `orders.tenant_id = tid` 门控 | 登记「等价」，不改 |
| `appeal.go` | 申诉单/订单门控加载 | 登记「等价」，不改 |
| `forwarding_session.go` | 调用方传 `instrument.TenantID` | 登记「调用方数据锚」，不改 |
| `transit_site_admin.go` / `transit_order.go` | #1935 平台顶层组织 | 登记「平台域」，不改 |
| `property.go` / `label.go` | P 表/S6 元数据 | 登记「P 表」，不改 |
| `merchant.go` | 商户定义行 tenant=创建者/共享，身份=org | 登记「定义/读取」，不改 |
| `confirmation_session.go` | `tenant_id = tid` 门控目标用户 | 登记「门控等价」，不改 |
| `staff_invite.go` / `site_member.go` | 经 `hasSiteAccess`（`sites.tenant_id`） | 登记「门控等价」，不改 |
| `site.go` | #2266（`87dabfe2`）锚=商户 org | 仅复核 |
| `invoice.go` | **需改**：客户端可指定 tenant | ✅ 归属由订单派生；`TestInvoiceSubmit_TenantDerivedFromOrders_2262` 红绿 |
| `merchant_invoice.go` | 按申请单 tenant 读 | 仅复核 |
| **`member_invite_2114.go`** | **需改**：`GetMyMerchant` OR 回退共享 tenant | ✅ 改 `resolveOperatorMerchantConfig`；`TestMyMerchant_SharedTenantOrgAnchor_2262` 红绿 |
| `services/webhook.go` | **需改**：商户身份用共享 tenant 作键 | ✅ 改商户 org（双读单写）；`TestMerchantWebhook_SharedTenantIsolation_2262` / `TestMerchantTenantID_ReturnsMerchantOrg_2262` |
| `frontend-mobile/src/pages/InvoiceConfirm.jsx` | 请求 payload 携带 `tenant_id` | ✅ 已移除 payload（分组展示保留） |

## §A7-B3 路由 × 表 显式过滤清单（#2261 返工）

> 审计返工后**只保留**以下已核查 + 有守卫的路由挂载 `middleware.ExplicitTenantScope()`；
> 其余原批次路由已**回滚开关**（恢复自动租户范围），待逐路由清单与守卫补齐后再切。

### 已切换（8 条）

| 路由 | Handler | 触达表 | 显式过滤证据 | 守卫 |
|---|---|---|---|---|
| `GET /instruments` | `GetInstruments` | instruments | `resolveOperatorMerchant*` → `tenant_id IN (org, tenant)`（`instrument.go` 列表范围） | `TestOrdersInstrumentsDomain_ExplicitScopeIsolation_2261` |
| `GET /technician-profiles` | `TechnicianProfileHandler.List` | technician_profiles, users | `tenant_id IN merchantScopeIDs2247`（`technician_profile.go:84+`） | `TestTechnicianProfilesDomain_ExplicitScopeIsolation_2261` |
| `DELETE /instruments/:id` | `DeleteInstrument` | instruments, orders | `id = ? AND tenant_id = ?`（`instrument.go:950`） | `TestOrdersInstrumentsDelete_ExplicitScope_2261` |
| `GET /orders/by-trade-no/:out_trade_no` | `GetOrdersByOutTradeNo` | payment_sessions, session_order_links, orders | `out_trade_no = ? AND tenant_id = ?`；orders 同（`order.go:1723/1749`） | `TestOrdersByTradeNo_ExplicitScopeIsolation_2261` |
| `POST /orders/:id/cancel` | `CancelOrder` | orders, order_payment_records, refunds | `id = ? AND tenant_id = ?`（`order.go:1048`） | `TestOrdersInstrumentsDelete_ExplicitScope_2261` |
| `GET /admin/repair-services` | `ListMerchantRepairServices` | repair_requests, merchants | 解析商户 org → `tenant_id IN merchantScopeIDs`（`repair_service_admin_2129.go:31-56`） | `TestRepairDomain_ExplicitScopeIsolation_2261` |
| `PUT /merchant/sites/:id` | `UpdateSite` | sites | `id = ? AND tenant_id = ?`（`site.go:547`） | `TestMembershipDomain_SitesExplicitScope_2261` |
| `GET /sites/:id/members` | `SiteMemberHandler.ListMembers` | sites, site_members, users | `hasSiteAccess`：`sites.id = ? AND sites.tenant_id = ?`（`site_member.go:325+`） | `TestMembershipDomain_SitesExplicitScope_2261` |

### 批次 A（#2276）：repair 只读（8 条新增）

| 路由 | Handler | 触达表 | 显式过滤证据 | 守卫 |
|---|---|---|---|---|
| `GET /repair/mine` | `ListMyRepairs` | instruments | `repair_worker_id = 操作者 user_id`（`repair.go:463`） | 用户维度守卫 |
| `GET /repair/pending` | `ListPendingRepairs` | instruments, site_members | `current_site_id IN 操作者站点成员hips`（`repair.go:479-490`） | `TestListPendingRepairs_TenantFilter` |
| `GET /repair/acceptance` | `ListAcceptanceRepairs` | instruments, site_members | 同上（站点成员hips） | 需 B/C 批次补路由级守卫 |
| `GET /repair/:id/records` | `ListRecords` | instruments, repair_records | 先 `id = ? AND tenant_id = ?` 校验乐器（`repair.go:418+`） | `TestRepairRecords_ExplicitScope_2276` |
| `GET /merchant/repair-requests` | `ListMerchantRepairRequests` | repair_requests | `tenant_id = 操作者 tid`（`repair_admin.go:24`） | 需补路由级守卫 |
| `GET /repair-appeals` | `ListAppeals` | appeals | `org_id = 操作者 oid`（`appeal.go:36-38`） | 需补路由级守卫 |
| `GET /config/repair` | `GetRepairAllSettings` | system_settings | 配置键 = 商户 org anchor（`repair_config.go:95`） | 已有配置隔离测试 |
| `GET /config/repair/single` | `GetRepairSetting` | system_settings | 同上（`repair_config.go:37`） | 已有配置隔离测试 |

> 批次 A 仅切只读；写路由（start/complete/accept/reject/records-add/return-shipping/transit）保持自动范围，待批次 A2。

### 已回滚（未切换，自动范围保留）

原批次挂载但**未完成逐路由清单/守卫**的入口全部回滚，包括：

- repair：`repairRequired` / `repairReqRequired` 两个路由组、`/config/repair*`、`/repair-appeals*`、`/merchant/repair-requests`、customer repair 路由；
- orders/instruments：`/orders/:id`（detail）、`/orders/:id/admin-update`、`/orders/:id/pickup`、`/outbound-*`、`/assessment`、`/instruments/:id`（detail/update）、media 写删、pricing/activity/scrap/import/export/loss/restore、`/instrument-loss`；
- membership：`/merchants*`、`/admin/merchants/*/members*`、`/merchant/sites`（Create）、`/sites/tree`、其余 site 成员写路由、technician-profiles 写路由。

> 原因：关闭自动范围会同时影响 Query/Delete；上述入口存在未显式带 `tenant_id/org_id/user_id/父对象授权` 的查询（如支付会话反查），必须先补过滤 + 路由级守卫（on/off 等价）再切换。
>
> 回滚方式：移除对应路由/组的 `middleware.ExplicitTenantScope()`；行为恢复为自动租户范围（本轮不改 handler 业务逻辑）。
