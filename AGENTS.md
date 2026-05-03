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

---

*Last updated: 2026-05-03*

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

