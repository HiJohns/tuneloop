# TuneLoop API 文档

> 版本: v2.0 (整合 Lin-IAM 深度集成要求)
> 最后更新: 2026-03-21
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
        "stock_status": "available" // available, renting_out, maintenance
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

**接口**: `POST /api/orders`

**请求 Body**:
```json
{
  "instrument_id": "instr-001",
  "level": "professional",
  "lease_term": 12,
  "deposit_mode": "free",
  "delivery_type": "self_pickup", // self_pickup, delivery
  "delivery_address_id": "addr-001",
  "agreement_signed": true
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "order_id": "order-001",
    "payment_url": "https://pay.example.com/...",
    "first_payment_amount": 760,
    "created_at": "2026-03-21T10:30:00Z"
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

### 6.5 获取租赁协议

**接口**: `GET /api/orders/:id/contract`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "contract_id": "contract-001",
    "content": "<html>租用协议内容...</html>",
    "signature_required": true,
    "signed": false
  }
}
```

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

### 8.5 收藏列表

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

### 8.6 添加收藏

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

### 8.7 取消收藏

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

### 8.8 地址管理

#### 8.8.1 地址列表

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

#### 8.8.2 新增地址

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

#### 8.8.3 更新地址

**接口**: `PUT /api/user/addresses/:id`

**响应**: 同新增地址

---

#### 8.8.4 删除地址

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

### 10.4 RBAC 权限配置

**接口**: `GET /api/admin/permissions`

**响应**:
```json
{
  "code": 20000,
  "data": {
    "roles": [
      {
        "role_id": "role-001",
        "role_name": "网点管理员",
        "permissions": [
          "asset:read",
          "asset:write",
          "lease:read",
          "maintenance:write"
        ]
      }
    ]
  }
}
```

---

### 10.5 更新权限配置

**接口**: `PUT /api/admin/permissions`

**请求 Body**:
```json
{
  "role_id": "role-001",
  "permissions": [
    "asset:read",
    "asset:write",
    "lease:read"
  ]
}
```

**响应**:
```json
{
  "code": 20000,
  "data": {
    "updated": true
  }
}
```

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

## 十二、技术实现要点

### 12.1 IAM 集成
- JWT 校验中间件: `IAMInterceptor`
- 从 JWT 提取: `sub`, `tenant_id`, `org_id`
- Token 失效自动刷新机制

### 12.2 数据模型约束
- `Order` 表必须包含 `accumulated_months` 字段
- `User` 表支持 `is_shadow` 标记（IAM 同步用户）

### 12.3 关键业务逻辑
- **租转售状态机**: 定时任务每月检查 `accumulated_months >= 12`
- **计费计算器**: 前端实时计算，后端签名验证
- **押金双轨制**: 校验用户免押资格（信用分/会员等级）
- **LBS 排序**: 使用 geohash 加速地理位置查询

### 12.4 错误码定义
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
