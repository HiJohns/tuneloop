# 页面审计 — 项目相关检查

> 本文档供 `/audit-page` 命令在 Step 3.2 等步骤中引用。包含 TuneLoop 项目特定的文件路径、检查命令和常见陷阱。

## 📁 核心文档

| 文档 | 路径 | 角色 |
|------|------|------|
| 需求文档 | `docs/cases.md` | Step 1 推导来源 |
| 设计文档 | `docs/ui.md` | Step 2 对比目标 |
| API 文档 | `docs/api.md` | 端点/权限参考 |
| 权限矩阵 | `docs/permissions.md` | 角色-权限对照 |
| AGENTS.md | `AGENTS.md` | 核心文档清单 + 规则 |

## 🔍 项目特定检查

### 端点匹配

```bash
# 提取所有后端路由
grep -E '\.(GET|POST|PUT|DELETE)\(' backend/main.go | grep -v '^[[:space:]]*//'

# 提取 Mobile 前端 API 调用
grep -rn "api\.\(get\|post\|put\|delete\)\|request(" frontend-mobile/src/services/api.js

# 提取 PC 前端 API 调用
grep -rn "staffApi\.\|api\.\(get\|post\|put\|delete\)" frontend-pc/src/services/api.js
```

**常见陷阱**：前端 `api.post('/users', data)` 的 baseURL 已在 `services/api.js` 定义为 `/api`，不要在前面再加 `/api`。

### 请求体合约

```bash
# 提取后端 struct json tags
grep -B2 -A20 "ShouldBindJSON" backend/handlers/{文件}.go

# 提取前端请求 body
grep -A10 "body: JSON.stringify\|api\.post\|api\.put" frontend-mobile/src/pages/{文件}.jsx
grep -A10 "body: JSON.stringify\|api\.post\|api\.put" frontend-pc/src/pages/{文件}.jsx
```

### 响应体合约

```bash
# 提取后端返回结构
grep -A30 "c.JSON\|gin.H{" backend/handlers/{文件}.go

# 提取前端消费路径
grep -n "result\.\|response\." 前端文件
```

**TuneLoop 标准响应格式**：`{ code: 20000, message: "success", data: {...} }`。前端应访问 `response.code` 和 `response.data.xxx`。

### 权限匹配

TuneLoop 使用两层权限系统：

| 层 | 前端检查 | 后端检查 | 示例 |
|------|---------|---------|------|
| 角色 | `businessRole === 'site_admin'` 等 | `RequireRole("ADMIN", "OWNER")` | 菜单可见性 |
| 位掩码(cus_perm) | `has('order:read')` → bitmask 检查 | `RequireCusPerm("order:read")` | 操作按钮 |
| 位掩码(sys_perm) | `checkPermission()` | `RequireSysPerm(bits.X)` | 系统管理功能 |

```bash
# 提取前端权限检查
grep -rn "has(\|businessRole\|checkPermission" frontend-mobile/src/pages/ frontend-pc/src/pages/

# 提取后端权限检查
grep -rn "RequireCusPerm\|RequireSysPerm\|RequireRole" backend/handlers/ backend/main.go
```

**常见陷阱**：
- 前端 `has('xxx')` 但后端无权限检查 → 安全漏洞
- 路由在 `authRequired` 组下（需要 org binding），但顾客 JWT 无 tid/oid → 40104
- 前端无权限门控，按钮可见但提交被后端拒绝 → 用户体验差

### IAM 调用链追踪

TuneLoop 后端 handler 中大量使用 `iamClient.XXX()` 调用 IAM API。**IAM 操作分为两类：**

| 认证方式 | 方法后缀 | 用途 |
|---------|---------|------|
| client_credentials | 无 `WithToken` | 创建/查询 IAM 用户、同步数据 |
| **操作者 token** | `WithToken` | 绑定组织、设置权限、分配角色模板 |

```bash
# 提取 handler 中所有 IAM 调用
grep -n "iamClient\." backend/handlers/{文件}.go

# 检查 token 来源
grep -n "ExtractUserToken\|GetClientToken\|userToken\|clientToken" backend/handlers/{文件}.go

# 检查 WithToken 版本是否用对
grep -n "WithToken\b" backend/handlers/{文件}.go
# 权限操作必须用 WithToken 版（如 BindUserToOrganizationWithToken、SetUserCustomerPermissionsWithToken）
```

**常见陷阱**：
- `SetUserCustomerPermissions()` 用 client_credentials → IAM 返回 403 "exceeds your authority"
- 应改用 `SetUserCustomerPermissionsWithToken(userToken, ...)`
- 操作者可能权限不够，但至少以操作者身份验证——IAM 侧才能正确拒或放
