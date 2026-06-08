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
| sys_perm | IAM 内置位码 (0-29, 6组×5位 CRUDL) | IAM JWT | 控制结构操作：商户管理、网点管理、人员管理、角色配置、客户端管理、权限管理 |
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
> `roles 包含 "namespace_admin"` → 仪表盘 + 商户管理 + 客户端管理 + 操作日志 + **媒体设置**。

---

## 二、sys_perm 系统权限

> 位定义和角色映射由 [BeaconIAM docs/permissions.md](https://github.com/HiJohns/beaconiam/blob/main/docs/permissions.md) v1.0 管理。
> Tuneloop 只消费以下位映射：

| Bit | 代码 | Tuneloop 用途 |
|-----|------|-------------|
| 0 | namespace:view | 客户端管理菜单/路由 |
| 5 | tenant:view | 商户管理菜单/路由 |
| 6 | tenant:list | GET /api/merchants |
| 7 | tenant:create | POST /api/merchants（创建商户） |
| 10 | organization:view | 网点管理菜单/路由 |
| 12 | organization:create | 网点批量导入 |
| 15 | user:view | 人员管理菜单/路由 |
| 17 | user:create | 人员批量导入 |
| 27 | permission:create | 权限管理菜单/路由及全部 API |

---

## 三、cus_perm 业务权限表（#660 重新设计）

> 来源: `backend/services/permission_registry.go`，14 个业务权限（原 10 个 + 媒体操作 3 个 + 定价策略 1 个）

### 3.1 权限码定义

| Bit | 代码 | Name | 域 | 说明 |
|-----|------|------|-----|------|
| 0 | `instrument:create` | 创建乐器 | 乐器 | 含分类/属性/标签创建 |
| 1 | `instrument:read` | 查看乐器 | 乐器 | 含列表/详情/分类/属性/标签/库存/维修记录 |
| 2 | `instrument:update` | 编辑乐器 | 乐器 | 含分类/标签/库存/调拨、标记维修中 |
| 3 | `instrument:delete` | 删除乐器 | 乐器 | 含分类/属性/标签删除 |
| 4 | `instrument:price` | 乐器定价 | 乐器 | 租金设定，独立于编辑 |
| 5 | `instrument:maintain` | 维修管理 | 乐器 | 进入维修乐器列表，执行维修（开始/完成） |
| 6 | `order:create` | 创建订单 | 订单 | 含租赁/押金创建 |
| 7 | `order:read` | 查看订单 | 订单 | 含订单/租赁/押金查看 |
| 8 | `order:update` | 编辑订单 | 订单 | 含租赁/押金/定损/支付/取件/归还 |
| 9 | `order:cancel` | 取消订单 | 订单 | 含终止 |
| 10 | `appeal:create` | 提交申诉 | 申诉 | 顾客对定损提出申诉 |
| 11 | `appeal:read` | 查看申诉 | 申诉 | 查看申诉列表/详情 |
| 12 | `appeal:handle` | 处理申诉 | 申诉 | 答复/关闭申诉 |
| 13 | `audit_log:read` | 查看日志 | 日志 | 查看操作日志 |
| 14 | `instrument:price_config` | 定价策略配置 | 定价 | 定价策略模板配置 |
| 15 | `instrument:media_upload` | 上传媒体 | 媒体 | 上传图片/视频到乐器 |
| 16 | `instrument:media_display` | 设置展示批次 | 媒体 | 指定乐器展示媒体批次 |
| 17 | `instrument:media_delete` | 删除媒体批次 | 媒体 | 删除乐器的媒体批次 |

### 3.2 旧码→新码迁移映射

| 旧码 | 新码 | 说明 |
|------|------|------|
| `instrument:list` / `instrument:view` | `instrument:read` | 合并 |
| `instrument:edit` | `instrument:update` | 重命名 |
| `instrument:create` / `instrument:delete` | 不变 | |
| `category:manage` | `namespace_admin` (cus_perm, bit 18) | #785 分类管理独立权限 |
| `attribute:manage` | `namespace_admin` (cus_perm, bit 19) | #785 属性管理独立权限 |
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
| 媒体 | 3 | instrument:media_upload, instrument:media_display, instrument:media_delete |
| 订单 | 4 | order:create, order:read, order:update, order:cancel |
| 申诉 | 3 | appeal:create, appeal:read, appeal:handle |
| 日志 | 1 | audit_log:read |

---

## 四、角色-权限分配矩阵

### 4.1 角色 cus_perm 分配

| 角色 | 代码 | cus_perm 数量 | 分配的权限 |
|------|------|-------------|----------|
| 商户管理员 | owner | 17 (全部) | 全部业务权限 |
| 网点管理员 | admin | 13 | instrument:create, instrument:read, instrument:update, instrument:price, instrument:maintain, **instrument:media_upload**, **instrument:media_display**, order:read, order:update, order:cancel, appeal:read, appeal:handle, audit_log:read |
| 网点员工 | staff | 9 | instrument:create, instrument:read, instrument:update, instrument:maintain, **instrument:media_upload**, order:create, order:read, order:update, audit_log:read |
| 维修工程师 | worker | 2 | instrument:read, instrument:maintain |
| 顾客 | customer | 4 | order:create, order:read, order:cancel, appeal:create |

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
| appeal:create | ✅ | ❌ | ❌ | ❌ | ✅ |
| appeal:read | ✅ | ✅ | ❌ | ❌ | ❌ |
| appeal:handle | ✅ | ✅ | ❌ | ❌ | ❌ |
| audit_log:read | ✅ | ✅ | ✅ | ❌ | ❌ |
| instrument:media_upload | ✅ | ✅ | ✅ | ❌ | ❌ |
| instrument:media_display | ✅ | ✅ | ❌ | ❌ | ❌ |
| instrument:media_delete | ✅ | ❌ | ❌ | ❌ | ❌ |

---

## 五、菜单-权限映射

### 5.1 菜单结构（#660 更新后）

> 来源: `frontend-pc/src/App.jsx`

| 菜单组 | 菜单项 | 路由 | 权限 |
|--------|--------|------|------|
| 乐器管理 | 乐器列表 | /instruments/list | cusPerm: instrument:create/read/update/delete |
| 乐器管理 | 分类设置 | /instruments/categories | cusPerm: category:manage |
| 乐器管理 | 属性管理 | /instruments/properties | cusPerm: attribute:manage |
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
| `backend/middleware/permissions.go` | sys_perm 位码常量定义 (0-29) + RequireSysPerm / RequireCusPerm 中间件 |
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
