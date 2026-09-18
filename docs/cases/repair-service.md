---
id: RS-00
domain: repair-service
flow: 维修服务改版总纲（#1942）
source: 用户需求 2026-09-17（维修是单项服务，类似一件商品；可选维修师；可咨询）
related: "#1943（咨询，holdon）｜docs/cases/repair.md（v3 报修，并存）"
---

# RS-00 维修服务总纲

> **定位变化**：维修从「已出租乐器的报修工单流程」重构为「**可独立购买的服务商品**」——用户选择维修师、可咨询、不绑定租赁乐器。
> **并存策略**：既有 v3 报修（已租琴报修）流程保留，入口区分「乐器报修」与「维修服务」；存量 pending 单继续走 v3。

## 与 v3 报修的差异

| 维度 | v3 报修（repair.md） | 维修服务（本文件） |
|------|---------------------|-------------------|
| 对象 | 已出租的乐器（租赁订单关联） | 任意乐器（无需租赁关系） |
| 商业形态 | 工单（定损/赔偿/结算） | 服务商品（报价→支付→履约） |
| 计费 | 定损金额 | **师傅报价（修理费+物流费）+ 加价** |
| 物流 | 网点/中转安排 | 用户自寄出（编码标记）+ 网点员工发回（**分段物流费**） |
| 评价 | 无 | 评分/留言/拍照，PC 后台可见 |

## RS-01 创建维修单（用户，weapp）——**入口经师傅详情**（2026-09-18 设计变更）

- **入口链路**：「维修」Tab → **师傅列表**（RS-02）→ 点击师傅 → **师傅详情页**（RS-02）→ 下方「**创建维修订单**」按钮 → **维修单创建页**
- **创建页要点**：**师傅锁定 = 刚才查看的师傅**（只读展示师傅头像/姓名）；**无需选择商户与网点**
- 表单：描述（必填）+ 照片（≤6）——**不填识别码**
- 提交后系统分配 **6 位唯一编码**（数字+大写字母，`repair_code` uniqueIndex，冲突重试），界面展示并提示「请将该编码写在物流单信息栏」
- **调用 / 获取**
  - `POST /api/user/repair-services {description, photos[], technician_id}`（**创建即锁定师傅**）→ `{id, repair_code, status}`
  - 照片上传：`POST /api/upload`（现有）→ 取返回 url 填入 `photos`
  - 服务单归属 = **师傅直属商户**（`tenant_id` 回填；**不再有 site 维度**，见 RS-13）
- **兼容**：`POST …/select-technician` 保留（老数据/未锁定单补选），新流程创建时即锁定

## RS-02 师傅列表 / 师傅详情（用户浏览）＋ 报价（师傅，weapp 工作台）

### 师傅列表（维修 Tab 首屏，2026-09-18 变更）
- **样式参照乐器列表**；每项展示：**个人照片** + 姓名 + **详细介绍摘要**（如「钢琴维修 12 年 · 小提琴维修 8 年」）
- **右上角『我的维修』链接**（师傅视角）：
  - **无活跃维修会话 → 置灰（仍可点击）**；**有活跃会话 → 点亮并显示个数**
  - 点击进入「**我的维修**」= **现存维修页（师傅工作台，TechRepairWorkbench）**，查看自己的维修会话
- 咨询能力 → #1943（holdon）

### 师傅详情页
- **样式参照乐器详情页**；展示：个人照片、完整介绍（专长与年限列表）、（评分展示待后续）
- **下方固定「创建维修订单」按钮** → 进入创建页（RS-01，师傅锁定）

### 师傅档案与归属（2026-09-18 设计变更，见 RS-13）
- **师傅不再挂靠网点**，**直属商户**（tenant 级）；`tenant_id` 即服务单归属
- 档案字段：`photo`（个人照片）、`bio`（详细介绍）、`experience`（专长与年限，结构化：`[{craft, years}]`）

### 报价（师傅，weapp 工作台）
- 师傅**报价**：`修理费` + `物流费预估`
  - 直连模式（无中转）：1 段受管物流（**师傅→用户**）
  - 受控组合：3 段受管物流（中转→受控、受控→中转、中转→用户）；**用户→中转段用户自担**
- **调用 / 获取**
  - `GET /api/common/repair-technicians` → 师傅列表（**商户直属**，档案字段；去 site 维度）　**【RS-API-1 修订】**
  - `GET /api/common/repair-technicians/:id` → 师傅详情　**【RS-API-8，新增】**
  - `GET /api/common/repair-technicians/active-session-count`（师傅本人）→ `{count}`（『我的维修』点亮/计数）　**【RS-API-9，新增】**
  - `GET /api/repair-services?scope=mine&status=pending_quote` → 待报价列表　**【RS-API-2】**
  - `POST /api/repair-services/:id/quote {quote_repair_cents, quote_logistics_cents}` → `{id, status, payable}`

## RS-03 用户接受报价并支付（用户，weapp）

- 接受报价 → **虚拟商品支付**（修理费 + 物流费预估；`order_type='repair'`，不绑乐器订单）
- 支付成功 → **会话建立（合约开始）**，状态 `paid`
- **调用 / 获取**
  - `POST /api/user/repair-services/:id/accept` → `{id, payable_cents}`（金额服务端重算）
  - `POST /api/pay/prepay {order_type:"repair", order_id:<维修单id>}`（现有；服务端按状态重算金额，客户端金额不可信）→ JSAPI 参数
  - 回调：虚拟商品路径（无物流上报收货确认）

## RS-04 用户寄出（用户，weapp）

- 用户自行联系物流寄出（运费自担），**填写物流单号**（物流单信息栏须写 6 位编码）
- **调用 / 获取**
  - 详情页需展示**寄件地址**（所选网点名称/地址/联系人/电话）→ `GET /api/user/repair-services/:id` 需返回 `site` 对象　**【RS-API-3，新增】**
  - `POST /api/user/repair-services/:id/ship {tracking_company, tracking_number}` → 状态 `shipping`

## RS-05 分段物流（网点/中转员工，weapp + PC 后台）

- 每段发运时，经手员工**实填本段物流费**（报价预估为参考）
- 受控组合的段序：中转→受控（网点员工）→（师傅修理）→ 受控→中转（受控侧员工）→ 中转→用户（网点员工，**末段触发结算**）
- 每段费用落库：`repair_logistics_fees`（repair_id/leg/amount_cents/filled_by）
- **调用 / 获取（员工）**
  - `POST /api/repair-services/:id/legs {leg, logistics_fee_cents}`
  - 待发回清单：`GET /api/repair-services?scope=site&status=done_repair`　**【RS-API-2】**（现 `GET /api/repair-services/pending-dispatch` 保留兼容）

## RS-06 加价（师傅报价修正，weapp 工作台）

- 师傅发现与描述不符需加钱：提交**加价申请**（新修理费**总价** + **到此为止修理费**，二者同填且语义不同）
  - 例：报价 200（4h×50/h）+ 物流预估 50 → 用户已付 250；干了 1h（到此为止 = 50）后发现需加物料 100 → 新总价 = 300，到此为止 = 50
- 用户选择：
  - **继续** → **立即补差价** = `新总价 − 原报价修理费`（= 100；**物流费不参与补差**，按各段实填结算）→ 虚拟商品支付 → 师傅继续修理
  - **不继续** → 师傅停止修理 → 乐器进入**待发回**（结算修理费基准 = **到此为止修理费**）
- 加价记录留痕（原报价/新报价/用户决定）
- **调用 / 获取**
  - 师傅：`POST /api/repair-services/:id/adjust {new_quote_cents, incurred_cents}` → `{id, status, payable_cents(差价), incurred_cents}`
  - 用户：`POST /api/user/repair-services/:id/adjust/accept` → `{payable_cents(差价)}`，随后 `POST /api/pay/prepay {order_type:"repair", order_id}`
  - 用户：`POST /api/user/repair-services/:id/adjust/decline` → 状态 `done_repair`（待发回）

## RS-07 完成修理（师傅，weapp 工作台）

- 师傅点「**完成修理**」→ 乐器进入**待发回**状态
- **网点员工接管**：联系物流、填单（填**本段实际物流费**）→ 发回 → **触发结算**
- ops: `POST /repair-services/:id/complete`（师傅）；`POST /repair-services/:id/dispatch {tracking_company, tracking_number, logistics_fee_cents}`（员工，末段实填 + 触发结算）

## RS-08 结算（系统，末段发回时触发）

- 结算口径：**预付 vs 实际（修理费基准 + 各段实际物流费合计）**
  - 修理费基准：用户**继续**加价 → **新总价**；用户**不继续** → **到此为止修理费**；未加价 → 原报价修理费
  - 实际 < 预付 → **退款**（微信原路 + 系统通知）
  - 实际 > 预付 → **补缴**（`order_payment_records(order_type='repair', status='pending')` + 系统通知；**未支付 → 阻止会员升级**，见 membership.md M-08）
- 争议：寄回后线下解决，不进系统
- 修理中丢失：走乐器丢失流程（instrument-loss.md），维修会话终止

## RS-09 评价（用户，weapp）

- 结算完成（发回）后推送「维修完成」通知（含评价邀请）
- 评价：**评分（1-5）+ 留言 + 照片**（≤6）
- **调用 / 获取**
  - 照片上传：`POST /api/upload`（现有）→ url 列表
  - 提交：`POST /api/user/repair-services/:id/review {rating, message, photos[]}`
  - 读取（本人/员工）：`GET /api/user/repair-services/:id` → `{repair, logistics_fees[], review?}`
- 展示：PC 后台维修管理页可见评分/留言/照片

## RS-11 状态机（权威，用户 2026-09-17 明确：维修单须如订单一般有自身状态机）

> **状态集**：`pending_quote` → `pending_payment` → `paid` → `shipping` → `repairing` → `done_repair` → `closed`；加价分支 `adjust_pending`。

| # | 起始状态 | 动作 | 触发者 | 守卫 | 结束状态 |
|---|---------|------|--------|------|---------|
| 1 | — | 创建维修单 | 用户 | 描述必填；分配 6 位编码 | `pending_quote` |
| 2 | `pending_quote` | 选维修师 | 用户 | 未选师；回填 site/tenant | `pending_quote`（不变） |
| 3 | `pending_quote` | 报价（修理费+物流预估） | 师傅/员工 | **必须已选师**且 JWT 归属匹配；写 `quote_status=pending` | `pending_payment` |
| 4 | `pending_payment` | 接受报价 | 用户 | `quote_status=pending` → `accepted` | `pending_payment`（不变） |
| 5 | `pending_payment` | 支付（初付，服务端重算） | 用户+系统回调 | prepay 仅允许 `pending_payment`/`adjust_pending` | `paid` |
| 6 | `paid` | 寄出（运单号） | 用户 | 详情须展示寄件网点地址 | `shipping` |
| 7 | `shipping` / `repairing` | 发起加价（新总价 + 到此为止） | 师傅/员工 | `incurred ≤ new_quote`；`quote_status=pending` | `adjust_pending` |
| 8 | `adjust_pending` | 继续并补差价 | 用户+系统回调 | 补差 = `new_quote − quote_repair`；回调置 `quote_status=accepted` | `repairing` |
| 9 | `adjust_pending` | 不继续 | 用户 | `quote_status=declined` | `done_repair` |
| 10 | `paid` / `shipping` / `repairing` | 完成修理 | 师傅/员工 | 归属匹配 | `done_repair` |
| 11 | `done_repair` | 发回 + 结算 | 员工 | 归属匹配；退款先行（失败 502 不闭单可重试） | `closed` |
| 12 | `closed` | 评价 | 用户 | `settled_at`/closed；一人一评 | `closed`（不变） |
| 13 | `closed` | 补缴支付（少补场景） | 用户+系统回调 | 存在 pending `order_type='repair'` 补缴记录 | `closed`（不变） |

**未实现（已知缺口，勿在实现中臆造）**：超时取消 / 用户主动取消 / 平台强制终止 —— 如需要请先补计划。

## RS-12 维修单详情与分状态列表（用户 2026-09-17 明确为阶段3 必备）

### 详情页（须对齐订单详情的"信息完整度"）
1. **状态时间线**：按时间展示全部状态迁移（含操作者/动作/备注），数据来源 = 维修单时间线记录（**当前缺失，需后端补**）
2. **费用明细**：报价（修理费/物流预估）→ 加价（新总价/到此为止/实际补差）→ 分段物流费逐段（段号/金额/经手人/时间）→ **已付合计** → 结算结果（退款/补缴额）→ 待补缴（若有）
3. **物流明细**：用户寄出（公司/单号）→ 各段实填（含中转段）→ 发回（公司/单号/时间）
4. **操作区**：按状态 × 角色（用户/师傅/员工）显示可用动作（即 RS-11 转换表）
5. 编码/描述/照片/寄件网点地址

### 分状态列表（不再扁平）
- **用户**「我的维修服务」：按状态分组（进行中 / 待我处理 / 已完成）+ 状态筛选；每项含编码、状态、关键金额、待办提示（如「待补差价 ¥100」/「待支付」/「待评价」）
- **师傅**工作台：待报价 / 维修中（含 `adjust_pending`）/ 已完成（自己相关）
- **员工**工作台：待发回 / 进行中（分段实填）/ 本网点全部
- **平台（PC）**：状态筛选（已有）+ 详情时间线（增强）

### 新增数据面需求（RS-API 增补）
| # | 端点/字段 | 说明 |
|---|----------|------|
| RS-API-4 | `GET /user/repair-services/:id` 增 `timeline[]` | 状态迁移时间线（所有迁移点写入） |
| RS-API-5 | `GET /user/repair-services/:id` 增 `payments{made_cents, pending_shortfall_cents, refund_cents, records[]}` | 已付/待补缴/退款汇总（补缴支付入口的前置） |
| RS-API-6 | `GET /user/repair-services?status=<csv>` | 用户侧列表状态过滤/分组（现仅返回全量） |
| RS-API-7 | prepay 允许 service 单在 `closed` + pending 补缴时按补缴额支付 | 补缴支付（#1955 item 8 缺口） |

## RS-API 端点总表（阶段3 前端实现依据）

> **登录上下文（强制，实现前必读）**：
> - **顾客（USER）**：JWT **无** `tid`/`oid` → 端点必须注册在 **`userOptionalAuth`**（`OptionalIAMInterceptor`），
>   且**不得**用 JWT 推导租户/网点（从**资源或参数**推导：如 repair 单的 `tenant_id/site_id`、`site_id` 查询参数）；
>   否则空 `tid` 触发 40104（#833 教训）。
> - **员工 / 师傅**：JWT **有** `tid`/`oid` → 端点注册在 **`authRequired`**（`IAMInterceptor`），
>   **必须**以 JWT `oid`/`tid` 做作用域与归属校验（`repairServiceStaffAllowed`，#688 教训）。
> - 同一资源两种上下文分别开端点（如顾客 `/api/user/repair-services/:id` vs 员工 `/api/repair-services/:id/...`），不复用。

| # | 端点 | 角色（登录上下文） | 路由组 | 用途 / 返回 |
|---|------|------------------|--------|------------|
| — | `POST /api/user/repair-services` | 顾客（无 oid） | userOptionalAuth | 创建 → `{id, repair_code}` |
| RS-API-1 | `GET /api/common/repair-technicians[?site_id=]`　**新增** | 顾客（无 oid） | userOptionalAuth | 可选师傅列表 → `{list:[{technician_id,name,avatar,site_id,site_name,site_address}]}`（**不得**依赖 JWT 租户；按入参/公共口径） |
| — | `POST /api/user/repair-services/:id/select-technician` | 顾客（无 oid） | userOptionalAuth | 选师 → `{id, site_id}`（归属校验按 `repair.user_id`） |
| RS-API-2 | `GET /api/repair-services?scope=mine\|site&status=<csv>`　**新增** | 师傅/员工（**有 oid**） | authRequired | 任务列表（scope=mine 指派给我；scope=site 用 JWT `oid`→`tid` 回退） |
| — | `POST /api/repair-services/:id/quote` | 师傅（有 oid） | authRequired | 报价（归属校验 `repairServiceStaffAllowed`） |
| — | `POST /api/user/repair-services/:id/accept` | 顾客（无 oid） | userOptionalAuth | 接受报价 → `{payable_cents}` |
| — | `POST /api/pay/prepay` | 顾客（无 oid） | userOptionalAuth | 支付（`order_type=repair`，服务端重算；租户从 repair 单推导） |
| RS-API-3 | `GET /api/user/repair-services/:id`　**扩展** | 顾客（无 oid）+ 员工（有 oid） | userOptionalAuth | 详情 + `site`（寄件地址/联系人）；员工可见性按 JWT 归属 |
| — | `POST /api/user/repair-services/:id/ship` | 顾客（无 oid） | userOptionalAuth | 寄出 |
| — | `POST /api/repair-services/:id/legs` | 员工（有 oid） | authRequired | 分段实填物流费 |
| — | `POST /api/repair-services/:id/adjust` | 师傅（有 oid） | authRequired | 加价申请（双字段） |
| — | `POST /api/user/repair-services/:id/adjust/accept\|decline` | 顾客（无 oid） | userOptionalAuth | 加价响应 |
| — | `POST /api/repair-services/:id/complete` | 师傅（有 oid） | authRequired | 完成修理 |
| — | `POST /api/repair-services/:id/dispatch` | 员工（有 oid） | authRequired | 末段发回 + 结算 |
| — | `POST /api/user/repair-services/:id/review` | 顾客（无 oid） | userOptionalAuth | 评价 |
| — | `GET /api/user/repair-services` | 顾客（无 oid） | userOptionalAuth | 我的维修单列表（按 `user_id`） |
| — | `GET /api/repair-services/pending-dispatch` | 员工（有 oid） | authRequired | 待发回（`RS-API-2` 兼容别名） |

> **新增端点归属**：RS-API-1/2/3 属**阶段3 前置后端补丁**（阶段2 实现时未覆盖的数据获取面），
> 在阶段3 首个执行单元中一并实现（3 文件：`handlers/repair_service.go` + `main.go` + 测试）。

## RS-10 与 v3 报修的并存与迁移

- 入口区分：小程序「维修」Tab → 「乐器报修」（v3，已租琴）/「维修服务」（本文件）
- 存量 v3 pending 单：继续走 v3 流程至完结，不迁移
- 状态机独立：`repair_requests(type='service')` 或新表（实现阶段定，倾向扩展 type）

## 角色矩阵

| 角色 | 能力 |
|------|------|
| 用户（weapp） | 浏览师傅列表/详情 → 创建（师傅锁定）/寄出/支付/加价响应/评价 |
| 师傅（weapp，**直属商户**） | 报价 / 加价 / 完成修理；**『我的维修』**（活跃会话计数） |
| 网点员工（weapp + PC） | 分段发运/实填运费 / 触发结算（物流段仍由网点/中转执行） |
| 平台管理员（PC） | 全量查看 + 评价审核视角 |

---
*Model: zhipuai/glm-5.3-flash*

## RS-13 师傅档案与归属（2026-09-18 设计变更）

- **归属**：师傅**直属商户**（tenant 级），**不再挂靠网点**。服务单 `tenant_id` = 师傅所属商户；**无 `site_id` 维度**（原「选师后回填网点」作废）
  - 过渡：历史数据中 `technician_id` 来自 `site_members(role=repair_technician)` 的服务单保持原样（不迁移）；新流程以师傅档案为准
- **档案模型**（`technician_profiles` 新表，或 users 扩展——实现时定，见实现 Issue）：
  - `user_id`（师傅，FK users）/ `tenant_id`（直属商户）/ `photo`（个人照片 URL）/ `bio`（详细介绍 text）/ `experience` jsonb（`[{craft, years}]` 专长与年限）/ `status`（active/inactive）/ `created_at/updated_at`
- **物流段执行方不变**：受控/中转模式下的分段发运仍由**网点/中转员工**执行（师傅不离开商户，但其不承担网点职能）
- **入口**：师傅列表（RS-02）仅列 `status='active'` 的档案；管理员在 PC 维护师傅档案（新增/编辑/停用）→ 实现 Issue

---
*Model: zhipuai/glm-5.3-flash*
