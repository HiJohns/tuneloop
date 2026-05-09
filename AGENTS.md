# AGENTS.md

> Universal coding rules: see `prompts/instructions.md` §1.1

This file contains instructions and guidelines for AI coding agents working in this repository.

## 核心文档 (Core Documents)

以下文档被视为项目的核心文档，修改这些文档需要同步更新相关配置和代码：

- `README.md` - 项目主文档，包含开发环境和预生产环境配置
- `AGENTS.md` - AI 助手工作指南
- `docs/features.md` - 功能需求文档
- `docs/api.md` - API 接口文档
- `docs/ui.md` - UI 设计文档
- `docs/iam.md` - IAM 集成说明
- `docs/permissions.md` - 权限-人员矩阵

---

## Project Structure

```
tuneloop/
├── backend/                 # Go REST API server
│   ├── main.go              # Route registration & server entry
│   ├── middleware/           # Auth (IAMInterceptor), permission guards
│   ├── handlers/            # HTTP handlers
│   ├── services/            # IAM client, permission registry
│   ├── models/              # GORM data models
│   └── database/            # DB init, migrations, context keys
├── frontend-pc/             # PC Web admin (React + Vite + Ant Design)
│   └── src/
│       ├── App.jsx          # Main layout, menu config, permission filter
│       ├── pages/           # Page components
│       ├── components/      # Shared components (ProtectedRoute, etc.)
│       ├── services/api.js  # Axios API client + auth interceptors
│       └── config/          # menuPermissions.js — permission rules
├── frontend-mobile/         # WeChat mini-program / mobile web (Vite)
│   └── src/
├── docs/                    # Core documentation (bilingual: zh + en)
└── scripts/                 # Build & CI helper scripts
```

## Repository Status
**Note**: This repository is currently empty. The guidelines below represent standard best practices for modern web development. Update them as the project structure becomes established.

## Build & Development Commands

### Database Access (PostgreSQL)

```bash
# Access PostgreSQL in development environment
docker exec -it jobmaster-postgres psql -U tuneloop_user -d postgres
```

### Package Managers
**Detect package manager by checking for:**
- `package-lock.json` → Use `npm`
- `yarn.lock` → Use `yarn`
- `pnpm-lock.yaml` → Use `pnpm`

### Common Commands
```bash
# Install dependencies
npm install        # or yarn, pnpm install

# Development server
npm run dev        # or yarn dev, pnpm dev

# Build for production
npm run build

# Run linter
npm run lint

# Run type checking
npm run typecheck  # or npm run tsc

# Run tests
npm test           # or npm run test
npm run test:watch # Watch mode
npm run test:unit  # Unit tests only
npm run test:ui    # Component tests

# Run a single test
npm test -- path/to/test.spec.ts
npm test -- --testNamePattern="test description"
```

### Framework-Specific Commands
- **Next.js**: `next dev`, `next build`, `next lint`
- **Vite**: `vite`, `vite build`
- **Create React App**: `react-scripts start`, `react-scripts build`
- **Turborepo**: `turbo run dev`, `turbo run build`, `turbo run lint`

## Learning More
As the codebase grows, this file should be updated with:
- Specific commands from `package.json` scripts
- Project-specific architectural decisions
- Component library conventions
- API patterns and data fetching strategies
- State management patterns
- Styling conventions (CSS modules, styled-components, etc.)

> Detailed UI specifications, navigation structure, permissions, and code locations: see `docs/ui.md`

---

## 🛡️ 审核经验积累 (Audit Lessons Learned)

### 2026-04-30: API 路径不匹配 Bug (#403)

**问题现象：**
前端调用 `POST /api/api/iam/organizations/sync`，后端注册 `/api/iam/organizations/sync`，返回 404。

**根因：**
前端 axios 实例已配置 `baseURL = '/api'`，但 API 方法又额外添加了 `/api` 前缀，导致路径重复。

**为什么审核没发现：**
- 只做了静态检查（go build, npm run lint）
- 没有运行时端到端验证
- 前后端分离审查，未对比路径一致性

**以后审核新增 API 端点时必须检查：**
1. [ ] **路径一致性**：前端调用路径 vs 后端注册路径是否一致
2. [ ] **baseURL 重复**：检查 axios/fetch 的 baseURL 是否导致路径前缀重复
3. [ ] **运行时验证**：建议用 curl 或浏览器 DevTools 验证实际请求的 URL
4. [ ] **响应码验证**：确认 200/201 而非 404

**检查方法：**
```bash
# 1. 查看后端注册的路由
grep -n "POST.*iam.*organizations" backend/main.go

# 2. 查看前端调用的路径
grep -n "iam.*organizations" frontend-pc/src/services/api.js

# 3. 检查 baseURL
grep -n "baseURL\|API_BASE_URL" frontend-pc/src/services/api.js

# 4. 运行时验证（后端启动后）
curl -X POST http://localhost:5554/api/iam/organizations/sync -H "Authorization: Bearer $TOKEN"
```

### 2026-05-06: JWT RSA 签名验证失败 — 多套 beaconiam 实例密钥不匹配

**问题现象：**
登录成功进入 Dashboard 后立即被踢回登录页，后端日志反复报错 `crypto/rsa: verification error`，返回 401。

**环境上下文：**
- 同一台服务器运行 **两套 beaconiam 实例**：
  - **预生产服**：端口 5560（NGINX 代理），独立 JWT 密钥对
  - **开发/测试服**：端口 5561（`go run cmd/api/main.go`），独立 JWT 密钥对
- Tuneloop 后端 `.env` 配置 `BEACONIAM_INTERNAL_URL=http://localhost:5561`

**根因：**
用户浏览器中的 token 是**预生产服 beaconiam（5560）** 签发的，但 Tuneloop 后端调用的是**开发服 beaconiam（5561）** 的 `/api/v1/auth/public-key.pem` 来验证签名。两套实例各自持有不同的 RSA 密钥对，公钥不匹配导致签名验证必然失败。

**时间线还原：**
1. 23:15 — 预生产服 beaconiam 启动（5560），生成密钥 A
2. 23:26 — 用户登录，拿到预生产服签发的 token（`iss: http://opencode.linxdeep.com:5552`）
3. 23:31 — 开发服 beaconiam 启动（5561），生成密钥 B
4. 23:39 — 开发服 Vite 启动（5552），用户重新登录但仍携带旧 token
5. Tuneloop 后端用密钥 B 的公钥验证密钥 A 签发的 token → 失败

**以后审核 JWT 验证失败时必须检查：**
1. [ ] **Token 签发方**：解码 token payload，确认 `iss` 字段指向哪个 beaconiam 实例
2. [ ] **公钥一致性**：对比签发方公钥 vs 验证方公钥是否相同
3. [ ] **环境隔离**：确认当前测试的是预生产服还是开发服，不要混用端口
4. [ ] **密钥轮换后处理**：beaconiam 重启生成新密钥后，必须清除浏览器 localStorage 里的 `token` 并重新登录
5. [ ] **配置一致性**：确认 `.env` 里的 `BEACONIAM_INTERNAL_URL` 与实际签 token 的实例一致

**检查方法：**
```bash
# 1. 查看各个 beaconiam 实例的公钥（对比是否一致）
curl -s http://localhost:5560/api/v1/auth/public-key.pem | sha256sum
curl -s http://localhost:5561/api/v1/auth/public-key.pem | sha256sum

# 2. 查看 token 的签发方（复制 payload 部分到 jwt.io 或 python 解码）
echo "TOKEN" | cut -d. -f2 | base64 -d 2>/dev/null | jq '.iss'

# 3. 确认 Tuneloop 后端配置的验证端点
grep "BEACONIAM_INTERNAL_URL" /home/coder/tuneloop/.env

# 4. 强制刷新 token：浏览器 DevTools → Application → Local Storage → 删除 token → 刷新页面重新登录
```

**关键认知：**
- `opencode.linxdeep.com` 域名解析到当前服务器，5552 端口是开发服 Vite（代理到 5561），5560 是预生产服 NGINX。
- 两套服务的 `jwt_private.pem` / `jwt_public.pem` 完全独立，不存在共享。
- 只要 token 的签发方和验证方不是同一个 beaconiam 进程（或同一份密钥文件），就一定会出现 `crypto/rsa: verification error`。

---

*Last updated: 2026-05-06*

---

## Page URL Checklist

Add new pages must verify:
- [ ] Support list page URL (`/:page`)
- [ ] Support detail page URL (`/:page/:id`)
- [ ] Support edit page URL (`/:page/:id/edit`)
- [ ] Support create page URL (`/:page/new`)
- [ ] URL updates correctly after operations
- [ ] Browser forward/back works correctly
- [ ] Left menu active state syncs correctly

## Properties & Instrument Level

### Properties Data Model

```
properties (属性定义)     property_options (属性选项)    instrument_properties (乐器属性)
├── id                   ├── id                        ├── id
├── name                 ├── property_id (FK)          ├── instrument_id (FK)
├── property_type        ├── value                     ├── property_id (FK)
├── is_required          ├── status                    └── ...
├── unit                 └── alias
└── ...
```

### Create Instrument with Properties

When POST /api/instruments includes `properties` field:
1. Look up property definition by `key` in `properties` table (`name = key`)
2. Match options by `property_id` and `value` in `property_options` table
3. Auto-create missing options with `status = 'pending'`
4. Create `instrument_properties` association records

### Instrument Level Data Model

```sql
instrument_levels
├── id (UUID, PK)
├── caption (varchar) - display name: 入门/专业/大师
├── code (varchar) - code: entry/professional/master
└── sort_order (int) - sort order
```

Usage:
1. Prefer `level_id`: `POST /api/instruments {"level_id": "uuid-here", ...}`
2. Backward compatible: `POST /api/instruments {"level": "专业", ...}` (auto match)
3. Fallback: if level not defined in instrument_levels table, use legacy string mapping

---

*Migrated from prompts/project.md during consolidation*-

---

## 🚫 红线禁令 (Absolute Prohibitions)

**严禁**：AI 不得自行终止任何非自己启动的进程（包括但不限于 `pkill`、`kill`、`fuser -k` 等命令）。
- 如需终止进程，**必须先告知用户**，由用户手动执行。

**严禁**：AI 不得自行修改数据库记录。
- 如需修改 DB，**必须先告知用户**，由用户手动执行或获得明确授权。

---

