# 微信小程序登录架构方案

> 2026-07-08
> 关联：Phase 2 微信原生能力

## 一、背景

H5 版使用 IAM OAuth 重定向流程：跳转到 IAM 登录页 → 用户输入账号密码 → IAM 回调 tuneloop → 换 JWT。此流程依赖浏览器 URL 重定向，**在微信小程序中不可行**。

## 二、H5 vs 小程序登录方式对比

| | H5 | 小程序 |
|------|-----|--------|
| 主通道 | IAM OAuth 重定向 | **微信一键登录** |
| 测试通道 | IAM OAuth 重定向 | **账号密码 → tuneloop → IAM** |
| 游客 | 跳转 IAM 后匿名 | **静默 wx.login（role=GUEST）** |

| 能力 | H5 | 小程序 |
|------|:--:|:------:|
| 外部 URL 重定向 | ✅ | ❌ |
| 外部页面嵌入 | ✅ WebView | ❌ |
| 微信 OAuth (`wx.login`) | ❌ | ✅ |
| 一键授权获手机号 | ❌ | ✅ `getPhoneNumber` |
| 多角色切换（测试） | ✅ IAM 直接重定向 | ✅ IAM 中转登录 |
| 记住 session | ✅ Cookie/localStorage | ✅ Storage |
| 域名白名单限制 | 无 | mp.weixin.qq.com 登记 |

## 三、推荐方案：三通道登录

### 3.0 登录页 UI

```
┌──────────────────────────────┐
│           登录               │
│                              │
│   📱 微信用户一键登录        │  ← 生产用户主通道
│                              │
│   ──── 其他方式 ────         │
│                              │
│   📧 邮箱/手机号登录         │  ← IAM 账号密码（测试/多角色切换）
│      邮箱/手机号             │     tuneloop backend 中转调用 IAM
│      密码                    │
│      [登录]                  │
│                              │
│   👀 随便看看                │  ← 匿名游客
│                              │
│  ───────────────────────     │
│  版本 v1.0.0                │  ← 连续点击 5 次进入开发者模式
└──────────────────────────────┘
```

**开发者模式**（版本号连点 5 次）额外暴露：切换账户、清空 token、查看日志等调试入口。

### 3.1 通道一：微信一键登录（getPhoneNumber）

小程序启动时**立即静默**调用 `wx.login()`，全程无用户感知。

```
小程序启动
  → wx.login() → 临时 code
  → POST /api/auth/wx-login { code }      ← 无 encryptedData
  → 后端用 code 换微信 session_key + openid
  → 创建匿名用户或绑定已有 openid
  → 签发 JWT（role=GUEST 或 USER if already bound）
```

### 3.2 通道二：IAM 账号密码登录（测试/多角色切换）

测试阶段需频繁切换 student/admin/worker 等多角色账号，微信绑定无法满足。保留 IAM 账号密码登录，但**不走前端直连 IAM**（小程序域名白名单只登记 `wx.cadenzayueqi.com`），改为 tuneloop backend 中转：

```
用户输入邮箱/手机号 + 密码
  → POST /api/auth/login { identifier, password }
  → tuneloop backend
      → 调用 beaconiam POST /api/v1/auth/login
      → 拿到 JWT → 返回前端
  → 存 storage，跳回目标页
```

**使用场景**：
- 开发阶段：QA/开发者测试不同角色（student ⇄ admin ⇄ worker）
- 生产阶段：员工使用内部账号登录（非微信绑定）
- 过渡阶段：用户已有 IAM 账号但尚未绑定微信

### 3.3 通道三：静默游客模式（wx.login）

与 3.1 共用 `wx.login()`，但不弹授权——静默获取 code，后端交换 session_key + openid。

```
小程序启动
  → wx.login() → 临时 code
  → POST /api/auth/wx-login { code }      ← 无 encryptedData
  → 后端用 code 换微信 session_key + openid
  → 查找/创建匿名用户 → 签发 JWT（role=GUEST）
```

**目标**：
- 所有用户（含游客）都有 token，API 层不再区分"有 token/无 token"
- 游客 role=GUEST，服务端做细粒度权限（可浏览、不可下单/不可查看个人数据）

## 四、后端变更需求

### 4.1 新增端点

**POST `/api/auth/wx-login`** — 微信一键登录 + 静默游客

```json
// Request
{
  "code": "临时凭证（来自 wx.login()）",
  "encryptedData": "加密数据（getPhoneNumber 回调带）",
  "iv": "初始化向量"
}

// Response (静默登录，无 encryptedData)
{
  "code": 20000,
  "data": {
    "token": "jwt_token",
    "user": {
      "id": "...",
      "role": "GUEST"
    }
  }
}

// Response (授权登录，有 encryptedData)
{
  "code": 20000,
  "data": {
    "token": "jwt_token",
    "user": {
      "id": "...",
      "name": "微信用户",
      "phone": "138xxxx",
      "role": "USER"
    }
  }
}
```

### 4.2 端点内部流程（wx-login）

```
POST /api/auth/wx-login
  1. code → 调微信服务器 POST https://api.weixin.qq.com/sns/jssession
      → 返回 session_key + openid
  2. 若 encryptedData 存在：
     a. AES-128-CBC 解密 encryptedData → 获取 phone + openid
     b. 在 users 表查找 openid
        - 存在 → 更新 session_key，返回
        - 不存在 → 创建用户（role=USER），保存 openid + phone
     c. 调 beaconiam BindUserToOrganization（如果适用）
     d. 签发正式 JWT（含 role, oid, tid）
  3. 若 encryptedData 不存在（静默登录）：
     a. 在 users 表查找 openid
        - 存在 → 返回正式 JWT
        - 不存在 → 创建匿名用户（role=GUEST），签发 GUEST JWT
     b. JWT 中 role=GUEST，oid/tid 为空
  4. 返回 { token, user }
```

### 4.3 IAM 中转登录

**POST `/api/auth/login`** — 账号密码登录（tuneloop backend 中转调用 IAM）

```json
// Request
{
  "identifier": "email@example.com 或 手机号",
  "password": "密码"
}

// Response
{
  "code": 20000,
  "data": {
    "token": "jwt_token（beaconiam 签发）",
    "user": { "id": "...", "name": "...", "role": "STAFF" }
  }
}
```

```
请求 → tuneloop backend
  → POST 到 BEACONIAM_INTERNAL_URL/api/v1/auth/login { identifier, password }
  → beaconiam 返回 JWT → tuneloop 透传 → 前端存 storage
```

### 4.4 依赖项

| 依赖 | 说明 | 状态 |
|------|------|:----:|
| 微信小程序 AppID + AppSecret | 在微信公众平台获取 | ❌ 需运营配置 |
| 微信服务器 API | `api.weixin.qq.com/sns/jssession` | ✅ 微信提供 |
| beaconiam 用户绑定 | `BindUserToOrganization` | ✅ 已有 |
| `.env` 新增 | `WX_APPID`, `WX_APPSECRET` | ❌ 待添加 |

## 五、前端变更

### 5.1 页面：登录页（`pages-weapp/login/`）

- 三按钮：微信一键登录 / 账号密码登录 / 随便看看
- 版本号连续点击 5 次 → 开发者模式（切换账户、清 token、看日志）
- 纯 inline style（跟随迁移规范）
- 注册到 `app.config.ts` weappPages

### 5.2 Platform 层补充

`src/platform/index.weapp.js` 新增函数：

```js
export const wxLogin = () => new Promise((resolve, reject) => {
  Taro.login({
    success: (res) => resolve(res.code),
    fail: (err) => reject(err),
  })
})

export const getPhoneNumber = (e) => {
  const { encryptedData, iv } = e.detail
  return { encryptedData, iv }
}
```

### 5.3 导航守卫

游客访问"我的"→ 检查 JWT role：
- `GUEST` → 展示登录页
- `USER`/`STAFF` → 正常显示 Profile

## 六、角色 × 权限矩阵（小程序版）

| 角色 | 浏览 | 下单 | "我的" | 报修 | 多角色切换 |
|------|:---:|:---:|:-----:|:---:|:---------:|
| GUEST（未登录） | ✅ | ❌ | ❌→登录页 | ❌ | — |
| USER（已绑定） | ✅ | ✅ | ✅ | ✅ | — |
| STAFF | ✅ | ✅ | ✅ | ✅ | ✅ IAM 登录 |

> **多角色切换**：STAFF 通过开发者模式或 IAM 账号密码登录切换到其他测试账号。

## 七、注册流程

### 7.1 微信一键登录 → 自动注册

微信一键登录**无需独立注册页面**。首次 `getPhoneNumber` 授权时，后端自动完成注册：

```
新用户首次点击"微信一键登录"
  → getPhoneNumber 返回 encryptedData + iv
  → POST /api/auth/wx-login { code, encryptedData, iv }
  → 后端解密 → 手机号、openid
  → 查 openid + phone → 未找到
  → 创建新用户：
      - role = USER
      - name = "微信用户" + phone 后 4 位  ← 临时占位
      - phone = 解密后的手机号
      - openid = 微信 openid
  → 签发 JWT
  → 返回 { token, user, is_new: true }
```

**返回 `is_new: true` 时**，前端引导进入"完善资料"页：

```
┌──────────────────────────┐
│      完善个人资料         │
│                          │
│   姓名: [________]       │
│   邮箱: [________]       │
│                          │
│   [跳过]  [保存]         │
│                          │
│   ⚠️ 租赁需要真实姓名     │
└──────────────────────────┘
```

- 用户可跳过 → 保留默认名称，后续下单时提示补全
- 用户保存 → `PUT /api/users/me { name, email }`

### 7.2 IAM 注册（备用通道）

账号密码登录的用户如需注册新账号，通过后端中转 IAM：

```
POST /api/auth/register
→ tuneloop backend → beaconiam POST /api/v1/auth/register
→ 创建 IAM 用户 + tuneloop 本地同步
→ 返回 JWT
```

注册页可在登录页中通过 "没有账号？注册" 链接切入。

### 7.3 完整新用户旅程

```
游客打开小程序
  → 静默 wx.login → role=GUEST
  → 浏览首页、详情，观看乐器
  → 想租赁 → 点"下单" → 检测到 GUEST
  → 弹登录页
  → 选择"微信一键登录"
  → 微信弹窗授权
  → 后端自动创建账号（is_new: true）
  → 弹出"完善资料"页
  → 用户填写姓名/邮箱（可跳过）
  → role=USER → 可以下单
```

### 7.4 注册相关的后端变更

**POST `/api/auth/wx-login` 响应扩展**：

```json
{
  "code": 20000,
  "data": {
    "token": "jwt_token",
    "user": { "id": "...", "name": "...", "role": "USER" },
    "is_new": true
  }
}
```

**POST `/api/auth/register`**（新增，IAM 中转）：

```json
// Request
{
  "name": "用户名",
  "phone": "手机号",
  "email": "email@example.com",
  "password": "密码"
}

// Response（透传 beaconiam）
{
  "code": 20000,
  "data": {
    "token": "jwt_token",
    "user": { "id": "...", "name": "...", "role": "USER" }
  }
}
```

### 7.5 身份绑定与合并

用户可能通过多个渠道创建账号（微信 openid A、微信 openid B、邮箱 C），需支持身份合并：

| 场景 | 处理 |
|------|------|
| 同一手机号，不同 openid | 合并 openid → 同一用户 |
| 微信登录后，又用 IAM 登录 | 如果手机号匹配 → 更新 openid 绑定 |
| 已通过按钮注册，无 openid | 暂不绑定，后续并入 |

---

## 八、注意事项

1. **wx.login code 有效期 5 分钟**，后端需及时调用微信服务器换取 session
2. **session_key 不应传到前端**，仅后端持有，用于解密 encryptedData
3. **静默登录在前端无感**，`app.tsx` `componentDidMount` 中即可调用
4. **JWT 过期处理**：`role=GUEST` 的 JWT 可设置较长过期时间（如 30 天），减少静默刷新频率
5. **调试**：微信开发者工具中 `wx.login` 返回的 code 可用于测试，但 `getPhoneNumber` 需真机
6. **`is_new` 标记**：首次微信授权后返回 `is_new: true`，前端据此引导完善资料，不强制阻塞
7. **身份合并**：同一手机号出现于多个 openid 时，后端应在 `wx-login` 中检测并合并，避免重复账户

---

*参考: `docs/wechat_transform.md`（迁移总计划）、`docs/weapp.md`（现有 weapp 架构）*
