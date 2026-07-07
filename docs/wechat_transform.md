# 微信小程序 Tailwind 类名转义改造

## 背景

微信小程序 WXS 引擎不支持 CSS 转义字符（`\`）。Tailwind v3 生成的类名中含有 `/`（如 `bg-black/40`）和 `.`（如 `py-0.5`），在标准 CSS 中需要通过 `\/` 和 `\.` 转义，但 WXS 不处理这些转义序列，导致：
- 保留 `\` → 微信上传验证器报错 `unexpected \`
- 去掉 `\` → 选择器语义改变（如 `.top-0.5` 会被解析为两个类 `.top-0` + `.5`）

**唯一可行的方案**：在源代码中避免使用含 `/` 或 `.` 的 Tailwind 类名，替换为等效的替代写法。

## 需要替换的模式

### 模式一：opacity 修饰符（`*/number`）

`bg-black/40` → `style={{ backgroundColor: 'rgba(0,0,0,0.4)' }}`
`text-white/70` → `style={{ color: 'rgba(255,255,255,0.7)' }}`
`border-white/10` → `style={{ borderColor: 'rgba(255,255,255,0.1)' }}`
`bg-zinc-50/40` → `style={{ backgroundColor: 'rgba(250,250,250,0.4)' }}`
`bg-[#C21838]/70` → `style={{ color: 'rgba(194,24,56,0.7)' }}` 或 `opacity-70`

### 模式二：小数 spacing 值（`*.number`）

| 原类名 | 替换方案 |
|--------|---------|
| `py-0.5` | `py-1`（padding 0.125rem → 0.25rem） |
| `px-2.5` | `px-3`（padding 0.625rem → 0.75rem） |
| `w-1.5` | `w-2`（width 0.375rem → 0.5rem） |
| `h-1.5` | `h-2` |
| `gap-0.5` | `gap-1` |
| `space-x-1.5` | `space-x-2` |
| `space-y-0.5` | `space-y-1` |
| `space-y-1.5` | `space-y-2` |
| `mx-0.5` | `mx-1` |
| `my-0.5` | `my-1` |
| `-mt-0.5` | `-mt-1` |
| `top-0.5` | `top-1` |
| `right-0.5` | `right-1` |
| `bottom-0.5` | `bottom-1` |
| `left-0.5` | `left-1` |
| `py-1.5` | `py-2` |
| `px-1.5` | `px-2` |
| `pb-0.5` | `pb-1` |
| `pt-0.5` | `pt-1` |
| `p-0.5` | `p-1` |
| `m-0.5` | `m-1` |
| `mt-0.5` | `mt-1` |
| `mb-0.5` | `mb-1` |
| `w-0.5` | `w-1` |
| `h-0.5` | `h-1` |
| `scale-90` | 不变（`scale-90` 不含 `.` 或 `/`） |
| `top-1/2` | `style={{ top: '50%' }}` |

## 涉及文件清单

### opacity 修饰符（需改为 style 或 opacity-n）

```
frontend-mobile/src/pages/Home.jsx
  69:    text-[#C21838]/70
 239:    bg-black/40
 298:    bg-black/40
 306:    text-white/70
 337:    w-3/4
 338:    w-1/2
 351:    text-white/60
 380:    bg-black/20

frontend-mobile/src/pages/ShippingInterface.jsx
 270:    bg-black/50

frontend-mobile/src/pages/ReceivingInterface.jsx
 206:    bg-black/50

frontend-mobile/src/pages/StaffInstrumentDetail.jsx
 321:    bg-black/30

frontend-mobile/src/pages/Detail.jsx
 534:    bg-black/50

frontend-mobile/src/pages/CreateRepairRequest.jsx
 159:    bg-black/50
 212:    bg-black/50
 233:    bg-black/50
 248:    bg-black/50

frontend-mobile/src/pages/StaffInstrumentForm.jsx
 213:    top-1/2
 311:    bg-black/50

frontend-mobile/src/pages/Messages.jsx
  68:    text-white/80

frontend-mobile/src/pages/Checkout.jsx
 689:    bg-zinc-50/40
 719:    border-zinc-200/60
```

### 小数 spacing 值（需上调到下一个整数级别）

```
frontend-mobile/src/pages/ShippingInterface.jsx
 228:    mt-0.5

frontend-mobile/src/pages/Home.jsx
  55:    py-0.5
 257:    space-x-1.5

frontend-mobile/src/pages/StaffReceiveConfirm.jsx
 146:    space-y-0.5

frontend-mobile/src/pages/ReceivingInterface.jsx
 163:    mt-0.5

frontend-mobile/src/pages/Detail.jsx
 240:    space-x-1.5
 394:    py-1.5
 396:    px-1.5, py-0.5

frontend-mobile/src/pages/PaymentComplete.jsx
  50:    py-2.5

frontend-mobile/src/pages/Checkout.jsx
 694:    mx-0.5
 705:    py-1.5

frontend-mobile/src/pages/ReceivingRepairScan.jsx
  73:    py-1.5
  77:    py-1.5

frontend-mobile/src/pages/StaffInstrumentForm.jsx
 311:    top-0.5, right-0.5, p-0.5

frontend-mobile/src/pages/Success.jsx
  23:    pb-0.5

frontend-mobile/src/pages/OrderDetail.jsx
 246:    mt-0.5
 251:    mt-0.5
 266:    mt-0.5
 273:    mt-0.5
 290:    mt-0.5
 298:    mt-0.5
 515:    w-0.5, mt-0.5
 521:    mt-0.5
 540:    mt-0.5
 549:    mt-0.5

frontend-mobile/src/pages/MembershipCenter.jsx
 156:    px-1.5, py-0.5
 160:    py-1.5
 162:    py-1.5
 164:    py-1.5

frontend-mobile/src/pages/Onboarding.jsx
 167:    w-1/2
 172:    w-1/4
 180:    w-1/4

frontend-mobile/src/pages/Cart.jsx
 203:    px-1.5, py-0.5

frontend-mobile/src/pages/StaffOrders.jsx
?       (需 grep 检查)

frontend-mobile/src/pages/UserRepairs.jsx
?       (需 grep 检查)
```

## Z-index 自定义值（`z-[number]`）

当前用的 `z-[10000]`、`z-[10001]` 等不会被 weapp 拒绝（不含 `.` 或 `/`），不需修改。

## 改造策略

### 选项 A：手动逐文件替换（推荐）

按上面清单逐文件替换。对于 opacity 修饰符，用 `style` 内联样式替代。
对于小数 spacing，上调到下一个整数 Tailwind 类名（如 `py-0.5` → `py-1`）。

### 选项 B：整体替换工具

```bash
# 替换 opacity 修饰符（风险高，需逐条确认）
# 替换小数 spacing
sed -i 's/py-0\.5/py-1/g' frontend-mobile/src/pages/*.jsx frontend-mobile/src/components/*.jsx
sed -i 's/px-0\.5/px-1/g' frontend-mobile/src/pages/*.jsx frontend-mobile/src/components/*.jsx
sed -i 's/mt-0\.5/mt-1/g' frontend-mobile/src/pages/*.jsx frontend-mobile/src/components/*.jsx
# ... 以此类推
```

## 验证方式

所有替换完成后，重新构建并检查 WXSS：

```bash
npm run build:weapp
# 确认无 backslash 字符
grep -c '\\' dist-weapp/app.wxss   # 应为 0
# 确认无 / 字符在 selectors 中
grep -o '\.[a-z-]*/[0-9]' dist-weapp/app.wxss | head -5
# 上传测试
npm run build:weapp && sed -i 's/\\//g' dist-weapp/app.wxss && miniprogram-ci upload ...
```

## 后续维护注意事项

- 新代码中避免使用 opacity 修饰符（`bg-*/number`、`text-*/number`）
- 小数 spacing 可用但要在构建后检查 WXSS 是否包含 `\` 和 `/` 选择器
- 不要写 .jsx 文件时在 className 中使用 `w-1/2` 等含 `/` 的类
