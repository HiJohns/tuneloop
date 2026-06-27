# 首页实现技术文档（Frontpage Architecture）

> 来源：Issue #1045 多轮迭代
> 文件：`frontend-mobile/src/pages/Home.jsx`
> 框架：React + Taro + Tailwind CSS

---

## 一、架构概述

首页采用五层 Z 轴分解架构，每一层职责单一、互不干扰：

```
  Z=10000  ┌─────────────────────────┐  A: 搜索条（absolute top-0, transparent）
  Z=10001  ├─────────────────────────┤  粘连菜单（fixed top-62px, 仅 menuStuck 时显示）
  Z=100    ├─────────────────────────┤  B: 裁剪容器（fixed, overflow:hidden）
           │  ┌───────────────────┐  │    ├── C: ScrollView（flex-1, 列表滚动）
           │  │ 自然菜单 (opacity) │  │    └── BottomNav（底条，B 层内 flex 子元素）
           │  │ 乐器列表 Cards     │  │
           │  └───────────────────┘  │
  Z=5      ├─────────────────────────┤  E: 磨砂背板（fixed inset-0, scroll 后 backdrop-blur）
  Z=0      └─────────────────────────┘  D: 轮换图（fixed inset-0, 全屏背景）
```

| 层 | Z-index | 定位 | 背景 | 职责 |
|----|---------|------|------|------|
| D | 0 | `fixed inset-0` | 轮换图 + 背景色 | 视觉底层，全屏轮换背景 |
| E | 5 | `fixed inset-0` | `透明 → backdrop-blur-md` | 滚动后提供统一磨砂背板 |
| B+C | 100 | `fixed top-94px bottom-0` | `overflow:hidden` | 裁剪内容 + 包底条 |
| 粘连菜单 | 10001 | `fixed top-62px` | `bg-transparent` | menuStuck 时接管菜单可见性 |
| A | 10000 | `absolute top-0` | `bg-transparent` | 始终透明搜索条 |

---

## 二、关键技术

### 2.1 B 层裁剪容器：`overflow:hidden` + `pointer-events:none`

**问题**：列表向上滚动时，卡片内容透过透明的搜索条和菜单条可见。

**思路历程**：
1. 第一阶段：各层独立毛玻璃（A/B/C 各自 `backdrop-blur-md`），导致多叠毛玻璃视觉怪异
2. 第二阶段：统一磨砂层 E + 纯色遮挡，颜色过重不自然
3. 第三阶段：`clip-path` 动态裁剪，坐标计算复杂且与 `transform` 存在渲染管线顺序问题
4. **最终方案**：B 层作为裁剪容器，`overflow:hidden` 从物理上阻断超出菜单下缘的内容

**实现**：

```jsx
// B 层：裁剪容器
<View className="fixed left-0 right-0 z-[100] flex flex-col"
  style={{ top: '94px', bottom: 0, overflow: 'hidden', pointerEvents: 'none' }}>

  // C 层：ScrollView（显式恢复触摸事件接收）
  <ScrollView className="flex-1 overflow-y-auto"
    style={{ pointerEvents: 'auto' }}>
    ...
  </ScrollView>

  // BottomNav 在 B 层内，不受裁剪影响
  <BottomNav ... />
</View>
```

**核心技巧**：
- `pointer-events: none` 让 B 层本身不拦截触摸事件，事件穿透到 ScrollView
- ScrollView 上显式 `pointer-events: auto` 恢复触摸接收
- BottomNav 放在 B 层内部作为 flex 子元素，确保不被列表遮挡

### 2.2 菜单双层方案

**问题**：菜单需要初始在列表上方（~240px），随列表滚动，碰到搜索条底部（62px）时粘住。但 B 层 `overflow:hidden` 会裁掉到达 sticky 位置的菜单。

**方案**：双层菜单——自然菜单 + 粘连菜单，在不同滚动阶段切换。

```
scrollY < 150：自然菜单可见（B 层内，opacity-0 时隐藏）
scrollY ≥ 150：粘连菜单接管（B 层外，fixed top-62px z-10001）
```

**实现**：

```jsx
// 状态：menuStuck = scrollY >= 150
const menuStuck = scrollY >= 150

// 粘连菜单：B 层外，z-10001 > 搜索条 z-10000，不被遮盖
{menuStuck && (
  <View className="fixed left-0 right-0 z-[10001] bg-transparent"
    style={{ top: '62px' }}>
    <MenuContent ... />
  </View>
)}

// 自然菜单：B 层内，menuStuck 时 opacity-0 不可见
<View className={menuStuck ? 'opacity-0' : 'bg-transparent'}>
  <MenuContent ... scrolled={true} />
</View>
```

**关键点**：
- 粘连菜单阈值设为 150（而非 178），早于 B 层裁切自然菜单的时机（~146px），避免裁切间隙
- 自然菜单用 `opacity-0` 而非条件渲染（`{!menuStuck && ...}`），避免 DOM 增减导致内容高度突变和 `scrollY` 反弹闪烁

### 2.3 E 层统一磨砂背板

**问题**：搜索条、菜单条、列表背景各自毛玻璃 → 叠加视觉怪异。

**方案**：单一 E 层覆盖全视口，滚动时转为磨砂，为所有透明层提供统一模糊背景。

```jsx
<View className={`fixed inset-0 z-[5] transition-colors duration-300
  ${scrolled ? 'bg-[#5A3B24]/15 backdrop-blur-md' : 'bg-transparent'}`} />
```

**优势**：
- 仅一层 `backdrop-blur`，渲染性能好
- 滚动时统一模糊 D 层轮换图，列表卡片白色背景自然凸显
- 未滚动时不模糊，轮换图清晰可见

### 2.4 轮换图 D 层

**特点**：
- `fixed inset-0 z-0`，全屏背景
- 图片 220px 高，`mode="aspectFill"` 裁剪填充，不受图片原始比例影响
- 下方 `flex-1` 区域填充 `bg_color`，撑满视口高度
- 圆点 `scrollY > 0` 时隐藏

**滑动切换**：

```jsx
// 透明触摸拦截层：z-60 覆盖轮换图区域
<View className="absolute top-0 left-0 right-0 z-[60]" style={{ height: 240 }}
  onTouchStart={e => { bannerTouchStartXRef.current = e.touches[0].clientX }}
  onTouchEnd={e => {
    const diff = e.changedTouches[0].clientX - bannerTouchStartXRef.current
    if (Math.abs(diff) > 50) {
      // 滑动切换
    } else {
      // 点击跳转 banner 链接
    }
  }}
/>
```

**自动轮播**：`setInterval` 每 4 秒切换，`useEffect` 管理生命周期。

### 2.5 搜索条 A 层

搜索条始终透明，无自身背景。滚动后搜索框微调边框透明度以适应 E 层磨砂背景：

```jsx
<View className="absolute top-0 left-0 right-0 z-[10000] pt-3 pb-2 px-6">
  <View className={`w-[250px] h-[42px] mx-auto rounded-full flex items-center px-4 shadow-sm
    ${scrolled
      ? 'bg-white/20 backdrop-blur-sm border border-white/10'
      : 'bg-white/10 backdrop-blur-sm border border-white/30'}`}>
    ...
  </View>
</View>
```

---

## 三、滚动状态管理

```jsx
const [scrollY, setScrollY] = useState(0)
const scrolled = scrollY > 0        // 控制圆点隐藏、E 层磨砂
const menuStuck = scrollY >= 150    // 控制菜单粘连切换
```

**关键常量**：

| 常量 | 值 | 含义 |
|------|-----|------|
| 搜索条高度 | 62px | `pt-3`(12) + `h-[42px]`(42) + `pb-2`(8) |
| 菜单自然位置 | 240px | B 层顶(94) + spacer(146) |
| B 层顶部 | 94px | 搜索条(62) + 菜单高(~32) |
| menuStuck 阈值 | 150px | 早于 B 裁切点(146) 4px |

---

## 四、常见误区与规避

### 4.1 不要用 `position:fixed` 的 ScrollView

`fixed` 定位 ScrollView 会导致内部触摸滚动失效。B 层用 `fixed` 包围 ScrollView，ScrollView 内部用 `flex-1` 或 `absolute`。

### 4.2 不要用 `clip-path` + `transform`

两者在 CSS 渲染管线中的处理顺序不确定，裁剪可能在变换之前，导致坐标错误。用 `overflow:hidden` 包裹更简单可靠。

### 4.3 不要动态改变内容高度

隐藏菜单时用 `opacity-0` 而非条件渲染（`{!condition && <Element/>}`），后者会导致 ScrollView 内容高度突变、`scrollY` 反弹、菜单闪烁来回切换。

### 4.4 `pointer-events: none` 会继承到子元素

B 层设置 `pointer-events:none` 后，其子元素也会失去触摸事件。必须显式给需要滚动的子元素设置 `pointer-events: auto`。

### 4.5 sticky 定位在 ScrollView 内部可能偏移

Taro 的 ScrollView 组件内部使用 CSS sticky 时，定位基准是 ScrollView 自身视口而非外层容器。当 ScrollView 在一个 offset 不为 0 的容器内时，需计算偏移量。

---

## 五、文件结构

```
frontend-mobile/src/pages/Home.jsx
├── InstrumentCard      # 乐器卡片子组件
├── parseImages()       # 图片解析工具
├── getDailyRate()      # 日租金计算
├── MenuContent()       # 菜单条子组件（支持触摸滑动）
└── Home()              # 首页主组件
```

---

## 六、扩展指南

### 添加新的前置覆盖层

在 E 层之前插入新层，设置对应 Z-index：
```jsx
<View className="fixed inset-0 z-[3]" style={{ ... }}>
```

### 修改裁切位置

改变 B 层的 `top` 值和 spacer 高度：
```jsx
// B 层 top
style={{ top: 'NEW_VALUEpx', ... }}

// Spacer
<View style={{ height: 'NEW_VALUEpx' }}></View>

// menuStuck 阈值相应调整
const menuStuck = scrollY >= (NEW_SPACER + 4)
```

### 适配不同底条高度

修改 B 层内 BottomNav 的 `py-2` 或整体高度，B 层会自动适配（`flex-col` 布局）。

---

*适用于：需要背景层 + 滚动内容层 + 透明覆盖层的复杂列表页面。核心思想：Z 轴分解 + overflow:hidden 裁剪 + pointer-events 穿透 + 双层菜单切换。*
