## Accepted Plan — #2616 [Bug] weapp 购物车：勾选框对比度过低 + 「删除所选(N)」与胶囊重叠且被裁切

基线：`origin/develop` @ `bdd14b69d`（`frontend-mobile/src/pages/Cart.jsx` 共 501 行，下述行号以该基线为准）。

### 背景与现状核对（代码为据）

| # | 事实 | 位置 |
|---|---|---|
| 1 | 购物车在两端共用同一份 `.jsx` 业务实现：H5 薄壳 `export { default } from '../Cart'`，weapp 薄壳 `useIdentityGuard()` + `PageShell` 包裹同一个 `Cart` | `frontend-mobile/src/pages/cart/index.tsx`、`frontend-mobile/src/pages-weapp/cart/index.tsx` |
| 2 | weapp 购物车页是**自定义导航**（无原生导航栏），头部完全由 `Cart.jsx` 自己绘制；标题左侧「❮」返回、右侧「删除所选(N)」 | `frontend-mobile/src/pages-weapp/cart/index.config.ts:2`（`navigationStyle: 'custom'`）、`Cart.jsx:321-333` |
| 3 | 头部只预留了**状态栏**高度（`getStatusBarHeight() + 8`），完全**没有预留右上角原生胶囊**（关闭/更多按钮）的空间，且用 `justify-between` + `px-4` 无右侧额外内缩 → 右侧文本压在胶囊下，长文本被裁 | `Cart.jsx:322-325` |
| 4 | 胶囊几何唯一来源是 `Taro.getMenuButtonBoundingClientRect()`；`menu.right` 是**绝对 X 坐标**（距屏幕左缘），不是「距右缘距离」。仓库内唯一先例是首页 | `frontend-mobile/src/pages-weapp/Home.jsx:169,516` |
| 5 | 首页那个 `paddingRight: windowWidth - menuRight + 8` 只适用于**居中的窄元素**（搜索框固定 150px）：它只保证居中元素不与胶囊相交。**右对齐**元素必须用胶囊**左边缘** `menu.left`，否则文本右端仍落在胶囊区间内 | `Home.jsx:516`（对照本单数值推导） |
| 6 | 勾选框未选中态 = `#d1d5db` 2px 边框 + 透明底；页面底色 `#FDFBF7`、卡片白底 → 边框与背景几乎同色，真机看不出「可勾选」 | `Cart.jsx:386-397`、`Cart.jsx:321` |
| 7 | 可点热区就是 20×20 的视觉框本身（父列 `width: 28`、`marginTop: 24`），小于移动端最小可点尺寸 32px | `Cart.jsx:384-400` |
| 8 | 同类扫描：全仓 `navigationStyle: 'custom'` 只有 `pages-weapp/home`（已按胶囊对齐）与 `pages-weapp/cart`（本单）→ 本单只此一处，无第三页待修 | `grep -rn "navigationStyle" frontend-mobile/src/pages-weapp` |
| 9 | 同类扫描：`#d1d5db` 其余命中为消息页空态图标、续期页输入框边框、合同页按钮背景，均非「勾选选择」语义；`borderWidth: 2` + `borderRadius: 4` 的 20px 勾选框全仓只有购物车这一处 | `grep -rn "#d1d5db\|borderRadius: 4, borderWidth: 2" frontend-mobile/src` |
| 10 | 渲染测试档位可直接复用：`vitest.config.js` 只收 `scripts/page-smoke/**/*.test.jsx`；`taro-stub.js` 已提供 `getSystemInfoSync()`（`windowWidth:375`、`statusBarHeight:20`）与 `getMenuButtonBoundingClientRect()`（`top:10,height:32,right:365,left:300,width:65`），`@tarojs/components` 由 `taro-shim.js` 映射为真实 DOM；`fixtures.cartItem` 与 `localStorage['cart_user-1']` 已在 `setup.js` 预置 | `frontend-mobile/vitest.config.js`、`frontend-mobile/scripts/page-smoke/taro-stub.js:39-40`、`setup.js`、`fixtures.js:159` |
| 11 | 既有全页冒烟已把 `/cart` 列入关键页（marker `购物车`），本次改动不得让该冒烟转红 | `frontend-mobile/scripts/page-smoke/routes.js`（`H5 /cart` 条目） |

**数值推导（weapp，`windowWidth=375`，胶囊 `left=300`）**

| 量 | 公式 | 值 |
|---|---|---|
| 胶囊避让右内边距 | `windowWidth - menu.left + 8` | `375 - 300 + 8 = 83` |
| 头部上内边距 | `statusBarHeight + 8` | `20 + 8 = 28` |
| 胶囊 API 不可用兜底右内边距 | 保守常量（典型胶囊宽 87 + 8 间隙） | `96` |
| H5 右内边距 / 上内边距 | 无胶囊，保持原视觉 | `16` / `12` |

### 变更点

#### 1. `frontend-mobile/src/pages/Cart.jsx` — 头部：胶囊避让（自定义导航）

在 `Cart` 组件内、`return` 之前新增（`useMemo`/`Taro`/`env`/`getStatusBarHeight` 均已 import，无需新增依赖）：

```jsx
  // #2616: 本页 weapp 端是自定义导航（pages-weapp/cart/index.config.ts
  // navigationStyle: 'custom'），头部必须为原生胶囊预留右上角空间，否则右侧动作
  // 会压在胶囊（关闭/更多）下并被裁切。menu.left 是胶囊左边缘的绝对 X 坐标：
  // 右对齐内容需要「屏幕宽 - 左边缘」的右侧内边距（Home 的 `windowWidth - menuRight`
  // 只适用于居中窄元素）。胶囊 API 不可用时保守预留 96，宁可留白也不重叠。
  const navBar = useMemo(() => {
    if (!env.isMiniProgram) return { statusBarHeight: 0, capsuleReserveRight: 0 }
    try {
      const sys = Taro.getSystemInfoSync() || {}
      const menu = Taro.getMenuButtonBoundingClientRect ? Taro.getMenuButtonBoundingClientRect() : null
      const windowWidth = sys.windowWidth || 375
      return {
        statusBarHeight: sys.statusBarHeight || 0,
        capsuleReserveRight: menu && menu.left > 0
          ? Math.max(0, windowWidth - menu.left + 8)
          : 96,
      }
    } catch {
      return { statusBarHeight: 0, capsuleReserveRight: 96 }
    }
  }, [])
```

头部 JSX 改为（标题可让位、删除文案不换行不压缩、容器显式 `border-box` 防小程序 content-box 溢出）：

```jsx
      <View
        data-testid="cart-header"
        className="pb-2 flex items-center bg-white border-b border-zinc-100 flex-shrink-0"
        style={{
          boxSizing: 'border-box',
          width: '100%',
          paddingTop: env.isMiniProgram ? navBar.statusBarHeight + 8 : 12,
          paddingLeft: 16,
          paddingRight: env.isMiniProgram ? navBar.capsuleReserveRight : 16,
        }}
      >
        <Text className="text-xl font-bold text-black" style={{ flexShrink: 0 }} onClick={() => nav(-1)}>❮</Text>
        <Text
          data-testid="cart-header-title"
          className="text-lg font-black text-black"
          style={{ flex: '1 1 0%', minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', textAlign: 'center' }}
        >购物车</Text>
        {/* #2042: 批量删除（有选择时显示）；#2616: 不换行、不被压缩 */}
        {selected.size > 0 ? (
          <Text
            data-testid="cart-batch-delete"
            className="text-sm text-red-500 font-bold"
            style={{ flexShrink: 0, whiteSpace: 'nowrap' }}
            onClick={handleBatchDelete}
          >删除所选({selected.size})</Text>
        ) : (
          <View style={{ width: 24, flexShrink: 0 }} />
        )}
      </View>
```

- 文案、点击行为、`selected.size > 0` 的显示条件**逐字不变**（#2042 契约保持）。
- 只替换容器类与样式；`justify-between` 去掉（改由标题 `flex:1 1 0%` 吸收剩余空间，空间不足时**标题先截断**，删除文案后截断）。

#### 2. `frontend-mobile/src/pages/Cart.jsx` — 勾选框：对比度 + 可点热区

现有块（`Cart.jsx:384-400`，`{/* Checkbox column */}` 之后）替换为：

```jsx
                          {/* Checkbox column: 36×36 热区（>=32px）内居中 20×20 视觉框 */}
                          <View
                            data-testid="cart-select-hit"
                            className="flex-shrink-0 flex items-center justify-center"
                            onClick={() => !rentedOut && toggleSelect(itemId)}
                            style={{ width: 36, height: 36, marginTop: 16 }}
                          >
                            <View
                              data-testid="cart-select-box"
                              style={{
                                width: 20, height: 20, borderRadius: 4, borderWidth: 2,
                                borderColor: rentedOut ? '#E4E4E7' : (selected.has(itemId) ? '#B98E5F' : '#71717A'),
                                backgroundColor: rentedOut ? '#F4F4F5' : (selected.has(itemId) ? '#B98E5F' : '#FFFFFF'),
                                display: 'flex', alignItems: 'center', justifyContent: 'center',
                                opacity: rentedOut ? 0.6 : 1,
                              }}
                            >
                              {!rentedOut && selected.has(itemId) && <Text style={{ color: 'white', fontSize: 12, fontWeight: 'bold' }}>✓</Text>}
                            </View>
                          </View>
```

- 未选中：`#71717A`（zinc-500，对白底对比度 ≈4.8:1，满足非文本控件 3:1）2px 边框 + **白底填充** `#FFFFFF`（与页面 `#FDFBF7` / 卡片白拉开层次）。
- 选中：`#B98E5F` 填充 + 白色 `✓`（保持主色，不回归）。
- `rentedOut`（已被租出）：`#E4E4E7` 边框 + `#F4F4F5` 底 + `opacity 0.6`，与加深后的未选中态明显可区分；点击不改变选择。
- 热区 36×36（视觉框仍 20×20）；`marginTop: 16` 使视觉框顶边 = `16 + (36-20)/2 = 24`，与改造前 `24` 完全一致（垂直位置零偏移）；列宽由 28 → 36，乐器图列右移 8px（可接受的最小代价，避免负 margin 在小程序端的兼容风险）。

#### 3. 测试（新增 2 个渲染档位）

- `frontend-mobile/scripts/page-smoke/cart-checkbox-2616.test.jsx`（H5 平台真实渲染，`@testing-library/react`）：`render(<MemoryRouter><Cart/></MemoryRouter>)`，用 `fireEvent.click` 驱动选择态切换；颜色断言一律用归一化比较（`const norm=(c)=>{const d=document.createElement('div');d.style.color=c;return d.style.color}`），不得硬编码 `rgb(...)` 字面量；断言后 `console.log` 输出 token（见 Evidence Contract）。
  - 例 1 选中态：`[data-testid="cart-select-box"]` 的 `backgroundColor`/`borderColor` 归一化后等于 `norm('#B98E5F')`，且框内含 `✓`。
  - 例 2 未选中态：`fireEvent.click([data-testid="cart-select-hit"])` 后 `borderColor === norm('#71717A')`、`backgroundColor === norm('#FFFFFF')`、`✓` 消失。
  - 例 3 热区：`hit.style.width`/`height` 解析为整数 ≥ 32（实测 36）。
  - 例 4 禁用态：存储预置第二件 `stock_status` 为 `rented` 的项（`fetch` 打桩 `/public/instruments/inst-2` → `{code:20000,data:{...fixtures.instrument, stock_status:'rented'}}`），该框 `borderColor === norm('#E4E4E7')`、`backgroundColor === norm('#F4F4F5')`、无 `✓`，且 `fireEvent.click` 后仍无 `✓`（不可选）。
- `frontend-mobile/scripts/page-smoke/cart-header-2616.test.jsx`（weapp 平台真实渲染）：`vi.mock('../../src/platform')` 取 `src/platform/index.weapp` 的 actual，并把 `env.isMiniProgram` 改为**可切换 getter**（`Object.defineProperty(env,'isMiniProgram',{get:()=>state.miniProgram,configurable:true})`），从而在同一文件覆盖「weapp 真值 / H5 兜底 / 胶囊 API 失效」三种形态。
  - 例 1（`miniProgram=true`，stub 胶囊 `left=300`）：`[data-testid="cart-header"]` 的 `paddingRight === '83px'`、`paddingTop === '28px'`。
  - 例 2（`miniProgram=false`）：`paddingRight === '16px'`、`paddingTop === '12px'`（H5 无胶囊，不得引入胶囊内边距）。
  - 例 3（`vi.spyOn(Taro,'getMenuButtonBoundingClientRect').mockImplementation(()=>{throw new Error('no capsule')})`）：`paddingRight === '96px'`（兜底不得为 0，否则回归重叠）。
  - 例 4 文案完整：预置 12 件（同一 `pricing_v2`）→ `[data-testid="cart-batch-delete"]` 的 `textContent === '删除所选(12)'`，`style.flexShrink === '0'`、`style.whiteSpace === 'nowrap'`；`[data-testid="cart-header-title"]` 的 `style.flex` 含 `1 1 0%` 且 `minWidth === '0px'`（空间不足时标题先让位）。
  - 上述例 1–4 通过后 `console.log` 输出 token。

#### 4. `docs/spec/ui/ui.md` §2.7 — 购物车页设计口径（见 Core Update）

### 被翻转 / 被强化的既有断言（强制清单）

| 文件:行 | 旧期望 | 新期望 | 理由 |
|---|---|---|---|
| `frontend-mobile/src/pages/Cart.jsx:386-397`（无任何既有测试） | 无断言；未选中 `#d1d5db` + 透明底、热区 20px | 未选中 `#71717A` + 白底、热区 36px | 缺陷 1 的可验收形态；新增渲染断言 `CART_CHECKBOX_UNSELECTED_CONTRAST` / `CART_CHECKBOX_TAP_SIZE` |
| `frontend-mobile/src/pages/Cart.jsx:322-325`（无任何既有测试） | 无断言；头部只留状态栏高度 | 头部留 `windowWidth - menu.left + 8` 右侧内边距，胶囊不可用时 96 | 缺陷 2；新增渲染断言 `CART_HEADER_WEAPP_PADDING_RIGHT=83` |
| **保持**：`frontend-mobile/scripts/page-smoke/routes.js`（`H5 /cart`，marker `购物车`） | 页面渲染出标题「购物车」 | 不变（标题文本逐字保留） | 防回归：断言 `CART_HEADER_TITLE_YIELDS` 只改样式，不改文案 |
| **保持**：`frontend-mobile/src/pages/Cart.jsx:329-331`（#2042「删除所选(N)」显示条件与 `handleBatchDelete`） | `selected.size > 0` 才显示、点击走 `handleBatchDelete` | 不变 | 本单只改样式与内边距，不动行为 |
| **保持**：`Cart.jsx:126-133` `isRentedOut` + `toggleSelect` 的禁选语义（#1659） | 已被租出项不可选中、不进 `grandTotal` | 不变（新增断言 `CART_CHECKBOX_DISABLED_NOT_SELECTABLE=1` 把它锁死） | 加深未选中色后必须证明「禁用态仍不可选」 |

### Core Update

`[Core Update Required]`

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/ui/ui.md` | §2.7「购物车页 (`/cart`)」只有分组/单项删除/阶梯计价/合并对话框/下单校验/存储 key，**没有**勾选框（选择态）视觉规范、没有「删除所选(N)」批量删除说明、没有自定义导航头与胶囊的空间约束 | §2.7 的「交互」列表后新增两条：①**勾选框视觉规范**：未选中 = `#71717A` 2px 边框 + `#FFFFFF` 填充、选中 = `#B98E5F` 填充 + 白色 `✓`、`rentedOut` 禁用 = `#E4E4E7`/`#F4F4F5` + `opacity 0.6` 且不可点，可点热区 **≥32×32**（视觉框 20×20）；②**自定义导航头胶囊约束**：`navigationStyle: 'custom'` 的页（weapp `pages-weapp/cart`、`pages-weapp/home`）头部必须预留 `windowWidth - menu.left + 8` 的右侧内边距，胶囊 API 不可用时保守预留 96px；右上角动作文案单行（`whiteSpace: nowrap`）且不得被压缩，空间不足时标题先截断；③补充「删除所选(N)」（#2042）批量删除与「有选择时才显示」的既有行为 | 这两条是用户可见的 UI 契约（对比度/可点尺寸/胶囊空间），AGENTS「方法論 7」已有胶囊对齐方法论但未落到购物车页；不写进 spec 则下一个自定义导航页会重复同一缺陷。**不新增跨端差异**：勾选与头部改动对两端同源生效，weapp 仅多出胶囊内边距（已由 A14「weapp 自定义导航 + 胶囊对齐」覆盖），故不改 `docs/cases/h5-weapp-differences.md` |

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状 | 真机上（a）未选中勾选框与背景几乎同色，看不出可勾选；（b）选中后右上角「删除所选(N)」压在微信胶囊（关闭按钮）下且右侧被裁切 | Issue 正文 Owner 真机走查；`Cart.jsx:386-397`、`Cart.jsx:322-325` |
| 2 | 触发条件 | （a）常态（未选中态）；（b）`selected.size > 0`（选中任意项后出现）+ weapp 自定义导航 + 窄屏（375 宽时胶囊占右端 ~87px） | `Cart.jsx:328-333`、`pages-weapp/cart/index.config.ts` |
| 3 | 根因 | （a）未选中态取 `#d1d5db` 且无填充，与 `#FDFBF7`/白底色差不足，热区只有 20px；（b）头部按「原生导航栏不存在」只预留状态栏，未对胶囊几何建模；且仓库里唯一先例（Home 搜索框）的 `windowWidth - menuRight + 8` 只解决居中窄元素，被误当作通用公式 | `Cart.jsx:322`、`Home.jsx:516`、AGENTS「方法論 7 自定义导航栏规范」 |
| 4 | 测试盲点 | `/cart` 只在 `routes.js` 里有「渲染出『购物车』」的 marker 级冒烟；没有任何断言覆盖勾选框颜色/尺寸、也没有覆盖自定义导航头部内边距 | `routes.js`（`H5 /cart`）、`grep -rn "cart-select\|删除所选" frontend-mobile/scripts/page-smoke` = 0 |
| 5 | 同类扫描 | `navigationStyle: 'custom'` 仅 home（已对齐）+ cart（本单）；20px `borderWidth:2/borderRadius:4` 勾选框仅购物车一处；`#d1d5db` 其余命中语义不同 | 背景表第 8、9 行 |
| 6 | 修复层 | 修在**页面组件层**（唯一受影响页面）：头部按胶囊左边缘预留 + 勾选框改对比度/热区；不引入新的 platform 抽象（Rule of Three：Home 已有 inline 先例，本处为第二处；出现第三处再收敛，另立 Issue），也不改 `taro-shim`/构建链 | 变更点 1–2、Decision Lock D1/D6 |
| 7 | 红绿实证 | 红：先在**未修**实现上跑 `cart-checkbox-2616` / `cart-header-2616` → 断言失败（未选中 `borderColor` 为 `norm('#d1d5db')` ≠ `norm('#71717A')`；`paddingRight` 为 `16px` ≠ `83px`；热区 `20` < 32）；绿：实现后两条命令 exit 0 且 token 齐备。红片段写入 Work Summary `### 红绿实证` | R1/R2 命令 + Work Summary |
| 8 | 防复发 | 断言口径与实现解耦：颜色经归一化比较、内边距断言**精确像素值**（83/28/16/12/96）、热区断言数值下限；胶囊 API 失效单独一例（兜底 96，负例「不得为 0」）；禁用态单独一例（不可被点选）；长计数单独一例（`删除所选(12)` 不压缩）；并把视觉契约写入 `ui.md` §2.7 | R1/R2 的 negative 列、Core Update |
| 9 | 影响面 | 只改购物车页头部样式与勾选框样式/热区：文案、`selected` 状态机、`grandTotal`、`handleBatchDelete`、`handleCheckout`、`rentedOut` 禁选语义、价格与押金展示、`/checkout` 跳转均不变；H5 端仅勾选框颜色与热区变化，头部内边距保持 16/12 | 被翻转清单的「保持」行、R1/R2 例 2 |
| 10 | 未修边界 | 未加「全选」控件（Issue 未要求，避免扩张）；未把胶囊计算收敛为共享 platform helper（Rule of Three，见第 6 行）；未改 `app.config.ts` 的 tabBar/navigationStyle；未调整其它页面的输入框/图标灰阶 | Out of Scope |

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 勾选框对比度 + 可点热区 + 禁用态（页内选择控件的 render 断言：`render(`/`fireEvent`） | `frontend-mobile/src/pages/Cart.jsx`、`frontend-mobile/scripts/page-smoke/cart-checkbox-2616.test.jsx` | H5 `/cart`（`render(<MemoryRouter><Cart/></MemoryRouter>)`） | 初始全选 → 点击取消选择；第二项 `stock_status=rented` | H5 + weapp 共用 `.jsx`（jsdom 真实渲染） | 选中 `#B98E5F` 填充 + `✓`；未选中 `#71717A` 2px 边框 + `#FFFFFF` 填充；禁用 `#E4E4E7`/`#F4F4F5` 且点击不可选；热区 36（≥32） | `bash scripts/verify-2616.sh r1 > docs/evidence/2616/R1-cart-checkbox.txt` |
| R2 自定义导航头胶囊避让 + 长文案不裁切（页头 render 断言） | `frontend-mobile/src/pages/Cart.jsx`、`frontend-mobile/scripts/page-smoke/cart-header-2616.test.jsx` | weapp `pages-weapp/cart`（`render(<MemoryRouter><Cart/></MemoryRouter>)`，platform 取 `index.weapp`） | `miniProgram=true`（胶囊 `left=300`）/ `miniProgram=false` / 胶囊 API 抛错 / 12 件选中 | weapp（jsdom + weapp platform mock）；H5 为负例 | weapp `paddingRight=83px`、`paddingTop=28px`；H5 `16px`/`12px`；胶囊失效 `96px`；`删除所选(12)` 完整、`flexShrink=0`、`whiteSpace=nowrap`；标题 `flex:1 1 0%` + `minWidth:0` | `bash scripts/verify-2616.sh r2 > docs/evidence/2616/R2-cart-header.txt` |
| R3 weapp/H5 构建 + weapp 产物可达性 + ESLint（no-undef） | `scripts/verify-2616.sh`、`frontend-mobile/src/pages/Cart.jsx` | `frontend-mobile` 构建链（`build:weapp` / `build:h5`）、`eslint` | 修后工作树 | weapp + H5 | `build:weapp`、`build:h5` exit 0；`dist-weapp/pages-weapp/cart/index.wxml` 无原生 HTML 交互标签；weapp 产物含新样式标记 `#71717A`；ESLint `no-undef` 计数 0 | `bash scripts/verify-2616.sh r3 > docs/evidence/2616/R3-build-lint.txt` |
| R4 pre-ready gate（四源一致性 + 合同完整性 + 远端可达） | `docs/evidence/2616/pre-ready-2616.json` | `pre-ready-check.sh 2616` | 修后干净树 + 最新 Accepted Plan | weapp + H5 | exit 0、`overall=PASS`、P12 每行 `pass=true`、P13 提交在 `origin/issue-2616` | R4（gate 命令见 Evidence Contract） |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | render | `@testing-library/react` 的 `render(` / `fireEvent` / `waitFor` 在 jsdom 真实渲染购物车页并驱动选择态 |
| R2 | render | 同上，platform 切到 `index.weapp` 形态（胶囊几何由 `taro-stub` 提供，另有用例注入抛错） |
| R3 | build | `taro build --type weapp` / `build:h5` + wxml 原标签探针 + `eslint`（`no-undef`） |
| R4 | gate | `pre-ready-check.sh` 报告（P1–P17） |

### Input Truth & Tool Preflight

- **Source of truth**：
  - 胶囊几何唯一来源 = `Taro.getMenuButtonBoundingClientRect()`（`menu.left` 为胶囊左边缘绝对 X；`menu.right` 为右边缘绝对 X）；
  - 状态栏高度唯一来源 = `Taro.getSystemInfoSync().statusBarHeight`（经 `platform.getStatusBarHeight()`）；
  - 购物车业务事实（选择集、`rentedOut`、`删除所选(N)` 显示条件）唯一来源 = `frontend-mobile/src/pages/Cart.jsx` 现有状态机，本单不改；
  - 视觉口径唯一来源 = 本计划的数值推导表 + 修后的 `docs/spec/ui/ui.md` §2.7。
- **实施前差异核对（先跑，结果入 Work Summary）**：
  - `grep -n "getStatusBarHeight() + 8" frontend-mobile/src/pages/Cart.jsx` → 期望 1 命中（:323），确认基线未被他单改动；
  - `grep -n "borderColor: rentedOut ? '#e4e4e7'" frontend-mobile/src/pages/Cart.jsx` → 期望 1 命中（:390）；
  - `grep -rn "navigationStyle" frontend-mobile/src/pages-weapp` → 期望只有 `home`、`cart` 两处 `custom`；
  - `grep -c "cart-select\|删除所选" frontend-mobile/scripts/page-smoke/*.test.jsx` → 修前 0（本单新增断言）；
  - `cd frontend-mobile && npx vitest run --config vitest.config.js scripts/page-smoke/cart-checkbox-2616.test.jsx` 在**未修**实现上跑 → 必须失败（红绿实证的红例）。
- **损坏输入拒绝规则**：
  - `getMenuButtonBoundingClientRect` 返回 `null` / `left=0` / 抛异常 → 一律走 96 兜底，**禁止**把 `capsuleReserveRight` 落到 0（那正是缺陷形态）；
  - `windowWidth` 缺失 → 用 375 兜底后再算内边距；`Math.max(0, ...)` 防负值；
  - 颜色断言禁止硬编码 `rgb(...)`：必须用 DOM 归一化函数把期望 hex 转成同样的字符串形式再比较（否则 jsdom/cssstyle 归一化差异会造成假绿/假红）。
- **工具盲区**：
  - jsdom 不做真实布局，`getBoundingClientRect()` 全为 0 → **不得**用「两元素矩形是否相交」证明胶囊不重叠；本单以「头部右侧内边距 = `windowWidth - menu.left + 8`」这一可判定等式 + 胶囊左边缘常量（stub `left=300`）为该约束的等价证据；
  - `taro-stub` 的胶囊坐标为固定合成值（375/300/65），只验证我方算术与接线，不验证真机胶囊像素值；
  - `build:weapp` 只证明产物可生成且含新样式，不证明真机渲染；真机回执按 `waitForConfirm` 另记（本单矩阵不承载）。
- **契约影响**：不改后端 API、不改 `docs/spec/api/api.md`；`docs/spec/ui/ui.md` 变更按 `## Core Update` 执行（pre-ready P7「契约三向核对」对 API 无影响，Summary 需注明）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 胶囊避让公式：右对齐内容用 `windowWidth - menu.left + 8`；**明确弃用** Home 的 `windowWidth - menu.right + 8`（那只对居中窄元素成立，会在本页留出不足） | 本计划锁定 |
| D2 | 胶囊 API 不可用/异常时兜底 `96px`（保守留白），**不得**退化为 0 | 本计划锁定 |
| D3 | 勾选框色板：未选中边框 `#71717A`（对白底 ≈4.8:1，满足非文本控件 3:1）+ 白底 `#FFFFFF`；选中 `#B98E5F` + 白 `✓`；禁用 `#E4E4E7`/`#F4F4F5` + `opacity 0.6`。三者必须互相可区分 | 本计划锁定 |
| D4 | 可点热区 36×36（视觉框保持 20×20，`marginTop: 16` 保证视觉框顶边仍为 24）；列宽 28 → 36，允许乐器图列右移 8px；**不使用负 margin**（小程序的 negative margin 兼容性风险高于 8px 位移） | 本计划锁定 |
| D5 | 头部布局：标题 `flex:1 1 0%` + `minWidth:0` 先让位，删除文案 `flexShrink:0` + `whiteSpace:nowrap` 后让位；容器显式 `boxSizing:'border-box'`（小程序 View 默认 content-box，`width:100%`+padding 会溢出） | 本计划锁定 |
| D6 | 不新增 platform 抽象：胶囊计算留在 `Cart.jsx`（Rule of Three：Home 已有 inline 实现，本处为第二处）。若后续出现第三处自定义导航页 → 另立 Issue 收敛为共享 helper，本单不做 | 本计划锁定 |
| D7 | 行为面逐字不变：`selected` 状态机、`rentedOut` 禁选、`handleBatchDelete`、`handleCheckout`、`grandTotal`、价格/押金展示、`/checkout` 跳转、`/instrument` 跳转、空车态 | 本计划锁定 |
| D8 | 不改 `pages-weapp/cart/index.config.ts`（继续 `custom`），不改 `app.config.ts` 的页面注册与 tabBar | 本计划锁定 |
| D9 | 颜色断言必须经 DOM 归一化（`element.style.color = hex` 再比较），禁止硬编码 `rgb(...)`；内边距断言用精确像素（`'83px'` 等） | 本计划锁定 |
| D10 | 重命令（`build:weapp` / `build:h5` / `vitest` / `eslint`）经 `scripts/heavy-run.sh` 取锁；`scripts/verify-2616.sh` 的 `gate` 模式只跑本单渲染用例 + 定向 eslint，不含全量构建（全量构建在 R3 一次性产出） | 本计划锁定 |
| D11 | 依赖与并行：无前置 Issue；若执行期发现 `Cart.jsx` 同 hunk 被他人改动（含 `#2042`/`#1659` 语义）→ HOLD 并回 Architect，禁止就地改写他人语义 | 本计划锁定 |

### Out of Scope

- 「全选」控件、批量操作条的重新设计（Issue 未要求；删除入口保持在头部，只修重叠与裁切）。
- 其它页面的 `navigationStyle: 'custom'` 改造（仅 home 已对齐；无第三页）。
- 把胶囊计算收敛为共享 `platform` helper（Rule of Three，见 D6）。
- 购物车行内其它灰阶（如 `#d1d5db` 在续期页输入框、消息页图标的用法）：语义不同，不在本单。
- 购物车行布局的其它视觉问题（列宽变化 8px 之外的间距、字号、图标）：本单只做上述两处缺陷。
- 真机像素级复看（需微信后台体验版分发与人工回执）：属部署侧 `waitForConfirm` 事项，在 Issue 收尾时按流程另记，不写进本矩阵。
- 后端/API 变更（购物车为纯前端本地存储）与 `docs/spec/api/api.md`。

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | `setup.js` 预置 `localStorage['cart_user-1'] = {items:[fixtures.cartItem]}`（`stock_status=available`）；用例内追加第二件 `inst-2` 且 `fetch` 对 `/public/instruments/inst-2` 返回 `stock_status:'rented'` | H5 `/cart`：`render(<MemoryRouter><Cart/></MemoryRouter>)` + `fireEvent.click([data-testid="cart-select-hit"])` | `docs/evidence/2616/R1-cart-checkbox.txt` | `CART_CHECKBOX_SELECTED_FILL=1`、`CART_CHECKBOX_UNSELECTED_CONTRAST=1`、`CART_CHECKBOX_UNSELECTED_WHITE_BG=1`、`CART_CHECKBOX_DISABLED_DISTINCT=1`、`CART_CHECKBOX_DISABLED_NOT_SELECTABLE=1`、`CART_CHECKBOX_TAP_SIZE=36`、`CART_CHECKBOX_RENDER_FAIL=0` | 未选中边框仍归一化为 `#d1d5db`、未选中无白底、热区 <32、`rentedOut` 项可被点选（出现 `✓`/计入选择）→ FAIL |
| R2 | weapp platform mock（`env.isMiniProgram` 可切换）+ `taro-stub` 胶囊 `left=300,right=365`、`windowWidth=375`、`statusBarHeight=20`；第二组注入 `getMenuButtonBoundingClientRect` 抛错；第三组预置 12 件 | weapp `pages-weapp/cart`：`render(<MemoryRouter><Cart/></MemoryRouter>)`（三组用例） | `docs/evidence/2616/R2-cart-header.txt` | `CART_HEADER_WEAPP_PADDING_RIGHT=83`、`CART_HEADER_WEAPP_PADDING_TOP=28`、`CART_HEADER_H5_PADDING_RIGHT=16`、`CART_HEADER_H5_PADDING_TOP=12`、`CART_HEADER_FALLBACK_PADDING_RIGHT=96`、`CART_HEADER_DELETE_TEXT_FULL=1`、`CART_HEADER_DELETE_NO_SHRINK=1`、`CART_HEADER_TITLE_YIELDS=1`、`CART_HEADER_RENDER_FAIL=0` | 胶囊失效时右内边距为 0、H5 也套上胶囊内边距、`删除所选(12)` 文本不完整/被 `flexShrink` 压缩、标题未让位（`flex` 不含 `1 1 0%`）→ FAIL |
| R3 | 修后工作树（`node` 经 `~/.nvm/nvm.sh` 切 22；不可用则 exit 2，绝不静默通过） | `bash scripts/verify-2616.sh r3`（`build:weapp` + `build:h5` + wxml 探针 + `eslint`） | `docs/evidence/2616/R3-build-lint.txt` | `WEAPP_BUILD_OK=1`、`H5_BUILD_OK=1`、`WXML_CART_NATIVE_TAG=0`、`WEAPP_BUNDLE_MARKER=1`、`ESLINT_NOUNDEF=0` | 构建非 0、`dist-weapp/pages-weapp/cart/index.wxml` 含 `div/span/img/button/input/textarea/p/select/a`、weapp 产物不含 `#71717A`、ESLint 出现 `no-undef` → FAIL |
| R4 | 修后干净树 + 最新 Accepted Plan 评论 + `docs/evidence/2616/evidence.json`（`dirty=false`） | `pre-ready-check.sh 2616`（canonical 四参） | `docs/evidence/2616/pre-ready-2616.json` | exit 0、`overall=PASS`、P12 对 evidence.json 每行 `pass=true`、P13 提交在 `origin/issue-2616`、P17 plan 快照与评论一致 | 任一 P 项失败 / evidence 缺行 / `dirty=true` / plan 快照不一致 → gate 非 0，禁止 ready |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2616.sh r1 > docs/evidence/2616/R1-cart-checkbox.txt` | `docs/evidence/2616/R1-cart-checkbox.txt` | exit 0 且含 `CART_CHECKBOX_SELECTED_FILL=1`、`CART_CHECKBOX_UNSELECTED_CONTRAST=1`、`CART_CHECKBOX_UNSELECTED_WHITE_BG=1`、`CART_CHECKBOX_DISABLED_DISTINCT=1`、`CART_CHECKBOX_DISABLED_NOT_SELECTABLE=1`、`CART_CHECKBOX_TAP_SIZE=36`、`CART_CHECKBOX_RENDER_FAIL=0` | 七项 token 全为期望值 | 未选中仍为 `#d1d5db`、无白底填充、热区 <32、禁用项可被选中、渲染抛错 → FAIL | dev |
| R2 | `bash scripts/verify-2616.sh r2 > docs/evidence/2616/R2-cart-header.txt` | `docs/evidence/2616/R2-cart-header.txt` | exit 0 且含 `CART_HEADER_WEAPP_PADDING_RIGHT=83`、`CART_HEADER_WEAPP_PADDING_TOP=28`、`CART_HEADER_H5_PADDING_RIGHT=16`、`CART_HEADER_H5_PADDING_TOP=12`、`CART_HEADER_FALLBACK_PADDING_RIGHT=96`、`CART_HEADER_DELETE_TEXT_FULL=1`、`CART_HEADER_DELETE_NO_SHRINK=1`、`CART_HEADER_TITLE_YIELDS=1`、`CART_HEADER_RENDER_FAIL=0` | 九项 token 全为期望值 | 胶囊失效兜底为 0、H5 误套胶囊内边距、长文案被裁/被压缩、标题不让位 → FAIL | dev |
| R3 | `bash scripts/verify-2616.sh r3 > docs/evidence/2616/R3-build-lint.txt` | `docs/evidence/2616/R3-build-lint.txt` | exit 0 且含 `WEAPP_BUILD_OK=1`、`H5_BUILD_OK=1`、`WXML_CART_NATIVE_TAG=0`、`WEAPP_BUNDLE_MARKER=1`、`ESLINT_NOUNDEF=0` | 五项 token 全为期望值 | weapp/h5 构建非 0、cart 页 wxml 含原生 HTML 标签、产物无新样式标记、ESLint `no-undef` → FAIL | dev |
| R4 | `~/.config/opencode/scripts/pre-ready-check.sh 2616 --summary-file docs/evidence/2616/summary.md --changed-files docs/evidence/2616/changed-files.txt --evidence-file docs/evidence/2616/evidence.json --affected-cmd "bash scripts/verify-2616.sh gate" --report docs/evidence/2616/pre-ready-2616.json` | `docs/evidence/2616/pre-ready-2616.json` | exit 0 且 `jq -e '.overall == "PASS"' docs/evidence/2616/pre-ready-2616.json` 成立；P12 对 `docs/evidence/2616/evidence.json` 每行 `pass=true` | `overall=PASS` 且证据行全 `pass=true` | 任一 P 项失败 / evidence 缺行 / `dirty=true` / plan 快照与 Accepted Plan 评论不一致 → gate 非 0，禁止 ready | dev |

### Artifact Provenance

- 生成器：`scripts/verify-2616.sh`（`r1` / `r2` / `r3` / `gate`）；`evidence.json` 与 `summary.md` / `changed-files.txt` 由 `/work` 按合同产出。
- 每个文本 artifact 首行必须为 `# build=1.0.<fix 短码> # commit=<40 位 sha> # dirty=false`，其下一行 `GENERATED_AT_HEAD=<40 位 sha>`；锚点为本单 fix commit（可用 `FIX_COMMIT` 覆盖），**证据必须在 fix commit 之后生成**（禁止只改 `evidence.json` 的 build/commit 冒充新证据）。
- 逐条清单（全部位于 `docs/evidence/2616/`）：
  - `docs/evidence/2616/R1-cart-checkbox.txt`
  - `docs/evidence/2616/R2-cart-header.txt`
  - `docs/evidence/2616/R3-build-lint.txt`
  - `docs/evidence/2616/evidence.json`
  - `docs/evidence/2616/pre-ready-2616.json`
  - `docs/evidence/2616/summary.md`
  - `docs/evidence/2616/changed-files.txt`
- `evidence.json` 必含键：`schema_version`、`issue`、`build`、`commit`、`dirty`、`plan_sha256`、`environment`、`rows[]`（每行 `id`/`command`/`exit`/`artifacts`/`assert`/`actual`/`expected`/`negative`/`negative_result`）。
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Files（变更文件清单）

- `frontend-mobile/src/pages/Cart.jsx`
- `frontend-mobile/scripts/page-smoke/cart-checkbox-2616.test.jsx`
- `frontend-mobile/scripts/page-smoke/cart-header-2616.test.jsx`
- `scripts/verify-2616.sh`
- `docs/spec/ui/ui.md`
- `docs/evidence/2616/plan.md`
- `docs/evidence/2616/R1-cart-checkbox.txt`
- `docs/evidence/2616/R2-cart-header.txt`
- `docs/evidence/2616/R3-build-lint.txt`
- `docs/evidence/2616/evidence.json`
- `docs/evidence/2616/pre-ready-2616.json`
- `docs/evidence/2616/summary.md`
- `docs/evidence/2616/changed-files.txt`

### Workload

- Size: M
- Basis: 1 个实现文件（`Cart.jsx` 两处 hunk）+ 2 个渲染测试文件 + 1 个证据脚本 + 1 份核心文档；4 条证据行（R1–R4）；无 migration、无 API/DB 变更、无跨仓依赖
- Confidence: high

### Dependencies

无

*Model: deepseek/deepseek-flash*

