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
          - {type: api, method: POST, path: /admin/merchants}
    api: {method: POST, path: /admin/merchants, params: [name, contact, type]}
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
          - {type: api, method: POST, path: /site-members}
    api: {method: POST, path: /site-members, params: [user_id, site_id, role]}
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
