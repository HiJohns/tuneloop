# #2318 weapp 确认订单冒烟证据

> 目的：补齐 QA 审计指出的 `weapp 确认订单` 矩阵行证据。
> 环境：Linux headless，无微信开发者工具/真机；采用项目 #2319 已接受的 weapp 副本 taro-shim 真实渲染口径。

## Harness

- 临时 Vite 入口直接 import `frontend-mobile/src/pages-weapp/Checkout.jsx`（weapp 专属页面）。
- `@tarojs/components` → 项目 `frontend-mobile/src/taro-shim.js`。
- `@tarojs/taro` → 最小 stub（`getCurrentInstance().router.params` 从 URL query 读取；导航/选图/Toast 为空实现）。
- Playwright 拦截 `/api/**` 返回 3 档 tiers（7 天 / 30 天 / 30 天以上），设置 token 后加载页面。
- 断言页面出现「费用明细」「1-7天」「/天」，且无 `TypeError` / pageerror。
- 临时 harness 未提交；页面冒烟基建见 #2319。

## 红绿输出

### 缺陷版（`edf1751c4^`，map 形参 `t` 遮蔽 i18n `t`）

```
WEAPP_SOURCE_RENDERED=NO
WEAPP_CRASH=YES
ERRORS=["pageerror: t2 is not a function", ...]
OLD_WEAPP_SMOKE_RC=1
```

### 修复版（`edf1751c4`，map 形参改为 `tier`）

```
WEAPP_SOURCE_RENDERED=YES
WEAPP_CRASH=NO
WEAPP_SOURCE_SMOKE_PASS
WEAPP_SMOKE_RC=0
```

## 构建产物佐证

`frontend-mobile/dist-weapp/pages-weapp/checkout/index.js` 中阶梯行为 `(0,O.t)("/天")`（模块作用域 i18n 引用），不再是被 map 形参遮蔽的 `t`；`npm run build:weapp` 成功。

## 边界

- 这不是微信开发者工具/真机截图；是 weapp 专属页面的真实 React 渲染（taro-shim）。真机 E2E 仍由 #2319 Phase 2 / QA 负责。
- 若 QA 要求必须微信开发者工具/真机，则属工具/环境根因，需回 Architect 将 weapp 真机行拆为独立 Issue。
