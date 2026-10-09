# H5 ↔ weapp 一致性：机械快照与白名单（#2172 Phase 1）

> 本文档两部分：**① 合理适配白名单（判定口径）** + **② 机械快照（自动生成）**。
> 快照由 `scripts/h5-weapp-parity.sh` 生成（只读、可复跑），请勿手改快照部分。

## 0. 判定口径（先定义，避免误报）

差异分两类，只有②才是「分歧」：
- **① 合理适配**：由平台机制差异**必然导致**、且用户可感知结果等价。→ 登记白名单，**不算分歧**。
- **② 行为/视觉分歧**：同一业务意图两端结果不等价（导航目标、弹窗、状态、刷新、入口、文案、样式静默失效）。→ 进矩阵，按 P0/P1/P2 收敛。

### 合理适配白名单（不算分歧）

| # | 差异 | 为何是适配 |
|---|------|-----------|
| W1 | weapp 系统导航栏（标题/返回）vs H5 页内标题 + 页内 `←` | H5 无原生导航栏，须页内补齐；**前提：标题文案、返回目标一致** |
| W2 | weapp 原生返回手势 vs H5 页内返回 | 同上 |
| W3 | 底条 `switchTab`（weapp）vs react-router（H5） | 机制不同、无动画二者皆是；**前提：tab 集合与 active 判定一致** |
| W4 | 沉浸式首页实现（weapp 自定义导航 + 胶囊对齐 vs H5） | 视觉机制不同；**前提：数据/导航/交互行为一致** |
| W5 | 微信专属能力（`wxLogin`/`getPhoneNumber`/`scanQRCode`/camera）H5 降级或隐藏 | 平台能力差异 |
| W6 | weapp 独有页面（login/bind/renewal/payment 等无 H5 对手） | **有意**；需登记确认（见快照 §1） |
| W7 | `env.isMiniProgram` 用于**平台能力**分支（storage/request/upload/icon） | 抽象层实现差异 |

### 白名单**不覆盖**（即真分歧，必须核对）
- 同一业务交互两端**导航目标不同**（如 `goService`：员工端 H5→A / weapp→B）。
- 一端弹窗/Toast、另一端静默（或文案不同）。
- 生命周期刷新不对称（weapp `useDidShow` 自动刷新 vs H5 不刷新，或反之）。
- 入口位置/文案/可达性不同（#1589 类）。
- 样式禁区导致 weapp 静默失效（#1831）。
- 交互控件在 weapp 不渲染（原生 HTML 标签，#2122）。

## 1. 方法论链接
- 结构/视觉：本文档快照 §1–§5、§7–§8。
- **交互行为（event-level，6 轴）**：快照 §6 给出热点排期；实际走查见 #2172 Phase 2.5（可触达 / 反馈 / 跳转 / 状态 / 副作用 / 生命周期）。

## 2. 运行方式
```bash
bash scripts/h5-weapp-parity.sh > docs/topics/h5-weapp-parity.md   # 重新生成快照（下半部分）
```
> 依赖：`python3`、`frontend-mobile/src`；§8 需先 `cd frontend-mobile && npm run build:weapp`。

---

# 机械快照（自动生成）
# H5 ↔ weapp 一致性机械快照（自动生成）

- 生成时间: 2026-10-09 01:45:06
- 源: `frontend-mobile/src`
- 说明: 本文件由 `scripts/h5-weapp-parity.sh` 生成（只读侦察），**请勿手改**。

## 1. 登记对称性（h5Pages ↔ weappPages ↔ App-H5 路由 ↔ ROUTE_MAP）

- weappPages: 52 条目 | h5Pages: 45 条目
- **weapp 有 / H5 无**（13）: account-select, bind, content, invoice, login, payment, profile-complete, profile/edit, renewal, repair-payment-complete, return-settlement, search, setting
- **H5 有 / weapp 无**（6）: lease-history, my-contracts, site-detail, staff-order-detail, transit-workflow, user-warnings
- App-H5 路由: 62 条 | ROUTE_MAP: 39 条
- **H5 路由无 weapp 映射（首段）**（14，候选：H5-only / 页面内显式跳转 / 遗漏，需人工判定）: /about, /account-select, /callback, /face-verify, /lease-history, /my-contracts, /receive, /register, /renewal, /return, /search, /setting, /site, /success

## 2. 双实现清单（pages/*.jsx ∩ pages-weapp/*.jsx，漂移风险最高）

| 页面 | H5 行数 | weapp 行数 | 行差 |
|------|--------:|----------:|-----:|
| Checkout.jsx | 1411 | 1327 | 84 |
| Detail.jsx | 590 | 569 | 21 |
| Home.jsx | 496 | 625 | 129 |
| MyLeases.jsx | 340 | 319 | 21 |
| Profile.jsx | 584 | 704 | 120 |
| Success.jsx | 32 | 86 | 54 |

## 3. 两端行为分叉点（env.isMiniProgram / isWeapp）

- 含分叉的文件: **49** | 分叉点总数: **218**（每处都需判定：合理适配 / 潜在分歧）

## 4. 组件分裂（components/ vs components-weapp/）

- 双实现: BottomNav.jsx 
- 仅 shared: AddressManager.jsx FaceCaptureUploader.jsx IdPhotoUploader.jsx ImageUploader.jsx InstrumentInfo.jsx IntroLetterUploader.jsx LeaseInfo.jsx OptionSheet.jsx OrderTimeline.jsx RepairRecordPanel.jsx RichContent.jsx SegmentedTabs.jsx StaffIdPhotoViewer.jsx TechRepairSections.jsx VerifyWarningBar.jsx 
- 仅 weapp: ErrorBoundary.jsx 

## 5. 平台抽象层对称（platform/browser.js ↔ platform/index.weapp.js）

- browser: 21 | weapp: 21
- **仅 browser**: 无
- **仅 weapp**: 无

## 6. 交互热点（每页交互密度，供 Phase 2.5 行为走查排期）

| 页面 | onClick | dialog | nav | useEffect | useDidShow | setState |
|------|--------:|-------:|----:|----------:|-----------:|---------:|
| Profile.jsx | 31 | 0 | 17 | 6 | 0 | 40 |
| RepairRequestDetail.jsx | 29 | 17 | 3 | 1 | 0 | 66 |
| RepairServiceDetail.jsx | 27 | 50 | 1 | 2 | 0 | 62 |
| OrderDetail.jsx | 23 | 14 | 16 | 5 | 0 | 28 |
| Checkout.jsx | 21 | 34 | 8 | 5 | 0 | 99 |
| MyRepairs.jsx | 16 | 2 | 2 | 2 | 0 | 25 |
| MessageDetail.jsx | 16 | 1 | 21 | 1 | 0 | 11 |
| Detail.jsx | 12 | 0 | 8 | 1 | 0 | 22 |
| RepairWorkflow.jsx | 11 | 8 | 1 | 1 | 0 | 22 |
| StaffInstrumentForm.jsx | 10 | 4 | 2 | 1 | 0 | 31 |
| StaffInstrumentDetail.jsx | 10 | 15 | 10 | 2 | 0 | 10 |
| RepairServiceCreate.jsx | 10 | 8 | 6 | 3 | 0 | 19 |
| MyLeases.jsx | 10 | 3 | 8 | 2 | 0 | 15 |
| FaceVerify.jsx | 10 | 0 | 3 | 2 | 0 | 47 |
| ReceivingRepairScan.jsx | 9 | 9 | 2 | 1 | 0 | 17 |
| InstrumentLossManage.jsx | 9 | 11 | 1 | 2 | 0 | 36 |
| Cart.jsx | 9 | 2 | 3 | 3 | 1 | 24 |
| MembershipCenter.jsx | 8 | 1 | 2 | 3 | 0 | 15 |
| StaffInstruments.jsx | 7 | 0 | 4 | 2 | 0 | 17 |
| ShippingInterface.jsx | 7 | 7 | 3 | 1 | 0 | 21 |
| ReceivingInterface.jsx | 7 | 8 | 2 | 2 | 0 | 34 |
| Invoice.jsx | 7 | 0 | 0 | 1 | 0 | 19 |
| Payment.jsx | 6 | 10 | 5 | 1 | 0 | 10 |
| Messages.jsx | 6 | 2 | 2 | 1 | 1 | 6 |
| Home.jsx | 6 | 0 | 6 | 3 | 0 | 33 |
| StaffReceiveConfirm.jsx | 5 | 6 | 4 | 3 | 0 | 15 |
| ReturnConfirm.jsx | 5 | 4 | 5 | 1 | 0 | 10 |
| RepairScan.jsx | 5 | 6 | 2 | 1 | 0 | 10 |
| Register.jsx | 5 | 8 | 3 | 1 | 0 | 21 |
| ReceiveConfirm.jsx | 5 | 5 | 4 | 1 | 0 | 8 |
| StaffOrders.jsx | 4 | 1 | 3 | 2 | 0 | 18 |
| Search.jsx | 4 | 0 | 2 | 1 | 0 | 8 |
| Renewal.jsx | 4 | 2 | 3 | 2 | 0 | 8 |
| TransitWorkflow.jsx | 3 | 5 | 2 | 1 | 0 | 12 |

## 7. weapp 样式禁区命中（#1831：硬禁区=必删类；软禁区=禁新增）

- 硬禁区（space-y/x、分数类）:
  - 0 命中
- 软禁区（变体类 / 任意值）:
        4 active:
        2 focus:

## 8. weapp 产物原生标签探针（需先 npm run build:weapp）

- ✅ 无原生 HTML 交互/媒体标签（通过）

---
_本快照仅为机械化事实，"合理适配 vs 分歧" 判定见白名单与 #2172 矩阵。_
