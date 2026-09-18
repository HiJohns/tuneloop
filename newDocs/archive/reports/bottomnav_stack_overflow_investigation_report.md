# 底部导航页面栈溢出调查报告

> Issue: #1558 | 日期: 2026-08-07 | 角色: Investigation Lead

## 1. 概述

- **目标模块**: 微信小程序自定义底部导航（BottomNav）与页面路由
- **问题现象**: 连续点击九次底条『租赁』或『首页』后，点击乐器卡片无响应（不跳转详情页）
- **根因**: 页面栈溢出 —— `Taro.navigateTo` 每次点击压栈一层，微信小程序页面栈上限为 **10 层**，栈满后 `navigateTo` 静默失败

## 2. 复现路径与证据链

### 2.1 关键代码路径

**`frontend-mobile/src/app.config.ts`** — 未配置 `tabBar`：

```typescript
export default {
  pages: isWeapp ? weappPages : h5Pages,   // 17 个页面，均为普通页面
  window: { ... },                          // 无 tabBar 字段！
}
```

首页、租赁、我的、维修**全部是普通页面**，非 tabBar 页面 → 只能 `navigateTo`，不能用 `switchTab`。

**`frontend-mobile/src/pages-weapp/Home.jsx:116`** — tab 跳转使用 `navigateTo`：

```jsx
const nav = (url) => { Taro.navigateTo({ url }) }
...
{ key: 'home', icon: '🏪', label: '首页', onClick: () => nav('/pages-weapp/home/index') },
{ key: 'rent', icon: '🪕', label: '租赁', onClick: () => nav('/pages-weapp/my-leases/index') },
```

**`frontend-mobile/src/pages-weapp/MyLeases.jsx:55,294-297`** — 同样的 `navigateTo` 模式。

**`frontend-mobile/src/pages-weapp/Profile.jsx:83,329-332`** — 同样的 `navigateTo` 模式。

**`frontend-mobile/src/pages-weapp/Home.jsx:414`** — 乐器卡片跳转：

```jsx
onClick={() => { const url = tenant ? `/pages-weapp/detail/index?id=${instrument.id}&tenant=${tenant}` : `/pages-weapp/detail/index?id=${instrument.id}`; nav(url) }}
```

### 2.2 栈深度数学验证（与"九次"完全吻合）

| 操作 | 页面栈变化 | 栈深 |
|------|-----------|:---:|
| 启动进入首页 | [Home] | 1 |
| 点『租赁』 | [Home, MyLeases] | 2 |
| 点『首页』 | [Home, MyLeases, Home] | 3 |
| 点『租赁』 | [..., MyLeases] | 4 |
| ... | ... | ... |
| **第 9 次点击 tab** | **[...]** | **10 = 上限** |
| **点击乐器卡片** | 需压入 Detail → **第 11 层 → FAIL** | **无响应** |

微信小程序文档规定：`wx.navigateTo` 的页面栈**最多十层**，超出后跳转失败（`fail` 回调，无用户提示）。用户报告"九次"与理论值 1+9=10 精确吻合。

### 2.3 为什么是"静默"失败

`Taro.navigateTo({ url })` 未传 `success/fail` 回调，微信在小程序端静默忽略超栈跳转（仅 console 报 `navigateTo:fail webview count limit exceed`），真机上用户无感知。

## 3. 受影响文件清单

| 文件 | 行号 | 问题 |
|------|------|------|
| `frontend-mobile/src/app.config.ts` | 63-70 | 缺 `tabBar` 配置，tab 页均为普通页面 |
| `frontend-mobile/src/pages-weapp/Home.jsx` | 116, 439-442 | `navigateTo` 跳 tab；卡片 414 行超栈失败 |
| `frontend-mobile/src/pages-weapp/MyLeases.jsx` | 55, 294-297 | `navigateTo` 跳 tab |
| `frontend-mobile/src/pages-weapp/Profile.jsx` | 83, 329-332 | `navigateTo` 跳 tab |

## 4. 周边发现（非本次根因，但相关）

1. **维修 tab 指向未注册页面**：MyLeases.jsx:296 / Profile.jsx:331 指向 `/pages-weapp/my-repairs/index`，但 `pages-weapp/` 下**不存在** my-repairs 页面，`app.config.ts` 也未注册 → 维修 tab 点击必失败（影响所有含维修 tab 的页面）。
2. **同一页面反复压栈**：在首页点击『首页』也会压入新 Home 实例（Home.jsx:439），加速栈满；Taro 对同页面 navigateTo 不去重。
3. **H5 端 (pages/)**：使用 react-router `navigate()`，不受 10 层限制，但反复 `navigate('/')` 会无限累积 history 条目，存在同源隐患（低危）。

## 5. 修复建议（供后续 /analyze 参考）

按推荐优先级排序：

### 方案 A：配置原生 tabBar（推荐，微信官方标准做法）
- `app.config.ts` 增加 `tabBar` 配置（list 含 首页/租赁/维修/我的）
- 所有 tab 跳转改用 `Taro.switchTab`（switchTab 不消耗页面栈）
- 保留自定义样式：`tabBar.custom: true` + `custom-tab-bar` 组件（需迁移自定义 UI）
- 注意：`switchTab` 不支持带 query 参数（tenant/category 需改走 storage/全局状态）

### 方案 B：tab 跳转改用 `Taro.reLaunch`（最小改动）
- BottomNav 所有 tab onClick 改为 `Taro.reLaunch({ url })` — 清空页面栈再跳转
- 代价：每次切 tab 全页面重建（状态、滚动位置丢失），且 reLaunch 不能带参数进栈

### 方案 C：栈深守卫（兜底，建议与 A/B 组合）
- 封装 `safeNav(url)`：`const pages = Taro.getCurrentPages(); if (pages.length >= 9) Taro.reLaunch({url}) else Taro.navigateTo({url})`
- 或 tab 目标已在栈中时 `navigateBack` 到对应页（delta 计算）

**建议**: A + C 组合（原生 tabBar + 栈深守卫兜底）。若短期内不想动 tabBar 结构，至少实施 C 以止血。

## 6. 附录

### 调用链时序图（正常 vs 溢出）

```
正常路径:  Home ──navigateTo──▶ Detail           ✅
                 (栈深 1→2，未超限)

溢出路径:  Home →MyLeases→Home→MyLeases→...→Home  (栈深 10)
                 │
                 └─navigateTo Detail ──▶ FAIL（静默）❌ 卡片无响应
```

### 修复验证清单
- [ ] 连续点击 tab 20+ 次后，乐器卡片仍可跳转详情
- [ ] 维修 tab 可正常跳转（方案 A 时需注册页面）
- [ ] 真机验证（模拟器可能不严格限制栈深）

---

*Model: deepseek/deepseek-v4-flash*
