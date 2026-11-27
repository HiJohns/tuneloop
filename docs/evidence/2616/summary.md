## Work Summary — #2616 [Bug][weapp] 购物车：勾选框对比度过低 + 「删除所选(N)」与胶囊重叠且被裁切

### 修改内容

1. **勾选框对比度 + 可点热区**（`Cart.jsx` 勾选列）：未选中态由 `#d1d5db` 2px 边框 + 透明底改为 `#71717A` 2px 边框 + `#FFFFFF` 填充（对白底 ≈4.8:1，满足非文本控件 3:1）；选中态保持 `#B98E5F` 填充 + 白色 `✓`；`rentedOut` 禁用态 `#E4E4E7`/`#F4F4F5` + `opacity 0.6` 且点击不可选中（#1659 语义不变）；可点热区由 20×20 改为 36×36（视觉框仍 20×20，`marginTop: 16` 使视觉框顶边仍为 24，垂直位置零偏移）。
2. **weapp 自定义导航头胶囊避让**（`Cart.jsx` 头部）：新增 `navBar` memo，右内边距 = `windowWidth - menu.left + 8`（胶囊左边缘绝对 X；Home 的 `windowWidth - menu.right + 8` 只适用于居中窄元素，D1 明确弃用），上内边距 = `statusBarHeight + 8`；胶囊 API 缺失/抛错 → 兜底 `96`（不得退化为 0，D2）；头部显式 `boxSizing: 'border-box'`（小程序 View 默认 content-box）；「删除所选(N)」`flexShrink: 0` + `whiteSpace: nowrap`，标题 `flex: 1 1 0%` + `minWidth: 0` 先让位（D5）。
3. **文档（Core Update）**：`docs/spec/ui/ui.md` §2.7 新增「勾选框视觉规范」「自定义导航头胶囊约束」两条用户可见契约，并补记 #2042「删除所选(N)」既有行为。

行为面逐字不变（D7）：`selected` 状态机、`rentedOut` 禁选、`handleBatchDelete`、`handleCheckout`、`grandTotal`、价格/押金展示、`/checkout` 与 `/instrument` 跳转、空车态；`pages-weapp/cart/index.config.ts`（仍 `custom`）与 `app.config.ts` 未改（D8）。

### 涉及文件

- `frontend-mobile/src/pages/Cart.jsx`
- `frontend-mobile/scripts/page-smoke/cart-checkbox-2616.test.jsx`（新增，R1 渲染档位）
- `frontend-mobile/scripts/page-smoke/cart-header-2616.test.jsx`（新增，R2 渲染档位）
- `scripts/verify-2616.sh`（新增，证据生成器 r1/r2/r3/gate）
- `docs/spec/ui/ui.md`
- `docs/evidence/2616/`（plan.md、R1/R2/R3 产物、evidence.json、pre-ready-2616.json、summary.md、changed-files.txt）

### Commit

- fix: `eba2d0daf` — `fix(issue-2616): weapp cart checkbox contrast + custom-nav capsule clearance`
- evidence: `docs(evidence): issue-2616 …`（fix commit **之后**从该 commit 重新构建并生成产物）

### 实现前差异核对（计划「Input Truth & Tool Preflight」逐条）

| 检查 | 期望 | 实测 |
|---|---|---|
| `git show HEAD:frontend-mobile/src/pages/Cart.jsx | wc -l` | 501 行基线 | 501 ✅ |
| `grep -n "getStatusBarHeight() + 8"`（基线） | 1 命中 | 1 命中（基线 :324；计划记 :323，同一行区域） ✅ |
| `grep -n "borderColor: rentedOut ? '#e4e4e7'"`（基线） | 1 命中 | 1 命中（基线 :390） ✅ |
| `grep -rn "navigationStyle" frontend-mobile/src/pages-weapp` | 只有 home、cart 两处 `custom` | `pages-weapp/cart/index.config.ts:2`、`pages-weapp/home/index.config.ts:2` ✅ |
| `git grep -c "cart-select\|删除所选" HEAD -- 'frontend-mobile/scripts/page-smoke/*.test.jsx'` | 修前 0 | 0 命中 ✅ |

### 验证层级（Issue #70）

- 本单为前端子任务（无 Parent），按计划 D10 只跑**定向**档位：两个新渲染用例 + 定向 `eslint src/pages/Cart.jsx` 与两个测试文件；`gate` 模式不重复全量构建。
- 全量构建在本单 R3 内一次性产出：`build:weapp` + `build:h5`（node v22.22.3，经 `heavy-run.sh` 取 host 锁）。
- 未跑 `go build/test`（本单无后端改动）。

### Remote

remote: origin/issue-2616; fix=eba2d0daf pushed=yes; evidence=`docs(evidence): issue-2616` 提交 pushed=yes

### 红绿实证

红（未修基线 `Cart.jsx`，同一命令）：`vitest run --config vitest.config.js scripts/page-smoke/cart-checkbox-2616.test.jsx scripts/page-smoke/cart-header-2616.test.jsx` → **exit 1，2 个文件 10/10 用例全失败**：

```
 ❯ scripts/page-smoke/cart-checkbox-2616.test.jsx (5 tests | 5 failed)
 ❯ scripts/page-smoke/cart-header-2616.test.jsx (5 tests | 5 failed)
AssertionError: expected +0 to be 2 // Object.is equality      ← [data-testid="cart-select-box"] 不存在
AssertionError: expected null to be truthy                      ← [data-testid="cart-header"] 不存在
 Test Files  2 failed (2)
      Tests  10 failed (10)
```

绿（fix commit `eba2d0daf`，同一命令）：`Tests 5 passed` + `Tests 5 passed`（两个文件各 5/5），R1/R2 两个 artifact 的 16 个 token 全部齐备（见下）。

### Accepted Plan 对照（13/13 完成）

| 计划项 | 状态 |
|---|---|
| 变更点 1 头部胶囊避让（navBar + 三处 padding + boxSizing） | ✅ |
| 变更点 2 勾选框对比度 + 白底 + 36px 热区 + 禁用态 | ✅ |
| 变更点 3 新增 2 个渲染测试档位（含胶囊失效/12 件长计数/禁用不可选负例） | ✅ |
| 变更点 4 `docs/spec/ui/ui.md` §2.7 Core Update | ✅ |
| 被翻转断言 2 行（勾选框、头部） | ✅ 已按新语义断言 |
| 保持断言 3 行（`/cart` marker 冒烟、#2042 显示条件、#1659 禁选语义） | ✅ 未改语义，新增断言锁死 |
| Decision Lock D1–D11 | ✅ 逐条遵守（D1 用 `menu.left`；D2 兜底 96；D3 三态可区分；D4 36 热区无负 margin；D5 标题先让位；D6 不新增 platform 抽象；D7 行为不变；D8 不改 config；D9 颜色归一化；D10 重命令取锁；D11 无同 hunk 冲突） |
| Out of Scope 未越界 | ✅ 未加「全选」、未改其它页、未收敛 helper |

### Evidence Contract（4/4 行，全部 PASS）

| 行 | 命令 | 产物 | 关键 token | 状态 |
|---|---|---|---|---|
| R1 | `bash scripts/verify-2616.sh r1` | `docs/evidence/2616/R1-cart-checkbox.txt` | `CART_CHECKBOX_SELECTED_FILL=1` / `CART_CHECKBOX_UNSELECTED_CONTRAST=1` / `CART_CHECKBOX_UNSELECTED_WHITE_BG=1` / `CART_CHECKBOX_DISABLED_DISTINCT=1` / `CART_CHECKBOX_DISABLED_NOT_SELECTABLE=1` / `CART_CHECKBOX_TAP_SIZE=36` / `CART_CHECKBOX_RENDER_FAIL=0` / `CART_CHECKBOX_OLD_GRAY_PRESENT=0` | PASS |
| R2 | `bash scripts/verify-2616.sh r2` | `docs/evidence/2616/R2-cart-header.txt` | `CART_HEADER_WEAPP_PADDING_RIGHT=83` / `CART_HEADER_WEAPP_PADDING_TOP=28` / `CART_HEADER_H5_PADDING_RIGHT=16` / `CART_HEADER_H5_PADDING_TOP=12` / `CART_HEADER_FALLBACK_PADDING_RIGHT=96` / `CART_HEADER_DELETE_TEXT_FULL=1` / `CART_HEADER_DELETE_NO_SHRINK=1` / `CART_HEADER_TITLE_YIELDS=1` / `CART_HEADER_RENDER_FAIL=0` | PASS |
| R3 | `bash scripts/verify-2616.sh r3` | `docs/evidence/2616/R3-build-lint.txt` | `WEAPP_BUILD_OK=1` / `H5_BUILD_OK=1` / `WXML_CART_NATIVE_TAG=0` / `WEAPP_BUNDLE_MARKER=1` / `ESLINT_NOUNDEF=0` | PASS |
| R4 | `pre-ready-check.sh 2616 --summary-file docs/evidence/2616/summary.md --changed-files docs/evidence/2616/changed-files.txt --evidence-file docs/evidence/2616/evidence.json --affected-cmd "bash scripts/verify-2616.sh gate" --report docs/evidence/2616/pre-ready-2616.json` | `docs/evidence/2616/pre-ready-2616.json` | `overall=PASS`、evidence 4 行 `pass=true` | PASS |

依据 `docs/evidence/2616/evidence.json`（schema_version=1、issue=2616、build=1.0.eba2d0da、commit=eba2d0daf…、dirty=false、plan_sha256=6e33aa43…）。

### G0 Golden Acceptance

| 行 | artifact | assertions（机器可判定） | negative |
|---|---|---|---|
| R1 | `docs/evidence/2616/R1-cart-checkbox.txt` | 7 个 token 全为期望值 + `CART_CHECKBOX_OLD_GRAY_PRESENT=0`；产物首两行 provenance（build/commit/dirty、GENERATED_AT_HEAD） | 未选中仍 `#d1d5db`、无白底、热区 <32、禁用项可选中 → FAIL（已由红例覆盖：基线 5/5 失败） |
| R2 | `docs/evidence/2616/R2-cart-header.txt` | 9 个 token 全为期望值（83/28/16/12/96 + 3 个文案 token） | 胶囊失效兜底 0、H5 套胶囊内边距、长文案被压缩、标题不让位 → FAIL |
| R3 | `docs/evidence/2616/R3-build-lint.txt` | `WEAPP_BUILD_OK=1`、`H5_BUILD_OK=1`、`WXML_CART_NATIVE_TAG=0`、`WEAPP_BUNDLE_MARKER=1`、`ESLINT_NOUNDEF=0` | 构建非 0、cart wxml 含原生标签、产物无 `#71717A`、eslint no-undef → FAIL |
| R4 | `docs/evidence/2616/pre-ready-2616.json` | `overall=PASS` + evidence 每行 `pass=true` | 任一 P 项失败 / 缺行 / dirty=true / plan 漂移 → gate 非 0，禁止 ready |

### Verification Matrix（矩阵 4/4）

| 变更点 | 状态 | 证据 |
|---|---|---|
| R1 勾选框对比度 + 可点热区 + 禁用态（页内选择控件的 render 断言：render(/fireEvent） | PASS | `docs/evidence/2616/R1-cart-checkbox.txt` |
| R2 自定义导航头胶囊避让 + 长文案不裁切（页头 render 断言） | PASS | `docs/evidence/2616/R2-cart-header.txt` |
| R3 weapp/H5 构建 + weapp 产物可达性 + ESLint（no-undef） | PASS | `docs/evidence/2616/R3-build-lint.txt` |
| R4 pre-ready gate（四源一致性 + 合同完整性 + 远端可达） | PASS | `docs/evidence/2616/pre-ready-2616.json` |

### 证据真实性 / 持久化

- 全部产物位于 `docs/evidence/2616/`（随 `docs(evidence)` 提交），非 tmp 唯一证据；每份文本产物首行 provenance = `# build=1.0.eba2d0da # commit=eba2d0daf… # dirty=false`，第二行 `GENERATED_AT_HEAD=eba2d0daf…`（与 evidence.json 的 build/commit 一致）。
- 三条证据各自对应不同入口/断言面，无跨行复用：R1 = 勾选控件（H5 platform 渲染 + 点击驱动）；R2 = 页头（weapp platform 渲染 + 三种胶囊形态）；R3 = 构建链 + 产物探针 + lint。
- 真机像素级复看不在本矩阵（Out of Scope，部署侧 `waitForConfirm`）。

### 测试保真

- R1/R2 走 `@testing-library/react` 的真实 DOM 渲染（`render(`/`fireEvent`/`waitFor`），不是源码 grep；颜色断言经 DOM 归一化（`div.style.color = hex`）后比较，避免 cssstyle 归一化造成假绿。
- R2 的 `vi.mock('../../src/platform')` 取 `src/platform/index.weapp` 的 actual 并让 `env.isMiniProgram` 成为可切换 getter，因此同一文件里「weapp 真值 / H5 兜底 / 胶囊 API 抛错」三种形态都真实经过 `Cart.jsx` 的 `navBar` 分支（token 由用例内 `process.stdout` 输出，产物可见）。
- R3 的 wxml 探针在 `dist-weapp/pages-weapp/cart` 上跑（`base.wxml` 排除），产物标记 `#71717A` 在 cart 产物内命中 1。

### 依赖顺序 / 验收规则

- `## Dependencies`：无；未改任何验收工具/规则（`pre-ready-check.sh`、`matrix_parser.sh`、`verify-2616.sh` 为新脚本但不替换既有验收面）。
- 未出现连续两轮验收失败；无延期项。

### Core Update

- `[Core Update Required]` 已执行：`docs/spec/ui/ui.md` §2.7「购物车页」新增勾选框视觉规范（未选中/选中/禁用 + 热区 ≥32）、自定义导航头胶囊约束（`windowWidth - menu.left + 8`、上内边距、兜底 96、文案单行、标题先让位）、#2042 批量删除既有行为。
- 未改 `docs/spec/api/api.md`（购物车为纯前端本地存储）；未新增跨端差异，故不改 `docs/cases/h5-weapp-differences.md`（胶囊对齐已由 A14 覆盖）。

### 与 Owner 追加评论（2026-11-27 20:04）的关系

- 追加评论的核心诉求「按钮完全点不到」与本单根因一致，已由本单修复：头部右侧内边距 83px 使「删除所选(N)」右端落在 x=292，完全位于胶囊左边缘（x=300）左侧 → **可见且可点、不重叠、不裁切**（R2 用可判定等式 + 精确像素断言证明；jsdom 不做真实布局，故不以矩形相交证明）。
- 追加评论的**首选**方案（把批量删除移到列表底部操作条）与 Accepted Plan 的 D1/D5（锁定头部胶囊避让，`Out of Scope` 明确「删除入口保持在头部，只修重叠与裁切」）不同。Developer 无决策权，本单按 Accepted Plan 执行；若 Owner 仍希望改为底部操作条，请按流程重新走 `/analyze`（或另立 Issue），本单不擅自扩面。

### 人工 / 部署侧（不由 Developer 完成）

| 事项 | 负责方 | 回执要求 |
|---|---|---|
| 真机验收（选中任意商品 → 「删除所选(N)」可见可点、与胶囊不重叠、文字不裁切；点击后弹窗确认列表项消失；未选中勾选框与背景可区分） | Owner | 微信后台设为体验版后真机确认，回评 #2616 |

*Model: deepseek/deepseek-flash*
