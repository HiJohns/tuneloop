# 微信小程序 Tailwind 类名转义改造

## 背景

微信小程序 WXS 引擎不支持 CSS 转义字符（`\`）。Tailwind v3 生成的类名中含有特殊字符，在标准 CSS 中需要通过 `\` 转义，但 WXS 不处理这些转义序列。

实测编译产物 `app.wxss` 中共有 **184 个** `\` 转义字符，分 **8 种类型**：

| 转义类型 | 数量 | 来源 | 去掉 `\` 后能否工作 |
|---------|:---:|------|:---:|
| `\[` | 46 | 任意值 `w-[250px]`、`z-[10000]` | ✅ `[` 非 CSS 特殊字符 |
| `\]` | 46 | 同上 | ✅ `]` 非 CSS 特殊字符 |
| `\/` | 27 | opacity 修饰符 `bg-black/40`、分数 `w-1/2` | ✅ `/` 非 CSS 特殊字符 |
| `\.` | 25 | 小数 spacing `py-0.5` | ❌ `.` 是类选择器分隔符 |
| `\#` | 19 | 任意 hex `bg-[#5A3B24]` | ❌ `#` 是 ID 选择器 |
| `\:` | 17 | 变体 `hover:bg-blue-600` | ❌ `:` 是伪类分隔符 |
| `\%` | 3 | 任意百分比 `max-w-[60%]` | ✅ `%` 非 CSS 特殊字符 |
| `\!` | 1 | important `!visible` | ❌ `!` 非法开头 |

## 改造策略

分三层处理，避免逐个替换 200+ 处类名：

### 第一层：Tailwind 配置（消除 `\!` 和 `\:`）

**File**: `frontend-mobile/tailwind.config.js`

```js
module.exports = {
  content: ['./src/**/*.{js,jsx,ts,tsx}'],
  corePlugins: {
    preflight: false,
  },
  // 禁用 !important 前缀类（消除 \!）
  blocklist: ['!visible', '!block', '!hidden', '!flex', '!grid'],
  // 禁用 weapp 不支持的伪类变体（消除 \:）
  variants: {
    extend: {
      backgroundColor: ['active', 'disabled'],
      borderColor: ['active', 'disabled'],
      textColor: ['active', 'disabled'],
      opacity: ['active', 'disabled'],
    },
  },
}
```

> 注：weapp 无鼠标，不支持 `:hover`、`:focus`。`:active`（触摸按下）和 `:disabled` 可用。
> `last:` 和 `sm:` 变体不含 `\:` 转义（编译为 `.last\:xxx:last-child`），weapp 支持 `:last-child`。

### 第二层：Post-build sed（处理 `\[`、`\]`、`\/`、`\%`）

构建后执行脚本，去掉这 4 类转义。这些字符去掉 `\` 后在 CSS 中不是特殊字符，WXS 引擎可以正确匹配。

**File**: `frontend-mobile/scripts/weapp-post-build.sh`（新文件）

```bash
#!/bin/bash
# 去掉 WXSS 中的 \[ \] \/ \% 转义（WXS 不支持 \）
# 保留 \. \# \: \! 的处理在源码层完成
WXSS="dist-weapp/app.wxss"
if [ -f "$WXSS" ]; then
  sed -i 's/\\\[/[/g; s/\\\]/]/g; s/\\\//g; s/\\%/%/g' "$WXSS"
  echo "Stripped [ ] / % escapes from $WXSS"
fi
```

### 第三层：源码替换（处理 `\.`、`\#`）

这两类去掉 `\` 后会改变 CSS 语义，必须在源码中替换。

#### 3a. 小数 spacing（`\.` → 整数级）

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

批量替换命令：
```bash
cd frontend-mobile/src
sed -i 's/py-0\.5/py-1/g; s/px-0\.5/px-1/g; s/mt-0\.5/mt-1/g; s/mb-0\.5/mb-1/g; s/mx-0\.5/mx-1/g; s/p-0\.5/p-1/g; s/w-0\.5/w-1/g; s/h-0\.5/h-1/g; s/pb-0\.5/pb-1/g; s/pt-0\.5/pt-1/g; s/top-0\.5/top-1/g; s/right-0\.5/right-1/g; s/bottom-0\.5/bottom-1/g; s/left-0\.5/left-1/g; s/-mt-0\.5/-mt-1/g; s/px-1\.5/px-2/g; s/py-1\.5/py-2/g; s/mt-1\.5/mt-2/g; s/space-x-1\.5/space-x-2/g; s/space-y-0\.5/space-y-1/g; s/space-y-1\.5/space-y-2/g; s/px-2\.5/px-3/g; s/py-2\.5/py-3/g; s/py-3\.5/py-4/g; s/w-1\.5/w-2/g; s/h-1\.5/h-2/g; s/w-2\.5/w-3/g; s/h-2\.5/h-3/g' pages/*.jsx components/*.jsx
```

#### 3b. 任意 hex 颜色（`\#` → `style` 内联）

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

> 这 17 个 hex 值需逐个在源码中替换为 `style` 内联。涉及 18 个文件。

## 完整文件清单

### 需要修改的文件（18 个）

```
frontend-mobile/src/components/BottomNav.jsx
  - bg-[#5A3B24], border-[#4E321E], border-[#5A3B24]
  - text-[10px], text-[9px], min-w-[16px]

frontend-mobile/src/pages/Home.jsx
  - bg-[#5A3B24], bg-[#915F38], bg-[#FDF4E7], bg-[#FDFBF7]
  - bg-[#8A2BE2], bg-[#0084FF], bg-[#FF6B00]
  - bg-[#C21838], bg-[#002140], bg-[#FF2A55], bg-[#B98E5F]
  - text-[#C21838], text-[#FF2A55], text-[1.4rem], text-[26px]
  - leading-[1.6rem], w-[250px], w-[72px], h-[42px], h-[72px]
  - z-[10000], z-[10001], z-[10002], z-[100], z-[40], z-[5], z-[1]
  - bg-black/40, bg-black/20, text-white/70, text-white/60
  - bg-[#5A3B24]/80, text-[#C21838]/70
  - w-1/2, w-3/4
  - py-0.5, space-x-1.5
  - from-[#FDF4E7]

frontend-mobile/src/pages/Detail.jsx
  - text-[10px], pb-[140px], space-x-1.5
  - py-1.5, px-1.5, py-0.5
  - bg-black/50
  - max-h-[80%] (通过 sed 处理)

frontend-mobile/src/pages/Checkout.jsx
  - text-[10px], text-[11px], max-w-[480px]
  - bg-zinc-50/40, border-zinc-200/60
  - py-1.5, mx-0.5

frontend-mobile/src/pages/OrderDetail.jsx
  - text-[10px], max-w-[480px]
  - mt-0.5 (×10), w-0.5

frontend-mobile/src/pages/Cart.jsx
  - text-[10px], text-[11px], max-w-[140px]
  - max-w-[60%], max-w-[90%], max-h-[80%]
  - px-1.5, py-0.5

frontend-mobile/src/pages/StaffInstrumentForm.jsx
  - top-1/2, bg-black/50
  - top-0.5, right-0.5, p-0.5

frontend-mobile/src/pages/CreateRepairRequest.jsx
  - bg-black/50 (×4)

frontend-mobile/src/pages/ShippingInterface.jsx
  - bg-black/50, mt-0.5

frontend-mobile/src/pages/ReceivingInterface.jsx
  - bg-black/50, mt-0.5

frontend-mobile/src/pages/StaffInstrumentDetail.jsx
  - bg-black/30

frontend-mobile/src/pages/StaffReceiveConfirm.jsx
  - space-y-0.5

frontend-mobile/src/pages/ReceivingRepairScan.jsx
  - py-1.5 (×2)

frontend-mobile/src/pages/PaymentComplete.jsx
  - py-2.5

frontend-mobile/src/pages/Success.jsx
  - pb-0.5

frontend-mobile/src/pages/Messages.jsx
  - text-white/80

frontend-mobile/src/pages/MembershipCenter.jsx
  - px-1.5, py-0.5, py-1.5 (×3)

frontend-mobile/src/pages/MessageDetail.jsx
  - min-h-[120px], max-w-[60%]

frontend-mobile/src/pages/Profile.jsx
  - text-[10px], max-w-[480px]

frontend-mobile/src/pages/MyService.jsx
  - max-w-[480px]

frontend-mobile/src/pages/Onboarding.jsx
  - w-1/2, w-1/4 (×2)
```

## 执行顺序

1. **更新 `tailwind.config.js`** — 添加 blocklist + 禁用 hover/focus 变体
2. **批量 sed 替换小数 spacing** — 一次性处理所有 `*.5` 类名
3. **逐文件替换 hex 颜色** — 17 个 hex 值改为 `style` 内联（需手动，因为要合并到已有的 `style` 属性）
4. **创建 `weapp-post-build.sh`** — 构建后自动处理 `\[ \] \/ \%` 转义
5. **更新 `Makefile`** — 在 `build:weapp` 后自动执行 post-build 脚本
6. **构建 + 上传验证**

## 验证方式

```bash
# 构建后检查
npm run build:weapp
bash scripts/weapp-post-build.sh

# 确认无 \ 字符
grep -c '\\' dist-weapp/app.wxss   # 应为 0

# 确认无 ! 在选择器中
grep -o '\.!' dist-weapp/app.wxss  # 应为空

# 上传测试
miniprogram-ci upload --pp dist-weapp ...
```

## 后续维护规范

- **禁止**在 className 中使用 `bg-[#xxx]`、`text-[#xxx]` → 用 `style` 内联
- **禁止**使用 `py-0.5` 等小数 spacing → 用 `py-1` 等整数
- **禁止**使用 `bg-black/40` 等 opacity 修饰符 → 用 `style={{ backgroundColor: 'rgba(...)' }}`
- **禁止**使用 `w-1/2` 等分数 → 用 `style={{ width: '50%' }}`
- **禁止**使用 `hover:` `focus:` 变体 → weapp 不支持
- `z-[10000]`、`w-[250px]` 等不含 `.` `#` 的任意值 → 可用（post-build sed 处理）
- `active:` `disabled:` `last:` 变体 → 可用
