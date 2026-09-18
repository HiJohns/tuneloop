# Analysis & Implementation Plan - Issue #688

## 📋 Task Overview

**Issue**: #688 — 数据隔离系统性漏洞：site_admin 可跨网点访问其他网点数据  
**Status**: Analysis Complete, Pending Implementation  
**Severity**: Critical (Security)  
**Scope**: Backend — All CRUD Handlers (137 handlers audited, 44+ isolation defects found)  
**Assigned To**: Junior Developer (code only, no architecture changes)  
**Review Required**: Senior/Architect review before merge  

---

## 🎯 初级程序员执行手册

> **重要原则**：本 Issue 只改代码，不改架构。所有设计决策已在本文档中确定，请严格按照步骤执行，不要自行决定"这样更好"的改法。
>
> **红线**：
> - 不要修改 `middleware/iam.go` 中的 `GetBusinessRole`、`ApplyOrgScope`、`GetVisibleOrgIDs` 等函数
> - 不要修改 `database/db.go` 中的回调注册逻辑
> - 不要修改 `models/*.go` 中的数据结构
> - 不要新增路由或修改路由注册逻辑（`main.go` 中的 `setupAPIRoutes`）
> - 不要修改前端代码

---

## 执行总览

| Phase | 内容 | 预计文件数 | 预计工时 | 编译必过 | 测试必过 |
|-------|------|-----------|---------|---------|---------|
| Phase 1 | `c.GetString` → `middleware.GetXxx` 全局替换 | 7 个 | 2h | ✅ | ✅ |
| Phase 2 | 给 Update 操作补 `tenant_id` WHERE + 修复 401 端点 | 2 个 | 1h | ✅ | ✅ |
| Phase 3 | 预初始化 handler 添加 `.WithContext(ctx)` | 3 个 | 1h | ✅ | ✅ |
| Phase 4 | 剩余裸 DB 调用添加 `.WithContext(ctx)` | 5 个 | 2h | ✅ | ✅ |
| Phase 5 | `ApplyOrgScope` 推广到关键 handler | 4 个 | 2h | ✅ | ✅ |
| Phase 6 | 验证测试 + 修复回归 | - | 2h | ✅ | ✅ |

**总计**：约 10 小时，6 个 Phase，必须按顺序执行。

---

## Phase 1: `c.GetString` → `middleware.GetXxx` 全局替换（P0）

### 1.1 修改 `handlers/label.go`

**修改范围**：全部 5 处 `c.GetString("tenant_id")`

**步骤**：
1. 确认文件顶部的 import 包含 `"tuneloop-backend/middleware"`
2. 将以下 5 处代码按表格替换

| 行号 | 修改前 | 修改后 |
|------|--------|--------|
| 33 | `query := h.db.Table("labels").Where("tenant_id = ?", c.GetString("tenant_id"))` | `tenantID := middleware.GetTenantID(c.Request.Context())` 然后 `query := h.db.WithContext(c.Request.Context()).Table("labels").Where("tenant_id = ?", tenantID)` |
| 68 | `"tenant_id": c.GetString("tenant_id"),` | `"tenant_id": middleware.GetTenantID(c.Request.Context()),` |
| 94 | `c.GetString("tenant_id")` | `middleware.GetTenantID(c.Request.Context())` |
| 112 | `c.GetString("tenant_id")` | `middleware.GetTenantID(c.Request.Context())` |
| 141 | `tenantID := c.GetString("tenant_id")` | `tenantID := middleware.GetTenantID(c.Request.Context())` |

**具体代码（GetLabels 方法为例）**：

```go
// BEFORE (line 22-38)
func (h *LabelHandler) GetLabels(c *gin.Context) {
    status := c.DefaultQuery("status", "")
    var labels []struct { ... }
    query := h.db.Table("labels").Where("tenant_id = ?", c.GetString("tenant_id"))
    // ...
}

// AFTER
func (h *LabelHandler) GetLabels(c *gin.Context) {
    status := c.DefaultQuery("status", "")
    ctx := c.Request.Context()
    tenantID := middleware.GetTenantID(ctx)
    var labels []struct { ... }
    query := h.db.WithContext(ctx).Table("labels").Where("tenant_id = ?", tenantID)
    // ...
}
```

**注意**：`MergeLabels` 方法（line 127+）中，替换后还需要把 `h.db` 改为 `h.db.WithContext(c.Request.Context())`。

### 1.2 修改 `handlers/lease.go`

**修改范围**：全部 `c.GetString("tenant_id")`（6 处，排除 `ListLeases` 已正确使用）

| 行号 | 方法 | 修改前 | 修改后 |
|------|------|--------|--------|
| 84 | GetLease | `tenantID := c.GetString("tenant_id")` | `tenantID := middleware.GetTenantID(c.Request.Context())` |
| 116 | CreateLease | `tenantID := c.GetString("tenant_id")` | `tenantID := middleware.GetTenantID(c.Request.Context())` |
| 164 | UpdateLease | `tenantID := c.GetString("tenant_id")` | `tenantID := middleware.GetTenantID(c.Request.Context())` |
| 219 | TerminateLease | `tenantID := c.GetString("tenant_id")` | `tenantID := middleware.GetTenantID(c.Request.Context())` |
| 327 | RenewLease | `tenantID := c.GetString("tenant_id")` | `tenantID := middleware.GetTenantID(c.Request.Context())` |
| 372 | GetLeasePayments | `tenantID := c.GetString("tenant_id")` | `tenantID := middleware.GetTenantID(c.Request.Context())` |

**import 检查**：确认已有 `tuneloop-backend/middleware`。

### 1.3 修改 `handlers/maintenance.go`

**修改范围**：`tenant_id`（5 处）、`org_id`（2 处）、`user_id`（2 处）

| 行号 | 修改前 | 修改后 |
|------|--------|--------|
| 42 | `tenantID := c.GetString("tenant_id")` | `tenantID := middleware.GetTenantID(c.Request.Context())` |
| 43 | `orgID := c.GetString("org_id")` | `orgID := middleware.GetOrgID(c.Request.Context())` |
| 92 | `userID := c.GetString("user_id")` | `userID := middleware.GetUserID(c.Request.Context())` |
| 106 | `tenantID := c.GetString("tenant_id")` | `tenantID := middleware.GetTenantID(c.Request.Context())` |
| 107 | `orgID := c.GetString("org_id")` | `orgID := middleware.GetOrgID(c.Request.Context())` |
| 365 | `userID := c.GetString("user_id")` | `userID := middleware.GetUserID(c.Request.Context())` |

**注意**：`maintenance.go` 中使用 `database.GetDB().WithContext(c.Request.Context())` 的地方不需要改（已经是正确的），只需要替换 `c.GetString` 的调用。

### 1.4 修改 `handlers/admin.go`

**修改范围**：2 处 `c.GetString("tenant_id")`

| 行号 | 修改前 | 修改后 |
|------|--------|--------|
| 21 | `tenantID := c.GetString("tenant_id")` | `tenantID := middleware.GetTenantID(c.Request.Context())` |
| 72 | `tenantID := c.GetString("tenant_id")` | `tenantID := middleware.GetTenantID(c.Request.Context())` |

**额外修改**：这 2 个方法使用 `h.db`（预初始化的裸 DB）。在获取 tenantID 后，把 `h.db` 替换为 `h.db.WithContext(c.Request.Context())`。

```go
// BEFORE (GetDashboardStats)
tenantID := c.GetString("tenant_id")
// ...
if err := h.db.Model(&models.Instrument{}).Where("tenant_id = ?", tenantID)...

// AFTER
ctx := c.Request.Context()
tenantID := middleware.GetTenantID(ctx)
// ...
if err := h.db.WithContext(ctx).Model(&models.Instrument{}).Where("tenant_id = ?", tenantID)...
```

### 1.5 修改 `handlers/auth.go`

**修改范围**：1 处（低风险，但保持一致性）

| 行号 | 修改前 | 修改后 |
|------|--------|--------|
| 235 | `c.GetString("tenant_id")` | `middleware.GetTenantID(c.Request.Context())` |

### 1.6 修改 `handlers/order.go`

**修改范围**：1 处 `c.GetString("user_id")`

| 行号 | 修改前 | 修改后 |
|------|--------|--------|
| 68 | `if uID := c.GetString("user_id"); uID != "" {` | `if uID := middleware.GetUserID(c.Request.Context()); uID != "" {` |

### 1.7 修改 `handlers/instrument_photo.go`

**修改范围**：2 处 `c.GetString("user_id")`

| 行号 | 修改前 | 修改后 |
|------|--------|--------|
| 187 | `"operator_id": c.GetString("user_id"),` | `"operator_id": middleware.GetUserID(c.Request.Context()),` |
| 290 | `OperatorID: c.GetString("user_id"),` | `OperatorID: middleware.GetUserID(c.Request.Context()),` |

### Phase 1 编译检查

```bash
cd /home/coder/tuneloop/backend
go build ./handlers/
```

如果编译失败，最常见的原因：
1. **"middleware undefined"** → 确认文件顶部 import 了 `"tuneloop-backend/middleware"`
2. **"c.Request.Context undefined"** → gin.Context 确实有这个方法，检查拼写
3. **"too many arguments"** → `middleware.GetTenantID` 只接受 1 个参数（ctx），不要传 `c`

### Phase 1 完成后验证

```bash
# 确认没有残留的 c.GetString("tenant_id")（应该只剩 test 文件）
grep -n 'c\.GetString("tenant_id")' handlers/*.go | grep -v '_test.go'

# 确认没有残留的 c.GetString("org_id")
grep -n 'c\.GetString("org_id")' handlers/*.go | grep -v '_test.go'

# 确认没有残留的 c.GetString("user_id")
grep -n 'c\.GetString("user_id")' handlers/*.go | grep -v '_test.go'
```

以上三个命令应该**没有任何输出**（除了可能存在的 test 文件）。如果有输出，继续替换。

---

## Phase 2: 修复跨租户 Update 漏洞 + 401 端点（P0）

### 2.1 修复 `PUT /api/instruments/:id/status`（instrument.go:665）

**修改前**：
```go
// line 662-667
db := database.GetDB().WithContext(c.Request.Context())

// Update the instrument
result := db.Model(&models.Instrument{}).
    Where("id = ?", instrumentID).
    Update("stock_status", req.StockStatus)
```

**修改后**：
```go
// line 662-667
db := database.GetDB().WithContext(c.Request.Context())
tenantID := middleware.GetTenantID(c.Request.Context())

// Update the instrument (scoped to tenant)
result := db.Model(&models.Instrument{}).
    Where("id = ? AND tenant_id = ?", instrumentID, tenantID).
    Update("stock_status", req.StockStatus)
```

**验证逻辑**：这样修改后，即使 attacker 知道其他 tenant 的乐器 ID，也无法修改，因为 WHERE 条件包含 tenant_id 过滤。

### 2.2 修复 `PUT /api/maintenance/tickets/:id/status`（maintenance.go:401）

**修改前**：
```go
// line 397-402
if req.Status == models.TicketStatusCompleted {
    now := time.Now()
    updates["completed_at"] = now

    if err := db.Model(&models.Instrument{}).Where("id = ?", ticket.InstrumentID).Update("stock_status", "available").Error; err != nil {
    }
}
```

**修改后**：
```go
// line 397-402
if req.Status == models.TicketStatusCompleted {
    now := time.Now()
    updates["completed_at"] = now

    tenantID := middleware.GetTenantID(c.Request.Context())
    if err := db.Model(&models.Instrument{}).Where("id = ? AND tenant_id = ?", ticket.InstrumentID, tenantID).Update("stock_status", "available").Error; err != nil {
        log.Printf("[UpdateTicketStatus] Failed to update instrument status: %v", err)
    }
}
```

**注意**：这里加了一个 `log.Printf`，因为原代码的空 block（`if err != nil { }`）是 bug，错误被静默吞掉了。如果不需要这个 log，至少要在 comment 中说明为什么忽略错误。

### 2.3 验证 401 端点已修复

Phase 1 中已经替换了 `maintenance.go:92` 和 `maintenance.go:365` 的 `c.GetString("user_id")` 为 `middleware.GetUserID(c.Request.Context())`。确认以下两个端点不再永远返回 401：

- `POST /api/maintenance/report`
- `PUT /api/maintenance/tickets/:id/status`

### Phase 2 编译检查

```bash
cd /home/coder/tuneloop/backend
go build ./handlers/
```

---

## Phase 3: 预初始化 Handler 添加 `.WithContext(ctx)`（P1）

### 3.1 `LabelHandler`（label.go）

这个 handler 在 `main.go:346` 通过 `handlers.NewLabelHandler(database.GetDB())` 初始化，`h.db` 是裸 DB。

**修改策略**：在每个方法开头，将 `h.db` 替换为 `h.db.WithContext(c.Request.Context())`。

**修改清单**（每个方法第一行使用 `h.db` 的地方）：

| 方法 | 修改前 | 修改后 |
|------|--------|--------|
| GetLabels | `query := h.db.Table("labels")...` | `query := h.db.WithContext(c.Request.Context()).Table("labels")...` |
| CreateLabel | `if err := h.db.Table("labels").Create...` | `if err := h.db.WithContext(c.Request.Context()).Table("labels").Create...` |
| ApproveLabel | `if err := h.db.Table("labels").Where...` | `if err := h.db.WithContext(c.Request.Context()).Table("labels").Where...` |
| RejectLabel | `if err := h.db.Table("labels").Where...` | `if err := h.db.WithContext(c.Request.Context()).Table("labels").Where...` |
| MergeLabels | 多处 `h.db.Table("labels")` | 全部改为 `h.db.WithContext(c.Request.Context()).Table("labels")` |

**技巧**：可以在每个方法开头定义 `db := h.db.WithContext(c.Request.Context())`，然后用 `db` 替换所有 `h.db`。

```go
// 以 GetLabels 为例，修改后结构：
func (h *LabelHandler) GetLabels(c *gin.Context) {
    ctx := c.Request.Context()
    tenantID := middleware.GetTenantID(ctx)
    db := h.db.WithContext(ctx)
    
    query := db.Table("labels").Where("tenant_id = ?", tenantID)
    // ... 后面所有 h.db 都改为 db
}
```

### 3.2 `DashboardHandler`（admin.go）

同样修改 2 个方法：

```go
// GetDashboardStats
func (h *DashboardHandler) GetDashboardStats(c *gin.Context) {
    ctx := c.Request.Context()
    tenantID := middleware.GetTenantID(ctx)
    db := h.db.WithContext(ctx)
    // ... 后面所有 h.db 改为 db
}

// GetNearTransfers
func (h *DashboardHandler) GetNearTransfers(c *gin.Context) {
    ctx := c.Request.Context()
    tenantID := middleware.GetTenantID(ctx)
    db := h.db.WithContext(ctx)
    // ... 后面所有 h.db 改为 db
}
```

### 3.3 `LeaseHandler`（lease.go）

`ListLeases` 已经使用了 `middleware.GetTenantID(c.Request.Context())` 并且显式加了 WHERE，但使用的是 `h.db`（裸 DB）。需要确认 `h.db` 在方法内被 `.WithContext(ctx)` 包裹。

**实际检查**：读取 lease.go 确认 `ListLeases` 如何使用 `h.db`。

如果 `ListLeases` 是：
```go
func (h *LeaseHandler) ListLeases(c *gin.Context) {
    tenantID := middleware.GetTenantID(c.Request.Context())
    // ...
    h.db.Where("tenant_id = ?", tenantID)...
}
```

则修改为：
```go
func (h *LeaseHandler) ListLeases(c *gin.Context) {
    ctx := c.Request.Context()
    tenantID := middleware.GetTenantID(ctx)
    db := h.db.WithContext(ctx)
    // ...
    db.Where("tenant_id = ?", tenantID)...
}
```

其余 `CreateLease`、`UpdateLease`、`TerminateLease`、`RenewLease`、`GetLeasePayments`、`GetLease` 同样处理。

### Phase 3 编译检查

```bash
cd /home/coder/tuneloop/backend
go build ./handlers/
go build .  # 编译 main.go，确认 handler 初始化没有类型错误
```

---

## Phase 4: 剩余裸 DB 调用添加 `.WithContext(ctx)`（P1）

### 4.1 需要修改的文件清单

以下文件存在 `database.GetDB()`（无 `.WithContext(ctx)`）的调用，需要逐一检查并修改：

| 文件 | 裸 DB 调用数 | 修改策略 |
|------|-------------|---------|
| `api.go` | 9 | 全部改为 `database.GetDB().WithContext(ctx)` |
| `instrument.go` | 4 | 全部改为 `database.GetDB().WithContext(ctx)` |
| `public.go` | 5 | 公共路由，无认证 context，**保持原样**（见下方说明） |
| `system.go` | 2 | `GetClients` 改为 WithContext；`GetTenants` 需确认是否加 tenant 过滤 |
| `notification.go` | 1 | 改为 WithContext，并加 `tenant_id` 过滤 |
| `confirmation_session.go` | 3 | 回调路由无标准认证，**保持原样** |
| `inventory.go` | 1 | 移除 `context.Background()` 显式绕过，改为 request context |
| `iam_proxy.go` | 1 | `SearchUsers` 加 tenant 过滤（见 Phase 5） |

### 4.2 `api.go` 的 9 处修改

逐个检查以下方法：

1. `GetInstrumentByID`（line 26）
2. `GetInstruments`（line 145）
3. `GetCategories`（line 319）
4. `GetCategoryChildren`（line 382）
5. `GetCategoryByID`（line 434）
6. `CreateCategory`（line 463）
7. `UpdateCategory`（line 535）
8. `DeleteCategory`（line 615）
9. `UpdateCategorySort`（line 658）

**统一修改模式**：

```go
// BEFORE
db := database.GetDB()
ctx := c.Request.Context()
tenantID := middleware.GetTenantID(ctx)

// AFTER
db := database.GetDB().WithContext(c.Request.Context())
ctx := c.Request.Context()
tenantID := middleware.GetTenantID(ctx)
```

**注意**：有些方法已经正确使用了 `middleware.GetTenantID(ctx)` 并且显式加了 WHERE，只需要把 `database.GetDB()` 改为 `database.GetDB().WithContext(ctx)` 即可。

### 4.3 `instrument.go` 的 4 处修改

1. `CreateInstrument`（line 150）
2. `UpdateInstrument`（line 362）
3. `GetInstrumentLevels`（line 760）
4. `DeleteInstrument`（line 805）

```go
// BEFORE
db := database.GetDB()

// AFTER
db := database.GetDB().WithContext(c.Request.Context())
```

**注意**：`UpdateInstrument` 在 line 376 已经有 `db.Where("id = ? AND tenant_id = ?", instrumentID, tenantID)`，是安全的。只需要把裸 DB 加上 WithContext。

### 4.4 `public.go` 的处理

**不做修改**。`public.go` 中的路由是 `/api/public/*`，设计意图就是公开访问，没有 JWT 认证，因此没有 request context 中的 tenant_id。

但请注意：Phase 5 会给 `GetPublicInstruments` 的 `tenant_id` 查询参数增加必填校验（如果需要限制的话）。

### 4.5 `system.go` 的处理

`GetClients`（line 20）：
```go
// BEFORE
db := database.GetDB()

// AFTER
db := database.GetDB().WithContext(c.Request.Context())
```

`GetTenants`（line 40）：**不做修改**。这个路由已经被 `middleware.RequireSysPerm(middleware.SysPermTenantList)` 保护，只有 namespace_admin 能访问，返回全系统租户是设计意图。

### 4.6 `notification.go` 的处理

`GetNotifications`（line 17）已经有 `db := database.GetDB().WithContext(ctx)`，正确。但 `MarkNotificationRead`（line 40）也正确。`GetInstrumentPhotoSpecs`（line 59）使用 `database.GetDB()`，但它是查询静态规格数据，不涉及租户隔离，**保持原样**。

等等，重新检查：`notification.go` 只有 3 个方法，前两个已经用了 `.WithContext(ctx)`。裸 DB 调用在 `GetInstrumentPhotoSpecs` 中（line 59），但这是查询 photo spec，无租户字段，不需要改。

### 4.7 `inventory.go` 的处理

`BatchUpdateRent` 中显式使用 `context.Background()` 绕过 request context：

```go
// BEFORE (line 450)
db.Session(&gorm.Session{Context: context.Background()}).
    Where("iam_sub = ? AND deleted_at IS NULL", userID).
    First(&currentUser)

// AFTER
db.Session(&gorm.Session{Context: ctx}).
    Where("iam_sub = ? AND deleted_at IS NULL", userID).
    First(&currentUser)
```

同样修改 line 457 和 line 501 的 `context.Background()` 为 `ctx`。

**注意**：这里保留 `db.Session(&gorm.Session{Context: ctx})` 的结构，只是把 `context.Background()` 替换为 `ctx`（即 `c.Request.Context()`）。

### 4.8 `confirmation_session.go` 的处理

`IAMConfirmationCallback` 和 `SMSCallback` 是 IAM 的回调端点，请求不携带 Tuneloop 的 JWT token，没有标准认证 context。**保持原样**。

### Phase 4 编译检查

```bash
cd /home/coder/tuneloop/backend
go build ./handlers/
go build .
```

### Phase 4 完成后验证

```bash
# 统计剩余裸 DB 调用（排除 test、public、confirmation callback、system/GetTenants）
grep -n 'database\.GetDB()' handlers/*.go | grep -v '_test.go' | grep -v 'public.go' | grep -v 'confirmation_session.go'
```

预期输出应该很少，只有以下情况可以保留：
- `main.go` 中的初始化代码（如 `database.GetDB()` 传给 NewXxxHandler）
- `system.go:40` 的 `GetTenants`（namespace_admin 专用）
- `notification.go:59` 的 `GetInstrumentPhotoSpecs`（无租户数据）
- `resolveTenantName` 中的 `syncDB := database.GetDB()`（异步 goroutine，无 request context）

---

## Phase 5: `ApplyOrgScope` 推广到关键 Handler（P1）

### 5.1 理解 `ApplyOrgScope` 的使用方式

参考 `site.go:ListSites` 的现有用法：

```go
func (h *SiteHandler) ListSites(c *gin.Context) {
    db := database.GetDB().WithContext(c.Request.Context())
    tenantID := middleware.GetTenantID(c.Request.Context())

    query := db.Model(&models.Site{}).Where("tenant_id = ?", tenantID)

    // Apply org scope for data isolation
    if scopedDB, err := middleware.ApplyOrgScope(query, c.Request.Context()); err == nil {
        query = scopedDB
    }
    // ...
}
```

`ApplyOrgScope` 会根据当前用户的 `business_role` 决定可见范围：
- `site_member` → 只能看到 `org_id = 当前 org_id`
- `site_admin` / `merchant_admin` → 看到本 org + 所有下级 org
- `namespace_admin` → 不做限制（返回 nil，即全可见）

### 5.2 需要添加 `ApplyOrgScope` 的 Handler 清单

**以下 handler 查询的数据需要 org 级隔离**（网点管理员不能看到其他网点的数据）：

1. **`api.go:GetInstruments`** — 乐器列表
2. **`instrument.go:CreateInstrument`** — 创建乐器时不需要 ApplyOrgScope（创建是写入，不是查询），但需要确保 `site_id` 在当前用户可见范围内
3. **`instrument.go:UpdateInstrument`** — 同上，更新前查询需要 scope
4. **`user_staff.go:ListStaff`** — 人员列表（已分析存在本地缓存依赖问题）
5. **`lease.go:ListLeases`** — 租约列表
6. **`maintenance.go:ListMerchantMaintenance`** — 维修工单列表
7. **`inventory.go:ListInventory`** — 库存列表

**实施策略**：由于 `ApplyOrgScope` 需要 model 有 `org_id` 字段，先确认以下 model 是否有 `org_id`：

| Model | 是否有 org_id | 是否需要 ApplyOrgScope |
|-------|--------------|----------------------|
| `Instrument` | ✅ 有（`OrgID *string`） | ✅ GetInstruments |
| `User` | ✅ 有（`OrgID string`） | ✅ ListStaff |
| `Lease` | 需确认 | 待确认 |
| `MaintenanceTicket` | 需确认 | 待确认 |
| `Site` | ✅ 有 | ✅（已有） |

**请按以下顺序执行**：

#### 步骤 A：确认 model 字段
读取 `backend/models/models.go` 确认 `Lease`、`MaintenanceTicket`、`Inventory` 等 model 是否有 `org_id` 字段。如果没有，**不要修改 model**，改为在查询时加 `site_id` 过滤（因为所有数据都有 `site_id`）。

#### 步骤 B：修改 `GetInstruments`

**修改前**（api.go:160-176）：
```go
query := db.Model(&models.Instrument{})
userID := middleware.GetUserID(ctx)
recursive := c.Query("recursive") == "true"

if userID != "" {
    var currentUser models.User
    if err := db.Where("iam_sub = ? AND deleted_at IS NULL", userID).First(&currentUser).Error; err == nil {
        role := middleware.GetRole(ctx)
        if recursive && (role == "OWNER" || role == "ADMIN") && currentUser.TenantID != "" {
            query = query.Where("tenant_id = ?", currentUser.TenantID)
        } else if currentUser.SiteID != nil {
            query = query.Where("site_id = ?", *currentUser.SiteID)
        } else if currentUser.TenantID != "" {
            query = query.Where("tenant_id = ?", currentUser.TenantID)
        }
    }
}
```

**修改后**：
```go
query := db.Model(&models.Instrument{})
tenantID := middleware.GetTenantID(ctx)

if tenantID != "" {
    query = query.Where("tenant_id = ?", tenantID)
}

// Apply org scope for data isolation
if scopedDB, err := middleware.ApplyOrgScope(query, ctx); err == nil {
    query = scopedDB
}
```

**注意**：修改后移除了 `userID != ""` 的分支和本地 `users` 表查询。因为：
1. `tenant_id` 已经从 JWT context 获取，不需要再从本地 users 表查
2. `ApplyOrgScope` 会自动处理 org 级可见范围（site_member 只看本 org，site_admin 看本 org + 下级）
3. 不再需要 `recursive` 参数的角色判断（ApplyOrgScope 内部已处理）

**但**：如果前端依赖 `recursive=true` 参数来让 merchant_admin 看到全商户数据，需要确认 `ApplyOrgScope` 对 `merchant_admin` 的行为是否正确。

**检查 `GetBusinessRole` 对 merchant_admin 的判定**：
```go
// middleware/iam.go:381
if tid == oid {
    return BusinessRoleMerchantAdmin
}
```

当 merchant_admin 登录时，`tid == oid`（org_id 等于 tenant_id，即根 org），`GetBusinessRole` 返回 `BusinessRoleMerchantAdmin`。

`GetVisibleOrgIDs` 对 `BusinessRoleMerchantAdmin` 的处理：
```go
// middleware/iam.go:404-410
switch businessRole {
case BusinessRoleSystemAdmin:
    return nil, nil  // 全可见
case BusinessRoleSiteMember:
    return []string{orgID}, nil  // 仅本 org
default:
    return getOrgDescendants(ctx, orgID)  // merchant_admin 和 site_admin 都走这里
}
```

所以 `merchant_admin` 会返回根 org + 所有下级 org（即全商户），这正是我们想要的行为。✅

#### 步骤 C：修改 `ListStaff`

**修改前**（user_staff.go:44-58）：
```go
query := db.Where("tenant_id = ? AND deleted_at IS NULL", tenantID)

currentUserID := middleware.GetUserID(ctx)
var currentUser models.User
if err := db.Where("iam_sub = ?", currentUserID).First(&currentUser).Error; err == nil {
    if currentUser.Role == "site_admin" && currentUser.SiteID != nil {
        siteID, err := uuid.Parse(*currentUser.SiteID)
        if err == nil {
            descendantIDs, err := getDescendantSiteIDs(db, tenantID, siteID)
            if err == nil && len(descendantIDs) > 0 {
                query = query.Where("site_id IN ?", descendantIDs)
            }
        }
    }
}
```

**修改后**：
```go
query := db.Where("tenant_id = ? AND deleted_at IS NULL", tenantID)

// Apply org scope for data isolation
if scopedDB, err := middleware.ApplyOrgScope(query, ctx); err == nil {
    query = scopedDB
}
```

**注意**：这移除了本地 `users.role` 的依赖。`ApplyOrgScope` 会根据 JWT claims 中的 role 自动决定可见范围。

#### 步骤 D：修改 `ListLeases`

`lease.go:22` 的 `ListLeases`：

```go
// 在现有 tenant_id 过滤之后，添加 ApplyOrgScope
query := h.db.WithContext(ctx).Model(&models.Lease{}).Where("tenant_id = ?", tenantID)

if scopedDB, err := middleware.ApplyOrgScope(query, ctx); err == nil {
    query = scopedDB
}
```

**注意**：需确认 `Lease` model 是否有 `org_id` 字段。如果没有，此步骤跳过，改为在后续 Issue 中处理。

### Phase 5 编译检查

```bash
cd /home/coder/tuneloop/backend
go build ./handlers/
go build .
```

---

## Phase 6: 验证测试（P1）

### 6.1 单元测试

```bash
cd /home/coder/tuneloop/backend
go test ./handlers/ -v -run "TestGetInstrument|TestListStaff|TestListLeases|TestGetLabels|TestGetDashboardStats"
```

如果有测试失败：
1. 检查测试是否使用了 mock context（test 中可能手动注入 `tenant_id` 到 gin context）
2. 如果测试用 `c.Set("tenant_id", ...)` 注入值，需要改为注入到 request context：`ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, "...")`

### 6.2 编译全量检查

```bash
cd /home/coder/tuneloop/backend
go build ./...
```

### 6.3 手动端到端验证清单

需要 Senior/Architect 协助或提供测试 token：

| 验证项 | 方法 | 预期结果 |
|--------|------|---------|
| site_admin A 获取乐器列表 | GET /api/instruments | 只能看到本网点 + 下级网点的乐器 |
| site_admin A 获取乐器详情（其他网点 ID） | GET /api/instruments/:id | 404 Not Found |
| site_admin A 修改乐器状态（其他网点 ID） | PUT /api/instruments/:id/status | 404 或 403（tenant_id 不匹配） |
| site_admin A 获取人员列表 | GET /api/staff | 只能看到本网点 + 下级网点的人员 |
| merchant_admin 获取乐器列表 | GET /api/instruments?recursive=true | 能看到全商户所有网点乐器 |
| ReportRepair 端点 | POST /api/maintenance/report | 不再返回 401，正常创建工单 |
| UpdateTicketStatus 端点 | PUT /api/maintenance/tickets/:id/status | 不再返回 401，正常更新 |

---

## 🔧 常见编译错误排查

### 错误 1: "undefined: middleware"
```
handlers/label.go:35:15: undefined: middleware
```
**解决**：在文件顶部 import 中添加 `"tuneloop-backend/middleware"`。

### 错误 2: "middleware.GetTenantID undefined"
```
handlers/label.go:35:25: middleware.GetTenantID undefined
```
**解决**：确认 `middleware/iam.go` 中有 `func GetTenantID(ctx context.Context) string`（已有，不需要你创建）。检查拼写是否正确。

### 错误 3: "too many arguments in call to middleware.GetTenantID"
```
handlers/label.go:35:30: too many arguments in call to middleware.GetTenantID
```
**解决**：`GetTenantID` 只接受一个参数 `ctx`（`context.Context`），不要传 `c`（`*gin.Context`）。正确写法：`middleware.GetTenantID(c.Request.Context())`。

### 错误 4: "db.WithContext undefined"
```
handlers/label.go:35:15: db.WithContext undefined
```
**解决**：确认 `db` 是 `*gorm.DB` 类型。如果 `db` 是自定义类型，检查类型定义。预初始化的 `h.db` 应该是 `*gorm.DB`（在 `main.go` 中通过 `database.GetDB()` 传入）。

### 错误 5: "cannot use c.Request.Context() (type context.Context) as type string"
```
handlers/label.go:35:40: cannot use ... as type string
```
**解决**：你混淆了 `c.GetString`（gin 方法）和 `middleware.GetTenantID`（自定义函数）。`GetTenantID` 返回 `string`，但它接受 `context.Context`。确保你没写成 `c.GetString(middleware.GetTenantID(...))`。

---

## 📝 代码变更检查清单（提交前必查）

提交 PR 前，请逐项确认：

### 全局替换检查
- [ ] `grep 'c\.GetString("tenant_id")' handlers/*.go | grep -v '_test.go'` 无输出
- [ ] `grep 'c\.GetString("org_id")' handlers/*.go | grep -v '_test.go'` 无输出
- [ ] `grep 'c\.GetString("user_id")' handlers/*.go | grep -v '_test.go'` 无输出
- [ ] 所有 `c.GetString` 仅允许在 `public.go`、`confirmation_session.go` 中存在（这些文件无标准 JWT）

### 编译检查
- [ ] `cd backend && go build ./handlers/` 成功
- [ ] `cd backend && go build .` 成功
- [ ] `cd backend && go test ./handlers/` 通过（如果测试存在）

### Update 操作隔离检查
- [ ] `grep -n 'Model(&models.Instrument{}).Where("id = \?").Update' handlers/*.go` 无输出（必须带 tenant_id）
- [ ] `grep -n 'Update("stock_status"' handlers/*.go` 确认都有 tenant_id WHERE

### WithContext 检查
- [ ] 所有非 test handler 方法中，`database.GetDB()` 后紧跟 `.WithContext(ctx)`（公共路由和 callback 除外）
- [ ] 预初始化的 `LabelHandler`、`DashboardHandler`、`LeaseHandler` 的方法内都使用了 `h.db.WithContext(ctx)`

### ApplyOrgScope 检查
- [ ] `api.go:GetInstruments` 调用了 `ApplyOrgScope`
- [ ] `user_staff.go:ListStaff` 调用了 `ApplyOrgScope`
- [ ] `site.go:ListSites` 仍然调用 `ApplyOrgScope`（未被破坏）

### 401 端点修复检查
- [ ] `POST /api/maintenance/report` 不再永远返回 401（`user_id` 从 context 正确获取）
- [ ] `PUT /api/maintenance/tickets/:id/status` 不再永远返回 401

---

## 🏷️ 关联 Issue

- #685 — IAM 角色绑定修复（已完成）
- #686 — CreateSite FK 约束 bug（已创建）
- #687 — username/name 字段 bug（已接受）

---

## 🏷️ 标签建议

`security`, `data-isolation`, `critical`, `backend`, `systemic`, `junior-dev-friendly`

---

*Analysis completed: 2026-05-29*  
*Audited files: backend/handlers/*.go (17 files), backend/middleware/iam.go, backend/database/db.go, backend/main.go*  
*Total methods audited: 235 handler methods, 44+ isolation defects identified*
