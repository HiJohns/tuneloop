# 微信小程序迁移方案

## 架构原则

**Taro 框架保留，样式层分离**——业务逻辑（services/platform/utils）共享，页面组件分两套：

```
frontend-mobile/src/
├── pages/              # H5 专属页面（Tailwind className）
├── pages-weapp/        # 小程序专属页面（纯 inline style）
├── components/         # H5 版组件（Tailwind）
├── components-weapp/   # 小程序版组件（inline style）
├── services/           # 共享 API 调用 ← 两边一样
├── platform/           # 共享平台抽象层 ← 两边一样
└── utils/              # 共享工具函数 ← 两边一样
```

样式层分离的理由：H5 用 Tailwind（标准 CSS），小程序 WXS 不支持 CSS 转义、伪类、通用选择器等大量标准特性，同源无法兼顾。

---

## 第一阶段：完美复刻（Phase 1）

### 目标

把 H5 首页在小程序上 1:1 还原——轮播图、搜索框、分类菜单、乐器卡片列表、底栏导航，所有视觉效果与 H5 一致。

### 实现方式

**不使用任何 Tailwind className**。全部用 `style={{...}}` 内联样式。已验证的映射规则：

| H5 Tailwind | 小程序等效 |
|------------|-----------|
| `className="w-full h-full"` | `style={{ width: '100%', height: '100%' }}` |
| `className="flex items-center"` | `style={{ display: 'flex', alignItems: 'center' }}` |
| `className="fixed inset-0"` | `style={{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0 }}` |
| `className="z-[40]"` | `style={{ zIndex: 40 }}` |
| `className="bg-[#915F38]"` | `style={{ backgroundColor: '#915F38' }}` |
| `className="rounded-2xl"` | `style={{ borderRadius: 16 }}` |
| `className="text-xl font-black"` | `style={{ fontSize: 20, fontWeight: '900' }}` |
| `className="shadow-md"` | `style={{ boxShadow: '0 4px 6px -1px rgba(0,0,0,0.1)' }}` |
| `<Image mode="aspectFill">` | `<Image mode="widthFix">` |

> 完整 CSS 对照表见前一版本文档的 Tailwind → inline style 映射。

### 构建流程

1. `npm run build:weapp`（Taro 编译 JSX → 小程序代码）
2. 运行 `scripts/weapp-post-build.sh` 清理不兼容 CSS（后续追加）
3. `miniprogram-ci upload` 上传到微信服务器

### 涉及页面

从 H5 首页 `Home.jsx` 逐组件复刻到 `pages-weapp/`：

| 序号 | 组件 | 说明 |
|------|------|------|
| 1 | Banner 轮播图 | 无限循环 + 触摸滑动 + 自动播放 |
| 2 | 搜索框 | 半透明圆角，仅展示 UI |
| 3 | 分类菜单 | 水平滚动，高亮当前 |
| 4 | 乐器卡片 | 封面图 + 名称 + 类别 + 级别标签 + 价格 |
| 5 | 底栏导航 | 四个 tab，当前页高亮 |
| 6 | 加载骨架 | 数据加载中显示占位卡片 |
| 7 | 空状态 | 无数据时显示占位图 |

### 已验证的技术栈

| 功能 | 实现方案 | 验证状态 |
|------|---------|:---:|
| 页面路由 | `Taro.navigateTo` / `Taro.navigateBack` | ✅ |
| 参数传递 | `Taro.getCurrentInstance().router?.params?.id` | ✅ |
| API 调用 | `Taro.request` 直接调用 | ✅ |
| 图片加载 | `<Image mode="widthFix" src="https://...">` | ✅ |
| 滑动切换 | `onTouchStart/onTouchEnd` + `clientX` | ✅ |
| CSS transform | `transform: translateX(-XX%)` | ✅ |
| CSS transition | `transition: transform 0.5s ease-in-out` | ✅ |
| ScrollView | 必须显式 `height: '100%'`（不能用 `flex: 1`） | ✅ |
| 调试面板 | `Taro.setEnableDebug({ enableDebug: true })` → 微信原生 vConsole | ✅ |

### API 配置

| 项目 | H5 | 小程序 |
|------|-----|-------|
| apiBaseUrl | `/api`（浏览器自动补齐域名） | `https://wx.cadenzayueqi.com/api`（必须绝对 HTTPS） |
| 域名白名单 | 不需要 | mp.weixin.qq.com → 服务器域名 必须登记 |

### 排障速查

| 现象 | 常见原因 | 检查项 |
|------|---------|--------|
| 页面全白 | 整份 WXSS 被拒绝 | `*`, `~`, `:not`, `\`, `[ ]` 任一残留即失败 |
| API 报 `request:fail invalid url` | 域名不在白名单 | 检查 `apiBaseUrl` 是否为绝对路径 + 微信后台域名登记 |
| 列表不滚动 | ScrollView 高度为 0 | `height` 改为显式像素值 |
| 图片模糊/变形 | 模式不对 | 改用 `mode="widthFix"` |
| 数据不渲染 | API 取层错误 | `res.data.data.list` 注意双层 `data` 嵌套 |
| 跳转失败 | `react-router-dom` 在小程序中被 alias | 改用 `Taro.navigateTo` |

---

## 第二阶段：微信原生能力（Phase 2）

### 目标

将微信平台独有能力的入口接入小程序——拍照、扫码、支付、位置等。

### 现有 H5 功能 vs 小程序对接

从代码库扫描所有 H5 端使用的平台抽象函数，逐项列出小程序端的微信 API 对接方案：

| 功能 | H5 实现 | 小程序实现 | 状态 |
|------|--------|-----------|:---:|
| **拍照/选图** | `<input type="file" accept="image/*">` | `wx.chooseImage` / `Taro.chooseImage` | 待开发 |
| **扫码** | `new BarcodeDetector()` | `wx.scanCode` / `Taro.scanCode` | 待开发 |
| **支付** | mock / H5 支付 | `wx.requestPayment` / `Taro.requestPayment` | 待开发 |
| **位置** | `navigator.geolocation` / 地址选择器 | `wx.chooseLocation` / `Taro.chooseLocation` | 待开发 |
| **手机号授权** | `<Button openType="getPhoneNumber">` | 微信原生能力 | 待开发 |
| **分享** | Web Share API | `wx.showShareMenu` / `Taro.showShareMenu` | 待开发 |
| **文件上传** | `fetch` + FormData | `wx.uploadFile` / `Taro.uploadFile` | 待开发 |
| **登录** | OAuth 重定向 | `wx.login()` → 获取 code → 后端换 JWT | ✅ (weapp.md 已设计) |
| **推送通知** | Web Push / 轮询 | `wx.requestSubscribeMessage` | 待开发 |
| **客服消息** | 独立聊天页面 | `<Button openType="contact">` | 待开发 |

### 平台抽象层对接

`brandon-mobile/src/platform/` 已有平台抽象层：
- `browser.js` — H5 实现（`navigator.geolocation`、`fetch`、存量逻辑）
- `index.weapp.js` — 已实现存储、请求、对话框
- **待补充**：`chooseImage`、`scanCode`、`requestPayment`、`chooseLocation`、`login`、`share`、`uploadFile`、`subscribeMessage`

每个函数需要在 `browser.js`（已有逻辑）和 `index.weapp.js`（接微信 API）中同时实现。

### 对接要点

**支付**：
- 下单接口不变（`POST /api/user/rental`）
- 支付通道从 H5 模拟改为调用 `wx.requestPayment({ timeStamp, nonceStr, package, signType, paySign })`
- 后端需返回微信支付参数（需微信支付商户号配置）

**扫码**：
- 目前 `BarcodeDetector` 是 H5 API，部分浏览器不支持
- 小程序直接用 `Taro.scanCode({ onlyFromCamera: false })` 返回 `{ result, scanType }`

**拍照**：
- H5 用 `<input type="file">` 选择本地文件
- 小程序用 `Taro.chooseImage({ count, sizeType, sourceType })` 返回 `{ tempFilePaths }`
- 上传使用 `Taro.uploadFile` 或转 base64 后走 `request`

**登录**：
- H5 用 OAuth 重定向 → `/callback` 页
- 小程序用 `wx.login()` → `Taro.login()` 获取临时 code
- `POST /api/wx/login { code }` → IAM 换取 JWT
- 已在 `docs/weapp.md` 设计完毕，需落实后端实现

---

## 执行计划

1. **建目录**：`src/pages-weapp/` `src/components-weapp/`
2. **逐组件迁移**：按 Phase 1 清单，从 `Home.jsx` 复刻到 `pages-weapp/Home.jsx`（全 inline style）
3. **验证**：每个组件完成后 build + 上传 + 真机查看
4. **完善平台层**：按 Phase 2 清单，在 `index.weapp.js` 中补全 `chooseImage`/`scanCode`/`requestPayment` 等函数
5. **对接后端**：支付参数、wx-login 端点
6. **提交审核**：全功能通过后提交微信审核

## 维护规范

- `pages-weapp/` 中**禁止**使用 Tailwind className
- 所有样式用 `style={{...}}` 内联
- 新功能先在 H5 完成，再复刻到小程序
- `services/` 和 `utils/` 两端共享，任何修改自动影响两边
- 每次提交前跑 `npm run build:weapp` 确保编译通过
