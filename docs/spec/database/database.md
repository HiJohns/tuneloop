# TuneLoop 数据库设计文档

> 版本: v1.0
> 最后更新: 2026-09-10
> 来源: `backend/models/` + `backend/database/migrations` 实际 schema

## 一、 概述

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

## 二、 表结构

### 2.1 users - 用户表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| iam_sub | VARCHAR(255) | UNIQUE, NOT NULL | IAM 统一身份标识 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX, NOT NULL | 组织 ID |
| name | VARCHAR(255) | | 用户姓名 |
| phone | VARCHAR(50) | | 手机号 |
| email | VARCHAR(255) | | 邮箱（可选） |
| credit_score | INT | DEFAULT 600 | 信用评分 |
| deposit_mode | VARCHAR(20) | DEFAULT 'standard' | 押金模式 |
| is_shadow | BOOLEAN | DEFAULT true | 是否为影子用户 |
| is_system_admin | BOOLEAN | DEFAULT false | 是否为系统管理员 |
| status | VARCHAR(20) | DEFAULT 'pending' | 用户状态: active/pending |
| force_password_change | BOOLEAN | DEFAULT false | 首次登录强制改密 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

**状态说明**:
- `pending`: 新创建用户，待 IAM 确认。创建后由 IAM 激活，登录时校验
- `active`: 已激活用户。IAM 侧确认后本地同步为 active
- 创建时设密码或自动生成 → 本地直接为 `active`（跳过 IAM 确认）

**实名核身字段（#1787/#1789）**:
| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| face_verified | BOOLEAN | DEFAULT false | 是否已核身（人脸比对通过或人工审核通过） |
| face_verified_at | TIMESTAMPTZ | | 核身通过时间 |
| face_verify_method | VARCHAR(10) | | 核身来源：`tencent`=自动比对 / `manual`=人工审核；信息变更时清除 |

**核身五态派生**（`id_verify_status`，非列存储，派生函数输出）：`none` / `uploaded` / `pending_review` / `verified` / `rejected`（判定优先级见 docs/cases/id-photos.md §核身状态派生与消费）

### 2.1.1 face_capture_batches - 核身自拍采集批次表（#1789 T1）

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| user_id | UUID | NOT NULL, FK → users(id) ON DELETE CASCADE, INDEX(user_id, submitted_at DESC) | 所属用户 |
| status | VARCHAR(20) | NOT NULL, DEFAULT 'pending' | pending（待审核）/ approved（通过）/ rejected（驳回） |
| reject_reason | TEXT | | 驳回原因（rejected 时填写） |
| submitted_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | 自拍提交时间 |
| reviewed_by | VARCHAR(255) | | 审核人（平台员工本地 users 缓存 name/ID） |
| reviewed_at | TIMESTAMPTZ | | 审核时间 |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | 创建时间 |

**说明**: 每次自拍采集一条批次；人工审核通过/驳回后留痕（reviewed_by/at）；信息变更（real_name/id_card_no/证件照）时 pending 批次自动作废（rejected, reason=「身份信息已变更，请重新采集」）。自拍素材存 `media_assets`（source_type=face_capture，GC 豁免）。

迁移：`20260829003_face_capture_batches.{up,down}.sql`

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
| phone | VARCHAR(50) | | 联系电话 |
| address | TEXT | | 地址 |
| admin_uid | UUID | INDEX | 管理员用户 ID |
| admin_pending | BOOLEAN | DEFAULT false | 管理员待确认 |
| status | VARCHAR(20) | DEFAULT 'active' | 状态 (active/inactive) |
| merchant_type | VARCHAR(20) | DEFAULT 'full' | 商户类型: full/controlled |
| transit_address | TEXT | | 受控商户转发地址 |
| transit_phone | VARCHAR(50) | | 受控商户转发电话 |
| transit_contact_name | VARCHAR(255) | | 受控商户转发联系人 |
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

### 2.5 instruments - 乐器表

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
- `idx_instruments_category_sort` ON (category_id, sort_order) — #1797 分类内排序查询索引

**sort_order 列（#1797）**:
| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| sort_order | INT | DEFAULT 0, INDEX | 子分类内排序序号。0 = 未排序（列表退化为 created_at 序）；同一 category_id 组内通过 `PUT /instruments/:id/sort` 交换调整 |

迁移：`20260829002_instruments_sort_order.{up,down}.sql`

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

### 2.6 instrument_levels - 乐器等级表

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

### 2.7 instrument_properties - 乐器动态属性关联表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| instrument_id | UUID | NOT NULL, INDEX | 乐器 ID |
| property_id | UUID | NOT NULL, INDEX | 属性定义 ID |
| value | VARCHAR(255) | NOT NULL | 属性值 |
| alias | VARCHAR(255) | | 别名（用于搜索优化） |
| created_at | TIMESTAMP | | 创建时间 |

**说明**: 乐器与属性值的多对多关联表，通过此表实现乐器的动态属性（品牌、型号、年份等）。同一乐器可以有多个属性值。

### 2.8 instrument_photo_batches - 乐器照片批次表 ⚠️ DEPRECATED

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| instrument_id | UUID | NOT NULL, INDEX | 乐器 ID (外键) |
| batch_type | VARCHAR(20) | NOT NULL, INDEX | 批次类型: outbound/return/maintenance |
| storage_path | VARCHAR(500) | NOT NULL | ZIP 文件存储路径 |
| operator_id | UUID | INDEX | 拍照员工 ID (外键) |
| created_at | TIMESTAMP | NOT NULL | 批次创建时间 |

**说明**: 记录乐器照片批次信息。每次员工按拍照要求对乐器拍照，生成一个批次，同一天同一乐器的员工拍照归为同一批次，打包为 ZIP 文件存储。

> ⚠️ **已废弃**: 此表已被 `instrument_media` 表取代（见 `docs/topics/media/media_directory.md`）。新代码禁止写入此表，旧数据保留供历史查询。

**存储结构**:
```
uploads/photos/{tenant_id}/{instrument_sn}/batch_{timestamp}/
  ├─ photo1.jpg
  ├─ photo2.jpg
  └─ manifest.yaml
```

**manifest.yaml 格式**:
```yml
version: "1.0"
batch_id: uuid
instrument_id: uuid
instrument_sn: string
batch_type: outbound|return|maintenance
operator_id: uuid
tenant_id: uuid
created_at: RFC3339
photos:
  - filename: string
    position: string
    timestamp: RFC3339
    size: int64
```

---

### 2.8.1 instrument_promo_overrides - 乐器促销覆盖配置表（#1863）

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| instrument_id | UUID | INDEX, NOT NULL | 乐器 ID |
| override_type | VARCHAR(20), NOT NULL | | 覆盖类型：`discount` / `rebate` / `rent_to_own` |
| enabled | BOOL, NOT NULL, DEFAULT true | | 是否启用 |
| content | TEXT, NOT NULL, DEFAULT '' | | 自定义文案（rent_to_own 类型；空=默认文案） |
| updated_at | TIMESTAMP | | 更新时间 |

**用途**: 按乐器粒度控制促销模块的可见性和文案。`rent_to_own` 类型控制移动端详情页的租购转化模块（默认可见）。

---

### 2.9 orders - 订单表

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
| end_date | DATE | | 租期末日（start_date + rent_days − 1，#1847）；「预期归还日」展示口径 = start_date + rent_days |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

**状态值** (参考 state-machine.md §1.1 标准):
- `in_store`: 在库/待租
- `reserved`: 已预约
- `paid`: 已支付/待发货
- `shipped`: 运输中
- `in_lease`: 租赁中
- `returning`: 归还中
- `maintenance`: 维修中

> **注意**: 以上为标准状态机值。所有 handler 中的状态字符串已使用 `models.OrderStatus*` 常量替代。

### 2.10 sites - 网点表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX | 网点自身的 IAM 组织 ID（非商户组织 ID，创建网点时由 IAM Create Organization 返回） |
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

### 2.11 site_images - 网点图片表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| site_id | UUID | NOT NULL | 网点 ID |
| url | VARCHAR(500) | NOT NULL | 图片 URL |
| sort_order | INT | DEFAULT 0 | 排序序号 |
| created_at | TIMESTAMP | | 创建时间 |

### 2.12 maintenance_tickets - 维修工单表

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

### 2.13 inventory_transfers - 库存调拨表

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

### 2.14 ownership_certificates - 所有权证书表

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

### 2.15 maintenance_workers - 维修师傅表

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

### 2.16 maintenance_sessions - 维修会话表

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

### 2.17 maintenance_session_records - 维修记录表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| session_id | UUID | INDEX, NOT NULL | 维修会话 ID |
| record_type | VARCHAR(20) | | 记录类型 (comment/photo) |
| content | TEXT | | 记录内容 |
| photos | JSONB | DEFAULT '[]' | 照片数组 |
| created_at | TIMESTAMP | | 创建时间 |

### 2.18 leases - 租赁记录表 (Legacy)

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|

### 2.19 damage_reports - 定损报告表

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
| condition | VARCHAR(20) | | 验收结果（good/damaged，#1708 并入） |
| notes | TEXT | | 备注说明（#1801 追缴费用区块第 4 输入框） |
| scan_time | TIMESTAMP | | 验收扫描时间 |
| overdue_days | INT | DEFAULT 0 | 逾期天数 |
| overdue_fee | BIGINT | DEFAULT 0 | 逾期费（分，#1743 Cents 列） |
| additional_shipping_fee | BIGINT | DEFAULT 0 | 追加物流费（分，#1801 归还验收时员工填写，结算合入 shipping 费用） |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

**状态值**:
- `pending`: 待处理（用户未响应）
- `agreed`: 用户同意
- `appealed`: 用户申诉中
- `resolved`: 已解决
- `cancelled`: 已撤销

### 2.20 appeals - 申诉记录表

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

### 2.21 damage_assessments - 定损评估记录表

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

### 2.22 order_status_history - 订单状态历史表

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

### 2.23 order_payment_records - 支付记录表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX | 组织 ID |
| user_id | UUID | INDEX, NOT NULL | 支付用户 ID |
| order_id | UUID | INDEX | 关联订单 ID |
| session_id | UUID | | 两阶段注册会话 ID（#1663；RawResponse 被回调覆盖后仍存续） |
| order_type | VARCHAR(20) | NOT NULL | 订单类型: rent/renewal/damage/repair/payment_shortfall/membership |
| out_trade_no | VARCHAR(32) | UNIQUE INDEX | 微信商户订单号 |
| transaction_id | VARCHAR(64) | | 微信支付单号 |
| openid | VARCHAR(128) | | 支付者 openid（#1731 回调权威源） |
| amount | BIGINT | NOT NULL | 金额（分，#1727 P2 起） |
| type | VARCHAR(20) | NOT NULL, DEFAULT 'payment' | 记录类型: payment |
| status | VARCHAR(20) | NOT NULL, DEFAULT 'pending' | 状态: pending/paid/failed |
| method | VARCHAR(20) | | 支付方式: jsapi/native/waived |
| prepay_id | VARCHAR(64) | | 预支付 ID |
| code_url | TEXT | | Native 支付二维码 URL |
| fail_reason | TEXT | | 失败原因 |
| raw_response | JSONB | | 原始响应（**会被微信回调覆盖**，见 days 列说明） |
| reminded_at | TIMESTAMP | | 催缴幂等标记（#1749） |
| **days** | INTEGER | | **续费天数（#1802 T1 独立持久化）**：仅 renewal 类型记录使用；RawResponse 会被微信回调（processPaymentCallback）覆盖，续费天数不再依赖其中 meta，改由此列读取 |
| **coupon_code** | VARCHAR(32) | | **本笔支付优惠码（#1853 逐笔入库）**：仅优惠笔写入；orders.coupon_code 为最近一笔覆盖式快照，此为逐笔明细 |
| **coupon_discount** | BIGINT | NOT NULL, DEFAULT 0 | **本笔支付折扣（分，#1853）**：优惠笔 = 当笔原价 − 当笔折后实付；无码为 0 |
| created_at | TIMESTAMPTZ | NOT NULL | 创建时间 |
| updated_at | TIMESTAMPTZ | NOT NULL | 更新时间 |

**coupon 列说明（#1853）**：
- 背景：orders.coupon_code/coupon_discount 为覆盖式快照（#1744），多笔优惠单（合同+多次续费各用码）只能还原最近一笔 → 无法按笔对账/审计
- 写入点：`wechatpay_prepay.go`（订单支付/补缴，record 随 Create 落库）与 `renewal.go`（续费确认，Create 后补写）；无码笔保持 NULL/0
- 历史记录不可回填（写入时未存）→ 列留空；orders.coupon_* 快照保留
- 迁移：`20260909001_payment_coupon_columns.{up,down}.sql`

**days 列说明（#1802 T1）**：
- 背景：续费天数（AdditionalDays）原存 `raw_response` JSON 中，但微信支付回调在 `applyRenewalSideEffects` **之前**覆盖 raw_response 为回调结果 → 真实回调路径续费天数丢失（潜在既有 bug）。
- 修复：`ConfirmRenewal` 创建记录时写入 `days` 列；`applyRenewalSideEffects` 优先从 `days` 读取，历史记录（无 days）fallback 到 raw_response meta。
- 历史续费记录天数不可回填 → 订单详情实付段该次续费降级展示（金额+日期，无阶梯明细）。
- 迁移：`20260829001_order_payment_records_days.{up,down}.sql`

**说明**: 每笔支付（首期租金/续费/定损/补缴/会员费）一行，paid 记录为订单详情实付段（fee_detail.paid_block）数据源。

### 2.24 audit_logs - 审计日志表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX | 组织 ID |
| user_id | UUID | INDEX, NOT NULL | 操作人 ID |
| actor_role | VARCHAR(50) | | 操作时的 IAM 角色 |
| action | VARCHAR(50) | NOT NULL | 操作类型: CREATE/UPDATE/DELETE/PAY/PICKUP/RETURN/CANCEL/TRANSFER/SYNC/IMPORT/LOGIN |
| resource_type | VARCHAR(50) | NOT NULL | 资源类型: user/merchant/site/order/instrument/lease/maintenance_ticket/... |
| resource_id | VARCHAR(100) | | 操作资源 ID |
| details | JSONB | | 变更详情（JSON） |
| request_body | JSONB | | 请求体（CRITICAL 操作记录，截断至 10KB） |
| ip_address | VARCHAR(45) | | 客户端 IP |
| user_agent | VARCHAR(500) | | 客户端 UA |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | 记录时间 |

**索引**:
- `idx_audit_logs_tenant_id` (tenant_id)
- `idx_audit_logs_org_id` (org_id)
- `idx_audit_logs_user_id` (user_id)
- `idx_audit_logs_resource` (resource_type, resource_id)
- `idx_audit_logs_created_at` (created_at)

**说明**: 通过 Gin 中间件 + 异步写入器记录所有 CRITICAL/HIGH 操作。日志保留 1 年，每日定时清理。

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

### 2.25 lease_sessions - 租赁会话表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX | 组织 ID |
| order_id | UUID | NOT NULL, INDEX | 关联订单 ID |
| user_id | UUID | NOT NULL, INDEX | 用户 ID |
| instrument_id | UUID | NOT NULL | 乐器 ID |
| start_date | DATE | NOT NULL | 起租日期 |
| end_date | DATE | NOT NULL | 租期末日（start_date + rent_days − 1，#1847）|
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

### 2.26 electronic_contracts - 电子合同表

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

### 2.27 confirmation_sessions - 确认会话表

**说明**: 本地状态跟踪表。确认流程委托 IAM 管理，IAM 确认后通过回调同步状态。本表不再主动创建会话，仅在回调时记录/更新。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX, NOT NULL | 租户 ID |
| org_id | UUID | INDEX | 组织 ID |
| user_id | UUID | NOT NULL, INDEX | 待确认用户 ID |
| iam_session_id | VARCHAR(100) | INDEX | IAM 侧确认会话 ID（关联 IAM Redis 会话） |
| callback_url | VARCHAR(500) | | IAM 确认后回调地址 |
| confirm_type | VARCHAR(20) | NOT NULL | 确认方式: email/phone |
| confirm_target | VARCHAR(255) | NOT NULL | 确认目标邮箱或手机号 |
| merchant_id | UUID | INDEX | 关联商户 ID |
| action_type | VARCHAR(50) | NOT NULL | 确认后执行动作: merchant_admin/site_manager/site_staff |
| action_target_id | UUID | | 动作目标 ID（商户 ID 或网点 ID） |
| status | VARCHAR(20) | DEFAULT 'waiting', INDEX | 会话状态 |
| message | TEXT | | 状态信息（如失败原因） |
| token | VARCHAR(100) | UNIQUE | 确认令牌（用于邮件链接） |
| expires_at | TIMESTAMP | NOT NULL | 过期时间（创建时间+24h） |
| confirmed_at | TIMESTAMP | | 确认时间 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

**状态值**:
- `waiting`: 等待确认（IAM 会话已创建）
- `confirmed`: 已确认（IAM 回调 result=accept）
- `failed`: 失败（IAM 回调 result=failed）
- `rejected`: 已拒绝（IAM 回调 result=reject）
- `expired`: 已过期（IAM 24h 超时）

**action_type 与 IAM confirm_type 映射**:
| Tuneloop action_type | IAM confirm_type | 说明 |
|----------------------|------------------|------|
| merchant_admin | create_org | 商户创建时 IAM 自动处理管理员关联 |
| site_manager | bind | 网点管理员绑定（下级组织仅通知，通常不创建会话） |
| site_staff | bind | 网点成员绑定（同上） |

**索引**:
- `idx_confirmation_sessions_token` UNIQUE (token)
- `idx_confirmation_sessions_iam_session` (iam_session_id)
- `idx_confirmation_sessions_user` (user_id, status)
- `idx_confirmation_sessions_expires` (expires_at, status)

---

### 2.28 pricing_templates - 定价模板表

**说明**: 系统定价策略模板表。其中最多仅有一条记录的 `is_system_default = true`，作为商户未自定义配置时的全局回退策略。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| code | VARCHAR(50) | UNIQUE, NOT NULL | 模板代码 |
| name | VARCHAR(100) | NOT NULL | 模板名称 |
| description | TEXT | | 模板描述 |
| config_schema | JSONB | NOT NULL, DEFAULT '{}' | 定价配置 Schema（含 tiers、deposit_mode 等） |
| is_active | BOOLEAN | DEFAULT true | 是否启用 |
| is_system_default | BOOLEAN | DEFAULT false | 是否为系统默认策略（全局回退用） |
| created_at | TIMESTAMP | | 创建时间 |

**索引**:
- `idx_pricing_templates_code` UNIQUE (code)
- `idx_pricing_templates_system_default` UNIQUE (is_system_default) WHERE is_system_default = true

---

### 2.29 merchant_pricing_configs - 商户定价配置表

**说明**: 商户自定义定价配置。每个商户最多一条记录。无记录时回退到 `pricing_templates` 中 `is_system_default = true` 的模板。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | UNIQUE, NOT NULL | 租户 ID |
| template_id | UUID | NOT NULL, FK → pricing_templates.id | 关联模板 ID |
| config | JSONB | NOT NULL, DEFAULT '{}' | 商户实际生效的定价配置 |
| updated_by | UUID | | 最后更新人 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

**索引**:
- `idx_merchant_pricing_configs_tenant` UNIQUE (tenant_id)

---


### 2.30 repair_records - 维修记录表（租赁乐器维修）

**说明**: 租赁乐器维修过程记录（师傅操作留痕）。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| instrument_id | UUID | NOT NULL, FK → instruments.id | 乐器 ID |
| worker_id | varchar(255) | NOT NULL | 维修师傅 ID |
| comment | text | | 记录内容 |
| photos | jsonb | DEFAULT '[]' | 照片 |
| created_at | TIMESTAMP | | 创建时间 |

---

### 2.31 user_instruments - 客户自有乐器表

**说明**: 顾客自有乐器（客户报修入口，SN 查自有乐器）。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| user_id | varchar(255) | NOT NULL | 用户 ID |
| sn | varchar(255) | NOT NULL, INDEX | 乐器序列号 |
| instrument_type | varchar(100) | | 乐器类型 |
| brand | varchar(100) | | 品牌 |
| model | varchar(100) | | 型号 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

**索引**:
- `idx_user_instruments_user_sn` (user_id, sn)

---

### 2.32 repair_requests - 报修单表

**说明**: 客户报修单（v3 主流程 + 维修服务分支 type='service'）。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | INDEX | 租户 ID |
| site_id | UUID | INDEX | 网点 ID |
| user_id | varchar(255) | NOT NULL, INDEX | 报修人 |
| user_instrument_id | UUID | INDEX | 自有乐器（warranty 分支） |
| status | varchar(20) | DEFAULT 'pending_ship' | 状态（见 §3.3 枚举） |
| merchant_type | varchar(10) | DEFAULT 'full' | v3: full / controlled |
| transit_site_id | UUID | INDEX | v3: 中转网点（受控路径） |
| controlled_site_id | UUID | INDEX | v3: 报价受控网点 |
| accepted_quote_id | UUID | INDEX | v3: 已接受报价 ID |
| check_fee_snapshot | bigint | | v3: 支付时系统检查费快照（分） |
| paid_amount | bigint | | v3: 已付总额（分） |
| expire_at | TIMESTAMP | | v3: 待估价过期时间 |
| reminder_sent | boolean | DEFAULT false | v3: 24h 提醒标记 |
| description | text | | 描述 |
| photos | jsonb | DEFAULT '[]' | 照片 |
| video_url | varchar(500) | | 视频 |
| quote_amount | bigint | | 已废弃：v3 用 repair_quotes |
| inspection_fee | bigint | | 已废弃：v3 用 check_fee_snapshot |
| shipping_fee | bigint | | 物流费 |
| tracking_company | varchar(100) | | 寄件物流公司 |
| tracking_number | varchar(100) | | 寄件物流单号 |
| return_company | varchar(100) | | 发回物流公司 |
| return_tracking_number | varchar(100) | | 发回物流单号 |
| worker_id | varchar(255) | | 维修师傅 |
| type | varchar(20) | DEFAULT 'warranty', INDEX | #1942: warranty（报修）/ service（维修服务） |
| repair_code | varchar(6) | UNIQUE | #1942: 6 位唯一编码（service 分支） |
| technician_id | UUID | INDEX | #1942: 师傅指派 |
| quote_repair_cents | bigint | | #1942: 报价修理费（分） |
| quote_logistics_cents | bigint | | #1942: 报价物流费预估（分） |
| quote_status | varchar(20) | DEFAULT '' | #1942: 报价状态 |
| adjusted_quote_cents | bigint | | #1942: 加价后新总价（分） |
| incurred_repair_cents | bigint | | #1942: 到此为止修理费（分） |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |
| closed_at | TIMESTAMP | | 关闭时间 |

**索引**:
- `idx_repair_requests_status` (status)
- `idx_repair_requests_type` (type)

---

### 2.33 repair_quotes - 报价单表

**说明**: 报修报价单（v3 多师傅竞价）。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| repair_request_id | UUID | NOT NULL, INDEX, FK → repair_requests.id | 报修单 ID |
| site_id | UUID | INDEX | 报价网点 |
| worker_id | varchar(255) | NOT NULL | 报价师傅 |
| quote_no | varchar(30) | UNIQUE | 报价单号 |
| material_fee | bigint | NOT NULL | 材料费（分） |
| service_fee | bigint | NOT NULL | 服务费（分） |
| logistics_fee | bigint | | 物流费 C 段（分） |
| duration | varchar(100) | | 工期 |
| comment | text | | 报价说明 |
| is_renegotiation | boolean | DEFAULT false | 是否重新协商 |
| status | varchar(20) | DEFAULT 'pending' | pending/accepted/rejected/superseded |
| created_at | TIMESTAMP | | 创建时间 |

---

### 2.34 repair_transit_orders - 中转单表

**说明**: 报修中转单（v3 受控路径转入/转出）。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| repair_request_id | UUID | INDEX, FK → repair_requests.id | 报修单 ID |
| transit_site_id | UUID | NOT NULL, INDEX | 中转网点 |
| controlled_site_id | UUID | INDEX | 受控网点 |
| direction | varchar(10) | | v3: in / out |
| status | varchar(20) | DEFAULT 'pending_activation' | pending_activation/active/received/relayed |
| transit_service_fee | bigint | | 中转服务费（分） |
| transit_logistics_fee | bigint | | 中转物流费 B+D 段（分） |
| note | text | | 备注 |
| unpack_photos | jsonb | DEFAULT '[]' | 拆箱照片 |
| repack_company | varchar(100) | | 重装物流公司 |
| repack_tracking_number | varchar(100) | | 重装物流单号 |
| transit_order_number | varchar(50) | | 中转单号 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

---

### 2.35 repair_request_records - 报修日志表

**说明**: 报修日志（v3 承载重新协商时间线）。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| repair_request_id | UUID | NOT NULL, INDEX, FK → repair_requests.id | 报修单 ID |
| worker_id | varchar(255) | | 操作人 |
| comment | text | | 内容 |
| photos | jsonb | DEFAULT '[]' | 照片 |
| record_type | varchar(20) | | 记录类型 |
| created_at | TIMESTAMP | | 创建时间 |

---

### 2.36 repair_logistics_fees - 维修服务分段物流费表（#1942）

**说明**: 维修服务（type='service'）每段实际物流费，经手员工实填。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| repair_id | UUID | NOT NULL, INDEX, FK → repair_requests.id | 维修单 ID |
| leg | int | NOT NULL | 物流段序号 |
| amount_cents | bigint | NOT NULL, DEFAULT 0 | 本段实际物流费（分） |
| filled_by | varchar(255) | | 经手员工 |
| created_at | TIMESTAMP | | 创建时间 |

---

### 2.37 repair_reviews - 维修服务评价表（#1942）

**说明**: 维修服务评价（RS-09）：评分/留言/拍照，PC 后台可见。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| repair_id | UUID | NOT NULL, UNIQUE, FK → repair_requests.id | 维修单 ID |
| user_id | UUID | NOT NULL, INDEX | 评价用户 |
| rating | int | NOT NULL | 评分 1-5 |
| message | text | | 留言 |
| photos | jsonb | DEFAULT '[]' | 照片 |
| created_at | TIMESTAMP | | 创建时间 |

---

### 2.38 报修单状态枚举（v3 + 维修服务）

**报修单（warranty）状态**:
```
pending_assessment(待估价) → pending_payment(待付款) → pending_ship(待发送)
  → shipping(已发货) → repairing(维修中) → return_pending(待发回) → returned(已发回) → closed(已关闭)
```
受控路径插入中转态：`transit_processing`（早期定价扇出）→ ... → `transit_in`（转入中）→ repairing → ... → `transit_out`（转出中）。

废弃旧态：`inspecting` / `quoted` / `pending_cancel`（v3 由 `pending_assessment` + 报价单表取代）。

**维修服务（type='service'）状态**:
```
pending_quote(待报价) → paid(已支付) → adjust_pending(加价待响应) → done_repair(已修完待发回) → ...
```

---

### 2.39 乐器维修状态枚举

```
repair_pending     → 待维修（定损后自动设置）
repair_in_progress → 维修中（师傅扫码开始）
repair_completed   → 已修复（师傅完成）
```
验收通过后 clear，`stock_status` 回 available。



### 2.40 membership_levels - 会员级别表

**说明**: 会员级别配置，按跨商户累计消费金额自动升级。级别名称和门槛金额由管理员在后台配置。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | int | PK | 级别 ID，数字越大级别越高 |
| name | varchar(50) | NOT NULL | 级别名称（由管理员定义） |
| min_amount | bigint | NOT NULL | 晋升门槛（分；Cents） |

> **数据说明**: 各级别名称和门槛金额由运营人员在管理后台设置，此处不预设默认值。

---

### 2.41 promo_plans - 促销方案表

**说明**: 促销方案活动（时间限定）。`plan_type` 可为 `discount_policy`（已弃用）/ `promo_campaign`；`scope_type` 可为 `system` / `merchant` / `site`（折扣政策不可用 `site`）。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| plan_type | varchar(20) | NOT NULL, DEFAULT 'promo_campaign' | discount_policy（弃用）/ promo_campaign |
| scope_type | varchar(20) | NOT NULL | system / merchant / site |
| scope_id | UUID | | 商户或网点 ID（system 级为空） |
| name | varchar(100) | NOT NULL | 方案名称 |
| start_date | date | | 开始日期（null = 长期） |
| end_date | date | | 结束日期（null = 长期） |
| stackable | bool | NOT NULL, DEFAULT false | 是否可叠加 |
| is_active | bool | NOT NULL, DEFAULT true | 是否启用 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

---

### 2.42 promo_plan_details - 促销方案明细表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| promo_plan_id | UUID | NOT NULL, INDEX, FK → promo_plans.id | 促销方案 ID |
| level_id | int | NOT NULL | 会员级别 ID |
| rent_discount | decimal(5,4) | | 租金折扣率（仅促销活动 promo_campaign；会员折扣已移除 #1543） |
| deposit_discount | decimal(5,4) | | 押金折扣率 |
| overdue_discount | decimal(5,4) | | 逾期租金折扣率 |

---

### 2.43 rebate_config - 返点配置表

> ⚠️ **已废弃（#1899 方案 A）**：返点配置页/接口已移除，实际返点由 `gift_policies.refund_ratio` 承担；本节仅存档参考。表与模型仍在（`backend/models/membership.go` RebateConfig、迁移 072/073）。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| level_id | int | NOT NULL, UNIQUE, FK → membership_levels.id | 会员级别 |
| rent_ratio | decimal(5,4) | NOT NULL, DEFAULT 0.01 | 返点与租金比例 |
| is_active | bool | NOT NULL, DEFAULT true | 是否启用 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

---

### 2.44 points_policies - 点数政策表（旧体系，v2 并入 gift_policies）

> ⚠️ **旧体系**：`points_policies.max_pay_ratio` 已并入 `gift_policies.pay_ratio`（#1939 v2）。表仍存在，作为历史存档。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| scope_type | varchar(20) | NOT NULL | system / merchant / site |
| scope_id | UUID | | 商户或网点 ID |
| max_pay_ratio | decimal(5,4) | | 可支付价格百分比上限 |
| valid_days | int | | 有效期（天） |
| is_active | bool | NOT NULL, DEFAULT true | 是否启用 |

优先级：网点 > 商户 > 系统。

---

### 2.45 gift_policies - 赠点策略表（v2 权威，#1939）

**说明**: 平台统一管理的点数规则，按会员级别独立设置（level_id=0 为默认兜底行）。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| level_id | int | NOT NULL, UNIQUE | 会员级别（0 = 兜底） |
| pay_ratio | decimal(5,4) | NOT NULL, DEFAULT 0.3 | 赠点使用比例（支付时抵扣上限） |
| refund_ratio | decimal(5,4) | NOT NULL, DEFAULT 0 | 退款返点比例（M-06 已取消"退款返点给自己"，字段保留） |
| is_active | bool | NOT NULL, DEFAULT true | 是否启用 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

> **M-04 裂变奖励比例**（`referral_ratio`，乐手 2% / 首席 5% / 演奏家 8%）：为手册承诺设计（cases/membership.md M-04），**后端尚未实现**（gift_policies 无此列），待立项。

---

### 2.46 membership_gift_ratios - 会员赠点比例表（旧体系，退役）

> ⚠️ **已退役（M-06）**：`SelfSpendRatio` / `ReferralSpendRatio` 分支废除（#1939 v2），表保留存档。裂变奖励由 `gift_policies.referral_ratio`（待实现）承担。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| level_id | int | NOT NULL, UNIQUE, FK → membership_levels.id | 会员级别 |
| self_spend_ratio | decimal(5,4) | NOT NULL, DEFAULT 0 | 自消费赠点比例（退役） |
| referral_reg_points | decimal(10,2) | NOT NULL, DEFAULT 0 | 推荐注册赠点（退役） |
| referral_spend_ratio | decimal(5,4) | NOT NULL, DEFAULT 0 | 推荐消费赠点比例（退役） |
| is_active | bool | NOT NULL, DEFAULT true | 是否启用 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

---

### 2.47 membership_level_benefits - 会员权益表（#1830）

**说明**: 每档会员权益文案（标题+说明），PC「系统管理 → 会员级别管理 → 权益」维护，移动端「会员中心」按 level_id 渲染。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| level_id | int | NOT NULL, INDEX | 会员级别 |
| sort_order | int | NOT NULL, DEFAULT 0 | 排序 |
| title | varchar(100) | NOT NULL | 权益标题 |
| description | varchar(500) | NOT NULL, DEFAULT '' | 权益说明 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

---

### 2.48 settlements - 结算表（#1738）

> ⚠️ 补充核对表：结算记录，会员累计消费权威来源之一（`settlements.actual_rent_amount`）。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| order_id | UUID | NOT NULL, INDEX | 订单 ID |
| actual_rent_days | int | NOT NULL, DEFAULT 0 | 实际租期天数 |
| actual_rent_amount | bigint | NOT NULL, DEFAULT 0 | 实际租金（分） |
| original_rent_amount | bigint | NOT NULL, DEFAULT 0 | 原租金（分） |
| gift_points_refunded | bigint | NOT NULL, DEFAULT 0 | 退还赠点（分） |
| cash_refundable | bigint | NOT NULL, DEFAULT 0 | 应退现金（分） |
| prepaid_refunded | bigint | NOT NULL, DEFAULT 0 | 退还预付（分） |
| refund_method | varchar(20) | NOT NULL, DEFAULT 'prepaid' | 退款方式 |
| refund_status | varchar(20) | NOT NULL, DEFAULT 'pending' | 退款状态 |
| overdue_charges_total | bigint | NOT NULL, DEFAULT 0 | 逾期扣款合计（分） |
| breakdown | jsonb | NOT NULL, DEFAULT '{}' | 结算明细 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

---

### 2.49 order_refund_records - 订单退款记录表

> ⚠️ 补充核对表：退款记录，会员累计消费扣除退款时用（`status='refunded'`）。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | NOT NULL | 租户 ID |
| payment_record_id | UUID | INDEX | 支付记录 ID |
| out_refund_no | varchar(32) | UNIQUE | 外部退款单号 |
| refund_id | varchar(64) | | 微信退款 ID |
| amount | bigint | NOT NULL | 退款金额（分） |
| reason | varchar(200) | | 退款原因 |
| status | varchar(20) | NOT NULL, DEFAULT 'pending' | 退款状态 |
| fail_reason | text | | 失败原因 |
| raw_response | jsonb | | 原始响应 |
| created_at | TIMESTAMP | | 创建时间 |
| updated_at | TIMESTAMP | | 更新时间 |

---

### 2.50 instrument_promo_overrides - 乐器促销覆盖表

**说明**: 单件乐器是否适用促销政策（折扣/返点）。网点管理员可开关，不能修改政策本身。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| tenant_id | UUID | NOT NULL | 租户 ID |
| instrument_id | UUID | NOT NULL, FK → instruments.id (ON DELETE CASCADE) | 乐器 ID |
| override_type | varchar(20) | NOT NULL | discount / rebate |
| enabled | bool | NOT NULL, DEFAULT true | 是否适用 |
| content | text | NOT NULL, DEFAULT '' | 覆盖内容 |
| updated_at | TIMESTAMP | | 更新时间 |

**约束**: UNIQUE (tenant_id, instrument_id, override_type)

---

### 2.51 会员域字段补充

**users 表新增字段**（FK → membership_levels.id）：
| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| membership_level_id | int | | 会员级别 ID（null = 未定级） |
| total_spending | bigint | DEFAULT 0 | 累计消费（分；#1542 实时聚合为主，此字段仅展示缓存） |
| promo_points | bigint | DEFAULT 0 | 乐币余额（分，1 乐币 = 1 元；#1757） |

**instruments 表新增字段**：
| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| min_membership_level | int | | 最低可租会员级别 ID（null = 无限制） |

**orders 表新增字段**：
| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| pricing_breakdown | jsonb | | 计费快照（订单创建时写入不可修改） |
| promo_points | bigint | DEFAULT 0 | 赠点抵扣（分） |
| gift_points_used | bigint | NOT NULL, DEFAULT 0 | 乐币使用（分） |
| points_policy_snapshot | jsonb | | 点数政策快照 |
| gift_points_refunded | bigint | NOT NULL, DEFAULT 0 | 已退赠点（分） |

**merchants 表新增字段**：
| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| rebate_opt_in | bool | NOT NULL, DEFAULT true | 商户是否参与返点（#1899 方案 A 保留） |

---


### 2.52 invoice_applications - 发票申请表（#1786）

**说明**: 电子发票申请，每个商户分组一张申请（用户一次提交按商户分组创建多张）。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | UUID | PK, DEFAULT gen_random_uuid() | 主键 |
| user_id | UUID | NOT NULL, INDEX | 申请用户 |
| tenant_id | UUID | NOT NULL, INDEX | 商户 ID |
| status | varchar(20) | NOT NULL, DEFAULT 'pending' | pending / replied |
| total_amount | bigint | NOT NULL, DEFAULT 0 | 申请金额合计（分） |
| order_count | int | NOT NULL, DEFAULT 0 | 关联订单数 |
| reply | text | | 商户回复 |
| invoice_file | text | | 发票文件 |
| replied_at | TIMESTAMPTZ | | 回复时间 |
| created_at | TIMESTAMPTZ | NOT NULL | 创建时间 |
| updated_at | TIMESTAMPTZ | NOT NULL | 更新时间 |

> **#1941 待实现**：发票类型（普通/专用）/ 抬头 / 税号三字段（`invoice_type` / `title` / `tax_number`）为用户需求，**后端尚未实现**（无迁移/无模型列），待立项。

---

### 2.53 invoice_application_orders - 发票申请-订单关联表

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| application_id | UUID | PK, FK → invoice_applications.id (ON DELETE CASCADE) | 发票申请 ID |
| order_id | UUID | PK | 订单 ID |

---

### 2.54 发票域字段补充

**orders 表新增字段**（迁移 #1786）：
| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| invoice_applied | boolean | NOT NULL, DEFAULT false | 是否已申请发票 |
| invoice_applied_at | TIMESTAMPTZ | | 申请时间 |

---

### 2.55 乐器丢失域（#1939 派生，待立项）

> ⚠️ **待立项**：乐器丢失与找回（LS-01~LS-06，`cases/instrument-loss.md`）为 2026-09-17 用户需求，**后端无表/无端点**（仅 `instruments.stock_status='lost'` 常量已定义）。丢失登记/找回/结算所需表结构（丢失记录、赔偿、恢复）待立项后补录。

---
