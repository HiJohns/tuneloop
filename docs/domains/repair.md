# TuneLoop 维修功能设计（域索引）

> 版本: v3（客户报修流程 v3 重构）
> 最后更新: 2026-09-19
> 来源: `docs/cases/cases.md` §3 维修 + #1100（租赁乐器维修）+ #1109（客户报修）+ 报修流程 v3 重构共识
>
> **本文档为维修域索引**：表结构、状态枚举、端点、页面路由、费用模型已逆迁移至横切文档（单一权威），正文不再重复。

---

## 1. 概述

TuneLoop 的维修功能覆盖两类场景：

| 场景 | 触发方 | 乐器来源 | 数据驱动 |
|------|--------|----------|----------|
| **租赁乐器维修** | 员工（定损后） | `instruments` 租用乐器 | `instrument.repair_status` |
| **客户报修** | 顾客（自有乐器） | `user_instruments` 客户自有 | `repair_request.status` |

两类场景共用维修师傅角色和维修记录面板，但数据流完全独立。

> 并存域：**维修服务**（#1942，可独立购买的服务商品，type='service'）与 v3 报修并存，入口区分「乐器报修」/「维修服务」，见 `docs/cases/repair-service.md`。

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

**流程规则（#1888 锁定，验收依据）**：
- **R1 禁止自验收**：`/repair` 验收操作仅**乐器所在站点**的 `site_admin/site_member` 且 `≠ repair_worker_id`；兼职（site_member + repair_technician）同样不得验收自己完成的维修
- **R4 接手范围**：仅同站点师傅可接手（操作员站点 ∩ 乐器 `current_site_id`）
- **R5 改派权限**：改派维修负责人仅 `site_admin`
- **R7 报价可见性**：报修人本人 / 报价所属站点成员；跨网点报价互不可见（接口级返回空集而非全量）

**双向脱敏（受控/合作商户情形）**：
- 师傅方向：受控网点师傅看不到报修人信息；报价单评论**禁止出现任何联系方式**（提交前校验）
- 用户方向：用户仅见报价金额与**报价单号**，看不到师傅姓名/联系方式
- 申诉方向：用户申诉转往受控网点管理员前，由中转网点员工人工核查并**剥离用户联系方式**
- 存储原则：评价/报价内部关联真实 网点/师傅/商户（支持多维统计），仅**展示**脱敏

---

## 3. 数据模型 → `docs/spec/database/database.md`

维修域表结构与状态枚举已迁入 `docs/spec/database/database.md`：

| 主题 | 权威位置 |
|------|---------|
| 维修记录表 repair_records | `database.md` §2.30 |
| 客户自有乐器 user_instruments | `database.md` §2.31 |
| 报修单 repair_requests | `database.md` §2.32 |
| 报价单 repair_quotes | `database.md` §2.33 |
| 中转单 repair_transit_orders | `database.md` §2.34 |
| 报修日志 repair_request_records | `database.md` §2.35 |
| 维修服务分段物流费 repair_logistics_fees | `database.md` §2.36 |
| 维修服务评价 repair_reviews | `database.md` §2.37 |
| 报修单状态枚举（v3 + 维修服务） | `database.md` §2.38 |
| 乐器维修状态枚举 | `database.md` §2.39 |

---

## 4. 业务流程 → `docs/cases/repair.md`

维修业务流程已迁入结构化用例：

| 流程 | 权威位置 |
|------|---------|
| 客户报修 v3 主流程 | `cases/repair.md` R-01（创建→报价→支付→寄送→收货） |
| 维修报价调整（requote） | `cases/repair.md` R-02 |
| 租赁乐器维修 | `cases/lease-repair.md` R-03 |
| 维修服务（#1942 独立商品） | `cases/repair-service.md` RS-00~RS-10 |

---

## 5. 费用模型 → 权威文档

v3 费用模型（材料/服务/物流分项、物流四段、结算规则）已并入各权威文档：

| 主题 | 权威位置 |
|------|---------|
| 费用字段归属 | `database.md` §2.32-2.34（check_fee_snapshot / transit_service_fee 等） |
| 报价/中转费用分项 | `database.md` §2.33-2.34 |
| 结算与回退规则 | `cases/repair.md` R-01/R-02 + `api.md` §7.14.8 |
| 维修服务结算口径 | `cases/repair-service.md` RS-08 |

---

## 6. 移动端页面路由 → `docs/spec/ui/ui.md`

维修域页面设计（角色矩阵 + 8 页面规格，#1888）已收录于 `ui.md` §2.9 与 §3.25（维修服务页组）：

| 页面 | 权威位置 |
|------|---------|
| 维修中心 / 工作台 / 报修详情 / 创建报修 / 报价 / 扫码 | `ui.md` §2.9.0-2.9.7 |
| 维修服务页组（weapp + PC 后台） | `ui.md` §3.25 |

---

## 7. 图片分层规范 → `AGENTS.md`

乐器/报修图片分层规范权威定义于 AGENTS.md §Instrument Image Hierarchy，维修域沿用，不重复。

---

## 8. 相关 API → `docs/spec/api/api.md`

维修域端点已迁入 `api.md` §7：

| 端点族 | 权威位置 |
|--------|---------|
| 租赁乐器维修动作 | `api.md` §7.11 |
| 客户报修 v3 — 报修单与记录 | `api.md` §7.12 |
| 客户报修 v3 — 报价与接受 | `api.md` §7.13 |
| 客户报修 v3 — 支付与物流流转 | `api.md` §7.14 |
| 维修服务（#1942，type='service'） | `api.md` §7.15 |
| 申诉 / 费用配置 | `api.md` §7.14.8 + §7.15 族 |

---

## 9. 相关文档

- `docs/spec/database/database.md` §2.30-2.39 — 维修域表结构与状态枚举
- `docs/spec/api/api.md` §7.11-7.15 — 维修域端点
- `docs/spec/ui/ui.md` §2.9 / §3.25 — 维修域页面设计
- `docs/cases/repair.md` — R-01 客户报修 v3 / R-02 报价调整
- `docs/cases/lease-repair.md` — R-03 租赁乐器维修结构化用例
- `docs/cases/repair-service.md` — RS-00~RS-10 维修服务（#1942 并存域）
- `AGENTS.md` §Instrument Image Hierarchy — 图片分层规范
- `AGENTS.md` §Repair Request Tables — 报修表字段说明
- `AGENTS.md` §维修页角色×页面矩阵 — 角色差异化视图