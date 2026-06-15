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
