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

## 已向 IAM 组提交的 Issue

| Issue | 内容 |
|-------|------|
| [beaconiam#313](https://github.com/HiJohns/beaconiam/issues/313) | UpdateUserRoleInOrg 改为 JSON body |
| [beaconiam#315](https://github.com/HiJohns/beaconiam/issues/315) | JWT Claims 文档修正（oid/gid）|
| [beaconiam#324](https://github.com/HiJohns/beaconiam/issues/324) | CreateUser/CreateOrg 支持 skip_activation 参数 |
| [beaconiam#325](https://github.com/HiJohns/beaconiam/issues/325) | CreateOrg 返回 initial_password |

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
