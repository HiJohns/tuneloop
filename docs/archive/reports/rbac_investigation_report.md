# TuneLoop RBAC 权限架构调查报告

## 1. 执行摘要

TuneLoop 系统的 RBAC（基于角色的访问控制）权限架构存在**关键的"断裂点"**：基础设施（数据库表、中间件函数）已完备，但**路由层和 Handler 层从未实际执行权限校验**。这正是 TECHNICIAN 能成功调用 OWNER 专属 API 并返回 201 而非 403 的根本原因。

---

## 2. 路由层分析 (`backend/main.go`)

### 2.1 现状：路由未配置角色校验

```go
// main.go 第 67-75 行
authRequired := api.Group("")
authRequired.Use(middleware.IAMInterceptor(iamService))  // 仅 JWT 校验
authRequired.Use(middleware.NoCache())
{
    authRequired.GET("/instruments", handlers.GetInstruments)
    authRequired.POST("/instruments", handlers.CreateInstrument)  // 无角色校验
    // ... 其他路由
}
```

### 2.2 发现

1. **仅挂载了 IAMInterceptor**（JWT 验证）— 未使用角色中间件
2. **RequireRole() 和 RequireOwner() 存在但从未使用**
3. **不存在 "AdminOnly" 或 "OwnerOnly" 路由组**
4. **所有已认证用户可访问所有路由**

---

## 3. 中间件层分析 (`backend/middleware/iam.go`)

### 3.1 现状：中间件具备 403 拦截能力，但未启用

```go
// 第 144-158 行：RequireRole 存在且可用 - 但未使用
func RequireRole(roles ...string) gin.HandlerFunc {
    return func(c *gin.Context) {
        userRole := GetRole(c.Request.Context())
        for _, role := range roles {
            if userRole == role {
                c.Next()
                return
            }
        }
        c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
            "code":    40300,
            "message": "insufficient permissions",
        })
    }
}

// 第 160-172 行：RequireOwner 存在 - 但未使用
func RequireOwner() gin.HandlerFunc {
    return func(c *gin.Context) {
        isOwner, ok := c.Request.Context().Value(ContextKeyIsOwner).(bool)
        if !ok || !isOwner {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
                "code":    40301,
                "message": "owner privileges required",
            })
            return
        }
        c.Next()
    }
}
```

### 3.2 当前 IAMInterceptor 功能（第 59-123 行）

- **仅验证 JWT Token 真实性**
- 提取 Claims：`Tid`, `Oid`, `Role`, `Own`, `Name`
- 设置上下文值但**从不检查角色**

---

## 4. Handler 层分析 (`backend/handlers/instrument.go`)

### 4.1 现状：Handler 内部无角色检查

```go
// 第 46-68 行：CreateInstrument - 无授权检查
func CreateInstrument(c *gin.Context) {
    db := database.GetDB()
    ctx := c.Request.Context()
    tenantID := middleware.GetTenantID(ctx)
    // ... 获取上下文
    
    // 注意：无角色检查！无权限检查！
    // 任何已认证用户都可以创建乐器
}
```

**不存在以下代码：**
- `if userRole != "OWNER" { return 403 }`
- `middleware.RequireRole("OWNER")`

---

## 5. 角色与权限定义

### 5.1 数据库表存在（`backend/database/migrations/007_add_permissions.up.sql`）

```sql
-- 第 51-57 行：默认角色定义
INSERT INTO roles (name, description, is_system) VALUES
    ('OWNER', 'Tenant owner with full access', true),
    ('ADMIN', 'Administrator with management access', true),
    ('TECHNICIAN', 'Maintenance technician', true),
    ('USER', 'Regular user', true);
```

### 5.2 角色-权限映射

| 角色 | 权限 |
|------|------|
| **OWNER** | 所有权限 |
| **ADMIN** | 除 `users:manage` 外的所有权限 |
| **TECHNICIAN** | 仅 `dashboard:view` + 维保相关权限 |
| **USER** | 仅查看权限（所有 `*:view`） |

---

## 6. 根因分析：为何 TECHNICIAN 能调用 OWNER 专属 API

### 断裂流程

```
1. TECHNICIAN 发送请求到 POST /api/instruments
           ↓
2. 路由匹配：authRequired.POST("/instruments", handlers.CreateInstrument)
           ↓
3. IAMInterceptor 检查 JWT Token 有效性（通过 - Token 有效）
           ↓
4. 未执行任何角色检查！（RequireRole/RequireOwner 未使用）
           ↓
5. CreateInstrument Handler 执行（内部也无角色检查）
           ↓
6. 返回 201 Created ✓（TECHNICIAN 成功创建乐器）
```

---

## 7. 修复建议

### 方案 1：在路由层添加中间件（推荐）

```go
// backend/main.go
ownerRequired := authRequired.Group("")
ownerRequired.Use(middleware.RequireOwner())
{
    ownerRequired.POST("/instruments", handlers.CreateInstrument)
    // 添加其他 Owner 专属路由
}

techRequired := authRequired.Group("")
techRequired.Use(middleware.RequireRole("TECHNICIAN"))
{
    techRequired.GET("/technician/tickets", maintHandler.ListTechnicianTickets)
}
```

### 方案 2：在 Handler 层添加权限检查

```go
// backend/handlers/instrument.go
func CreateInstrument(c *gin.Context) {
    // ... 现有代码 ...
    
    role := middleware.GetRole(ctx)
    if role != "OWNER" && role != "ADMIN" {
        c.JSON(http.StatusForbidden, gin.H{
            "code":    40300,
            "message": "insufficient permissions to create instruments",
        })
        return
    }
    
    // ... 现有代码 ...
}
```

---

## 8. 总结

### 断裂点位置

RBAC 断裂点存在于**两个位置**：

1. **路由层（main.go）**：`RequireRole()` 和 `RequireOwner()` 中间件存在但**从未挂载**到任何路由
2. **Handler 层**：`CreateInstrument` 等函数内部**无手动权限检查**

### 现有能力

- ✅ 中间件已具备返回 403 的能力
- ✅ 数据库 Schema 和默认数据已正确配置
- ❌ 路由和 Handler 未启用权限校验

### 下一步行动

1. 在 `backend/main.go` 的路由定义中添加 `RequireRole()` 或 `RequireOwner()` 中间件
2. 或在关键 Handler 函数内部添加角色检查逻辑
3. 重新运行集成测试验证 TECHNICIAN 返回 403

---

## 附录：文件索引

| 文件 | 行号 | 内容 |
|------|------|------|
| `backend/main.go` | 67-75 | 路由定义与中间件挂载 |
| `backend/middleware/iam.go` | 59-123 | IAMInterceptor JWT 验证 |
| `backend/middleware/iam.go` | 144-158 | RequireRole 中间件（未使用） |
| `backend/middleware/iam.go` | 160-172 | RequireOwner 中间件（未使用） |
| `backend/handlers/instrument.go` | 46-68 | CreateInstrument 函数（无权限检查） |
| `backend/database/migrations/007_add_permissions.up.sql` | 51-57 | 角色定义 |

---

*Model: moonshotai-cn/kimi-k2-thinking*
