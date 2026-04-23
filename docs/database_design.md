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
| is_system_admin | BOOLEAN | DEFAULT false | 是否为系统管理员 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

### 2.2 merchants - 商户表

**说明**: 商户对应 IAM 中的 Organization

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX, NOT NULL | IAM Organization ID |
| name | VARCHAR(255) | NOT NULL | 商户名称 |
| code | VARCHAR(100) | UNIQUE, NOT NULL | 商户代码（URL slug） |
| contact_name | VARCHAR(255) | | 联系人姓名 |
| contact_email | VARCHAR(255) | | 联系人邮箱 |
| contact_phone | VARCHAR(50) | | 联系人电话 |
| admin_uid | UUID | INDEX | 管理员用户 ID |
| status | VARCHAR(20) | DEFAULT 'active' | 状态 (active/inactive) |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

**索引**:
- `idx_merchants_tenant_code` UNIQUE (tenant_id, code)
- `idx_merchants_admin` (admin_uid)

### 2.3 site_members - 网点成员表

**说明**: 多对多关系表（users ↔ sites），支持用户属于多个网点

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| site_id | UUID | INDEX, NOT NULL | 网点 ID |
| user_id | UUID | INDEX, NOT NULL | 用户 ID |
| role | VARCHAR(20) | DEFAULT 'Staff' | 角色 (Manager/Staff) |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

**约束**:
- UNIQUE (tenant_id, site_id, user_id) — 同一用户在同一网点只能有一条记录

**索引**:
- `idx_site_members_site` (site_id, user_id)
- `idx_site_members_user` (user_id, site_id)

### 2.4 categories - 乐器分类表

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

**重要变更** (2026-04-16):
- 乐器不再有 `name`（名称）字段，完全由 `sn`（识别码）标识
- 品牌、型号等属性作为动态属性存在于 `instrument_properties` 表中
- 乐器的基本信息仅包含：识别码、分类、网点、等级

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX | 组织 ID |
| category_id | UUID | INDEX | 分类 ID |
| category_name | VARCHAR(100) | | 分类名称 (冗余字段) |
| sn | VARCHAR(100) | | **序列号/识别码（唯一标识）** |
| level_id | UUID | INDEX | 等级 ID (引用 instrument_levels) |
| level_name | VARCHAR(50) | | 等级名称 (冗余字段) |
| site_id | UUID | INDEX | 归属网点 ID |
| current_site_id | UUID | INDEX | 当前所在网点 ID |
| description | TEXT | | 描述 |
| images | JSONB | DEFAULT '[]' | 图片 URL 数组 |
| video | VARCHAR(500) | | 视频 URL |
| specifications | JSONB | DEFAULT '{}' | 技术规格 (JSON) |
| pricing | JSONB | DEFAULT '{}' | 定价信息 (JSON) |
| stock_status | VARCHAR(20) | DEFAULT 'available' | 库存状态 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

**已移除字段**:
- `name`: 乐器名称（不再使用，完全由 sn 标识）
- `brand`: 品牌（改为动态属性）
- `model`: 型号（改为动态属性）
- `level`: 等级字符串（已废弃，使用 level_id）
- `site`: 网点名称（已废弃，使用 site_id）

**索引**:
- `idx_instruments_tenant_category` ON (tenant_id, category_id)
- `idx_instruments_tenant_status` ON (tenant_id, stock_status)

**pricing JSONB 结构**:
```json
[
  {
    "name": "standard",      // 定价等级名称
    "daily_rent": 100.00,    // 日租金
    "monthly_rent": 2500.00, // 月租金
    "deposit": 5000.00,      // 押金
    "stock": 5               // 库存数量
  }
]
```

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

### 2.11 maintenance_workers - 维修师傅表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX | 组织 ID |
| site_id | UUID | INDEX | 所属网点 ID |
| name | VARCHAR(100) | | 姓名 |
| phone | VARCHAR(50) | | 手机号 |
| join_date | DATE | | 入职日期 |
| status | VARCHAR(20) | DEFAULT 'active' | 状态 (active/inactive) |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |
| deleted_at | TIMESTAMP | | 删除时间（软删除） |

### 2.12 maintenance_sessions - 维修会话表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX | 组织 ID |
| maintenance_ticket_id | UUID | NOT NULL | 关联维修工单 ID |
| worker_id | UUID | INDEX | 维修师傅 ID |
| status | VARCHAR(20) | DEFAULT 'pending', INDEX | 状态 |
| start_time | TIMESTAMP | | 开始时间 |
| end_time | TIMESTAMP | | 结束时间 |
| progress_notes | TEXT | | 进度备注 |
| completion_notes | TEXT | | 完工备注 |
| inspection_result | VARCHAR(20) | | 验收结果 (passed/failed) |
| inspection_comment | TEXT | | 验收备注 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

**状态值**:
- `pending`: 待分配
- `assigned`: 已分配
- `in_progress`: 维修中
- `completed`: 验收中
- `passed`: 验收通过
- `failed`: 验收不通过

### 2.13 maintenance_session_records - 维修记录表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| session_id | UUID | INDEX, NOT NULL | 维修会话 ID |
| record_type | VARCHAR(20) | | 记录类型 (comment/photo) |
| content | TEXT | | 记录内容 |
| photos | JSONB | DEFAULT '[]' | 照片数组 |
| created_at | TIMESTAMP | | 创建时间 |

### 2.14 leases - 租赁记录表 (Legacy)

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|

### 2.15 damage_reports - 定损报告表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX | 组织 ID |
| lease_id | UUID | NOT NULL, INDEX | 关联租赁会话 ID |
| instrument_id | UUID | NOT NULL | 乐器 ID |
| user_id | UUID | NOT NULL, INDEX | 用户 ID |
| damage_amount | DECIMAL(10,2) | | 定损金额 |
| damage_description | TEXT | | 定损说明 |
| damage_photos | JSONB | DEFAULT '[]' | 定损照片 |
| assessed_by | UUID | | 定损人 ID（员工） |
| assessed_at | TIMESTAMP | | 定损时间 |
| deposit_deducted | DECIMAL(10,2) | DEFAULT 0 | 已扣除押金 |
| status | VARCHAR(20) | DEFAULT 'pending', INDEX | 状态 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

**状态值**:
- `pending`: 待处理（用户未响应）
- `agreed`: 用户同意
- `appealed`: 用户申诉中
- `resolved`: 已解决
- `cancelled`: 已撤销

### 2.16 appeals - 申诉记录表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX | 组织 ID |
| damage_report_id | UUID | NOT NULL, INDEX | 关联定损报告 ID |
| user_id | UUID | NOT NULL, INDEX | 用户 ID |
| appeal_reason | TEXT | NOT NULL | 申诉理由 |
| evidence_photos | JSONB | DEFAULT '[]' | 证据照片 |
| status | VARCHAR(20) | DEFAULT 'pending', INDEX | 状态 |
| submitted_at | TIMESTAMP | | 申诉提交时间 |
| resolved_at | TIMESTAMP | | 申诉解决时间 |
| resolution | VARCHAR(20) | | 仲裁结果 (no_damage/adjust/confirm) |
| final_amount | DECIMAL(10,2) | | 最终确定金额 |
| manager_comment | TEXT | | 经理仲裁说明 |
| resolved_by | UUID | | 仲裁人 ID |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

**状态值**:
- `pending`: 待处理
- `reviewing`: 经理仲裁中
- `resolved`: 已处理
- `cancelled`: 用户撤销

### 2.17 damage_assessments - 定损评估记录表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX | 组织 ID |
| order_id | UUID | NOT NULL, INDEX | 关联订单 ID |
| instrument_id | UUID | NOT NULL | 乐器 ID |
| user_id | UUID | NOT NULL, INDEX | 用户 ID |
| assessed_by | UUID | INDEX | 评估人 ID（库管员工） |
| condition | VARCHAR(20) | INDEX | 验货结果 (good/damaged) |
| photos | JSONB | DEFAULT '[]' | 验收照片 |
| notes | TEXT | | 备注说明 |
| scan_time | TIMESTAMP | | 扫码时间 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

### 2.18 order_status_history - 订单状态历史表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX | 组织 ID |
| order_id | UUID | NOT NULL, INDEX | 关联订单 ID |
| status_from | VARCHAR(20) | | 原状态 |
| status_to | VARCHAR(20) | | 新状态 |
| notes | TEXT | | 状态变更说明 |
| changed_by | UUID | INDEX | 操作人 ID |
| changed_at | TIMESTAMP | | 变更时间 |

**说明**: 记录所有订单状态变更历史，用于追溯物流和租赁周期

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

### 2.19 lease_sessions - 租赁会话表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX | 组织 ID |
| order_id | UUID | NOT NULL, INDEX | 关联订单 ID |
| user_id | UUID | NOT NULL, INDEX | 用户 ID |
| instrument_id | UUID | NOT NULL | 乐器 ID |
| start_date | DATE | NOT NULL | 起租日期 |
| end_date | DATE | NOT NULL | 结束日期 |
| actual_end_date | DATE | | 实际归还日期 |
| status | VARCHAR(20) | DEFAULT 'active', INDEX | 状态 |
| delivery_address | JSONB | | 收货地址 |
| return_method | VARCHAR(20) | | 归还方式 |
| return_tracking | VARCHAR(100) | | 归还物流单号 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

**状态值**:
- `active`: 租赁中
- `expiring_soon`: 即将到期（3天内）
- `overdue`: 已逾期
- `return_requested`: 已申请归还
- `returning`: 归还中
- `completed`: 已完成
- `cancelled`: 已取消

### 2.20 electronic_contracts - 电子合同表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX | 组织 ID |
| order_id | UUID | NOT NULL, INDEX | 关联订单 ID |
| user_id | UUID | NOT NULL, INDEX | 用户 ID |
| instrument_id | UUID | NOT NULL | 乐器 ID |
| contract_url | VARCHAR(500) | NOT NULL | 合同 PDF URL |
| contract_number | VARCHAR(50) | UNIQUE | 合同编号 |
| generated_at | TIMESTAMP | NOT NULL | 生成时间 |
| status | VARCHAR(20) | DEFAULT 'active' | 状态 |
| created_at | TIMESTAMP | | 创建时间 |

**说明**: 支付完成后自动生成，作为租赁凭证存入用户资料

---

*Model: glm-5*
