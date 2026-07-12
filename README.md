# TuneLoop - 乐器租赁系统

> 版本: 2.1 | 最后更新: 2026-06-13

基于 **12-Factor App** 和 **Lin-IAM** 身份底座的乐器租赁平台，支持 **租转售 (Rent-to-Own)** 模式。

---

## 📖 项目简介

TuneLoop 是一个面向乐器租赁业务的 SaaS 平台，提供：
- **用户端（微信小程序）**：便捷的乐器浏览、租赁、维保服务
- **商家管理端（PC Web）**：多网点库存管理、订单处理、财务结算
- **平台运营端（PC Web）**：全局治理、商家准入、计费规则配置

---

## 🎯 核心功能

### 用户端（微信小程序）
- ✅ 微信快捷登录，游客可浏览 30 天免登录，注册后会话 1 小时（超时需重新一键登录）
- ✅ 乐器分类与阶梯定价（入门/专业/大师）
- ✅ 3/6/12个月租期选择，12个月享95折
- ✅ **租转售**：租满12个月自动获得所有权
- ✅ 信用免押金体系（信用分达标）
- ✅ 在线维保申请与进度追踪
- ✅ 电子所有权证书下载

### 商家管理端（PC）
- ✅ 设备台账（SN码管理、折旧计算）
- ✅ 库存监控（在库/在租/维保三态）
- ✅ 所有权预警（即将转售资产）
- ✅ 逾期提醒与工单管理
- ✅ 网点间资产调拨
- ✅ 佣金结算与流水报表
- ✅ **Excel批量导入/导出** - 支持乐器信息批量导入，字段映射，错误行提示；支持按条件导出，自定义字段

### 平台运营端（PC）
- ✅ 商家准入审核与RBAC权限配置
- ✅ 定价矩阵（Excel网格编辑）
- ✅ 维保服务包配置
- ✅ 全局结算与押金监管
- ✅ 资产全流程Timeline追踪
- ✅ 全网资产分布地图

---

## 🏗️ 技术架构

### 后端
- **语言**: Go 1.21+
- **框架**: Gin Web Framework
- **数据库**: PostgreSQL (支持JSONB)
- **认证**: Lin-IAM 集成
- **迁移**: golang-migrate

### 前端
- **PC端**: React 18 + TypeScript + Ant Design 5.x + Tailwind CSS
- **移动端**: Taro 4.x（一套 React 代码同时输出 **H5** 和 **微信小程序**）
- **地图**: 高德地图 AMap

> **架构要点**：移动端通过 `src/platform/` 抽象层实现跨端适配。同一份 `.jsx` 代码经 Vite 编译为 H5（浏览器），经 Taro 编译为微信小程序。`.tsx` 文件仅为 Taro 页面注册所需的薄壳（`export { default } from '../../Xxx'`），业务逻辑全在 `.jsx` 中。详见 `docs/weapp.md` 和 `AGENTS.md §跨端代码复用架构`。

### 核心依赖
```
backend:
  - github.com/gin-gonic/gin v1.9.1
  - github.com/golang-jwt/jwt/v5 v5.0.0
  - gorm.io/gorm v1.25.5
  - gorm.io/driver/postgres v1.5.4
  - github.com/golang-migrate/migrate/v4 v4.16.2

frontend-pc:
  - react: ^18.2.0
  - antd: ^5.12.0
  - react-router-dom: ^6.20.0
  - tailwindcss: ^3.3.5

frontend-mobile:
  - react: ^18.2.0
  - @tarojs/cli: ^4.2.0
  - @tarojs/components: ^4.2.0
  - @tarojs/taro: ^4.2.0
  - tailwindcss: ^3.3.5
  - vite: ^5.0.0
```

---

## 🔐 Lin-IAM 集成

### 认证流程
1. **重定向**: 业务系统引导用户至 IAM 登录页
2. **回调**: IAM 携带 `code` 跳转回业务系统
3. **令牌交换**: 后端用 `code` 换取 JWT
4. **上下文注入**: 自动提取 `tid` (租户)、`oid` (组织)、`sub` (用户ID)

### JWT Claims
```json
{
  "iss": "beacon-iam",
  "sub": "uuid-user-id",
  "tid": "uuid-tenant-id",
  "oid": "uuid-org-id",
  "role": "OWNER",
  "own": true
}
```

### 会话有效期

| 用户类型 | 有效期 | 配置位置 |
|---------|--------|---------|
| 游客 (GUEST) | 30 天 | `init.js` → `ensureGuestToken()` |
| 已注册用户 | 1 小时 | IAM `JWT_SESSION_TIMEOUT=60` (分钟) |

注册用户超时后点击「微信一键登录」即可恢复会话（无需重新注册，wx_openid 已绑定）。

### DEBUG 模式

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `DEBUG_MODE` | `false` | 设为 `true` 开启调试功能（库存/订单状态编辑等） |

### 白标化 (White-labeling)
- **动态主题**: 根据 `client_id` 加载品牌色和 Logo
- **配置端点**: `GET /api/common/brand-config`
- **覆盖范围**: 登录页、Dashboard、所有组件

---

## 🚀 快速开始

### 环境要求
- Go 1.21+
- Node.js 22 LTS（Taro 4.x 不兼容 Node.js v24）
- PostgreSQL 14+
- 微信开发者工具（用于 weapp 预览和调试）

### 后端启动
```bash
cd backend

# 1. 安装依赖
go mod tidy

# 2. 配置环境变量
cp .env.example .env
# 编辑 .env 文件，配置数据库和IAM参数

# 3. 数据库迁移
go run cmd/migrate/main.go

# 4. 启动服务
go run main.go
# 后端 API: http://localhost:5557
```

### PC端前端启动
```bash
cd frontend-pc

# 1. 安装依赖
npm install

# 2. 配置环境变量
cp .env.example .env
# 编辑 .env 文件

# 3. 开发模式
npm run dev

# 4. 构建生产包
npm run build
```

### 移动端 (H5 + 微信小程序)

```bash
cd frontend-mobile

# 1. 安装依赖
npm install

# 2. H5 开发模式 (浏览器)
npm run dev
# → http://localhost:5553

# 3. 微信小程序开发模式 (需 Node.js v22)
nvm use 22
npm run dev:weapp
# → 微信开发者工具 → 导入 dist-weapp/ 目录

# 4. 构建生产包
npm run build      # H5
npm run build:weapp # 小程序
```

> **架构说明**：同一套 `.jsx` 代码同时编译为 H5（Vite）和微信小程序（Taro）。`src/platform/` 层根据编译目标自动切换浏览器 API ↔ Taro API。详见 `docs/weapp.md`。

---

## 🚀 开发环境

### 工作目录
项目根目录

### 启动服务
```bash
# 启动后端服务
make run

# 启动 PC 前端 Vite 服务
make web-dev

# 启动移动端 H5 Vite 服务
make mobile-dev

# 启动移动端小程序开发编译 (watch)
make mobile-weapp-dev

# 构建全量发布包 (H5 + PC + 小程序 + 后端)
make release

# 上传小程序 (需私钥)
make weapp-upload VERSION=1.0.0 DESC="release note"
```

### 访问地址
- https://opencode.linxdeep.com:5552 → IAM (Vite)
- https://opencode.linxdeep.com:5553 → Mobile H5 (Vite)
- https://opencode.linxdeep.com:5554 → PC Web (Vite)
- https://opencode.linxdeep.com:5557 → 后端 API
- https://opencode.linxdeep.com:5561 → IAM API

### 配置
在根目录的 `.env` 中完成 IAM 相关跳转配置

---

## 🚀 预生产环境

> 详细操作手册：**[docs/prerelease-guide.md](docs/prerelease-guide.md)**

### 工作目录
`/opt/tuneloop/` (symlink → `/opt/flow/<version>/`)

### 编译
```bash
make release    # 一步完成 PC+Mobile+Backend 编译，输出 zip + 包装 test.zip
```
产物：
- `/opt/flow/tuneloop_<timestamp>_<git-hash>.zip` — 原始部署包
- `~/test.zip` — 包裹原始 ZIP 的包装文件（内层即 `tuneloop_<timestamp>_<hash>.zip`），用于 Seafile 传输

### 部署方式 A：直推（`make release` 自动执行）
```bash
make release                              # 编译 + scp 到 cadenza:/opt/flow/
# 然后在 cadenza 上：
/opt/flow/deploy.sh /opt/flow/tuneloop_<timestamp>_<hash>.zip
```

### 部署方式 B：经由 Seafile（适用于直推速度慢时）
```bash
# 1. 编译
make release

# 2. 上传到 Seafile（在本地执行，需配置环境变量）
export SEAFILE_SERVER_URL=https://seafile.example.com
export SEAFILE_USERNAME=your_email
export SEAFILE_PASSWORD=your_password
export SEAFILE_REPO_ID=your_repo_uuid
export SEAFILE_PATH=/deploy
bash scripts/push_seafile.sh

# 3. cadenza 上从 Seafile 下载并部署
export SEAFILE_DEPLOY=https://seafile.example.com/d/.../test.zip
/opt/flow/deploy.sh
```
`deploy.sh` 自动解包 `test.zip` → 找到内层 ZIP → 按文件名解析 `tuneloop` → 正常部署。

### 部署脚本
| 文件 | 位置 | 说明 |
|------|------|------|
| `scripts/push_seafile.sh` | 开发机 | SCP 拉取 `test.zip` 到本地 → 上传 Seafile |
| `/opt/flow/deploy.sh` | cadenza | 接受 ZIP 路径 或 `SEAFILE_DEPLOY` URL |

### 访问地址
- https://web.cadenzayueqi.com => NGINX => :5558 (PC端)
- https://wx.cadenzayueqi.com => NGINX => :5559 (微信端)
- https://iam.cadenzayueqi.com => NGINX => :5560 (IAM 服务)

### 配置
在 `/opt/tuneloop/.env` 中完成 IAM 相关配置

---

## 📁 项目结构

```
tuneloop/
├── backend/                          # Go后端
│   ├── main.go                      # 入口文件
│   ├── handlers/                    # HTTP请求处理器
│   │   ├── api.go                   # 现有API
│   │   ├── auth.go                  # 认证回调
│   │   ├── brand.go                 # 品牌配置
│   │   └── user.go                  # 用户同步
│   ├── middleware/                  # 中间件
│   │   └── iam.go                   # IAM JWT校验
│   ├── services/                    # 业务服务
│   │   └── iam.go                   # IAM交互服务
│   ├── database/                    # 数据库
│   │   ├── db.go                    # 连接管理
│   │   └── migrations/              # 迁移文件
│   │       └── 001_initial_schema.up.sql
│   └── models/                      # 数据模型
│       └── models.go                # 所有模型定义
│
├── frontend-pc/                     # PC前端
│   ├── src/
│   │   ├── App.jsx                  # 根组件
│   │   ├── components/
│   │   │   └── BrandProvider/       # 白标化Provider
│   │   ├── pages/
│   │   │   ├── Login/               # 登录页
│   │   │   ├── Dashboard/           # Dashboard
│   │   │   └── ...
│   │   └── layouts/
│   │       └── MainLayout/          # 主布局
│   └── package.json
│
├── frontend-mobile/                 # 微信小程序
│   ├── pages/                       # 页面目录
│   ├── components/                  # 组件目录
│   └── app.js
│
└── docs/                            # 文档
    ├── features.md                  # 功能需求
    ├── api.md                       # API文档(v2.0)
    ├── ui.md                        # UI设计文档(v2.0)
    ├── iam.md                       # IAM集成说明
    └── prerelease-guide.md          # 预发布操作手册
```

---

## 🔧 关键功能实现

### 1. 数据库迁移
```bash
# 创建新迁移
cd backend/database/migrations
goose create add_new_table sql

# 执行迁移
go run cmd/migrate/main.go
```

### 2. IAM 中间件使用
```go
// 在路由组中应用
api := r.Group("/api")
api.Use(middleware.IAMInterceptor(publicKey))
{
    api.GET("/orders", handlers.GetOrders)
}
```

### 3. 上下文提取
```go
func GetOrders(c *gin.Context) {
    tenantID := middleware.GetTenantID(c.Request.Context())
    orgID := middleware.GetOrgID(c.Request.Context())
    
    // 自动过滤当前租户数据
    orders := db.Where("tenant_id = ?", tenantID).Find(&orders)
}
```

### 4. 白标化配置
```tsx
// 在组件中使用
const { config } = useBrand();

<Button style={{ 
  backgroundColor: config?.primary_color 
}}>
  主题按钮
</Button>
```

---

## 📜 环境变量

### 后端 (.env)
```bash
# PostgreSQL Database
POSTGRES_HOST=localhost          # Database host (default: localhost)
POSTGRES_PORT=5432              # Database port (default: 5432)
POSTGRES_USER=tuneloop          # Database username (default: tuneloop)
POSTGRES_PASSWORD=your_password # Database password
TUNELOOP_DB=tuneloop            # Database name (default: tuneloop)
DB_SSLMODE=disable              # SSL mode (default: disable)

# Beacon IAM Integration
BEACONIAM_EXTERNAL_URL=https://iam.example.com  # External IAM URL (frontend redirect)
BEACONIAM_INTERNAL_URL=http://localhost:8080    # Internal IAM URL (backend API calls)
IAM_CLIENT_ID=tuneloop
IAM_CLIENT_SECRET=your_secret_key

# Service URLs
TUNELOOP_WWW_URL=http://localhost:5554  # PC Web service URL (default)
TUNELOOP_WX_URL=http://localhost:5553   # WeChat mobile service URL (default)
```

### 前端 (.env)
```bash
# IAM (uses external URL for frontend redirect)
VITE_IAM_URL=https://iam.example.com
VITE_CLIENT_ID=tuneloop

# API (backend service)
VITE_API_BASE=http://localhost:5554/api
```

---

## 📝 开发规范

### Commit 规范
```bash
feat: 新功能
fix: 修复bug
docs: 文档更新
test: 测试代码
refactor: 重构代码
```

### 代码风格
- Go: 遵循 Uber Go Style Guide
- React: 遵循 Airbnb React Style Guide
- TypeScript: 严格模式，禁用 `any`

---

## 🤝 贡献指南

1. **Fork** 项目
2. **创建分支**: `git checkout -b feature/your-feature`
3. **提交变更**: `git commit -am 'feat: add new feature'`
4. **推送分支**: `git push origin feature/your-feature`
5. **创建 Pull Request**

---

## 📄 许可证

MIT License

---

## 📞 联系方式

- **项目主页**: https://github.com/HiJohns/tuneloop
- **问题反馈**: https://github.com/HiJohns/tuneloop/issues
- **文档**: https://github.com/HiJohns/tuneloop/tree/main/docs

---

*最后更新: 2026-03-21 | 基于 docs/features.md v26.3.16*
*Model: minimax-m2.5-free*
