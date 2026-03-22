# PC 端界面开发现状调查报告

> 调查日期: 2026-03-22 | Issue: #70

## 1. 概述

### 1.1 调查目标
- 确认 PC 端界面是否全部使用 Dummy 数据
- 检查是否实现了登录状态检查（Authentication）
- 检查是否实现了权限控制（Authorization/RBAC）
- 对照 docs 目录下的文档，评估各界面开发完成度

### 1.2 涉及文件
| 文件 | 功能 |
|------|------|
| `frontend-pc/src/App.jsx` | 路由配置、布局 |
| `frontend-pc/src/pages/Dashboard.jsx` | 仪表盘 |
| `frontend-pc/src/pages/Login/index.tsx` | 登录页 |
| `frontend-pc/src/data/mockData.js` | Mock 数据源 |
| `frontend-pc/src/components/BrandProvider/index.tsx` | 白标化组件 |

---

## 2. 核心发现

### 2.1 Dummy 数据使用情况 ⚠️

| 页面 | 数据源 | 状态 |
|------|--------|------|
| Dashboard | `mockData.js` | ❌ 全部 Dummy |
| InstrumentStock | `mockData.js` | ❌ 全部 Dummy |
| FinanceConfig | localStorage | ⚠️ 本地存储，未连接后端 |
| LeaseLedger | - | ❌ 未实现 |
| SiteManagement | - | ❌ 未实现 |
| RolePermission | - | ❌ 未实现 |
| WorkOrderList | - | ❌ 未实现 |
| 其他页面 | - | ❌ 占位符/未实现 |

**Mock 数据文件 (`frontend-pc/src/data/mockData.js`):**
```javascript
export const assets = [
  {
    id: "TL-PI-2026-081",
    name: "雅马哈 U1 立式钢琴",
    // ... 完全硬编码的数据
  }
];
```

### 2.2 认证状态检查 ❌

**当前实现：**
```javascript
// Login/index.tsx
const handleLogin = () => {
  const iamUrl = import.meta.env.VITE_IAM_URL;
  window.location.href = `${iamUrl}/oauth/authorize?...`;
};
```

**缺失功能：**
| 功能 | 状态 | 说明 |
|------|------|------|
| Token 存储 | ❌ | 未实现 JWT 存储 |
| Token 刷新 | ❌ | 未实现自动刷新 |
| 会话保持 | ❌ | 页面刷新后丢失状态 |
| 登出功能 | ❌ | 未实现 |
| 受保护路由 | ❌ | 未实现路由守卫 |

**App.jsx 当前路由：**
```javascript
// 无任何认证中间件
<BrowserRouter>
  <MainLayout />  // 直接渲染，无保护
</BrowserRouter>
```

### 2.3 权限控制 (RBAC) ❌

| 功能 | 状态 |
|------|------|
| 权限定义 | ❌ 无 |
| 权限中间件 | ❌ 无 |
| 路由级权限 | ❌ 无 |
| 组件级权限 | ❌ 无 |
| API 级权限 | ❌ 无 |

---

## 3. 页面完成度评估

### 3.1 功能对比表

| 页面/功能 | Docs 设计 | 实际实现 | 完成度 |
|-----------|-----------|----------|--------|
| **登录页** | BrandProvider + IAM 重定向 | ✅ 基础实现 | 60% |
| **Dashboard** | 统计卡片 + 待办事项 | ❌ 仅 Dummy 数据 | 30% |
| **资产管理** | 设备台账/库存监控 | ❌ 占位符 | 10% |
| **租约管理** | 租约台账/逾期预警 | ❌ 占位符 | 5% |
| **乐器定价配置** | 定价矩阵 Excel 编辑 | ⚠️ 本地配置 | 40% |
| **乐器库存** | 库存监控 | ❌ 仅 Dummy 数据 | 30% |
| **Site 网点管理** | 网点管理 | ❌ 未实现 | 0% |
| **工单列表** | 维保调度 | ❌ 未实现 | 0% |
| **报价单管理** | 报价中心 | ❌ 占位符 | 5% |
| **角色权限** | RBAC 配置 | ❌ 未实现 | 0% |
| **供应商库** | - | ❌ 未实现 | 0% |
| **熔断规则** | - | ❌ 未实现 | 0% |
| **押金流水** | - | ❌ 未实现 | 0% |
| **到期预警** | - | ❌ 未实现 | 0% |

### 3.2 总体完成度

```
已完成功能 (Dummy):
██████████████████░░░░░░░░  25% (3/12 页面有 UI)

真实后端对接:
░░░░░░░░░░░░░░░░░░░░░░░░  0%

认证与权限:
░░░░░░░░░░░░░░░░░░░░░░░░  0%
```

---

## 4. 关键问题分析

### 4.1 问题 1: 全局 Dummy 数据依赖

**影响文件：**
- `frontend-pc/src/data/mockData.js` - 单一数据源
- `Dashboard.jsx` - 导入使用
- `InstrumentStock.jsx` - 导入使用

**建议方案：**
```javascript
// 替换为 API 服务层
import { fetchAssets } from '@/services/asset';
import { useQuery } from '@tanstack/react-query';

// Dashboard.jsx
const { data: assets } = useQuery({
  queryKey: ['assets'],
  queryFn: fetchAssets,
});
```

### 4.2 问题 2: 缺少认证状态管理

**影响文件：**
- `App.jsx` - 需要路由守卫
- `Login/index.tsx` - 需要完善回调处理

**建议方案：**
```javascript
// 新建 AuthProvider
<AuthProvider>
  <ProtectedRoute path="/dashboard">
    <Dashboard />
  </ProtectedRoute>
</AuthProvider>
```

### 4.3 问题 3: 缺少权限系统

**当前状态：**
- 无权限定义文件
- 无权限中间件
- 所有用户同等访问权限

**建议方案：**
```javascript
// 新建 permission.js
export const PERMISSIONS = {
  VIEW_DASHBOARD: 'dashboard:view',
  MANAGE_ASSETS: 'assets:manage',
  // ...
};

// 组件级权限
<HasPermission permission="assets:manage">
  <AssetManagement />
</HasPermission>
```

---

## 5. 改进建议

### 5.1 优先级排序

| 优先级 | 任务 | 工作量 | 影响 |
|--------|------|--------|------|
| P0 | 添加 API 服务层 | 中 | 解除 Dummy 依赖 |
| P0 | 实现认证状态管理 | 中 | 安全性基础 |
| P1 | 实现路由守卫 | 低 | 保护敏感页面 |
| P1 | 添加权限控制 | 高 | 完整 RBAC |
| P2 | 完善各页面后端对接 | 高 | 功能完整性 |

### 5.2 实施建议

**Phase 1: 数据层 (1-2天)**
1. 创建 `src/services/` 目录
2. 实现 `assetService.js`、`orderService.js` 等
3. 使用 React Query 或 SWR 管理数据获取
4. 替换所有 Dummy 数据引用

**Phase 2: 认证层 (1-2天)**
1. 创建 `AuthContext` 管理登录状态
2. 实现 `ProtectedRoute` 组件
3. 处理 IAM 回调逻辑
4. 添加 Token 刷新机制

**Phase 3: 权限层 (2-3天)**
1. 定义权限常量
2. 实现权限检查 Hook
3. 添加权限守卫组件
4. 配置路由级权限

---

## 6. 结论

### 6.1 现状总结

| 维度 | 状态 | 说明 |
|------|------|------|
| UI 完成度 | 25% | 部分页面有 UI，但无后端对接 |
| 数据真实性 | 0% | 全部使用 Dummy 数据 |
| 认证 | 0% | 无认证状态管理 |
| 权限 | 0% | 无权限控制系统 |
| 后端对接 | 0% | 无 API 调用 |

### 6.2 下一步行动

建议按优先级实施以下任务：
1. 创建 API 服务层，解除 Dummy 依赖
2. 实现认证状态管理和路由守卫
3. 完善各页面的后端数据对接
4. 实现完整的 RBAC 权限系统

---

## 附录

### A. 文件索引

```
frontend-pc/src/
├── App.jsx                    # 路由配置（无认证保护）
├── data/
│   └── mockData.js           # Dummy 数据源
├── pages/
│   ├── Dashboard.jsx         # 仪表盘（Dummy）
│   ├── LeaseLedger.jsx       # 租约台账（占位符）
│   ├── FinanceConfig.jsx     # 定价配置（本地存储）
│   ├── InstrumentStock.jsx    # 乐器库存（Dummy）
│   ├── Login/index.tsx       # 登录页（部分实现）
│   └── ...
└── components/
    └── BrandProvider/         # 白标化（已实现）
```

### B. Docs 文档位置
- 功能需求: `docs/features.md`
- UI 设计: `docs/ui.md`

---

*Model: kimi-k2.5*