# TuneLoop RBAC Permission Architecture Investigation Report

## 1. Executive Summary

TuneLoop's RBAC (Role-Based Access Control) system has a **critical "break point"**: the infrastructure (database tables, middleware functions) is complete, but **role-based authorization is never actually enforced** on routes or handlers. This is the root cause why TECHNICIAN can successfully call OWNER-exclusive APIs and receive 201 instead of 403.

---

## 2. Route Layer Analysis (`backend/main.go`)

### 2.1 Current State: Routes Have No Role-Based Authorization

```go
// main.go lines 67-75
authRequired := api.Group("")
authRequired.Use(middleware.IAMInterceptor(iamService))  // JWT validation only
authRequired.Use(middleware.NoCache())
{
    authRequired.GET("/instruments", handlers.GetInstruments)
    authRequired.POST("/instruments", handlers.CreateInstrument)  // No role check
    // ... other routes
}
```

### 2.2 Findings

1. **Only IAMInterceptor attached** - No role middleware used
2. **RequireRole() and RequireOwner() exist but are NEVER USED**
3. **No "AdminOnly" or "OwnerOnly" route groups exist**
4. **All authenticated users can access all routes**

---

## 3. Middleware Layer Analysis (`backend/middleware/iam.go`)

### 3.1 Current State: Middleware Has 403 Capability - But Not Enabled

```go
// Lines 144-158: RequireRole exists and works - BUT NOT USED
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

// Lines 160-172: RequireOwner exists - BUT NOT USED
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

### 3.2 Current IAMInterceptor Function (Lines 59-123)

- **Only validates JWT token authenticity**
- Extracts Claims: `Tid`, `Oid`, `Role`, `Own`, `Name`
- Sets context values but **never checks role**

---

## 4. Handler Layer Analysis (`backend/handlers/instrument.go`)

### 4.1 Current State: No Manual Role Checking Inside Handlers

```go
// Lines 46-68: CreateInstrument - NO authorization check
func CreateInstrument(c *gin.Context) {
    db := database.GetDB()
    ctx := c.Request.Context()
    tenantID := middleware.GetTenantID(ctx)
    // ... get context
    
    // NOTE: No role check! No permission check!
    // Any authenticated user can create instruments
}
```

**This code does NOT exist:**
- `if userRole != "OWNER" { return 403 }`
- `middleware.RequireRole("OWNER")`

---

## 5. Role & Permission Definitions

### 5.1 Database Tables Exist (`backend/database/migrations/007_add_permissions.up.sql`)

```sql
-- Lines 51-57: Default roles defined
INSERT INTO roles (name, description, is_system) VALUES
    ('OWNER', 'Tenant owner with full access', true),
    ('ADMIN', 'Administrator with management access', true),
    ('TECHNICIAN', 'Maintenance technician', true),
    ('USER', 'Regular user', true);
```

### 5.2 Role-Permission Mapping

| Role | Permissions |
|------|-------------|
| **OWNER** | ALL permissions |
| **ADMIN** | All except `users:manage` |
| **TECHNICIAN** | `dashboard:view` + maintenance permissions only |
| **USER** | View-only (all `*:view` permissions) |

---

## 6. Root Cause Analysis: Why TECHNICIAN Can Call OWNER-Only APIs

### Break Point Flow

```
1. TECHNICIAN sends request to POST /api/instruments
           ↓
2. Route matched: authRequired.POST("/instruments", handlers.CreateInstrument)
           ↓
3. IAMInterceptor checks JWT token validity (PASSES - valid token)
           ↓
4. NO role check performed! (RequireRole/RequireOwner NOT used)
           ↓
5. CreateInstrument handler executes (no role check inside)
           ↓
6. Returns 201 Created ✓ (TECHNICIAN successfully created instrument)
```

---

## 7. Fix Recommendations

### Option 1: Add Middleware at Route Level (Recommended)

```go
// backend/main.go
ownerRequired := authRequired.Group("")
ownerRequired.Use(middleware.RequireOwner())
{
    ownerRequired.POST("/instruments", handlers.CreateInstrument)
    // Add other owner-only routes
}

techRequired := authRequired.Group("")
techRequired.Use(middleware.RequireRole("TECHNICIAN"))
{
    techRequired.GET("/technician/tickets", maintHandler.ListTechnicianTickets)
}
```

### Option 2: Add Permission Check in Handler

```go
// backend/handlers/instrument.go
func CreateInstrument(c *gin.Context) {
    // ... existing code ...
    
    role := middleware.GetRole(ctx)
    if role != "OWNER" && role != "ADMIN" {
        c.JSON(http.StatusForbidden, gin.H{
            "code":    40300,
            "message": "insufficient permissions to create instruments",
        })
        return
    }
    
    // ... existing code ...
}
```

---

## 8. Conclusion

### Break Point Locations

RBAC "break point" exists in **two locations**:

1. **Route Layer (main.go)**: `RequireRole()` and `RequireOwner()` middleware exist but **are never attached** to any routes
2. **Handler Layer**: No manual permission checks inside handlers like `CreateInstrument`

### Existing Capabilities

- ✅ Middleware already has capability to return 403
- ✅ Database schema and default data properly configured
- ❌ Routes and Handlers have not enabled permission checks

### Next Steps

1. Add `RequireRole()` or `RequireOwner()` middleware to route definitions in `backend/main.go`
2. Or add role checking logic inside key Handler functions
3. Re-run integration tests to verify TECHNICIAN returns 403

---

## Appendix: File Index

| File | Lines | Content |
|------|-------|---------|
| `backend/main.go` | 67-75 | Route definitions and middleware attachment |
| `backend/middleware/iam.go` | 59-123 | IAMInterceptor JWT validation |
| `backend/middleware/iam.go` | 144-158 | RequireRole middleware (unused) |
| `backend/middleware/iam.go` | 160-172 | RequireOwner middleware (unused) |
| `backend/handlers/instrument.go` | 46-68 | CreateInstrument function (no permission check) |
| `backend/database/migrations/007_add_permissions.up.sql` | 51-57 | Role definitions |

---

*Model: moonshotai-cn/kimi-k2-thinking*
