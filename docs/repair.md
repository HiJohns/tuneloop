# TuneLoop 维修功能设计

> 来源：`docs/cases.md` §3 维修 + #1100（租赁乐器维修）+ #1109（客户报修）+ 报修流程 v3 重构共识（2026-07-04）
> 最后更新：2026-07-04（客户报修 **v3**：估价前置+竞价+中转扇出+四段物流+重新协商）
>
> **版本说明**：§4.1 租赁乐器维修流程保持不变；§3.3/§4.2/§5 及数据模型为客户报修 **v3** 设计，替换先前"先寄件后估价"的旧流程。

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
| 顾客 | USER | 创建报修单、接受/拒绝报价、支付、评价、申诉 | 自己创建的报修单；对报价单仅见金额+报价单号（受控情形不见师傅身份） |
| 网点员工 | site_member / site_admin | 收货识别、发回物流（v3 仅此两个动作，其余只读） | 本网点报修单 + 本网点乐器 |
| 维修师傅 | repair_technician | 提交报价（材料/服务/物流/工期/评论）、维修、完成、维修中重新报价（仅一次） | 分配到本网点的报修单；报价单仅本网点成员+报修人可见，**跨网点报价人互不可见** |
| 中转网点员工 | site_member / site_admin（中转网点） | **独立动作**：填中转服务费/物流费、扫中转单号、拆箱/拍照/重装、申诉人工核查脱敏 | 本中转网点报修单；与实际作业网点员工/师傅动作分离 |
| 商户管理员 | merchant_admin (OWNER) | 查看各网点报修列表、设置商户级费用、处理申诉 | 商户下属所有网点 |
| 系统管理员 | namespace_admin | 设置**检查费**（系统统一） | 全局 |

维修师傅是业务侧角色（`site_members.role = 'repair_technician'`），不涉及 IAM 修改。可与 site_member 角色共存（兼职工）。

**双向脱敏（受控/合作商户情形）**：
- 师傅方向：受控网点师傅看不到报修人信息；报价单评论**禁止出现任何联系方式**（提交前校验）
- 用户方向：用户仅见报价金额与**报价单号**，看不到师傅姓名/联系方式
- 申诉方向：用户申诉转往受控网点管理员前，由中转网点员工人工核查并**剥离用户联系方式**
- 存储原则：评价/报价内部关联真实 网点/师傅/商户（支持多维统计），仅**展示**脱敏

---

## 3. 数据模型

### 3.1 表结构

| 表 | 说明 | 关键字段 |
|----|------|---------|
| `instruments` | 乐器主表 | `stock_status`, `repair_status`, `repair_worker_id` |
| `instrument_media` | 乐器媒体 | `storage_key`, `is_display`, `batch_type` |
| `repair_records` | 维修记录 | `instrument_id`, `worker_id`, `comment`, `photos` |
| `user_instruments` | 客户自有乐器 | `user_id`, `sn`, `instrument_type`, `brand`, `model` |
| `repair_requests` | 报修单 | `user_id`, `user_instrument_id`, `status`(v3 状态见 §3.3), `site_id`；**v3 新增**：`merchant_type`(full/partner), `transit_site_id`, `controlled_site_id`, `accepted_quote_id`, `check_fee_snapshot`, `paid_amount`, `expire_at` |
| `repair_quotes` **(v3 新增)** | 报价单（多师傅竞价） | `id`, `repair_request_id`, `site_id`, `worker_id`, `quote_no`(报价单号), `material_fee`, `service_fee`, `logistics_fee`(C段), `duration`(工期), `comment`, `status`(pending/accepted/rejected/superseded), `is_renegotiation`(bool), `created_at` |
| `repair_transit_orders` **(v3 新增)** | 中转单（转入/转出） | `id`, `repair_request_id`, `transit_site_id`, `direction`(in/out), `transit_no`(中转单号), `transit_service_fee`, `transit_logistics_fee`(B+D段), `status`(pending_activation/active/received/relayed), `note`, `created_at` |
| `repair_request_records` | 报修日志（v3 承载重新协商时间线） | `repair_request_id`, `worker_id`, `comment`, `photos`, `record_type` |
| `appeals` | 申诉 | `category`, `object_type`, `object_id`, `appellant_id`, `description` |

> **费用字段归属**：报价单拆分为 `material_fee`(材料费)+`service_fee`(服务费)+`logistics_fee`(物流费·C段)；中转费 `transit_service_fee`+`transit_logistics_fee`(B+D段) 存于 `repair_transit_orders`（受控情形）；`check_fee` 系统统一、回退时快照到 `repair_requests.check_fee_snapshot`。

### 3.2 乐器维修状态枚举

```go
repair_pending    → 待维修（定损后自动设置）
repair_in_progress → 维修中（师傅扫码开始）
repair_completed  → 已修复（师傅完成）
// 验收通过后 clear，stock_status 回 available
```

### 3.3 报修单状态枚举（v3）

**全权商户路径**
```
pending_assessment(待估价) → pending_payment(待付款) → pending_ship(待发送)
  → shipping(已发货) → repairing(维修中) → return_pending(待发回) → returned(已发回) → closed(已关闭)
```

**合作/受控商户路径**（多一个前置的中转处理态 + 实物中转态）
```
transit_processing(中转处理中) → pending_assessment(待估价) → pending_payment → pending_ship
  → shipping → transit_in(转入中) → repairing → return_pending → transit_out(转出中) → returned → closed
```

**分支与循环**
```
pending_assessment ──(5工作日内未接受任何报价)──> closed        （到期前24h双通道提醒）
repairing ──(师傅重新报价·全程仅一次)──> 用户接受 → 补差款 → repairing
                                        └ 用户拒绝 → 回退结算(§5) → return_pending
returned → appealing → (管理员关闭) → closed
```

> **与旧流程的根本差异**：v3 **先远程估价+竞价、后寄件**（旧流程为先寄件、到货 `inspecting` 质检后再 `quoted` 报价）。旧状态 `inspecting`/`quoted`/`pending_cancel` 被 v3 的 `pending_assessment` + 报价单表 + 回退结算取代。`transit_processing`（早期定价扇出，无实物）≠ `transit_in`/`transit_out`（后期实物中转）。

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

### 4.2 客户报修流程（v3：估价前置 + 竞价 + 中转）

> 核心变化：**先远程报价、用户接受并付款后才寄件**。乐器物理运输发生在付款之后，实际维修在到货之后。

#### 阶段 0：选择商户与网点

1. 用户打开微信端，「维修」→ 右下角 `+` → 创建报修单页
2. 填识别码（500ms 防抖查自有乐器→回填类型/品牌/型号），填描述、拍照/视频
3. 选择商户类型：
   - **全权商户** → 直接看到并选择其**网点**
   - **合作商户** → 看到并选择一个**中转网点**（其关联的受控网点对用户不可见）
4. 提交 → 全权：进 `pending_assessment`；合作：进 `transit_processing`

#### 阶段 1：中转处理（仅合作/受控）

1. 单据进入 `transit_processing`（中转处理中）
2. 中转网点员工在单上填写〔中转服务费 + 中转物流费(B+D段)〕并提交
3. 提交后 → 该中转网点关联的**所有受控网点**对本单可见 → 进 `pending_assessment`
4. 中转员工超过 24h 未处理 → 告警系统升级至网点管理员（内部管理，不影响用户）

#### 阶段 2：待估价 + 竞价（`pending_assessment`）

1. 系统**双通道**（站内消息 + 微信模板）通知目标网点 / 受控网点的师傅
2. 各师傅提交报价单：〔材料费 + 服务费 + 物流费(C段) + 工期 + 评论〕
   - 报价单仅〔报价人所在网点成员〕+〔报修人〕可见；**跨网点报价人互不可见**
   - 受控情形：师傅看不到报修人；用户仅见报价单号；评论禁含联系方式（提交前校验）
3. 用户可继续补传照片/评论/视频；查看对自己可见的报价；**择一接受** → `pending_payment`
4. **有效期 5 个工作日**（从进入本态起算）；到期前 24h 双通道提醒；逾期未接受任何报价 → 自动 `closed`
5. 全部拒绝：无检查费、无物流费（未寄件未实检），单据保留至接受或到期

**API（待建）**：`POST /api/repair-requests/:id/quotes`（师傅报价）、`POST /api/repair-requests/:id/quotes/:qid/accept`（用户接受）

#### 阶段 3：待付款（`pending_payment`）

- 付款页显示计费明细（会员优惠 / 预付点数 / 赠点）
- 全权应付 = 材料 + 服务 + 物流；受控应付 = 材料 + 服务 + 物流 + 中转服务 + 中转物流
- 支付取整：现金/运费/总额 `Math.ceil()`；赠点/预付点上限 `Math.floor()`；赠点开关由商户管理员设置
- 调试阶段跳过实际支付 → 支付完成页 → `pending_ship`

**API**：`POST /api/repair-requests/:id/pay`

#### 阶段 4：待发送（`pending_ship`）

1. 师傅/员工全程只读
2. 系统创建**转入单**（`repair_transit_orders` direction=in，status=pending_activation）
3. 用户填物流：系统提供收货人信息（全权=目标网点；受控=**中转网点**地址/电话），转入单号在此可见并**要求写入物流留言**
4. 提交发货 → **激活转入单** → `shipping`

**API**：`PUT /api/repair-requests/:id/tracking`

#### 阶段 5：到货 / 实物中转

- **全权**：目标网点扫码收货 → `repairing`
- **受控**：`shipping` → 中转网点扫**转入单号** → 拆箱 → 拍照 → 重装 → 发往受控网点 → `transit_in`（转入中）→ 受控网点收货 → `repairing`

**API**：`POST /api/repair-requests/:id/receive`、`POST /api/repair-requests/:id/transit/relay`

#### 阶段 6：维修（含重新协商，全程仅一次）

1. 任一师傅可接手；拍照 + 评论记录 → 维修完成（至少一张识别码特写照片）→ `return_pending`
2. **重新协商（仅一次）**：若实况与远程报价不符，师傅可拍照/评论/**重新报价一次** → 双通道通知用户
   - 用户接受 → 待补款页（显示新清单 / 已付 / 应补差额）→ 支付 → 回到 `repairing`
   - 用户拒绝 → 回退结算（见 §5）→ `return_pending`

#### 阶段 7：发回（`return_pending`）

1. 系统创建**转出单**（direction=out，status=pending_activation）
2. 员工填发回物流，**转出单号写入物流留言**，发出 → 激活转出单
3. **全权**：直邮用户 → `returned`
4. **受控**：发往中转网点 → `transit_out`（转出中）→ 中转网点扫转出单号/拆箱/拍照/重装/发用户 → `returned`

#### 阶段 8：确认收货 / 评价 / 申诉

1. `returned` → 用户确认收货 → **评价**（三维关联 网点 + 师傅 + 商户，系统按不同维度算平均分；展示脱敏、存储真实）→ `closed`
2. 或用户**申诉** → `appealing`：
   - 全权 → 对应网点管理员处理
   - 受控 → 中转网点员工人工核查并**双向脱敏**（剥离用户联系方式）→ 转对应受控网点管理员
3. 管理员线下处理 → 线上关闭申诉 → 级联关闭报修单 → `closed`

**API**：`POST /api/appeals` + `POST /api/appeals/:id/close`

> **押金**：客户报修为用户自有乐器，**无押金**（区别于租赁归还定损）。

---

## 5. 费用模型（v3）

### 5.1 费用项与设置者

| 费用项 | 设置者 | 来源 | 说明 |
|--------|--------|------|------|
| 材料费 `material_fee` | 师傅 | 报价单 | 每份报价 |
| 服务费 `service_fee` | 师傅 | 报价单 | 每份报价 |
| 物流费 `logistics_fee` | 师傅 | 报价单 | **C 段**（返程 受控/网点→中转/用户） |
| 工期 `duration` | 师傅 | 报价单 | 展示给用户参考 |
| 中转服务费 `transit_service_fee` | 中转网点员工 | 中转单 | 仅受控情形 |
| 中转物流费 `transit_logistics_fee` | 中转网点员工 | 中转单 | **B+D 段**，仅受控情形 |
| 检查费 `check_fee` | **系统管理员**（系统统一） | 全局配置 | 仅中断维修回退时收取 |
| 报修允许使用赠点 | merchant_admin | 商户配置 | boolean 开关 |

### 5.2 物流四段模型

```
顾客 ─A─▶ 中转 ─B─▶ 受控网点 ─C─▶ 中转 ─D─▶ 顾客
```
- **A**：用户直付快递（不入系统账）
- **C**：师傅报价里的 `logistics_fee`（返程）
- **B + D**：中转员工填的 `transit_logistics_fee`
- **全权情形**：无中转，仅 C 段（返程单程）；发货 A 段用户直付

### 5.3 结算规则

| 场景 | 用户应付 | 退款 |
|------|---------|------|
| 全权·接受报价 | 材料 + 服务 + 物流 | — |
| 受控·接受报价 | 材料 + 服务 + 物流 + 中转服务 + 中转物流 | — |
| 重新协商·接受 | 新总额（页面显示 已付 / 应补差额） | — |
| 全权·中断回退 | 检查费 + 物流 | `max(0, 材料+服务 − 检查费)` |
| 受控·中断回退 | 检查费 + 物流 + 中转服务 + 中转物流 | `max(0, 材料+服务 − 检查费)` |
| 待估价全部拒绝 / 超时 | 0（自动关闭） | — |

> 回退不重新报价：B/C/D 段物流与中转服务在发货前已锁定，中断时乐器仍走完全程，故仅用"检查费"置换"材料+服务费"；退款封底 0。

**支付取整**：现金/运费/总额 `Math.ceil()`；赠点/预付点上限 `Math.floor()`。

**API（部分待建）**：`GET/PUT /api/config/repair/check-fee`（系统级）+ 中转费经中转处理接口写入 `repair_transit_orders`。

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

### 8.2 客户报修（v3；标 🆕 为待建端点）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/repair-requests` | 创建报修单（含 merchant_type 全权/合作、目标网点/中转网点） |
| GET | `/api/repair-requests` | 报修单列表（按角色过滤） |
| GET | `/api/repair-requests/:id` | 报修单详情 |
| 🆕 POST | `/api/repair-requests/:id/transit-process` | 中转网点填中转费并扇出至受控网点（`transit_processing`→`pending_assessment`） |
| 🆕 POST | `/api/repair-requests/:id/quotes` | 师傅提交报价单（材料/服务/物流/工期/评论） |
| 🆕 GET | `/api/repair-requests/:id/quotes` | 查看可见报价单（按可见性/脱敏过滤） |
| 🆕 POST | `/api/repair-requests/:id/quotes/:qid/accept` | 用户接受某报价 → `pending_payment` |
| POST | `/api/repair-requests/:id/pay` | 支付（首次全额 / 重新协商补差额） |
| PUT | `/api/repair-requests/:id/tracking` | 待发送填物流（激活转入单） |
| 🆕 POST | `/api/repair-requests/:id/receive` | 网点/受控网点扫码收货 → `repairing` |
| 🆕 POST | `/api/repair-requests/:id/transit/relay` | 中转网点扫单号/拆箱拍照重装/转发（转入或转出） |
| 🆕 POST | `/api/repair-requests/:id/requote` | 师傅维修中重新报价（仅一次）→ 通知用户 |
| 🆕 POST | `/api/repair-requests/:id/return-ship` | 员工填发回物流（激活转出单）→ `returned`/`transit_out` |
| 🆕 POST | `/api/repair-requests/:id/confirm-receipt` | 用户确认收货 |
| 🆕 POST | `/api/repair-requests/:id/evaluate` | 用户评价（三维关联，展示脱敏存储真实） |
| GET | `/api/repair-requests/:id/records` | 报修记录（含重新协商时间线） |
| GET | `/api/user-instruments/lookup` | SN 查自有乐器 |
| GET | `/api/merchant/repair-requests` | 商户管理员查看 |

> 旧端点 `POST /api/repair-requests/:id/quote`（单一报价）在 v3 中被 `quotes` 系列取代；`inspecting`/`quoted`/`pending_cancel` 状态废弃。具体端点签名与迁移在 Issue 阶段细化。

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
