# 会员与促销系统设计文档

> 来源：Issue #880 各轮评论汇总
> 版本：v4

---

## 一、数据库变动

### 1.1 新增表：`membership_levels`（会员级别）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | integer | PK | 级别 ID，数字越大级别越高 |
| `name` | varchar(50) | NOT NULL | 级别名称（初级/中级/高级） |
| `min_amount` | decimal | NOT NULL | 最低累计消费金额（元） |

默认数据：

| id | name | min_amount |
|:--:|------|----------:|
| 1 | 初级 | 0 |
| 2 | 中级 | 5000 |
| 3 | 高级 | 10000 |

### 1.2 新增表：`promo_plans`（会员折扣政策与促销方案）

此表承载两类用途：
1. **会员折扣政策**：长期生效，无需起止时间。`scope_type` 仅可为 `system` / `merchant`（不可为 `site` — 网点无制订权）。
2. **促销方案活动**：时间限定促销。`scope_type` 可为 `system` / `merchant` / `site`。

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | UUID | PK |
| `plan_type` | varchar(20) | `discount_policy` / `promo_campaign` — 方案类型（见下方） |
| `scope_type` | varchar(20) | `system` / `merchant` / `site`（折扣政策不可用 `site`） |
| `scope_id` | UUID | 商户 ID 或网点 ID（system 级为空） |
| `name` | varchar(100) | 方案名称 |
| `start_date` | date | 开始日期（折扣政策可设为 null 表示长期有效） |
| `end_date` | date | 结束日期（折扣政策可设为 null） |
| `stackable` | bool | 是否可与其他方案叠加 |
| `is_active` | bool | 是否启用 |
| `created_at` | timestamp | |
| `updated_at` | timestamp | |

**plan_type 约束**：
- `discount_policy`（会员折扣政策）：`scope_type` 仅可为 `system` / `merchant`
- `promo_campaign`（促销方案活动）：`scope_type` 可为 `system` / `merchant` / `site`

**CHECK 约束**：`(plan_type = 'discount_policy' AND scope_type != 'site') OR (plan_type = 'promo_campaign')`

### 1.3 新增表：`promo_plan_details`（促销方案明细）

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | UUID | PK |
| `promo_plan_id` | UUID | FK → promo_plans.id |
| `level_id` | integer | FK → membership_levels.id |
| `rent_discount` | decimal(5,4) | 租金折扣率（如 0.9 = 9折） |
| `deposit_discount` | decimal(5,4) | 押金折扣率 |
| `overdue_discount` | decimal(5,4) | 逾期租金折扣率 |

### 1.4 新增表：`rebate_config`（返点配置，按会员级别定义比例）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | UUID | PK | |
| `level_id` | integer | FK → membership_levels.id, UNIQUE | 会员级别，每个级别一条 |
| `rent_ratio` | decimal(5,4) | | 该级别返点与租金的比例（如 0.01 = 1%） |
| `is_active` | bool | | 该级别是否启用返点 |
| `created_at` | timestamp | | |
| `updated_at` | timestamp | | |

默认数据：

| level_id | rent_ratio |
|:--------:|-----------:|
| 1（初级） | 0.005 |
| 2（中级） | 0.01 |
| 3（高级） | 0.02 |

权限：仅系统管理员可管理比例（`rebate:manage` cus_perm）。

### 1.5 新增表：`points_policies`（促销点数政策，三级可覆盖）

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | UUID | PK |
| `scope_type` | varchar(20) | `system` / `merchant` / `site` |
| `scope_id` | UUID | 商户 ID 或网点 ID |
| `max_pay_ratio` | decimal(5,4) | 可支付价格的百分比上限（如 0.3 = 30%） |
| `valid_days` | integer | 有效期（天） |
| `is_active` | bool | |

优先级：网点 > 商户 > 系统。

### 1.6 用户表（`users`）新增字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `membership_level_id` | integer | FK → membership_levels.id |
| `total_spending` | decimal | 消费金额总计（跨商户累计） |
| `prepaid_points` | decimal | 预付点数 |
| `promo_points` | decimal | 促销点数 |

### 1.7 乐器表（`instruments`）新增字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `min_membership_level` | integer | 最低可租会员级别 ID（FK → membership_levels），null 表示无限制 |

### 1.8 新增表：`instrument_promo_overrides`（乐器促销覆盖）

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | UUID | PK |
| `tenant_id` | UUID | FK → tenants/merchants，数据隔离字段 |
| `instrument_id` | UUID | FK → instruments.id |
| `override_type` | varchar(20) | `discount` / `rebate` — 覆盖类型（折扣政策/返点政策） |
| `enabled` | bool | 该乐器是否适用此类型政策 |
| `updated_at` | timestamp | |

约束：同一 `(tenant_id, instrument_id, override_type)` 唯一。

网点管理员可设置单件乐器是否参与折扣政策和返点政策，但不能修改政策本身。

### 1.9 订单表（`orders`）新增字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `pricing_breakdown` | jsonb | 价格计算明细，**计费快照**，订单创建时写入后不可修改 |

**权威性说明**：
- `pricing_breakdown` 为计费权威源，订单创建时 snapshot 当前所有折扣政策
- `Order.MonthlyRent` / `Deposit` 等字段必须与 `pricing_breakdown` 中对应值一致
- 后续政策变更不影响已生成订单的价格（历史订单价格不变）

结构示例：
```json
{
  "base_daily_rent": 10.00,
  "tier_discount_rate": 0.7,
  "membership_discount_rate": 0.9,
  "promo_discount_rates": [0.95],
  "final_daily_rent": 5.99,
  "rent_days": 30,
  "total_amount": 179.70,
  "applied_policies": [
    {"type": "membership_discount", "plan_name": "系统默认", "rate": 0.9},
    {"type": "promo_campaign", "plan_name": "夏日促销", "rate": 0.95}
  ]
}
```

---

## 二、商业逻辑

### 2.1 级别升级

- 用户注册即为初级会员
- `total_spending` 跨所有商户累计（租金 + 购买点数）
- 不计入累积：运费、押金、损坏赔偿、使用点数支付的租金
- 达到阈值自动升级到对应级别
- 仅升级不降级

### 2.2 折扣计算

```
最终日租金 = 基础日租 × 阶梯折扣 × (乐器折扣开关 ? 会员折扣 : 1.0) × 促销折扣
```

叠加规则：阶梯定价折扣 × (乐器开关 ? 会员折扣 : 1.0) × 促销折扣（乘法叠加）。

- `乐器折扣开关` = `instrument_promo_overrides.enabled`（仅对 discount 类型），false 时会员折扣不参与计算

例：基础价 10 元/天，高级会员（9折），促销方案（95折），乐器启用折扣：
- 第 1~30 天：10 × 1.0 × 0.9 × 0.95 = 8.55 元/天
- 第 181~365 天：10 × 0.7 × 0.9 × 0.95 = 5.99 元/天

例：同场景但乐器**禁用**折扣：
- 第 1~30 天：10 × 1.0 × 1.0 × 0.95 = 9.50 元/天（会员折扣被跳过）

**计算透明度要求**：
- 购物车/结算页面须展示完整计算过程：**原价 → 适用政策（名称+折扣率）→ 最终价格**
- 订单生成时将 `pricing_breakdown` 写入订单记录（见 §1.9），后续查看订单时可还原计费明细

### 2.2a 长期政策（无终止日期）

- 折扣政策和返点政策均可设置为长期有效（不设 `end_date`，或设 `end_date` 为 null）
- UI 上明确提供"长期有效"选项，而非强制填写截止日期
- 长期政策的优先级低于有时间限制的政策（便于临时促销覆盖长期折扣）

### 2.3 会员折扣政策（两级覆盖，非促销活动）

会员折扣政策按级别定义折扣率（存储在 `promo_plans` / `promo_plan_details`），是**长期生效的基础政策**，非有起止时间的促销活动。

| 级别 | 制订者 | 覆盖范围 | 优先级 |
|------|--------|---------|:---:|
| 系统 | sys_admin | 全站默认 | 低 |
| 商户 | merchant_admin | 本商户所有网点（覆盖系统） | 高 |

- 商户管理员可采纳系统默认方案（不创建），或创建本商户方案覆盖系统
- 商户方案影响本商户下**所有网点**，网点管理员无权创建或修改会员折扣政策
- 网点管理员仅可决定单件乐器是否适用该政策（通过 `instrument_promo_overrides`，不修改政策本身）

### 2.3a 促销方案活动（三级，时间限定）

与上述基础折扣政策不同，促销方案活动有明确起止时间，可叠加在基础折扣之上。

| 级别 | 制订者 | 覆盖范围 |
|------|--------|---------|
| 系统 | sys_admin | 全站 |
| 商户 | merchant_admin | 本商户 |
| 网点 | site_admin | 本网点 |

- 每套方案有起止时间，过期自动失效
- 多套方案共存时按优先级（网点 > 商户 > 系统）生效，同一优先级取最新
- 可叠加方案：`stackable=true` 的方案与其他方案乘法叠加
- 商户可设 `is_active=false` 完全退出促销活动
- 网点可指定单件乐器不参加某促销活动

### 2.4 返点

- 按会员级别设置不同的返还比例（`rebate_config`），系统管理员可管理（`rebate:manage` cus_perm）
- 商户管理员可决定本商户是否参与返点（opt-in/opt-out），仅影响参与决策，不影响比例本身
- 网点管理员可决定单件乐器是否适用返点（通过 `instrument_promo_overrides`）
- 按实际支付租金的一定比例返还为点数
- 返点政策不可由商户或网点创建/修改，仅系统管理员可配置比例

**返点生效条件（三者 AND）**：
```
返点生效 = rebate_config.is_active（系统开关）
           AND 商户 rebate_opt_in（商户开关，默认 true）
           AND 乐器 override.enabled（网点开关，默认 true）
```
任一条件为 false，则该级别/商户/乐器不参与返点。

### 2.4a 返点发放规则

- **发放时机**：订单状态变为 `leased`（租赁中，即确认收货后）时发放
- **存入字段**：`users.promo_points`
- **发放比例**：按用户当前 `membership_level_id` 对应的 `rebate_config.rent_ratio` × 实际支付月租金
- **订单取消/提前终止**：按实际租赁天数比例追回已发放返点（从 `promo_points` 中扣除）
- **返点不计入 total_spending**（避免循环升级）
- **乐器禁用返点**（`instrument_promo_overrides.enabled=false`）时，该乐器订单不产生返点

### 2.5 促销点数政策

- 三级均可制订，网点优先 → 商户其次 → 系统最低
- 点数使用限制：可支付价格百分比上限、有效期
- 点数由商户/网点授予（将来实现）
- 使用点数时不计入 `total_spending`（避免重复升级）

### 2.6 乐器最低可租级别

- 网点员工创建/编辑乐器时可设置
- 低于该级别的用户可看到乐器但无法下单
- 提示"需要 XX 级会员才可租借"

---

## 三、用例

### 3.1 系统管理员

| 用例 | 操作 |
|------|------|
| 管理会员级别 | 增删改级别名称、门槛金额 |
| 设置系统会员折扣政策 | 创建/修改折扣方案，指定各级别折扣率、起止时间、是否可叠加 |
| 设置返点比例（按级别） | 为每个会员级别配置不同的租金→点数返还比例 |
| 设置系统点数政策 | 配置点数使用百分比上限、有效期 |

### 3.2 商户管理员

| 用例 | 操作 |
|------|------|
| 查看本商户各级会员 | 列表，按级别筛选 |
| 设置商户会员折扣政策 | 采纳系统默认方案，或制订本商户方案覆盖系统 |
| 决定是否参与返点 | 仅 opt-in/opt-out，不修改比例 |
| 设置商户点数政策 | 覆盖系统政策 |
| 授予促销点数 | 将来实现 |

### 3.3 网点管理员/员工

| 用例 | 操作 |
|------|------|
| 设置网点促销方案 | 覆盖上级方案 |
| 按乐器决定折扣政策适用 | 开启/关闭单件乐器的会员折扣政策适用（不修改政策本身） |
| 按乐器决定返点政策适用 | 开启/关闭单件乐器的返点政策适用（不修改政策本身） |
| 设置网点点数政策 | 覆盖上级政策 |
| 授予促销点数 | 将来实现 |
| 设置乐器最低可租级别 | 创建/编辑乐器时选择 |

### 3.4 顾客（会员）

| 用例 | 操作 |
|------|------|
| 查看会员级别 | 显示当前级别名称和徽章，距下一级别所需消费金额进度条 |
| 查看累计消费 | `total_spending` 为**全平台累计**；本商户累计通过 `SUM(orders.total_amount WHERE tenant_id=当前商户)` 实时计算，不单独存储字段 |
| 查看点数余额 | 显示 `promo_points` + `prepaid_points` 及有效期 |
| 查看订单价格明细 | 订单详情页展示 `pricing_breakdown`：原价 → 各项折扣 → 最终价格 |
| 下单时查看价格计算 | 购物车/结算页逐行展示折扣计算过程（见 §2.2） |
| 使用促销点数抵扣 | 下单时选择使用点数 |
| 看到因级别不够无法租赁的乐器 | 提示"需要 XX 级会员才可租借" |
| 自动升级 | 消费达标后系统自动升级，发送通知 |

---

## 四、权限控制

| 操作 | 权限 | 说明 |
|------|------|------|
| 管理级别表 | `membership:manage` | 新增 cus_perm，仅 sys_admin |
| 管理/创建系统会员折扣政策 | sys_admin + `promo:manage` | 全站默认折扣方案 |
| 管理/创建商户会员折扣政策 | merchant_admin + `promo:manage` | 可采纳系统方案或创建本商户方案覆盖；影响本商户所有网点 |
| 管理/创建返点比例（按级别） | `rebate:manage` | 新增 cus_perm，仅 sys_admin |
| 商户决定是否参与返点 | merchant_admin | 仅 opt-in/opt-out，不修改比例 |
| 决定乐器是否适用折扣/返点 | `promo:override` | 新增 cus_perm，site_admin，通过 `instrument_promo_overrides` 开关，不修改政策本身 |
| 管理促销方案活动（各级） | 对应级管理员 | 时间限定促销活动，非基础折扣政策 |
| 管理促销点数政策 | 对应级管理员 + `points:manage` | 新增 cus_perm，三级可覆盖 |
| 设置乐器最低可租级别 | site_admin / site_member | 创建/编辑乐器时设置 |

### 4.1 cus_perm 注册清单

以下 4 个新 cus_perm 码必须在 `backend/services/permission_registry.go:getTuneLoopPermissions()` 中注册（从 bit 21 开始），否则 `RequireCusPerm` 中间件会直接穿透放行：

| 权限码 | bit | 名称 | 授予角色 |
|--------|-----|------|---------|
| `rebate:manage` | 21 | 返点管理 | sys_admin |
| `promo:manage` | 22 | 折扣政策管理 | sys_admin, merchant_admin |
| `promo:override` | 23 | 乐器促销覆盖 | site_admin |
| `points:manage` | 24 | 点数政策管理 | sys_admin, merchant_admin, site_admin |
| `membership:manage` | 25 | 会员级别管理 | sys_admin |

### 4.2 错误码

| 错误码 | 说明 | 触发场景 |
|--------|------|---------|
| `40310` | `membership_level_insufficient` | 用户会员级别低于乐器 `min_membership_level` |
| `40311` | `promo_not_applicable` | 乐器禁用该类型促销政策（`instrument_promo_overrides.enabled=false`） |
