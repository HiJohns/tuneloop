# TC-1555 Review — Home 页面滚动无回弹 + 点击灵敏

> TC Issue: #1555 | 生成日期: 2026-08-07 | 类型: 前端静态 + 真机 checklist

## 1. 目标

验证首页滚动无回弹、各元素点击灵敏（#1540 修复后不再出现卡顿/穿透）。

## 2. 验证方式

- [ ] 后端测试: 不适用（纯前端 UI）
- [x] 静态验证（代码层 Z 轴/触摸/防回弹审查）
- [ ] 真机 checklist（必须）

## 3. 静态审查证据

### 3.1 ✅ Z 轴分层正确

**代码证据**: `frontend-mobile/src/pages-weapp/Home.jsx`

| Z 层 | 元素 | 代码行 | 检查 |
|------|------|:---:|------|
| Z=0 | 走马灯背景 | :280 | ✅ `position:fixed; inset:0; zIndex:0` |
| Z=40 | 指示点 | :336 | ✅ `absolute; zIndex:40` |
| Z=50 | 购物车 | (components) | ✅ `position:fixed; zIndex:50` |
| Z=100 | 内容 ScrollView | :387 | ✅ `position:fixed; zIndex:100; top:contentTop; bottom:0` |
| Z=10000 | 搜索条 | :374 | ✅ `position:fixed; zIndex:10000` |
| Z=10002 | 粘连菜单/swipe层 | :382/354 | ✅ `menuStuck?1:0 opacity` 切换；`scrolled?0:10002` |

**合规检查**: 各层 Z-index 单调递增，无重叠覆盖导致触摸穿透 ✅

### 3.2 ✅ Swipe 层滚动时穿透

**代码证据**: `:354` — `zIndex: scrolled ? 0 : 10002` → 滚动时降为 0，释放下方菜单触摸 ✅

### 3.3 ✅ 防回弹措施

**代码证据**:
- `:380` — `menuStuck` 用 opacity 替代 `{!cond && ...}`（避免 DOM 增删 → 内容高度突变 → ScrollView 回弹）
- `:448` — 底部 2000px 垫片确保 ScrollView 内容始终 > viewport
- `:385` — debounced scroll（500ms 延迟）防止抖动

**合规检查**: 条件渲染已替换为 opacity ✅；固定垫片 2000px ✅

### 3.4 ✅ 走马灯 fallback 背景色

**代码证据**: `:280` — `backgroundColor: banners.length === 0 ? '#915F38' : 'transparent'` ✅

### 3.5 ✅ 磨砂模糊过渡

**代码证据**: `:326` — `opacity: scrolled ? 1 : 0` blur 层 + `transition: 'opacity 0.3s'` ✅

## 4. 页面注册与跳转

**静态证据**: `bash scripts/static-verify.sh rent` 输出:
```
✅ 首页 — pages-weapp/home/index 已注册
✅ 首页卡片跳详情
✅ switchTab 迁移（首页 tab）
```

## 5. ⚠️ 需真机验证部分（不可自动化）

| # | 检查项 | 预期 | 方法 |
|---|--------|------|------|
| 1 | 垂直滚动无回弹 | 不出现肉眼可见的回弹 | 真机快速上下滑动 |
| 2 | 水平拖动菜单流畅 | 不粘连、不触发页面级滚动 | 真机左右滑动菜单 |
| 3 | 卡片点击跳转详情 | 始终响应，不因栈满无反应 | 连续点 tab 20 次后再点卡片 |
| 4 | 走马灯滑动切换 | 左右 swipe 流畅，自动播放正常 | 观察 3 轮自动播放 |
| 5 | 搜索条点击 | 跳转搜索页 | 点击放大镜 |
| 6 | 购物车点击 | 已登录→购物车/未登录→登录页 | 分别测试 |
| 7 | 多/中/少三项分类切换 | 不同列表长度下无回弹 | 切换 3 个不同大小的分类 |

## 6. 结论

| 审查维度 | 结果 |
|----------|:---:|
| Z 轴分层 | ✅ 无重叠 |
| Swipe 层触摸穿透 | ✅ scrolled→z=0 |
| 防回弹（opacity + 垫片） | ✅ |
| fallback 背景色 | ✅ |
| 页面注册 + switchTab 迁移 | ✅ |
| 真机 checklist | ⚠️ 待执行 |

**静态部分总判定**: ✅ 通过（代码层审查全部合规）

**真机验证**: 待手动执行 7 项 checklist

---

*Model: deepseek/deepseek-v4-flash*
