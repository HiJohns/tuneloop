# 状态机实现差距评估报告

> 本报告记录当前代码实现与 `docs/state-machine.md` 标准状态机之间的全部差距，用于指导后续子任务拆分和修复优先级排序。
>
> **关联 Issue**:
> - 标准文档审核与决议：#710
> - 代码全面差距评估：#711

---

## 1. 评估范围

| 维度 | 覆盖文件 |
|------|---------|
| 后端 Handlers | `backend/handlers/order.go`, `user_rental.go`, `warehouse.go`, `order_terminate.go`, `maintenance_session.go`, `api.go`, `public.go` |
| 后端模型 | `backend/models/models.go` |
| 后端路由 | `backend/main.go` |
| 前端 API | `frontend-pc/src/services/api.js` |
| 前端页面 | `frontend-pc/src/pages/OrderPayment.jsx`, `ReturnProcess.jsx`, `WarehouseManagement.jsx`, `InstrumentDetailUser.jsx` |
| 数据库文档 | `docs/database.md` |

---

## 2. 差距总览

| 编号 | 类别 | 严重程度 | 文件位置 | 一句话描述 |
|------|------|---------|---------|-----------|
| G001 | 死代码 / 双创建路径 | 🔴 高 | `backend/handlers/order.go` + `main.go` | 死代码 `POST /api/orders` 创建 `pending` 订单，与标准路径 `POST /api/user/orders` 并存 |
| G002 | 状态值不一致 | 🔴 高 | `backend/models/models.go:127` | Order 模型默认状态为 `pending`，标准应为 `reserved` |
| G003 | 状态门缺失 | 🔴 高 | `backend/handlers/order.go:443` | `PayOrder` 只接受 `pending`，标准应接受 `reserved` |
| G004 | 前端-后端路径不一致 | 🔴 高 | `frontend-pc/src/pages/OrderPayment.jsx:23,35,53` | 前端调用 `/user/orders/:id/*`，后端无此路由 |
| G005 | 状态门缺失 | 🔴 高 | `backend/handlers/warehouse.go:93` | `UpdateShipping` 不校验前置状态，任意状态可直接变 `shipped` |
| G006 | 状态门缺失 / 幽灵状态 | 🔴 高 | `backend/handlers/warehouse.go:374,380` | `AssessDamage` 无状态门，且设置幽灵状态 `inspecting` |
| G007 | 状态门缺失 | 🟡 中 | `backend/handlers/warehouse.go:186` | `ConfirmDelivery` 不校验 `shipped` 前置状态 |
| G008 | 状态机不完整 | 🔴 高 | `backend/handlers/order.go:653` | `CancelOrder` 仅允许 `pending` 取消，未实现 `reserved`/`paid` → `in_store` |
| G009 | 幽灵状态 | 🟡 中 | `backend/handlers/warehouse.go:122` + 测试 | 幽灵状态 `preparing` 被写入历史表，但系统从未设置该状态 |
| G010 | 幽灵状态 | 🟡 中 | `backend/handlers/warehouse.go:380,399` | 幽灵状态 `inspecting` 被 `AssessDamage` 设置，标准中不存在 |
| G011 | 状态值不一致 | 🔴 高 | `backend/handlers/warehouse.go:282` | 验收通过设置 `in_stock`，标准应为 `in_store` |
| G012 | 状态值不一致 | 🔴 高 | `backend/handlers/order_terminate.go:34` | 终止订单设置 `terminated`，标准应为 `in_store` |
| G013 | 代码质量 | 🟢 低 | 全后端 handlers | 订单状态全为裸字符串，无常量定义 |
| G014 | 租赁会话不完整 | 🟡 中 | `backend/handlers/warehouse.go:228-351` | 归还验收后未更新 `LeaseSession.Status` 为 `completed` |
| G015 | 前端-后端路径不一致 | 🔴 高 | `frontend-pc/src/services/api.js:342` | `ordersApi.create` 调用死代码路径 `/orders` |
| G016 | 前端-后端路径不一致 | 🔴 高 | `frontend-pc/src/pages/ReturnProcess.jsx:23` | 调用不存在的 `GET /user/rentals/:id` |
| G017 | 前端-后端路径不一致 | 🟡 中 | `frontend-pc/src/pages/WarehouseManagement.jsx:147` | 调用不存在的 `GET /warehouse/orders/:id` |
| G018 | 前端状态展示错误 | 🟡 中 | `frontend-pc/src/pages/WarehouseManagement.jsx:156-178` | 前端使用非标准状态值（`preparing`/`delivered` 等） |
| G019 | 数据隔离缺失 | 🔴 高 | `backend/handlers/order.go:359,435,486` | `GetOrder` / `PayOrder` / `PickupOrder` 未校验 `tenant_id` |
| G020 | 数据隔离缺失 | 🔴 高 | `backend/handlers/warehouse.go:38-56` | `ListOrders` 未按 `tenant_id` 过滤，仅按 `org_id` |
| G021 | Schema 差距 | 🔴 高 | `backend/models/models.go:114` + `warehouse.go:46` | `orders` 表无 `site_id` 字段，但 `ListOrders` 按 `site_id` 过滤 |
| G022 | Schema 差距 | 🔴 高 | `backend/models/models.go` | `forwarding_sessions` 表完全未定义 |
| G023 | 功能缺失 | 🟡 中 | — | 自动确认收货（物流签收 + 48h）机制完全缺失 |
| G024 | 状态机不完整 | 🟡 中 | — | `maintenance → in_store`（维修完成）handler 缺失 |
| G025 | 前端响应处理错误 | 🟡 中 | `frontend-pc/src/pages/InstrumentDetailUser.jsx:24` | 未解包 `response.data`，直接 `setInstrument(data)` |
| G026 | 文档/Schema 差距 | 🟢 低 | `docs/database.md:45-66` | `merchants` 表未记录 #706 转发地址字段 |
| G027 | 模型/Schema 不一致 | 🔴 高 | `backend/models/models.go:445` + `docs/database.md:461-478` | `DamageAssessment` 模型与数据库文档 schema 不一致 |
| G028 | 数据隔离缺失 | 🟡 中 | `backend/handlers/public.go:237-255` | `GetPublicCategories` 未按 `tenant_id` 过滤 |

---

## 3. 按严重程度分组

### 🔴 高风险（15 项）— 阻塞/数据错误/安全漏洞

> 这些差距会导致状态机失效、数据不一致、横向越权或功能完全不可用。**应优先处理。**

| 编号 | 描述 | 标准依据 | 实际代码 | 影响 |
|------|------|---------|---------|------|
| G001 | 死代码 `POST /api/orders` | §1.1 唯一路径为 `POST /api/user/orders` | `order.go:111-287` 创建 `pending` 订单；`main.go:242` 注册路由；`api.js:342` 前端调用 | 双轨状态机，零 UUID 破坏租户隔离 |
| G002 | Order 默认状态 `pending` | §1.1 起始状态为 `reserved` | `models.go:127` `default:"pending"` | 绕过标准路径的订单进入错误状态 |
| G003 | `PayOrder` 不接受 `reserved` | §1.3 `reserved → paid` | `order.go:443` `if order.Status != "pending"` | 标准路径订单无法支付 |
| G004 | `OrderPayment.jsx` 路径错误 | §6 #3 前端路径一致性 | 调用 `/user/orders/:id/*`（后端无此路由） | 支付页面 404 |
| G005 | `UpdateShipping` 无状态门 | §1.3 `paid → shipped` | `warehouse.go:93` 直接 Updates | 任意状态可发货 |
| G006 | `AssessDamage` 无状态门 + `inspecting` | §1.3 `returning → maintenance`；标准无 `inspecting` | `warehouse.go:354-434` 无校验，设置 `inspecting` | 非 returning 订单可被定损；引入幽灵状态 |
| G008 | `CancelOrder` 仅允许 `pending` | §1.3 `reserved/paid → in_store` | `order.go:653` 仅 `status == "pending"` | `reserved`/`paid` 无法取消 |
| G011 | 验收通过用 `in_stock` | §1.1 终端状态为 `in_store` | `warehouse.go:282` `newStatus := "in_stock"` | 状态枚举不一致 |
| G012 | 终止用 `terminated` | §1.3 终止统一为 `in_store` | `order_terminate.go:34` `order.Status = "terminated"` | 引入非标准终态 |
| G015 | `ordersApi.create` 调用死代码 | §6 #1 仅保留 `/user/orders` | `api.js:342` `api.post('/orders', data)` | 前端走错误创建路径 |
| G016 | `ReturnProcess.jsx` 路径错误 | 路径一致性 | `ReturnProcess.jsx:23` 调用不存在路由 | 归还页面 404 |
| G019 | 订单查询无 `tenant_id` | `AGENTS.md` §维度 4 | `order.go:359,435,486` 无隔离 | 横向越权 |
| G020 | `ListOrders` 无 `tenant_id` | `AGENTS.md` §维度 4 | `warehouse.go:38-56` 无隔离 | 跨租户数据泄漏 |
| G021 | `orders` 无 `site_id` 但按之过滤 | `database.md` §2.7 | `models.go` 无 `SiteID`；`warehouse.go:46` 按 `site_id` 过滤 | 筛选功能失效 |
| G022 | `forwarding_sessions` 未定义 | §5.4 | 无模型、无迁移、无 handler | 受控商户功能完全缺失 |
| G027 | `DamageAssessment` 模型不一致 | `database.md` §2.17 | `models.go:445-456` 缺少多个字段 | 运行时错误风险 |

### 🟡 中风险（9 项）— 功能缺失/体验问题

> 不会立即导致系统崩溃，但会造成功能不完整或用户体验问题。

| 编号 | 描述 | 标准依据 | 实际代码 | 影响 |
|------|------|---------|---------|------|
| G007 | `ConfirmDelivery` 无状态门 | §1.3 `shipped → in_lease` | `warehouse.go:186` 无校验 | 任意状态可确认收货 |
| G009 | 幽灵状态 `preparing` | 标准无此状态 | `warehouse.go:122` 历史表使用 | 审计混乱 |
| G010 | 幽灵状态 `inspecting` | 标准无此状态 | `warehouse.go:380,399` 多处设置 | 状态语义分裂 |
| G014 | `LeaseSession` 未更新 | §3.2 验收后应为 `completed` | `warehouse.go:228-351` 未触及 | 用户端租赁列表不一致 |
| G017 | `WarehouseManagement.jsx` 路径错误 | 路径一致性 | `WarehouseManagement.jsx:147` 调用不存在路由 | 库管查看详情 404 |
| G018 | 前端使用非标准状态值 | §1.1 | 使用 `preparing`/`delivered`/`return_requested` 等 | 操作按钮显示错乱 |
| G023 | 自动确认收货缺失 | `cases.md` §2.3 | 无任何定时任务 | 订单永远卡在 `shipped` |
| G024 | `maintenance → in_store` 缺失 | §1.3 | 维修验收后未更新 Order/Instrument | 维修完无法回库 |
| G025 | 前端未解包响应体 | `AGENTS.md` §维度 3 | `InstrumentDetailUser.jsx:24` | 详情页数据错位 |
| G028 | 公开接口未隔离 | `cases.md` §1.3 | `public.go:237-255` 无 `tenant_id` | 返回全系统分类 |

### 🟢 低风险（4 项）— 代码质量/文档同步

> 不影响当前功能，但会降低可维护性或造成文档不一致。

| 编号 | 描述 | 标准依据 | 实际代码 | 影响 |
|------|------|---------|---------|------|
| G013 | 状态裸字符串 | 代码规范 | 全后端 handlers | 易拼写错误，重构困难 |
| G026 | `database.md` 未同步 #706 | `database.md` §2.2 | 缺少 4 个新字段 | 文档与代码不同步 |

---

## 4. 建议的子任务分组

以下分组基于**依赖关系**和**修复范围**划分，每组可独立作为一个子 Issue：

### 子任务 A：订单创建路径清理（G001, G002, G003, G015）
- 删除 `backend/handlers/order.go` 死代码 `CreateOrder`/`PreviewOrder`
- `main.go` 注销 `POST /api/orders`
- `api.js` `ordersApi.create` 改为 `/user/orders`
- `models.go` Order.Status 默认值改为 `reserved`
- `PayOrder` 接受 `reserved`（兼容 `pending`）
- **预估工作量**：小

### 子任务 B：前端路径修复（G004, G016, G017, G025）
- `OrderPayment.jsx` 路径改为 `/orders/:id` 系列
- `ReturnProcess.jsx` 修复 API 路径（或后端新增路由）
- `WarehouseManagement.jsx` 修复 API 路径（或复用现有路由）
- `InstrumentDetailUser.jsx` 解包 `response.data`
- **预估工作量**：小

### 子任务 C：状态门与幽灵状态（G005, G006, G007, G008, G009, G010, G011, G012）
- `UpdateShipping` 增加 `AND status = 'paid'` 校验
- `ConfirmDelivery` 增加 `AND status = 'shipped'` 校验
- `AssessDamage` 增加 `AND status = 'returning'`，目标状态改为 `maintenance`
- `CancelOrder` 接受 `reserved`/`paid`，取消后统一为 `in_store`
- 删除所有 `preparing`、`inspecting` 引用
- 验收通过改为 `in_store`，终止改为 `in_store`
- 同步更新测试代码
- **预估工作量**：中

### 子任务 D：状态常量化（G013）
- `models.go` 中定义 `OrderStatusReserved = "reserved"` 等常量
- 全后端 handlers 替换裸字符串
- **预估工作量**：中（涉及面广但改动机械）

### 子任务 E：数据隔离修复（G019, G020, G021, G028）
- `GetOrder`/`PayOrder`/`PickupOrder` 增加 `tenant_id` + `user_id` 过滤
- `ListOrders` 增加 `tenant_id` 基线过滤
- `Order` 模型增加 `SiteID` 字段（或改用 JOIN）
- `GetPublicCategories` 增加 `tenant_id` 过滤
- **预估工作量**：中

### 子任务 F：LeaseSession 与维修闭环（G014, G024）
- `InspectReturn` 中更新 `LeaseSession.Status = "completed"`
- 维修验收 handler 中更新 `Order.Status = "in_store"` + `Instrument.StockStatus = "available"`
- **预估工作量**：小

### 子任务 G：前端状态值对齐（G018）
- `WarehouseManagement.jsx` 状态配置按 `state-machine.md` §1.1 重写
- 删除 `preparing`/`delivered`/`return_requested` 等非标准状态
- **预估工作量**：小

### 子任务 H：Schema 与文档同步（G026, G027）
- `database.md` 补充 `merchants` 表 #706 字段
- `DamageAssessment` 模型按 `database.md` §2.17 重构
- `InspectReturn` 补充 `instrument_id`, `user_id`, `condition`, `photos`, `scan_time`
- **预估工作量**：小

### 子任务 I：自动确认收货定时任务（G023）
- 新增 `internal/tasks/auto_confirm.go` 定时任务
- 扫描 `status = 'shipped' AND shipped_at < NOW() - INTERVAL '48 hours'`
- 自动更新为 `in_lease`，记录历史
- **预估工作量**：小

### 子任务 J：forwarding_sessions 完整实现（G022 + #705 Phase 2）
- 按 `state-machine.md` §5.4 创建模型和迁移
- 在 `user_rental.go` `CreateOrder` 和 `ReturnRental` 中自动创建记录
- 新增 `GET /api/forwarding/sessions` 等管理接口
- 实现 `session_code` 生成与查询
- 转发网点员工 UI + 照片上传
- **预估工作量**：大

### 子任务 K：包裹丢失与报废机制（#710 决议 7, 9）
- `forwarding_session.status` 增加 `lost`
- `instrument.stock_status` 增加 `lost`
- Alert 机制（物流超时、丢失标记、7 天未处理升级）
- 库存工作台 `scrap` 功能（仅管理员，仅 `in_store`/`maintenance`）
- **预估工作量**：中

---

## 5. 修复优先级建议

| 优先级 | 子任务 | 理由 |
|--------|--------|------|
| **P0（立即）** | A + B + C | 订单创建和支付是核心路径，当前支付功能 404/无法使用 |
| **P1（本周）** | E + F | 数据隔离是安全红线；LeaseSession 完整性影响用户端数据 |
| **P2（下周）** | D + G + H + I | 代码质量、前端对齐、文档同步、自动确认 |
| **P3（后续迭代）** | J + K | forwarding_sessions 和丢失/报废机制是 #705 Phase 2 内容 |

---

## 6. 附录：#710 确认的关键决议回顾

| 决议 | 内容 | 相关差距 |
|------|------|---------|
| 1 | `active` 从 `in_lease` 起算；增加物流签收 48h 自动确认 | G023 |
| 2 | 命名空间管理员不参与日常业务，转发会话不可见 | —（设计决策） |
| 3 | `forwarding_sessions` 增加 `tracking_numbers jsonb` | G022 |
| 4 | 补充 `returning` 可见性矩阵 | —（文档更新） |
| 5 | 物流单统一标注 `session_code`（6位短码） | G022 |
| 6 | 照片复用 `instrument_photo_batches`，扩展 `batch_type` | G022 |
| 7 | 报废（`scrap`）作为库存工作台功能，仅管理员 | 子任务 K |
| 8 | 两段物流状态：`pending → in_transit → received → ready → last_mile → delivered → completed`；`last_mile` 前可取消 | G022 |
| 9 | 包裹丢失：`lost` 状态 + Alert + 找回/报废双路径 | 子任务 K |

---

## 7. 端到端测试计划

> 本章节定义验证状态机修复的端到端测试策略。测试应**在修复代码的同时编写**——先写失败的测试（验证当前 bug），再修复代码使其通过。

### 7.1 绕过 IAM 的策略

**结论：不复用 JWT，采用 Context 注入。**

现有 `backend/handlers/integration_test.go` 已使用纯 Context 注入绕过 IAM：

```go
router.Use(func(c *gin.Context) {
    ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
    ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
    ctx = context.WithValue(ctx, middleware.ContextKeyRole, "STAFF")
    ctx = context.WithValue(ctx, middleware.ContextKeySysPerm, sysPerm)
    ctx = context.WithValue(ctx, middleware.ContextKeyCusPerm, cusPerm)
    c.Request = c.Request.WithContext(ctx)
    c.Next()
})
```

| 方案 | 优点 | 缺点 | 结论 |
|------|------|------|------|
| **Context 注入** | 零外部依赖、速度快、可控性强 | 不经过 `IAMInterceptor` issuer 校验 | ✅ **采用** |
| HS256 手搓 JWT | 经过完整中间件链路 | 需初始化 `IAMService`、配置 `clientSecret` | ❌ 不采用 |

端到端测试的重点是**状态机流转正确性**，而非 IAM 验证本身。Context 注入已足够。

### 7.2 测试基础设施

#### 角色工厂（建议新增 `backend/testutil/role_factory.go`）

```go
type TestActor struct {
    UserID   string
    TenantID string
    OrgID    string
    SiteID   string
    Role     string        // "ADMIN" / "STAFF" / "WORKER" / "USER"(customer)
    SysPerm  int64
    CusPerm  int64
}

func MakeCustomer(tenantID, userID string) TestActor
func MakeSiteMember(tenantID, orgID, siteID, userID string) TestActor
func MakeForwardingSiteMember(tenantID, orgID, siteID, userID string) TestActor
func MakeSiteAdmin(tenantID, orgID, siteID, userID string) TestActor
```

#### 状态断言辅助函数（建议新增 `backend/testutil/state_assert.go`）

```go
// 一次性断言订单、乐器、租赁会话、转发会话的状态
func AssertState(t *testing.T, db *gorm.DB, orderID string,
    expected struct {
        OrderStatus              string
        InstrumentStatus         string
        LeaseSessionStatus       string
        ForwardingSessionStatus  *string // nil if not applicable
    })

// 断言状态历史记录存在且顺序正确
func AssertStateHistory(t *testing.T, db *gorm.DB, orderID string,
    expectedTransitions []struct{ From, To string })
```

### 7.3 核心端到端测试场景

#### 场景 A：标准商户完整租赁闭环（顾客 + 网点员工）

| 步骤 | 操作者 | 动作 | 断言点 |
|------|--------|------|--------|
| A1 | 顾客 | 浏览乐器列表 | 乐器状态 `available` |
| A2 | 顾客 | 下单租赁 | Order=`reserved`, Instrument=`reserved`, LeaseSession=`active` |
| A3 | 顾客 | 支付 | Order=`paid` |
| A4 | 网点员工 | 填写物流发货 | Order=`shipped`, Instrument=`shipping` |
| A5 | 顾客 | 确认收货 | Order=`in_lease`, Instrument=`rented` |
| A6 | 顾客 | 发起归还 | Order=`returning`, Instrument=`returning` |
| A7 | 网点员工 | 验收通过 | Order=`in_store`, Instrument=`available`, LeaseSession=`completed` |
| A8 | — | 验证状态历史 | 记录 `shipped→in_lease`, `in_lease→returning`, `returning→in_store` |

#### 场景 B：受控商户转发流程（顾客 + 受控商户员工 + 转发网点员工）

| 步骤 | 操作者 | 动作 | 断言点 |
|------|--------|------|--------|
| B1 | 顾客 | 下单受控商户乐器 | 自动创建 forwarding_session: direction=`outbound`, status=`pending`, session_code 存在 |
| B2 | 受控商户员工 | 发货（填写物流） | Order=`shipped`, forwarding_session=`in_transit` |
| B3 | 转发网点员工 | 收货 + 拆封拍照 | forwarding_session=`received`, photo_batch 创建 |
| B4 | 转发网点员工 | 重新打包 + last_mile 发货 | forwarding_session=`last_mile` |
| B5 | 顾客 | 确认收货 | Order=`in_lease`, forwarding_session=`completed` |
| B6 | 顾客 | 发起归还 | 自动创建 forwarding_session: direction=`return`, status=`pending` |
| B7 | 顾客 | 填写归还物流 | forwarding_session=`in_transit` |
| B8 | 转发网点员工 | 收货 + 拆封拍照 | forwarding_session=`received` |
| B9 | 转发网点员工 | 重新打包发往商户 | forwarding_session=`last_mile` |
| B10 | 受控商户员工 | 收货验收 | Order=`in_store`, forwarding_session=`completed` |

#### 场景 C：取消边界测试

| 步骤 | 操作者 | 动作 | 预期结果 |
|------|--------|------|---------|
| C1 | 顾客 | reserved 状态取消 | 成功，Order=`in_store`, Instrument=`available` |
| C2 | 顾客 | paid 状态取消 | 成功，Order=`in_store`, Instrument=`available` |
| C3 | 顾客 | shipped 状态取消 | **失败**（403 或 400） |
| C4 | 顾客 | last_mile 后取消 | **失败** |
| C5 | 网点员工 | 尝试从 available 发货 | **失败**（状态门拦截） |

#### 场景 D：包裹丢失与报废

| 步骤 | 操作者 | 动作 | 断言点 |
|------|--------|------|--------|
| D1 | 系统 | 模拟物流超时（>48h 无更新） | forwarding_session=`lost`, Alert 记录生成 |
| D2 | 管理员 | 标记找回 | forwarding_session 恢复上一状态 |
| D3 | 管理员 | 确认丢失 → scrap | Instrument=`archived` 或新状态 `scrapped` |
| D4 | 管理员 | 尝试 scrap available 乐器 | **失败**（仅 in_store/maintenance 可 scrap） |

#### 场景 E：数据隔离

| 步骤 | 操作者 | 动作 | 预期结果 |
|------|--------|------|---------|
| E1 | 网点A员工 | 查看网点B订单 | 空列表（无越权） |
| E2 | 顾客A | 查看顾客B订单 | 404 或空（无越权） |
| E3 | 受控商户员工 | 查看真实用户信息 | 隐藏（仅转发网点地址） |

### 7.4 与现有测试的衔接

现有 `backend/handlers/integration_test.go` 已有 4 个场景：

| 现有场景 | 状态 | 处理方式 |
|---------|------|---------|
| Scenario1: 租赁闭环 | ✅ 保留 | 修正 `preparing` 幽灵状态，扩展断言 |
| Scenario2: 库管流程 | ⚠️ 需修正 | `Status: "preparing"` → 改为 `reserved` 或 `paid` |
| Scenario3: 维修流程 | ✅ 保留 | 无状态冲突，补充 LeaseSession 状态断言 |
| Scenario4: 申诉流程 | ✅ 保留 | 无状态冲突 |

**新增测试文件建议**：

| 文件 | 覆盖内容 |
|------|---------|
| `backend/handlers/state_machine_e2e_test.go` | 场景 A、C（标准商户完整闭环 + 取消边界） |
| `backend/handlers/forwarding_session_e2e_test.go` | 场景 B（受控商户转发流程） |
| `backend/handlers/exception_e2e_test.go` | 场景 D、E（丢失/报废/隔离） |

### 7.5 测试执行顺序建议

```
1. 子任务 A（订单创建路径清理）
   └── 先写 state_machine_e2e_test.go 中 Scenario A 的 A1-A3
   └── 修复 G001/G002/G003 → 测试通过

2. 子任务 C（状态门与幽灵状态）
   └── 先写 Scenario C（取消边界 + 状态门）
   └── 修复 G005/G006/G007/G008/G011/G012 → 测试通过

3. 子任务 J（forwarding_sessions）
   └── 先写 forwarding_session_e2e_test.go 中 Scenario B
   └── 实现 forwarding_sessions 表 + 触发逻辑 → 测试通过

4. 子任务 K（丢失与报废）
   └── 先写 exception_e2e_test.go 中 Scenario D
   └── 实现 lost/scrap 机制 → 测试通过
```

---

*文档生成日期：2026-06-02*  
*基于：docs/state-machine.md @ main（#710 accepted）、backend 代码审计（#711）*
