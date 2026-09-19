# Tuneloop 侧对 IAM 的补充说明和过渡期记录

> IAM 权威文档请直接阅读 `docs/iam.md`（symlink → `../../beaconiam/README.md`）。

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

## 微信小程序登录流程

> 完整架构说明见 `docs/weapp.md`。

```
wx.login() → code → POST /api/wx/login → BeaconIAM
                                              ↓
                                  jscode2session → openid
                                              ↓
                                   查 users.wx_openid
                                  /               \
                              不存在             存在
                                ↓                 ↓
                          创建用户(USER)        返回 JWT
                          随机名 wx_xxxx
                                ↓
                           返回 JWT
```

**关键点**:
- Tuneloop 仅做代理转发，不直接处理 wx code
- IAM `users` 表新增 `wx_openid` 字段，唯一索引（NULL 排除）
- 首次登录自动创建 `USER` 角色用户，随机名 `wx_{8chars}`
- 下单时检测信息完整性，缺 phone/email 则跳转注册补全页
- 详情见 `docs/weapp.md`

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
