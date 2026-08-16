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
    action: 来源 A 分流：0/1/N 账户（0 账户再查注册会话）
    frontend:
      - platform: [weapp]
        page: /profile
        role: [guest]
        gate: "wx-accounts 返回"
        reach: ""
        controls: []
        displays: []
        ops:
          - {type: navigate, target: /profile, gate: "accounts.length == 1 → wx-login-select 直接登录"}
          - {type: navigate, target: /account-select, gate: "accounts.length >= 2"}
          - {type: api, method: GET, path: /auth/registration-sessions/me, gate: "accounts.length == 0 → 查 pending 会话"}
    api:
      - {method: POST, path: /auth/wx-login-select, params: [code, user_id]}
      - {method: POST, path: /auth/wx-login, params: [code]}
      - {method: GET, path: /auth/registration-sessions/me, params: [code]}
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
    action: 注册页填写信息 + 支付会员费（两阶段会话制）
    frontend:
      - platform: [weapp]
        page: /profile-complete
        role: [guest]
        gate: ""
        reach: "分流 0 账户 / 来源 B 注册提示 / 会员中心「注册为会员」/「继续完成注册」→ 注册页"
        controls: [昵称输入, 姓名输入, 手机号输入, 邮箱输入, 收货地址选择, 支付会员费按钮, 用户名密码登录链接]
        displays: [会员费金额]
        ops:
          - {type: api, method: POST, path: /auth/registration-sessions, params: [nickname, name, phone, email, exchange_token, address]}
          - {type: navigate, target: "/payment?type=membership&session_id=:id", gate: "会话创建成功 → 跳支付页"}
          - {type: navigate, target: /account-select, gate: "点击「用户名密码登录」且非 mode=member"}
    api: {method: POST, path: /auth/registration-sessions, params: [nickname, name, phone, email, exchange_token, address]}
  - seq: 6b
    action: 会员费支付（优惠码 + 微信支付）
    frontend:
      - platform: [weapp]
        page: /payment
        role: [guest]
        gate: "type=membership&session_id 存在"
        reach: "注册页「支付会员费」→ 支付页"
        controls: [优惠码输入, 应用优惠码按钮, 发起支付按钮]
        displays: [会员费金额, 优惠后金额, 权益说明]
        ops:
          - {type: api, method: POST, path: /pay/prepay, params: [order_type, session_id, coupon_code, open_id]}
    api: {method: POST, path: /pay/prepay, params: [order_type, session_id, coupon_code, open_id]}
  - seq: 6c
    action: 支付完成自动登录（回调建户后）
    frontend:
      - platform: [weapp]
        page: /payment
        role: [guest]
        gate: "微信支付成功回调"
        reach: "支付成功 → 轮询会话状态 → 触发登录"
        controls: []
        displays: [注册处理中提示]
        ops:
          - {type: api, method: GET, path: /auth/registration-sessions/:id/status}
          - {type: api, method: GET, path: /auth/wx-accounts, gate: "会话 completed → 重新分流登录"}
          - {type: navigate, target: /profile, gate: "wx-accounts 1 账户 → wx-login-select 登录 → 会员中心"}
    api: {method: GET, path: /auth/registration-sessions/:id/status, params: []}
  - seq: 6d
    action: 未支付完成重进 → 继续完成注册
    frontend:
      - platform: [weapp]
        page: /profile
        role: [guest]
        gate: "未登录且存在 pending 会话"
        reach: "会员中心未登录 → GET /auth/registration-sessions/me 查 pending 会话"
        controls: [继续完成注册按钮]
        displays: []
        ops:
          - {type: api, method: GET, path: /auth/registration-sessions/me}
          - {type: navigate, target: "/profile-complete?session_id=:id", gate: "点击「继续完成注册」→ 恢复表单"}
    api: {method: GET, path: /auth/registration-sessions/me, params: [code]}
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
- **未登录** → **进入页面时静默查询** `wx.login` → `GET /auth/wx-accounts`，**按钮文案按查询结果动态决定**（非固定「注册为会员」）：
  - 关联账户数 ≥ 1（当前微信用户已注册，只是未登录）→ 按钮「**登录**」→ 点击后 1 账户直接 wx-login-select / N 账户进账户列表页
  - 0 账户 + 有 pending 会话 → 按钮「**继续完成注册**」→ 注册页恢复表单
  - 0 账户 + 无 pending 会话 → 按钮「**注册为会员**」→ 注册页（显示用户名密码登录）
- 点击按钮 → 按上述分流执行（点击时重新 `wx.login` 拿新 code，进入页面时查询消耗的 code 不影响）

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

## 注册页改造（两阶段会话制，weapp）
- 去掉「用户名」输入框（#1638 后端：username 由 phone 派生）
- 昵称 = 微信昵称（可编辑）：weapp 注册页（ProfileComplete，weapp 独占）用 `type="nickname"` 让微信键盘带出昵称快捷填充；H5 薄壳共享页禁用（#1589 教训：H5 下锁死输入）
- 底部「用户名密码登录」→ 账户列表页
- `?mode=member`（员工弹窗进入）→ 隐藏「用户名密码登录」
- 提交按钮文案：「**支付会员费**」（非「注册」）

### 两阶段注册流程（#1663）
1. **第一阶段**（注册页）：填写表单 → 点击「支付会员费」→ `POST /auth/registration-sessions`（表单 + exchange_token）→ 后端创建会话（status=pending，金额=会员费基准价）→ 返回 `session_id` + `amount`
2. session_id 存本地 `pending_registration_session`
3. **第二阶段**（支付页）：`?type=membership&session_id=:id&amount=:amount` → 输入优惠码 → `POST /pay/prepay { order_type: membership, session_id, coupon_code }` → 后端计算金额 → 微信支付
4. **支付回调建户**（服务端权威）：微信支付成功回调 → 查会话 → 标记 paid → **服务端完成注册**（CreateUser + WxBind/绑定 openid + 本地同步 + 赠点）→ 标记 completed
5. **自动登录**：前端轮询 `GET /auth/registration-sessions/:id/status` → completed → 清除本地 session → 触发 wx-accounts 分流 → 1 账户 → wx-login-select 登录 → 会员中心

### 优惠码
- 会员费支付页输入优惠码（membership 场景）
- 金额**后端计算**（前端只传 code，不可信）
- 默认优惠码（生产库）：`OREZ`（waive 全额免除 → ¥0，直接完成注册，仍写 payment record 留痕）、`ENO`（percent 1% → ¥0.99）
- 测试完成后从库中删除（优惠码有 active 状态可停用）

### 会话恢复（继续完成注册）
- 支付未完成（pending），再次打开会员中心（未登录）：
  - `GET /auth/registration-sessions/me`（wx.login code → 后端兑换 openid 查会话）
  - 有 pending 会话 → 登录按钮「**继续完成注册**」→ 打开注册页恢复表单（form_data 预填）
  - 无会话 → 登录按钮「**注册为会员**」→ 注册页
- 会话按 openid 关联：换设备/清本地缓存也能找回

### 关键约束
- 支付完成前不创建任何账户（users 表无记录 → 无孤儿）
- 服务端支付回调为权威：建户只在回调内完成，客户端提交仅为查询/兜底
- 会话状态机幂等：pending → paid → completed；completed 只执行一次建户；回调可重复到达
- exchange_token 过期（#1648）：前端清除旧 token 重试走新 wx.login code

## 会员中心（Profile）
- 纯用户（无关联员工账户）→ 只有「退出登录」
- 员工账户 → 「退出登录」+「切换账户」→ 账户列表页
- **未登录（guest）** → 登录按钮三态：
  - 有 pending 注册会话 → 「**继续完成注册**」→ 注册页恢复表单
  - 无会话且微信未注册 → 「**注册为会员**」→ 注册页
  - （微信已注册 → 走 wx-login 正常登录，见登录分流规则）

## 购物车合并（方案 b：前端去重合并）
- 登录后，游客购物车（`cart`）与账户购物车（`cart_${userId}`）合并**去重**（按 instrument_id + 租期）
- 合并后清空游客 cart，写入账户 cart

## 验收
- 我的：0/1/N 账户分叉正确
- 购物车提交/立即租赁：有顾客直接登录回目标页；无顾客弹注册提示
- 账户列表页：顾客昵称、员工昵称（商户-网点主组织）
- 注册页：无用户名、昵称可编辑、mode=member 隐藏用户名密码登录、提交按钮「支付会员费」
- 两阶段注册：会话创建 → 支付 → 回调建户 → 自动登录
- 优惠码：OREZ→¥0 直接完成（留痕）、ENO→¥0.99，金额后端计算
- 会话恢复：未支付重进 → 「继续完成注册」恢复表单；未注册无缓存 → 「注册为会员」
- 会员中心：员工切换账户、纯用户只退出登录、未登录三态按钮
- 支付完成前无孤儿用户（users 表无记录）
- 购物车合并去重
- eslint 0 no-undef + H5/weapp build 通过
- checklist-verify.py --behavioral 通过（本 cases 的 navigate/gate 条目兜底）
- 注册 exchange_token 过期后重试走新 code（无死循环）

---

*Last updated: 2026-08-13*
