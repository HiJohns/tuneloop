---
id: NAV-01
domain: navigation
flow: 底部导航（角色 × 显隐与跳转）
steps:
  - seq: 1
    action: 进入任意主页面，底部常驻导航渲染
    frontend:
      - platform: [weapp, h5]
        page: (全局) BottomNav
        role: [customer, staff, technician, guest]
        gate: ""
        reach: "首页 / 租赁 / 维修 / 我的 等主页面底部常驻"
        controls: [首页, 租赁, 维修, 我的]
        displays: [当前 tab 高亮, 角标（profile 未读）]
        ops:
          - {type: api, method: GET, path: /api/site-members/me}
          - {type: navigate, target: "见「跳转语义」"}
    api: {method: GET, path: /api/site-members/me, params: []}
  - seq: 2
    action: 点击「维修」Tab
    frontend:
      - platform: [weapp, h5]
        page: /my-repairs 或 /tech-list
        role: [customer, staff, technician]
        gate: ""
        reach: "底栏 → 维修"
        controls: [维修 Tab]
        displays: []
        ops:
          - {type: navigate, target: "员工 → /my-repairs；顾客 → /tech-list"}
    api: {}
---

# NAV-01 底部导航（角色 × 显隐与跳转）

> 来源：#2083（统一组件）｜关联：#1884（纯师傅隐藏租赁）/ #2050（角色互斥）/ #2081（个人中心「切换身份」按钮）/ #2082（roles 含 fn_roles）
> 权威 UI 规则：`docs/spec/ui/ui.md` 底部导航章节（本文件为用例视角，两者须一致）

## 前置条件
- 角色数据源：`GET /api/site-members/me` → `roles` = **网点角色 ∪ JWT `fn_roles`**（过滤 `customer`，`#2082`）
- 纯维修师判定：`isPureTechnician(roles)` = `roles` 含 `repair_technician` 且不含 `site_admin`/`site_member`（`utils/role.js`）
- 员工判定：`isStaffRole()`（`oid`/`tid` 非空 **或** 角色非 `USER`/`GUEST`）
- 未登录/游客：不拉取角色，按顾客处理（`#1681` 匿名态）

## 显隐矩阵（tabs 由统一组件构造）

| 角色 | 首页 | 租赁 | 维修 | 我的 |
|------|:---:|:---:|:---:|:---:|
| 顾客（`role=USER`） | ✅ | ✅ | ✅ → `/tech-list` | ✅ |
| 网点员工（`site_admin`/`site_member`） | ✅ | ✅ | ✅ → `/my-repairs` | ✅ |
| **纯维修师**（`repair_technician` 且无 site 角色） | ✅ | **❌ 隐藏**（#1884） | ✅ → `/my-repairs` | ✅ |
| 游客/未登录 | ✅ | ✅ | ✅ → `/tech-list` | ✅ |

## 跳转语义
- **首页 / 租赁 / 我的**：weapp `Taro.switchTab`（tab 页）；H5 `navigate`
  - H5 租赁目标：员工 → `/staff/orders`；顾客 → `/my-leases`
- **维修**：员工 → `/my-repairs`；顾客 → `/tech-list`
  - weapp：`Taro.navigateTo`（页面栈 ≥9 时 `reLaunch`）；当前已在维修页 → **no-op**
  - H5：`navigate`
- **tenant 透传**：weapp 首页把 `tenant` 传给组件 → 组件暂存 `tab_params`（租赁页读取）；H5 首页把 `tenant` 拼入 service/profile 查询串

## 实现约定（#2083，强制）
- tabs **统一由** `components/BottomNav.jsx`（H5）/ `components-weapp/BottomNav.jsx`（weapp）**按角色构造**；页面只传 `active` / `badges`（H5 另传 `navigate`；weapp 首页另传 `tenant`）
- **禁止页面自行拼 `tabs` 数组**（#2083 事故根因：各页显隐口径漂移）
- 角色拉取在组件内完成（挂载时一次 `GET /site-members/me`；未登录跳过）

## 关键规则
- 纯维修师**不得**显示「租赁」Tab（#1884）
- 员工**不得**进入顾客维修入口：直达 `/tech-list` → 重定向回 `/my-repairs`（#2050）
- 底栏为 `absolute` 覆盖层：页面滚动区底部需留垫片（≥ nav 高度）
- 个人中心另有「切换身份」按钮（#2081，条件：员工上下文 **或** 可登录上下文 >1），与底栏 Tab 互不替代
- 个人中心金刚区按角色适配（#2089）：纯师傅 = 待报价（角标）/ 维修中 / 已完成 / 系统通知；员工兼师傅 = 员工项 + 待报价；详见 `docs/spec/ui/ui.md`「金刚区」

## 验收（真机）
- [ ] 顾客：四项齐备；点「维修」进 `/tech-list`
- [ ] 网点员工：四项齐备；点「维修」进 `/my-repairs`
- [ ] 纯维修师：**仅三项**（无租赁）；点「维修」进 `/my-repairs`
- [ ] 游客：四项齐备（维修 → `/tech-list`）
- [ ] weapp 首页带 `tenant` 进入后，租赁页可读到 `tab_params.tenant`
