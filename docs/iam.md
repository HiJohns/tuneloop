# 🛡️ BeaconIAM 系统设计文档 (V1.0)

## 一、 系统架构与设计哲学

### 1.1 设计原则

* **Identity-first (身份优先)**：废除固定 `admin` 账号，所有操作权限基于 `User + Role + Organization`。
* **Delegated Admin (分权管理)**：贯彻“管人不管事”，支持组织内部自治。
* **Lightweight & Embedded (轻量内嵌)**：后端 Go 二进制文件内嵌前端静态资源，单文件即可运行。
* **Multi-organization Ready (多组织就绪)**：支持命名空间隔离，每个命名空间拥有独立的品牌定制和应用生态。

### 1.2 系统架构图

系统由 **Core Engine (Go)**、**Embedded UI (React)** 和 **Database (PG/SQLite)** 组成。

---

## 二、 核心数据模型

### 2.1 命名空间 (Clients/Namespaces)

命名空间是顶级逻辑隔离单元，每个命名空间对应一个独立应用生态。

* `client_id`: 命名空间唯一标识（如：`tuneloop`）
* `client_secret`: 命名空间秘钥（用于 OAuth 认证）
* `old_secret`: 轮换期间的旧秘钥（10分钟有效期）
* `css_style`: 自定义 CSS 样式（支持品牌深度定制）
* `is_active`: 启用状态（软删除标志）

### 2.2 组织 (Organizations)

> **v1.2 术语说明**：在 Issue #138 架构设计中，"租户"和"组"都是 Organization 实体的不同形态：
> - **租户**（Tenant）：顶级组织，`parent_id` 为空的组织记录
> - **组**（Group）：下级组织，`parent_id` 非空的组织记录

支持无限层级的树状结构，区分内部/外部实体。

* `namespace_id`: 归属命名空间（关联到 `clients` 表）
* `parent_id`: 父组织 ID（支持树状层级）
* `tenant_id`: 所属租户 ID（关联到顶级组织）
* `org_path`: 组织路径（如 `1/5/12/`），用于递归查询优化
* `meta` 字段（JSONB）：存储业务规则（如 `max_dispatch_hops`）
* `is_primary`: 主组织标志（每个命名空间可有且仅有一个主组织）
* `is_active`: 启用状态（软删除标志）

### 2.3 用户 (Users)

* `username`: 用户名（唯一，可用于登录）
* `email`: 邮箱（唯一，可用于登录）
* `phone`: 手机号（唯一，可用于登录）
* `password_hash`: 密码哈希（bcrypt 加密）
* `org_id`: 归属组织
* `role`: 功能角色（OWNER, ADMIN, STAFF, WORKER）
* `is_owner`: 物理所有权标志（不可删除）
* `status`: 用户状态（active/inactive/pending）

### 2.4 应用 (NamespaceApps)

命名空间下的应用实例，支持 web/wechat/mobile 类型。

* `namespace_id`: 归属命名空间
* `app_type`: 应用类型（web/wechat/mobile）
* `client_id`: 应用 Client ID
* `client_secret`: 应用秘钥
* `redirect_uris`: 回调地址列表

### 2.5 用户-组织关系 (UserOrganizationRelations)

管理用户与组织的多对多关系。

* `user_id`: 用户 ID
* `org_id`: 组织 ID
* `role`: 用户在组织中的角色
* `is_active`: 关系是否激活

---

## 三、 端口分配与环境配置

### 3.1 端口分配表

| 环境 | 服务 | 端口 | 说明 |
| --- | --- | --- | --- |
| 开发环境 | Vite 开发服务器 | 5552 | 前端开发服务器，带热重载 |
| 开发环境 | 后端 API 服务 | 5561 | 调试后端服务端口 |
| 预生产环境 | IAM 服务 | 5560 | NGINX 反向代理到 IAM |
| 预生产环境 | Web 服务 | 443/80 | HTTPS/HTTP 公网访问 |
| 数据库 | PostgreSQL | 5432 | 数据库服务端口 |
| 缓存 | Redis | 6379 | 缓存服务端口 |

### 3.2 环境配置对比

| 配置项 | 开发环境 | 预生产环境 |
| --- | --- | --- |
| **工作目录** | 项目根目录 | `prerelease/` |
| **数据库** | `beaconiam_debug` | `beaconiam_db` |
| **端口** | 5552 (前端) / 5561 (后端) | 5560 (NGINX) |
| **配置位置** | 根目录 `.env` | `prerelease/.env` |
| **运行方式** | `make web-dev` + `make run` | `make prerelease` + systemd |
| **访问地址** | `http://localhost:5552` | `https://iam.cadenzayueqi.com` |

## 四、 部署指南

### 4.1 开发环境启动

```bash
# 1. 启动后端服务（端口 5561）
make run

# 2. 启动前端开发服务器（端口 5552，代理到后端 5561）
make web-dev

# 访问 http://localhost:5552
```

### 4.2 预生产环境部署

#### 工作目录
`/opt/beaconiam/` (symlink → `/opt/flow/<version>/beaconiam/`)

#### 构建

```bash
# 完整打包
make release
# 产物：/opt/flow/beaconiam_<timestamp>.zip

# 或增量更新（直接替换，不重建 symlink）
make ui-build && go build -o /opt/flow/<cur>/beaconiam/service/beaconiam ./cmd/api && cp -r ui/dist/* /opt/flow/<cur>/beaconiam/www/
sudo systemctl restart beaconiam
```

#### 部署

```bash
# 首次部署（创建 /opt/beaconiam/.env + JWT 密钥）
sudo cp -r /opt/flow/<version>/beaconiam/service /opt/flow/<version>/beaconiam/www /opt/beaconiam/
sudo chown deploy:deploy -R /opt/beaconiam
sudo systemctl restart beaconiam
```

#### 服务管理

```bash
sudo systemctl start/stop/restart beaconiam
sudo journalctl -u beaconiam -f
```

#### 超级管理员

设置 `ADMIN_DEFAULT_PASSWORD` 环境变量后首次启动自动创建：

| 用户名 | 角色 | 说明 |
|--------|------|------|
| `administrator` | OWNER | 全局超级管理员，首次登录强制修改密码 |

```env
ADMIN_DEFAULT_PASSWORD=<初始密码>
```

## 五、 API 调用接口规范

### 3.1 系统初始化 (Bootstrap)

无需认证即可访问的端点，用于系统首次启动时的初始化。

| 接口名称 | 方法 | 路径 | 说明 |
| --- | --- | --- | --- |
| **检查初始化状态** | GET | `/api/v1/bootstrap/status` | 检查系统是否已初始化 |
| **执行初始化** | POST | `/api/v1/bootstrap/init` | 创建管理员账户，完成系统初始化 |

### 3.2 认证模块 (Auth)

| 接口名称 | 方法 | 路径 | 说明 |
| --- | --- | --- | --- |
| **授权重定向** | GET | `/oauth/authorize` | 获取 `code`，带上 `client_id` 和 `redirect_uri` |
| **令牌交换** | POST | `/api/v1/auth/token` | 业务后端用 `code` 换取 `access_token` (JWT) |
| **身份预检** | GET | `/api/v1/auth/me` | 返回当前用户详细信息 |
| **获取 JWT 公钥** | GET | `/api/v1/auth/public-key` | 获取 JWT 公钥（用于验证签名） |

**响应格式**

```json
// POST /api/v1/auth/token (client_credentials)
{
  "access_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
  "token_type": "Bearer",
  "expires_in": 3600
}
```

### 3.3 命名空间管理 (Namespaces)

权限要求：需要认证，OWNER/ADMIN 角色可操作。

| 接口名称 | 方法 | 路径 | 权限要求 |
| --- | --- | --- | --- |
| **列出命名空间** | GET | `/api/v1/namespaces` | Staff+ |
| **获取详情** | GET | `/api/v1/namespaces/:id` | Staff+ |
| **创建命名空间** | POST | `/api/v1/namespaces` | Owner |
| **更新信息** | PUT | `/api/v1/namespaces/:id` | Admin+ |
| **停用命名空间** | DELETE | `/api/v1/namespaces/:id` | Owner |
| **轮换秘钥** | POST | `/api/v1/namespaces/:id/rotate-secret` | Owner |

**响应格式**

```json
// GET /api/v1/namespaces/:id
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "namespace_id": "tuneloop",
  "name": "TuneLoop",
  "description": "乐器租赁与管理系统",
  "logo_url": "https://example.com/logo.png",
  "primary_color": "#2A6DF4",
  "welcome_text": "欢迎使用 TuneLoop",
  "is_active": true,
  "apps": [
    {
      "app_id": "app_001",
      "app_type": "web",
      "client_id": "tuneloop_web",
      "client_secret": "***REDACTED***",
      "redirect_uris": ["http://localhost:3000/callback"],
      "is_active": true,
      "created_at": "2026-04-19T10:00:00Z"
    }
  ],
  "created_at": "2026-04-19T10:00:00Z",
  "updated_at": "2026-04-19T10:00:00Z"
}
```

### 3.4 组织管理 (Organizations)

权限要求：需要认证，OWNER/ADMIN 角色可操作。

| 接口名称 | 方法 | 路径 | 权限要求 |
| --- | --- | --- | --- |
| **列出组织** | GET | `/api/v1/organizations` | Admin+ |
| **获取详情** | GET | `/api/v1/organizations/:id` | Staff+ |
| **创建组织** | POST | `/api/v1/organizations` | Owner |
| **更新组织** | PUT | `/api/v1/organizations/:id` | Admin+ |
| **停用组织** | DELETE | `/api/v1/organizations/:id` | Owner |
| **获取组织用户** | GET | `/api/v1/organizations/:id/users` | Staff+ |

**响应格式**

```json
// GET /api/v1/organizations
{
  "organizations": [
    {
      "id": "org_001",
      "name": "主组织",
      "namespace_id": "tuneloop",
      "parent_id": null,
      "is_primary": true,
      "is_active": true,
      "max_hops": 5,
      "meta": { "max_dispatch_hops": 3 },
      "created_at": "2026-04-19T10:00:00Z"
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 20
}

// GET /api/v1/organizations/:id
{
  "id": "org_001",
  "name": "主组织",
  "namespace_id": "tuneloop",
  "parent_id": null,
  "is_primary": true,
  "is_active": true,
  "max_hops": 5,
  "meta": { "max_dispatch_hops": 3 },
  "created_at": "2026-04-19T10:00:00Z",
  "updated_at": "2026-04-19T10:00:00Z"
}
```

### 3.5 用户管理 (Users)

权限要求：需要认证。

| 接口名称 | 方法 | 路径 | 权限要求 |
| --- | --- | --- | --- |
| **列出用户** | GET | `/api/v1/users` | Admin+ |
| **获取用户详情** | GET | `/api/v1/users/:id` | Staff+ |
| **更新用户信息** | PUT | `/api/v1/users/:id` | Staff+ |
| **停用用户** | DELETE | `/api/v1/users/:id` | Admin+ |
| **重发确认邮件** | POST | `/api/v1/users/resend-confirmation` | Staff+ |
| **重置密码** | POST | `/api/v1/users/reset-password` | Staff+ |
| **设置密码** | POST | `/api/v1/auth/setup-password` | 无需认证（需 session） |

**响应格式**

```json
// GET /api/v1/users
{
  "users": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "username": "admin",
      "email": "admin@example.com",
      "phone": "13800138000",
      "name": "管理员",
      "status": "active",
      "role": "OWNER",
      "created_at": "2026-04-19T10:00:00Z"
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 20
}

### 3.6 应用管理 (Apps)

| 接口名称 | 方法 | 路径 | 权限要求 |
| --- | --- | --- | --- |
| **列出应用** | GET | `/api/v1/clients` | Staff+ |

---

## 四、 对接开发指南

### 4.1 登录跳转流 (The Redirect Flow)

1. **引导**：业务系统发现用户未登录，重定向至 IAM 登录页，带上 `client_id` 和 `redirect_uri`。
2. **验证**：用户在 IAM 页面完成登录，IAM 携带 `code` 跳回 `redirect_uri`。
3. **握手**：业务后端在服务器侧调 IAM 的 `/token` 接口完成身份确权。

### 4.2 界面定制化 (White-labeling)

Beacon-IAM 支持根据命名空间自动换肤：

* **逻辑**：前端登录页加载时，根据 URL 中的 `client_id` 向后端请求该命名空间的配置。
* **配置项**：
  - `logo_url`: 品牌 Logo
  - `primary_color`: 主题色
  - `welcome_text`: 欢迎词
  - `css_style`: 自定义 CSS（支持复杂样式覆盖）

### 4.3 秘钥轮换机制

命名空间秘钥支持安全轮换：

1. 调用 `/api/v1/namespaces/:id/rotate-secret` 生成新秘钥
2. 旧秘钥保留 10 分钟有效性（平滑过渡）
3. 10 分钟后旧秘钥自动失效
4. 所有关联应用需在此期间更新配置

---

## 五、 部署说明

### 5.1 环境变量配置参考

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `BEACONIAM_PORT` | `5552` | 服务端口 |
| `DB_TYPE` | `sqlite` | 数据库类型 (sqlite/postgres) |
| `DB_PATH` | `./data/beaconiam.db` | SQLite 文件路径 |
| `POSTGRES_HOST` | `localhost` | PostgreSQL 主机 |
| `POSTGRES_USER` | `postgres` | PostgreSQL 用户名 |
| `POSTGRES_PASSWORD` | - | PostgreSQL 密码 |
| `BEACONIAM_DB` | `beaconiam` | PostgreSQL 数据库名 |
| `JWT_SECRET` | - | JWT 签名密钥 |
| `JWT_SESSION_TIMEOUT` | `60` | Session 超时时间（分钟），支持系统级、命名空间级、组织级配置 |
| `APP_ENV` | `development` | 应用环境 |
| `BOOTSTRAP_CLIENT_ID` | - | 自动引导 Client ID |
| `ADMIN_DEFAULT_PASSWORD` | - | 首次启动时创建超级管理员 `administrator`（仅当无 OWNER 用户时生效） |

#### Session 超时配置 | Session Timeout Configuration

系统支持三层配置的优先级（从高到低）：
1. **组织级**：通过 `PUT /api/v1/organizations/:id` 的 meta 字段设置 `session_timeout`（单位：分钟）
2. **命名空间级**：通过 `PUT /api/v1/namespaces/:id` 设置 `session_timeout` 字段（单位：分钟）
3. **系统默认**：通过环境变量 `JWT_SESSION_TIMEOUT` 设置（默认 60 分钟）

JWT Token 的过期时间将按此优先级获取配置。

**配置示例**：
```bash
# 设置系统默认 session 超时为 2 小时（120 分钟）
export JWT_SESSION_TIMEOUT=120
```

**注意**：组织级和命名空间级的配置会覆盖系统默认值。如果不设置，将使用系统默认的 60 分钟。

### 5.2 邮件配置说明

Beacon IAM 支持三种邮件发送模式：

#### 模式一：Stub 模式（默认）
当 `SMTP_HOST` 为空时，系统仅记录邮件日志到控制台，不发送真实邮件。

```bash
SMTP_HOST=
```

#### 模式二：Debug 模式（开发推荐）
设置 `MAIL_DEBUG=true`（或 `MAIL_DEBUG=1`），邮件内容将以文本格式保存到 `debug/` 目录，便于开发调试。无需配置 `SMTP_HOST`。

```bash
MAIL_DEBUG=true
```

**调试文件示例**：`debug/user_at_example.com_1642153325.txt`

**文件内容格式**：
```
To: user@example.com
Subject: Confirm your email - Beacon IAM

[邮件正文内容...]
```

#### 模式三：SMTP 生产模式
配置完整的 SMTP 参数，系统将通过真实邮件服务器发送邮件。

```bash
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=your-email@gmail.com
SMTP_PASSWORD=your-app-password
SMTP_FROM=noreply@yourdomain.com
SMTP_USE_TLS=1
MAIL_DEBUG=0
```

**邮件配置参考表**

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `SMTP_HOST` | - | SMTP 服务器地址（留空使用 Stub 模式） |
| `SMTP_PORT` | `587` | SMTP 服务器端口 |
| `SMTP_USER` | - | SMTP 认证用户名 |
| `SMTP_PASSWORD` | - | SMTP 认证密码 |
| `SMTP_FROM` | `noreply@beaconiam.com` | 发件人地址 |
| `SMTP_USE_TLS` | `1` | 是否启用 TLS 加密（1=启用，0=禁用） |
| `MAIL_DEBUG` | `0` | Debug 模式（1=保存邮件到文件，0=真实发送） |

**云原生配置原则**: 生产环境 (`APP_ENV=production`) 仅依赖环境变量。

---

## 六、 快速开始

### 6.1 安装

```bash
# 下载二进制文件
curl -L https://github.com/HiJohns/beaconiam/releases/latest/download/beaconiam-linux-amd64 -o beaconiam
chmod +x beaconiam

# 运行（使用 SQLite）
./beaconiam
```

### 6.2 系统初始化

首次访问系统会自动重定向到初始化页面：

1. 访问 `http://localhost:5552`
2. 创建管理员账户
3. 保存显示的密码（仅显示一次）
4. 10秒后自动跳转到登录页

### 6.3 首次使用

1. **创建命名空间**：进入“命名空间管理”页面，创建新的命名空间
2. **添加应用**：在命名空间详情页，添加应用并配置回调地址
3. **创建组织**：在“组织管理”页面，创建组织
4. **添加用户**：在“用户管理”页面，创建用户并分配到组织

---

## 七、 开发者集成 Tip

### 7.1 JWT 令牌标准

> **v1.2 扩展**：根据 Issue #138 架构设计，JWT Payload 已扩展。

JWT Payload 包含以下字段：

| 字段 | 说明 | 示例 |
|------|------|------|
| `iss` | 签发者 | "beacon-iam" |
| `sub` | 用户 ID | "550e8400-e29b-41d4-a716-446655440000" |
| `nid` | 命名空间 ID | "770g...9900" |
| `tid` | 租户 ID（顶级组织） | "tenant-uuid" |
| `gid` | 组 ID（当前组织） | "group-uuid" |
| `role` | 结构角色 | "OWNER" |
| `is_owner` | 是否所有者 | true |
| `roles` | 职能角色列表 | ["repair-tech"] |
| `sys_perm` | 系统权限位图 | 255 |
| `cus_perm` | 客户权限位图 | 127 |
| `cus_perm_ext` | 客户权限位图扩展 | "base64..." |

> **说明**：`tid` 和 `gid` 均来自 `organizations` 表。租户是顶级组织（parent_id 为空），组是下级组织（parent_id 非空）。

### 7.2 Tuneloop 本地 ID 策略

Tuneloop 不维护独立的租户/组织本地 UUID。所有 TenantID / OrgID 字段直接使用 IAM 的 org ID。

| 本地表 | 字段 | 来源 |
|--------|------|------|
| `tenants.ID` | 主键 | IAM org ID（与 `merchants.OrgID` 相同） |
| `merchants.TenantID` | namespace scope | namespace IAM org ID |
| `merchants.OrgID` | 商户 IAM org ID | IAM `organizations` 表 |
| `sites.TenantID` | 父租户 | 父 IAM org ID |
| `sites.OrgID` | 网站 IAM org ID | IAM `organizations` 表 |
| `site_members.tenant_id` | 父租户 | 父 IAM org ID（JWT `tid`） |
| `users.TenantID` | 父租户 | 父 IAM org ID（JWT `tid`） |
| `users.OrgID` | IAM org ID | IAM `organizations` 表 |

> **设计决策（#651）**：废除 `tenants` 表 `default:gen_random_uuid()`。所有 ID 字段均为 IAM org 体系中的 UUID，无本地标识符。管理页面同步 IAM 组织时按需创建租户记录。

### 7.3 密码重置集成 (Password Reset Integration)

下游系统调用 `POST /api/v1/users/reset-password` 为用户发送密码重置邮件。

**请求示例**：
```json
POST /api/v1/users/reset-password
Authorization: Bearer <user_jwt>

{
  "user_ids": ["uuid1", "uuid2"],
  "redirect_url": "https://your-app.example.com/login"
}
```

**字段说明**：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `user_ids` | string[] | ✅ | 目标用户 ID 列表 |
| `redirect_url` | string | **强烈建议** | 用户设置密码成功后跳转回客户端应用的 URL |

**⚠️ `redirect_url` 的重要性**：

- **传入 `redirect_url`**：用户设置密码后，跳转到 IAM 登录页（带 `redirect_uri` 参数），登录后自动返回客户端应用
- **不传 `redirect_url`**：用户停留在 IAM 的"激活成功"页面，需手动导航回客户端

**完整流程**：
1. 客户端调用 `reset-password`，传入 `user_ids` 和 `redirect_url`
2. IAM 为每个用户创建密码重置会话，发送重置邮件
3. 用户点击邮件链接，在 IAM 页面设置新密码
4. 密码设置成功后，跳转到 IAM 登录页（`/login?redirect_uri=https://your-app.example.com/login`）
5. 用户在 IAM 登录后，自动跳转回 `redirect_url`（即客户端应用）
6. 用户回到客户端应用，完成身份认证

**权限要求**：需 STAFF 及以上角色的用户 JWT，不支持 client_credentials token。

### 7.4 下游系统集成建议

1. **验证签名**：使用 Beacon-IAM 提供的公钥验证 JWT。
2. **上下文提取**：将 `nid`、`tid`、`gid` 和权限位图注入请求上下文。
3. **权限判断**：使用 `sys_perm` 和 `cus_perm` 进行位运算判断，无需查库。
4. **刷新机制**：Access Token 过期后，使用 Refresh Token 获取新令牌。

### 7.5 安全警告

⚠️ JWT Payload 不加密，严禁存放敏感信息。

### 7.6 组织管理员操作指南 (Org Admin Operations)

> ⚠️ 客户端开发必读：以下 API 的行为存在关键差异，错误调用会导致静默失败。

客户端在管理组织/商户管理员时，应按以下流程操作：

#### 7.6.1 添加用户为组织管理员

**必须**使用 `BindUser`，不可直接调 `UpdateUserRoleInOrg`：

```
PUT /api/v1/organizations/{org_id}/users/{uid}/bind
{"action": "bind", "role": "OWNER"}
```

| 组织类型 | 行为 |
|----------|------|
| 顶级组织 | 排队任务 → 用户需通过邮件确认后才生效 |
| 次级组织 | 检查父租户成员关系 → 直接绑定（已在父租户）或排队父租户+子组织任务 |

#### 7.6.2 降级管理员为普通成员

```
PUT /api/v1/organizations/{org_id}/users/{uid}/role
role=USER
```

**限制**：此端点仅更新已存在的 `user_org_relations` 行。若用户不在组织内，API 返回 200 但**无任何效果**（静默 no-op）。

#### 7.6.3 移除用户

```
PUT /api/v1/organizations/{org_id}/users/{uid}/bind
{"action": "unbind"}
```

用户从组织移除（`is_active = false`）。

#### 7.6.4 典型错误与正确做法

| ❌ 错误做法 | 后果 | ✅ 正确做法 |
|------------|------|------------|
| `UpdateUserRoleInOrg` 调用在非成员用户上 | 静默 no-op，用户未加入组织 | 先 `BindUser(action=bind, role=OWNER)` |
| `Unbind` 不检查是否有替代管理员 | 组织可能无管理员 | 前端先校验：至少保留一名管理员 |
| 假设顶级组织 bind 立即生效 | 返回 `task_ids` 而非即时的关系创建 | 检查响应，引导用户查收确认邮件 |

#### 7.6.5 替换管理员（推荐流程）

```
Step 1: PUT /organizations/{org_id}/users/{new_uid}/bind  {"action":"bind","role":"OWNER"}
Step 2: 等待新管理员确认（如顶级组织走任务队列）
Step 3: PUT /organizations/{org_id}/users/{old_uid}/role  role=USER
  或:   PUT /organizations/{org_id}/users/{old_uid}/bind  {"action":"unbind"}
```

### 7.7 冷启动流程 (Cold Start)

客户端应用首次接入 BeaconIAM 的完整流程，所有端点的鉴权均依赖 `X-Namespace-Secret` 头（命名空间开启时获得，非 OAuth JWT）。

```
Step 1: 激活命名空间                              Step 2 (可选): 注册额外应用
POST /api/v1/namespaces/:id/activate              POST /api/v1/namespaces/:id/apps
X-Namespace-Secret: <ns_secret>                    X-Namespace-Secret: <ns_secret>
{                                                  {
  "apps": [{"type":"web","redirect_uris":[...]}]     "type": "mobile",
}                                                    "redirect_uris": ["myapp://callback"]
         ↓                                          }
         { "apps": [{"client_id":"...",              ↓
           "client_secret":"...", ...}] }            { "client_id":"...", "client_secret":"..." }

Step 3: 创建命名空间管理员
POST /api/v1/namespaces/:id/admin
X-Namespace-Secret: <ns_secret>
{"email": "admin@example.com", "name": "Admin"}
         ↓
    201: {"user": {"id": "...", "email": "...", "status": "pending"}}  (首次)
    200: {"user": {...}}                                                   (已存在，幂等返回)

Step 4 (可选): 重发管理员密码设置邮件
POST /api/v1/namespaces/:id/resend-admin-email
X-Namespace-Secret: <ns_secret>
{"email": "admin@example.com"}
         ↓
    200: {"status": "sent", "session": "..."}
```

| 步骤 | 端点 | 说明 |
|------|------|------|
| 1 | `POST /namespaces/:id/activate` | 由 client_credential 鉴权（namespace secret），创建 OAuth app 并返回凭证 |
| 2 | `POST /namespaces/:id/apps` | （可选）注册额外应用，参数仅需 `type` 和 `redirect_uris` |
| 3 | `POST /namespaces/:id/admin` | 创建命名空间管理员。用户状态为 `pending`，IAM 发送密码设置邮件。同一 email 幂等 |
| 4 | `POST /namespaces/:id/resend-admin-email` | （可选）重发管理员密码设置邮件，用于邮件丢失或过期场景 |

**完整流程**：
1. 管理员在 IAM 管理界面创建命名空间，获得 `namespace_id` 和 `client_secret`
2. 将 `namespace_id` + `client_secret` 存入客户端配置
3. 客户端启动 → 调用 `activate` 激活命名空间 → 获得 app 凭证
4. 调用 `admin` 创建管理员用户（或检查已有管理员）
5. 管理员查收邮件，点击链接设置密码，账户激活后登录系统
6. 若邮件丢失或过期，调用 `resend-admin-email` 重新发送

---

## 八、 TuneLoop 权限集成（#660）

### 8.1 cus_perm 权限体系

TuneLoop 的业务权限（cus_perm）由 IAM 存储和签发，Tuneloop 本地定义权限码语义（10 码），IAM 不参与权限位的含义定义。完整权限矩阵见 `docs/permissions.md`。

### 8.2 SetUserCustomerPermissions（个人 cus_perm）

设置用户在组织中的个人业务权限（raw bits 模式，beaconiam #293 Phase 1）：

```
PUT /api/v1/organizations/{org_id}/users/{uid}/customer-permissions
```

**请求 Body**（raw_bits 模式）：
```json
{
  "raw_bits": true,
  "cus_perm": 3,
  "cus_perm_ext": null
}
```

- `cus_perm`: bits 0-63 的位图（int64）
- `cus_perm_ext`: bits 64+ 的扩展位图（base64 编码，当前 Tuneloop 10 码无需扩展）
- Tuneloop 本地计算 bitmap 后直接传 raw bits

### 8.3 角色模板 cus_perm API

创建自定义角色（#293 Phase 2 保留）：
```
POST /api/v1/namespaces/{ns_id}/role-templates
{ "code": "custom", "name": "自定义角色", "sys_perm": 0, "cus_perm": 7 }
```

设置角色 cus_perm 位图：
```
PUT /api/v1/namespaces/{ns_id}/role-templates/{template_id}
{ "cus_perm": 15, "cus_perm_ext": null }
```

TuneLoop 创建的角色默认 `sys_perm = 0`（不持有系统权限）。

### 8.4 角色分配 API

给用户分配功能角色：
```
POST /api/v1/users/{user_id}/roles
{ "role_template_id": "template-uuid" }
```

给用户在组织中更新角色（role 为 query parameter，非 JSON body）：
```
PUT /api/v1/organizations/{org_id}/users/{uid}/role?role=site_admin
```

### 8.5 JWT cus_perm OR 计算

IAM 在签发 JWT 时计算最终 cus_perm（待 IAM 恢复 `role.CusPerm` OR 分支）：

```
token.cus_perm     = relation.CusPerm | role.CusPerm
token.cus_perm_ext = relation.CusPermExt | role.CusPermExt
```

- `relation.CusPerm`: 个人直接授权（SetUserCustomerPermissions 写入）
- `role.CusPerm`: 角色权限（SetRoleCustomerPermissions 写入）
- Tuneloop 不参与 OR 计算，全由 IAM JWT 签发时完成

### 8.6 启动同步

Tuneloop 启动时同步所有角色模板的 sys_perm + cus_perm 到 IAM：

```go
for code, template := range services.AllRoleTemplates {
    iamClient.SyncRoleTemplateSysPerm(nsID, code, template.SysPermBits)
    cusPerm, cusPermExt := ComputeCusPermBitmapExt(template.CusPermCodes, registry.GetCusPermBit)
    iamClient.SyncRoleTemplateCusPerm(nsID, code, cusPerm, cusPermExt)
}
```

### 8.7 新增 sys_perm 位（bits 25-26）

现有 bits 0-24 不变。追加：

| Bit | 常量名 | 含义 | 持有者 |
|-----|--------|------|--------|
| 25 | `tenant:create` | 创建租户 | 命名空间管理员（仅） |
| 26 | `permission:manage` | 管理权限 | 租户管理员（商户管理员） |

Tuneloop 必须通过 IAM API 间接操作 sys_perm，禁止直接写入。

### 8.8 perm_version 增量

权限变更后调用 `POST /api/v1/perm-version/increment`，通知客户端刷新权限映射：

```go
func (c *IAMClient) IncrementPermVersion() error {
    // POST /api/v1/perm-version/increment
}
```

Tuneloop 的 `SetUserCustomerPermissions` 和 `UpdateUserRoleInOrg` 均已内置调用 `IncrementPermVersion`。

---


