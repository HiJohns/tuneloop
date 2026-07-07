# 微信小程序 Tailwind 类名转义改造

## 背景

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
