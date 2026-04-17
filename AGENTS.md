# AGENTS.md

This file contains instructions and guidelines for AI coding agents working in this repository.

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

## Code Style Guidelines

### General Principles
- Write clean, readable, and maintainable code
- Follow existing patterns in the codebase
- Keep functions small and focused (single responsibility)
- Use descriptive variable and function names
- Add tests for new features and bug fixes

### TypeScript/JavaScript
- **Prefer TypeScript** for new files when available
- Use explicit types over `any` or implicit typing
- Use `interface` for object shapes that might be extended
- Use `type` for unions, intersections, and primitive types
- Enable strict mode in `tsconfig.json`

```typescript
// Good
interface User {
  id: string;
  name: string;
  email: string;
}

// Avoid
const user: any = { id: '1', name: 'John' };
```

### Imports & Exports
- **JavaScript/TypeScript**: Use ES modules (`import/export`)
- **Order imports**: External libraries → Internal modules → Relative imports
- **Group imports**: React imports, then other libraries, then local files
- Use absolute imports if configured (`@/components/...`)

```typescript
import React from 'react';
import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { User } from './types';
```

### Naming Conventions
- **Files**: `kebab-case.ts` for utilities, `PascalCase.tsx` for components
- **Components**: `PascalCase` (e.g., `UserProfile.tsx`)
- **Functions**: `camelCase` (e.g., `getUserData()`)
- **Constants**: `UPPER_SNAKE_CASE` (e.g., `API_BASE_URL`)
- **Types/Interfaces**: `PascalCase` (e.g., `UserData`)
- **Hooks**: Start with `use` (e.g., `useAuth()`)
- **Boolean variables**: Start with `is/has/should` (e.g., `isLoading`, `hasPermission`)

### Functions & Methods
- Keep functions pure when possible
- Use arrow functions for inline callbacks
- For React components, use function declarations or arrow functions
- Add JSDoc comments for complex functions

```typescript
/**
 * Calculates the total price with tax
 * @param amount - The base amount
 * @param taxRate - The tax rate as a decimal
 * @returns The total amount with tax
 */
function calculateTotal(amount: number, taxRate: number): number {
  return amount * (1 + taxRate);
}
```

### Error Handling
- Use try-catch for async operations and error-prone code
- Provide meaningful error messages
- Log errors appropriately (avoid console.log in production)
- Handle edge cases and validation

```typescript
try {
  const data = await fetchUserData();
} catch (error) {
  console.error('Failed to fetch user data:', error);
  throw new Error('Unable to load user information');
}
```

### Formatting
- Use the existing formatter (Prettier is common)
- Consistent indentation (2 spaces is standard)
- Semicolons at the end of statements
- Single quotes for strings
- Trailing commas in multiline structures

### React Guidelines
- Use functional components with hooks
- Follow React naming conventions (`PascalCase` for components, `camelCase` for props)
- Keep components small and composable
- Use `useCallback` and `useMemo` for performance optimization when needed
- Manage state at the appropriate level (lift state up when needed)
- Use `key` prop when rendering lists
- Prefer composition over inheritance

```tsx
// Component naming
function UserCard({ user, onEdit }: UserCardProps) {
  return <div>{user.name}</div>;
}

// Hooks naming
function useAuth() {
  const [user, setUser] = useState<User | null>(null);
  // ...
}
```

### Testing
- Write unit tests for utilities and business logic
- Write integration tests for API calls and data flow
- Use descriptive test names
- Follow AAA pattern (Arrange, Act, Assert)

```typescript
describe('calculateTotal', () => {
  it('should calculate total with tax', () => {
    // Arrange
    const amount = 100;
    const taxRate = 0.1;
    
    // Act
    const result = calculateTotal(amount, taxRate);
    
    // Assert
    expect(result).toBe(110);
  });
});
```

### Git Workflow
- Create feature branches for new work
- Write clear commit messages
- Keep commits focused and atomic
- Review code before merging
- Update this file when project-specific conventions are established

## Learning More
As the codebase grows, this file should be updated with:
- Specific commands from `package.json` scripts
- Project-specific architectural decisions
- Component library conventions
- API patterns and data fetching strategies
- State management patterns
- Styling conventions (CSS modules, styled-components, etc.)

## Frontend Navigation Structure

### PC端侧边栏菜单 (Last Update: 2026-04-17, Commit: $COMMIT_HASH)

#### 菜单层级

**1. 仪表盘 (Dashboard)**
- **路由**: `/`
- **权限**: 所有已登录用户
- **说明**: 系统首页

**2. 乐器管理**
- **路由**: 一级菜单
- **权限**: 所有已登录用户
- **子菜单**:
  - 乐器列表 (`/instruments/list`)
  - 分类设置 (`/instruments/categories`)
  - 属性管理 (`/instruments/properties`)

**3. 库存监控** ⭐
- **路由**: 一级菜单
- **权限**: site_manager, admin, owner
- **子菜单**:
  - 库存调拨 (`/inventory/transfer`)
  - 租金设定 (`/inventory/rent-setting`)

**4. 组织管理**
- **路由**: 一级菜单
- **权限**: 所有已登录用户
- **子菜单**:
  - 网点管理 (`/organization/sites`)

**5. 系统管理**
- **路由**: 一级菜单
- **权限**: 所有已登录用户
- **子菜单**:
  - 客户端管理 (`/system/clients`)
  - 租户管理 (`/system/tenants`)

#### 权限控制汇总

| 菜单 | site_manager | admin | owner | 普通用户 |
|------|-------------|-------|-------|---------|
| 仪表盘 | ✅ | ✅ | ✅ | ✅ |
| 乐器管理 | ✅ | ✅ | ✅ | ✅ |
| 库存监控 | ✅ | ✅ | ✅ | ❌ |
| 组织管理 | ✅ | ✅ | ✅ | ✅ |
| 系统管理 | ✅ | ✅ | ✅ | ✅ |

#### 右上角用户信息

**显示格式**: 👤 **{name}** (**{role}**)

**数据来源**（优先级）:
1. JWT token payload（优先）
   - name: name, username, preferred_username, displayName, nickName, nickname
   - email: email, mail
   - role: role, roles, authorities
2. localStorage fallback (`user_info`)

**面包屑导航**:
- TuneLoop: 可点击，返回首页
- 乐器管理: 可点击，返回首页（在相关页面）

#### 关键代码位置

- **主布局**: `frontend-pc/src/App.jsx::MainLayout()`
- **用户加载**: lines 66-99 (useEffect)
- **菜单定义**: lines 101-129 (items)
- **面包屑**: lines 146-169 (breadcrumbItems)
- **用户显示**: lines 175-183 (Header)

#### 权限判断逻辑

```javascript
// 库存菜单可见性 (lines 117-130)
const role = userInfo.role || ''
const shouldShow = role === 'site_manager' || role === 'admin' || role === 'owner'
```

支持的角色值: `site_manager`, `admin`, `owner`

#### 最后更新记录

- **日期**: 2026-04-17
- **Commit**: $COMMIT_HASH
- **修复内容**:
  - ✅ 中文用户名显示（JWT token 多字段支持）
  - ✅ 面包屑导航点击事件修复
  - ✅ OWNER 角色库存菜单可见性
  - ✅ 添加调试日志

#### 构建与部署

```bash
# 拉取最新代码
git pull origin main

# 构建前端
cd frontend-pc && npm install && npm run build

# 验证
cd frontend-pc && npm run build  # 应该成功
```

**注意**: 用户必须重新登录才能看到效果，因为 userInfo 从登录时的 JWT token 加载。

--------------------------------------------------------------------------------

