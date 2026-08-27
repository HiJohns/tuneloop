# 微信小程序架构与部署

> 父 Issue: #872 — frontend-mobile 小程序化迁移 (Taro)

## 技术选型

| 决策 | 选择 | 原因 |
|------|------|------|
| 框架 | **Taro** | React 一等公民、Tailwind 官方方案、同时输出 H5 + 小程序 |
| 构建策略 | **Taro 统一构建** | 一套路由、一套 platform 层、无 `TARO_ENV` 分支，废弃独立 Vite |
| 登录 | 小程序: `wx.login()` → openid; H5: OAuth | 双通道，后端代理到 IAM |

## 构建命令

```bash
npm run dev:h5      # Taro H5 dev server (开发期逻辑验证 + H5 调试)
npm run dev:weapp   # Taro watch 小程序 (微信开发者工具导入 dist/)
npm run build:h5    # 生产 H5 构建
npm run build:weapp # 生产小程序构建
```

## 三端架构

| 端 | 入口 | 前端部署位置 | API 地址 | 登录方式 |
|----|------|-------------|---------|---------|
| 小程序 | 微信内搜索/扫码/分享 | 微信服务器 | `https://wx.cadenzayueqi.com/api` | `wx.login()` 无密登录 |
| H5 | `https://wx.cadenzayueqi.com` | 生产服 NGINX | 同域 `/api` 代理到后端 | OAuth |
| PC | `https://web.cadenzayueqi.com` | 生产服 NGINX | 同域 `/api` 代理到后端 | OAuth |

- 小程序和 H5 共用同一后端 API 和同一域名 `wx.cadenzayueqi.com`
- 区别仅在登录通道和前端部署位置
- PC 端独立域名和部署，不受小程序迁移影响

## 小程序编译与发布流程

### 开发期

```
npm run dev:weapp
        ↓
dist/ 目录生成小程序代码
        ↓
微信开发者工具 → 导入项目 → 选择 dist/ 目录
        ↓
预览 / 真机调试
```

> 微信开发者工具 → 详情 → 本地设置 → 勾选「不校验合法域名」以支持开发环境 HTTP

### 发布上线

```
taro build --type weapp → dist/
        ↓
微信开发者工具 → 导入 dist/ → 预览确认
        ↓
点击「上传」→ 填版本号 + 备注
        ↓
代码上传到微信服务器 (不在自己的服务器)
        ↓
mp.weixin.qq.com → 版本管理 → 提交审核 (1-7 天)
        ↓
审核通过 → 发布上线
```

### 版本管理规范（单 appid 策略，#1694）

**单一小程序（wxcb44a1be70e356ed）**：不再有独立的预生产小程序。服务器端生产/预生产保持完全分离（数据库/配置独立）；小程序前端通过**编译期注入的 apiBaseUrl** 区分：
- **开发版**（测试）：`make weapp-build-pre` 构建（apiBaseUrl=prewx.cadenzayueqi.com）→ `make weapp-upload-dev VERSION=<归档> APP_VERSION=1.0.x-dev` 上传为开发版 → 微信后台设为**体验版**，二维码分发给测试人员（关联 prewx 后端，数据隔离）
- **发布版**（正式）：`make weapp-build-prod` 构建（apiBaseUrl=wx.cadenzayueqi.com）→ `make weapp-upload-prod VERSION=<归档> APP_VERSION=1.0.x` 上传为正式版 → 提交审核 → 通过后发布（关联 wx 生产后端）

**微信版本体系**：开发版（版本号自定义，如 1.0.x-dev）→ 体验版（从开发版选择）→ 审核版（提交审核的版本）→ 线上版（发布后全量可见）。审核版本号**必须高于线上版本**。

### 发布版上传动作清单（每次发布必须依次执行）

```bash
# 1. 确认版本号：1.0.<x+1>（高于线上 + 审核中版本），语义化 major.minor.patch
# 2. （可选）git tag v1.0.x
# 3. 构建正式版（wx apiBaseUrl）
make weapp-build-prod VERSION=<归档名>     # 归档名自动 = YYYYMMDD-HHMMSS_COMMITID
# 4. 上传正式版（指定微信版本号 APP_VERSION=1.0.x）
make weapp-upload-prod VERSION=<归档名> APP_VERSION=1.0.x DESC="release note"
# 5. 微信公众平台 → 版本管理 → 提交审核（审核版本号 = 1.0.x）
# 6. 审核通过 → 发布
# 7. 发布后测试：切换回开发版（prewx）体验版继续日常测试
```

**开发版上传**：
```bash
make weapp-build-pre VERSION=<归档名>      # prewx apiBaseUrl
make weapp-upload-dev VERSION=<归档名> APP_VERSION=1.0.x-dev DESC="dev build"
# 微信公众平台 → 版本管理 → 设体验版 → 二维码分发
```

- 上传只消费归档（不重新编译）——保证发布内容 = 已验证内容
- 回退 = 上传更早归档（体验版切换或重新提交审核）

## H5 部署

```
taro build --type h5 → dist/ 静态文件
        ↓
部署到生产服 NGINX
        ↓
wx.cadenzayueqi.com 提供服务
```

- NGINX 配置 `/api` 代理到 Go 后端
- HTTPS 必须（小程序和现代浏览器要求）

## 登录流程

### 小程序通道

```
wx.login() → code
        ↓
POST /api/wx/login { code }
        ↓
Tuneloop 后端代理 → BeaconIAM POST /api/v1/auth/wx-login
        ↓
IAM 调用微信 jscode2session → 获取 openid
        ↓
查 users 表: 存在 → 返回 JWT; 不存在 → 创建用户(USER 角色) → 返回 JWT
        ↓
前端存储 token，后续请求携带 Authorization header
```

### H5 / PC 通道 (不变)

```
浏览器 → 重定向到 IAM OAuth 授权页
        ↓
用户登录/注册 → IAM 回调 /callback?code=xxx
        ↓
Tuneloop 后端用 code 换取 JWT
        ↓
前端存储 token
```

### 手机号授权 (小程序)

```
用户点击 <Button openType="getPhoneNumber">
        ↓
微信返回加密数据 (encrypted_data + iv)
        ↓
POST /api/wx/phone { encrypted_data, iv }
        ↓
Tuneloop 后端代理 → IAM 解密 → 更新用户 phone
        ↓
消除影子账号状态
```

### 已有用户绑定

用户先在 H5 通过 OAuth 登录，后开小程序 → openid 无关联 → 需绑定：

1. 注册补全页提供"已有账号？绑定手机号"入口
2. 通过手机号匹配已有用户
3. 将 `wx_openid` 写入该用户记录
4. 删除临时 `wx_xxxx` 影子账号

## 微信小程序配置要求

### 服务器域名白名单

在 mp.weixin.qq.com → 开发管理 → 开发设置 → 服务器域名：

| 类型 | 域名 |
|------|------|
| request 合法域名 | `https://wx.cadenzayueqi.com` |
| uploadFile 合法域名 | `https://wx.cadenzayueqi.com` (或 OSS 域名) |
| downloadFile 合法域名 | OSS 域名 (如有) |

> 所有域名必须 HTTPS

### 隐私协议

小程序收集用户信息（手机号、位置等）需在「用户隐私保护指引」中声明：

- 手机号：用于登录和订单联系
- 位置：用于附近网点推荐（如使用）
- 身份证照片/姓名/身份证号：用于实名认证与人脸核身（#1787）
- 人脸信息：由慧眼插件采集，用于身份证有效性确认（#1787）

### 慧眼人脸核身插件（#1787）

人脸识别核身依赖腾讯云慧眼 FaceID 插件，**需企业主体**：

1. 腾讯云控制台开通「人脸核身」服务，创建 API 密钥（SecretId/SecretKey，与 #1782 OCR 共用同一套凭证）
2. 微信小程序后台 → 设置 → 第三方服务 → 添加「慧眼人脸核身」插件（企业主体审核，1-3 工作日）
3. 前端 `EditProfile.jsx` 的 `handleFaceVerify` 中完成插件跳转接入（当前为 TODO，`plugin://faceid/verify?token=...`）
4. 慧眼按次计费套餐购买
5. 未配置时降级：face-verify 端点返回 40012，前端隐藏人脸认证按钮，三处身份证照片上传不受影响

### 环境变量

BeaconIAM 需新增：

```env
WX_APPID=wx1234567890abcdef
WX_SECRET=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

Tuneloop 后端新增（#1787，dev/预生产/生产三处 `.env`）：

```env
TENCENTCLOUD_SECRET_ID=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
TENCENTCLOUD_SECRET_KEY=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
TENCENTCLOUD_FACEID_REGION=ap-guangzhou
```

> 未配置 TENCENTCLOUD_SECRET_ID/KEY 时服务正常启动，face-verify 返回 40012（降级模式）。

## 数据模型变更

### BeaconIAM: users 表

| 新增字段 | 类型 | 说明 |
|---------|------|------|
| `wx_openid` | `varchar(128)` | 微信 OpenID，唯一索引 (NULL 排除) |
| `wx_session_key` | `varchar(256)` | 微信会话密钥，用于解密手机号等，有 TTL |

### Tuneloop: users 表

| 新增字段 | 类型 | 说明 |
|---------|------|------|
| `wx_openid` | `varchar(128)` | 微信 OpenID，本地缓存，权威源在 IAM |

> 按 #685 教训：IAM 为权威源，本地仅为缓存。wx-login 创建用户后需同步到本地 `users` 表。

### 新增 API 端点

| 端点 | 位置 | 说明 |
|------|------|------|
| `POST /api/v1/auth/wx-login` | BeaconIAM | code → openid → 查/建用户 → JWT |
| `POST /api/v1/auth/wx-phone` | BeaconIAM | 解密手机号 + 绑定到用户 |
| `POST /api/wx/login` | Tuneloop | 代理到 IAM wx-login，同步本地用户 |
| `POST /api/wx/phone` | Tuneloop | 代理到 IAM wx-phone |

## Linux CI / 自动化部署

### 构建命令

| 命令 | 说明 | 依赖 |
|------|------|------|
| `make mobile-weapp-dev` | Taro weapp watch 模式（开发） | Node.js v22 |
| `npm run build:weapp` | 生产构建 → `dist-weapp/` | Node.js v22 |
| `make weapp-upload-prod VERSION=x.y.z DESC="msg"` | 构建 + 上传到微信服务器 | Node.js v22 + 私钥 |
| `make release` | 全量打包（含 weapp 产物） | Node.js v22（weapp 步骤） |

### CI 部署流程

```bash
# 1. 构建 weapp
cd frontend-mobile && npm run build:weapp

# 2. 上传到微信（需私钥文件 private.APPID.key）
make weapp-upload-prod VERSION=1.2.3 DESC="bug fixes"

# 3. 登录 mp.weixin.qq.com → 版本管理
#    - 设为体验版 → 白名单测试
#    - 提交审核 → 发布上线
```

### 前置条件

- **Node.js v22**: `nvm use 22`
- **私钥文件**: 放在 `frontend-mobile/private.APPID.key`（已加入 `.gitignore`）
- **IP 白名单**: 构建服务器的公网 IP 需在微信后台添加

### 私钥安全

`.gitignore` 已忽略 `*.key` 文件，私钥通过安全渠道（环境变量或 CI Secrets）管理。

## 关联 Issue

| Issue | 内容 | 状态 |
|-------|------|------|
| #872 | 小程序化迁移 Epic | Closed |
| #873 | Phase 1: Taro 骨架 + 双端构建 | Closed |
| #874 | Phase 2A: 纯展示页迁移 | Closed |
| #875 | Phase 2B: 交互页迁移 | Closed |
| #876 | Phase 2C: 员工操作页迁移 | Closed |
| #877 | Phase 3: 无密登录 | Closed |
| #878 | Phase 4: 文档更新 + 发布 | Closed |
| #879 | Makefile + CI: weapp build | In Progress |
| #881 | Taro .tsx page entries | In Progress |
| [beaconiam#366](https://github.com/HiJohns/beaconiam/issues/366) | wx-login 端点 + openid 字段 | Open |

---

> Last updated: 2026-06-11
