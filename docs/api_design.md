# TuneLoop API 设计文档

## 1. 概述

### 1.1 文档目的
本文档定义 TuneLoop 乐器租赁管理系统的所有后端 API 接口，包括端点定义、请求参数、响应格式和错误码规范。

### 1.2 基础规范
- **Base URL**: `/api`
- **认证方式**: Bearer Token (JWT)
- **内容类型**: `application/json`
- **统一响应格式**:
```json
{
  "code": 20000,
  "message": "success",
  "data": { ... }
}
```

### 1.3 错误码规范
| 错误码 | 说明 |
|--------|------|
| 20000 | 成功 |
| 20100 | 创建成功 |
| 40001 | 请求参数错误 |
| 40002 | 业务逻辑错误 |
| 40100 | 未认证 |
| 40101 | Token 过期 |
| 40300 | 无权限 |
| 40400 | 资源不存在 |
| 50000 | 服务器内部错误 |

---

## 2. 认证相关 API

### 2.1 OAuth 回调
```
GET /api/auth/callback
POST /api/auth/callback
```
**说明**: IAM OAuth 授权回调接口

### 2.2 登录
```
POST /api/auth/login
```
**请求体**:
```json
{
  "code": "authorization_code",
  "redirect_uri": "http://..."
}
```

### 2.3 刷新 Token
```
POST /api/auth/refresh
```
**请求体**:
```json
{
  "refresh_token": "xxx"
}
```

---

## 2.4 冷启动 (Setup) API

### 2.4.1 获取系统初始化状态

```
GET /api/setup/status
```

**说明**: 检查系统是否需要初始化（User 表是否为空），无需认证

**响应**:
```json
{
  "code": 20000,
  "data": {
    "requires_setup": true,
    "user_count": 0
  }
}
```

### 2.4.2 初始化系统

```
POST /api/setup/init
```

**说明**: 创建系统第一个管理员账户，无需认证，仅 User 表为空时可调用

**请求体**:
```json
{
  "email": "admin@example.com",
  "password": "secure_password"
}
```

**响应**:
```json
{
  "code": 20100,
  "data": {
    "user_id": "uuid",
    "oidc_url": "https://iam.example.com/oauth/authorize?..."
  }
}
```

**错误码**:
- `40300`: 系统已初始化，禁止重复操作
- `40001`: 请求参数错误（邮箱格式、密码强度）

---

## 2.5 商户管理 API

**权限**: 仅 `project_admin` 角色可访问

### 2.5.1 获取商户列表

```
GET /api/merchants
```

**查询参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| page | int | 页码，默认 1 |
| pageSize | int | 每页数量，默认 20 |
| status | string | 状态筛选 (active/inactive) |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "uuid",
        "name": "北京旗舰店",
        "code": "beijing-flagship",
        "contact_name": "张三",
        "contact_email": "zhangsan@example.com",
        "contact_phone": "13800000000",
        "admin_uid": "user-uuid",
        "status": "active",
        "created_at": "2024-01-15T10:00:00Z"
      }
    ],
    "total": 1,
    "page": 1,
    "pageSize": 20
  }
}
```

### 2.5.2 获取商户详情

```
GET /api/merchants/:id
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "id": "uuid",
    "name": "北京旗舰店",
    "code": "beijing-flagship",
    "contact_name": "张三",
    "contact_email": "zhangsan@example.com",
    "contact_phone": "13800000000",
    "admin_uid": "user-uuid",
    "status": "active",
    "site_count": 5,
    "active_orders": 12,
    "created_at": "2024-01-15T10:00:00Z"
  }
}
```

### 2.5.3 创建商户

```
POST /api/merchants
```

**请求体**:
```json
{
  "name": "北京旗舰店",
  "code": "beijing-flagship",
  "contact_name": "张三",
  "contact_email": "zhangsan@example.com",
  "contact_phone": "13800000000",
  "admin_uid": "user-uuid"
}
```

**说明**:
1. 调用 IAM 创建 Organization（name = merchant.name, code = merchant.code）
2. 调用 IAM 将 admin_uid 关联至该组织并赋予"组织管理员"角色
3. 本地商户表记录信息

**响应**:
```json
{
  "code": 20100,
  "data": {
    "id": "uuid",
    "name": "北京旗舰店",
    "iam_org_id": "iam-org-uuid"
  }
}
```

**错误码**:
- `40002`: 商户代码已存在
- `40001`: admin_uid 用户不存在
- `40300`: 无权限创建商户

### 2.5.4 更新商户

```
PUT /api/merchants/:id
```

**请求体**:
```json
{
  "name": "北京旗舰店新名称",
  "contact_name": "李四",
  "contact_email": "lisi@example.com",
  "contact_phone": "13900000000"
}
```

**说明**: code 和 admin_uid 不可修改

### 2.5.5 删除商户

```
DELETE /api/merchants/:id
```

**说明**: 安全删除，前置检查：
- 该商户下无 active 状态的网点
- 该商户下无未结清的订单（status = 'paid' 或 'in_lease'）

**成功响应**:
```json
{
  "code": 20000,
  "message": "商户已删除"
}
```

**错误码**:
- `40002`: 商户下有活跃网点或未完成订单
- `40300`: 无权限删除商户

---

## 2.6 网点成员管理 API

### 2.6.1 获取网点成员列表

```
GET /api/sites/:id/members
```

**说明**: 获取指定网点的所有成员

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "user_id": "uuid",
        "user_name": "张三",
        "user_email": "zhangsan@example.com",
        "role": "Manager",
        "joined_at": "2024-01-15T10:00:00Z"
      },
      {
        "user_id": "uuid2",
        "user_name": "李四",
        "user_email": "lisi@example.com",
        "role": "Staff",
        "joined_at": "2024-01-16T11:00:00Z"
      }
    ],
    "total": 2
  }
}
```

### 2.6.2 添加网点成员

```
POST /api/sites/:id/members
```

**请求体**:
```json
{
  "user_id": "user-uuid",
  "role": "Staff"  // 可选，默认为 Staff
}
```

**说明**: 使用「指定用户对话框」获取 user_id

**响应**:
```json
{
  "code": 20100,
  "data": {
    "site_id": "site-uuid",
    "user_id": "user-uuid",
    "role": "Staff"
  }
}
```

**错误码**:
- `40002`: 该用户已是网点成员
- `40001`: user_id 为空或无效

### 2.6.3 更新成员角色

```
PUT /api/sites/:id/members/:user_id
```

**请求体**:
```json
{
  "role": "Manager"  // Manager 或 Staff
}
```

**说明**: 切换角色时检查保护规则

**保护规则**:
- 若目标用户是当前网点最后一名 Manager，禁止将其改为 Staff
- 检查方法: 查询 `site_members` 表中 `site_id = :id AND role = 'Manager'` 的数量
  - 若数量为 1 且目标用户 role = 'Manager' → 拒绝操作

**响应**:
```json
{
  "code": 20000,
  "data": {
    "site_id": "site-uuid",
    "user_id": "user-uuid",
    "new_role": "Manager"
  }
}
```

**错误码**:
- `40002`: 最后一名管理员不可修改角色
- `40400`: 成员不存在

### 2.6.4 移除网点成员

```
DELETE /api/sites/:id/members/:user_id
```

**说明**: 移除成员，保护规则同上

**保护规则**:
- 最后一名 Manager 不可移除
- 移除后 `site_members` 表中对应记录被删除（物理删除或标记为 inactive）

**响应**:
```json
{
  "code": 20000,
  "message": "成员已移除"
}
```

**错误码**:
- `40002`: 最后一名管理员不可移除
- `40400`: 成员不存在

---

## 3. 乐器管理 API

### 3.1 获取乐器列表
```
GET /api/instruments
```
**查询参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| page | int | 页码，默认 1 |
| pageSize | int | 每页数量，默认 20 |
| category | string | 分类 ID |
| status | string | 状态 (available/rented/maintenance) |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [...],
    "total": 100,
    "page": 1,
    "pageSize": 20
  }
}
```

### 3.2 获取乐器详情
```
GET /api/instruments/:id
```

### 3.3 创建乐器 (需要 OWNER 角色)
```
POST /api/instruments
```

**重要变更** (2026-04-16):
- 乐器不再有 `name`、`brand`、`model` 字段
- 品牌、型号等属性通过 `properties` 动态字段传递

**请求体**:
```json
{
  "sn": "SN123456",
  "category_id": "uuid",
  "site_id": "uuid",
  "level_id": "uuid",
  "description": "描述",
  "images": ["url1", "url2"],
  "video": "url",
  "properties": {
    "品牌": "Yamaha",
    "型号": "U1",
    "颜色": ["黑色", "白色"],
    "年份": "2020"
  }
}
```

### 3.4 更新乐器
```
PUT /api/instruments/:id
```

### 3.5 检查识别码唯一性
```
GET /api/instruments/check?sn=xxx
```

### 3.6 获取乐器分级
```
GET /api/instruments/levels
```

### 3.7 获取乐器定价
```
GET /api/instruments/:id/pricing
```

### 3.8 更新乐器状态
```
PUT /api/instruments/:id/status
```
**请求体**:
```json
{
  "status": "available"
}
```

---

## 4. 分类管理 API

### 4.1 获取分类列表
```
GET /api/categories
```

### 4.2 获取分类详情
```
GET /api/categories/:id
```

### 4.3 创建分类
```
POST /api/categories
```
**请求体**:
```json
{
  "name": "钢琴",
  "icon": "🎹",
  "parent_id": null,
  "visible": true
}
```

### 4.4 更新分类
```
PUT /api/categories/:id
```

### 4.5 删除分类
```
DELETE /api/categories/:id
```

### 4.6 获取子分类
```
GET /api/categories/:id/children
```

### 4.7 批量更新分类排序
```
PUT /api/categories/sort
```
**请求体**:
```json
{
  "items": [
    {"id": "uuid1", "sort": 1},
    {"id": "uuid2", "sort": 2}
  ]
}
```

---

## 5. 订单/租赁 API

### 5.1 预览订单
```
POST /api/orders/preview
```
**请求体**:
```json
{
  "instrument_id": "uuid",
  "level": "standard",
  "lease_term": 3,
  "deposit_mode": "standard"
}
```

### 5.2 创建订单
```
POST /api/orders
```

### 5.3 获取订单列表
```
GET /api/orders
```
**查询参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| status | string | pending/paid/in_lease/completed/cancelled |
| page | int | 页码 |
| pageSize | int | 每页数量 |

### 5.4 获取订单详情
```
GET /api/orders/:id
```

### 5.5 支付订单
```
POST /api/orders/:id/pay
```

### 5.6 取货确认
```
POST /api/orders/:id/pickup
```

### 5.7 归还确认
```
POST /api/orders/:id/return
```

### 5.8 取消订单
```
POST /api/orders/:id/cancel
```

### 5.9 获取逾期租赁
```
GET /api/overdue-leases
```

### 5.10 转移所有权
```
POST /api/orders/:id/transfer-ownership
```

### 5.11 终止订单
```
PUT /api/orders/:id/terminate
```

---

## 6. 维修工单 API

### 6.1 提交维修申请
```
POST /api/maintenance
```
**请求体**:
```json
{
  "order_id": "uuid",
  "instrument_id": "uuid",
  "problem_description": "问题描述",
  "images": ["url1"],
  "service_type": "repair"
}
```

### 6.2 报修
```
POST /api/maintenance/report
```

### 6.3 获取维修详情
```
GET /api/maintenance/:id
```

### 6.4 取消维修
```
PUT /api/maintenance/:id/cancel
```

### 6.5 商家列表
```
GET /api/merchant/maintenance
```

### 6.6 商家受理
```
PUT /api/merchant/maintenance/:id/accept
```

### 6.7 分配技师
```
PUT /api/merchant/maintenance/:id/assign
```

### 6.8 更新进度
```
PUT /api/merchant/maintenance/:id/update
```

### 6.9 发送报价
```
POST /api/merchant/maintenance/:id/quote
```

### 6.10 技师工单列表
```
GET /api/technician/tickets
```

### 6.11 技师接单
```
PUT /api/technician/tickets/:id/accept
```

### 6.12 技师完工
```
POST /api/technician/tickets/:id/complete
```

### 6.13 创建维修师傅账户
```
POST /api/maintenance/workers
```
**请求体**:
```json
{
  "name": "张三",
  "phone": "13800000000"
}
```

### 6.14 获取维修师傅列表
```
GET /api/maintenance/workers
```
**查询参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 姓名筛选 |
| phone | string | 电话筛选 |
| site_id | string | 网点筛选 |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "uuid",
        "name": "张三",
        "phone": "13800000000",
        "join_date": "2024-01-01",
        "recent_orders": 15,
        "current_orders": 3
      }
    ]
  }
}
```

### 6.15 获取师傅详情
```
GET /api/maintenance/workers/:id
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "id": "uuid",
    "name": "张三",
    "phone": "13800000000",
    "join_date": "2024-01-01",
    "recent_orders": 15,
    "current_orders": 3,
    "history": [
      {
        "id": "uuid",
        "date": "2024-01-15",
        "category": "钢琴",
        "status": "已完成"
      }
    ]
  }
}
```

### 6.16 删除维修师傅账户
```
DELETE /api/maintenance/workers/:id
```

### 6.17 更新维修会话状态
```
PUT /api/maintenance/sessions/:id/status
```
**请求体**:
```json
{
  "status": "in_progress",
  "comment": "开始工作",
  "photos": ["url1", "url2"]
}
```

**状态值说明**:
- `assigned`: 已分配
- `in_progress`: 维修中
- `completed`: 验收中
- `passed`: 验收通过（在库）
- `failed`: 验收不通过（待处理）

### 6.18 扫码开始工作
```
POST /api/maintenance/sessions/:id/start-work
```
**请求体**:
```json
{
  "instrument_sn": "SN123456",
  "scan_time": "2024-01-15T10:00:00Z"
}
```

### 6.19 提交维修记录
```
POST /api/maintenance/sessions/:id/records
```
**请求体**:
```json
{
  "type": "comment",
  "content": "更换琴弦",
  "photos": ["url1"]
}
```

### 6.20 验收处理
```
PUT /api/maintenance/sessions/:id/inspect
```
**请求体**:
```json
{
  "result": "passed",  // or "failed"
  "comment": "验收通过",
  "photos": ["url1", "url2"]
}
```

---

## 7. 申诉处理 API

### 7.1 获取申诉列表
```
GET /api/merchant/appeals
```
**查询参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| status | string | 申诉状态 (pending/reviewing/resolved) |
| site_id | string | 网点 ID |
| page | int | 页码 |
| pageSize | int | 每页数量 |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "uuid",
        "instrument": {
          "id": "uuid",
          "category": "钢琴",
          "level": "专业级",
          "brand": "Yamaha",
          "model": "U1"
        },
        "damage_report": {
          "amount": 500.00,
          "comment": "琴弦断裂",
          "photos": ["url1"]
        },
        "user_appeal": {
          "reason": "琴弦是自然老化",
          "submitted_at": "2024-01-15T10:00:00Z"
        },
        "status": "reviewing",
        "created_at": "2024-01-15T10:00:00Z"
      }
    ],
    "total": 1,
    "page": 1,
    "pageSize": 20
  }
}
```

### 7.2 获取申诉详情
```
GET /api/merchant/appeals/:id
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "id": "uuid",
    "instrument": {...},
    "lease_info": {
      "rental_period": "2024-01-01 至 2024-01-31",
      "total_rent": 2500.00
    },
    "damage_report": {...},
    "user_appeal": {...},
    "employee_info": {
      "name": "李四",
      "damage_assessment": "用户操作不当"
    },
    "status": "reviewing"
  }
}
```

### 7.3 处理申诉
```
PUT /api/merchant/appeals/:id/resolve
```
**请求体**:
```json
{
  "decision": "adjust",  // no_damage, adjust, confirm
  "adjust_amount": 200.00,  // 仅在 decision=adjust 时有效
  "comment": "经理判定琴弦为自然老化"
}
```

**decision 说明**:
- `no_damage`: 无损坏，取消赔款，直接生成退还事务，乐器在库状态
- `adjust`: 调整定损金额
- `confirm`: 确认原判（不调整）

**响应**:
```json
{
  "code": 20000,
  "message": "success",
  "data": {
    "refund_deposit": 5000.00,
    "status": "resolved"
  }
}
```

### 7.4 用户提交申诉
```
POST /api/user/appeals
```
**请求体**:
```json
{
  "damage_report_id": "uuid",
  "reason": "申诉理由",
  "evidence": ["url1", "url2"]
}
```

### 7.5 用户同意定损
```
POST /api/user/appeals/:damage_id/agree
```

**响应**:
```json
{
  "code": 20000,
  "message": "success",
  "data": {
    "payment_url": "https://pay.example.com/123"  // 仅押金不足时返回
  }
}
```

---

## 8. 库管工作台 API

### 8.1 获取订单列表
```
GET /api/warehouse/orders
```
**查询参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| status | string | 订单状态 (preparing/shipped/in_lease/returning) |
| site_id | string | 网点 ID |
| page | int | 页码 |
| pageSize | int | 每页数量 |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "uuid",
        "instrument": {...},
        "user": {...},
        "status": "shipped",
        "shipping_info": {
          "tracking_number": "SF123456",
          "company": "顺丰",
          "shipped_at": "2024-01-15T10:00:00Z"
        }
      }
    ],
    "total": 10
  }
}
```

### 8.2 录入物流信息
```
PUT /api/warehouse/orders/:id/shipping
```
**请求体**:
```json
{
  "tracking_number": "SF123456",
  "company": "顺丰",
  "shipped_at": "2024-01-15T10:00:00Z"
}
```

**响应**:
```json
{
  "code": 20000,
  "message": "success",
  "data": {
    "order_id": "uuid",
    "status": "shipped"
  }
}
```

### 8.3 确认收货（租赁中）
```
PUT /api/warehouse/orders/:id/delivered
```
**请求体**:
```json
{
  "delivered_at": "2024-01-16T15:00:00Z"
}
```

**说明**: 确认收货后订单状态变为 in_lease，以物流到达时间点为起租点

### 8.4 归还验收
```
POST /api/warehouse/orders/:id/inspect
```
**请求体**:
```json
{
  "instrument_sn": "SN123456",
  "scan_time": "2024-01-31T10:00:00Z",
  "photos": ["url1", "url2"],
  "condition": "good",
  "notes": "外观完好"
}
```

**condition 说明**:
- `good`: 正常，直接进入在库状态
- `damaged`: 损坏，进入定损流程

**响应**:
```json
{
  "code": 20000,
  "message": "success",
  "data": {
    "status": "completed"  // 或 "inspecting"
  }
}
```

### 8.5 开始定损
```
POST /api/warehouse/orders/:id/assess-damage
```
**请求体**:
```json
{
  "damage_description": "琴弦断裂",
  "damage_photos": ["url1"],
  "damage_amount": 500.00,
  "notes": "需要更换琴弦"
}
```

**说明**: 提交后订单状态变为 inspecting，创建 damage_report 记录

---

## 9. 用户租赁 API

### 9.1 获取乐器列表（用户端）
```
GET /api/user/instruments
```
**查询参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| category_id | string | 分类筛选 |
| site_id | string | 网点筛选 |
| level | string | 级别筛选 |
| status | string | 状态筛选 (available) |
| sort | string | 排序方式 (price/distance/rating) |
| page | int | 页码 |
| pageSize | int | 每页数量 |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "uuid",
        "sn": "SN123456",
        "brand": "Yamaha",
        "model": "U1",
        "category": "钢琴",
        "level": "专业级",
        "images": ["url1", "url2"],
        "daily_rent": 100.00,
        "site_id": "uuid",
        "site_name": "北京总店",
        "status": "available"
      }
    ],
    "total": 100,
    "page": 1,
    "pageSize": 20
  }
}
```

### 9.2 获取乐器详情（用户端）
```
GET /api/user/instruments/:id
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "id": "uuid",
    "brand": "Yamaha",
    "model": "U1",
    "description": "描述信息",
    "category": "钢琴",
    "level": "专业级",
    "images": ["url1"],
    "pricing": [
      {
        "name": "standard",
        "daily_rent": 100.00,
        "weekly_rent": 630.00,
        "monthly_rent": 2400.00,
        "deposit": 5000.00
      }
    ],
    "site_id": "uuid",
    "site_name": "北京总店",
    "stock": 5
  }
}
```

### 9.3 创建租赁订单
```
POST /api/user/orders
```
**请求体**:
```json
{
  "instrument_id": "uuid",
  "start_date": "2024-02-01",
  "end_date": "2024-02-15",
  "delivery_address": {
    "name": "张三",
    "phone": "13800000000",
    "address": "北京市朝阳区..."
  },
  "notes": "备注"
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "order_id": "uuid",
    "amount": 1500.00,  // 租金总额
    "deposit": 5000.00,
    "payment_url": "https://pay.example.com/123"
  }
}
```

### 9.4 获取我的租赁列表
```
GET /api/user/rentals
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "uuid",
        "instrument": {...},
        "start_date": "2024-02-01",
        "end_date": "2024-02-15",
        "status": "active",
        "days_remaining": 3
      }
    ]
  }
}
```

### 9.5 发起归还
```
POST /api/user/rentals/:id/return
```
**请求体**:
```json
{
  "return_method": "logistics",
  "logistics_company": "顺丰",
  "tracking_number": "SF987654"
}
```

**响应**:
```json
{
  "code": 20000,
  "message": "归还已提交，请等待验收",
  "data": {
    "status": "returning"
  }
}
```

### 9.6 获取电子合同
```
GET /api/user/contracts/:id
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "id": "uuid",
    "order_id": "uuid",
    "contract_url": "https://cdn.example.com/contract_123.pdf",
    "created_at": "2024-02-01T10:00:00Z"
  }
}
```

**说明**: 合同在支付完成后自动生成

---

## 10. 库存管理 API

### 7.1 获取库存列表
```
GET /api/merchant/inventory
```

### 7.2 调拨申请
```
POST /api/merchant/inventory/transfer
```
**请求体**:
```json
{
  "asset_id": "uuid",
  "from_site_id": "uuid",
  "to_site_id": "uuid",
  "reason": "调拨原因"
}
```

### 7.3 调拨记录
```
GET /api/merchant/inventory/transfers
```

### 7.4 获取库存租金列表
```
GET /api/inventory/rent-setting
```
**查询参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| brand | string | 品牌筛选 |
| model | string | 型号筛选 |
| category_id | string | 分类 ID 筛选 |
| level_id | string | 等级 ID 筛选 |
| site_id | string | 网点 ID 筛选 |
| page | int | 页码，默认 1 |
| pageSize | int | 每页数量，默认 20 |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "uuid",
        "sn": "SN123456",
        "category_name": "钢琴",
        "level_name": "专业级",
        "brand": "Yamaha",
        "model": "U1",
        "site_name": "北京总店",
        "daily_rent": 100.00
      }
    ],
    "total": 100,
    "page": 1,
    "pageSize": 20
  }
}
```

### 7.5 批量更新租金
```
PUT /api/inventory/rent-setting/batch
```
**请求体**:
```json
{
  "items": [
    {"id": "uuid1", "daily_rent": 120.00},
    {"id": "uuid2", "daily_rent": 150.00}
  ]
}
```
**响应**:
```json
{
  "code": 20000,
  "message": "success",
  "data": {"updated": 2}
}
```

---

## 8. 网点管理 API

### 8.1 获取网点列表
```
GET /api/common/sites
```

### 8.2 获取附近网点
```
GET /api/common/sites/nearby?lat=xx&lng=xx
```

### 8.3 获取网点详情
```
GET /api/common/sites/:id
```

### 8.4 创建网点
```
POST /api/merchant/sites
```

### 8.5 更新网点
```
PUT /api/merchant/sites/:id
```

### 8.6 删除网点
```
DELETE /api/merchant/sites/:id
```

### 8.7 获取网点树
```
GET /api/sites/tree
GET /api/sites/tree?root=:id
```

---

## 9. 属性管理 API

### 9.1 获取属性列表
```
GET /api/properties
```

### 9.2 创建属性
```
POST /api/property
```
**请求体**:
```json
{
  "name": "颜色",
  "property_type": "select",
  "is_required": true,
  "unit": ""
}
```

### 9.3 更新属性
```
PUT /api/property/:id
```

### 9.4 创建属性选项
```
POST /api/property/option
```

### 9.5 确认属性值
```
PUT /api/property/confirm
```

### 9.6 合并属性值
```
PUT /api/property/merge
```

---

## 10. 租赁管理 API

### 10.1 获取租赁列表
```
GET /api/merchant/leases
```

### 10.2 获取租赁详情
```
GET /api/merchant/leases/:id
```

### 10.3 创建租赁
```
POST /api/merchant/leases
```

### 10.4 更新租赁
```
PUT /api/merchant/leases/:id
```

### 10.5 终止租赁
```
DELETE /api/merchant/leases/:id
```

---

## 11. 押金管理 API

### 11.1 获取押金列表
```
GET /api/merchant/deposits
```

### 11.2 创建押金
```
POST /api/merchant/deposits
```

### 11.3 更新押金
```
PUT /api/merchant/deposits/:id
```

---

## 12. 标签管理 API

### 12.1 获取标签列表
```
GET /api/labels
```

### 12.2 创建标签
```
POST /api/labels
```

### 12.3 审核通过
```
PUT /api/labels/:id/approve
```

### 12.4 审核拒绝
```
PUT /api/labels/:id/reject
```

### 12.5 合并标签
```
POST /api/labels/merge
```

---

## 13. 所有权管理 API

### 13.1 获取所有权信息
```
GET /api/user/ownership/:id
```

### 13.2 下载所有权证书
```
GET /api/user/ownership/:id/download
```

---

## 14. 评估报告 API

### 14.1 获取评估数据
```
GET /api/orders/:id/assessment
```

### 14.2 提交评估
```
POST /api/orders/:id/assessment
```

### 14.3 生成评估报告
```
GET /api/reports/assessment/:order_id
```

---

## 15. 出库确认 API

### 15.1 获取出库照片
```
GET /api/orders/:id/outbound-photos
```

### 15.2 确认出库
```
POST /api/orders/:id/outbound-confirm
```

---

## 16. 权限管理 API

### 16.1 获取权限列表
```
GET /api/admin/permissions
```

### 16.2 获取角色列表
```
GET /api/admin/roles
```

### 16.3 获取角色权限
```
GET /api/admin/roles/:id/permissions
```

### 16.4 更新角色权限
```
PUT /api/admin/roles/:id/permissions
```

### 16.5 创建角色
```
POST /api/admin/roles
```

### 16.6 删除角色
```
DELETE /api/admin/roles/:id
```

---

## 17. 系统管理 API

### 17.1 获取客户端列表
```
GET /api/system/clients
```

### 17.2 获取租户列表
```
GET /api/system/tenants
```

---

## 18. IAM 代理 API

### 18.1 查询用户
```
GET /api/iam/users/lookup
```

### 18.2 创建用户
```
POST /api/iam/users
```

---

## 19. 仪表盘 API

### 19.1 获取统计数据
```
GET /api/admin/dashboard/stats
```

### 19.2 获取即将到期列表
```
GET /api/admin/dashboard/near-transfers
```

---

## 20. 导入导出 API

### 20.1 导入乐器
```
POST /api/instruments/import
```

### 20.2 导出乐器
```
GET /api/instruments/export
```

### 20.3 下载导入模板
```
GET /api/instruments/import/template
```

### 20.4 批量导入预览
```
POST /api/instruments/batch-import/preview
```

**Request (multipart/form-data)**:
- `file`: CSV file (required)
- `site_id`: UUID (optional, for site-scope import)

**Response**:
```json
{
  "code": 20000,
  "data": {
    "total_rows": 50,
    "valid_rows": 48,
    "errors": [
      { "row": 5, "sn": "SN-005", "error": "Duplicate SN in database" },
      { "row": 12, "sn": "", "error": "Missing required field: sn" }
    ],
    "preview": [...]
  }
}
```

**Error Response**:
```json
{ "code": 40003, "message": "Invalid CSV format" }
```

### 20.5 执行批量导入
```
POST /api/instruments/batch-import
```

**Request (multipart/form-data)**:
- `file`: CSV file (required)
- `session_id`: UUID (optional, for ZIP media import)
- `site_id`: UUID (optional, for site-scope import)

**Response**:
```json
{
  "code": 20000,
  "data": {
    "total": 50,
    "success": 48,
    "failed": 2,
    "failed_details": [
      { "sn": "SN-005", "error": "Duplicate SN in database" }
    ]
  }
}
```

---

## 21. 文件上传 API

### 21.1 上传文件
```
POST /api/upload
```
**说明**: 支持图片、视频等文件上传

---

## 附录 A: 角色权限说明

| 角色 | 说明 |
|------|------|
| ADMIN | 管理员 |
| OWNER | 所有者 |
| USER | 普通用户 |

---

## 附录 B: 乐器状态说明

| 状态 | 说明 |
|------|------|
| available | 可租 |
| rented | 已租出 |
| maintenance | 维修中 |

---

## 附录 C: 订单状态说明

| 状态 | 说明 |
|------|------|
| pending | 待支付 |
| paid | 已支付 |
| in_lease | 租赁中 |
| completed | 已完成 |
| cancelled | 已取消 |

---

*Model: glm-5*
