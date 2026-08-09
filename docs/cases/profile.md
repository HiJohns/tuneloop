---
id: P-01
domain: profile
flow: 个人资料编辑
steps:
  - seq: 1
    action: 打开个人中心
    frontend:
      - platform: [weapp, h5]
        page: /profile
        role: [customer]
        gate: "已登录（非游客）"
        reach: "底部导航 → 我的"
        controls: [编辑资料入口, 昵称显示, 会员等级显示]
        displays: [昵称, 会员等级, 积分]
        ops:
          - {type: api, method: GET, path: /users/me}
    api: {method: GET, path: /users/me, params: []}
  - seq: 2
    action: 进入编辑页
    frontend:
      - platform: [weapp]
        page: /profile/edit
        role: [customer]
        gate: ""
        reach: "个人中心 → 编辑资料"
        controls: [昵称输入, 手机号输入, 邮箱输入, 保存按钮]
        displays: [当前昵称, 当前手机号, 当前邮箱]
        ops:
          - {type: api, method: GET, path: /users/me}
    api: {method: GET, path: /users/me, params: []}
  - seq: 3
    action: 保存修改
    frontend:
      - platform: [weapp]
        page: /profile/edit
        role: [customer]
        gate: ""
        reach: ""
        controls: [保存按钮]
        displays: []
        ops:
          - {type: api, method: PUT, path: /users/me}
          - {type: navigate, target: /profile}
    api: {method: PUT, path: /users/me, params: [nickname, phone, email]}
  - seq: 4
    action: 返回个人中心验证
    frontend:
      - platform: [weapp, h5]
        page: /profile
        role: [customer]
        gate: ""
        reach: "编辑页返回"
        controls: [昵称显示]
        displays: [更新后的昵称, 更新后的手机号]
        ops:
          - {type: api, method: GET, path: /users/me}
    api: {method: GET, path: /users/me, params: []}
---

# P-01 个人资料编辑

## 前置条件
- 用户已登录（顾客）

## 流程
1. 个人中心 → 编辑资料（weapp 独立页 / H5 弹窗）
2. 修改昵称/手机号/邮箱 → 保存 → PUT /users/me
3. 返回个人中心 → 显示更新后的值（useDidShow 刷新）

## 关键规则
- GET /users/me 必须返回 nickname 字段（#1588 缺陷 1）
- 编辑保存返回后页面必须刷新显示新值（#1588 缺陷 2）
- 邮箱修改需 IAM 确认（email_confirmation=pending）

---

*Model: deepseek/deepseek-v4-flash*
