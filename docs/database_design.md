# TuneLoop 数据库设计文档

## 1. 概述

### 1.1 文档目的
本文档定义 TuneLoop 乐器租赁管理系统的数据库表结构设计。

### 1.2 数据库类型
- **PostgreSQL** (推荐)
- 使用 GORM 作为 ORM 框架

### 1.3 命名规范
- 表名: 蛇形命名 (snake_case)
- 主键: `id` (UUID 类型)
- 租户隔离: 所有业务表包含 `tenant_id` 字段
- 时间戳: 使用 `created_at`, `updated_at`

---

## 2. 表结构

### 2.1 users - 用户表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| iam_sub | VARCHAR(255) | UNIQUE, NOT NULL | IAM 统一身份标识 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX, NOT NULL | 组织 ID |
| name | VARCHAR(255) | | 用户姓名 |
| phone | VARCHAR(50) | | 手机号 |
| email | VARCHAR(255) | | 邮箱 |
| credit_score | INT | DEFAULT 600 | 信用评分 |
| deposit_mode | VARCHAR(20) | DEFAULT 'standard' | 押金模式 |
| is_shadow | BOOLEAN | DEFAULT true | 是否为影子用户 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

### 2.2 categories - 乐器分类表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| name | VARCHAR(100) | NOT NULL | 分类名称 |
| icon | VARCHAR | | 分类图标 (emoji 或 URL) |
| parent_id | UUID | | 父分类 ID (一级分类为 NULL) |
| level | INT | DEFAULT 1 | 层级 (1=一级, 2=二级) |
| sort | INT | DEFAULT 0 | 排序序号 |
| visible | BOOLEAN | DEFAULT true | 是否可见 |
| created_at | TIMESTAMP | | 创建时间 |

### 2.3 instruments - 乐器表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX | 组织 ID |
| category_id | UUID | INDEX | 分类 ID |
| category_name | VARCHAR(100) | | 分类名称 (冗余字段) |
| name | VARCHAR(255) | | 乐器名称 |
| brand | VARCHAR(100) | | 品牌 |
| model | VARCHAR(100) | | 型号 |
| level | VARCHAR(20) | | 等级 (已废弃, 使用 level_id) |
| level_name | VARCHAR(50) | | 等级名称 (已废弃) |
| level_id | UUID | INDEX | 等级 ID (引用 instrument_levels) |
| sn | VARCHAR(100) | | 序列号/识别码 |
| site | VARCHAR(255) | | 归属网点名称 (冗余) |
| site_id | UUID | INDEX | 归属网点 ID |
| current_site_id | UUID | INDEX | 当前所在网点 ID |
| description | TEXT | | 描述 |
| images | JSONB | DEFAULT '[]' | 图片 URL 数组 |
| video | VARCHAR(500) | | 视频 URL |
| specifications | JSONB | DEFAULT '{}' | 技术规格 |
| pricing | JSONB | DEFAULT '{}' | 定价信息 |
| stock_status | VARCHAR(20) | DEFAULT 'available' | 库存状态 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

**索引**:
- `idx_instruments_tenant_category` ON (tenant_id, category_id)
- `idx_instruments_tenant_status` ON (tenant_id, stock_status)

### 2.4 instrument_levels - 乐器等级表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| caption | VARCHAR(50) | UNIQUE, NOT NULL | 等级显示名称 |
| code | VARCHAR(20) | UNIQUE, NOT NULL | 等级代码 |
| sort_order | INT | DEFAULT 0 | 排序序号 |
| created_at | TIMESTAMP | | 创建时间 |

**示例数据**:
| code | caption |
|------|---------|
| beginner | 入门级 |
| intermediate | 中级 |
| advanced | 高级 |
| professional | 专业级 |

### 2.5 orders - 订单表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX | 组织 ID |
| user_id | UUID | NOT NULL, INDEX | 用户 ID |
| instrument_id | UUID | NOT NULL | 乐器 ID |
| level | VARCHAR(20) | NOT NULL | 租赁等级 |
| lease_term | INT | NOT NULL | 租赁期限 (月) |
| deposit_mode | VARCHAR(20) | DEFAULT 'standard' | 押金模式 |
| monthly_rent | DECIMAL(10,2) | NOT NULL | 月租金 |
| deposit | DECIMAL(10,2) | DEFAULT 0 | 押金金额 |
| accumulated_months | INT | DEFAULT 0 | 已累计月份 |
| status | VARCHAR(20) | DEFAULT 'pending', INDEX | 订单状态 |
| start_date | DATE | | 开始日期 |
| end_date | DATE | | 结束日期 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

**状态值**:
- `pending`: 待支付
- `paid`: 已支付
- `in_lease`: 租赁中
- `completed`: 已完成
- `cancelled`: 已取消

### 2.6 sites - 网点表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX | 组织 ID |
| parent_id | UUID | INDEX | 父网点 ID (顶级为 NULL) |
| manager_id | UUID | INDEX | 负责人 ID |
| name | VARCHAR(255) | NOT NULL | 网点名称 |
| address | VARCHAR(500) | | 地址 |
| type | VARCHAR(50) | | 网点类型 |
| latitude | DECIMAL(6,6) | | 纬度 |
| longitude | DECIMAL(6,6) | | 经度 |
| phone | VARCHAR(50) | | 联系电话 |
| business_hours | VARCHAR(100) | | 营业时间 |
| status | VARCHAR(20) | DEFAULT 'active' | 状态 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

### 2.7 site_images - 网点图片表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| site_id | UUID | NOT NULL | 网点 ID |
| url | VARCHAR(500) | NOT NULL | 图片 URL |
| sort_order | INT | DEFAULT 0 | 排序序号 |
| created_at | TIMESTAMP | | 创建时间 |

### 2.8 maintenance_tickets - 维修工单表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX | 组织 ID |
| order_id | UUID | NOT NULL | 关联订单 ID |
| instrument_id | UUID | NOT NULL | 乐器 ID |
| user_id | UUID | NOT NULL, INDEX | 用户 ID |
| problem_description | TEXT | | 问题描述 |
| images | JSONB | DEFAULT '[]' | 问题图片 |
| service_type | VARCHAR(20) | | 服务类型 |
| status | VARCHAR(20) | DEFAULT 'PENDING', INDEX | 状态 |
| assigned_site_id | UUID | | 分配网点 ID |
| technician_id | UUID | INDEX | 技师 ID |
| progress_notes | TEXT | | 进度备注 |
| repair_report | TEXT | | 维修报告 |
| repair_photos | JSONB | DEFAULT '[]' | 维修照片 |
| estimated_cost | DECIMAL(10,2) | DEFAULT 0 | 预估费用 |
| accepted_at | TIMESTAMP | | 受理时间 |
| completion_notes | TEXT | | 完工备注 |
| completion_photos | JSONB | DEFAULT '[]' | 完工照片 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |
| completed_at | TIMESTAMP | INDEX | 完成时间 |

**状态值**:
- `PENDING`: 待处理
- `PROCESSING`: 处理中
- `COMPLETED`: 已完成

### 2.9 inventory_transfers - 库存调拨表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX | 组织 ID |
| asset_id | UUID | INDEX, NOT NULL | 资产 ID (即 instrument_id) |
| from_site_id | UUID | NOT NULL | 源网点 ID |
| to_site_id | UUID | NOT NULL | 目标网点 ID |
| reason | TEXT | | 调拨原因 |
| status | VARCHAR(20) | DEFAULT 'pending' | 状态 |
| created_by | UUID | | 创建人 ID |
| created_at | TIMESTAMP | | 创建时间 |
| completed_at | TIMESTAMP | | 完成时间 |

### 2.10 ownership_certificates - 所有权证书表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX | 组织 ID |
| order_id | UUID | UNIQUE, NOT NULL | 关联订单 ID |
| user_id | UUID | INDEX | 用户 ID |
| instrument_id | UUID | INDEX | 乐器 ID |
| transfer_date | TIMESTAMP | | 转让日期 |
| certificate_url | VARCHAR(500) | | 证书 PDF URL |
| created_at | TIMESTAMP | | 创建时间 |

### 2.11 technicians - 技师表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX | 组织 ID |
| site_id | UUID | INDEX | 所属网点 ID |
| name | VARCHAR(100) | | 姓名 |
| phone | VARCHAR(50) | | 手机号 |

### 2.12 leases - 租赁记录表 (Legacy)

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| user_id | UUID | INDEX, NOT NULL | 用户 ID |
| instrument_id | UUID | INDEX, NOT NULL | 乐器 ID |
| start_date | DATE | NOT NULL | 开始日期 |
| end_date | DATE | NOT NULL | 结束日期 |
| monthly_rent | DECIMAL(10,2) | NOT NULL | 月租金 |
| deposit_amount | DECIMAL(10,2) | NOT NULL | 押金金额 |
| status | VARCHAR(20) | DEFAULT 'active', INDEX | 状态 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

### 2.13 deposits - 押金记录表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| lease_id | UUID | INDEX, NOT NULL | 租赁 ID |
| user_id | UUID | INDEX, NOT NULL | 用户 ID |
| amount | DECIMAL(10,2) | NOT NULL | 金额 |
| type | VARCHAR(20) | NOT NULL | 类型 |
| status | VARCHAR(20) | DEFAULT 'pending', INDEX | 状态 |
| transaction_date | DATE | NOT NULL | 交易日期 |
| notes | TEXT | | 备注 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

### 2.14 labels - 标签/别名表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| name | VARCHAR(100) | NOT NULL, INDEX | 标签名称 |
| alias | JSONB | DEFAULT '[]' | 别名列表 |
| audit_status | VARCHAR(20) | DEFAULT 'pending' | 审核状态 |
| normalized_to_id | UUID | INDEX | 标准化目标 ID |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

### 2.15 tenants - 租户表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| name | VARCHAR(100) | NOT NULL | 租户名称 |
| status | VARCHAR(20) | DEFAULT 'active' | 状态 |
| description | TEXT | | 描述 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

### 2.16 clients - OAuth 客户端表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| client_id | VARCHAR(100) | UNIQUE, NOT NULL | 客户端 ID |
| client_secret | VARCHAR(255) | | 客户端密钥 |
| name | VARCHAR(100) | | 名称 |
| redirect_uris | TEXT | | 允许的重定向 URI |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

### 2.17 properties - 乐器属性定义表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| name | VARCHAR(100) | NOT NULL | 属性名称 |
| property_type | VARCHAR(20) | NOT NULL | 属性类型 |
| is_required | BOOLEAN | DEFAULT false | 是否必填 |
| unit | VARCHAR(50) | | 单位 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

### 2.18 property_options - 属性选项表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| property_id | UUID | INDEX, NOT NULL | 属性 ID |
| value | VARCHAR(255) | NOT NULL | 选项值 |
| status | VARCHAR(20) | DEFAULT 'pending' | 状态 |
| alias | UUID | INDEX | 别名指向 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

### 2.19 instrument_properties - 乐器属性值表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| instrument_id | UUID | INDEX, NOT NULL | 乐器 ID |
| property_id | UUID | INDEX, NOT NULL | 属性 ID |
| value | VARCHAR(255) | | 属性值 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

### 2.20 brand_configs - 品牌配置表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UINT | PK | 主键 (自增) |
| tenant_id | UUID | INDEX | 租户 ID |
| client_id | VARCHAR(100) | UNIQUE, NOT NULL | 客户端 ID |
| primary_color | VARCHAR(20) | DEFAULT '#6366F1' | 主色调 |
| logo_url | VARCHAR(500) | | Logo URL |
| brand_name | VARCHAR(100) | | 品牌名称 |
| support_phone | VARCHAR(50) | | 客服电话 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

---

## 附录 A: 关系图

```
users (1) ---> (N) orders
users (1) ---> (N) maintenance_tickets
orders (1) ---> (1) instruments
orders (1) ---> (1) ownership_certificates
instruments (N) ---> (1) categories
instruments (N) ---> (1) instrument_levels
instruments (N) ---> (N) sites (via inventory_transfers)
instruments (N) ---> (N) properties (via instrument_properties)
categories (1) ---> (N) categories (self-reference)
sites (1) ---> (N) sites (self-reference)
sites (1) ---> (N) technicians
tenants (1) ---> (N) users
tenants (1) ---> (N) clients
```

---

## 附录 B: 索引汇总

| 表名 | 索引类型 | 索引字段 |
|------|----------|----------|
| users | INDEX | tenant_id |
| users | INDEX | iam_sub (UNIQUE) |
| categories | INDEX | tenant_id |
| instruments | INDEX | tenant_id |
| instruments | INDEX | category_id |
| instruments | INDEX | site_id |
| instruments | INDEX | level_id |
| instruments | INDEX | stock_status |
| orders | INDEX | tenant_id |
| orders | INDEX | user_id |
| orders | INDEX | instrument_id |
| orders | INDEX | status |
| sites | INDEX | tenant_id |
| sites | INDEX | parent_id |
| sites | INDEX | manager_id |
| maintenance_tickets | INDEX | tenant_id |
| maintenance_tickets | INDEX | order_id |
| maintenance_tickets | INDEX | instrument_id |
| maintenance_tickets | INDEX | user_id |
| maintenance_tickets | INDEX | technician_id |
| maintenance_tickets | INDEX | status |
| maintenance_tickets | INDEX | completed_at |

---

*Model: glm-5*
