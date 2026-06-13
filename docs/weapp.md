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

### 版本管理规范

- 小程序版本号与 tuneloop 版本同步
- 体验版：开发测试用，仅白名单用户可访问
- 审核版：提交微信审核的版本
- 线上版：审核通过后发布的版本

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

### 环境变量

BeaconIAM 需新增：

```env
WX_APPID=wx1234567890abcdef
WX_SECRET=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

Tuneloop 后端无需新增环境变量（使用现有 IAM 代理配置）。

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
| `make weapp-upload VERSION=x.y.z DESC="msg"` | 构建 + 上传到微信服务器 | Node.js v22 + 私钥 |
| `make release` | 全量打包（含 weapp 产物） | Node.js v22（weapp 步骤） |

### CI 部署流程

```bash
# 1. 构建 weapp
cd frontend-mobile && npm run build:weapp

# 2. 上传到微信（需私钥文件 private.APPID.key）
make weapp-upload VERSION=1.2.3 DESC="bug fixes"

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
