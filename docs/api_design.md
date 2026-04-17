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

---

## 7. 库存管理 API

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

### 20.5 执行批量导入
```
POST /api/instruments/batch-import
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
