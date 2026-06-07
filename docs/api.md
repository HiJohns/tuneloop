# TuneLoop API 文档

> 版本: v2.2 (权限管理重构: 10 cus_perm + sys_perm 25-26)
> 最后更新: 2026-05-27
> 覆盖度: 100% features.md

---

## 一、基础规范

### 1.1 统一前缀
所有 API 端点前缀: `/api`

### 1.2 认证方式
- Header: `Authorization: Bearer <JWT_TOKEN>`
- JWT 由 Lin-IAM 统一颁发
- Token 校验中间件: `IAMInterceptor`

### 1.3 统一响应格式

**成功响应:**
```json
{
  "code": 20000,
  "data": {},
  "message": "success"
}
```

**错误响应:**
```json
{
  "code": 40001,
  "message": "错误描述信息"
}
```

### 1.4 分页参数
所有列表接口支持:
- `page`: 页码 (默认: 1)
- `pageSize`: 每页数量 (默认: 20, 最大: 100)

### 1.5 权限模型 (v2.1)

权限控制基于 JWT 中的两层位图。

> 完整 sys_perm 位码表见 [`docs/permissions.md` §二](./permissions.md#二sys_perm-系统权限位码表)
> 完整 cus_perm 业务权限列表见 [`docs/permissions.md` §三](./permissions.md#三cus_perm-业务权限表)
> 角色-权限矩阵见 [`docs/permissions.md` §四](./permissions.md#四角色-权限分配矩阵)

**API 权限要求汇总**：

| 端点分类 | 权限类型 | 示例权限 |
|---------|---------|---------|
| 认证（登录/回调/刷新） | 无 | — |
| 乐器查看/分类查看 | 已登录 | — |
| 乐器创建/编辑/删除 | cus_perm | instrument:create/update/delete |
| 分类配置 | cus_perm | instrument:update |
| 属性管理 | cus_perm | instrument:update |
| 库存查看 | cus_perm | instrument:read |
| 库存调拨 | cus_perm | instrument:update |
| 租金设定 | cus_perm | instrument:price |
| 订单创建 | 已登录 | — |
| 订单/租约管理 | cus_perm | order:create/read/update/cancel |
| 维修提交 | 已登录 | — |
| 维修管理 | cus_perm | instrument:maintain |
| 商户管理 | sys_perm | tenant_* (bits 5-9) |
| 商户创建 | sys_perm | tenant:create (bit 25, 命名空间管理员仅) |
| 网点管理 | sys_perm | organization_* (bits 10-14) |
| 人员管理 | sys_perm | user_* (bits 15-19) |
| IAM 同步 | sys_perm | organization_create / user_create |
| 权限管理 | sys_perm | permission:manage (bit 26) |
| 客户端管理 | sys_perm | namespace_* (bits 0-4) |
| 定价管理 | cus_perm | instrument:price |

---

## 二、认证与授权模块

### 2.1 IAM 回跳处理
> **说明**: 业务系统不直接处理微信登录，引导用户跳转 IAM 登录页

**接口**: `GET /api/auth/callback`

**请求参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| code | string | IAM 授权码 (必填) |
| state | string | 随机状态值 (防 CSRF) |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "access_token": "eyJhbGc...",
    "refresh_token": "eyJhbGc...",
    "expires_in": 2592000,
    "token_type": "Bearer"
  }
}
```

**流程说明**:
1. 前端重定向至 IAM 登录页: `https://iam.example.com/oauth/authorize?client_id=xxx`
2. 用户登录后，IAM 回调业务系统: `/api/auth/callback?code=xxx&state=xxx`
3. 业务系统用 code 换取 JWT token

---

### 2.2 Token 刷新

**接口**: `POST /api/auth/refresh`

**请求 Body**:
```json
{
  "refresh_token": "eyJhbGc..."
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "access_token": "eyJhbGc...",
    "refresh_token": "eyJhbGc...",
    "expires_in": 2592000
  }
}
```

---

### 2.3 用户信息

**接口**: `GET /api/auth/profile`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "sub": "uuid-user-id",
    "tenant_id": "tenant-001",
    "org_id": "org-001",
    "phone": "138****8888",
    "email": "user@example.com",
    "credit_score": 750,
    "deposit_mode": "free" // free | standard
  }
}
```

---

### 2.4 影子用户同步

**接口**: `POST /api/auth/sync-user`

**说明**: Hook 接口，用户首次登录时从 JWT 创建本地业务用户记录

**请求 Body**:
```json
{
  "sub": "uuid-user-id",
  "tenant_id": "tenant-001",
  "org_id": "org-001",
  "phone": "13800000000"
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "user_id": "user-001",
    "is_created": true
  }
}
```

---

### 2.5 IAM 代理接口

> **说明**: 业务系统与 Lin-IAM 的代理层，支持 JIT 用户创建

#### 2.5.1 查询 IAM 用户

**接口**: `GET /api/iam/users/lookup`

**请求参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| query | string | 邮箱或手机号查询 |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "user_id": "user-001",
    "email": "manager@example.com",
    "phone": "13800000000",
    "role": "site_manager",
    "status": "active",
    "org_id": "org-001"
  }
}
```

#### 2.5.2 创建 IAM 用户 (JIT)

**接口**: `POST /api/iam/users`

**说明**: Just-In-Time 用户创建，用于网点负责人注册

**请求 Body**:
```json
{
  "email": "newmanager@example.com",
  "phone": "13800000000",
  "role": "site_manager",
  "org_id": "org-001"
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "user_id": "user-002",
    "status": "pending",
    "message": "User creation in progress"
  }
}
```

#### 2.5.3 同步 IAM 组织

**接口**: `POST /api/iam/organizations/sync`

**说明**: 手动触发从 IAM 同步组织列表到本地 sites 表

**权限**: ADMIN, OWNER

**请求 Body**: 无

**响应**:
```json
{
  "code": 20000,
  "data": {
    "synced": 3,
    "skipped": 1,
    "conflicts": 0
  },
  "message": "success"
}
```

**字段说明**:
- `synced`: 成功同步的组织数量
- `skipped`: 跳过的组织数量（已存在且一致）
- `conflicts`: 发生冲突的数量（IAM 与本地数据不一致，IAM 数据优先）

---

#### 2.5.4 同步 IAM 用户

**接口**: `POST /api/iam/users/sync`

**说明**: 手动触发从 IAM 同步用户列表到本地 users 表，包括 name、email、phone、role、org_id、status

**权限**: ADMIN, OWNER

**请求 Body**: 无

**响应**:
```json
{
  "code": 20000,
  "data": {
    "synced": 5,
    "skipped": 2,
    "conflicts": 0
  },
  "message": "success"
}
```

**字段说明**:
- `synced`: 成功同步的用户数量
- `skipped`: 跳过的用户数量（已存在且一致）
- `conflicts`: 发生冲突的数量（IAM 与本地数据不一致，IAM 数据优先）

---

## 三、白标化配置模块

### 3.1 品牌配置

**接口**: `GET /api/common/brand-config`

**请求参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| client_id | string | IAM 客户端 ID |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "primary_color": "#6366F1",
    "logo_url": "https://cdn.example.com/logo.png",
    "brand_name": "TuneLoop",
    "support_phone": "400-123-4567"
  }
}
```

---

## 三、公共浏览 API（无需认证）

### 3.1 乐器列表

**接口**: `GET /api/public/instruments`

**查询参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| page | int | 页码 (默认: 1) |
| pageSize | int | 每页数量 (默认: 20, 最大: 100) |
| category_id | string | 分类 ID (可选) |
| site_id | string | 网点 ID (可选) |
| level_id | string | 级别 ID (可选) |
| tenant | string | 租户 ID (可选, 不传则返回所有租户) |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "uuid",
        "name": "乐器名称",
        "brand": "品牌",
        "model": "型号",
        "category_id": "cat-01",
        "category_name": "分类名",
        "level_name": "级别名",
        "images": ["url1", "url2"],
        "pricing": {},
        "stock_status": "available", // available/reserved/shipping/rented/returning/maintenance/expired/archived
        "tenant_id": "uuid",
        "site_id": "uuid",
        "site_name": "网点名",
        "description": "描述"
      }
    ],
    "total": 100,
    "page": 1,
    "pageSize": 20
  }
}
```

### 3.2 乐器详情

**接口**: `GET /api/public/instruments/:id`

**响应**: 同上单条乐器数据, 包含 `tenant_id` 用于购物车按租户分组

### 3.3 分类列表

**接口**: `GET /api/public/categories`

### 3.4 网点列表

**接口**: `GET /api/public/sites`

---

## 四、网点与 LBS 模块

### 4.1 网点列表

**接口**: `GET /api/common/sites`

**请求参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| city | string | 城市编码 |
| status | string | 状态: active, closed |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "site-001",
        "name": "北京朝阳店",
        "address": "北京市朝阳区xxx路123号",
        "latitude": 39.9042,
        "longitude": 116.4074,
        "phone": "010-12345678",
        "business_hours": "09:00-21:00",
        "status": "active"
      }
    ],
    "total": 15
  }
}
```

---

### 4.2 附近网点

**接口**: `GET /api/common/sites/nearby`

**请求参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| lat | float | 纬度 (必填) |
| lng | float | 经度 (必填) |
| radius | int | 搜索半径 (米, 默认 5000) |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "site-001",
        "name": "北京朝阳店",
        "address": "北京市朝阳区xxx路123号",
        "distance": 1200, // 距离 (米)
        "phone": "010-12345678",
        "stock_status": {
          "piano": 12,
          "violin": 8
        }
      }
    ]
  }
}
```

---

### 4.3 网点详情

**接口**: `GET /api/common/sites/:id`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "id": "site-001",
    "name": "北京朝阳店",
    "address": "北京市朝阳区xxx路123号",
    "latitude": 39.9042,
    "longitude": 116.4074,
    "phone": "010-12345678",
    "business_hours": "09:00-21:00",
    "images": ["image1.jpg", "image2.jpg"],
    "stock_status": {
      "piano": {
        "available": 12,
        "renting": 8,
        "maintenance": 2
      }
    }
  }
}
```

---

### 4.4 网点树结构

**接口**: `GET /api/sites/tree`

**说明**: 获取网点层级树，用于管理端组织架构展示

**响应**:
```json
{
  "code": 20000,
  "data": {
    "tree": [
      {
        "id": "org-001",
        "name": "北京分公司",
        "type": "org",
        "children": [
          {
            "id": "site-001",
            "name": "北京朝阳店",
            "type": "site",
            "manager": "user-001"
          }
        ]
      }
    ]
  }
}
```

### 4.5 网点管理 (商家端)

#### 4.5.1 创建网点

**接口**: `POST /api/merchant/sites`

**请求 Body**:
```json
{
  "name": "北京海淀店",
  "address": "北京市海淀区xxx路456号",
  "latitude": 39.9562,
  "longitude": 116.2987,
  "phone": "010-87654321",
  "business_hours": "09:00-21:00",
  "manager_user_id": "user-002"
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "site_id": "site-002",
    "created_at": "2026-03-22T10:00:00Z"
  }
}
```

#### 4.5.2 更新网点

**接口**: `PUT /api/merchant/sites/:id`

**请求 Body**: 同创建

**响应**: 同创建

#### 4.5.3 删除网点

**接口**: `DELETE /api/merchant/sites/:id`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "deleted": true,
    "deleted_at": "2026-03-22T11:00:00Z"
  }
}
```

---

## 五、乐器租赁模块

### 5.1 乐器分类

**接口**: `GET /api/instruments/categories`

**响应**:
```json
{
  "code": 20000,
  "data": [
    {
      "id": "cat-01",
      "name": "钢琴",
      "icon": "piano.png",
      "children_count": 5
    },
    {
      "id": "cat-02",
      "name": "弦乐器",
      "icon": "string.png",
      "children_count": 8
    }
  ]
}
```

---

### 5.2 二级分类

**接口**: `GET /api/instruments/categories/:parentId/items`

**响应**:
```json
{
  "code": 20000,
  "data": [
    {
      "id": "sub-01",
      "name": "雅马哈立式钢琴",
      "brand": "Yamaha",
      "model": "U1",
      "min_price": 300,
      "stock_count": 15
    }
  ]
}
```

---

### 5.3 乐器列表

**接口**: `GET /api/instruments`

**请求参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| category_id | string | 分类 ID |
| brand | string | 品牌 |
| level | string | 级别: entry, professional, master |
| min_price | int | 最低租金 |
| max_price | int | 最高租金 |
| sort_by | string | 排序: price_asc, price_desc, popular |
| site_id | string | 网点 ID (筛选有库存) |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "instr-001",
        "name": "雅马哈立式钢琴 U1",
        "cover_image": "piano.jpg",
        "level": "professional",
        "level_name": "专业级",
        "monthly_rent": 800,
        "deposit": 5000,
        "stock_status": "available" // available, reserved, shipping, rented, returning, maintenance, expired, archived
      }
    ],
    "total": 120
  }
}
```

---

### 5.4 乐器详情

**接口**: `GET /api/instruments/:id`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "id": "instr-001",
    "name": "雅马哈立式钢琴 U1",
    "brand": "Yamaha",
    "level": "professional",
    "level_name": "专业级",
    "description": "专业级立式钢琴，适合进阶学习...",
    "images": ["img1.jpg", "img2.jpg", "img3.jpg"],
    "video": "intro.mp4",
    "specifications": {
      "material": "实木",
      "size": "121cm",
      "suitable": "进阶学习者"
    },
    "pricing": {
      "monthly_rent": 800,
      "deposit": 5000,
      "discounts": {
        "3_months": 1.0,
        "6_months": 0.98,
        "12_months": 0.95
      }
    },
    "available_sites": [
      {
        "id": "site-001",
        "name": "北京朝阳店",
        "distance": 1200
      }
    ]
  }
}
```

---

### 5.5 阶梯定价方案

**接口**: `GET /api/instruments/:id/pricing`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "entry_level": {
      "monthly_rent": 300,
      "deposit": 2000,
      "service_coverage": ["基础清洁", "免费调音 1 次/年"]
    },
    "professional_level": {
      "monthly_rent": 800,
      "deposit": 5000,
      "service_coverage": ["深度清洁", "免费调音 2 次/年", "免费维修"]
    },
    "master_level": {
      "monthly_rent": 2000,
      "deposit": 10000,
      "service_coverage": ["专家精调", "无限次调音", "免费维修", "上门保养"]
    }
  }
}
```

---

### 5.6 乐器管理扩展

#### 5.6.1 检查乐器 SN 码

**接口**: `GET /api/instruments/check`

**请求参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| sn | string | SN 码 |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "available": true,
    "message": "SN 码可用"
  }
}
```

#### 5.6.2 更新乐器状态

**接口**: `PUT /api/instruments/:id/status`

**请求 Body**:
```json
{
  "status": "maintenance",
  "reason": "用户报修"
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "asset_id": "instr-001",
    "old_status": "renting",
    "new_status": "maintenance",
    "updated_at": "2026-03-22T10:00:00Z"
  }
}
```

#### 5.6.3 下载导入模板

**接口**: `GET /api/instruments/import/template`

**响应**: Excel 文件流

---

### 5.7 Excel批量导入/导出

#### 5.7.1 导入乐器信息

**接口**: `POST /api/instruments/import`

**Content-Type**: `multipart/form-data`

**表单字段**:
- `file`: Excel文件 (.xlsx, .xls)

**请求示例**:
```bash
curl -X POST http://localhost:5554/api/instruments/import \  -H "Authorization: Bearer <JWT_TOKEN>" \  -F "file=@instruments.xlsx"
```

**成功响应 (部分成功)**:
```json
{
  "code": 20000,
  "data": {
    "total": 100,
    "success": 95,
    "failed": 5,
    "errors": [
      {
        "row": 10,
        "error": "Missing required field: name"
      },
      {
        "row": 25,
        "error": "Invalid price format"
      }
    ]
  },
  "message": "Import completed: 95 success, 5 failed (23.5 records/s)"
}
```

**错误响应**:
```json
{
  "code": 40003,
  "message": "Only Excel files (.xlsx, .xls) are supported"
}
```

**Excel模板字段**:
| 字段名 | 中文标题 | 必填 | 说明 |
|--------|----------|------|------|
| name | 乐器名称 | ✅ | 乐器名称 |
| brand | 品牌 | ❌ | 品牌名称 |
| model | 型号 | ❌ | 型号 |
| category_name | 分类名称 | ✅ | 分类名称，支持模糊匹配 |
| level | 级别 | ❌ | enum: entry/pro/master，默认entry |
| daily_rate | 日租金 | ❌ | 数字格式，如: 50 |
| monthly_rate | 月租金 | ❌ | 数字格式 |
| deposit | 押金 | ❌ | 数字格式 |
| stock | 库存数量 | ❌ | 整数，默认0 |
| status | 状态 | ❌ | enum: available/rented/maintenance，默认available |
| description | 描述 | ❌ | 乐器描述 |
| images | 图片URL | ❌ | 支持多个，逗号分隔 |

**业务规则**:
- 支持部分成功导入，每行独立验证
- 重复检测: name + brand + model 组合唯一
- 分类模糊匹配，未找到时自动归类到"未分类"
- 批次提交，每100条提交一次事务

---

#### 5.7.2 导出乐器列表

**接口**: `GET /api/instruments/export`

**查询参数**:
- `category`: 分类筛选 (可选)
- `status`: 状态筛选 (可选)
- `search_text`: 搜索文本，匹配name或brand (可选)
- `fields`: 导出字段，逗号分隔 (可选，默认全部)

**请求示例**:
```bash
curl -X GET "http://localhost:5554/api/instruments/export?category=钢琴&status=available&fields=name,brand,price" \  -H "Authorization: Bearer <JWT_TOKEN>" \  --output instruments.xlsx
```

**成功响应**:
```
HTTP/1.1 200 OK
Content-Type: application/octet-stream
Content-Disposition: attachment; filename="instruments_1234567890.xlsx"

[Binary Excel File Content]
```

**错误响应**:
```json
{
  "code": 40006,
  "message": "Export failed: no instruments found with given filters"
}
```

**可用导出字段**:
- `name` - 乐器名称
- `brand` - 品牌
- `model` - 型号
- `category_name` - 分类名称
- `level` - 级别
- `daily_rate` - 日租金
- `monthly_rate` - 月租金
- `deposit` - 押金
- `stock` - 库存
- `status` - 状态
- `description` - 描述
- `images` - 图片URL

---

#### 5.7.3 下载导入模板

**接口**: `GET /api/instruments/import/template`

**请求示例**:
```bash
curl -X GET http://localhost:5554/api/instruments/import/template \  -H "Authorization: Bearer <JWT_TOKEN>" \  --output instrument_template.xlsx
```

**成功响应**:
```
HTTP/1.1 200 OK
Content-Type: application/octet-stream
Content-Disposition: attachment; filename="instrument_import_template.xlsx"

[Binary Excel File Content]
```

**模板内容**:
- 第1行: 字段标题（红色为必填）
- 第2行: 示例数据
- 第4-8行: 填写说明

**安全特性**:
- Excel公式注入防护: 自动转义以`=`, `+`, `-`, `@`开头的值
- 输入长度限制: 超过1000字符自动截断
- 严格数值验证: 价格字段必须为有效数字\n\n#### 5.6.1 导入乐器信息\n\n**接口**: `POST /api/instruments/import`\n\n**Content-Type**: `multipart/form-data`\n\n**表单字段**:\n- `file`: Excel文件 (.xlsx, .xls)\n\n**请求示例**:\n```bash\ncurl -X POST http://localhost:5554/api/instruments/import \\  -H "Authorization: Bearer <JWT_TOKEN>" \\  -F "file=@instruments.xlsx"\n```\n\n**成功响应 (部分成功)**:\n```json\n{\n  "code": 20000,\n  "data": {\n    "total": 100,\n    "success": 95,\n    "failed": 5,\n    "errors": [\n      {\n        "row": 10,\n        "error": "Missing required field: name"\n      },\n      {\n        "row": 25,\n        "error": "Invalid price format"\n      }\n    ]\n  },\n  "message": "Import completed: 95 success, 5 failed (23.5 records/s)"\n}\n```\n\n**错误响应**:\n```json\n{\n  "code": 40003,\n  "message": "Only Excel files (.xlsx, .xls) are supported"\n}\n```\n\n**Excel模板字段**:\n| 字段名 | 中文标题 | 必填 | 说明 |\n|--------|----------|------|------|\n| name | 乐器名称 | ✅ | 乐器名称 |\n| brand | 品牌 | ❌ | 品牌名称 |\n| model | 型号 | ❌ | 型号 |\n| category_name | 分类名称 | ✅ | 分类名称，支持模糊匹配 |\n| level | 级别 | ❌ | enum: entry/pro/master，默认entry |\n| daily_rate | 日租金 | ❌ | 数字格式，如: 50 |\n| monthly_rate | 月租金 | ❌ | 数字格式 |\n| deposit | 押金 | ❌ | 数字格式 |\n| stock | 库存数量 | ❌ | 整数，默认0 |\n| status | 状态 | ❌ | enum: available/rented/maintenance，默认available |\n| description | 描述 | ❌ | 乐器描述 |\n| images | 图片URL | ❌ | 支持多个，逗号分隔 |\n\n**业务规则**:\n- 支持部分成功导入，每行独立验证\n- 重复检测: name + brand + model 组合唯一\n- 分类模糊匹配，未找到时自动归类到"未分类"\n- 批次提交，每100条提交一次事务\n\n---\n\n#### 5.6.2 导出乐器列表\n\n**接口**: `GET /api/instruments/export`\n\n**查询参数**:\n- `category`: 分类筛选 (可选)\n- `status`: 状态筛选 (可选)\n- `search_text`: 搜索文本，匹配name或brand (可选)\n- `fields`: 导出字段，逗号分隔 (可选，默认全部)\n\n**请求示例**:\n```bash\ncurl -X GET "http://localhost:5554/api/instruments/export?category=钢琴&status=available&fields=name,brand,price" \\  -H "Authorization: Bearer <JWT_TOKEN>" \\  --output instruments.xlsx\n```\n\n**成功响应**:\n```\nHTTP/1.1 200 OK\nContent-Type: application/octet-stream\nContent-Disposition: attachment; filename="instruments_1234567890.xlsx"\n\n[Binary Excel File Content]\n```\n\n**错误响应**:\n```json\n{\n  "code": 40006,\n  "message": "Export failed: no instruments found with given filters"\n}\n```\n\n**可用导出字段**:\n- `name` - 乐器名称\n- `brand` - 品牌\n- `model` - 型号\n- `category_name` - 分类名称\n- `level` - 级别\n- `daily_rate` - 日租金\n- `monthly_rate` - 月租金\n- `deposit` - 押金\n- `stock` - 库存\n- `status` - 状态\n- `description` - 描述\n- `images` - 图片URL\n\n---\n\n#### 5.6.3 下载导入模板\n\n**接口**: `GET /api/instruments/import/template`\n\n**请求示例**:\n```bash\ncurl -X GET http://localhost:5554/api/instruments/import/template \\  -H "Authorization: Bearer <JWT_TOKEN>" \\  --output instrument_template.xlsx\n```\n\n**成功响应**:\n```\nHTTP/1.1 200 OK\nContent-Type: application/octet-stream\nContent-Disposition: attachment; filename="instrument_import_template.xlsx"\n\n[Binary Excel File Content]\n```\n\n**模板内容**:\n- 第1行: 字段标题（红色为必填）\n- 第2行: 示例数据\n- 第4-8行: 填写说明\n\n**安全特性**:\n- Excel公式注入防护: 自动转义以`=`, `+`, `-`, `@`开头的值\n- 输入长度限制: 超过1000字符自动截断\n- 严格数值验证: 价格字段必须为有效数字

---

### 5.8 乐器照片存储 (Deprecated)

> ⚠️ **已废弃**: 此模块已被 §5.9 乐器媒体管理 替代。`POST /api/instruments/:id/photos/upload` 和 `GET /api/instruments/:id/photos/latest` 保留向后兼容，不再新增记录。新功能请使用 §5.9 的接口。

#### 5.8.1 上传乐器照片批次

**接口**: `POST /api/instruments/:id/photos/upload`

**Content-Type**: `multipart/form-data`

**路径参数**:
- `id`: 乐器ID (UUID)

**表单字段**:
- `photos`: 图片文件数组 (支持多个文件)
- `batch_type`: 批次类型 (enum: outbound/return/maintenance)

**认证**: 需要 `Authorization: Bearer <JWT_TOKEN>`

**请求示例**:
```bash
curl -X POST http://localhost:5554/api/instruments/123e4567-e89b-12d3-a456-426614174000/photos/upload \
  -H "Authorization: Bearer <JWT_TOKEN>" \
  -F "batch_type=outbound" \
  -F "photos=@front.jpg" \
  -F "photos=@side.jpg"
```

**成功响应**:
```json
{
  "code": 20000,
  "data": {
    "batch_id": "abc123...",
    "instrument_id": "123e4567-e89b-12d3-a456-426614174000",
    "batch_type": "outbound",
    "storage_path": "/uploads/photos/tenant_abc/SN-12345/batch_20240101_120000.zip",
    "photo_count": 2,
    "created_at": "2024-01-01T12:00:00Z"
  }
}
```

**错误响应**:
```json
{
  "code": 40004,
  "message": "No photos uploaded"
}
```

**存储结构**:
```
uploads/photos/{tenant_id}/{instrument_sn}/batch_{timestamp}/
  ├─ photo1.jpg
  ├─ photo2.jpg
  └─ manifest.yaml
```

**manifest.yaml 内容**:
```yaml
version: "1.0"
batch_id: abc123...
instrument_id: 123e4567-e89b-12d3-a456-426614174000
instrument_sn: SN-12345
batch_type: outbound
operator_id: user_789
tenant_id: tenant_abc
created_at: "2024-01-01T12:00:00Z"
photos:
  - filename: front.jpg
    position: front
    timestamp: "2024-01-01T12:00:01Z"
    size: 2048576
  - filename: side.jpg
    position: side
    timestamp: "2024-01-01T12:00:02Z"
    size: 1872451
```

---

#### 5.8.2 获取最新照片批次

**接口**: `GET /api/instruments/:id/photos/latest`

**路径参数**:
- `id`: 乐器ID (UUID)

**认证**: 需要 `Authorization: Bearer <JWT_TOKEN>`

**请求示例**:
```bash
curl -X GET http://localhost:5554/api/instruments/123e4567-e89b-12d3-a456-426614174000/photos/latest \
  -H "Authorization: Bearer <JWT_TOKEN>"
```

**成功响应**:
```json
{
  "code": 20000,
  "data": {
    "instrument_id": "123e4567-e89b-12d3-a456-426614174000",
    "instrument_sn": "SN-12345",
    "photos": [
      "/uploads/photos/tenant_abc/SN-12345/latest/front.jpg",
      "/uploads/photos/tenant_abc/SN-12345/latest/side.jpg"
    ],
    "count": 2
  }
}
```

**错误响应 (无照片)**:
```json
{
  "code": 40401,
  "message": "No photo batches found for this instrument"
}
```

**存储结构**:
系统将创建 `uploads/photos/{tenant_id}/{instrument_sn}/latest/` 软链接，指向最新的员工拍照批次目录。

---

#### 5.8.3 获取乐器照片批次列表

**接口**: `GET /api/instruments/:id/photos/batches`

**路径参数**:
- `id`: 乐器ID (UUID)

**查询参数**:
- `page`: 页码 (默认: 1)
- `pageSize`: 每页数量 (默认: 20, 最大: 100)
- `batch_type`: 按批次类型筛选 (可选)

**认证**: 需要 `Authorization: Bearer <JWT_TOKEN>`

**请求示例**:
```bash
curl -X GET "http://localhost:5554/api/instruments/123e4567-e89b-12d3-a456-426614174000/photos/batches?page=1&batch_type=outbound" \
  -H "Authorization: Bearer <JWT_TOKEN>"
```

**成功响应**:
```json
{
  "code": 20000,
  "data": {
    "items": [
      {
        "batch_id": "abc123...",
        "batch_type": "outbound",
        "storage_path": "/uploads/photos/tenant_abc/SN-12345/batch_20240101_120000.zip",
        "photo_count": 2,
        "operator_id": "user_789",
        "created_at": "2024-01-01T12:00:00Z"
      },
      {
        "batch_id": "def456...",
        "batch_type": "return",
        "storage_path": "/uploads/photos/tenant_abc/SN-12345/batch_20240115_140000.zip",
        "photo_count": 3,
        "operator_id": "user_012",
        "created_at": "2024-01-15T14:00:00Z"
      }
    ],
    "total": 2,
    "page": 1,
    "pageSize": 20
  }
}
```

**错误响应**:
```json
{
  "code": 40400,
  "message": "Instrument not found"
}
```

---

### 5.9 乐器媒体管理

> 取代 §5.8 照片存储系统，支持图片/视频上传、OSS/本地双模式、按批次管理。

#### 5.9.1 通用文件上传

**接口**: `POST /api/upload`

**Content-Type**: `multipart/form-data`

**表单字段**:
- `file`: 图片或视频文件
- `filename`: 可选，指定文件名（不含扩展名）

**允许的文件类型**: JPEG, PNG, GIF, WebP, MP4, WebM, MOV

**认证**: 需要 `Authorization: Bearer <JWT_TOKEN>`

**成功响应**:
```json
{
  "code": 20000,
  "data": {
    "url": "/uploads/media/1234567890_a1b2c3d4.jpg",
    "file_key": "1234567890_a1b2c3d4.jpg",
    "fileName": "original.jpg",
    "size": 2048576
  }
}
```
`file_key` 为后续绑定到乐器时的唯一标识。

#### 5.9.2 绑定媒体到乐器

**接口**: `POST /api/instruments/:id/media`

**请求 Body**:
```json
{
  "batch_type": "shipping",
  "is_display": true,
  "files": [
    { "file_key": "1234567890_a1b2c3d4.jpg", "file_type": "image", "sort_order": 1 },
    { "file_key": "0987654321_e5f6g7h8.mp4", "file_type": "video", "sort_order": 0 }
  ]
}
```

**batch_type 枚举**: shipping / forwarding / accepting / returning / relaying / receiving / repaired

**行为说明**:
- `is_display=true` 时自动重置同乐器的其他展示批次
- 视频唯一性：后上传的视频自动替换旧视频（删除旧视频 + 缩略图 + DB 记录）
- 视频上传后自动提取缩略图（需容器部署 FFmpeg）

**成功响应**: `{ "code": 20000, "data": { "batch_id": "uuid" } }`

#### 5.9.3 设置展示批次

**接口**: `PUT /api/instruments/:id/media/display`

**请求 Body**: `{ "batch_id": "uuid" }`

设置后自动同步到 `Instrument.Images`/`Video` 字段以保持向后兼容。

#### 5.9.4 删除媒体批次

**接口**: `DELETE /api/instruments/:id/media/:batch_id`

删除对应存储文件及 DB 记录，自动同步 `Instrument.Images`/`Video`。

#### 5.9.5 获取乐器媒体列表

**接口**: `GET /api/instruments/:id/media`

**成功响应**:
```json
{
  "code": 20000,
  "data": {
    "display": [
      { "batch_id": "uuid", "batch_type": "shipping", "file_type": "image", "url": "/uploads/media/...", "sort_order": 1 }
    ],
    "batches": [
      { "batch_id": "uuid", "batch_type": "shipping", "count": 5, "created_at": "2026-06-06T00:00:00Z" }
    ],
    "video": { "batch_id": "uuid", "batch_type": "shipping", "file_type": "video", "url": "/uploads/media/...", "thumb_url": "/uploads/media/..._thumb.jpg", "sort_order": 0 },
    "groups": [
      {
        "batch_id": "uuid",
        "batch_type": "shipping",
        "created_at": "2026-06-06T00:00:00Z",
        "items": [
          { "batch_id": "uuid", "batch_type": "shipping", "file_type": "image", "url": "/uploads/media/...", "sort_order": 1 }
        ]
      }
    ]
  }
}
```

`display`: 当前设为展示的图片列表。`batches`: 所有批次的汇总信息（不含具体文件）。`groups`: 按 `batch_id` 分组的完整文件列表。

#### 5.9.6 公共乐器媒体列表

**接口**: `GET /api/public/instruments/:id/media`

**说明**: 无登录访问，仅返回当前展示图片和当前视频，不暴露历史批次数据。

**成功响应**:
```json
{
  "code": 20000,
  "data": {
    "images": [
      { "url": "/uploads/media/...", "file_type": "image" }
    ],
    "video": { "url": "/uploads/media/...", "thumb_url": "/uploads/media/..._thumb.jpg", "file_type": "video" }
  }
}
```

`images`: 当前展示的图片列表。`video`: 当前视频（含缩略图封面）。无视频时 `video` 为 `null`。

#### 5.9.7 上传大小限制

全站点设置，存储于 `system_settings` 表：

| 设置字段 | 默认值 | 说明 |
|---------|--------|------|
| `upload_image_max_size` | 10 MB | 图片最大尺寸 |
| `upload_video_max_size` | 100 MB | 视频最大尺寸 |

仅命名空间管理员可通过 `GET/PUT /api/settings/:key` 修改。

---

## 六、订单模块

### 6.1 预计算首期费用

**接口**: `POST /api/orders/preview`

**请求 Body**:
```json
{
  "instrument_id": "instr-001",
  "level": "professional",
  "lease_term": 12, // 3, 6, 12 个月
  "deposit_mode": "free" // free | standard
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "first_month_rent": 760, // 800 * 0.95
    "deposit": 0, // 免押金
    "total_amount": 760,
    "discount_info": "12个月租期享95折",
    "deposit_info": "信用分达标，押金已免除",
    "contract_preview": "租用协议摘要..."
  }
}
```

---

### 6.2 创建订单

**接口**: `POST /api/user/orders`

**请求 Body**:
```json
{
  "instrument_id": "instr-001",
  "start_date": "2026-03-21",
  "end_date": "2026-06-21",
  "delivery_address": {},
  "notes": ""
}
```

**响应**:
```json
{
  "code": 20000,
  "message": "success",
  "data": {
    "order_id": "order-001",
    "amount": 2800,
    "deposit": 500,
    "lease_id": "lease-001",
    "contract_id": "contract-001",
    "payment_url": "https://pay.example.com/..."
  }
}
```

---

### 6.3 订单列表

**接口**: `GET /api/orders`

**请求参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| status | string | 状态: pending, active, completed, terminated |
| type | string | 类型: lease, maintenance |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "order-001",
        "instrument_name": "雅马哈立式钢琴 U1",
        "status": "active",
        "created_at": "2026-03-21T10:30:00Z",
        "next_payment_date": "2026-04-21",
        "accumulated_months": 8, // 已累计租期
        "transfer_progress": 66.7 // 租转售进度 %
      }
    ],
    "total": 5
  }
}
```

---

### 6.4 订单详情

**接口**: `GET /api/orders/:id`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "id": "order-001",
    "instrument": {
      "id": "instr-001",
      "name": "雅马哈立式钢琴 U1",
      "level": "professional"
    },
    "lease_term": 12,
    "monthly_rent": 760,
    "deposit": 0,
    "deposit_refunded": false,
    "status": "active",
    "created_at": "2026-03-21T10:30:00Z",
    "start_date": "2026-03-21",
    "end_date": "2027-03-21",
    "accumulated_months": 8,
    "transfer_progress": 66.7,
    "transfer_eligible": false, // 是否满足转售条件
    "payment_history": [
      {
        "month": 1,
        "amount": 760,
        "paid_at": "2026-03-21T10:35:00Z",
        "status": "paid"
      }
    ]
  }
}
```

---

### 6.5 获取合同列表

**接口**: `GET /api/user/contracts`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "contract-001",
        "order_id": "order-001",
        "contract_number": "CT-order-00",
        "status": "active",
        "contract_url": "",
        "generated_at": "2026-03-21T10:30:00Z",
        "created_at": "2026-03-21T10:30:00Z"
      }
    ]
  }
}
```

### 6.6 获取合同详情

**接口**: `GET /api/user/contracts/:id`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "id": "contract-001",
    "order_id": "order-001",
    "contract_number": "CT-order-00",
    "status": "active",
    "contract_url": "",
    "generated_at": "2026-03-21T10:30:00Z",
    "instrument_name": "雅马哈立式钢琴 U1",
    "order_status": "reserved",
    "start_date": "2026-03-21",
    "end_date": "2026-06-21",
    "monthly_rent": 2500,
    "deposit": 500,
    "created_at": "2026-03-21T10:30:00Z"
  }
}
```

> **注**: PDF 生成和签署功能（原 §6.5 租赁协议签署 / §6.6 签署协议）尚未实现，需单独 Issue 处理。

---

### 6.6 签署协议

**接口**: `POST /api/orders/:id/sign`

**请求 Body**:
```json
{
  "signature": "data:image/png;base64,..."
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "signed_at": "2026-03-21T10:40:00Z",
    "contract_url": "https://cdn.example.com/contracts/001.pdf"
  }
}
```

---

### 6.7 终止租约

**接口**: `PUT /api/orders/:id/terminate`

**请求 Body**:
```json
{
  "reason": "个人原因"
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "terminated_at": "2026-06-21T15:00:00Z",
    "refund_amount": 1500 // 押金退还金额
  }
}
```

---

### 6.8 触发所有权转移

**接口**: `POST /api/orders/:id/transfer-ownership`

> **说明**: 租满 12 个月后，由系统定时任务或管理员手动触发

**响应**:
```json
{
  "code": 20000,
  "data": {
    "transfer_completed": true,
    "ownership_certificate_id": "cert-001",
    "transferred_at": "2027-03-21T00:00:00Z"
  }
}
```

---

### 6.8 出库确认管理

#### 6.8.1 获取出库照片

**接口**: `GET /api/orders/:id/outbound-photos`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "outbound_photos": [
      {
        "url": "https://cdn.example.com/outbound/photo1.jpg",
        "batch_id": "batch-001",
        "taken_at": "2026-03-21T10:30:00Z"
      }
    ],
    "assessment_photos": []
  }
}
```

#### 6.8.2 确认出库

**接口**: `POST /api/orders/:id/outbound-confirm`

**请求 Body**:
```json
{
  "confirmed_by": "user-001",
  "photos": ["img-001", "img-002"],
  "condition_notes": "外观完好，音色正常"
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "outbound_confirmed": true,
    "confirmed_at": "2026-03-21T10:35:00Z"
  }
}
```

### 6.9 损伤评估管理

#### 6.9.1 获取评估数据

**接口**: `GET /api/orders/:id/assessment`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "outbound_condition": {
      "notes": "外观完好",
      "photos": ["img-001"]
    },
    "return_condition": {
      "notes": "琴键磨损",
      "photos": ["img-003"],
      "damage_level": "minor"
    },
    "assessment_status": "pending"
  }
}
```

#### 6.9.2 提交评估

**接口**: `POST /api/orders/:id/assessment`

**请求 Body**:
```json
{
  "damage_items": [
    {
      "label_id": "label-001",
      "severity": "minor",
      "repair_cost": 200
    }
  ],
  "liability": "user", // user, normal_wear, covered
  "total_deduction": 500,
  "notes": "琴键正常磨损"
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "assessment_id": "assmt-001",
    "total_deduction": 500,
    "deposit_adjustment": 500
  }
}
```

#### 6.9.3 生成评估报告

**接口**: `GET /api/reports/assessment/:order_id`

**响应**: PDF 文件流

**Headers**:
```
Content-Type: application/pdf
Content-Disposition: attachment; filename="assessment_order_001.pdf"
```

---

## 七、维保服务模块

### 7.1 查询服务包覆盖项

**接口**: `GET /api/maintenance/coverage/:instrumentId`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "instrument_level": "professional",
    "free_services": [
      "基础清洁",
      "免费调音 2 次/年",
      "免费维修"
    ],
    "paid_services": [
      "专家上门调律 (+￥200)",
      "深度保养 (+￥500)"
    ]
  }
}
```

---

### 7.2 提交报修工单

**接口**: `POST /api/maintenance`

**请求 Body**:
```json
{
  "order_id": "order-001",
  "instrument_id": "instr-001",
  "problem_description": "琴弦松动，音准不准",
  "images": ["img1.jpg", "img2.jpg"],
  "service_type": "self_delivery", // self_delivery, pickup
  "preferred_site_id": "site-001"
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "ticket_id": "ticket-001",
    "status": "pending",
    "created_at": "2026-03-22T09:00:00Z",
    "estimated_cost": 0 // 预估费用（服务包内免费则为 0）
  }
}
```

---

### 7.3 工单列表

**接口**: `GET /api/maintenance`

**请求参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| status | string | 状态: pending, processing, completed |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "ticket-001",
        "instrument_name": "雅马哈立式钢琴 U1",
        "status": "processing",
        "created_at": "2026-03-22T09:00:00Z",
        "progress": "维修中"
      }
    ],
    "total": 3
  }
}
```

---

### 7.4 工单详情

**接口**: `GET /api/maintenance/:id`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "id": "ticket-001",
    "instrument": {
      "id": "instr-001",
      "name": "雅马哈立式钢琴 U1"
    },
    "problem_description": "琴弦松动",
    "images": ["img1.jpg"],
    "status": "processing",
    "service_type": "self_delivery",
    "assigned_site": {
      "id": "site-001",
      "name": "北京朝阳店"
    },
    "progress_updates": [
      {
        "status": "已接单",
        "description": "师傅已确认接单",
        "updated_at": "2026-03-22T10:00:00Z"
      },
      {
        "status": "维修中",
        "description": "更换琴弦，调整音准",
        "updated_at": "2026-03-22T14:00:00Z"
      }
    ],
    "estimated_cost": 0,
    "actual_cost": 0
  }
}
```

---

### 7.5 取消报修

**接口**: `PUT /api/maintenance/:id/cancel`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "status": "cancelled",
    "cancelled_at": "2026-03-22T10:00:00Z"
  }
}
```

---

### 7.6 技师工作台

#### 7.6.1 技师工单列表

**接口**: `GET /api/technician/tickets`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "ticket-001",
        "instrument_name": "雅马哈立式钢琴 U1",
        "problem": "琴弦松动",
        "status": "pending",
        "created_at": "2026-03-22T09:00:00Z",
        "assigned_site": "北京朝阳店"
      }
    ],
    "total": 3
  }
}
```

#### 7.6.2 技师接单

**接口**: `PUT /api/technician/tickets/:id/accept`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "status": "processing",
    "accepted_at": "2026-03-22T10:00:00Z",
    "technician_id": "tech-001"
  }
}
```

#### 7.6.3 完成工单

**接口**: `POST /api/technician/tickets/:id/complete`

**请求 Body**:
```json
{
  "actual_cost": 0,
  "repair_details": "更换琴弦，调整音准",
  "completion_photos": ["repair1.jpg"]
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "status": "completed",
    "completed_at": "2026-03-22T18:00:00Z"
  }
}
```

### 7.7 工单状态更新

**接口**: `PUT /api/maintenance/tickets/:id/status`

**请求 Body**:
```json
{
  "status": "processing",
  "notes": "已分配师傅"
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "status": "processing",
    "updated_at": "2026-03-22T10:30:00Z"
  }
}
```

---

## 八、个人中心模块

### 8.1 租约管理

**接口**: `GET /api/user/leases`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "lease-001",
        "order_id": "order-001",
        "instrument_name": "雅马哈立式钢琴 U1",
        "start_date": "2026-03-21",
        "end_date": "2027-03-21",
        "accumulated_months": 8,
        "status": "active"
      }
    ],
    "total": 3
  }
}
```

---

### 8.2 租转售进度

**接口**: `GET /api/user/leases/:id/progress`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "lease_id": "lease-001",
    "instrument_name": "雅马哈立式钢琴 U1",
    "accumulated_months": 8,
    "total_required_months": 12,
    "progress_percentage": 66.7,
    "remaining_months": 4,
    "estimated_transfer_date": "2025-07-21",
    "transfer_eligible": false,
    "message": "🎁 距离永久拥有仅剩 4 个月"
  }
}
```

---

### 8.3 电子所有权证明

**接口**: `GET /api/user/ownership/:id`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "certificate_id": "cert-001",
    "order_id": "order-001",
    "instrument": {
      "id": "instr-001",
      "name": "雅马哈立式钢琴 U1",
      "sn": "SN-2024-0001"
    },
    "owner": {
      "user_id": "user-001",
      "name": "张三",
      "phone": "138****8888"
    },
    "transfer_date": "2027-03-21",
    "certificate_url": "https://cdn.example.com/certificates/cert-001.pdf"
  }
}
```

---

### 8.4 下载 PDF 证明

**接口**: `GET /api/user/ownership/:id/download`

**响应**: 二进制 PDF 文件流

**Headers**:
```
Content-Type: application/pdf
Content-Disposition: attachment; filename="ownership_certificate_001.pdf"
```

---

### 8.5 文件上传

**接口**: `POST /api/upload`

**Content-Type**: `multipart/form-data`

**表单字段**:
- `file`: 文件数据
- `type`: 文件类型 (optional)

**响应**:
```json
{
  "code": 20000,
  "data": {
    "file_id": "file-001",
    "url": "https://cdn.example.com/uploads/image.jpg",
    "filename": "image.jpg",
    "size": 2048576
  }
}
```

### 8.6 逾期租约列表

**接口**: `GET /api/overdue-leases`

**说明**: 获取所有逾期未归还的租约列表

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "lease_id": "lease-002",
        "order_id": "order-002",
        "instrument_name": "雅马哈立式钢琴 U3",
        "user_name": "李四",
        "user_phone": "139****9999",
        "end_date": "2026-03-15",
        "overdue_days": 7,
        "monthly_rent": 800,
        "deposit": 5000
      }
    ],
    "total": 3
  }
}
```

### 8.7 收藏列表

**接口**: `GET /api/user/favorites`

**响应**:
```json
{
  "code": 20000,
  "data": {

**接口**: `GET /api/user/favorites`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "fav-001",
        "instrument_id": "instr-001",
        "instrument_name": "雅马哈立式钢琴 U1",
        "added_at": "2026-03-20T18:00:00Z"
      }
    ],
    "total": 5
  }
}
```

---

### 8.8 添加收藏

**接口**: `POST /api/user/favorites`

**请求 Body**:
```json
{
  "instrument_id": "instr-002"
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "favorite_id": "fav-002",
    "created": true
  }
}
```

---

### 8.9 取消收藏

**接口**: `DELETE /api/user/favorites/:id`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "deleted": true
  }
}
```

---

### 8.10 地址管理

#### 8.10.1 地址列表

**接口**: `GET /api/user/addresses`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "addr-001",
        "receiver_name": "张三",
        "phone": "13800000000",
        "province": "北京市",
        "city": "北京市",
        "district": "朝阳区",
        "detail": "xxx路123号",
        "is_default": true
      }
    ]
  }
}
```

---

#### 8.10.2 新增地址

**接口**: `POST /api/user/addresses`

**请求 Body**:
```json
{
  "receiver_name": "李四",
  "phone": "13900000000",
  "province": "上海市",
  "city": "上海市",
  "district": "浦东新区",
  "detail": "xxx路456号",
  "is_default": false
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "address_id": "addr-002"
  }
}
```

---

#### 8.10.3 更新地址

**接口**: `PUT /api/user/addresses/:id`

**响应**: 同新增地址

---

#### 8.10.4 删除地址

**接口**: `DELETE /api/user/addresses/:id`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "deleted": true
  }
}
```

---

## 九、商家管理端

> ⚠️ **所有接口必须从 Context 获取 tenant_id 和 org_id**

### 9.1 设备台账

**接口**: `GET /api/merchant/assets`

**请求参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| sn | string | SN 码筛选 |
| level | string | 级别筛选 |
| status | string | 状态筛选 |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "asset-001",
        "sn": "SN-2024-0001",
        "name": "雅马哈立式钢琴 U1",
        "level": "professional",
        "purchase_date": "2024-01-15",
        "depreciation_rate": 0.85,
        "current_value": 17000,
        "status": "renting",
        "current_order_id": "order-001",
        "accumulated_rent_months": 18
      }
    ],
    "total": 150
  }
}
```

---

### 9.2 设备详情

**接口**: `GET /api/merchant/assets/:id`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "id": "asset-001",
    "sn": "SN-2024-0001",
    "name": "雅马哈立式钢琴 U1",
    "level": "professional",
    "purchase_info": {
      "date": "2024-01-15",
      "price": 20000,
      "supplier": "雅马哈中国"
    },
    "depreciation": {
      "rate": 0.85,
      "current_value": 17000,
      "method": "直线法"
    },
    "lifecycle": {
      "status": "renting",
      "current_order_id": "order-001",
      "rental_history": [
        {
          "order_id": "order-001",
          "user_name": "张三",
          "start_date": "2026-03-21",
          "end_date": "2027-03-21"
        }
      ],
      "maintenance_history": [
        {
          "ticket_id": "ticket-001",
          "date": "2026-08-10",
          "issue": "琴弦松动",
          "cost": 0
        }
      ]
    }
  }
}
```

---

### 9.3 库存监控

**接口**: `GET /api/merchant/inventory`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "overview": {
      "total_assets": 150,
      "in_stock": 45,
      "renting": 85,
      "maintenance": 20
    },
    "by_category": [
      {
        "category": "钢琴",
        "in_stock": 15,
        "renting": 35,
        "maintenance": 5
      }
    ]
  }
}
```

---

### 9.4 强制状态翻转

**接口**: `PUT /api/merchant/inventory/:id/status`

**请求 Body**:
```json
{
  "status": "maintenance",
  "reason": "用户报修"
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "asset_id": "asset-001",
    "old_status": "renting",
    "new_status": "maintenance",
    "updated_at": "2026-03-22T10:00:00Z"
  }
}
```

---

### 9.5 网点间调拨

**接口**: `POST /api/merchant/inventory/transfer`

**请求 Body**:
```json
{
  "asset_id": "asset-001",
  "from_site_id": "site-001",
  "to_site_id": "site-002",
  "reason": "库存调配"
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "transfer_id": "transfer-001",
    "asset_id": "asset-001",
    "status": "pending",
    "created_at": "2026-03-22T11:00:00Z"
  }
}
```

---

### 9.6 所有权监控

**接口**: `GET /api/merchant/ownership-monitor`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "asset_id": "asset-001",
        "sn": "SN-2024-0001",
        "name": "雅马哈立式钢琴 U1",
        "accumulated_rent_months": 11,
        "remaining_months": 1,
        "estimated_transfer_date": "2025-04-21",
        "current_user": "张三"
      }
    ],
    "total": 8
  }
}
```

---

### 9.7 租约台账

**接口**: `GET /api/merchant/leases`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "lease-001",
        "order_id": "order-001",
        "instrument_name": "雅马哈立式钢琴 U1",
        "user_name": "张三",
        "user_phone": "138****8888",
        "start_date": "2026-03-21",
        "end_date": "2027-03-21",
        "monthly_rent": 760,
        "deposit": 0,
        "status": "active"
      }
    ],
    "total": 85
  }
}
```

---

### 9.8 逾期预警

**接口**: `GET /api/merchant/leases/overdue`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "lease-002",
        "order_id": "order-002",
        "instrument_name": "雅马哈立式钢琴 U3",
        "user_name": "李四",
        "user_phone": "139****9999",
        "end_date": "2026-03-15",
        "overdue_days": 7,
        "monthly_rent": 800,
        "deposit": 5000
      }
    ],
    "total": 3
  }
}
```

---

### 9.9 发送逾期提醒

**接口**: `POST /api/merchant/leases/:id/notify`

**请求 Body**:
```json
{
  "notify_type": "sms", // sms, wechat, phone
  "template": "overdue_reminder"
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "notification_sent": true,
    "sent_at": "2026-03-22T10:00:00Z"
  }
}
```

---

### 9.10 维保工单列表

**接口**: `GET /api/merchant/maintenance`

**请求参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| status | string | pending, processing, completed |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "ticket-001",
        "instrument_name": "雅马哈立式钢琴 U1",
        "user_name": "张三",
        "problem": "琴弦松动",
        "status": "processing",
        "created_at": "2026-03-22T09:00:00Z",
        "assigned_technician": "李师傅"
      }
    ],
    "total": 12
  }
}
```

---

### 9.11 接单

**接口**: `PUT /api/merchant/maintenance/:id/accept`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "status": "processing",
    "accepted_at": "2026-03-22T10:00:00Z"
  }
}
```

---

### 9.12 分配师傅

**接口**: `PUT /api/merchant/maintenance/:id/assign`

**请求 Body**:
```json
{
  "technician_id": "tech-001"
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "status": "processing",
    "technician": "李师傅",
    "assigned_at": "2026-03-22T10:30:00Z"
  }
}
```

---

### 9.13 确认取琴

**接口**: `PUT /api/merchant/maintenance/:id/pickup`

**请求 Body**:
```json
{
  "pickup_time": "2026-03-22T15:00:00Z",
  "notes": "用户已送至门店"
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "status": "processing",
    "picked_up_at": "2026-03-22T15:00:00Z"
  }
}
```

---

### 9.14 更新维修进度

**接口**: `PUT /api/merchant/maintenance/:id/update`

**请求 Body**:
```json
{
  "progress": "更换琴弦中",
  "images": ["repair1.jpg"],
  "estimated_completion": "2026-03-23T18:00:00Z"
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "status": "processing",
    "progress": "更换琴弦中",
    "updated_at": "2026-03-22T16:00:00Z"
  }
}
```

---

### 9.15 维保报价

**接口**: `POST /api/merchant/maintenance/:id/quote`

**请求 Body**:
```json
{
  "service_item": "专家精调",
  "price": 200,
  "reason": "超出服务包范围"
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "quote_id": "quote-001",
    "price": 200,
    "approval_required": true,
    "created_at": "2026-03-22T17:00:00Z"
  }
}
```

---

### 9.16 佣金明细

**接口**: `GET /api/merchant/finance/commissions`

**请求参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| start_date | string | 开始日期 (YYYY-MM-DD) |
| end_date | string | 结束日期 (YYYY-MM-DD) |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "order_id": "order-001",
        "type": "lease",
        "amount": 760,
        "commission_rate": 0.15,
        "commission_amount": 114,
        "status": "settled",
        "settled_at": "2026-03-25T00:00:00Z"
      }
    ],
    "total": 25,
    "summary": {
      "total_commission": 2850,
      "pending_settlement": 450
    }
  }
}
```

---

### 9.17 流水报表导出

**接口**: `GET /api/merchant/finance/statement`

**请求参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| start_date | string | 开始日期 |
| end_date | string | 结束日期 |
| format | string | csv, excel (默认 excel) |

**响应**: 二进制文件流

**Headers**:
```
Content-Type: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
Content-Disposition: attachment; filename="statement_202603.xlsx"
```

---

## 十、平台运营端

### 10.1 商家准入审核

**接口**: `POST /api/admin/merchants`

**请求 Body**:
```json
{
  "merchant_name": "北京音乐之家",
  "legal_person": "王五",
  "business_license": "image.jpg",
  "contact_phone": "13800000000",
  "deposit_amount": 50000
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "merchant_id": "merchant-001",
    "audit_status": "pending",
    "created_at": "2026-03-22T10:00:00Z"
  }
}
```

---

### 10.2 商家列表

**接口**: `GET /api/admin/merchants`

**请求参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| status | string | pending, approved, rejected |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "merchant-001",
        "name": "北京音乐之家",
        "legal_person": "王五",
        "status": "approved",
        "created_at": "2026-03-22T10:00:00Z",
        "deposit_paid": true
      }
    ],
    "total": 15
  }
}
```

---

### 10.3 商家资质审核

**接口**: `PUT /api/admin/merchants/:id/audit`

**请求 Body**:
```json
{
  "status": "approved", // approved, rejected
  "remark": "资料齐全，符合要求"
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "merchant_id": "merchant-001",
    "audit_status": "approved",
    "audited_at": "2026-03-23T09:00:00Z"
  }
}
```

---

### 10.4 权限管理 — 成员列表

**接口**: `GET /api/admin/users`

**守卫**: `RequireSysPerm(26)` (permission:manage)

**响应**:
```json
{
  "code": 20000,
  "data": [
    {
      "user_id": "uuid",
      "name": "张三",
      "site_id": "site-uuid",
      "site_name": "朝阳店",
      "role_code": "admin",
      "role_name": "网点管理员",
      "cus_perm_codes": ["instrument:read", "instrument:update"]
    }
  ]
}
```

### 10.5 权限管理 — 设置个人权限

**接口**: `PUT /api/admin/users/:id/permissions`

**守卫**: `RequireSysPerm(26)`

**请求 Body**:
```json
{
  "cus_perm_codes": ["instrument:read", "instrument:maintain"]
}
```

**验证规则**: 每个 code 必须 ⊆ 当前管理员的 cus_perm，不可授予自己没有的权限。

**响应**:
```json
{
  "code": 20000,
  "message": "permissions updated, will take effect on next login"
}
```

### 10.5.1 权限管理 — 设置成员角色

**接口**: `PUT /api/admin/users/:id/roles`

**守卫**: `RequireSysPerm(26)`

**请求 Body**:
```json
{
  "role_code": "worker"
}
```

**响应**:
```json
{
  "code": 20000,
  "message": "role updated, will take effect on next login"
}
```

### 10.5.2 权限管理 — 角色列表

**接口**: `GET /api/admin/roles`

**守卫**: `RequireSysPerm(26)`

**响应**:
```json
{
  "code": 20000,
  "data": [
    {
      "id": "uuid",
      "name": "商户管理员",
      "code": "owner",
      "cus_perm_codes": ["instrument:create", "instrument:read", "instrument:update", "instrument:delete", "instrument:price", "instrument:maintain", "order:create", "order:read", "order:update", "order:cancel"],
      "is_system": true,
      "permission_count": 10
    }
  ]
}
```

### 10.5.3 权限管理 — 创建角色

**接口**: `POST /api/admin/roles`

**守卫**: `RequireSysPerm(26)` + 商户管理员

**请求 Body**:
```json
{
  "name": "库管员",
  "code": "warehouse_keeper",
  "cus_perm_codes": ["instrument:read", "instrument:update"]
}
```

**响应**:
```json
{
  "code": 20000,
  "message": "role created",
  "data": { "id": "uuid" }
}
```

### 10.5.4 权限管理 — 更新角色

**接口**: `PUT /api/admin/roles/:id`

**守卫**: `RequireSysPerm(26)` + 商户管理员

**请求 Body**:
```json
{
  "cus_perm_codes": ["instrument:read", "instrument:price", "order:read"]
}
```

### 10.5.5 权限管理 — 删除角色

**接口**: `DELETE /api/admin/roles/:id`

**守卫**: `RequireSysPerm(26)` + 商户管理员

**约束**: 系统角色不可删除；有成员使用的角色不可删除（需先重新分配）

---

### 10.6 定价矩阵

**接口**: `GET /api/admin/pricing-matrix`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "matrix": {
      "piano": {
        "entry": {
          "monthly_rent": 300,
          "deposit": 2000,
          "discount_12months": 0.95
        },
        "professional": {
          "monthly_rent": 800,
          "deposit": 5000,
          "discount_12months": 0.95
        },
        "master": {
          "monthly_rent": 2000,
          "deposit": 10000,
          "discount_12months": 0.90
        }
      }
    }
  }
}
```

---

### 10.7 更新定价矩阵

**接口**: `PUT /api/admin/pricing-matrix`

**请求 Body**:
```json
{
  "category": "piano",
  "level": "professional",
  "monthly_rent": 850,
  "deposit": 5500
}
```

---

### 10.8 维保服务包配置

**接口**: `GET /api/admin/maintenance-packages`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "packages": {
      "entry": {
        "name": "基础服务包",
        "services": ["基础清洁", "免费调音 1 次/年"]
      },
      "professional": {
        "name": "标准服务包",
        "services": ["深度清洁", "免费调音 2 次/年", "免费维修"]
      },
      "master": {
        "name": "尊享服务包",
        "services": ["专家精调", "无限次调音", "免费维修", "上门保养"]
      }
    }
  }
}
```

---

### 10.9 更新服务包

**接口**: `PUT /api/admin/maintenance-packages`

**请求 Body**:
```json
{
  "level": "professional",
  "services": [
    "深度清洁",
    "免费调音 2 次/年",
    "免费维修",
    "新增服务项"
  ]
}
```

---

### 10.10 强制触发所有权转移

**接口**: `POST /api/admin/ownership/trigger`

**请求 Body**:
```json
{
  "order_id": "order-001",
  "approve": true
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "transfer_completed": true,
    "certificate_generated": true
  }
}
```

---

### 10.11 全局结算

**接口**: `GET /api/admin/settlements`

**请求参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| period | string | 结算周期: weekly, monthly |
| status | string | pending, completed |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "settlement_id": "stl-001",
        "merchant_id": "merchant-001",
        "merchant_name": "北京音乐之家",
        "period": "2026-03",
        "total_revenue": 50000,
        "commission_rate": 0.15,
        "commission_amount": 7500,
        "settlement_amount": 42500,
        "status": "pending"
      }
    ],
    "total": 15
  }
}
```

---

### 10.12 结算确认

**接口**: `PUT /api/admin/settlements/:id`

**请求 Body**:
```json
{
  "status": "completed",
  "payment_transaction_id": "txn-001"
}
```

---

### 10.13 押金监管

**接口**: `GET /api/admin/deposits`

**请求参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| status | string | frozen, released, deducted |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "deposit_id": "dep-001",
        "order_id": "order-001",
        "user_name": "张三",
        "amount": 5000,
        "status": "frozen",
        "frozen_at": "2026-03-21",
        "reason": "标准押金"
      }
    ],
    "total": 120,
    "summary": {
      "total_frozen": 600000,
      "total_released": 450000,
      "total_deducted": 15000
    }
  }
}
```

---

### 10.14 押金处理

**接口**: `PUT /api/admin/deposits/:id`

**请求 Body**:
```json
{
  "action": "release", // release, deduct
  "amount": 5000,
  "reason": "租约正常结束"
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "deposit_id": "dep-001",
    "action": "release",
    "processed_at": "2026-03-22T10:00:00Z"
  }
}
```

---

### 10.15 资产流转轨迹

**接口**: `GET /api/admin/assets/:id/trail`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "asset_id": "asset-001",
    "sn": "SN-2024-0001",
    "timeline": [
      {
        "event": "入库",
        "date": "2024-01-15",
        "location": "北京总仓",
        "description": "采购入库"
      },
      {
        "event": "租赁",
        "date": "2026-03-21",
        "location": "北京朝阳店",
        "description": "租给用户张三"
      },
      {
        "event": "维保",
        "date": "2026-08-10",
        "location": "北京朝阳店",
        "description": "琴弦松动维修"
      }
    ]
  }
}
```

---

### 10.16 统计大屏

**接口**: `GET /api/admin/dashboard`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "overview": {
      "total_assets": 1500,
      "total_value": 30000000,
      "rental_rate": 85.3, // 在租率 %
      "total_revenue": 2500000
    },
    "by_category": {
      "piano": {
        "total": 500,
        "renting": 420,
        "revenue": 1200000
      }
    },
    "transfer_stats": {
      "total_transferred": 120,
      "conversion_rate": 8.0 // 转售转化率 %
    }
  }
}
```

---

### 10.17 人员管理

#### 10.17.1 创建用户

**接口**: `POST /api/users`

**权限**: `sys_perm bit 17 (user:create)`

**请求 Body**:
```json
{
  "name": "张三",
  "phone": "13800000000",
  "email": "zhangsan@example.com",
  "username": "zhangsan",
  "position": "销售",
  "user_type": "员工",
  "site_id": "uuid-here",
  "role": "site_member",
  "password": "MyPwd123",
  "auto_generate": false,
  "force_password_change": true
}
```

**参数说明**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | 是 | 姓名 |
| phone | string | 是 | 手机号 |
| email | string | 否 | 邮箱（改选填） |
| username | string | 否 | 用户名（不传时自动使用 email 前缀） |
| position | string | 否 | 职位 |
| user_type | string | 否 | 用户类型（员工/维修技师） |
| site_id | string | 否 | 归属网点 ID |
| role | string | 否 | 角色：site_member / site_admin / worker |
| password | string | 否 | 管理员设置的初始密码（8位+大写+小写+数字） |
| auto_generate | bool | 否 | 自动生成密码（true时忽略 password 字段） |
| force_password_change | bool | 否 | 首次登录强制修改密码 |

**密码设置场景**:

| 场景 | password | auto_generate | 说明 |
|------|----------|---------------|------|
| 手动设密 | 提供 | false | 用户直接激活，无确认邮件 |
| 自动生成 | 空 | true | 后端返回 initial_password |
| 兼容旧流程 | 空 | false | IAM 发送确认邮件 |

**角色默认值逻辑**：
- 未传 `role` 时，若指定了 `site_id`，查询该网点已有成员数：
  - **第一个成员** → `site_admin`（网点管理员）
  - **后续成员** → `site_member`（网点员工）
- 未传 `role` 且未指定 `site_id` → `site_member`

**约束**：
- `phone`、`email`、`username` 在租户内唯一，冲突返回 `40900`
- 只有 `site_admin`/`merchant_admin` 可以创建 `site_admin` 角色
- 创建用户后自动在 IAM 侧绑定组织、设置权限位图、分配角色模板

**响应**:
```json
{
  "code": 20000,
  "message": "success",
  "data": {
    "id": "uuid",
    "username": "zhangsan",
    "name": "张三",
    "phone": "13800000000",
    "email": "zhangsan@example.com",
    "position": "销售",
    "user_type": "员工",
    "created_at": "2026-01-01T00:00:00Z",
    "updated_at": "2026-01-01T00:00:00Z",
    "initial_password": "aB3xK9mQ2pL7"
  }
}
```

> `initial_password` 仅在 `auto_generate=true` 时返回，展示一次后应丢弃。

---

#### 10.17.3 修改个人密码

**接口**: `POST /api/user/change-password`

**权限**: 登录即可（自服务）

**说明**: 当前登录用户直接修改密码（无需邮件确认）。成功后清除 `force_password_change` 标志。

**请求 Body**:
```json
{
  "new_password": "NewPwd123"
}
```

**参数说明**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| new_password | string | 是 | 新密码（8位+大写+小写+数字） |

**频率限制**: 同用户每 5 分钟最多 3 次。

**响应**:
```json
{
  "code": 20000,
  "message": "密码修改成功"
}
```

---

#### 10.17.4 重置个人密码

**接口**: `POST /api/user/reset-password`

**说明**: 当前登录用户请求发送密码重置邮件。后端代理转发到 beaconiam 的 `POST /api/v1/users/reset-password`。

**权限**: 已登录用户（从 JWT 获取 `iam_sub`）

**频率限制**: 每用户每 30 分钟最多 3 次，超出返回 `42900`

**请求 Body**: 无（空请求体）

**成功响应**:
```json
{
  "code": 20000,
  "message": "密码重置邮件已发送至 z***@example.com，请查收",
  "data": {
    "email_masked": "z***@example.com",
    "expires_in_minutes": 60
  }
}
```

**错误响应**:
| HTTP 状态码 | code | message |
|-------------|------|---------|
| 400 | 40001 | 您的账户未绑定邮箱，请联系管理员 |
| 429 | 42900 | 操作过于频繁，请 30 分钟后再试 |
| 500 | 50002 | 邮件发送失败，请稍后重试 |

---

#### 10.17.5 批量导入用户

**接口**: `POST /api/admin/bulk-import/accounts`

**权限**: `sys_perm bit 17 (user:create)`

**Content-Type**: `multipart/form-data`

**表单字段**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| file | file | 是 | CSV 文件 |
| skip_activation | bool | 否 | 跳过邮箱激活（默认 false） |

**查询参数**:
- `skip_activation=true`：自动生成密码，IAM 发送通知邮件，用户直接激活
- `skip_activation=false`（默认）：用户为 pending 状态，需邮箱确认

**CSV 格式**:
```csv
username,name,email,phone,site,role
zhangsan,张三,zhangsan@example.com,13800000000,朝阳网点,site_member
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "summary": { "total": 2, "created": 2, "failed": 0 },
    "details": [
      { "row": 1, "key": "zhangsan@example.com", "action": "created" }
    ]
  }
}
```

---

#### 10.17.6 预览批量导入

**接口**: `POST /api/admin/bulk-import/accounts?dry_run=true`

**说明**: 预览解析结果，不实际创建用户。支持 `skip_activation` 参数。

---

## 十一、通用模块

### 11.1 文件上传

**接口**: `POST /api/common/upload`

**请求**: multipart/form-data

**响应**:
```json
{
  "code": 20000,
  "data": {
    "file_id": "file-001",
    "url": "https://cdn.example.com/uploads/image.jpg",
    "filename": "image.jpg",
    "size": 2048576
  }
}
```

---

### 11.2 地区数据

**接口**: `GET /api/common/regions`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "provinces": [
      {
        "code": "110000",
        "name": "北京市"
      }
    ],
    "cities": [
      {
        "code": "110100",
        "name": "北京市",
        "province_code": "110000"
      }
    ]
  }
}
```

---

## 十二、系统管理模块

### 12.1 客户端管理

**接口**: `GET /api/system/clients`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "clients": [
      {
        "client_id": "tuneloop-pc",
        "client_name": "PC Web Client",
        "redirect_uris": ["http://localhost:5554/callback"]
      }
    ]
  }
}
```

### 12.2 租户管理

**接口**: `GET /api/system/tenants`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "tenants": [
      {
        "tenant_id": "tenant-001",
        "tenant_name": "TuneLoop Primary",
        "org_count": 5
      }
    ]
  }
}

### 12.3 商户管理

#### 12.3.1 创建商户

**接口**: `POST /api/merchants`

**请求 Body**:
```json
{
  "name": "cadenza",
  "phone": "13800000000",
  "address": "北京市海淀区",
  "admin_name": "管理员姓名",
  "admin_email": "admin@example.com",
  "admin_phone": "13800000001",
  "merchant_type": "controlled",
  "transit_address": "中转库房地址",
  "transit_phone": "13911112222",
  "transit_contact_name": "中转联系人",
  "skip_activation": true
}
```

**字段说明**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | string | ✅ | 商户名，命名空间下唯一 |
| phone | string | | 联系电话 |
| address | string | | 地址 |
| admin_uid | string | | 已有用户 UUID（与 admin_name+admin_email 二选一） |
| admin_name | string | | 新管理员姓名（与 admin_uid 二选一） |
| admin_email | string | | 新管理员邮箱（与 admin_uid 二选一） |
| admin_phone | string | | 新管理员手机号 |
| merchant_type | string | | 商户类型：`full`（全权商户，默认）或 `controlled`（受控商户）。受控商户使用中转地址隔离消费者与商户直接联系 |
| transit_address | string | | 中转地址（受控商户必填） |
| transit_phone | string | | 中转电话（受控商户必填） |
| transit_contact_name | string | | 中转联系人（可选） |
| skip_activation | bool | | 跳过邮箱验证，管理员直接激活（默认 false）。`true` 时管理员无需确认邮件即可登录，响应中返回 `initial_password` |

**响应**:
```json
{
  "code": 20100,
  "data": {
    "id": "uuid",
    "name": "cadenza",
    "code": "cadenza",
    "iam_org_id": "uuid",
    "admin_uid": "uuid",
    "directly_added": ["uuid"],
    "callback_url": "https://example.com/api/iam/confirmation-callback",
    "iam_admin_id": "uuid",
    "initial_password": "AbCd1234XyZ"
  }
}
```

**响应字段补充**:
| 字段 | 类型 | 说明 |
|------|------|------|
| initial_password | string | 仅在 `skip_activation=true` 时返回。管理员初始密码，请尽快通知管理员修改 |

**错误码**:
- `40002`: 商户名已存在
- `40001`: 请求参数错误
- `40900`: IAM 组织名称冲突

#### 12.3.2 商户列表

**接口**: `GET /api/merchants`

**查询参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
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
        "name": "cadenza",
        "phone": "13800000000",
        "address": "北京市海淀区",
        "admin_uid": "uuid",
        "merchant_type": "full",
        "transit_address": "",
        "transit_phone": "",
        "status": "active",
        "created_at": "2026-05-18T04:49:55Z"
      }
    ],
    "total": 1
  }
}
```

#### 12.3.3 更新商户

**接口**: `PUT /api/merchants/:id`

**请求 Body**:
```json
{
  "name": "new-name",
  "phone": "13900000000",
  "address": "新地址",
  "merchant_type": "controlled",
  "transit_address": "新中转地址",
  "transit_phone": "13911112222",
  "transit_contact_name": "新联系人"
}
```

**响应**: 常规成功响应 `{ code: 20000, data: { ... } }`

#### 12.3.4 删除商户

**接口**: `DELETE /api/merchants/:id`

**响应**: `{ "code": 20000, "message": "success" }`
```

### 12.4 审计日志

#### 12.4.1 查询审计日志列表

**接口**: `GET /api/admin/audit-logs`

**查询参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| page | int | 页码，默认 1 |
| pageSize | int | 每页数量，默认 20，最大 100 |
| resource_type | string | 资源类型筛选 |
| action | string | 操作类型筛选 |
| user_id | string | 操作用户 ID |
| date_from | string | 起始时间 (YYYY-MM-DD) |
| date_to | string | 结束时间 (YYYY-MM-DD) |
| keyword | string | 关键词搜索（resource_type/resource_id/action） |

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "uuid",
        "tenant_id": "uuid",
        "org_id": "uuid",
        "user_id": "uuid",
        "actor_role": "ADMIN",
        "action": "CREATE",
        "resource_type": "order",
        "resource_id": "uuid",
        "details": null,
        "request_body": null,
        "ip_address": "192.168.1.1",
        "user_agent": "Mozilla/5.0",
        "created_at": "2026-05-19T03:00:00Z"
      }
    ],
    "total": 100,
    "page": 1,
    "pageSize": 20
  }
}
```

**RBAC 可见范围**:
| 角色 | 可见范围 |
|------|---------|
| ADMIN/OWNER | 全租户（tenant_id 范围） |
| site_admin | 本组织（org_id 范围） |
| 其他用户 | 仅本人日志（user_id 范围） |

#### 12.4.2 获取审计日志详情

**接口**: `GET /api/admin/audit-logs/:id`

**响应**: 单条审计日志对象，格式同列表中的元素

#### 12.4.3 导出审计日志

**接口**: `POST /api/admin/audit-logs/export`

**请求 Body**（可选筛选参数，同列表接口）:
```json
{
  "resource_type": "order",
  "action": "CREATE",
  "user_id": "uuid",
  "date_from": "2026-01-01",
  "date_to": "2026-12-31",
  "keyword": ""
}
```

**响应**: CSV 文件下载（Content-Type: text/csv, Content-Disposition: attachment; filename=audit_logs.csv）

**CSV 列**: Time, UserID, ActorRole, Action, ResourceType, ResourceID, IPAddress

---

## 十三、标签与属性管理模块

### 13.1 标签管理

#### 13.1.1 获取标签列表

**接口**: `GET /api/labels`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "label-001",
        "name": "琴弦松动",
        "status": "pending",
        "created_at": "2026-03-22T09:00:00Z"
      }
    ],
    "total": 25
  }
}
```

#### 13.1.2 创建标签

**接口**: `POST /api/labels`

**请求 Body**:
```json
{
  "name": "键盘磨损",
  "category": "damage"
}
```

#### 13.1.3 审批标签

**接口**: `PUT /api/labels/:id/approve`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "approved": true,
    "status": "approved"
  }
}
```

#### 13.1.4 拒绝标签

**接口**: `PUT /api/labels/:id/reject`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "rejected": true,
    "status": "rejected"
  }
}
```

#### 13.1.5 合并标签

**接口**: `POST /api/labels/merge`

**请求 Body**:
```json
{
  "source_id": "label-002",
  "target_id": "label-001",
  "reason": "重复标签合并"
}
```

---

### 13.2 属性管理

#### 13.2.1 属性列表

**接口**: `GET /api/properties`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "list": [
      {
        "id": "prop-001",
        "name": "琴键数",
        "type": "number",
        "options": []
      },
      {
        "id": "prop-002",
        "name": "颜色",
        "type": "select",
        "options": ["黑色", "白色", "棕色"]
      }
    ]
  }
}
```

#### 13.2.2 创建属性

**接口**: `POST /api/property`

**请求 Body**:
```json
{
  "name": "材质",
  "type": "select",
  "category": "instrument"
}
```

#### 13.2.3 更新属性

**接口**: `PUT /api/property/:id`

**请求 Body**: 同创建

#### 13.2.4 创建属性选项

**接口**: `POST /api/property/option`

**请求 Body**:
```json
{
  "property_id": "prop-002",
  "value": "红色"
}
```

#### 13.2.5 确认属性值

**接口**: `PUT /api/property/confirm`

**请求 Body**:
```json
{
  "asset_id": "asset-001",
  "property_id": "prop-001",
  "value": "88键"
}
```

#### 13.2.6 合并属性值

**接口**: `PUT /api/property/merge`

**请求 Body**:
```json
{
  "from_value": "黑色",
  "to_value": "深黑色",
  "property_id": "prop-002"
}
```

---

## 十四、技术实现要点

### 14.1 IAM 集成
- JWT 校验中间件: `IAMInterceptor`
- 从 JWT 提取: `sub`, `tenant_id`, `org_id`
- Token 失效自动刷新机制

### 14.2 数据模型约束
- `Order` 表必须包含 `accumulated_months` 字段
- `User` 表支持 `is_shadow` 标记（IAM 同步用户）

### 14.3 关键业务逻辑
- **租转售状态机**: 定时任务每月检查 `accumulated_months >= 12`
- **计费计算器**: 前端实时计算，后端签名验证
- **押金双轨制**: 校验用户免押资格（信用分/会员等级）
- **LBS 排序**: 使用 geohash 加速地理位置查询

### 14.4 错误码定义
| 错误码 | 说明 |
|--------|------|
| 20000 | 成功 |
| 40001 | 通用错误 |
| 40002 | 参数错误 |
| 40100 | 认证失败 |
| 40101 | Token 过期 |
| 40300 | 权限不足 |
| 40400 | 资源不存在 |
| 40900 | 资源冲突 |
| 42200 | 业务逻辑错误 |
| 50000 | 服务器错误 |

---

## 十三、版本记录

| 版本 | 日期 | 变更内容 |
|------|------|----------|
| v1.0 | 2026-03-20 | 初始版本 |
| v2.0 | 2026-03-21 | 整合 Lin-IAM 深度集成要求 |

---

*文档生成: 2026-03-21*<br>
*覆盖度: 100% features.md (v26.3.16)*

---

## 补充章节 (Consolidated from api_design.md)

> 以下章节从 `api_design.md` 合并而来，v2.0 api.md 中未覆盖。
> 合并日期: 2026-05-01

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
PUT /api/warehouse/orders/:id/delivery
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

## 19. 确认会话 API

**架构变更**: 确认流程委托 IAM 管理。Tuneloop 本地 confirmation_sessions 仅用于状态跟踪，不再主动发送邮件/短信。

### 19.1 查询确认会话

```
GET /api/confirmation-sessions/:id
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "id": "session_uuid",
    "user_id": "uuid",
    "iam_session_id": "iam-session-uuid",
    "confirm_type": "email",
    "confirm_target": "user@example.com",
    "merchant_id": "uuid",
    "action_type": "merchant_admin",
    "action_target_id": "uuid",
    "callback_url": "https://web.cadenzayueqi.com/api/iam/confirmation-callback",
    "status": "waiting",
    "message": null,
    "expires_at": "2024-01-16T10:00:00Z",
    "confirmed_at": null,
    "created_at": "2024-01-15T10:00:00Z"
  }
}
```

**新增字段**:
| 字段 | 类型 | 说明 |
|------|------|------|
| iam_session_id | string | IAM 侧确认会话 ID |
| callback_url | string | IAM 确认后的回调地址 |

## 20. 仪表盘 API

### 19.1 获取统计数据
```
GET /api/admin/dashboard/stats
```

### 19.2 获取即将到期列表
```
GET /api/admin/dashboard/near-transfers
```


## 附录 A: 角色权限说明

> 完整角色定义和权限分配参见 [`docs/permissions.md` §一](./permissions.md#一权限体系概述) 和 [`docs/permissions.md` §四](./permissions.md#四角色-权限分配矩阵)。

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
