# 用例目录（Use Case Index）

> 来源: #1583 | cases.md 拆分迁移 | 面向 AI 的结构化用例体系
>
> **身份模型基线（#2025 / #2026 S0 裁定，全部用例适用）**：一人一记录 + `customer` 顾客角色 + 多组织 relation + 上下文切换。
> 四个裁定：**D1** 顾客上下文 JWT `tid` 保持空（隔离体系零改动）｜**D2** 手机号已注册的添加成员按管理员信任制**直接复用**（无 SMS 验证，将来接入 SMS 可升级）｜**D3** 被标记删除用户再登录激活原户｜**D4** 删除 = 标记删除 + 解除关联（记录保留、可加回）。
> 权威说明见 `docs/topics/iam/iam-notes.md` 身份模型章节。

## 目录结构

| 文件 | 业务域 | 用例编号 | 来源章节 |
|------|--------|:---:|:---:|
| `bootstrapping.md` | 冷启动/商户/用户 | B-01~ | cases.md §0 |
| `instrument.md` | 乐器管理 | I-01~ | cases.md §1 |
| `lease.md` | 租赁闭环 | L-01~L-08 | cases.md §2 |
| `lease-repair.md` | 租赁乐器维修 | R-03 | cases.md §3 + #1888 |
| `repair.md` | 客户报修 v3 | R-01~R-02 | cases.md §3 + docs/domains/repair.md |
| `organization.md` | 组织管理 | O-01~ | cases.md §4 |
| `transit.md` | 中转工作流 | T-01~ | cases.md §5 |
| `cart.md` | 购物车 | C-01~ | #1665 教训沉淀（数据刷新/下单移除/跨端导航） |
| `category.md` | 分类管理 | CAT-01~ | #1545 分类维护流程 |
| `profile.md` | 个人资料编辑 | P-01 | #1589（H5 个人中心编辑资料） |
| `user-management.md` | 平台用户管理 | P-02 | #1545 用户管理 |
| `registration-h5.md` | H5 用户注册 | P-03 | #1588 注册流程 |
| `id-photos.md` | 身份证照片全流程 | P-04 | #1787 实名核身 |
| `account-select.md` | 微信多账户登录分流 | P-05 | #1637 三通道登录 |
| `membership.md` | 会员与乐币（手册口径 v2） | M-01~ | #1939（9.14.docx 手册对齐） |
| `repair-service.md` | 维修服务改版 | RS-01~ | #1942 |
| `instrument-loss.md` | 乐器丢失与找回 | IL-01~ | #1939 派生（用户定义 2026-09-17） |
| `invoice.md` | 发票申请 v2 | INV-01~ | #1941 |

## 用例编号规范

- 格式：`{域前缀}-{序号}`（L-01, R-02, O-03...）
- 域前缀：B(ootstrapping) / I(nstrument) / L(ease) / R(epair) / O(rganization) / T(ransit) / C(art) / P(rofile&user) / M(embership) / RS(epair-service) / IL(instrument-loss) / INV(invoice) / CAT(egory)

## YAML 前置块规范（AI 消费格式）

每个用例文档头部含 YAML front-matter，字段固定：

```yaml
---
id: L-01            # 用例编号
domain: lease        # 业务域
flow: 正常租赁       # 流程名
steps:               # 步骤列表
  - seq: 1           # 步骤序号
    action: 提交订单 # 动作描述
    frontend:        # 前端维度（可多端）
      - platform: [weapp, h5]   # 端：weapp/h5/pc
        page: /checkout         # 页面路由
        role: [customer]        # 角色
        gate: ""                # 可见性/权限门控条件
        reach: "来源 → 触发 → 目标"  # 如何到达
        controls: [...]         # 控件清单
        displays: [...]         # 显示字段
        ops:                    # 支持操作
          - {type: api, method: POST, path: /user/orders}
          - {type: navigate, target: /payment}
    api:             # API 契约（后端维度）
      method: POST
      path: /user/orders
      params: [instrument_id, start_date, end_date, rent_days]
---
```

### 字段规则

| 字段 | 必填 | 说明 |
|------|:---:|------|
| id / domain / flow | ✅ | 用例标识 |
| steps[].seq / action | ✅ | 步骤序 + 动作 |
| steps[].frontend[].platform | ✅ | 涉及的端 |
| steps[].frontend[].page | ✅ | 页面路由 |
| steps[].frontend[].role | ✅ | 操作角色 |
| steps[].frontend[].gate | ⬜ | 门控条件（空串 = 无门控） |
| steps[].frontend[].reach | ✅ | 到达路径 |
| steps[].frontend[].controls | ✅ | 控件清单（AI 静态检查目标） |
| steps[].frontend[].displays | ✅ | 显示字段（对照 API 响应） |
| steps[].frontend[].ops | ✅ | 操作：`api`/`interact`/`navigate` |
| steps[].api | ⬜ | 该步骤的 API 契约（纯交互步骤可省） |

### ops 类型

- `api` — 调用后端 API（method + path 必填）
- `interact` — 纯前端交互（无需 API）

### checklist-verify platform 过滤语义（#1613）

`scripts/checklist-verify.py` 的页面注册检查按 `platform` 字段过滤：

| platform 组合 | 页面注册检查 | 说明 |
|---------------|:---:|------|
| 仅 `[pc]` | 跳过 weapp/H5 检查 | `/staff`、`/admin` 等 PC 页豁免（frontend-pc 单独验证） |
| 含 `weapp` 或 `h5` | 必须真实注册 | 跨端页面在 weappPages 或 H5 react-router 任一注册即通过 |
| 无 platform | 按现状严格检查 | 不豁免 |

**跨端死链检查**：扫描 weapp 源码中 `pages-weapp/xxx` 跳转目标，未注册于 weappPages 的报「跨端死链」（捕获 #1609 类缺口）。
- `navigate` — 页面跳转（target 必填）

## 覆盖矩阵（进展跟踪）

| 用例 | API 测试 | 前端清单 | 状态 |
|------|:---:|:---:|:---:|
| L-01 正常租赁 | ✅ TestScenarioA_StandardClosedLoop | ✅ lease.md | done |
| L-02 提前归还 | ✅ TestLeaseEarlyReturn | ✅ lease.md | done |
| L-03 超期归还 | ✅ TestInspectReturn_OverdueFee | ✅ lease.md | done |
| L-04 定损申诉（退款三路径） | ⚠️ 部分覆盖，待补退款闭环 | ✅ lease.md | wip |
| L-05 乐币规则配置 | ⬜ 待建 | ✅ lease.md | todo |
| L-06 退款差额结算与返点 | ⬜ 待建 | ✅ lease.md | todo |
| L-07 订单详情页行为（跨端标准） | ⬜ 待建 | ✅ lease.md | todo |
| L-08 员工工作台（跨端入口） | ⬜ 待建 | ✅ lease.md | todo |
| I-01 乐器录入 | ✅ TestInstrumentCRUD | ⬜ instrument.md | wip |
| P-01 个人资料编辑 | ⬜ 待建 | ✅ profile.md | wip |
| P-02 平台用户管理 | ⬜ 待建 | ✅ user-management.md | wip |
| O-01 网点管理 | ✅ TestOrgManagement | ⬜ organization.md | wip |
| 其余域 | 待建/已有 | 待写 | todo |

## 模板

- 新用例文档遵循 `docs/cases/_template.md`

---
