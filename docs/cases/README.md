# 用例目录（Use Case Index）

> 来源: #1583 | cases.md 拆分迁移 | 面向 AI 的结构化用例体系

## 目录结构

| 文件 | 业务域 | 用例编号 | 来源章节 |
|------|--------|:---:|:---:|
| `bootstrapping.md` | 冷启动/商户/用户 | B-01~ | cases.md §0 |
| `instrument.md` | 乐器管理 | I-01~ | cases.md §1 |
| `lease.md` | 租赁闭环 | L-01~L-06 | cases.md §2 |
| `repair.md` | 维修 | R-01~ | cases.md §3 + docs/repair.md |
| `organization.md` | 组织管理 | O-01~ | cases.md §4 |
| `transit.md` | 中转工作流 | T-01~ | cases.md §5 |

## 用例编号规范

- 格式：`{域前缀}-{序号}`（L-01, R-02, O-03...）
- 域前缀：B(ootstrapping) / I(nstrument) / L(ease) / R(epair) / O(rganization) / T(ransit)

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
- `navigate` — 页面跳转（target 必填）

## 覆盖矩阵（进展跟踪）

| 用例 | API 测试 | 前端清单 | 状态 |
|------|:---:|:---:|:---:|
| L-01 正常租赁 | ✅ TestScenarioA_StandardClosedLoop | ✅ lease.md | done |
| L-02 提前归还 | ✅ TestLeaseEarlyReturn | ✅ lease.md | done |
| L-03 超期归还 | ✅ TestInspectReturn_OverdueFee | ✅ lease.md | done |
| L-04 定损申诉（退款三路径） | ⚠️ 部分覆盖，待补退款闭环 | ✅ lease.md | wip |
| L-05 赠点策略配置 | ⬜ 待建 | ✅ lease.md | todo |
| L-06 退款差额结算与返点 | ⬜ 待建 | ✅ lease.md | todo |
| I-01 乐器录入 | ✅ TestInstrumentCRUD | ⬜ instrument.md | wip |
| P-01 个人资料编辑 | ⬜ 待建 | ✅ profile.md | wip |
| P-02 平台用户管理 | ⬜ 待建 | ✅ user-management.md | wip |
| O-01 网点管理 | ✅ TestOrgManagement | ⬜ organization.md | wip |
| 其余域 | 待建/已有 | 待写 | todo |

## 模板

- 新用例文档遵循 `docs/cases/_template.md`

---

*Model: deepseek/deepseek-v4-flash*
