# 移动端页面渲染冒烟（page-smoke）

> 关联：#2319。目标：在发布前捕获「构建通过、ESLint 通过，但页面运行期崩溃」的问题（#2298 `t is not a function` 由用户发现）。
> 工具：`vitest` + `jsdom` + `@testing-library/react` + 项目 `taro-shim.js`。

## 1. 运行

```bash
cd frontend-mobile
source ~/.nvm/nvm.sh && nvm use 22
npm run smoke
```

- 任一页面出现 `TypeError` / `ReferenceError` / React 渲染异常 / `unhandledrejection` → 非 0 退出，输出「页面 + 错误栈」。
- 任一页面未渲染出预期标记（如确认订单的「费用明细」）→ 失败。
- 输出示例：`✓ H5 /checkout renders without fatal errors`、`× weapp payment ...`。

## 2. 覆盖范围（Phase 1）

| 端 | 页面 | 入口 |
|----|------|------|
| H5 | 确认订单 | `/checkout?id=inst-1` |
| H5 | 支付 | `/payment?type=rent&id=order-1` |
| H5 | 订单详情 | `/order/order-1` |
| H5 | 购物车 | `/cart` |
| H5 | 我的 | `/profile` |
| H5 | 注册 | `/register` |
| weapp | 确认订单 | `pages-weapp/checkout` |
| weapp | 支付 | `pages-weapp/payment` |
| weapp | 订单详情 | `pages-weapp/order-detail` |
| weapp | 购物车 | `pages-weapp/cart`（共享 Cart） |
| weapp | 我的 | `pages-weapp/profile` |
| weapp | 注册资料 | `pages-weapp/profile-complete` |

`routes.js` 同时提供 `discoverH5Routes()` / `discoverWeappPages()`，从 `App-H5.jsx` 与 `app.config.ts` 发现页面；Phase 2 扩到全部路由。

## 3. Mock 与富 fixture

- `mock-platform.js` stub 全局 `fetch`，按 URL 返回 `fixtures.js` 的富数据。
- **fixture 必须字段齐全且数组非空**：`pricingV2.tiers` 非空、地址/担保人/订单 `fee_detail.tiers` 等；空数据不崩不代表有数据不崩（#2298/#2315 教训）。
- `setup.js` 注入：
  - 假 JWT（`role=STAFF`，带 `oid/tid`，绕过核身重定向）；
  - 登录 token、`cart_user-1` 购物车；
  - `window.matchMedia` / `ResizeObserver` / `IntersectionObserver` / `scrollTo` 等 jsdom 缺失 API。
- `taro-stub.js` 替代 `@tarojs/taro`：`getCurrentInstance/useRouter` 从 `window.location.search` 读取参数，`useLoad/useDidShow` 以 effect 执行；导航/选图/Toast 为空实现。`@tarojs/components` 走项目 `src/taro-shim.js`。

## 4. 错误捕获

`harness.jsx` 的 `captureRender()` 同时捕获：

1. React `render()` 抛出的渲染异常；
2. `window.onerror` / `unhandledrejection`；
3. `console.error` 中的 `TypeError` / `ReferenceError` / `is not a function` / `The above error occurred` / `Cannot read propert...`；
4. 页面标记缺失（调用方断言）。

等待 300ms 让异步 API/effect 完成；页面有更长链路时可调大等待或改用 `waitFor`。

## 5. 红绿自证

### 5.1 页面级 red-green（真实 Checkout 副本）

```bash
cd frontend-mobile
npm run smoke:defect   # 必须 FAIL（注入 t 遮蔽后的真实 Checkout）
npm run smoke          # 必须 PASS（修复后的全部关键页）
```

- `scripts/page-smoke/run-defect.mjs` 读取当前 `src/pages/Checkout.jsx`，仅对阶梯定价块做机械注入：
  `map((tier, i)` → `map((t, i)`、块内 `tier.` → `t.`，生成临时 `src/pages/CheckoutAuditDefect.jsx`（测试后自动删除，不提交）。
- `scripts/page-smoke/checkout-defect.spec.jsx` 用真实 3 档 `tiers` fixture 渲染该副本；预期 `expect(errors).toEqual([])` **失败**，输出 `H5 /checkout injected t-shadow` + `TypeError: t is not a function`。
- 该 spec 不匹配 `npm run smoke` 的 `*.test.jsx` include，只在 `npm run smoke:defect` 显式运行；这是可重放的页面级红绿证据。

### 5.2 合成单元保护（额外）

`smoke.test.jsx` 另含 `captures the #2298 t-shadow defect (red-green)`：`red-green.fixture.jsx` 的最小 `.map((t, i) => t('/天'))` 触发错误，断言 harness 捕获。它只作额外保护，不能替代 5.1 的真实 Checkout 行。

## 6. 门禁接入

- 触碰 `frontend-mobile/src/**` 的 Issue，`pre-ready` 的 `--affected-cmd` 必须包含：
  ```bash
  cd frontend-mobile && npm run smoke
  ```
- `AGENTS.md`「强制验证清单」已新增「页面冒烟」行：**构建通过 ≠ 页面可打开**。

## 7. 边界

- jsdom 不执行真实 Taro 原生组件/微信渲染；weapp 页面用 `taro-shim` 渲染的是**页面业务 JS**。真机 E2E 仍由 QA/#2319 Phase 2 负责。
- Phase 1 只覆盖关键路径；Phase 2 扩全路由。
- 纯文案/配置改动不强制跑 smoke；触碰页面/渲染代码时强制。
