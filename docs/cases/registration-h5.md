---
id: P-03
domain: auth
flow: H5 用户注册
steps:
  - seq: 1
    action: 从 IAM 登录页跳转到注册页
    frontend:
      - platform: [h5]
        page: /register
        role: [guest]
        gate: ""
        reach: "IAM 登录页 → 「没有账号？注册」链接 → H5 注册页"
        controls: []
        displays: []
        ops: []
    api: {}
  - seq: 2
    action: 填写注册信息
    frontend:
      - platform: [h5]
        page: /register
        role: [guest]
        gate: ""
        reach: "/register 页面直接访问"
        controls: [用户名输入, 姓名输入, 手机号输入, 邮箱输入, 密码输入, 头像上传, 昵称输入, 收货地址省市区选择, 身份证正反面上传]
        displays: []
        ops: []
    api: {}
  - seq: 3
    action: 提交注册
    frontend:
      - platform: [h5]
        page: /register
        role: [guest]
        gate: ""
        reach: ""
        controls: [注册按钮]
        displays: [提交中状态]
        ops:
          - {type: api, method: POST, path: /auth/register}
    api: {method: POST, path: /auth/register, params: [username, name, nickname, phone, email, password, ref]}
  - seq: 4
    action: 注册成功后跳转会员费支付
    frontend:
      - platform: [h5]
        page: /payment
        role: [customer]
        gate: "注册成功，token 已存储"
        reach: "注册成功 → redirect 到会员费支付"
        controls: [支付按钮]
        displays: [金额, 会员权益说明]
        ops:
          - {type: api, method: GET, path: /user/membership/info}
    api: {method: GET, path: /user/membership/info, params: []}
  - seq: 5
    action: 已有账号时从注册页跳回登录
    frontend:
      - platform: [h5]
        page: /register
        role: [guest]
        gate: ""
        reach: "注册页 → 「已有账号？登录」→ IAM 登录页"
        controls: [登录链接]
        displays: []
        ops: []
    api: {}
  - seq: 6
    action: IAM 登录页增加注册入口
    frontend:
      - platform: [beaconiam]
        page: /oauth/authorize
        role: [guest]
        gate: ""
        reach: "IAM 登录页底部 → 「没有账号？注册」→ H5 注册页"
        controls: [注册链接]
        displays: []
        ops: []
    api: {}
---

# P-03 H5 用户注册

## 前置条件
- 用户访问 H5 页面（浏览器），未登录
- IAM namespace 已激活，OAuth app 已注册
- beaconiam 侧已配置 `registration_redirect_url` 指向 tuneloop H5 注册页

## 流程
1. IAM 登录页底部显示「没有账号？注册」链接 → 跳转 `/register`
2. 用户填写注册信息（用户名、姓名、手机号必填）→ 提交 `POST /api/auth/register`
3. 后端：IAM 建户 → 微信绑定（H5 无 wx_code，此步跳过）→ 密码登录获取 JWT → 本地 users 表同步 → 注册赠点（99pt）+ 推荐奖励（如有 ref）
4. 前端存储 token → 重定向到会员费支付页（`/payment?type=membership`）
5. 会员费支付完成后 → 会员等级激活（`applySideEffects`）→ 进入首页

## H5 vs weapp 注册差异

| 维度 | H5 注册 | weapp 注册 (ProfileComplete) |
|------|---------|------------------------------|
| 后端 API | `POST /api/auth/register`（同） | `POST /api/auth/register`（同） |
| 身份来源 | 用户名+密码 | 微信 openid（wx_code） |
| 注册赠点 | 99pt（同） | 99pt（同） |
| 推荐系统 | ✅（ref 参数） | ✅（ref 参数 + scene） |
| 头像上传 | ✅（自定义文件选择器） | ✅（微信原生选择器） |
| 身份证上传 | ✅（H5 文件选择器） | ❌（待补） |
| 收货地址 | ✅ | ✅（P-01 已覆盖） |

## 与 Onboarding 页面的关系

注册页完成后：
1. `POST /api/auth/register` 返回 token → 用户已登录
2. 注册成功后**不**再跳转 `/onboarding`（login_reason/post_auth_redirect 已被注册流程覆盖）
3. 注册页本身收集了 Onboarding 所需的全部字段（昵称、地址、身份证）→ Onboarding 对该用户无意义
4. **建议**: 注册成功的用户标记 `onboarding_completed = true`，避免后续 Onboarding 拦截

## IAM 侧改动（beaconiam）
- `AppRegistration` 或 `ActivateNamespace` 时写入 `registration_redirect_url` 字段
- IAM 登录页 (`/oauth/authorize`) 底部渲染此链接：「没有账号？注册」
- 点击后跳转到 `registration_redirect_url`（如 `https://web.cadenzayueqi.com/register`）
- 此改动在 beaconiam 仓库，关联 Issue: [beaconiam#482](https://github.com/HiJohns/beaconiam/issues/482)

## 验收
- Go test: `POST /api/auth/register` with password → 20000，DB 本地用户创建 + 赠点 transaction
- checklist-verify.py: P-03 controls/displays 与注册页 JSX 交叉验证
- 端到端: 注册 → 会员费支付 → 激活 → 首页看到会员等级

## 已知缺口
- 注册页无需微信登录按钮（H5 无 wx.login）
- 注册页无需区分「微信昵称」——昵称即为自由文本
- 密码字段需要二次确认（confirm password，避免 typos）

---

*Last updated: 2026-08-09*
