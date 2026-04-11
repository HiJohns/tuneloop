# 环境变量规范 / Environment Variables Specification

## 命名约定

本项目遵循以下环境变量命名约定：

### 数据库配置 (PostgreSQL)
| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `POSTGRES_HOST` | 数据库主机地址 | `localhost` |
| `POSTGRES_PORT` | 数据库端口 | `5432` |
| `POSTGRES_USER` | 数据库用户名 | `tuneloop` |
| `POSTGRES_PASSWORD` | 数据库密码 | - |
| `TUNELOOP_DB` | 数据库名称 | `tuneloop` |
| `DB_SSLMODE` | SSL 模式 | `disable` |

### Beacon IAM 配置
| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `BEACONIAM_EXTERNAL_URL` | 前端 IAM 跳转地址 | - |
| `BEACONIAM_INTERNAL_URL` | 内部 IAM 调用地址 | - |
| `IAM_CLIENT_ID` | IAM 客户端 ID | - |
| `IAM_CLIENT_SECRET` | IAM 客户端密钥 | - |

### 服务地址配置
| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `TUNELOOP_WWW_URL` | PC Web 服务地址 | `http://localhost:5554` |
| `TUNELOOP_WX_URL` | 微信小程序服务地址 | `http://localhost:5553` |

### 文件上传配置
| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `UPLOAD_MAX_SIZE` | 最大文件大小（MB） | `10` |

## 配置优先级

1. 新变量名优先：`POSTGRES_*`, `BEACONIAM_*`, `TUNELOOP_WWW_URL`
2. 旧变量名兼容：`DB_HOST`, `IAM_URL` (保留向后兼容)

## 12-Factor App 合规

所有配置均通过环境变量注入，代码中无硬编码配置。

## 任务完成后验证清单 (Post-Task Verification)

每个任务标记为 `status:ready` 前，必须完成以下验证：

### 通用验证
- [ ] 代码可通过 `go build` (Golang) 或 `npm run build` (JavaScript) 编译
- [ ] 如涉及 API 变更，使用 `curl` 或 Postman 测试端点返回预期结果
- [ ] 对于 Bug 修复，复现原始问题并验证已解决

### 认证/授权相关任务（关键）
- [ ] **完整登录流程测试**：从 IAM OAuth 登录到所有受保护页面访问
- [ ] 验证 token 解析：检查 `[DEBUG Callback] Setting cookies for tenant: <实际tenant_id>` 日志
- [ ] 验证 cookie 设置：浏览器 developer tools 中可见 `token` 和 `refresh_token`
- [ ] **JWT token 完整性**：确保前端能正确读取包含多个 `=` padding 字符的 token
- [ ] 测试 token 过期场景：登录后等待 token 过期，验证自动刷新或重定向
- [ ] 后端 middleware 日志应显示 tenant_id 已正确设置（无空值）

### 前端相关任务
- [ ] 运行构建命令（`npm run build`）无语法错误
- [ ] 在隐身模式/清空缓存后测试页面加载和功能
- [ ] 验证所有修改的文件中的类似代码模式已同步修复

### 文档要求
- [ ] 新增环境变量已记录在本文件的「环境变量规范」章节
- [ ] 接口变更已更新到 `docs/api.md` (如存在)
- [ ] 复杂功能添加简短说明到本文件的「功能说明」章节

**特别警告**：OAuth、token 处理、权限相关的任务，必须在真实登录场景下完整测试后才能标记完成。
## 🔐 Authentication Error Handling

### Common Error Codes

| Code | Description | Frontend Action |
|------|-------------|-----------------|
| 40101 | Token expired | Auto refresh token, then logout if refresh fails |
| 401 | Unauthorized | Clear tokens and redirect to IAM login |

### Implementation Requirements

All API calls MUST handle these error codes:

#### Frontend (frontend-pc/src/services/api.js)
- Check HTTP status 401
- Check response body `code: 40101`
- Try token refresh once
- If refresh fails, clear tokens and redirect to IAM

#### Backend Middleware
- Return consistent error format:
  ```json
  {"code": 40101, "message": "token expired"}
  ```

### Code Checklist

When adding new API calls:
- [ ] Check if response.status === 401
- [ ] Check if data.code === 40101
- [ ] Handle token refresh
- [ ] Clear tokens on failure
- [ ] Redirect to IAM login

## 🚫 Frontend API Call Rules

### Mandatory API Module Usage

**All backend API calls MUST use the centralized `api.js` module. Direct use of `fetch` is PROHIBITED.**

#### Why This Rule Exists
- The `api.js` module provides unified authentication handling (Token management, refresh)
- The `api.js` module provides unified error code handling (401, 40101, 50000, etc.)
- The `api.js` module provides unified response normalization
- Direct use of `fetch` bypasses all these unified mechanisms

#### Allowed Patterns
```javascript
// ✅ CORRECT: Use api module
import { api, instrumentsApi, categoriesApi } from '../services/api'

// List
const data = await api.get('/endpoint')

// Create
await api.post('/endpoint', data)

// Update
await api.put('/endpoint/:id', data)

// Delete
await api.delete('/endpoint/:id')

// Or use modular API
await instrumentsApi.list()
await categoriesApi.getById(id)
```

#### Forbidden Patterns
```javascript
// ❌ FORBIDDEN: Direct fetch
fetch('/api/endpoint')
  .then(res => res.json())
```

#### Exceptions (When Direct fetch IS Allowed)
1. **OAuth Authentication**: `/auth/callback` - special auth flow
2. **File Upload with FormData**: multipart/form-data requests
3. **Third-party APIs**: External services not under TuneLoop control

#### Implementation
When migrating existing code:
1. Replace `fetch()` calls with `api.get/post/put/delete()`
2. Remove manual `.then(res => res.json())` - api.js handles response parsing
3. Remove manual `if (result.code === 20000)` checks - api.js normalizes response
4. Keep try-catch for error handling (errors will be thrown for non-2xx responses)

#### Verification
Before marking a task as complete:
- [ ] Run `grep -r "fetch(" frontend-pc/src --include="*.js" --include="*.jsx" --include="*.ts" --include="*.tsx"` to verify no forbidden fetch usage remains
- [ ] Only the following files are allowed to contain fetch:
   - `services/api.js` (the module itself)
  - `App.jsx` (OAuth callback only)
  - `pages/AuthCallback/*` (OAuth auth flow)

## 🌐 URL 路由规范 (URL Routing Specification)

### RESTful URL 模式

项目采用统一的 RESTful 风格 URL 路由，所有功能页面必须支持以下 URL 直通模式：

| URL 模式 | 说明 | 状态要求 |
|----------|------|----------|
| `/:group/:page` | 列表页，无选中 | 列表无选中，详情提示选择 |
| `/:group/:page/:id` | 详情页，选中指定项 | 列表选中该项，显示详情 |
| `/:group/:page/:id/edit` | 编辑页 | 列表选中该项，进入编辑模式 |
| `/:group/:page/new` | 创建页 | 列表无选中，进入创建模式 |

### 应用范围

以下模块必须支持 URL 直通功能：

- **乐器相关** (`/instruments/*`)
- **分类相关** (`/instruments/categories`)
- **网点相关** (`/sites/*`)
- **属性相关** (`/instruments/properties`)

### 示例

```
/instruments/categories          → 分类管理页，无选中分类
/instruments/categories/:id      → 分类管理页，选中该分类，显示详情
/instruments/categories/:id/edit → 分类管理页，选中该分类，进入编辑模式
/instruments/categories/new      → 分类管理页，无选中，创建顶级分类
```

### 实现要求

#### 路由配置 (App.jsx)
```jsx
<Route path="/instruments/categories" element={<CategoryList />} />
<Route path="/instruments/categories/:id" element={<CategoryList />} />
<Route path="/instruments/categories/:id/edit" element={<CategoryList />} />
<Route path="/instruments/categories/new" element={<CategoryList />} />
```

#### 组件 URL 解析 (useEffect)
```javascript
useEffect(() => {
  const path = window.location.pathname
  
  // 1. Edit mode: /:page/:id/edit
  const editMatch = path.match(/\/:page\/([^/]+)\/edit$/)
  if (editMatch) {
    // Load item and enter edit mode
  }
  
  // 2. Create mode: /:page/new
  if (path.endsWith('/new')) {
    // Enter create mode
  }
  
  // 3. Detail mode: /:page/:id
  const detailMatch = path.match(/\/:page\/([^/]+)$/)
  if (detailMatch) {
    // Load item and show detail
  }
}, [])
```

#### URL 状态同步
- 创建/编辑完成后：更新 URL 为 `/:page/:id`（详情页）
- 取消操作后：更新 URL 为 `/:page`（列表页）
- 使用 `window.history.pushState()` 更新 URL
- 监听 `popstate` 事件处理浏览器前进/后退

### 验证清单

添加新页面时必须验证：
- [ ] 支持列表页 URL（`/:page`）
- [ ] 支持详情页 URL（`/:page/:id`）
- [ ] 支持编辑页 URL（`/:page/:id/edit`）
- [ ] 支持创建页 URL（`/:page/new`）
- [ ] 操作后 URL 正确更新
- [ ] 浏览器前进/后退正常工作
- [ ] 左侧菜单选中状态同步
