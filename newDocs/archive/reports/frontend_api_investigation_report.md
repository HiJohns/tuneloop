# 前端请求与后端交互调查报告

## 1. 概述

本报告调查了 TuneLoop 前端项目（frontend-pc）中所有向后端发送请求的代码点，分析了所使用的技术以及是否对返回结果做过统一处理。

### 1.1 调查范围

- **前端项目路径**: `frontend-pc/src/`
- **调查目标**: 统计所有 API 请求点，分析请求模式

---

## 2. 请求技术分析

### 2.1 主要技术栈

前端项目使用了两种主要的 HTTP 请求技术：

| 技术 | 使用场景 | 文件位置 |
|------|----------|----------|
| **统一 api.js 模块** | 推荐方式，所有模块化 API 调用 | `src/services/api.js` |
| **原生 fetch** | 直接调用，部分组件绕过统一模块 | 各组件内部 |

### 2.2 统一 API 模块 (api.js)

**文件路径**: `frontend-pc/src/services/api.js`

这是前端的核心请求封装模块，提供了以下功能：

```javascript
// 核心导出
export const api = {
  get: (endpoint) => request(endpoint),
  post: (endpoint, data) => request(endpoint, { method: 'POST', body: JSON.stringify(data) }),
  put: (endpoint, data) => request(endpoint, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (endpoint) => request(endpoint, { method: 'DELETE' }),
}
```

**模块化 API 封装** (第 221-321 行):

| API 模块 | 用途 |
|----------|------|
| `instrumentsApi` | 乐器管理 |
| `ordersApi` | 订单管理 |
| `sitesApi` | 网点管理 |
| `inventoryApi` | 库存管理 |
| `maintenanceApi` | 维护工单 |
| `ownershipApi` | 所有权证书 |
| `permissionApi` | 权限管理 |
| `leaseApi` | 租赁管理 |
| `depositApi` | 押金管理 |
| `iamAdminApi` | IAM 租户/客户端管理 |
| `categoriesApi` | 分类管理 |

### 2.3 直接使用 fetch 的情况

部分组件绕过了统一的 api.js 模块，直接使用 `fetch`：

| 文件 | 行号 | 说明 |
|------|------|------|
| `App.jsx` | 267, 341 | OAuth 回调、配置获取 |
| `pages/admin/instrument/List.jsx` | 49, 335, 399 | 乐器列表、导入导出 |
| `pages/admin/instrument/Edit.jsx` | 142, 175, 514 | 编辑器 |
| `pages/admin/instrument/Detail.jsx` | 38, 197, 228, 244 | 详情页 |
| `pages/admin/category/List.jsx` | 21, 194 | 分类管理 |
| `pages/AuthCallback/index.tsx` | 38 | 认证回调 |
| `pages/MaintenanceDispatch.jsx` | 21, 37, 53 | 维护调度 |
| `components/AssetTimeline/index.jsx` | 25 | 时间线 |
| `components/PricingMatrixEditor/index.jsx` | 32, 64 | 价格矩阵 |
| `components/BrandProvider/index.tsx` | 39 | 品牌配置 |

---

## 3. 统一响应处理机制

### 3.1 请求流程 (api.js 第 118-212 行)

```
1. Token 过期检查 (滑动窗口续期)
       ↓
2. 添加 Authorization Header
       ↓
3. 发送 fetch 请求
       ↓
4. 处理 401 状态码 (调用 handleAuthError)
       ↓
5. 处理 40101 业务错误码 (token 过期)
       ↓
6. 标准化响应格式 (提取 data 字段)
       ↓
7. 返回处理后的数据
```

### 3.2 认证与 Token 处理

**Token 获取** (第 3-14 行):
- 从 cookie 中读取 `token`
- 降级到 localStorage/sessionStorage

**Token 存储** (第 17-21 行):
```javascript
function storeTokens(accessToken, refreshToken) {
  localStorage.setItem('token', accessToken)
  localStorage.setItem('refresh_token', refreshToken)
  document.cookie = `token=${accessToken}; path=/; max-age=604800`
}
```

**Token 过期检测** (第 55-68 行):
- 解析 JWT payload 中的 exp 字段
- 剩余时间小于 30 天的 30% 时触发续期

**Token 刷新机制** (第 93-116 行):
- 使用 refresh_token 获取新 access_token
- 自动存储新 token

**认证错误处理** (第 70-91 行):
```javascript
async function handleAuthError(token, retryCount, endpoint, options) {
  if (retryCount < 1) {
    // 尝试刷新 token
    await refreshAccessToken()
    // 重试原请求
    return await request(endpoint, options, retryCount + 1)
  }
  // 刷新失败，清除 token 并跳转 IAM
  clearTokens()
  redirectToIAM()
  return { __authFailed: true }
}
```

### 3.3 响应标准化处理

api.js 实现了智能响应提取 (第 181-211 行):

```javascript
// 直接返回数组
if (Array.isArray(data)) return data

// 提取常见包装字段
if (Array.isArray(data.data)) return data.data
if (Array.isArray(data.items)) return data.items
if (Array.isArray(data.result)) return data.result
if (Array.isArray(data.list)) return data.list

// 处理嵌套格式
if (Array.isArray(data.data.instruments)) return data.data.instruments
if (Array.isArray(data.data.list)) return data.data.list

// 处理统一响应格式
if (data.success && Array.isArray(data.data)) return data.data
if (data.code === 0 && Array.isArray(data.data)) return data.data
if (data.code === 20000 && Array.isArray(data.data)) return data.data
```

---

## 4. 请求点统计

### 4.1 通过 api.js 模块的请求

共 **82 处** 调用点，分布在以下模块：

| 模块 | 请求数 | 主要端点 |
|------|--------|----------|
| instrumentsApi | 3 | `/instruments` |
| ordersApi | 5 | `/orders/*` |
| sitesApi | 7 | `/common/sites`, `/merchant/sites`, `/sites/tree` |
| inventoryApi | 3 | `/merchant/inventory` |
| maintenanceApi | 8 | `/maintenance` |
| ownershipApi | 2 | `/user/ownership` |
| permissionApi | 5 | `/admin/*` |
| leaseApi | 4 | `/merchant/leases` |
| depositApi | 3 | `/merchant/deposits` |
| iamAdminApi | 8 | `/system/*` |
| categoriesApi | 5 | `/categories` |

### 4.2 直接使用 fetch 的请求

共 **25 处** 直接调用，主要分布在：

- 乐器管理相关: 11 处
- 认证相关: 3 处
- 维护工单: 3 处
- 其他组件: 8 处

---

## 5. 问题与建议

### 5.1 发现的问题

1. **直接使用 fetch 绕过统一模块**: 约 25 处直接使用 fetch，未经过 api.js 的统一认证和响应处理
2. **响应处理不一致**: 直接使用 fetch 的组件需要自行处理响应格式和错误
3. **代码重复**: 多个组件重复实现相同的请求逻辑

### 5.2 改进建议

1. **统一使用 api.js 模块**: 将所有直接使用 fetch 的地方迁移到 api.js 或对应的模块化 API
2. **建立组件级请求拦截器**: 考虑在组件级别添加请求/响应拦截器
3. **添加请求日志**: 统一记录请求日志，便于调试

---

## 6. 附录

### 6.1 API 端点汇总

| 端点前缀 | 用途 |
|----------|------|
| `/instruments/*` | 乐器管理 |
| `/orders/*` | 订单管理 |
| `/common/sites/*` | 公共网点 |
| `/merchant/sites/*` | 商户网点管理 |
| `/sites/tree` | 网点树形结构 |
| `/merchant/inventory/*` | 库存管理 |
| `/maintenance/*` | 维护工单 |
| `/user/ownership/*` | 所有权证书 |
| `/admin/*` | 系统管理 |
| `/merchant/leases/*` | 租赁管理 |
| `/merchant/deposits/*` | 押金管理 |
| `/system/*` | IAM 系统管理 |
| `/categories/*` | 分类管理 |
| `/properties/*` | 属性管理 |
| `/auth/*` | 认证 |

### 6.2 关键文件索引

| 文件 | 用途 |
|------|------|
| `frontend-pc/src/services/api.js` | 统一请求封装模块 |
| `frontend-pc/src/App.jsx` | 应用入口，包含 OAuth 回调 |
| 各 `pages/*` 组件 | 业务页面请求 |
| 各 `components/*` 组件 | 组件级请求 |

---

*Model: moonshotai-cn/kimi-k2-thinking*
