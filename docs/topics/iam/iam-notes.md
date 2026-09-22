# Tuneloop 侧对 IAM 的补充说明和过渡期记录

> IAM 权威文档请直接阅读 `docs/topics/iam/iam.md`（symlink → `../../beaconiam/README.md`）。

## 已知文档差异

### JWT Claims

| 字段 | IAM 文档中写为 | 实际 JWT 中的 Key | 说明 |
|------|-------------|-----------------|------|
| 组织 ID | `gid` (❌ 不存在) | `oid` | beaconiam#315 已报修 |

Tuneloop 的 `IAMClaims` 结构体中同时有 `Oid` 和 `Gid`。`Gid` 在 IAM JWT 中不存在，应废弃。

### UpdateUserRoleInOrg 参数格式

- beaconiam #313 计划将端点改为接受 JSON body `{"role": "ADMIN"}`
- 当前 tuneloop 发送 query param `?role=ADMIN` 作为过渡
- 等 beaconiam 部署 JSON body 支持后，tuneloop 需切回 JSON body

## 身份模型：一人一记录 + 顾客角色 + 组织上下文（#2025 / #2026 / #2027 B1）

> **权威口径**：本节为 S0（#2026）裁定后的身份模型基线，S1–S3（#2027/#2028/#2029）向此看齐。
> IAM 侧权威文档（`beaconiam/README.md`）的同步由 S1 在 beaconiam 仓库完成（`docs/topics/iam/iam.md` 为 symlink，禁止本地改写）。

### 六原则

1. **一人一记录**：一个自然人 = 一条 `users` 记录（不再为同一人建多户）
2. **微信号 ↔ 用户 = 一对多（绑定表）**：`wx_user_bindings`（openid, user_id）为**唯一权威来源**；`users.wx_openid` 已废弃删除（#2019）。同一 openid 可绑定多个历史账户（兼容期），新流程按一人一记录收敛
3. **多身份**：员工/商户管理员/维修师傅 = 「用户 ↔ 组织」的 `user_org_relations`；**顾客 = 根组织（namespace primary org，即顶层组织）member 关系上的 `customer` 职能角色**（B1，**非独立组织**）。同一用户可同时具备
4. **顾客身份 = 零权限职能角色（B1,#2027）**：`customer` 职能角色附着在用户「**根组织**（namespace primary org）member 关系」的 `functional_roles` 上；bootstrap 为每个 namespace 播种 `customer` 角色模板（零权限）；**不建独立顾客组织**（根组织全员 member，不能当顾客组织）
5. **组织上下文切换**：微信登录返回 `contexts[]{org_id, org_name, label}`；选择某上下文 → 按其 relation 签发 JWT；「切换账户」= 切换组织上下文
6. **删除 = 解除关联/标记删除**：网点删 relation、商户级联、系统管理员标记删除（记录保留、可重新加回）

### 四个裁定（用户已批准）

| # | 裁定 | 内容 |
|---|------|------|
| **D1** | 顾客上下文 JWT **tid 保持空** | 顾客身份（`customer` 角色）仅用于身份归属/切换展示，**不改变数据隔离模型**（#688/#833/#1579 地基不动）；顾客仍以 `oid`/`tid` 空、从 instrument/order 反推租户的方式工作，`/api/user*` 保持 userOptionalAuth |
| **D2** | 手机号/邮箱已有账户时 → **唯一性硬校验，禁止管理员代挂**（一人一号） | **不静默复用、不允许管理员从冲突清单挑既有用户挂接**（防误绑）。返回 `40902`「该手机号/邮箱已在本平台注册，请让本人登录后自助加入（作为员工加入 / 注册为顾客）」。**身份追加仅限本人自助**（微信/手机登录即身份凭证）。当前无 SMS provider，不做验证码确认 |
| **B1** | 顾客身份用**角色**而非组织 | `customer` 职能角色（零权限）附着在既有**根组织** `member` 关系上。因载体关系是 `member`（`wxAccountPayload` 显式跳过）→ **D1 自动满足、隔离零改动**。授予：会员注册 / 「注册为顾客」；**管理员建户不给**。撤销 = 移除角色（D4） |
| **D3** | 被标记删除用户再次微信登录 → **重新激活原户** | 保留业务数据归属，避免归属再分裂（不新建户） |
| **D4** | 删除 = **标记删除 + 解除关联** | relation 删除（网点）/商户级联/系统管理员标记删除；用户记录保留、可重新加回 |
| （衔接） | 解绑微信 → **删除绑定行**（#2020） | `wx_user_bindings` 为权威源，解绑必须落在此表 |

### 关键约定

- **切换账户页展示**：`组织名 + 角色标签`（顾客 / 海淀店员工），顶部 greeting「欢迎 {name}」；不再展示「账户昵称」语义（一人一记录）
- **顾客标签**：持有 `customer` 角色者显示「顾客」；组织上下文显示 `{org_name} + {角色标签}`（site_admin/member→员工，merchant_admin→商户管理员，repair_technician→维修师傅）
- **兼容期**：存量多户（同一 openid 多 user）在 #2029 合并前保持双形态可用；`wx-accounts` 返回旧形态时前端需兼容

## 微信小程序登录流程

> 完整架构说明见 `docs/topics/wechat/weapp.md`（#2016 起以 `wx_user_bindings` 为绑定权威源）。

```
wx.login() → code → POST /api/auth/wx-accounts → BeaconIAM
                                              ↓
                                  jscode2session → openid
                                              ↓
                            查 wx_user_bindings WHERE openid
                                  /               \
                              无绑定           有 1..N 绑定
                                ↓                 ↓
                        查注册会话/引导注册    返回 contexts[]
                                              （组织名+标签）
                                                    ↓
                                        选择上下文 → wx-login-select
                                        → 按 relation 签发 JWT
```

**关键点**:
- Tuneloop 仅做代理转发，不直接处理 wx code
- **绑定关系唯一权威源 = `wx_user_bindings`**（`users.wx_openid` 已废弃，#2019 删除本地列）
- 一人一记录：新流程不再为同一 openid 新建第二个用户
- 顾客上下文 `tid` 保持空（D1）
- 下单时检测信息完整性，缺 phone/email 则跳转注册补全页
- 详情见 `docs/topics/wechat/weapp.md`

## 已向 IAM 组提交的 Issue

| Issue | 内容 |
|-------|------|
| [beaconiam#313](https://github.com/HiJohns/beaconiam/issues/313) | UpdateUserRoleInOrg 改为 JSON body |
| [beaconiam#315](https://github.com/HiJohns/beaconiam/issues/315) | JWT Claims 文档修正（oid/gid）|
| [beaconiam#324](https://github.com/HiJohns/beaconiam/issues/324) | CreateUser/CreateOrg 支持 skip_activation 参数 |
| [beaconiam#325](https://github.com/HiJohns/beaconiam/issues/325) | CreateOrg 返回 initial_password |
| [beaconiam#366](https://github.com/HiJohns/beaconiam/issues/366) | 微信小程序登录: wx-login 端点 + users 表 openid 字段 |

## skip_activation 功能说明

Tuneloop `POST /api/merchants` 新增 `skip_activation` 参数（tuneloop #730）。

### 调用路径

**Path A — skip_activation=true**：
```
CreateMerchant → CreateUser(SkipActivation=true, password)  ← CreateOrg(SkipActivation=true)
  → IAM 创建 active 用户                                        → IAM 创建 org + active admin
  → BindUser 立即执行                                            → BindUser 立即执行
  → SetUserCustomerPermissions 立即执行                          → SetUserCustomerPermissions 立即执行
  → AssignRoleTemplate 立即执行                                  → AssignRoleTemplate 立即执行
```
依赖 beaconiam #324（skip_activation API）和 #325（CreateOrg 返回 initial_password）。

**Path B — skip_activation=false**（现有流程）：
```
CreateMerchant → CreateUser(CallbackURL)  ← CreateOrg(CallbackURL)
  → IAM 创建 pending 用户                                       → IAM 创建 org + pending admin
  → BindUser 入队                                                → BindUser 入队
  → SetUserCustomerPermissions 入队（依赖 #323）                → SetUserCustomerPermissions 入队（依赖 #323）
  → 用户确认邮箱 → AcceptTasks 统一执行                          → 用户确认邮箱 → AcceptTasks 统一执行
```
依赖 beaconiam #323（SetUserCustomerPermissions 入队支持）。

## JWT 主体校验与会话吊销（#1735，配合 beaconiam#487）

> 2026-08-21 起，tuneloop 在 JWT 签名校验通过后增加一层**主体有效性校验**，以 beaconiam 为权威（本地 `users` 仅是缓存）。解决「删号/禁用/改密后旧 token 静默回落路人」问题。

### 校验规则（`backend/middleware/iam.go: enforceSubjectValidity`）

按优先级，签名校验通过后执行（GUEST token 跳过）：

| 条件 | 响应 | 语义 |
|------|------|------|
| IAM 查不到用户 | `40105 account_not_found` | 账户已删除 |
| `status != "active"` | `40106 account_inactive` | 账户已禁用 |
| `iat(秒) < token_version/1000` | `40107 token_revoked` | 改密码/禁用后签发的旧 token 已吊销 |

- **数据来源**：`IAMClient.GetUserAuthState(userID)` → beaconiam `GET /api/v1/users/:id` 的 `token_version` + `status` 字段（beaconiam#487 提供），进程内 TTL 缓存 30s（tuneloop 无 Redis）。
- **秒粒度比较**：JWT `iat` 是秒级 NumericDate、`token_version` 是毫秒——统一折算到秒比较，避免同秒内签发的合法 token 被误杀。
- **fail-open**：beaconiam 网络/5xx 故障时放行（日志告警），避免抖动导致全员登出；下个请求重试。

### 前端清 token 语义

小程序/H5/PC 收到 `40105/40106/40107` → **跳过静默 refresh** → 清除本地凭证 → 引导重新登录。禁止用 refresh 救活已吊销会话；网络错误保持登录态可重试。

### beaconiam 侧配套（#487 / #488）

- `users.token_version`（UnixMilli）：改密码 / 禁用 / 激活时 bump（`BumpTokenVersion()`）
- refresh 端点拒绝非 active 用户
- **refresh token 吊销校验（#488，已修复）**：refresh grant 同样校验 refresh token 自身的 `iat(秒) < token_version/1000` → 401 `token revoked`——改密后旧 refresh token 不再能换新 token，吊销机制在协议层闭环。秒级对称比较与 tuneloop 侧 40107 判定完全一致；`token_version=0`（从未 bump 的 legacy 用户）跳过校验。
  - 部署状态：beaconiam main `82566fb`（待随 #487 一并部署预生产后实测，验证清单见 tuneloop#1736）
