# H5 ↔ weapp 登录/注册/绑定 统一设计（草案）

> 状态：**草案（待评审）**｜关联：#2172（H5↔weapp 一致性）、#1638（注册 username 来源）、beaconiam（IAM 身份源）
> 目的：明确 H5（手机浏览器）与微信小程序在**登录/注册/微信绑定**上的差异、目标设计、可行性与待决策项。

## 1. 术语

| 端 | 载体 | 代码库 |
|----|------|--------|
| H5 | `wx.cadenzayueqi.com`（手机浏览器，非微信内置） | `frontend-mobile`（Vite H5） |
| weapp | 微信小程序（`wxcb44a1be70e356ed`） | `frontend-mobile`（Taro weapp） |
| IAM | `iam.cadenzayueqi.com` | beaconiam |

## 2. 现状（能力矩阵）

| 能力 | H5 | weapp | 后端端点 |
|------|:--:|:-----:|----------|
| 微信一键登录 | ❌ | ✅ `wx.login` | `POST /auth/wx-login` |
| 账号密码登录 | ❌（现走 **IAM OAuth 重定向**） | ✅ (`pages-weapp/login/Login.jsx`) | `POST /auth/login {identifier, password}` |
| 邮箱/密码注册 | 🟡 `Register.jsx`（**强制手机号**，见下） | ✅（微信注册无需密码） | `POST /auth/register`（**要求 phone，username=phone 派生，见 #1638**） |
| 微信绑定（把微信绑到已有账号） | ❌ | ❌（UI 未暴露） | `POST /users/me/wx-bind`（**已存在**） |
| 微信扫码登录 | ❌ | — | 无（需开放平台「网站应用」） |
| 游客态 | ✅（无 token） | ✅（静默 wx.login → GUEST） | — |

**关键约束**（`docs/topics/wechat/wechat-login.md`）：H5 无 `wx.login`（拿不到微信身份）；小程序无法做 URL 重定向；故二者主通道天然不同。

## 3. 目标设计（用户裁定）

1. **主因**：H5 拿不到微信号 → H5 需自带**用户名/邮箱/密码**注册与登录。
2. **H5 注册页**：指定**用户名、邮箱、密码**。
3. **独立账号密码登录页**：未登录时 H5 个人中心显示「**登录**」按钮 → 进入登录页 → 输入**用户名或邮箱 + 密码**。
4. **weapp 绑定微信**：对未绑定微信的用户，在「个人中心 → 编辑资料」显示「**绑定微信**」按钮（`POST /users/me/wx-bind`）。
5. **登录二维码**：账号密码登录页可显示二维码，用手机微信扫码登录（**需调研，见 §4**）。

## 4. 微信扫码登录（调研）

### 4.1 机制
微信**开放平台** → 创建「**网站应用**」→ 得 appid/secret（**独立于**小程序、公众号）→
`https://open.weixin.qq.com/connect/qrconnect?appid=<网站应用APPID>&redirect_uri=<回调>&response_type=code&scope=snsapi_login&state=<随机>` → 手机微信扫码授权 → 回调 `?code=&state=` → 后端用 `code` 换 `openid`（网站应用维度）→ 登录或绑定。

### 4.2 前提（决定可行性）
- 需**微信开放平台账号** + **网站应用**（企业认证通常必要）；回调域名白名单。
- **打通两端的前提 = `unionid`**：网站应用与小程序须绑定**同一开放平台账号**，同一用户 unionid 一致 → 可将「网站应用 openid」与「小程序 openid」归一到同一 tuneloop 账号。
- 现状：仓库仅见**小程序 appid**，**未发现开放平台/公众号/网站应用** → 需确认/申请。

### 4.3 手机浏览器悖论（重要）
扫码登录是**桌面范式**（手机扫电脑屏）。H5 在**手机浏览器**里"本机扫本机"不可行 → 二维码登录**只对「桌面浏览器打开 H5 URL」有意义**；手机端登录应走「用户名密码」。

### 4.4 后续流程（二选一）
- **(a) 同页轮询（推荐）**：前端带随机 `state` 生成二维码 → 本页轮询后端「该 state 是否已扫码并登录」→ 后端在微信回调中写 `state→结果` → 前端取到后置登录态（二维码嵌页内、不跳转）。
- **(b) 回调重定向**：微信回调后端 → 后端 302 回前端并带一次性 token（需跳转/新窗口）。

### 4.5 登录 or 绑定
扫码得 openid/unionid：命中已有账号 → 登录；未命中 → **引导绑定**到现有账号（或注册）——与 §3.4 绑定体系共用。

## 5. 待决策开放问题（需产品/资质确认）

| # | 问题 | 影响 |
|---|------|------|
| Q1 | **是否回退 #1638**（H5 注册允许 `username`、弱化 `phone`）？ | 决定 §3.2 能否实现（现状 register 强制 phone 且 username=phone） |
| Q2 | **是否已有微信开放平台账号 + 网站应用**？小程序是否在其下（有 unionid）？ | 决定 §4 扫码登录**能否做** |
| Q3 | 扫码登录目标场景是否**桌面浏览器**？ | 决定 §4 是否值得做 |
| Q4 | H5 登录：**新建原生登录页**（用户名密码）**替换**现有 IAM OAuth 重定向，还是**并存**？ | 决定 H5 登录改造范围 |

## 6. 涉及端点/文件（实现时的落点，非本期改动）

- 后端：`POST /auth/login`、`POST /auth/register`、`POST /auth/wx-login`、`POST /users/me/wx-bind`（已存在）；扫码登录需**新增**（qrconnect 生成 + 回调 + state 轮询）。
- H5：`pages/Register.jsx`（username 支持）、**新建** `pages/Login.jsx`（账号密码，+可选二维码）、`pages/Profile.jsx`（未登录「登录」入口）。
- weapp：`pages-weapp/Profile.jsx` + 编辑资料（未绑微信→「绑定微信」按钮）。
- 文档：本文件、`docs/spec/api/api.md`（登录/注册/绑定契约）、`docs/cases/h5-weapp-differences.md`（登录通道差异条目）。

## 7. 建议推进顺序（待 Q1–Q4 答复后拆 Issue）

1. **H5 账号密码登录页 + Profile「登录」入口**（后端 `/auth/login` 已就绪）——不依赖新资质，最先可做。
2. **weapp「绑定微信」按钮**（后端 `/users/me/wx-bind` 已就绪）。
3. **H5 注册 username 支持**（依赖 Q1 / #1638 回退）。
4. **微信扫码登录**（依赖 Q2 资质 + unionid；`state` 轮询选型）。

---
_本文件为设计草案，不含实现。_
