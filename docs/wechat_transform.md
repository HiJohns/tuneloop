# 微信小程序 Tailwind 类名转义改造

## 核心结论：H5 → 小程序 实现对照表

| 维度 | H5 版本 | 小程序等效方案 |
|------|---------|--------------|
| 图片缩放 | `mode="aspectFit"` + `objectFit: contain` | `mode="widthFix"`（weapp 专属模式，等宽保比例）|
| 尺寸定位 | `className="w-full h-full"` `fixed inset-0` | `style={{ width:'100%', height:'100%' }}` |
| z-index 任意值 | `z-[40]` `z-[10000]` | ❌ `[` `]` 被当属性选择器 → `style={{ zIndex: 40 }}` |
| opacity 修饰符 | `bg-black/40` `text-white/70` | ✅ 同写法，post-build `sed` 去掉 `\` |
| 小数 spacing | `py-0.5` `space-x-1.5` | ❌ `.5` 变数字开头类名 → 改为 `py-1` `space-x-2` |
| hex 颜色 | `bg-[#915F38]` | ❌ `#` 被当 ID 选择器 → 改 `style={{ backgroundColor }}` |
| 变体伪类 | `hover:` `focus:` | ❌ weapp 无 `:hover`，丢弃不用 |
| | `active:` `last:` | 待验证（`:active` `:last-child` 可能支持）|
| Tailwind 生成 | `@tailwind base/components/utilities` | 生成后需 `sed` 清除 `*` `~` `:not` `\` 四种不兼容规则 |
| 动画/滑动 | `translateX` + `transition` + clone 技术 | ✅ 完全相同（`transform`/`transition` 在 weapp 生效）|
| 触摸事件 | `onTouchStart/onTouchEnd` + `clientX` | ✅ 完全相同 |

### weapp WXS 不支持清单

| 特性 | 说明 |
|------|------|
| `\` 转义 | 任何 CSS `\` 都会导致上传验证失败 |
| `*` 通用选择器 | `*,::after,::before{...}` 在 WXS 中报错 |
| `~` 兄弟选择器 | `space-x-*` `divide-*` 依赖的 `A ~ B` 模式不支持 |
| `:not()` 取反伪类 | `:not([hidden])` 不支持，已改为 `view` 标签 |
| `[` `]` 属性选择器 | `.z-[10000]` 中的 `[10000]` 被当属性选择器 |
| `::after` `::before` | 伪元素不支持 |
| `hover:` `focus:` | 伪类不支持（触屏设备无鼠标） |

### 构建后处理脚本

`scripts/weapp-post-build.sh`：

```bash
WXSS="dist-weapp/app.wxss"
sed -i 's|\\||g' "$WXSS"            # 去掉所有 \ 转义
sed -i '/> :not/d' "$WXSS"           # 去掉 :not() 规则
sed -i '/> view ~/d' "$WXSS"         # 去掉 ~ 兄弟选择器规则
sed -i '/~ view/d' "$WXSS"
sed -i 's/\*,::after,::before{[^}]*}::backdrop{[^}]*}//' "$WXSS"  # 去掉 Tailwind 全局变量块
```

---

## 背景（实验前）

微信小程序 WXS 引擎不支持 CSS 转义字符（`\`）。Tailwind v3 生成的类名中含有特殊字符，在标准 CSS 中需要通过 `\` 转义。

**实测验证结果：**

| 转义类型 | 去 `\` 后的 CSS selector | weapp 行为 | 结论 |
|---------|------------------------|-----------|------|
| `\/` → `/` | `.bg-black/40` | ✅ 正常匹配 | sed 可行 |
| `\[` `\]` → `[` `]` | `.z-[10000]` | ❌ `[10000]` 被解析为属性选择器 | **必须改源码** |
| `\.` → `.` | `.right-0.5` | ❌ PDF 为 `.right-0` + `.5`（数字开头类名） | **必须改源码** |
| `\#` → `#` | `.bg-[#5A3B24]` | ❌ `#5A3B24]` 被解析为 ID 选择器 | **必须改源码** |
| `\:` → `:` | `.hover:bg-blue-600` | ❌ `:bg-blue-600` 被解析为伪类 | **必须改源码** |
| `\!` → `!` | `.!visible` | ❌ 非法选择器 | tailwind.config blocklist |
| `\%` → `%` | `.max-w-[60%]` | 已验证通过 | sed 可行 |

## 改造策略（修订版）

### 第一层：Tailwind 配置

**File**: `frontend-mobile/tailwind.config.js`

```js
module.exports = {
  content: ['./src/**/*.{js,jsx,ts,tsx}'],
  corePlugins: { preflight: false },
  blocklist: ['!visible'],
}
```

> `preflight: false` 关闭后，Tailwind 仍会在 CSS 最前面生成 `*,::after,::before{...}` 全局变量声明。`*` 和 `::after`/`::before` 在 weapp 中不被支持，需通过 post-build sed 移除。

### 第二层：Post-build sed

构建后执行以下脚本，处理**两种已验证可行的转义**：

**File**: `frontend-mobile/scripts/weapp-post-build.sh`（新文件）

```bash
#!/bin/bash
WXSS="dist-weapp/app.wxss"
if [ -f "$WXSS" ]; then
  # 去掉 \/ 转义（opacity 修饰符、分数宽度）
  sed -i 's|\\/|/|g' "$WXSS"
  # 去掉 \% 转义（百分比任意值）
  sed -i 's|\\%|%|g' "$WXSS"
  # 移除 Tailwind preflight 全局变量块（*,::after,::before 不被 weapp 支持）
  sed -i 's/\*,::after,::before{[^}]*}::backdrop{[^}]*}//' "$WXSS"
  echo "Post-build processing complete"
fi
```

### 第三层：源码替换

所有含 `[` `]` `#` `.` `:` 的 Tailwind 类名必须改为 `style` 内联或整数级 Tailwind 类名。

#### 3a. 任意值类名（`[` `]` 括号）→ `style` 内联

| 原类名 | 替换 |
|--------|------|
| `z-[10000]` | `style={{ zIndex: 10000 }}` |
| `z-[10001]` | `style={{ zIndex: 10001 }}` |
| `z-[10002]` | `style={{ zIndex: 10002 }}` |
| `z-[100]` | `style={{ zIndex: 100 }}` |
| `z-[40]` | `style={{ zIndex: 40 }}` |
| `z-[5]` | `style={{ zIndex: 5 }}` |
| `z-[1]` | `style={{ zIndex: 1 }}` |
| `w-[250px]` | `style={{ width: 250 }}` |
| `w-[72px]` | `style={{ width: 72 }}` |
| `h-[42px]` | `style={{ height: 42 }}` |
| `h-[72px]` | `style={{ height: 72 }}` |
| `pb-[140px]` | `style={{ paddingBottom: 140 }}` |
| `min-h-[120px]` | `style={{ minHeight: 120 }}` |
| `min-w-[16px]` | `style={{ minWidth: 16 }}` |
| `max-w-[480px]` | `style={{ maxWidth: 480 }}` |
| `max-w-[140px]` | `style={{ maxWidth: 140 }}` |
| `max-w-[60%]` | `style={{ maxWidth: '60%' }}` |
| `max-w-[90%]` | `style={{ maxWidth: '90%' }}` |
| `max-h-[80%]` | `style={{ maxHeight: '80%' }}` |
| `max-h-[90vh]` | `style={{ maxHeight: '90vh' }}` |
| `text-[10px]` | `style={{ fontSize: 10 }}` |
| `text-[11px]` | `style={{ fontSize: 11 }}` |
| `text-[26px]` | `style={{ fontSize: 26 }}` |
| `text-[9px]` | `style={{ fontSize: 9 }}` |
| `text-[1.4rem]` | `style={{ fontSize: '1.4rem' }}` |
| `leading-[1.6rem]` | `style={{ lineHeight: '1.6rem' }}` |
| `left-[-5px]` | `style={{ left: -5 }}` |

> 注意：`text-[1.4rem]`、`leading-[1.6rem]`、`max-w-[60%]`、`max-h-[80%]` 同时包含 `[` `]` 和 `.` 或 `%`。

#### 3b. 任意 hex 颜色（`\#`）→ `style` 内联

| 原类名 | 替换 |
|--------|------|
| `bg-[#5A3B24]` | `style={{ backgroundColor: '#5A3B24' }}` |
| `bg-[#915F38]` | `style={{ backgroundColor: '#915F38' }}` |
| `bg-[#FDFBF7]` | `style={{ backgroundColor: '#FDFBF7' }}` |
| `bg-[#FDF4E7]` | `style={{ backgroundColor: '#FDF4E7' }}` |
| `bg-[#C21838]` | `style={{ backgroundColor: '#C21838' }}` |
| `bg-[#002140]` | `style={{ backgroundColor: '#002140' }}` |
| `bg-[#0084FF]` | `style={{ backgroundColor: '#0084FF' }}` |
| `bg-[#8A2BE2]` | `style={{ backgroundColor: '#8A2BE2' }}` |
| `bg-[#B98E5F]` | `style={{ backgroundColor: '#B98E5F' }}` |
| `bg-[#A87D50]` | `style={{ backgroundColor: '#A87D50' }}` |
| `bg-[#FF2A55]` | `style={{ backgroundColor: '#FF2A55' }}` |
| `bg-[#FF6B00]` | `style={{ backgroundColor: '#FF6B00' }}` |
| `text-[#C21838]` | `style={{ color: '#C21838' }}` |
| `text-[#FF2A55]` | `style={{ color: '#FF2A55' }}` |
| `border-[#4E321E]` | `style={{ borderColor: '#4E321E' }}` |
| `border-[#5A3B24]` | `style={{ borderColor: '#5A3B24' }}` |
| `from-[#FDF4E7]` | `style={{ backgroundImage: 'linear-gradient(to bottom, #FDF4E7, white)' }}` |

#### 3c. 小数 spacing（`\.`）→ 整数级

| 原类名 | 替换 | 说明 |
|--------|------|------|
| `py-0.5` | `py-1` | 0.125rem → 0.25rem |
| `px-0.5` | `px-1` | |
| `mt-0.5` | `mt-1` | |
| `mb-0.5` | `mb-1` | |
| `mx-0.5` | `mx-1` | |
| `p-0.5` | `p-1` | |
| `w-0.5` | `w-1` | |
| `h-0.5` | `h-1` | |
| `pb-0.5` | `pb-1` | |
| `pt-0.5` | `pt-1` | |
| `top-0.5` | `top-1` | |
| `right-0.5` | `right-1` | |
| `bottom-0.5` | `bottom-1` | |
| `left-0.5` | `left-1` | |
| `-mt-0.5` | `-mt-1` | |
| `px-1.5` | `px-2` | 0.375rem → 0.5rem |
| `py-1.5` | `py-2` | |
| `mt-1.5` | `mt-2` | |
| `space-x-1.5` | `space-x-2` | |
| `space-y-0.5` | `space-y-1` | |
| `space-y-1.5` | `space-y-2` | |
| `px-2.5` | `px-3` | 0.625rem → 0.75rem |
| `py-2.5` | `py-3` | |
| `py-3.5` | `py-4` | |
| `w-1.5` | `w-2` | |
| `h-1.5` | `h-2` | |
| `w-2.5` | `w-3` | |
| `h-2.5` | `h-3` | |
| `top-1/2` | `style={{ top: '50%' }}` | 分数定位 |
| `w-1/2` | `style={{ width: '50%' }}` | 分数宽度 |
| `w-3/4` | `style={{ width: '75%' }}` | |
| `w-1/4` | `style={{ width: '25%' }}` | |

批量 sed（小数 spacing）：
```bash
cd frontend-mobile/src
sed -i 's/py-0\.5/py-1/g; s/px-0\.5/px-1/g; s/mt-0\.5/mt-1/g; s/mb-0\.5/mb-1/g; s/mx-0\.5/mx-1/g; s/p-0\.5/p-1/g; s/w-0\.5/w-1/g; s/h-0\.5/h-1/g; s/pb-0\.5/pb-1/g; s/pt-0\.5/pt-1/g; s/top-0\.5/top-1/g; s/right-0\.5/right-1/g; s/bottom-0\.5/bottom-1/g; s/left-0\.5/left-1/g; s/-mt-0\.5/-mt-1/g; s/px-1\.5/px-2/g; s/py-1\.5/py-2/g; s/mt-1\.5/mt-2/g; s/space-x-1\.5/space-x-2/g; s/space-y-0\.5/space-y-1/g; s/space-y-1\.5/space-y-2/g; s/px-2\.5/px-3/g; s/py-2\.5/py-3/g; s/py-3\.5/py-4/g; s/w-1\.5/w-2/g; s/h-1\.5/h-2/g; s/w-2\.5/w-3/g; s/h-2\.5/h-3/g' pages/*.jsx components/*.jsx
```

## 完整文件清单（18 个文件）

### 需改任意值类名（54 处 `[` `]`）

```
frontend-mobile/src/components/BottomNav.jsx
  - z-50, bg-[#5A3B24], border-[#4E321E], border-[#5A3B24]
  - text-[10px], text-[9px], min-w-[16px]

frontend-mobile/src/pages/Home.jsx — 数量最多
  - bg-[#xx] × 11, text-[#xx] × 3
  - z-[10000/10001/10002/100/40/5/1] × 7
  - w-[250px], w-[72px], h-[42px], h-[72px]
  - text-[1.4rem], text-[26px], leading-[1.6rem]
  - bg-black/40(x2), bg-black/20, text-white/70, text-white/60
  - bg-[#5A3B24]/80, text-[#C21838]/70
  - w-1/2, w-3/4, py-0.5, space-x-1.5
  - from-[#FDF4E7]

frontend-mobile/src/pages/Detail.jsx
  - text-[10px], pb-[140px], space-x-1.5
  - py-1.5, px-1.5, py-0.5, bg-black/50
  - max-h-[80%]

frontend-mobile/src/pages/Checkout.jsx
  - text-[10px], text-[11px], max-w-[480px]
  - bg-zinc-50/40, border-zinc-200/60, py-1.5, mx-0.5

frontend-mobile/src/pages/OrderDetail.jsx
  - text-[10px], max-w-[480px], mt-0.5(x10), w-0.5

frontend-mobile/src/pages/Cart.jsx
  - text-[10/11px], max-w-[140/60/90%], max-h-[80%], px-1.5, py-0.5

frontend-mobile/src/pages/StaffInstrumentForm.jsx
  - top-1/2, bg-black/50, top-0.5, right-0.5, p-0.5

frontend-mobile/src/pages/CreateRepairRequest.jsx
  - bg-black/50(x4)

frontend-mobile/src/pages/ShippingInterface.jsx
  - bg-black/50, mt-0.5

frontend-mobile/src/pages/ReceivingInterface.jsx
  - bg-black/50, mt-0.5

frontend-mobile/src/pages/StaffInstrumentDetail.jsx
  - bg-black/30

frontend-mobile/src/pages/StaffReceiveConfirm.jsx
  - space-y-0.5

frontend-mobile/src/pages/ReceivingRepairScan.jsx
  - py-1.5(x2)

frontend-mobile/src/pages/PaymentComplete.jsx
  - py-2.5

frontend-mobile/src/pages/Success.jsx
  - pb-0.5

frontend-mobile/src/pages/Messages.jsx
  - text-white/80

frontend-mobile/src/pages/MembershipCenter.jsx
  - px-1.5, py-0.5, py-1.5(x3)

frontend-mobile/src/pages/MessageDetail.jsx
  - min-h-[120px], max-w-[60%]

frontend-mobile/src/pages/Profile.jsx
  - text-[10px], max-w-[480px]

frontend-mobile/src/pages/MyService.jsx
  - max-w-[480px]

frontend-mobile/src/pages/Onboarding.jsx
  - w-1/2, w-1/4(x2)
```

## 执行顺序

1. **更新 `tailwind.config.js`** — `blocklist: ['!visible']`（已做 ✅）
2. **保留 `@tailwind` 指令** — `app.css`（已做 ✅）
3. **批量 sed 替换小数 spacing** — 一次性处理所有 `*.5` 类名
4. **逐文件替换含 `[` `]` 类名** — 改为 `style` 内联（54 处）
5. **逐文件替换 hex 颜色** — 改为 `style` 内联（17 个值，部分与 4 重叠）
6. **创建 `weapp-post-build.sh`** — 处理 `\/`、`\%` + 移除 `*` 通用选择器
7. **构建 + 上传验证**

## 后续维护规范

- **禁止**在 className 中使用 `[-]`、`[#]`、`[/]`、`[.]`
- `bg-black/40`、`text-white/70` 等 opacity 修饰符 → 可用（post-build sed 处理）
- `active:opacity-80`、`last:border-b-0` 等变体 → **待验证**（`:active` `:last-child` 伪类在 weapp 中的支持情况不明）
- `hover:xxx`、`focus:xxx` → 禁止（weapp 不支持鼠标交互）

---

## 实战踩坑记录 (2026-07-07)

### 1. Tailwind 根本生成不了 CSS

**现象**：`app.wxss` 只有 `body{margin:0}`，页面样式全塌。

**根因**：`app.css` 缺 `@tailwind base/components/utilities` 指令 + 缺 `tailwind.config.js` 的 `content` 扫描路径。

**解决**：
- `app.css` 顶部加入 `@tailwind base; @tailwind components; @tailwind utilities;`
- 创建 `tailwind.config.js`：`content: ['./src/**/*.{js,jsx,ts,tsx}']`, `preflight: false`

### 2. 上传验证器拒绝 `\` → csso 拒绝 `/` → WXS 拒绝 `!` `.` `*` `~` `:not` `[ ]`

**完整排除过程**（每次上传一个错误，逐个踩）：

| 尝试 | 错误 | 解决 |
|------|------|------|
| 保留 `\` | `unexpected \` | 必须全量 `sed 's|\\||g'` |
| 全去 `\` | `unexpected !` | `blocklist: ['!visible']` + `sed '/\.!visible/d'` |
| 去 `\`+`!` | `unexpected *` | `sed 's/\*,::after,::before{[^}]*}::backdrop{[^}]*}//'` |
| 去 `\`+`!`+`*` | `error token [` | `[` `]` 在 CSS 中是属性选择器 → 源码改用 `style` |
| 去 `\`+`!`+`*`+`[ ]` | `error token :` | `:not()` 不支持 → `sed '/> :not/d'` |
| 去 `\`+`!`+`*`+`[ ]`+`:not` | `error token ~` | `~` 兄弟选择器不支持 → `sed '/~ view/d'` |

**最终 post-build 脚本**（`scripts/weapp-post-build.sh`）：
```bash
WXSS="dist-weapp/app.wxss"
sed -i 's|\\||g' "$WXSS"                              # 1. 去掉所有 CSS 转义
sed -i '/\.!visible/d' "$WXSS"                         # 2. 去掉 !important 变体
sed -i 's/\*,::after,::before{[^}]*}::backdrop{[^}]*}//' "$WXSS"  # 3. 去掉 Tailwind 变量块
sed -i '/> :not/d' "$WXSS"                              # 4. 去掉 :not() 伪类规则
sed -i '/> view ~/d' "$WXSS"                            # 5. 去掉 ~ 兄弟选择器规则
sed -i '/~ view/d' "$WXSS"
```

### 3. API 域名白名单

**现象**：`Taro.request` 返回 `request:fail invalid url`，所有 API 调用失败。

**根因**：微信小程序要求所有网络请求域名在后台登记。`index.weapp.js:168` 中 `apiBaseUrl` 为相对路径 `/api`，在小程序中无法解析。

**解决**：
- `apiBaseUrl` 改为绝对路径 `https://wx.cadenzayueqi.com/api`
- mp.weixin.qq.com → 开发 → 服务器域名 → request 合法域名 添加 `https://wx.cadenzayueqi.com`

### 4. API 响应数据取层错误

**现象**：调试层显示 API 调用成功（200），但页面不渲染数据。

**根因**：`Taro.request` 的 `success` 回调中 `res.data` 是完整 API 响应体 `{code: 20000, data: {list: [...]}}`，需要再取 `.data` 层。

**解决**：`r1.data?.list` → `r1.data?.data?.list`

### 5. ScrollView 必须用显式高度

**现象**：列表无法滚动，内容卡住不动。

**根因**：`ScrollView` 不能用 `flex: 1` 撑开高度，在小程序中必须显式指定 `height`。

**解决**：`style={{ height: '100%' }}` 替代 `style={{ flex: 1 }}`

### 6. 图片缩放模式差异

**现象**：`mode="aspectFill"` 在 H5 填满容器，在小程序中等高缩放导致部分图片模糊。

**根因**：`aspectFill` 在小程序中是"等比例缩放、裁剪溢出"——裁的是宽度的溢出部分，不符合"等宽展示"需求。

**解决**：改用 `mode="widthFix"`——等宽缩放、自动计算高度。

### 7. 页面导航差异

**现象**：`useNavigate()` / `react-router-dom` 在小程序中不工作。

**根因**：`react-router-dom` 在小程序构建时被 alias 到 stub 文件，URL 路由不可用。小程序使用原生页面栈导航。

**解决**：
- 跳转：`Taro.navigateTo({ url: '/pages/X/index?id=xxx' })`
- 返回：`Taro.navigateBack()`
- 获取参数：`Taro.getCurrentInstance().router?.params?.id`

### 8. CSS 不能有空格在数字和单位之间

**现象**：`style={{ marginTop: '10 px' }}` 或 `style={{ fontSize: 10 }}`（weapp 期望 rpx 或 px 后缀）。

**根因**：小程序内联样式对数值类型的处理与 H5 不同，`fontSize: 10` 在小程序中被忽略。

**解决**：所有尺寸值加 `rpx` 或 `px` 后缀：`style={{ fontSize: '10px' }}`、`style={{ height: '42px' }}`
