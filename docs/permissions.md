# TuneLoop 权限-人员矩阵

> 版本: v1.0
> 最后更新: 2026-05-06
> 来源: 本文档汇总了 `backend/middleware/permissions.go`、`backend/services/permission_bootstrap.go`、`frontend-pc/src/config/menuPermissions.js`、`docs/iam.md` 和 `docs/ui.md` 中的权限定义

---

## 一、权限体系概述

TuneLoop 使用 BeaconIAM JWT 中的双层位图实现权限控制：

| 层级 | 来源 | 存储 | 用途 |
|------|------|------|------|
| sys_perm | IAM 内置位码 (0-24) | IAM JWT | 控制结构操作：商户管理、网点管理、人员管理、角色配置、客户端管理、IAM 同步 |
| cus_perm | TuneLoop 注册 (启动时 PUT 至 IAM) | IAM JWT (OR 运算) | 控制业务操作：乐器 CRUD、库存、订单、维修、财务、申诉 |

**角色层级结构**（IAM 定义，`docs/iam.md:46-55`）：

| 角色 | IAM 代码 | TuneLoop 名称 | 角色说明 |
|------|----------|-------------|---------|
| 命名空间管理员 | — | system_admin | `sys_perm > 0 && cus_perm === 0`，仅见仪表盘+商户+客户端 |
| 商户管理员 | OWNER | owner (merchant_admin) | 全部菜单和权限 |
| 网点管理员 | ADMIN | admin (site_admin) | 本网点业务管理 |
| 网点员工 | STAFF | staff (site_member) | 有限操作权限 |
| 维修工程师 | WORKER | worker | 维修相关操作 |

**命名空间管理员规则**（`menuPermissions.js:179-187`）：
> `sys_perm > 0 && cus_perm === 0` → 仅仪表盘 + 商户管理 + 客户端管理可见。

---

## 二、sys_perm 系统权限位码表

> 来源: `backend/middleware/permissions.go:10-37`，与 IAM `docs/database.md` 定义一致

| 位码 | 常量名 | 代码 | 权限域 | 说明 |
|------|--------|------|--------|------|
| 0 | SysPermNamespaceView | namespace_view | 命名空间 | 查看客户端 |
| 1 | SysPermNamespaceList | namespace_list | 命名空间 | 列出客户端 |
| 2 | SysPermNamespaceCreate | namespace_create | 命名空间 | 创建客户端 |
| 3 | SysPermNamespaceUpdate | namespace_update | 命名空间 | 编辑客户端 |
| 4 | SysPermNamespaceDelete | namespace_delete | 命名空间 | 删除客户端 |
| 5 | SysPermTenantView | tenant_view | 租户 | 查看商户 |
| 6 | SysPermTenantList | tenant_list | 租户 | 列出商户 |
| 7 | SysPermTenantCreate | tenant_create | 租户 | 创建商户 |
| 8 | SysPermTenantUpdate | tenant_update | 租户 | 编辑商户 |
| 9 | SysPermTenantDelete | tenant_delete | 租户 | 删除商户 |
| 10 | SysPermOrganizationView | organization_view | 组织 | 查看网点 |
| 11 | SysPermOrganizationList | organization_list | 组织 | 列出网点 |
| 12 | SysPermOrganizationCreate | organization_create | 组织 | 创建网点 |
| 13 | SysPermOrganizationUpdate | organization_update | 组织 | 编辑网点 |
| 14 | SysPermOrganizationDelete | organization_delete | 组织 | 删除网点 |
| 15 | SysPermUserView | user_view | 用户 | 查看人员 |
| 16 | SysPermUserList | user_list | 用户 | 列出人员 |
| 17 | SysPermUserCreate | user_create | 用户 | 创建人员 |
| 18 | SysPermUserUpdate | user_update | 用户 | 编辑人员 |
| 19 | SysPermUserDelete | user_delete | 用户 | 删除人员 |
| 20 | SysPermRoleView | role_view | 角色 | 查看角色 |
| 21 | SysPermRoleList | role_list | 角色 | 列出角色 |
| 22 | SysPermRoleCreate | role_create | 角色 | 创建角色 |
| 23 | SysPermRoleUpdate | role_update | 角色 | 编辑角色 |
| 24 | SysPermRoleDelete | role_delete | 角色 | 删除角色 |

**权限域与菜单/API 对应关系：**

| 位码范围 | 权限域 | 对应的 IAM 位码 | TuneLoop 菜单 | 对应 API 端点 |
|---------|--------|---------------|--------------|-------------|
| 0-4 | 命名空间 | namespace_* | 系统管理→客户端管理 | `GET/POST/PUT/DELETE /system/clients` |
| 5-9 | 租户 | tenant_* | 系统管理→商户管理 | `GET/POST/PUT/DELETE /merchants` |
| 10-14 | 组织 | organization_* | 组织管理→网点管理 | `GET/POST/PUT/DELETE /sites`, `POST /iam/organizations/sync` |
| 15-19 | 用户 | user_* | 组织管理→人员管理 | `GET/POST/PUT/DELETE /users`, `POST /iam/users/sync` |
| 20-24 | 角色 | role_* | 系统管理→角色管理 | `GET/PUT /admin/roles/:id/permissions` |

---

## 三、cus_perm 业务权限表

> 来源: `backend/services/permission_bootstrap.go:34-68`，15 个业务权限

| 序号 | 权限代码 | 权限域 | 说明 | 控制菜单 |
|------|---------|--------|------|---------|
| 1 | instrument:create | 乐器 | 创建乐器 | 乐器列表 |
| 2 | instrument:edit | 乐器 | 编辑乐器 | 乐器列表 |
| 3 | instrument:delete | 乐器 | 删除乐器 | 乐器列表 |
| 4 | instrument:view | 乐器 | 查看乐器 | 乐器列表 |
| 5 | category:manage | 乐器 | 管理分类 | 分类设置 |
| 6 | property:manage | 乐器 | 管理属性 | 属性管理 |
| 7 | inventory:view | 库存 | 查看库存 | 库管工作台 |
| 8 | inventory:manage | 库存 | 管理库存 | 库管工作台 |
| 9 | rent:setting | 库存 | 租金设定 | 租金设定 |
| 10 | order:view | 订单 | 查看订单 | 订单管理 |
| 11 | order:manage | 订单 | 管理订单 | 订单管理 |
| 12 | maintenance:view | 维修 | 查看维修 | 会话管理 |
| 13 | maintenance:assign | 维修 | 分派维修 | 师傅管理 / 会话管理 |
| 14 | maintenance:complete | 维修 | 完成维修 | 会话管理 |
| 15 | finance:config | 财务 | 财务配置 | 财务配置 |
| 16 | appeal:handle | 申诉 | 处理申诉 | 申诉处理 |

**权限域分组总结：**

| 权限域 | cus_perm 数量 | 权限代码 |
|--------|-------------|---------|
| 乐器 | 6 | instrument:create, instrument:edit, instrument:delete, instrument:view, category:manage, property:manage |
| 库存 | 3 | inventory:view, inventory:manage, rent:setting |
| 订单 | 2 | order:view, order:manage |
| 维修 | 3 | maintenance:view, maintenance:assign, maintenance:complete |
| 财务 | 1 | finance:config |
| 申诉 | 1 | appeal:handle |

**权限注册流程**（`docs/iam.md:590-597`）：
1. TuneLoop 启动时使用 `client_id/secret` 获取 IAM service token
2. `PUT /api/v1/namespaces/:ns/customer-permissions` 注册全部 15 个业务权限
3. `GET /api/v1/namespaces/:ns/customer-permissions` 获取 IAM 分配的位码映射
4. 缓存到 `tmp/.permission_cache.json`，每 5 分钟后台同步

---

## 四、角色-权限分配矩阵

### 4.1 默认 role → cus_perm 映射

> 来源: `backend/services/permission_bootstrap.go:34-68` + `docs/iam.md:599-606`

| 角色 | 角色代码 | cus_perm 数量 | 分配的权限 |
|------|---------|-------------|----------|
| 商户管理员 | owner | 16 (全部) | instrument:create, instrument:edit, instrument:delete, instrument:view, category:manage, property:manage, inventory:view, inventory:manage, rent:setting, order:view, order:manage, maintenance:view, maintenance:assign, maintenance:complete, finance:config, appeal:handle |
| 网点管理员 | admin | 8 | inventory:view, inventory:manage, order:view, order:manage, maintenance:view, maintenance:assign, maintenance:complete, appeal:handle |
| 网点员工 | staff | 3 | instrument:view, maintenance:view, maintenance:complete |
| 维修工程师 | worker | 2 | maintenance:view, maintenance:complete |

### 4.2 完整角色-权限对照矩阵

| 权限代码 | 命名空间管理员 | 商户管理员(owner) | 网点管理员(admin) | 网点员工(staff) | 维修工程师(worker) |
|----------|:----------:|:----------:|:----------:|:----------:|:----------:|
| instrument:create | ❌ | ✅ | ❌ | ❌ | ❌ |
| instrument:edit | ❌ | ✅ | ❌ | ❌ | ❌ |
| instrument:delete | ❌ | ✅ | ❌ | ❌ | ❌ |
| instrument:view | ❌ | ✅ | ❌ | ✅ | ❌ |
| category:manage | ❌ | ✅ | ❌ | ❌ | ❌ |
| property:manage | ❌ | ✅ | ❌ | ❌ | ❌ |
| inventory:view | ❌ | ✅ | ✅ | ❌ | ❌ |
| inventory:manage | ❌ | ✅ | ✅ | ❌ | ❌ |
| rent:setting | ❌ | ✅ | ❌ | ❌ | ❌ |
| order:view | ❌ | ✅ | ✅ | ❌ | ❌ |
| order:manage | ❌ | ✅ | ✅ | ❌ | ❌ |
| maintenance:view | ❌ | ✅ | ✅ | ✅ | ✅ |
| maintenance:assign | ❌ | ✅ | ✅ | ❌ | ❌ |
| maintenance:complete | ❌ | ✅ | ✅ | ✅ | ✅ |
| finance:config | ❌ | ✅ | ❌ | ❌ | ❌ |
| appeal:handle | ❌ | ✅ | ✅ | ❌ | ❌ |

### 4.3 角色可见菜单

| 角色 | 可见菜单组 | 可见子菜单 |
|------|----------|----------|
| 命名空间管理员 (system_admin) | 仪表盘、系统管理 | 仪表盘、商户管理、客户端管理、租户管理 |
| 商户管理员 (owner) | 全部 | 全部（16 cus_perm 覆盖所有业务菜单） |
| 网点管理员 (admin) | 乐器、库存、维修、组织 | 乐器列表、分类设置、属性管理、库管工作台、租金设定、师傅管理、会话管理、申诉处理 |
| 网点员工 (staff) | 乐器、维修 | 乐器列表、会话管理 |

---

## 五、菜单-权限映射

> 来源: `frontend-pc/src/config/menuPermissions.js:40-132` + `App.jsx:155-220`

### 5.1 纯 sys_perm 控制菜单（命名空间管理员可见）

| 菜单路径 | 路由 | 所需 sys_perm | 所属菜单组 |
|---------|------|-------------|----------|
| 商户管理 | /merchants | tenant_view (bit 5) | 商户管理 |
| 客户端管理 | /system/clients | namespace_view (bit 0) | 系统管理 |
| 租户管理 | /system/tenants | tenant_list (bit 6) | 系统管理 |

### 5.2 纯 cus_perm 控制菜单（业务角色可见）

| 菜单路径 | 路由 | 所需 cus_perm (OR) |
|---------|------|-------------------|
| 乐器列表 | /instruments/list | instrument:create, instrument:edit, instrument:delete, inventory:view, instrument:view |
| 分类设置 | /instruments/categories | category:manage |
| 属性管理 | /instruments/properties | property:manage |
| 租金设定 | /inventory/rent-setting | rent:setting |
| 库管工作台 | /warehouse | inventory:view, inventory:manage |
| 师傅管理 | /maintenance/workers | maintenance:assign |
| 会话管理 | /maintenance/sessions | maintenance:view, maintenance:assign, maintenance:complete |
| 申诉处理 | /appeals | appeal:handle |

### 5.3 组合权限菜单（sys_perm AND cus_perm 同时满足）

| 菜单路径 | 路由 | 所需 sys_perm | 所需 cus_perm (OR) | requireAllGroups |
|---------|------|-------------|-------------------|:---:|
| 网点管理 | /organization/sites | organization_view (bit 10) | instrument:create, inventory:view, maintenance:view | ✅ |
| 人员管理 | /staff | user_view (bit 15) | instrument:create, inventory:view, maintenance:view | ✅ |
| 人员批量导入 | /staff/bulk-import | user_create (bit 17) | instrument:create, inventory:view, maintenance:view | ✅ |
| 网点批量导入 | /organization/sites/bulk-import | organization_create (bit 12) | instrument:create, inventory:view, maintenance:view | ✅ |

### 5.4 Grace Period 规则

> 来源: `menuPermissions.js:162-167`

当 `cus_perm === 0 && sys_perm > 0`（即管理员未初始化业务权限）时，所有包含 cus_perm 条件的菜单规则自动通过。这确保管理员在首次登录时能看到全部菜单以完成权限配置引导。

---

## 六、代码位置索引

### 6.1 后端（Go）

| 文件 | 内容 | 关键行号 |
|------|------|---------|
| `backend/middleware/permissions.go` | sys_perm 位码常量定义 | 10-37 |
| `backend/middleware/permissions.go` | RequireSysPerm / RequireCusPerm 中间件 | 101-158 |
| `backend/services/permission_bootstrap.go` | 15 cus_perm 定义 + 默认角色权限 | 34-68 |
| `backend/services/permission_registry.go` | IAM 注册/同步/缓存 | — |
| `backend/services/iam_client.go` | IAM API 封装 (RegisterCustomerPermissions) | — |
| `backend/main.go` | GET /config/permissions 位码映射端点 + 启动注册 | — |

### 6.2 前端（JavaScript）

| 文件 | 内容 | 关键行号 |
|------|------|---------|
| `frontend-pc/src/config/menuPermissions.js` | sys_perm 位码常量 | 5-31 |
| `frontend-pc/src/config/menuPermissions.js` | 菜单权限规则定义 (menuRules) | 40-132 |
| `frontend-pc/src/config/menuPermissions.js` | checkRule / isNamespaceAdmin 函数 | 138-187 |
| `frontend-pc/src/App.jsx` | 菜单结构定义 (menuConfig) | 155-220 |
| `frontend-pc/src/App.jsx` | 角色过滤 + 位权限过滤 (filteredItems) | 222-270 |
| `frontend-pc/src/App.jsx` | 面包屑路由映射 | 288-300 |

### 6.3 文档

| 文件 | 内容 | 关键行号 |
|------|------|---------|
| `docs/iam.md` | TuneLoop 权限消费机制 | 566-615 |
| `docs/iam.md` | IAM 角色定义 + sys_perm 位码说明 | 46-55, 577-588 |
| `docs/iam.md` | cus_perm 注册缓存 + 默认角色权限 | 590-606 |
| `docs/ui.md` | PC 端侧边栏菜单结构与权限控制表 | 1610-1666 |

---

## 七、权限检查流程

### 7.1 后端权限检查链路

```
HTTP Request
  → IAMInterceptor (JWT 验证 + 提取 sys_perm/cus_perm/tenant_id)
  → RequireSysPerm(bit) / RequireCusPerm(code) 中间件
  → 位运算判断
  → 放行或 403
```

### 7.2 前端菜单过滤链路

```
加载用户 JWT
  → 解析 sys_perm / cus_perm
  → filterMenuByRole (structuralRoles + functionalRoles)
  → checkRule (menuRules 位权限过滤)
  → Grace Period 检查
  → 渲染过滤后的菜单
```

---

*数据来源汇总于 `backend/middleware/permissions.go`、`backend/services/permission_bootstrap.go`、`frontend-pc/src/config/menuPermissions.js`、`docs/iam.md`、`docs/ui.md`*
