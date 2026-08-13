---
id: P-05
domain: auth
flow: 微信多账户登录分流（weapp）
steps:
  - seq: 1
    action: 触发登录分流（来源 A：个人中心「我的」）
    frontend:
      - platform: [weapp]
        page: /profile
        role: [guest]
        gate: "未登录"
        reach: "底部导航 → 我的 → wx.login → GET /auth/wx-accounts"
        controls: [我的入口]
        displays: []
        ops:
          - {type: api, method: GET, path: /auth/wx-accounts}
    api: {method: GET, path: /auth/wx-accounts, params: [code]}
  - seq: 2
    action: 来源 A 分流：0/1/N 账户
    frontend:
      - platform: [weapp]
        page: /profile
        role: [guest]
        gate: "wx-accounts 返回"
        reach: ""
        controls: []
        displays: []
        ops:
          - {type: navigate, target: /profile-complete, gate: "accounts.length == 0"}
          - {type: navigate, target: /profile, gate: "accounts.length == 1 → wx-login-select 直接登录"}
          - {type: navigate, target: /account-select, gate: "accounts.length >= 2"}
    api:
      - {method: POST, path: /auth/wx-login-select, params: [code, user_id]}
      - {method: POST, path: /auth/wx-login, params: [code]}
  - seq: 3
    action: 触发登录分流（来源 B：购物车提交/立即租赁）
    frontend:
      - platform: [weapp]
        page: /checkout
        role: [guest]
        gate: "未登录且点击提交/立即租赁"
        reach: "Checkout 提交 / Detail 立即租赁 → wx.login → GET /auth/wx-accounts"
        controls: [提交订单按钮, 立即租赁按钮]
        displays: []
        ops:
          - {type: api, method: GET, path: /auth/wx-accounts}
    api: {method: GET, path: /auth/wx-accounts, params: [code]}
  - seq: 4
    action: 来源 B 分流：有顾客 vs 无顾客
    frontend:
      - platform: [weapp]
        page: /checkout
        role: [guest]
        gate: "wx-accounts 返回"
        reach: ""
        controls: [注册提示弹窗]
        displays: []
        ops:
          - {type: navigate, target: /checkout, gate: "有顾客账户 → wx-login-select 登录后回目标页"}
          - {type: navigate, target: "/profile-complete?mode=member", gate: "无顾客账户 → 弹「您尚未注册会员」→ 是"}
          - {type: navigate, target: /checkout, gate: "无顾客账户 → 弹窗选「否」→ 停留原页"}
    api: {method: POST, path: /auth/wx-login-select, params: [code, user_id]}
  - seq: 5
    action: 账户列表页展示与登录
    frontend:
      - platform: [weapp]
        page: /account-select
        role: [guest]
        gate: ""
        reach: "来源 A N 账户 → 账户列表页"
        controls: [账户登录按钮, 用户名密码登录链接, 注册为会员链接]
        displays: [顾客昵称, 员工昵称（商户名-网点名）]
        ops:
          - {type: api, method: POST, path: /auth/wx-login-select, params: [code, user_id]}
          - {type: navigate, target: /profile-complete, gate: "点击「注册为会员」"}
    api: {method: POST, path: /auth/wx-login-select, params: [code, user_id]}
  - seq: 6
    action: 注册页去用户名 + mode=member
    frontend:
      - platform: [weapp]
        page: /profile-complete
        role: [guest]
        gate: ""
        reach: "分流 0 账户 / 来源 B 注册提示 → 注册页"
        controls: [昵称输入, 姓名输入, 手机号输入, 注册按钮, 用户名密码登录链接]
        displays: []
        ops:
          - {type: api, method: POST, path: /auth/register, params: [nickname, name, phone, exchange_token]}
          - {type: navigate, target: /account-select, gate: "点击「用户名密码登录」且非 mode=member"}
    api: {method: POST, path: /auth/register, params: [nickname, name, phone, exchange_token]}
  - seq: 7
    action: 会员中心切换账户
    frontend:
      - platform: [weapp]
        page: /profile
        role: [staff]
        gate: "员工账户（有 oid/tid）"
        reach: "会员中心 → 退出登录 + 切换账户"
        controls: [退出登录, 切换账户]
        displays: []
        ops:
          - {type: navigate, target: /account-select, gate: "点击「切换账户」"}
    api: {}
  - seq: 8
    action: 购物车合并去重
    frontend:
      - platform: [weapp]
        page: /cart
        role: [customer]
        gate: "登录后"
        reach: "登录完成后回购物车 → 游客 cart 与 cart_${userId} 合并去重"
        controls: []
        displays: []
        ops: []
    api: {}
---

# P-05 微信多账户登录分流

## 前置条件
- weapp 端（微信小程序），beaconiam #483 多绑定已部署
- 依赖后端接口：`GET /auth/wx-accounts`、`POST /auth/wx-login-select`（tuneloop #1638）

## 登录分流规则（核心）

### 来源 A：个人中心「我的」
- **已登录** → 直接进会员中心（不重新查 openid）
- **未登录** → `wx.login` → `GET /auth/wx-accounts`
  - 0 账户 → 注册页（显示用户名密码登录）
  - 1 账户 → `POST /auth/wx-login-select { code, user_id }` 直接登录 → 会员中心
  - N 账户 → 账户列表页 `/account-select`

### 来源 B：购物车提交 / 立即租赁
- `wx.login` → `GET /auth/wx-accounts`
- 关联账户中**有顾客** → 直接登录该顾客 → 回到目标页
- 关联账户中**无顾客**（0 账户或只有员工）→ 弹「您尚未注册会员，要注册吗？」
  - 是 → 注册页 `?mode=member`（隐藏用户名密码登录）
  - 否 → 取消，停留原页

## 账户列表页（/account-select）
- 顾客账户 → 显示昵称
- 员工账户 → 显示昵称（商户名-网点名，主组织）
- 「用户名密码登录」链接（仅员工场景显示）→ 展开账号密码面板
- 「注册为会员」链接（无顾客账户时显示）→ 注册页
- 点击账户 → `POST /auth/wx-login-select { code, user_id }` → 登录 → 回目标页

## 注册页改造
- 去掉「用户名」输入框（#1638 后端：username 由 phone 派生）
- 昵称 = 微信昵称（可编辑，#1589 教训：可输入，不用 type="nickname"）
- 底部「用户名密码登录」→ 账户列表页
- `?mode=member`（员工弹窗进入）→ 隐藏「用户名密码登录」
- 注册提交绑定身份：优先传 `exchange_token`（wx-accounts mint，单次、5min TTL）；无 token 时 fallback 新 `wx.login()` code
- **exchange_token 过期重试**（#1648）：exchange_token 超过 5min 过期 → 注册返回 409「微信绑定失败」→ 前端清除 session `wx_login_token` → 用户重试时走新 `wx.login()` code（全新 code，无 40163）

## 会员中心（Profile）
- 纯用户（无关联员工账户）→ 只有「退出登录」
- 员工账户 → 「退出登录」+「切换账户」→ 账户列表页

## 购物车合并（方案 b：前端去重合并）
- 登录后，游客购物车（`cart`）与账户购物车（`cart_${userId}`）合并**去重**（按 instrument_id + 租期）
- 合并后清空游客 cart，写入账户 cart

## 验收
- 我的：0/1/N 账户分叉正确
- 购物车提交/立即租赁：有顾客直接登录回目标页；无顾客弹注册提示
- 账户列表页：顾客昵称、员工昵称（商户-网点主组织）
- 注册页：无用户名、昵称可编辑、mode=member 隐藏用户名密码登录
- 会员中心：员工切换账户、纯用户只退出登录
- 购物车合并去重
- eslint 0 no-undef + H5/weapp build 通过
- checklist-verify.py --behavioral 通过（本 cases 的 navigate/gate 条目兜底）
- 注册 exchange_token 过期后重试走新 code（无死循环）

---

*Last updated: 2026-08-13*
