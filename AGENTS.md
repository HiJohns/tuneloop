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

## Environment Guide

See `prompts/instructions.md` for full port mapping. Key rule:

**默认上下文是开发环境 (Dev)，不是预生产 (Prerelease)。**

| 环境 | beaconiam | tuneloop PC | tuneloop Backend |
|------|-----------|-------------|------------------|
| Dev | 5552 (Vite) / 5561 (API) | 5554 (Vite) / 5557 (Go) | `BEACONIAM_INTERNAL_URL=http://localhost:5561` |
| Prerelease | 5560 (NGINX) | 5558 (Go) | `BEACONIAM_INTERNAL_URL=http://localhost:5560` |

- 除非用户明确指明"预生产"，否则所有讨论、调试、Issue 都指 Dev 环境
- 两套环境有独立的 beaconiam 实例、独立的 RSA 密钥对、独立的数据库
- Never mix: 预生产的 token 在开发环境验证必报 `crypto/rsa: verification error`

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

**审查新增 Handler 时必须检查**：

1. [ ] DB 实例是否调用 `.WithContext(ctx)`？（否则自动 scoping 被绕过）
2. [ ] C(R)UD 操作是否包含 `tenant_id` / `org_id` / `site_id` WHERE 子句？
3. [ ] 是否使用了 `c.GetString("tenant_id")`？（**禁止** — 必须用 `middleware.GetTenantID(ctx)`）
4. [ ] 是否使用了 `GetVisibleOrgIDs()` 或 `ApplyOrgScope()` 进行 org 级隔离？
5. [ ] Update 操作是否显式加了 `WHERE tenant_id = ?`？（GORM Update 回调未注册自动 scoping）
6. [ ] 是否依赖本地 `users` 表的 `SiteID`/`Role` 做 scoping 决策？（**禁止** — 必须用 JWT claims）
7. [ ] 如果处理 instrument，operate 后是否保持了 `tenant_id`/`org_id` 不变？

**公共路由豁免**：`/api/public/*` 路由有意跨租户暴露数据——如需限制，需在 Issue 中独立提出。

---

### 动态属性设计原则

> 动态属性输入框允许手动输入新值。新值自动写入 `property_options` 表（`status='pending'`）。网点经理在属性管理页可看到各属性下的新增值，可以：
> - **接受**：将 pending 值改为 active
> - **归并**：设为已有值的别名（如 "YAMAHA" → "雅马哈"）
> - **修正**：直接修改 pending 值（修正 typo）

---

*Last updated: 2026-05-30*

