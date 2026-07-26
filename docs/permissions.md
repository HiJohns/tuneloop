# TuneLoop 权限-人员矩阵

> 版本: v2.1  
> 最后更新: 2026-07-26  
> 来源: 本文档汇总了 `backend/middleware/permissions.go`、`backend/services/permission_registry.go`、`backend/services/role_templates.go`、`frontend-pc/src/config/menuPermissions.js` 和 `docs/iam.md` 中的权限定义  
> 重大变更: cus_perm 当前 30 码（bits 0-29），含业务操作 18 码 + 平台配置 8 码 + 维修 4 码（#660 后持续扩展）

---

## 一、权限体系概述

TuneLoop 使用 BeaconIAM JWT 中的双层位图实现权限控制：

| 层级 | 来源 | 存储 | 用途 |
|------|------|------|------|
| sys_perm | IAM 内置位码 (0-29, 6组×5位 CRUDL) | IAM JWT | 控制结构操作：商户管理、网点管理、人员管理、角色配置、客户端管理、权限管理 |
| cus_perm | TuneLoop 注册 (30 码) | IAM JWT (4 层 OR 运算) | 控制业务操作 + 平台配置 + 维修 |

> **cus_perm OR 逻辑**（IAM 侧计算，4 层叠加）：  
> `effective = org.DefaultCusPerm | relation.CusPerm | override.CusPerm | role.CusPerm`  
> Tuneloop 不参与 OR 计算，由 IAM JWT 签发时自动完成。各层含义：  
> - `org.DefaultCusPerm` — 组织默认权限（极少使用）
> - `relation.CusPerm` — 用户个人直授权（fallback，Boss 特殊授予）
> - `override.CusPerm` — 用户权限覆写（极少使用）
> - `role.CusPerm` — **角色模板权限（主源）**，用户通过 `functional_roles` 继承角色模板的 cus_perm  
> 
> 详见 IAM `permission_calc.go:20-80`。

**权限管理页面**（`/system/permissions`）：
- 守卫：sys_perm bit 26 (`permission:manage`)
- 商户管理员可访问，含「成员权限」和「角色管理」两个 Tab
- 网点管理员无此权限，通过人员管理页面（`/staff`）分配角色

**角色层级结构**（IAM 定义）：

| 角色 | 功能角色代码 | 角色说明 |
|------|-------------|---------|
| 命名空间管理员 | `namespace_admin` | sys_perm (bit 5-9, 15-19) + cus_perm 7 码（平台配置：category/attribute/banner/rebate/promo/points/membership） |
| 商户管理员 | `merchant_admin` | sys_perm (bit 10-29) + cus_perm 20 码（全部业务操作 + promo:manage + points:manage） |
| 网点管理员 | `site_admin` | sys_perm (bit 15-17) + cus_perm 18 码（乐器 CRUD + 订单 + 媒体 + appeal + audit_log + promo:override + points） |
| 网点员工 | `site_member` | 无 sys_perm + cus_perm 11 码（乐器基本操作 + 订单创建/查看 + 媒体 + audit_log） |
| 维修工程师 | `repair_technician` | 无 sys_perm + cus_perm 2 码（instrument:read, instrument:maintain） |

**命名空间管理员规则**：
> `functional_roles` 包含 `"namespace_admin"` → 仪表盘 + 商户管理 + 操作日志 + 分类设置 + 属性管理 + 轮播图管理 + **经营策略整组菜单**（折扣政策/返点配置/会员级别管理等）。

> 注：namespace_admin 的前端菜单可见性部分走白名单绕过（见 §5.2），不依赖 cus_perm 位掩码判定。

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

## 三、cus_perm 业务权限表

### 3.1 权限码定义

> 来源: `backend/services/permission_registry.go`，30 个业务权限（bits 0-29）

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
| 18 | `category:manage` | 分类管理 | 平台 | 乐器分类 CRUD |
| 19 | `attribute:manage` | 属性管理 | 平台 | 动态属性定义 CRUD |
| 20 | `banner:manage` | 轮播图管理 | 平台 | 首页轮播图 CRUD |
| 21 | `rebate:manage` | 返点管理 | 平台 | 会员返佣比例配置 |
| 22 | `promo:manage` | 折扣政策管理 | 平台 | 系统级折扣策略 CRUD |
| 23 | `promo:override` | 乐器促销覆盖 | 营销 | 单乐器促销覆盖开关 |
| 24 | `points:manage` | 点数政策管理 | 平台 | 积分/点数政策配置 |
| 25 | `membership:manage` | 会员级别管理 | 平台 | 会员等级 CRUD |
| 26 | `repair:read` | 查看维修 | 维修 | 查看维修工单/进度 |
| 27 | `repair:start` | 开始维修 | 维修 | 启动维修流程/指派师傅 |
| 28 | `repair:complete` | 完成维修 | 维修 | 标记维修完成 |
| 29 | `repair:accept` | 验收维修 | 维修 | 验收维修结果 |

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
| 定价 | 1 | instrument:price_config |
| 平台配置 | 8 | category:manage, attribute:manage, banner:manage, rebate:manage, promo:manage, points:manage, membership:manage, promo:override |
| 维修 | 4 | repair:read, repair:start, repair:complete, repair:accept |

---

## 四、角色-权限分配矩阵

### 4.1 角色 cus_perm 分配

| 角色 | 功能角色代码 | cus_perm 数量 | 分配的权限 |
|------|------------|-------------|----------|
| 命名空间管理员 | namespace_admin | 7 | category:manage, attribute:manage, banner:manage, rebate:manage, promo:manage, points:manage, membership:manage |
| 商户管理员 | merchant_admin | 20 | 乐器 CRUD+price+price_config+maintain, 媒体 3, 订单 CRUD, 申诉 3, audit_log, promo:manage, points:manage |
| 网点管理员 | site_admin | 18 | 乐器 CRUD+price+maintain, 媒体 3, 订单 3 (无 create), appeal:read/handle, audit_log, promo:override, points:manage |
| 网点员工 | site_member | 11 | 乐器 CRUD+maintain, 媒体 upload/delete, 订单 3 (无 cancel), audit_log |
| 维修工程师 | repair_technician | 2 | instrument:read, instrument:maintain |
| 顾客 | customer | 4 | order:create, order:read, order:cancel, appeal:create |

### 4.2 完整对照矩阵

| 权限代码 | 命名空间管理员 | 商户管理员 | 网点管理员 | 网点员工 | 维修工程师 | 顾客 |
|----------|:---------:|:--------:|:--------:|:------:|:---------:|:---:|
| instrument:create | ❌ | ✅ | ✅ | ✅ | ❌ | ❌ |
| instrument:read | ❌ | ✅ | ✅ | ✅ | ✅ | ❌ |
| instrument:update | ❌ | ✅ | ✅ | ✅ | ❌ | ❌ |
| instrument:delete | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ |
| instrument:price | ❌ | ✅ | ✅ | ❌ | ❌ | ❌ |
| instrument:maintain | ❌ | ✅ | ✅ | ✅ | ✅ | ❌ |
| order:create | ❌ | ✅ | ❌ | ✅ | ❌ | ✅ |
| order:read | ❌ | ✅ | ✅ | ✅ | ❌ | ✅ |
| order:update | ❌ | ✅ | ✅ | ✅ | ❌ | ❌ |
| order:cancel | ❌ | ✅ | ✅ | ❌ | ❌ | ✅ |
| appeal:create | ❌ | ✅ | ❌ | ❌ | ❌ | ✅ |
| appeal:read | ❌ | ✅ | ✅ | ❌ | ❌ | ❌ |
| appeal:handle | ❌ | ✅ | ✅ | ❌ | ❌ | ❌ |
| audit_log:read | ❌ | ✅ | ✅ | ✅ | ❌ | ❌ |
| instrument:price_config | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ |
| instrument:media_upload | ❌ | ✅ | ✅ | ✅ | ❌ | ❌ |
| instrument:media_display | ❌ | ✅ | ✅ | ❌ | ❌ | ❌ |
| instrument:media_delete | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ |
| category:manage | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| attribute:manage | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| banner:manage | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| rebate:manage | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| promo:manage | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ |
| promo:override | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ |
| points:manage | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ |
| membership:manage | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| repair:read | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| repair:start | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| repair:complete | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| repair:accept | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |

---

## 五、菜单-权限映射

### 5.1 菜单结构

> 来源: `frontend-pc/src/App.jsx` 和 `frontend-pc/src/config/menuPermissions.js`

| 菜单组 | 菜单项 | 路由 | 权限 |
|--------|--------|------|------|
| 基础配置 | 分类设置 | /instruments/categories | cusPerm: category:manage |
| 基础配置 | 属性管理 | /instruments/properties | cusPerm: attribute:manage |
| 基础配置 | 轮播图管理 | /system/banners | cusPerm: banner:manage |
| 经营策略 | 租金设定 | /inventory/rent-setting | cusPerm: instrument:price |
| 经营策略 | 定价策略 | /pricing/config | cusPerm: instrument:price_config |
| 经营策略 | 系统折扣政策 | /system/promo-plans | cusPerm: promo:manage |
| 经营策略 | 报修设置 | /repair/settings | cusPerm: instrument:price_config |
| 经营策略 | 返点配置 | /system/rebate-config | cusPerm: rebate:manage |
| 经营策略 | 会员级别管理 | /system/membership-levels | cusPerm: membership:manage |
| 运营管理 | 订单管理 | /orders | cusPerm: order:read |
| 运营管理 | 乐器列表 | /instruments/list | cusPerm: instrument:create/read/update/delete |
| 运营管理 | 库管工作台 | /warehouse | cusPerm: instrument:read, instrument:update |
| 运营管理 | 会话管理 | /maintenance/sessions | cusPerm: instrument:read, instrument:maintain |
| 运营管理 | 中转路由 | /transit-routes | sysPerm: [5] |
| 运营管理 | 逾期告警 | /overdue-alerts | cusPerm: instrument:read |
| 维修管理 | 师傅管理 | /maintenance/workers | cusPerm: instrument:maintain |
| 组织管理 | 网点管理 | /organization/sites | sysPerm: [10] AND cusPerm: [instrument:create, instrument:read] |
| 组织管理 | 人员管理 | /staff | sysPerm: [15] AND cusPerm: [instrument:create, instrument:read] |
| 组织管理 | 申诉处理 | /appeals | cusPerm: appeal:read |
| 组织管理 | 与 IAM 同步 | /organization/iam-sync | sysPerm: [10] AND cusPerm: [instrument:create, instrument:read] |
| 系统管理 | 商户管理 | /merchants | sysPerm: [5] |
| 系统管理 | 操作日志 | /system/audit-logs | sysPerm: [5] |
| 系统管理 | 权限管理 | /system/permissions | sysPerm: [27] |
| 系统管理 | 警告管理 | /system/warnings | sysPerm: [5] |
| 系统管理 | 警告配置 | /system/warning-settings | sysPerm: [5] |

### 5.2 命名空间管理员白名单绕过

部分菜单项对 `namespace_admin` 角色走**白名单绕过**机制（`getNamespaceAdminMenuKeys()`），即命中白名单后不检查 `cus_perm` 位掩码，直接显示。

```js
// frontend-pc/src/config/menuPermissions.js
function getNamespaceAdminMenuKeys() {
  return ['/', '/merchants', '/system/audit-logs', '/instruments/categories', '/instruments/properties',
    '/system/banners', '/system/promo-plans', '/system/rebate-config', '/system/membership-levels']
}
```

白名单应用场景：
- **菜单渲染**（`App.jsx:385`）：白名单中的菜单项对 namespace_admin 直接可见
- **路由守卫**（`ProtectedRoute.jsx:143`）：白名单中的路由对 namespace_admin 直接放行

> 注：白名单是安全网（补偿 `cus_perm=0` 时的菜单不可见问题），主源仍是 IAM 角色模板 `cus_perm`。

---

## 六、代码位置索引

### 6.1 后端（Go）

| 文件 | 内容 |
|------|------|
| `backend/middleware/permissions.go` | sys_perm 位码常量定义 (0-29) + RequireSysPerm / RequireCusPerm 中间件 |
| `backend/services/permission_registry.go` | 30 cus_perm 定义 + GetCusPermBit / GetCusPermMapping |
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

IAM JWT 签发时：effective = org.DefaultCusPerm | relation.CusPerm | override.CusPerm | role.CusPerm
```

---

*数据来源: `backend/middleware/permissions.go`、`backend/services/permission_registry.go`、`backend/services/role_templates.go`、`frontend-pc/src/config/menuPermissions.js`、`docs/iam.md`*
