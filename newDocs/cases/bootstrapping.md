---
id: B-01
domain: bootstrapping
flow: 商户创建
steps:
  - seq: 1
    action: 创建商户
    frontend:
      - platform: [pc]
        page: /merchants/new
        role: [tenant_admin]
        gate: "拥有商户创建权限"
        reach: "商户管理 → 新建"
        controls: [商户名称, 联系人, 类型]
        displays: []
        ops:
          - {type: api, method: POST, path: /merchants}
    api: {method: POST, path: /merchants, params: [name, contact, type]}
    related:
      - {method: GET, path: /merchants}
      - {method: GET, path: /merchants/:id}
      - {method: PUT, path: /merchants/:id}
      - {method: DELETE, path: /merchants/:id}
      - {method: GET, path: /staff}
      - {method: GET, path: /users}
      - {method: POST, path: /user/reset-password}
      - {method: POST, path: /user/change-password}
---

# B-01 商户创建

## 前置条件
- 租户管理员

## 流程
1. 创建商户 → IAM namespace 内创建组织
2. 初始化角色（#663）

## 关键规则
- 商户创建后必须初始化角色模板

---
id: B-02
domain: bootstrapping
flow: 用户绑定（IAM 三步）
steps:
  - seq: 1
    action: 绑定用户到组织
    frontend:
      - platform: [pc]
        page: /staff/:id
        role: [merchant_admin]
        gate: ""
        reach: "人员管理 → 绑定"
        controls: [绑定按钮]
        displays: []
        ops:
          - {type: api, method: POST, path: /sites/:id/members}
    api: {method: POST, path: /sites/:id/members, params: [user_id, site_id, role]}
---

# B-02 用户绑定

## 流程
1. IAM bind → cus_perm → role 模板三步绑定
2. 本地 users/site_members 缓存同步

## 关键规则
- IAM 是权威源，本地仅缓存（#685）
- 操作顺序：先 IAM 后本地

---

*Model: deepseek/deepseek-v4-flash*

---
id: B-02
domain: bootstrapping
flow: 商户编辑交互（#1717）
steps:
  - seq: 1
    action: 进入商户详情
    frontend:
      - platform: [pc]
        page: /merchants/:id
        role: [tenant_admin]
        reach: "商户管理 → 点击商户 → 详情"
        controls: [商户标题栏（名称/类型/状态标签）, 信息 Tab, 分账配置 Tab, 成员管理 Tab]
        displays: [商户基本信息（ID/名称/电话/地址/类型/返点）]
  - seq: 2
    action: 编辑商户基本信息
    frontend:
      - platform: [pc]
        page: /merchants/:id
        role: [tenant_admin]
        gate: "详情态"
        reach: "信息 Tab → 编辑基本信息按钮（唯一编辑入口）"
        controls: [编辑基本信息按钮, 表单字段（商户名/电话/地址/类型/中转信息/返点）]
        displays: [编辑表单, 商户标题栏（名称/类型/状态标签保留可见）]
        ops:
          - {type: api, method: PUT, path: /merchants/:id}
  - seq: 3
    action: 取消编辑
    frontend:
      - platform: [pc]
        page: /merchants/:id
        role: [tenant_admin]
        gate: "编辑表单态"
        controls: [取消按钮（唯一，无回退按钮）]
        ops:
          - {type: navigate, target: /merchants/:id}
    # 取消后回到商户详情（基本信息 Tab），面板不消失（#1717）
---

# B-02 商户编辑交互（#1717）

## 前置条件
- 租户管理员，已进入商户详情

## 关键规则
- 编辑入口唯一：信息 Tab「编辑基本信息」（标题栏无重复编辑按钮）
- 编辑态保留商户标题栏（名称/类型/状态标签可见）
- 取消按钮唯一（无回退按钮）；取消后回到商户详情（基本信息 Tab），面板不消失
