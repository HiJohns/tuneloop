# TuneLoop 权限-人员矩阵

> 版本: v2.0  
> 最后更新: 2026-05-27  
> 来源: 本文档汇总了 `backend/middleware/permissions.go`、`backend/services/permission_registry.go`、`backend/services/role_templates.go`、`frontend-pc/src/config/menuPermissions.js` 和 `docs/iam.md` 中的权限定义  
> 重大变更: cus_perm 从 70 码精简为 10 码（#660）

---

## 一、权限体系概述

TuneLoop 使用 BeaconIAM JWT 中的双层位图实现权限控制：

| 层级 | 来源 | 存储 | 用途 |
|------|------|------|------|
| sys_perm | IAM 内置位码 (0-26) | IAM JWT | 控制结构操作：商户管理、网点管理、人员管理、角色配置、客户端管理、权限管理 |
| cus_perm | TuneLoop 注册 (10 码) | IAM JWT (OR 运算) | 控制业务操作：乐器 CRUD + 定价 + 维修 + 订单 CRUD |

> **cus_perm OR 逻辑**（IAM 侧计算）：  
> `token.cus_perm = relation.CusPerm | role.CusPerm`  
> Tuneloop 不参与 OR 计算，由 IAM JWT 签发时自动完成。

**权限管理页面**（`/system/permissions`）：
- 守卫：sys_perm bit 26 (`permission:manage`)
- 商户管理员可访问，含「成员权限」和「角色管理」两个 Tab
- 网点管理员无此权限，通过人员管理页面（`/staff`）分配角色

**角色层级结构**（IAM 定义）：

| 角色 | IAM 代码 | TuneLoop 名称 | 角色说明 |
|------|----------|-------------|---------|
| 命名空间管理员 | — | system_admin | 全部 sys_perm (bit 0-25)，无 cus_perm |
| 商户管理员 | OWNER | owner (merchant_admin) | 全部 sys_perm (bit 5-19,26) + 全部 cus_perm (10) |
| 网点管理员 | ADMIN | admin (site_admin) | user_* sys_perm + cus_perm (7) |
| 网点员工 | STAFF | staff (site_member) | 无 sys_perm + cus_perm (5) |
| 维修工程师 | WORKER | worker | 无 sys_perm + cus_perm (2) |

**命名空间管理员规则**：
> `roles 包含 "namespace_admin"` → 仅仪表盘 + 商户管理 + 客户端管理 + 操作日志。

---

## 二、sys_perm 系统权限位码表

> 来源: `backend/middleware/permissions.go`

| 位码 | 常量名 | 代码 | 权限域 | 说明 | 持有者 |
|------|--------|------|--------|------|--------|
| 0 | SysPermNamespaceView | namespace_view | 命名空间 | 查看客户端 | namespace_admin |
| 1 | SysPermNamespaceList | namespace_list | 命名空间 | 列出客户端 | namespace_admin |
| 2 | SysPermNamespaceCreate | namespace_create | 命名空间 | 创建客户端 | namespace_admin |
| 3 | SysPermNamespaceUpdate | namespace_update | 命名空间 | 编辑客户端 | namespace_admin |
| 4 | SysPermNamespaceDelete | namespace_delete | 命名空间 | 删除客户端 | namespace_admin |
| 5 | SysPermTenantView | tenant_view | 租户 | 查看商户 | namespace_admin, merchant_admin |
| 6 | SysPermTenantList | tenant_list | 租户 | 列出商户 | namespace_admin, merchant_admin |
| 7 | SysPermTenantCreate | tenant_create | 租户 | 管理商户 | namespace_admin, merchant_admin |
| 8 | SysPermTenantUpdate | tenant_update | 租户 | 编辑商户 | namespace_admin, merchant_admin |
| 9 | SysPermTenantDelete | tenant_delete | 租户 | 删除商户 | namespace_admin, merchant_admin |
| 10 | SysPermOrganizationView | organization_view | 组织 | 查看网点 | namespace_admin, merchant_admin |
| 11 | SysPermOrganizationList | organization_list | 组织 | 列出网点 | namespace_admin, merchant_admin |
| 12 | SysPermOrganizationCreate | organization_create | 组织 | 创建网点 | namespace_admin, merchant_admin |
| 13 | SysPermOrganizationUpdate | organization_update | 组织 | 编辑网点 | namespace_admin, merchant_admin |
| 14 | SysPermOrganizationDelete | organization_delete | 组织 | 删除网点 | namespace_admin, merchant_admin |
| 15 | SysPermUserView | user_view | 用户 | 查看人员 | namespace_admin, merchant_admin, site_admin |
| 16 | SysPermUserList | user_list | 用户 | 列出人员 | namespace_admin, merchant_admin, site_admin |
| 17 | SysPermUserCreate | user_create | 用户 | 创建人员 | namespace_admin, merchant_admin, site_admin |
| 18 | SysPermUserUpdate | user_update | 用户 | 编辑人员 | namespace_admin, merchant_admin |
| 19 | SysPermUserDelete | user_delete | 用户 | 删除人员 | namespace_admin, merchant_admin |
| 20 | SysPermRoleView | role_view | 角色 | 查看角色 | namespace_admin |
| 21 | SysPermRoleList | role_list | 角色 | 列出角色 | namespace_admin |
| 22 | SysPermRoleCreate | role_create | 角色 | 创建角色 | namespace_admin |
| 23 | SysPermRoleUpdate | role_update | 角色 | 编辑角色 | namespace_admin |
| 24 | SysPermRoleDelete | role_delete | 角色 | 删除角色 | namespace_admin |
| **25** | **SysPermTenantCreateEx** | **tenant:create** | **租户** | **创建租户（仅命名空间管理员）** | **namespace_admin** |
| **26** | **SysPermPermissionManage** | **permission:manage** | **权限** | **管理权限（商户管理员）** | **merchant_admin** |

> **Bits 25-26** 为 #660 新增，IAM v2 扩展，从 bit 25 追加。

---

## 三、cus_perm 业务权限表（#660 重新设计）

> 来源: `backend/services/permission_registry.go`，10 个业务权限

### 3.1 权限码定义

| Bit | 代码 | Name | 域 | 说明 |
|-----|------|------|-----|------|
| 0 | `instrument:create` | 创建乐器 | 乐器 | 含分类/属性/标签创建 |
| 1 | `instrument:read` | 查看乐器 | 乐器 | 含列表/详情/分类/属性/标签/库存/维修记录 |
| 2 | `instrument:update` | 编辑乐器 | 乐器 | 含分类/属性/标签/库存/调拨、标记维修中 |
| 3 | `instrument:delete` | 删除乐器 | 乐器 | 含分类/属性/标签删除 |
| 4 | `instrument:price` | 乐器定价 | 乐器 | 租金设定，独立于编辑 |
| 5 | `instrument:maintain` | 维修管理 | 乐器 | 进入维修乐器列表，执行维修（开始/完成） |
| 6 | `order:create` | 创建订单 | 订单 | 含租赁/押金创建 |
| 7 | `order:read` | 查看订单 | 订单 | 含订单/租赁/押金查看 |
| 8 | `order:update` | 编辑订单 | 订单 | 含租赁/押金/定损/支付/取件/归还 |
| 9 | `order:cancel` | 取消订单 | 订单 | 含终止 |

### 3.2 旧码→新码迁移映射

| 旧码 | 新码 | 说明 |
|------|------|------|
| `instrument:list` / `instrument:view` | `instrument:read` | 合并 |
| `instrument:edit` | `instrument:update` | 重命名 |
| `instrument:create` / `instrument:delete` | 不变 | |
| `category:manage` / `property:manage` | `instrument:update` | 归入乐器编辑 |
| `inventory:view` | `instrument:read` | 归入乐器查看 |
| `inventory:manage` | `instrument:update` | 归入乐器编辑 |
| `rent:setting` / `finance:config` | `instrument:price` | 归入定价 |
| `maintenance:view` | `instrument:read` | 查看维修记录=查看乐器 |
| `maintenance:assign` / `maintenance:complete` | `instrument:maintain` | 维修管理 |
| `order:list` / `order:view` | `order:read` | 合并 |
| `order:manage` | `order:update` + `order:cancel` | 拆为两个 |
| `order:pay` / `order:pickup` / `order:return` | `order:update` | 归入订单编辑 |

### 3.3 权限域分组

| 权限域 | cus_perm 数量 | 权限代码 |
|--------|-------------|---------|
| 乐器 | 6 | instrument:create, instrument:read, instrument:update, instrument:delete, instrument:price, instrument:maintain |
| 订单 | 4 | order:create, order:read, order:update, order:cancel |

---

## 四、角色-权限分配矩阵

### 4.1 角色 cus_perm 分配

| 角色 | 代码 | cus_perm 数量 | 分配的权限 |
|------|------|-------------|----------|
| 商户管理员 | owner | 10 (全部) | 全部业务权限 |
| 网点管理员 | admin | 8 | instrument:create, instrument:read, instrument:update, instrument:price, instrument:maintain, order:read, order:update, order:cancel |
| 网点员工 | staff | 7 | instrument:create, instrument:read, instrument:update, instrument:maintain, order:create, order:read, order:update |
| 维修工程师 | worker | 2 | instrument:read, instrument:maintain |
| 顾客 | customer | 3 | order:create, order:read, order:cancel |

### 4.2 完整对照矩阵

| 权限代码 | 商户管理员 | 网点管理员 | 网点员工 | 维修工程师 | 顾客 |
|----------|:------:|:------:|:------:|:------:|:------:|
| instrument:create | ✅ | ✅ | ✅ | ❌ | ❌ |
| instrument:read | ✅ | ✅ | ✅ | ✅ | ❌ |
| instrument:update | ✅ | ✅ | ✅ | ❌ | ❌ |
| instrument:delete | ✅ | ❌ | ❌ | ❌ | ❌ |
| instrument:price | ✅ | ✅ | ❌ | ❌ | ❌ |
| instrument:maintain | ✅ | ✅ | ✅ | ✅ | ❌ |
| order:create | ✅ | ❌ | ✅ | ❌ | ✅ |
| order:read | ✅ | ✅ | ✅ | ❌ | ✅ |
| order:update | ✅ | ✅ | ✅ | ❌ | ❌ |
| order:cancel | ✅ | ✅ | ❌ | ❌ | ✅ |

---

## 五、菜单-权限映射

### 5.1 菜单结构（#660 更新后）

> 来源: `frontend-pc/src/App.jsx`

| 菜单组 | 菜单项 | 路由 | 权限 |
|--------|--------|------|------|
| 乐器管理 | 乐器列表 | /instruments/list | cusPerm: instrument:create/read/update/delete |
| 乐器管理 | 分类设置 | /instruments/categories | cusPerm: instrument:update |
| 乐器管理 | 属性管理 | /instruments/properties | cusPerm: instrument:update |
| 维修管理 | 师傅管理 | /maintenance/workers | cusPerm: instrument:maintain |
| 维修管理 | 会话管理 | /maintenance/sessions | cusPerm: instrument:read, instrument:maintain |
| 库存监控 | 租金设定 | /inventory/rent-setting | cusPerm: instrument:price |
| 库存监控 | 库管工作台 | /warehouse | cusPerm: instrument:read, instrument:update |
| 组织管理 | 网点管理 | /organization/sites | sysPerm: [10] AND cusPerm: [instrument:create, instrument:read] |
| 组织管理 | 人员管理 | /staff | sysPerm: [15] AND cusPerm: [instrument:create, instrument:read] |
| 系统管理 | 商户管理 | /merchants | sysPerm: [5] |
| 系统管理 | 操作日志 | /system/audit-logs | sysPerm: [5] |
| 系统管理 | 权限管理 | /system/permissions | sysPerm: [26] |

### 5.2 Grace Period 规则

当 `cus_perm === 0 && sys_perm > 0` 时，所有包含 cus_perm 条件的菜单规则自动通过。

---

## 六、代码位置索引

### 6.1 后端（Go）

| 文件 | 内容 |
|------|------|
| `backend/middleware/permissions.go` | sys_perm 位码常量定义 (0-26) + RequireSysPerm / RequireCusPerm 中间件 |
| `backend/services/permission_registry.go` | 10 cus_perm 定义 + GetCusPermBit / GetCusPermMapping |
| `backend/services/role_templates.go` | AllRoleTemplates 角色-权限模板 |
| `backend/services/iam_client.go` | SetUserCustomerPermissions / SyncRoleTemplateCusPerm / CreateRoleTemplate |
| `backend/handlers/permission_manage.go` | 成员权限列表 / 设置个人权限 / 分配角色 |
| `backend/handlers/role_manage.go` | 角色 CRUD（调 IAM API + 本地缓存） |
| `backend/main.go` | 路由注册 / 守卫配置 / startup 同步 |

### 6.2 前端（JavaScript）

| 文件 | 内容 |
|------|------|
| `frontend-pc/src/config/menuPermissions.js` | SysPermBits 常量 + checkPermission 函数 |
| `frontend-pc/src/App.jsx` | 菜单结构定义 + 路由注册 |
| `frontend-pc/src/pages/admin/PermissionManage/index.jsx` | 权限管理页（成员权限 + 角色管理） |
| `frontend-pc/src/components/SiteMemberManagement.jsx` | 网点成员角色下拉 |
| `frontend-pc/src/services/api.js` | adminApi (权限管理 API 封装) |

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

### 7.2 cus_perm 同步流程（#660 修正）

```
Tuneloop 创建角色 → POST /namespaces/:ns/role-templates (sys_perm=0)
Tuneloop 角色权限 → PUT /role-templates/:id/customer-permissions (cus_perm bitmap)
Tuneloop 分配角色 → POST /users/:id/roles
Tuneloop 个人授权 → PUT /orgs/:id/users/:uid/customer-permissions (raw_bits=true)

IAM JWT 签发时：token.cus_perm = relation.CusPerm | role.CusPerm
```

---

*数据来源: `backend/middleware/permissions.go`、`backend/services/permission_registry.go`、`backend/services/role_templates.go`、`frontend-pc/src/config/menuPermissions.js`、`docs/iam.md`*
