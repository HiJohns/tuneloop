# TuneLoop 维修功能设计

> 来源：`docs/cases.md` §3 维修 + #1100（租赁乐器维修）+ #1109（客户报修）  
> 最后更新：2026-06-30

---

## 1. 概述

TuneLoop 的维修功能覆盖两类场景：

| 场景 | 触发方 | 乐器来源 | 数据驱动 |
|------|--------|----------|----------|
| **租赁乐器维修** | 员工（定损后） | `instruments` 租用乐器 | `instrument.repair_status` |
| **客户报修** | 顾客（自有乐器） | `user_instruments` 客户自有 | `repair_request.status` |

两类场景共用维修师傅角色和维修记录面板，但数据流完全独立。

---

## 2. 角色与权限

| 角色 | 英文标识 | 职责 | 可见范围 |
|------|----------|------|----------|
| 顾客 | USER | 创建报修单、确认/拒绝报价、支付、评价 | 自己创建的报修单 |
| 网点员工 | site_member / site_admin | 收货识别、发回物流、验收维修 | 本网点报修单 + 本网点乐器 |
| 维修师傅 | repair_technician | 质检报价、维修、完成 | 分配给自己 + 待处理的维修 |
| 商户管理员 | merchant_admin (OWNER) | 查看各网点报修列表、设置费用标准 | 商户下属所有网点 |

维修师傅是业务侧角色（`site_members.role = 'repair_technician'`），不涉及 IAM 修改。可与 site_member 角色共存（兼职工）。

---

## 3. 数据模型

### 3.1 表结构

| 表 | 说明 | 关键字段 |
|----|------|---------|
| `instruments` | 乐器主表 | `stock_status`, `repair_status`, `repair_worker_id` |
| `instrument_media` | 乐器媒体 | `storage_key`, `is_display`, `batch_type` |
| `repair_records` | 维修记录 | `instrument_id`, `worker_id`, `comment`, `photos` |
| `user_instruments` | 客户自有乐器 | `user_id`, `sn`, `instrument_type`, `brand`, `model` |
| `repair_requests` | 报修单 | `user_id`, `user_instrument_id`, `status`(11状态), `quote_amount`, `worker_id`, `site_id` |
| `repair_request_records` | 报修日志 | `repair_request_id`, `worker_id`, `comment`, `photos`, `record_type` |
| `appeals` | 申诉 | `category`, `object_type`, `object_id`, `appellant_id`, `description` |

### 3.2 乐器维修状态枚举

```go
repair_pending    → 待维修（定损后自动设置）
repair_in_progress → 维修中（师傅扫码开始）
repair_completed  → 已修复（师傅完成）
// 验收通过后 clear，stock_status 回 available
```

### 3.3 报修单状态枚举

```
pending_ship → shipping → inspecting → quoted ──┬── pending_payment → repairing → return_pending → returned → closed
                                                 │
                                                 └── pending_cancel → (付款) → return_pending

returned → appealing → (管理员关闭) → closed
```

---

## 4. 业务流程

### 4.1 租赁乐器维修流程

```
归还定损(有损坏) → repair_pending → 师傅扫码开始 → repair_in_progress → 拍照/评论/完成 → repair_completed → 员工验收 → available
                                                                                                              ↓ 不通过
                                                                                                        repair_in_progress
```

**角色参与**：

- 员工：定损后触发维修（`assessment.go`/`warehouse.go` → `repair_status = repair_pending`）
- 师傅：扫码 → 开始维修 → 拍照/记录 → 完成维修（至少一张照片）
- 员工：验收通过/不通过（不通过回退到维修中）

**API**：`POST /api/repair/:id/{start|complete|takeover|accept|reject|records}`

### 4.2 客户报修流程

#### 阶段一：用户创建报修单

1. 用户打开微信端，点击底条「维修」→ 点击右下角 `+` → 进入创建报修单页
2. 填写识别码（500ms 防抖自动查找自有乐器→填充类型/品牌/型号）
3. 选择乐器类型/品牌/型号（如不在自有乐器表中则创建新记录）
4. 填写描述、拍照、选择商户→网点（双层查找）
5. 可选填物流信息（填了就是发送中，没填就是待发送）
6. 提交生成报修单

**API**：`POST /api/repair-requests`

#### 阶段二：物流发往网点

- 若有物流单号 → 状态= `shipping`（发送中）
- 若无 → 状态= `pending_ship`（待发送），可事后补填 → `shipping`

**API**：`PUT /api/repair-requests/:id/tracking`

#### 阶段三：员工收货

1. 员工在收货界面扫描乐器识别码
2. 系统智能匹配：报修单表（shipping）vs 乐器表
3. 匹配到报修单 → 状态转为 `inspecting`（质检中）
4. 匹配到乐器表 → 走租赁归还-接收定损流程
5. 两条都匹配 → 弹窗让员工选择

**API**：`GET /repair-requests?status=shipping` 交叉比对

#### 阶段四：维修师傅质检+报价

1. 师傅在 `/my-repairs` 看到质检中报修单
2. 点击进入报修详情页
3. 拍照（需拍摄识别码特写）+ 填写报价金额 → 提交
4. 状态转为 `quoted`（待回复）

**API**：`POST /api/repair-requests/:id/quote`

#### 阶段五：用户确认/拒绝报价

- **确认**：状态= `pending_payment` → 支付（报价金额）→ `repairing`（维修中）
- **拒绝**：状态= `pending_cancel` → 支付（检查费）→ `return_pending`（待发回）

**支付取整规则**：
- 现金/运费/总额：`Math.ceil()` 向上取整
- 可用赠点/预付点上限：`Math.floor()` 向下取整
- 可选用预付点和赠点（赠点开关由商户管理员设置）

**API**：`POST /api/repair-requests/:id/pay`

#### 阶段六：维修师傅维修

1. 师傅看到维修中报修单 → 进入详情页
2. 拍照+添加评论记录 → 维修完成
3. 完成前必须至少上传一张识别码特写照片
4. 状态转为 `return_pending`（待发回）

#### 阶段七：员工发回

1. 员工看到待发回报修单 → 填写物流公司+单号
2. 状态转为 `returned`（已发回）

#### 阶段八：用户评价/申诉

1. 已发回 → 用户可评价服务，报修单关闭
2. 已发回 → 用户可提出申诉 → 状态转为 `appealing`
3. 管理员线下处理申诉 → 线上关闭申诉 → 级联关闭报修单

**API**：`POST /api/appeals` + `POST /api/appeals/:id/close`

---

## 5. 费用配置

| 配置项 | 级别 | 设置者 | 取值说明 |
|--------|:---:|--------|----------|
| 检查费 | 商户 | merchant_admin | 用户拒绝报价时收取 |
| 物流费默认值 | 商户 | merchant_admin | 网点可覆盖 |
| 物流费覆盖值 | 网点 | site_admin | 覆盖商户默认值 |
| 报修允许使用赠点 | 商户 | merchant_admin | boolean 开关 |

**API**：`GET/PUT /api/config/repair/*`（商户级）+ `GET/PUT /api/sites/:id/config/shipping-fee`（网点级）

**权限**：`middleware.RequireRole("OWNER")` 限制商户管理员

---

## 6. 移动端页面路由

| 路由 | 组件 | 说明 | 访问角色 |
|------|------|------|----------|
| `/my-repairs` | MyRepairs.jsx | 维修中心（扫码+角色差异化列表+创建按钮） | 全角色（不同角色看到不同内容） |
| `/create-repair` | CreateRepairRequest.jsx | 创建报修单 | 顾客 |
| `/repair-request` | RepairRequestDetail.jsx | 报修详情（报价/付款/维修/评价） | 全角色 |
| `/repair` | RepairWorkflow.jsx | 租赁乐器维修工作流（多面板） | 员工、师傅 |
| `/receiving-repair-scan` | ReceivingRepairScan.jsx | 员工收货智能识别 | 员工 |

---

## 7. 图片分层规范

| 图片类型 | 数据源 | 顾客详情页 | 员工详情页 | 列表/卡片 |
|----------|--------|:---:|:---:|:---:|
| 海报 | `instruments.poster` | ✅ 原图 | ✅ 原图 | ❌ |
| 展示图 | `instrument_media` (is_display=true) | ✅ 原图 | ✅ 原图 | ✅ 小图(128px) |
| 流程记录 | `instrument_media` (is_display=false) | ❌ | ✅ 日志面板 | ❌ |
| 视频缩略图 | `instrument_media` (file_type='video_thumb') | ❌ | ✅ | ❌ |

---

## 8. 相关 API 汇总

### 8.1 租赁乐器维修

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/repair/:id/start` | 开始维修 |
| POST | `/api/repair/:id/complete` | 完成维修（需照片） |
| POST | `/api/repair/:id/takeover` | 接手维修 |
| POST | `/api/repair/:id/accept` | 验收通过 |
| POST | `/api/repair/:id/reject` | 验收不通过 |
| POST | `/api/repair/:id/records` | 添加维修记录 |
| GET | `/api/repair/:id/records` | 查看维修记录 |
| GET | `/api/repair/mine` | 我的维修列表 |
| GET | `/api/repair/pending` | 待维修列表 |

### 8.2 客户报修

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/repair-requests` | 创建报修单 |
| GET | `/api/repair-requests` | 报修单列表（按角色过滤） |
| GET | `/api/repair-requests/:id` | 报修单详情 |
| PUT | `/api/repair-requests/:id/tracking` | 补填物流 |
| POST | `/api/repair-requests/:id/pay` | 支付（含 total_spending 更新） |
| GET | `/api/repair-requests/:id/records` | 报修记录 |
| GET | `/api/user-instruments/lookup` | SN 查自有乐器 |
| GET | `/api/merchant/repair-requests` | 商户管理员查看 |

### 8.3 申诉

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/appeals` | 申诉列表 |
| POST | `/api/appeals` | 创建申诉 |
| POST | `/api/appeals/:id/close` | 关闭申诉（级联关报修单） |

### 8.4 费用配置

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/config/repair` | 获取全部报修配置 |
| PUT | `/api/config/repair/single` | 设置单项配置 |
| GET/PUT | `/api/sites/:id/config/shipping-fee` | 网点物流费 |

---

## 9. 相关文档

- `docs/cases.md` §3 — 维修用例
- `AGENTS.md` §Instrument Image Hierarchy — 图片分层规范
- `AGENTS.md` §Repair Request Tables — 报修表字段说明
- `AGENTS.md` §维修页角色×页面矩阵 — 角色差异化视图
- `docs/ui.md` §1.3 — 跨端渲染基本准则
