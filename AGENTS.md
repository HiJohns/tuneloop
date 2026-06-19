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
- `docs/iam.md` - IAM 集成说明（**symlink → `../../beaconiam/README.md`，禁止本地修改，I AM 权威文档以 beaconiam 仓库为准**）
- `docs/iam-notes.md` - tuneloop 侧对 IAM 的补充说明和过渡期记录
- `docs/permissions.md` - 权限-人员矩阵
- `docs/account-lifecycle.md` - 账户生命周期与数据完整性
- `docs/weapp.md` - 微信小程序架构与部署（Taro 构建策略、三端架构、登录流程、发布流程）

## Environment Guide

See `prompts/instructions.md` for full port mapping. Key rule:

**默认上下文是开发环境 (Dev)，不是预生产 (Prerelease)。**

| 环境 | beaconiam | tuneloop PC | tuneloop Backend | tuneloop Mobile |
|------|-----------|-------------|------------------|-----------------|
| Dev | 5552 (Vite) / 5561 (API) | 5554 (Vite) / 5557 (Go) | `BEACONIAM_INTERNAL_URL=http://localhost:5561` | 5553 (Vite/Taro H5) |
| Prerelease | 5560 (NGINX) | 5558 (Go) | `BEACONIAM_INTERNAL_URL=http://localhost:5560` | — |

- 除非用户明确指明"预生产"，否则所有讨论、调试、Issue 都指 Dev 环境
- 两套环境有独立的 beaconiam 实例、独立的 RSA 密钥对、独立的数据库
- Never mix: 预生产的 token 在开发环境验证必报 `crypto/rsa: verification error`

## Node.js 版本要求

**Taro v4 在 Node.js v24 下存在兼容性问题**（`module is not defined in ES module scope`）。`frontend-mobile` 构建必须使用 **Node.js v22 LTS**（当前: v22.22.3）。

```bash
# 使用 nvm 切换
nvm use 22
```

### Weapp 构建与部署

```bash
# 开发期监听构建
make mobile-weapp-dev
# 或手动：nvm use 22 && cd frontend-mobile && npm run dev:weapp

# 生产构建
cd frontend-mobile && npm run build:weapp   # → dist-weapp/

# 上传到微信服务器（需私钥）
make weapp-upload VERSION=1.0.0 DESC="release note"

# 全量打包（含 weapp 产物）
make release
```

**注意**：`make weapp-upload` 依赖 `frontend-mobile/private.APPID.key` 私钥文件（已 gitignore）。

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
├── frontend-mobile/         # WeChat mini-program / mobile web (Vite + Taro)
│   └── src/
│       ├── platform/         # 跨端适配层（浏览器 ↔ 小程序）
│       │   ├── browser.js    # 浏览器实现（localStorage, fetch, lucide-react）
│       │   ├── index.js      # 条件导出（TARO_ENV → weapp/browser）
│       │   └── index.weapp.js # 小程序实现（Taro Storage, Taro.request, text-icon）
│       ├── taro-shim.js      # Vite 模式：@tarojs/components → HTML 元素映射
│       ├── pages/            # 页面组件（.jsx，多端共用唯一业务逻辑源）
│       └── utils/            # 通用工具
├── docs/                    # Core documentation (bilingual: zh + en)
└── scripts/                 # Build & CI helper scripts
```

## 跨端代码复用架构（强制工程原则）

> **来源**: #882 — 前端双代码库问题分析与架构决策。

`frontend-mobile` 必须在**一套 `.jsx` 代码**上同时支撑 Vite H5（浏览器）和 Taro weapp（微信小程序）两端的编译运行。**禁止**为两端创建独立的业务逻辑副本。

### 架构模式

```
.jsx 文件 (唯一业务逻辑源)
   │
   ├── 平台 API               → import { storage, navigation, env } from '../platform'
   ├── 图标组件               → import { ArrowLeft, ChevronRight } from '../platform'
   ├── Taro 组件 (<View>等)   → import from '@tarojs/components'
   │                              ├─ Vite mode: taro-shim.js → HTML 元素
   │                              └─ Taro mode: 原生小程序组件
   │
   └── 平台抽象层 (src/platform/)
        ├── browser.js          → localStorage, fetch, lucide-react, window.location
        ├── index.weapp.js      → Taro.getStorageSync, Taro.request, text图标, Taro.navigateTo
        └── index.js            → 条件导出 (process.env.TARO_ENV)
```

### 强制规则

| 规则 | 说明 |
|------|------|
| **禁止** `import ... from 'react-router-dom'` | 用 `import { navigation } from '../platform'` 替代 |
| **禁止** `import ... from 'lucide-react'` | 用 `import { IconName } from '../platform'` 替代 |
| **禁止** 直接 `localStorage` / `fetch` | 用 `import { storage, request } from '../platform'` 替代 |
| `.tsx` 只能做薄壳 | `.tsx` = `export { default } from '../../Xxx'` —— 任何业务逻辑必须写在 `.jsx` 中 |

### 页面入口架构

```
src/pages/
├── Home.jsx                    ← 唯一业务逻辑 (两端共用)
├── home/
│   └── index.tsx               ← 薄壳: export { default } from '../../Home'
├── Profile.jsx                 ← 唯一业务逻辑
├── profile/
│   └── index.tsx               ← 薄壳: export { default } from '../../Profile'
...

app.config.ts                   ← Taro 页面注册 (指向 .tsx 薄壳)
```
---

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

## 🛡️ 前后端集成审核清单 (Frontend-Backend Integration Audit Checklist)

> 统一审核框架，源自 #403（路径重复）、#697（参数名不匹配 + api.get 丢参）、#695（数据一致性审计遗漏）等教训。
> **每次审核新增/修改 API 端点时，必须逐项检查以下 5 个维度。**

### 维度 1：路径一致性 (Path Contract)

| 检查项 | 说明 |
|--------|------|
| 前端调用路径 vs 后端注册路由是否一致 | `api.js` 中的 `/xxx` 必须与 `main.go` 中注册的路径匹配 |
| baseURL 是否导致路径前缀重复 | `API_BASE_URL = '/api'`，前端不应再加 `/api` 前缀（#403 教训） |
| 后端路由是否实际存在 | 前端调用的端点必须能在 `main.go` 中找到对应注册 |

**检查方法：**
```bash
# 提取后端所有路由
grep -E '^\s+\w+\.(GET|POST|PUT|DELETE|PATCH)' backend/main.go

# 提取前端所有 API 调用路径
grep -rn "api\.\(get\|post\|put\|delete\)\|request(" frontend-pc/src/services/api.js | grep -oP "'/[^']*'" | sort -u

# 对比差异
diff <(grep -oP "'/[^']*'" frontend-pc/src/services/api.js | sort -u) <(grep -oP '"\/[^"]*"' backend/main.go | sort -u)
```

### 维度 2：参数名一致性 (Parameter Contract)

| 检查项 | 说明 |
|--------|------|
| Query 参数名：前端发送名 vs 后端 `c.Query()` 读取名 | 如前端发 `?identifier=` 但后端读 `c.Query("phone")` → 400（#697 教训） |
| Body 字段名：前端发送名 vs 后端 `ShouldBindJSON()` 结构体字段名 | 如前端发 `{ amount }` 但后端要求 `{ damage_amount }` → 绑定失败 |
| 分页参数命名约定 | 前端常用 `pageSize`（camelCase），后端常用 `page_size`（snake_case）——必须确认对应关系 |
| 传输层是否实际发送参数 | 确认 `api.get` 等 HTTP 方法函数是否支持并传递了调用方传入的 params（#697 教训：旧 `api.get` 忽略第二个参数） |

**检查方法：**
```bash
# 后端 handler 读取的所有 query param 名
grep -n 'c\.Query\|c\.DefaultQuery' backend/handlers/*.go | grep -oP 'Query\("([^"]+)"' | sort -u

# 前端发送的所有 query param 名
grep -rn 'params:' frontend-pc/src/ --include='*.js' --include='*.jsx' | grep -oP '(\w+):' | sort -u

# 后端结构体绑定字段
grep -n 'ShouldBindJSON\|ShouldBind\|json:"' backend/handlers/*.go | head -40
```

### 维度 3：响应访问模式 (Response Contract)

| 检查项 | 说明 |
|--------|------|
| 前端访问 `response.code` vs `response.data.code` | `request()` 返回完整 JSON `{ code, data, message }`，应使用 `response.code` |
| 前端访问 `response.data.xxx` vs `response.xxx` | 内层数据在 `response.data` 中，列表应为 `response.data.list` |
| 前端检查的成功码是否与后端一致 | 后端成功返回 `code: 20000`，前端不应检查 `=== 20100` |

**标准响应访问模式：**
```js
const result = await staffApi.list(params)
if (result.code === 20000) {        // ✅ 正确：result 是完整响应
  const list = result.data?.list     // ✅ 正确：data 是内层数据
  const total = result.data?.total
}
```
```js
if (result.data.code === 20000) {   // ❌ 错误：多了一层 data
  const list = result.data.data?.list // ❌ 错误：应该是 result.data.list
}
```

### 维度 4：数据源权威性 (Data Source Authority)

| 检查项 | 说明 |
|--------|------|
| 用户/权限操作是否以 IAM 为准 | 本地 DB 仅是缓存，操作顺序：先调 IAM → 再更新本地（#685 教训） |
| 数据隔离是否使用 JWT claims | 禁止用 `c.GetString("tenant_id")`，必须用 `middleware.GetTenantID(ctx)`（#688 教训） |
| Update 操作是否显式加 WHERE | GORM Update 回调未注册自动 scoping，必须手动加 `WHERE tenant_id = ?`（#688 教训） |

### 维度 5：环境一致性 (Environment Contract)

| 检查项 | 说明 |
|--------|------|
| Token 签发方与验证方是否同一实例 | 两套 beaconiam 有独立 RSA 密钥对，混用必报 `crypto/rsa: verification error`（#JWT 教训） |
| `BEACONIAM_INTERNAL_URL` 是否指向正确的实例 | Dev=5561, Prerelease=5560，不可混用 |
| 前端 baseURL 是否与当前环境匹配 | Dev: `/api` → Vite proxy → 5557, Prerelease: 不同的代理配置 |

---

### 历史案例索引 (Historical Incident Reference)

| 日期 | Issue | 类型 | 维度 | 一句话描述 |
|------|-------|------|------|-----------|
| 2026-04-30 | #403 | 路径重复 | 1-路径 | 前端 `baseURL='/api'` + 方法内又写 `/api/` → `/api/api/xxx` |
| 2026-05-06 | JWT | 密钥不匹配 | 5-环境 | Dev/Prerelease 各自 RSA 密钥，token 混用 → verification error |
| 2026-05-23 | #685 | 数据源 | 4-数据源 | 本地 DB 当权威源，IAM 侧无数据 → 用户无法访问 |
| 2026-05-29 | #688 | 数据隔离 | 4-数据源 | `c.GetString` 返回空 + GORM Update 无 scoping → 跨租户泄漏 |
| 2026-05-30 | #695 | 审计盲区 | 2-参数 | 数据一致性审计只查字段值，未查 API 参数名 → 漏掉 #697 |
| 2026-05-30 | #697 | 参数名+传输层 | 2-参数 | `api.get` 忽略第二参数 + `identifier` ≠ `phone`/`email` → 400 |

### 2026-05-06 JWT 密钥不匹配 — 检查方法备忘

```bash
# 对比各 beaconiam 实例公钥
curl -s http://localhost:5560/api/v1/auth/public-key.pem | sha256sum
curl -s http://localhost:5561/api/v1/auth/public-key.pem | sha256sum

# 解码 token 查看签发方
echo "TOKEN" | cut -d. -f2 | base64 -d 2>/dev/null | jq '.iss'

# 确认后端配置
grep "BEACONIAM_INTERNAL_URL" /home/coder/tuneloop/.env
```

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

> **平台级资源声明**: Categories, Properties, and PropertyOptions are **platform-level shared resources** — readable by all authenticated users, writable only by namespace_admin (cus_perm: `category:manage`, `attribute:manage`). The `tenant_id` on these models stores creator metadata, NOT access scope.

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

**严禁**：AI 不得自行修改生产服（prerelease/production）的数据和数据库定义（DDL）。
- 包括但不限于 `ALTER TABLE`、`CREATE TABLE`、`DROP TABLE`、`INSERT`、`UPDATE`、`DELETE` 等。
- 即使 SSH 可达，也**必须先与用户确认**，由用户决定是否执行。
- 此规则同时适用于开发环境的数据库，修改前也需确认。

---

## 🧪 调试效率经验 (Debugging Efficiency Lessons)

### ⚠️ 强制规则：排查任何 Issue 必须先检查后端日志

接收到任何排查请求时（无论表面是前端问题、API 错误、还是数据异常），**第一步永远是检查后端日志**：

```bash
tail -100 backend/backend.log
```

**理由**：
- Go 后端错误（SQLSTATE、panic、nil dereference 等）只在日志中可见，前端只能看到 HTTP 状态码和通用错误信息
- 本次 #699 事故：前端问题描述看似纯前端（40900 未提示 + 输入时去重缺 username），实际根因是后端 `column "username" does not exist` 导致 500 错误，而前端没有收到预期响应
- 如果第一时间看了 `backend.log`，SQLSTATE 42703 错误一目了然，不会浪费时间去追溯前端代码

> 日志文件路径：`backend/backend.log`（`make run` 通过 `tee backend.log` 写入）。
> 如果日志文件不存在（例如用其他方式启动），检查 `make run` 启动的终端输出，或用 `docker logs` 查看容器日志。

---

> 来自 #569 — 商户创建流程调试（15 个 Bug，11 个回合）

### Pre-Work: 追踪完整数据流

在写任何代码前，画完整路径：
```
请求 → 中间件链 → 处理器 → 外部调用(IAM) → DB 写入 → 响应
```
逐层检查每一环的条件判断，尤其是角色/权限检查。

### Ghost Code Audit（幽灵代码审计）

搜索并审计所有运行时可能永假的判断：
- `RequireRole("xxx")` — 角色名在 IAM 中真实存在吗？
- `if GetBusinessRole(ctx) != BusinessRoleXxx` — `BusinessRoleXxx` 有代码路径能返回吗？
- `if condition1; if condition2` — 两者是互斥还是顺序执行？

### Cross-Repo Boundary（跨仓库边界）

- **严禁**在未经确认的情况下修改 beaconiam 或其他外部仓库代码
- 先在目标仓库建 Issue 描述问题，等待该侧分析
- 跨仓库调试的 99% 时间消耗在"这是谁的问题"的判断上 — 先确认归属

### Environment Parity（环境差异显式化）

记录 DEV vs PRERELEASE 的关键差异：
- `IAM_SECRET` 是否真实值？
- OAuth App 是否已通过 `ActivateNamespace` 注册？
- 数据库状态：冷启动是否已完成？tenant 表是否有记录？

### Integration Test for Critical Paths（关键路径集成测试）

商户创建、OAuth 登录等关键流程必须有端到端测试：
- Go: `go test -run TestCreateMerchant_FullFlow`
- 验证：请求 → 响应码 → DB 记录 → IAM 侧数据一致

### "Check the Neighbors" Rule（邻居检查）

发现一个 Bug 后，搜索同样模式在代码库中是否重复出现。
例：发现 `project_admin` 是虚构角色 → 审计所有 `RequireRole` 调用。

### 2026-05-23: 用户账户操作必须以 IAM 为准，本地库仅是缓存

**问题现象：**
网点管理中添加/切换/移除成员后，本地 `site_members` 表正确更新，但 IAM 侧的 `user_org_relations` 始终为空。用户登录后 JWT 的 `oid`/`tid` 为空字符串，无法访问任何资源。

同时影响 `CreateSite`、`UpdateSite`、`UpdateMemberRole`、`RemoveMember` 四个 handler——它们都只更新了本地数据库，未调用 IAM 的 BindUser/UnbindUser/UpdateUserRole。

**根因：**
Tuneloop 将本地数据库当作真实数据源。但用户账户和权限相关的操作必须以 **IAM 为准**。本地 `users`、`site_members` 等表只是 IAM 数据的本地缓存快照，不应作为操作目标。

**原则：**
```
用户账户相关操作：
  1. 首先调用 IAM API 完成绑定/解绑/角色变更
  2. IAM 成功后，同步更新本地缓存
  3. 本地缓存仅用于前端展示加速，不用于权限判断
```

**以后审核新增 Handler 时必须检查：**
1. [ ] 是否涉及用户账户/权限/绑定操作？
2. [ ] 如果是，是否调用了对应的 IAM API（BindUserToOrganization / UnbindUser / UpdateUserRoleInOrg）？
3. [ ] IAM 调用是否在本地 DB 更新**之前**执行？
4. [ ] 是否把本地 DB 当作唯一数据源（应该以 IAM 为准）？

**检查方法：**
```bash
# 查找所有直接更新 site_members / users 表但未调 IAM 的地方
grep -n 'db.Model\|db.Create\|db.Update' handlers/site_member.go | while read line; do
  echo "Check if IAM call precedes this line: $line"
done
```

#### 2026-05-28: 本地仅缓存，真实数据源是 IAM（#685）

**原则**：所有用户账户、权限、角色绑定相关操作必须以 IAM 为准，本地 DB（`users`、`site_members`、`roles` 等）仅为缓存加速。

**操作顺序**：
```
1. 先调 IAM API 完成绑定/解绑/授权/角色变更
2. IAM 成功后，同步更新本地缓存
3. 本地缓存仅用于前端展示加速，不用于权限判断
```

**检查清单**（审核新增 Handler 时必查）：
- [ ] 是否涉及用户账户/权限/绑定操作？
- [ ] 如果涉及，是否调用了对应的 IAM API（`BindUserToOrganization` / `AssignRoleTemplateToUserWithToken` / `SetUserCustomerPermissions`）？
- [ ] IAM 调用是否在本地 DB 更新**之前**执行？
- [ ] 是否把本地 DB 当作唯一数据源（禁止，应以 IAM 为准）？

**涉及的操作**：
- `CreateMerchant` — 创建商户后必须初始化角色（#663 已修）
- `CreateSite` — 创建网点后必须绑定管理员
- `AddMember` — 添加成员后必须绑定 + 分配角色模板（#685）
- `UpdateMemberRole` — 变更角色后必须在 IAM 侧同步
- `RemoveMember` — 移除成员后必须在 IAM 侧解绑

---

#### 2026-05-29: 数据隔离系统性漏洞 — 全系统 137 Handler 审计发现 44 处隔离缺失（#688）

**问题**：site_admin 可跨网点访问其他网点的乐器和人员数据。

**根因分类**：

| 根因 | 影响 | 典型位置 |
|------|:--:|------|
| `c.GetString("tenant_id")` 始终返回空字符串 | 17 处 | `lease.go`, `maintenance.go`, `label.go`, `admin.go` |
| GORM Update 未注册自动 scoping 回调 | 15+ 处 | `instrument.go`, `warehouse.go`, `maintenance.go` |
| 完全无 tenant/org 过滤 | 10 处 | `outbound.go`, `assessment.go`, `inventory.go` |
| 依赖本地 `users` 缓存做 scoping 决策 | 2 处 | `api.go:GetInstruments`, `user_staff.go:ListStaff` |

**隔离规则（强制）**：

| 角色 | 可见范围 | 实现方式 |
|------|---------|---------|
| namespace_admin | namespace 内所有租户 | `GetVisibleOrgIDs` + `ApplyOrgScope` |
| merchant_admin | 本商户全部网点 | JWT `tid` 过滤 |
| site_admin | 本网点 + 下级网点 | JWT `oid` 过滤 |
| site_member | 本网点 | JWT `oid` 过滤 |
| worker | 本网点 | JWT `oid` 过滤 |
| customer (USER) | 无组织/网点边界 | JWT 中 `oid`/`tid` 为空；操作订单时从 instrument 推导 merchant/site |

**审查新增 Handler 时必须检查**：

1. [ ] DB 实例是否调用 `.WithContext(ctx)`？（否则自动 scoping 被绕过）
2. [ ] C(R)UD 操作是否包含 `tenant_id` / `org_id` / `site_id` WHERE 子句？
3. [ ] 是否使用了 `c.GetString("tenant_id")`？（**禁止** — 必须用 `middleware.GetTenantID(ctx)`）
4. [ ] 是否使用了 `GetVisibleOrgIDs()` 或 `ApplyOrgScope()` 进行 org 级隔离？
5. [ ] Update 操作是否显式加了 `WHERE tenant_id = ?`？（GORM Update 回调未注册自动 scoping）
6. [ ] 是否依赖本地 `users` 表的 `SiteID`/`Role` 做 scoping 决策？（**禁止** — 必须用 JWT claims）
7. [ ] 如果处理 instrument，operate 后是否保持了 `tenant_id`/`org_id` 不变？
8. [ ] 是否允许 customer 角色访问？若是，路由是否在 `userOptionalAuth` 组？handler 是否从 instrument/order 推导 tenant/org？（#833 教训）

**公共路由豁免**：`/api/public/*` 路由有意跨租户暴露数据——如需限制，需在 Issue 中独立提出。

**Customer 路由规则（强制）**：Customer (USER) 角色没有组织/租户绑定（JWT 中 `oid`/`tid` 为空字符串）。任何 customer 可调用的订单操作接口（支付、取消、确认收货等）必须：
1. 注册在 `userOptionalAuth` 路由组（使用 `OptionalIAMInterceptor`，不强制要求 tid）
2. Handler 中通过 order → instrument 推导 `tenant_id`/`org_id`（参考 `user_rental.go:214-224` 模式）
3. **严禁**注册在 `authRequired` 组 — 该组使用 `IAMInterceptor`，空 tid 会触发 40104（#833 教训）

---

### 动态属性设计原则

> 动态属性输入框允许手动输入新值。新值自动写入 `property_options` 表（`status='pending'`）。网点经理在属性管理页可看到各属性下的新增值，可以：
> - **接受**：将 pending 值改为 active
> - **归并**：设为已有值的别名（如 "YAMAHA" → "雅马哈"）
> - **修正**：直接修改 pending 值（修正 typo）

---

> *Last updated: 2026-06-10*

---

## 🚨 IAM 问题处理原则

### 如果问题可能出在 IAM 侧，不要在 Tuneloop 侧硬改

当遇到以下情况时，**应先在 beaconiam 仓库创建 Issue 要求协助调查**，而非在 tuneloop 侧做 workaround：

- IAM API 返回 `access denied`、`unauthorized` 等权限/认证错误
- JWT token 签发或验证相关问题
- IAM 用户数据（name/email/phone）与实际不符或无法获取
- OAuth 流程异常（code exchange、redirect_uri 等）

**操作步骤**：
1. 在 `https://github.com/HiJohns/beaconiam` 创建 Issue，描述调用方（tuneloop）的请求详情（URL、token、响应）
2. 附上对应的 tuneloop Issue 链接
3. 等待 beaconiam 侧分析后给出结论或修复方案
4. 在 tuneloop 侧仅配合调整调用方式，不做逻辑替代

---

> *Last updated: 2026-06-10*

---

## 📋 Phase 0: 页面审计 (Page Audit)

> **目的**：在对一个页面/场景做任何修改之前，先对比 `cases.md` → `ui.md` → 代码，发现设计缺失和实现缺失。

### 触发条件

当用户说"分析一下 XX 页面"、"XX 页面应该有哪些功能"、"对比 cases.md 和 XX 页面"时，执行以下流程。

### 执行步骤

#### Step 1: 从 `cases.md` 推导页面契约

从 cases.md 中找到与目标页面相关的所有段落，提取以下信息：

| 维度 | 要提取的内容 | 示例 |
|------|-------------|------|
| 目标用户 | 谁能进入这个页面 | "员工在PC端登录" → 角色=STAFF |
| 前置条件 | 进入页面时数据应处于什么状态 | "归还的乐器到货后" → order.status=returning |
| 信息展示 | 用户能看到什么数据 | "乐器相关信息" → 乐器SN/分类/网点 |
| 操作 | 用户可以做什么 | "点击定损，输入评论，金额" |
| 权限 | 什么角色能做什么操作 | "网点员工" → site_admin/site_member |
| 操作结果 | 操作后数据如何变化 | "乐器进入维修状态" → stock_status=maintenance |
| API | 操作对应什么后端调用 | "定损" → PUT /api/warehouse/orders/:id/return-inspect |
| 导航 | 从哪里来，到哪里去 | "从订单详情点击收货" → /staff/receiving |

#### Step 2: 对比 `docs/ui.md`

将 Step 1 的推导结果与 `docs/ui.md` 中对应该页面的描述逐项对比。

| 检查项 | 方法 |
|--------|------|
| 页面是否存在 | ui.md 中是否有该页面的描述章节 |
| 路由是否正确 | ui.md 中声明的路由是否与 App.jsx 一致 |
| 权限声明是否完整 | ui.md 中的权限声明是否覆盖 cases.md 的角色要求 |
| 功能描述是否覆盖 | ui.md 是否列出了 cases.md 要求的所有信息和操作 |

**发现设计缺失时→输出关键结论，等待用户确认才建议改 ui.md。**

#### Step 3: 对比代码

将 Step 1+2 的结论与前端代码逐项对比。

| 检查项 | 方法 |
|--------|------|
| 路由权限门控 | 页面组件是否有权限检查（`businessRole`、`has()`） |
| 信息展示 | 页面 JSX 中是否渲染了所需的字段 |
| 操作按钮 | 按钮是否按状态/权限条件渲染 |
| API 调用 | 调用的 endpoint、method、body 是否与后端一致 |
| 导航 | 按钮/链接的 `navigate()` 目标是否正确 |

#### Step 4: 输出 Page Audit Card

使用以下模板输出审计卡片：

```markdown
## Page Audit Card: {场景名称}

来源: cases.md:XXX-XXX

| 需求 | cases.md | ui.md | 代码 | 差距 |
|------|----------|-------|------|------|
| {信息/操作} | ✅ LXXX | ✅/❌ | ✅/❌ | 设计/实现/— |

### 关键发现
- {设计缺失：ui.md 未覆盖的}
- {实现缺失：代码未实现的}
- {设计冲突：ui.md 与 cases.md 矛盾的}
- {权限不一致：代码权限与声明的差异}
```

### 示例

```
## Page Audit Card: 归还验收

来源: cases.md:612-648

| 需求 | cases.md | ui.md | 代码 | 差距 |
|------|----------|-------|------|------|
| 乐器信息 | ✅ L624 | ✅ §3.19 | ✅ StaffReceive | — |
| 租赁信息 | ✅ L624 | ✅ | ✅ | — |
| 出库照片对比 | ✅ L619 | ❌ | ❌(修复前) | 设计+实现缺失 |
| 拍照规格要求 | ✅ L615 | ❌ | ❌(修复前) | 设计+实现缺失 |
| 无损验收 | ✅ L626 | ✅ | ✅ | — |
| 定损 | ✅ L629 | ✅ | ✅ | — |
| 角色 | 员工 | 员工 | site_admin/member | 一致 |

### 关键发现
- ui.md §3.19 对出库照片对比和拍照规格描述不足
- StaffReceiveConfirm 实现时缺少这两个功能
```

### 注意

- **只分析，不修改代码**。发现差距后先报告，等用户决策。
- 分析完成后应提供明确的建议："是否将此差距创建为 Issue？"
- 如果 ui.md 完全缺失某页面，应在报告中明确标注"设计缺失"而非"实现缺失"。


## 🎨 移动端 UI 开发方法论 (Mobile UI Development Methodology)

> 来源：首页 UI 多轮迭代（Home.jsx），横跨字体尺寸、搜索框、走马灯、Z 轴布局、配色。

### 方法论 1：精确值 → 视觉反馈闭环 (Exact-Spec → Visual Feedback Loop)

**模式**：用户给出像素级规格 → 实现 → 视觉对比 → 逐轮微调。

**适用场景**：字体尺寸、间距、边框等 UI 精调。

**操作原则**：
1. 先落地用户给出的精确值（如 `w-[300px]`、`text-[42px]`）
2. 在真实渲染环境中对比后，再按"相对变化"微调（如 +2px、再大一个字号、`py-2` → `py-[3px]`）
3. **禁止**在首轮就做"看起来更好"的主观调整 —— 先按规格来，再迭代

**反面案例**：用户给 `text-[42px]`，先落地，再改为 `text-2xl +2px` → `text-[26px]` → `text-3xl`。

### 方法论 2：根因分析法（不修补表象）(Root Cause Over Patchwork)

**模式**：标准方案失败 → 不换组件参数 → 先检查数据/尺寸/层叠关系。

**典型案例** — 分类菜单水平拖动失效：
1. ❌ 先试 `catchMove` on ScrollView → 无效
2. ❌ 将菜单移出外层 ScrollView → 破坏垂直滚动
3. ✅ **发现根因：菜单内容超宽，导致页面级水平滚动** → 用 `overflow-hidden` + touch 手动 `translateX` 解决

**检查清单**（Taro/微信小程序 ScrollView 问题排查）：
- [ ] 数据是否超出容器宽度？用 `overflow-hidden` 剪裁
- [ ] 是否嵌套 ScrollView？优先用 touch 事件 + `transform` 替代内层 ScrollView
- [ ] `catchMove` 是否真的在运行时生效？（可能被平台忽略）

### 方法论 3：Z 轴图层分解 (Z-Axis Layer Decomposition)

**模式**：将页面从"垂直堆叠"改为"背景层 + 透明内容层"。

**要点**：

| 层 | 定位 | 职责 | 背景 |
|-----|------|------|------|
| Z=0 | `fixed` 全屏 | 走马灯/背景装饰 | 有颜色/图片 |
| Z=10 | `relative` 可滚动 | 交互内容 | **透明** |
| (局部) | 卡片/菜单 | 信息容器 | 保留原有底色 |

**适用信号**：
- 设计意图是"背景变化 + 内容独立滚动"
- 多个 section 共享同一个背景图/色
- 需要走马灯覆盖全屏而非仅某个区域

**实现模板**：
```jsx
<View className="relative">
  {/* Z=0: fixed full-screen background */}
  <View className="fixed inset-0 z-0">{carousel}</View>
  {/* Z=10: scrollable transparent content */}
  <ScrollView className="relative z-10">{content}</ScrollView>
</View>
```

### 方法论 4：平台约束显式化 (Platform Constraint Awareness)

**原则**：在写小程序 UI 变更前，明确了解以下组件原生行为边界：

| 平台行为 | 影响 | 应对 |
|---------|------|------|
| `ScrollView scrollY` 不裁切水平溢出 | 超宽内容必拖页 | 外层加 `overflow-hidden` |
| `fixed` 定位在 ScrollView 内可能异常 | 不能依赖嵌套 fixed | 将 fixed 层放到 ScrollView 外层 |
| `catchMove` 可能被运行时忽略 | 不能作为唯一防线 | 配合 `overflow-hidden` + 手动 scroll |

### 方法论 5：配色增量迭代 (Incremental Color Palette Design)

**流程**：同色系（保守）→ 视觉对比判断 → 不够 → 跨色系（激进）。

**案例**：
1. 首轮：`#915F38`（赭石）/ `#A6794E`（琥珀）→ 太接近
2. 第二轮：放大差异，引入跨色系的 `#4A6B7C`（钢蓝）→ 对比明确

**原则**：先在同 hue 内调整 saturation/lightness，视觉对比不足时再换 hue。

### 方法论 6：Record Keeper 模式 (Record Keeper Pattern)

**原则**：多轮交互完成后，立即用 Record Keeper 流程将修改正式记录为 GitHub Issue + Commit 绑定。

**触发条件**：满足以下即应创建 Record Issue：
- 与 AI 多轮交互后问题已解决但未事先创建 Issue
- 进行的修改跨越多个独立 topic（如字体调整、布局重构、配色迭代）
- 需要与一个 Issue 绑定以通过审计流程

**流程**：
1. 收集工作摘要（改动范围 + 调查过程 + 解决方案）
2. 创建 Issue（`opencode_gh.sh record`，`status:ready`）
3. Commit + Push（绑定 `Closes #N`）
4. Comment commit hash 到 Issue


> *Last updated: 2026-06-15*

