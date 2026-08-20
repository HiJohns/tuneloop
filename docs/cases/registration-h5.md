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
        controls: [姓名输入, 手机号输入, 邮箱输入, 密码输入, 头像上传, 昵称输入, 收货地址省市区选择, 身份证正反面上传]
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
    api: {method: POST, path: /auth/register, params: [nickname, name, phone, email, password, ref]}
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
          - {type: api, method: POST, path: /pay/calculate}
    api: {method: POST, path: /pay/calculate, params: [type, id]}
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
2. 用户填写注册信息（姓名、手机号必填；昵称 = 微信昵称可编辑，#1638 去用户名）→ 提交 `POST /api/auth/register`
3. 后端：IAM 建户（用户名由 phone 派生，无 username 输入路径）→ 微信绑定（H5 无 wx_code，此步跳过）→ 密码登录获取 JWT → 本地 users 表同步 → 注册赠点（99pt）+ 推荐奖励（如有 ref）
4. 前端存储 token → 重定向到会员费支付页（`/payment?type=membership`）
5. 会员费支付完成后 → 会员等级激活（`applySideEffects`）→ 进入首页

## H5 vs weapp 注册差异

| 维度 | H5 注册 | weapp 注册 (ProfileComplete) |
|------|---------|------------------------------|
| 后端 API | `POST /api/auth/register`（同） | **两阶段会话制**：`POST /auth/registration-sessions`（写会话，不建户）→ 支付回调建户（#1663） |
| 身份来源 | 用户名+密码 | 微信 openid（wx_code / exchange_token） |
| 用户名 | 无输入框（phone 派生） | 无输入框（phone 派生，#1639 改造） |
| 注册时机 | 提交即建户返回 token | **支付会员费完成后才建户**（支付前无账户） |
| 提交按钮 | 注册 | **支付会员费** |
| 会员费 | 注册后跳支付页（可选） | 注册前强制支付（两阶段） |
| 优惠码 | 无 | OREZ（全额免）/ ENO（1%） |
| 注册赠点 | 99pt（同） | 99pt（回调建户时） |
| 推荐系统 | ✅（ref 参数） | ✅（ref 参数 + scene） |
| 头像上传 | ✅（自定义文件选择器） | ✅（微信原生选择器） |
| 身份证上传 | ✅（H5 文件选择器） | ❌（待补） |
| 收货地址 | ✅ | ✅（P-01 已覆盖） |

> **注意**：H5 注册保留 `PostRegister` 直接建户的现状（H5 无微信支付会员费强制流程）。两阶段会话制仅 weapp 端（见 `docs/cases/account-select.md` P-05）。

## 与 Onboarding 页面的关系

注册页完成后：
1. `POST /api/auth/register` 返回 token → 用户已登录
2. 注册页本身收集了 Onboarding 所需的全部字段（昵称、地址、身份证）→ Onboarding 对该用户无意义
3. **Onboarding 页面已废弃**：注册用户标记 `onboarding_completed = true`；OAuth 登录回调默认跳首页（不再跳 `/onboarding`）；前端 `/onboarding` 路由与 Onboarding.jsx 已移除（字段收集由注册页 + 编辑资料页承担）
4. 后端 `GET/PUT /user/onboarding` 端点保留（兼容存量数据，无前端调用方）

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

## 两阶段注册（weapp，#1663）

> 仅 weapp 端（微信小程序）采用两阶段会话制，H5 保留 PostRegister 直接建户。详见 `docs/cases/account-select.md` P-05。

**设计原则**：支付完成前不能使用账户；支付完成后无论何种情况留痕可追溯。

### 数据模型
- `registration_sessions`：会话表（openid / exchange_token / form_data JSONB / coupon_code / amount / status: pending→paid→completed / error / completed_at）
- `coupons`：优惠码表（code / type: waive|percent / value / active）

### 后端接口
| 接口 | 说明 |
|------|------|
| `POST /auth/registration-sessions` | 写注册会话（表单 + exchange_token）→ 返回 session_id + amount（不建户） |
| `GET /auth/registration-sessions/me` | 按 openid 查 pending 会话（「继续完成注册」恢复） |
| `GET /auth/registration-sessions/:id/status` | 查询会话状态（前端轮询） |
| `POST /pay/prepay`（membership 扩展） | 传 session_id + coupon_code → 后端计算金额 |

### 支付回调建户（服务端权威）
微信支付回调 → 查会话 → 标记 paid → 服务端完成注册（CreateUser + 绑定 openid + 本地同步 + 赠点 + 推荐奖励）→ 标记 completed。幂等：completed 只执行一次建户。

### 优惠码（#1719 通用化）

> **金额表示法（#1728）**：API/存储一律分（int64，1 元 = 100 分）；前端显示 ÷100。
- `OREZ`：waive 全额免除（¥0，直接完成注册，仍写 payment record 留痕）
- `ENO`：percent 1%
- 优惠码**所有支付类型通用**（membership/rent/repair/renewal/damage），金额后端计算，前端只传 code；为长期功能（非测试临时码）

---

*Last updated: 2026-08-14*
