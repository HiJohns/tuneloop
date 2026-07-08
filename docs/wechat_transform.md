# 微信小程序迁移方案

## 架构原则

**Taro 框架保留，样式层分离**——业务逻辑（services/platform/utils）共享，页面组件分两套：

```
frontend-mobile/src/
├── pages/              # H5 专属页面（Tailwind className）
├── pages-weapp/        # 小程序专属页面（纯 inline style）
├── components/         # H5 版组件（Tailwind）
├── components-weapp/   # 小程序版组件（inline style）
├── styles-weapp.js     # 共享样式常量（避免重复）
├── services/           # 共享 API 调用 ← 两边一样
├── platform/           # 共享平台抽象层 ← 两边一样
└── utils/              # 共享工具函数 ← 两边一样
```

### 关键决策：weapp 不加载 Tailwind

weapp 页面**零 Tailwind 类名**，构建时也不加载 `@tailwind` 指令。这样 `app.wxss` 只有 `body{margin:0}`，不需要任何 post-build sed 清理。

实现方式——`app.tsx` 条件导入：
```tsx
if (process.env.TARO_ENV !== 'weapp') {
  import './app.css'  // 只有 H5 加载 Tailwind
}
```

### app.config.ts 条件页面

```ts
const isWeapp = process.env.TARO_ENV === 'weapp'
const weappPages = [
  'pages-weapp/home/index',
  'pages-weapp/detail/index',
  'pages-weapp/checkout/index',
  // ...
]
const h5Pages = [
  'pages/home/index',
  'pages/detail/index',
  // ...
]
export default { pages: isWeapp ? weappPages : h5Pages, ... }
```

---

## 第一阶段：直接迁移（Phase 1）

### 策略

**从 H5 源码直接转换**，不从测试页"生长"。每个页面复制到 `pages-weapp/`，然后逐行将 Tailwind className 替换为 inline style。这样保留 H5 已有的全部视觉细节。

### 迁移步骤（每个页面通用）

1. **复制** `pages/Home.jsx` → `pages-weapp/Home.jsx`
2. **替换路由** `import { useNavigate } from 'react-router-dom'` → `import Taro from '@tarojs/taro'`，`navigate(url)` → `Taro.navigateTo({ url })`
3. **替换图标** `import { ArrowLeft } from 'lucide-react'` → emoji 或 `<Text>←</Text>`
4. **替换 antd** `import { Modal } from 'antd'` → 自定义 inline style 组件
5. **逐行替换 className** → `style={{...}}`，从 `styles-weapp.js` 引用常量
6. **替换 Image mode** `aspectFit` → `widthFix`
7. **构建验证** `npm run build:weapp` + 上传 + 真机查看

### 共享样式常量

`styles-weapp.js` 避免重复内联样式：

```js
export const card = { backgroundColor: '#fff', borderRadius: 16, padding: 12, display: 'flex', boxShadow: '0 4px 6px -1px rgba(0,0,0,0.1)' }
export const cardThumb = { width: 80, height: 80, borderRadius: 12, overflow: 'hidden', flexShrink: 0 }
export const textLg = { fontSize: 22, fontWeight: '900', color: '#000' }
export const textSm = { fontSize: 14, color: '#71717a', fontWeight: '700' }
export const levelBadge = (bg) => ({ backgroundColor: bg, color: '#fff', fontSize: 14, padding: '2px 10px', borderRadius: 999, fontWeight: '900', alignSelf: 'flex-start' })
export const priceText = { color: '#C21838', fontWeight: '900', fontSize: 26 }
export const navBar = { display: 'flex', justifyContent: 'space-around', alignItems: 'center', backgroundColor: '#5A3B24', borderTop: '1px solid #4E321E', paddingTop: 10, paddingBottom: 10, position: 'absolute', bottom: 0, left: 0, right: 0, zIndex: 50 }
export const searchBar = { width: 250, height: 42, borderRadius: 999, display: 'flex', alignItems: 'center', paddingLeft: 16, paddingRight: 16, backgroundColor: 'rgba(255,255,255,0.2)', border: '1px solid rgba(255,255,255,0.3)' }
// ... 按需扩展
```

### Tailwind → inline style 速查

| Tailwind | inline style |
|----------|-------------|
| `w-full h-full` | `{ width: '100%', height: '100%' }` |
| `flex items-center justify-between` | `{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }` |
| `fixed inset-0` | `{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0 }` |
| `z-[40]` | `{ zIndex: 40 }` |
| `bg-[#915F38]` | `{ backgroundColor: '#915F38' }` |
| `bg-black/40` | `{ backgroundColor: 'rgba(0,0,0,0.4)' }` |
| `text-white/70` | `{ color: 'rgba(255,255,255,0.7)' }` |
| `rounded-2xl` | `{ borderRadius: 16 }` |
| `rounded-full` | `{ borderRadius: 999 }` |
| `text-xl font-black` | `{ fontSize: 20, fontWeight: '900' }` |
| `shadow-md` | `{ boxShadow: '0 4px 6px -1px rgba(0,0,0,0.1)' }` |
| `px-4 py-2` | `{ paddingLeft: 16, paddingRight: 16, paddingTop: 8, paddingBottom: 8 }` |
| `py-0.5` | `{ paddingTop: 2, paddingBottom: 2 }` |
| `space-x-1.5` | 给子元素加 `{ marginLeft: 6 }`（weapp 不支持 `~` 选择器） |
| `truncate` | `{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }` |
| `overflow-hidden` | `{ overflow: 'hidden' }` |
| `flex-1` | `{ flex: '1 1 0%' }` |
| `flex-shrink-0` | `{ flexShrink: 0 }` |

### 第三方依赖替换

| H5 依赖 | 用途 | weapp 替代 |
|---------|------|-----------|
| `react-router-dom` | `useNavigate` `useSearchParams` | `Taro.navigateTo` / `Taro.navigateBack` / `Taro.getCurrentInstance().router?.params` |
| `lucide-react` | 箭头、关闭、设置等图标 | emoji（← ✕ ⚙）或 `<Text>` 字符 |
| `antd` | Modal / Upload / Steps / InputNumber | 自定义 inline style 组件 |

### 页面优先级

| 优先级 | 页面 | 理由 |
|--------|------|------|
| P0 | Home | 首页入口 |
| P0 | Detail | 乐器详情 |
| P0 | Checkout | 支付确认 |
| P0 | Success | 支付完成 |
| P1 | Profile | 个人中心 |
| P1 | MyLeases | 我的租赁 |
| P2 | StaffInstrumentForm | 员工录入乐器 |
| P2 | CreateRepairRequest | 用户报修 |
| P2 | MyRepairs | 我的报修 |
| P3 | 其余所有页面 | 后续迭代 |

### 已验证技术栈

| 功能 | 实现方案 | 验证状态 |
|------|---------|:---:|
| 页面路由 | `Taro.navigateTo` / `Taro.navigateBack` | ✅ |
| 参数传递 | `Taro.getCurrentInstance().router?.params?.id` | ✅ |
| API 调用 | `Taro.request` | ✅ |
| 图片加载 | `<Image mode="widthFix" src="https://...">` | ✅ |
| 滑动切换 | `onTouchStart/onTouchEnd` + `clientX` | ✅ |
| CSS transform | `transform: translateX(-XX%)` | ✅ |
| CSS transition | `transition: transform 0.5s ease-in-out` | ✅ |
| ScrollView | 必须显式 `height`（不能用 `flex: 1`） | ✅ |
| 调试面板 | `Taro.setEnableDebug({ enableDebug: true })` | ✅ |
| 轮播图无限循环 | clone 技术 + `onTransitionEnd` 无动画跳转 | ✅ |

### API 配置

| 项目 | H5 | 小程序 |
|------|-----|-------|
| apiBaseUrl | `/api`（浏览器自动补齐域名） | `https://wx.cadenzayueqi.com/api`（必须绝对 HTTPS） |
| 域名白名单 | 不需要 | mp.weixin.qq.com → 服务器域名 必须登记 |
| 图片 URL | 相对路径 `/uploads/...` 可用 | 必须绝对 `https://wx.cadenzayueqi.com/uploads/...` |

### 排障速查

| 现象 | 常见原因 | 检查项 |
|------|---------|--------|
| 页面全白 | CSS 被拒绝 或 JS 报错 | 检查 WXSS 是否有 `*` `~` `:not` `\` 残留；用 `setEnableDebug` 看 Console |
| API 报 `request:fail invalid url` | 域名不在白名单 | 检查 `apiBaseUrl` 是否为绝对路径 + 微信后台域名登记 |
| 列表不滚动 | ScrollView 高度为 0 | `height` 改为显式像素值 |
| 列表滚动后回弹复位 | `onScroll` → `setState` 触发 re-render → ScrollView 滚动位重置 | 见下方 §滚动回弹案例 |
| 图片模糊/变形 | 模式不对 | 改用 `mode="widthFix"` |
| 数据不渲染 | API 取层错误 | `res.data.data.list` 注意双层 `data` 嵌套 |
| 跳转失败 | `react-router-dom` 在小程序中被 alias | 改用 `Taro.navigateTo` |
| 样式细节不对 | inline style 写法与 Tailwind 有差异 | 逐项对照速查表，注意 `alignSelf`、`flexShrink` 等细节 |

### 滚动回弹案例（#Home.jsx 2026-07-08）

**现象**：ScrollView 中乐器列表向上滚动约 10px 后自动弹回原位，无法正常浏览。

**排查过程**：
1. 先用 `flex: 1` → weapp ScrollView 不解析 flex 高度（无效）
2. 换成 `position: absolute, top: 0, bottom: 50` → 仍然回弹
3. 移除 `overflow: hidden`（weapp `<View>` 不支持 CSS overflow 属性，会阻断 ScrollView 触摸事件）→ 无效
4. **最终定位**：移除 ScrollView 的 `onScroll` handler 后立即恢复

**根因**：

| 层 | 问题 | 说明 |
|----|------|------|
| **主因** | `onScroll` → `setState` → re-render 死循环 | 每次手指滚动触发 `onScroll` → `setScrollY(value)` → React state 更新 → `Home` 组件 re-render → weapp 中 ScrollView re-render 时内部滚动位置被重置为 0 → UI 回到原位。手指持续拖动形成"滚一点→复位→滚一点→复位"的 ping-pong 效果 |
| 次因 | `overflow: hidden` 阻断事件 | weapp `<View>` 不支持 CSS `overflow` 属性，设置后会干扰 ScrollView 的触摸事件传递链 |
| 次因 | `flex: 1` 不适用 ScrollView | weapp ScrollView 必须使用显式像素高度（从 `Taro.getSystemInfoSync().windowHeight` 计算），`flex: 1` 及 `height: 0` 技巧部分版本无效 |

**修复**：
```jsx
// ❌ 错误：onScroll 触发 setState → re-render → 滚动位重置
<ScrollView onScroll={e => setScrollY(e.detail.scrollTop)} ...>

// ✅ 正确：需要滚动监听时使用 useRef 避免 re-render
const scrollYRef = useRef(0)
<ScrollView onScroll={e => { scrollYRef.current = e.detail.scrollTop }} ...>

// ✅ ScrollView 高度用显式像素值
const wh = Taro.getSystemInfoSync().windowHeight
<ScrollView style={{ height: wh - 192 }} scrollY showScrollbar={false}>
```

**教训**：
- ScrollView 的 `onScroll` 必须使用 `useRef` 而非 `useState`。任何触发 re-render 的操作都会导致 weapp 重置滚动位置。
- `overflow: hidden` 在 weapp `<View>` 上无效果且有害。
- ScrollView 高度一律用显式像素计算，不依赖 CSS flex。

---

## 第二阶段：微信原生能力（Phase 2）

### 目标

将微信平台独有能力的入口接入小程序——拍照、扫码、支付、位置等。

### 功能清单

| 功能 | H5 实现 | 小程序实现 | 外部依赖 | 状态 |
|------|--------|-----------|---------|:---:|
| **拍照/选图** | `<input type="file" accept="image/*">` | `Taro.chooseImage` | 无 | 待开发 |
| **扫码** | `new BarcodeDetector()` | `Taro.scanCode` | 无 | 待开发 |
| **支付** | mock / H5 支付 | `Taro.requestPayment` | **微信支付商户号** | 阻塞 |
| **位置** | `navigator.geolocation` | `Taro.chooseLocation` | 无 | 待开发 |
| **手机号授权** | `<Button openType="getPhoneNumber">` | 微信原生能力 | 无 | 待开发 |
| **分享** | Web Share API | `Taro.showShareMenu` | 无 | 待开发 |
| **文件上传** | `fetch` + FormData | `Taro.uploadFile` | 无 | 待开发 |
| **登录** | OAuth 重定向 → `/callback` | `wx.login()` → code → 后端换 JWT | **beaconiam wx-login 端点** | 阻塞 |
| **推送通知** | 轮询 | `Taro.requestSubscribeMessage` | **微信模板消息 ID** | 阻塞 |
| **客服消息** | 独立聊天页面 | `<Button openType="contact">` | 无 | 待开发 |

### 阻塞项

| 阻塞 | 说明 | 解决方式 |
|------|------|---------|
| 微信支付商户号 | 需要在微信支付平台申请 mch_id + API key | 运营层面操作，开发无法推进 |
| beaconiam wx-login | `POST /api/v1/auth/wx-login` 端点未实现 | beaconiam 仓库建 Issue |
| 微信模板消息 | 需在 mp.weixin.qq.com 配置订阅消息模板 | 运营层面操作 |

### 平台抽象层补充

`src/platform/index.weapp.js` 需新增以下函数：

```js
// 拍照/选图
export const chooseImage = (count = 1) => Taro.chooseImage({ count, sizeType: ['compressed'], sourceType: ['album', 'camera'] })

// 扫码
export const scanCode = () => Taro.scanCode({ onlyFromCamera: false })

// 支付
export const requestPayment = (params) => Taro.requestPayment(params)

// 位置
export const chooseLocation = () => Taro.chooseLocation()

// 分享
export const showShareMenu = () => Taro.showShareMenu({ withShareTicket: true })

// 文件上传
export const uploadFile = (url, filePath, name = 'file') => Taro.uploadFile({ url, filePath, name })
```

`src/platform/browser.js` 对应的 H5 实现保持现有逻辑不变。

---

## 执行计划

| 步骤 | 内容 | 产出 |
|------|------|------|
| 1 | 建 `pages-weapp/` `components-weapp/` 目录 + `styles-weapp.js` | 骨架 |
| 2 | 修改 `app.tsx` 条件导入 + `app.config.ts` 条件页面 | 双端构建 |
| 3 | 迁移 P0 页面（Home → Detail → Checkout → Success） | 核心流程可用 |
| 4 | 迁移 P1 页面（Profile → MyLeases） | 用户中心可用 |
| 5 | 补充平台层 `chooseImage` / `scanCode` / `uploadFile` | 原生能力可用 |
| 6 | 对接后端 wx-login + 支付（解除阻塞后） | 登录+支付闭环 |
| 7 | 迁移 P2/P3 页面 | 全功能 |
| 8 | 提交微信审核 | 上线 |

## 维护规范

- `pages-weapp/` 中**禁止**使用 Tailwind className
- 所有样式用 `style={{...}}` 内联，或从 `styles-weapp.js` 引用常量
- 新功能先在 H5 完成（`pages/`），再复刻到小程序（`pages-weapp/`）
- `services/` 和 `utils/` 两端共享，任何修改自动影响两边
- 每次提交前 `npm run build:weapp` 确保编译通过
- 图片 URL 在 `pages-weapp/` 中必须用绝对路径 `https://wx.cadenzayueqi.com/...`
