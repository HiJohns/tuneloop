## Accepted Plan — #2615 [Design] 丢失登记页信息架构：记录列表 + 单一登记入口 + 详情页恢复

基线：`origin/develop` @ `bdd14b69d`（`frontend-mobile/src/pages/InstrumentLossManage.jsx` 334 行、`backend/handlers/instrument_loss.go` 434 行，行号以该基线为准）。

### 背景与现状核对（代码为据）

| # | 事实 | 位置 |
|---|---|---|
| 1 | 功能域文档把「丢失登记」定义为「员工小程序 → 乐器管理 → 找到乐器 → 「丢失」」，把「恢复」定义在乐器管理行上；本单要把入口收敛为「丢失记录列表 + 单一登记入口」，**这两处入口描述与现状一致但与目标形态不同**，必须同步改文档（见 Core Update） | `docs/cases/instrument-loss.md` LS-01 / LS-05、`docs/spec/ui/ui.md` §3.26 |
| 2 | 现页三段式：`丢失中（N）` 卡片（含「恢复」按钮）→ `在管乐器（N）` **每行**一个 `btnDangerStyle`（`#DC2626`）「丢失登记」→ `丢失台账（N）`（只读）。`在管乐器` 是服务端真分页触底加载，1000 台即 1000 个红色按钮 | `InstrumentLossManage.jsx:296-311`（`btnDangerStyle`）、`:283-296`（在管乐器行）、`:231-249`（丢失中）、`:303-320`（台账） |
| 3 | 台账段已具备「行级信息」雏形：`责任方 · 比例`、`赔偿/承担`、`created_at`、`restored_at` 状态徽标；但**没有乐器标识**（记录只有 `instrument_id`），也没有「什么时候丢的」可点击进入的详情 | `InstrumentLossManage.jsx:308-320` |
| 4 | `GET /api/instrument-loss` 只返回 `InstrumentLossRecord` 原始行（`instrument_id`、`photos` 为 JSON **字符串**、`restore_photos` 同），`Limit(200)` 无分页、无状态过滤（仅 `open=true`）；响应 `total` 实为 `len(list)` | `backend/handlers/instrument_loss.go:404-433` |
| 5 | 无 `GET /api/instrument-loss/:id` 单条详情端点（详情页无法按记录 id 深链） | `backend/main.go:442` 仅注册 `GET /instrument-loss` |
| 6 | 登记端点 `POST /instruments/:id/lost` 已支持 `photos[]`（`Register` 的 body 含 `Photos []string`），但现有 UI **登记表单没有照片控件**（只有恢复表单有）→ 详情页「当时填写的照片」在登记侧永远是空 | `instrument_loss.go:146-160`、`InstrumentLossManage.jsx:203-241`（登记表单无照片）、`:257-286`（恢复表单有照片） |
| 7 | 恢复端点 `POST /instruments/:id/restore` 以**乐器 id** 定位最新未恢复记录（`instrument_id = ? AND restored_at IS NULL`），因此详情页只要拿到记录的 `instrument_id` 即可提交恢复 | `instrument_loss.go:344-405` |
| 8 | 恢复语义已简化：仅「恢复上架 + 留痕」（无冲正/重算），失败/成功均为 `20000` + `action: put_back_on_shelf` | `docs/cases/instrument-loss.md` LS-05 / LS-05a、`instrument_loss.go:379-404` |
| 9 | 乐器检索端点已支持按 SN 模糊搜索 + 状态排除 + 分页，可直接作为「登记表单的乐器选择器」数据源 | `backend/handlers/api.go:372-420`（`sn` ILIKE / `exclude_status` / `page` / `pageSize`） |
| 10 | 跨端导航必须登记映射（weapp 用 `/pages-weapp/...` 全路径）；新页必须同时进 `app.config.ts` 的 `weappPages`/`h5Pages` + H5 薄壳，否则 `build:h5` 直接失败（#1989） | `frontend-mobile/src/platform/navigation.js:10-50`、`frontend-mobile/src/app.config.ts:8-72`、`frontend-mobile/src/App-H5.jsx:345` |
| 11 | weapp 页面统一用原生导航栏 + `index.config.ts` 标题（页内不画标题条）；H5 页保留页内头部（`!env.isMiniProgram` 守卫） | `pages-weapp/instrument-loss-manage/index.config.ts`、`InstrumentLossManage.jsx:203-209` |
| 12 | 渲染测试档位可复用：`vitest.config.js` 只收 `scripts/page-smoke/**/*.test.jsx`；`taro-stub` 提供 `getStorageSync`（localStorage 底座）/`showModal`/`previewImage`/`chooseImage`；`mock-platform.js` 的 `installFetchMock` 是所有页面测试的 fetch 桩 | `frontend-mobile/vitest.config.js`、`scripts/page-smoke/taro-stub.js`、`scripts/page-smoke/mock-platform.js` |
| 13 | 后端测试档位可复用：`testfixtures.SetupTestDB`（Postgres）+ `SeedTenantFK`/`SeedSiteFK` + `lossPost` 等既有 helper，夹具必须区分 `sites.id` 与 `sites.org_id`（#2510 教训） | `backend/handlers/instrument_loss_test.go:24-70`、`:380-430` |
| 14 | `lossApi.list` 在 PC `api.js` 有定义但**无任何页面调用**（`grep -rn "lossApi" frontend-pc/src --include=*.jsx` = 0），因此本次归一化 `photos`/新增 `instrument_brief` 不会破坏既有消费方 | `frontend-pc/src/services/api.js:530-535` |

### 变更点

#### 1. `backend/handlers/instrument_loss.go` — `List` 分页 + 状态过滤 + 乐器摘要

```go
// List GET /api/instrument-loss?page=&pageSize=&status=open|restored|all&instrument_id=
func (h *InstrumentLossHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	db := database.GetDB().WithContext(ctx)
	tid := middleware.GetTenantID(ctx)
	if tid == "" {
		c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
		return
	}
	page := atoiDefault(c.DefaultQuery("page", "1"), 1)
	pageSize := clampPageSize(atoiDefault(c.DefaultQuery("pageSize", "20"), 20), 20, 100)
	if page < 1 { page = 1 }
	status := c.DefaultQuery("status", "all")
	if c.Query("open") == "true" { status = "open" } // 兼容旧参数

	query := db.Model(&models.InstrumentLossRecord{}).Where("tenant_id = ?", tid)
	if iid := c.Query("instrument_id"); iid != "" { query = query.Where("instrument_id = ?", iid) }
	switch status {
	case "open":     query = query.Where("restored_at IS NULL")
	case "restored": query = query.Where("restored_at IS NOT NULL")
	case "all":
	default: c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "status must be open/restored/all"}); return
	}

	var total int64
	if err := query.Count(&total).Error; err != nil { /* 500/50000 failed to count loss records */ }
	var list []models.InstrumentLossRecord
	if err := query.Order("created_at DESC").Offset((page-1)*pageSize).Limit(pageSize).Find(&list).Error; err != nil { /* 500/50000 */ }

	// 乐器摘要：一次 IN 查询（禁 N+1），tenant 过滤，附 cover_image（Instrument.CoverImage）
	briefs := lossInstrumentBriefs(db, tid, list)
	out := make([]gin.H, 0, len(list))
	for _, r := range list {
		out = append(out, lossRecordJSON(r, briefs[r.InstrumentID]))
	}
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{
		"list": out, "total": total, "page": page, "page_size": pageSize,
	}})
}
```

- 新增 helper `lossInstrumentBriefs(db, tenantID string, recs []models.InstrumentLossRecord) map[string]gin.H`：收集去重 `instrument_id` → `WHERE tenant_id = ? AND id IN ?` 一次查询 → `{id, sn, category_name, level_name, cover_image, stock_status}`。
- 新增 helper `lossRecordJSON(r models.InstrumentLossRecord, brief gin.H) gin.H`：原始字段全保留，**`photos` / `restore_photos` 归一化为 `[]string`**（模型列是 `jsonb` 字符串；空值输出 `[]` 而不是 `null`），并挂 `instrument_brief`（未命中时为 `null`）。
- `clampPageSize` 已存在于 handlers 包（`api.go` 使用），直接复用；若签名不符则以 `if pageSize > 100 { pageSize = 100 }` 就地实现并注明。

#### 2. `backend/handlers/instrument_loss.go` — 新增 `Get` 单条详情

```go
// Get GET /api/instrument-loss/:id（员工/管理员详情页）
func (h *InstrumentLossHandler) Get(c *gin.Context) {
	// tenant 过滤 + WithContext；未命中一律 404/40400（不区分「不存在」与「他人租户」，避免探测）
	// 命中 → 复用 lossInstrumentBriefs + lossRecordJSON 输出 {code:20000,data:{...}}
}
```

- 跨租户 / 不存在 → `404` + `40400`（`loss record not found`）；`tid == ""` → `403/40300`（与 `List` 一致）。
- 路由：`backend/main.go` 在 `GET /instrument-loss` 之后加 `authRequired.GET("/instrument-loss/:id", middleware.ExplicitTenantScope(), middleware.RequireCusPerm("instrument:read"), lossHandler.Get)`。

#### 3. `frontend-mobile/src/pages/InstrumentLossManage.jsx` — 重写为「丢失记录列表」

- **删除**三段式里的「在管乐器」段与行内红色按钮（`btnDangerStyle` 从列表中移除；该样式常量保留给「确认恢复」等确认动作所在页面使用，或下沉到各自页面）。
- 列表数据：`GET /api/instrument-loss?page=<n>&pageSize=20&status=all`；触底 `onScrollToLower` 加载下一页（`hasMore = list.length < total`）；`useDidShow` 重新拉取第 1 页（从登记/详情返回后同步）。
- 行渲染（`data-testid="loss-row"`）：封面缩略图（`instrument_brief.cover_image`，缺失用占位）、`SN: {sn}` + `{category_name}`、`丢失于 {created_at}`（`formatBeijingDate`）、状态徽标（`restored_at ? '已恢复'（#16A34A） : '丢失中'（#DC2626）`）、`责任方 · 比例 N%` + `赔偿/承担`。整行 `onClick` → `nav('/instrument-loss-detail?id=<record.id>')`。
- **单一入口**：页首（标题行右侧）`data-testid="loss-register-entry"` 的「＋ 登记丢失」按钮（主色 `#171717`）→ `nav('/instrument-loss-register')`；空态同样只给这一个入口。
- 行内**不得**出现任何确认/危险动作按钮（`LOSS_LIST_DANGER_BUTTON_IN_ROW=0`）。

#### 4. `frontend-mobile/src/pages/InstrumentLossRegister.jsx`（新增）— 登记表单页

- 顶部：所选乐器卡片（未选 → 提示「请选择乐器」）；乐器选择器 = SN 搜索框（`data-testid="loss-inst-sn"`，`onInput` 防抖 300ms）+ 结果列表（`GET /instruments?sn=<kw>&exclude_status=lost,archived,sold&page=1&pageSize=20`，单选取 `data-testid="loss-inst-option"`）。
- 表单字段（沿用现有语义与默认值）：责任方 4 选（`user/logistics/platform/site`，默认 `user`；切换时 `user→ratio=100`，其余 `→ratio=0`）、用户责任比例（0-100，`data-testid="loss-ratio"`）、赔偿金额（元）、用户承担金额（元，留空=按比例）、丢失描述（必填，`data-testid="loss-desc"`）、照片 ≤6（`Taro.chooseImage` + 预览 + `platform.uploadFile` 逐个上传，复用现有 `uploadOne` 语义）。
- 校验（就地 `dialog.alert`，与现网一致）：未选乐器 →「请选择丢失的乐器」；描述空 →「请填写丢失描述」；比例非 0-100 →「责任比例须为 0-100」。
- 提交：`POST /instruments/<id>/lost`，body `{description, responsible_party, user_ratio, compensation_cents, photos}`；成功 → `dialog.alert('丢失登记完成')` → `nav(-1)`（回列表，列表 `useDidShow` 刷新）。按钮文案「提交丢失登记」/「处理中...」，`disabled` 时灰显并显示前置条件文案。

#### 5. `frontend-mobile/src/pages/InstrumentLossDetail.jsx`（新增）— 详情页承载恢复

- 参数：`useSearchParams().get('id')`（**禁止 `useParams`**，weapp 无路径参数，#1674）。
- 数据：`GET /api/instrument-loss/<id>` → 展示 `instrument_brief`（SN/类别/封面）+ 记录内容：丢失时间、责任方 · 比例、赔偿/承担金额、描述、照片（`Taro.previewImage` 多图预览，用 Taro `<Image>`）。
- 已恢复记录（`restored_at` 非空）：显示「已恢复」徽标 + 恢复时间 / 是否有损坏 / 找回说明 / 恢复照片，**不渲染「恢复」按钮**（`LOSS_DETAIL_RESTORED_HIDES_RESTORE=1`）。
- 丢失中记录：底部行动条「恢复上架」（`data-testid="loss-open-restore"`）→ 展开恢复表单（是否有损坏 是/否、找回说明、照片 ≤6）→ 提交 `POST /instruments/<record.instrument_id>/restore` `{damaged, description, photos}` → 成功 `dialog.alert('已恢复上架（有损坏，已留痕）' | '已恢复上架（无损坏）')` → `nav(-1)`。
- 失败：`dialog.alert(resolveErrorMessage(result))`（不得静默）。

#### 6. 跨端接线（每页 1 个 H5 薄壳 + 1 个 weapp 薄壳 + weapp 页配置）

| 路由 | H5 组件与路由 | H5 薄壳 | weapp 页 | weapp 配置 |
|---|---|---|---|---|
| 列表 | `InstrumentLossManage`（已存在）`/instrument-loss-manage` | 已存在 | `pages-weapp/instrument-loss-manage` | 已存在 |
| 登记 | `/instrument-loss-register` | `src/pages/instrument-loss-register/index.tsx` | `pages-weapp/instrument-loss-register/index`（`useIdentityGuard` + `PageShell`） | `navigationBarTitleText: '登记丢失'` |
| 详情 | `/instrument-loss-detail` | `src/pages/instrument-loss-detail/index.tsx` | `pages-weapp/instrument-loss-detail/index`（同上） | `navigationBarTitleText: '丢失记录详情'` |

- `app.config.ts`：`weappPages` +2、`h5Pages` +2；`App-H5.jsx` +2 个 `ProtectedRoute` 路由；`platform/navigation.js` `ROUTE_MAP` +2 条（`/instrument-loss-register`、`/instrument-loss-detail`，`navigateTo`，query 由 `toWeappRoute` 保留）。
- 入口不变：`StaffInstruments.jsx:117` 仍跳列表页（`/instrument-loss-manage`）。

#### 7. 测试与证据

- **后端**（新增文件 `backend/handlers/instrument_loss_list_2615_test.go`）：复用 `setupLossFixture` + `mkLossInstrument` + `lossPost`；造 3 条记录（同一租户，2 条未恢复 + 1 条已恢复，分属不同乐器，SN 各异），断言 `total/page/page_size/status 过滤/instrument_brief.sn/photos 数组化`；`GET /:id` 命中与不存在；**跨租户**用另一 tenant 的 token 请求同记录 id → 404（负例：不得 200 泄漏）。
- **前端**（新增 `frontend-mobile/scripts/page-smoke/instrument-loss-2615.test.jsx`）：三个 `describe` 分别渲染列表 / 登记 / 详情，fetch 用 `vi.stubGlobal` 按 URL 分派（`/instrument-loss?...`、`/instrument-loss/<id>`、`/instruments?...`、`/instruments/<id>/lost`、`/instruments/<id>/restore`），导航用 `MemoryRouter` + `useNavigate` 断言（或 `vi.mock('react-router-dom')` 仅替换 `useNavigate` 以捕获目标 URL）；断言后 `console.log` 输出 token。

### 被翻转 / 被强化的既有断言（强制清单）

| 文件:行 | 旧期望 | 新期望 | 理由 |
|---|---|---|---|
| `frontend-mobile/src/pages/InstrumentLossManage.jsx:283-296`（无既有测试） | 「在管乐器」每行有 `btnDangerStyle`「丢失登记」 | 列表不再渲染在管乐器，行内无危险按钮（`LOSS_LIST_DANGER_BUTTON_IN_ROW=0`） | 本单核心 IA 变更 |
| `frontend-mobile/src/pages/InstrumentLossManage.jsx:231-249`（无既有测试） | 「丢失中」卡片带「恢复」直接展开恢复表单 | 列表行只展示 + 可点进详情；恢复动作移到详情页 | 详情承载恢复 |
| `backend/handlers/instrument_loss.go:404-433`（既有测试只覆盖 Register/Restore/结算） | `Limit(200)` 无分页、`total=len(list)`、`photos` 为字符串 | 分页 + `total` 为真实计数 + `photos`/`restore_photos` 数组 + `instrument_brief` | 列表分页与行级乐器标识的前置 |
| **保持**：`backend/handlers/instrument_loss_test.go` 全量（`TestInstrumentLoss_*`、`TestInstrumentLoss2510_*`） | 登记/恢复/结算/冲正/ID 域行为 | 逐条不变（本单只加 List/Get 与响应归一化） | 防止 IA 改造波及业务语义 |
| **保持**：`GET /instrument-loss?instrument_id=&open=true` 旧参数 | 过滤语义 | `open=true` 继续等价 `status=open`（兼容层） | 避免既有调用方（PC `lossApi.list`）语义漂移 |
| **保持**：`POST /instruments/:id/lost` 与 `/restore` 的请求体与错误码（40002/40300/40400/40900） | 现值 | 不变 | 前端三页只消费既有端点 |

### Core Update

`[Core Update Required]`

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | 完全没有 `instrument-loss` 系列端点章节（`grep -n "instrument-loss"` = 0） | 新增小节「乐器丢失台账」：`GET /api/instrument-loss`（参数 `page/pageSize/status=open\|restored\|all/instrument_id`、响应 `{list,total,page,page_size}`、行内 `photos`/`restore_photos` 为字符串数组、`instrument_brief` 字段表）、`GET /api/instrument-loss/:id`（20000 / 40400）、并回链既有 `POST /api/instruments/:id/lost`、`POST /api/instruments/:id/restore` 的请求体与错误码 | 本单新增两个端点语义（分页/状态过滤/详情）+ 归一化 `photos`，属对外契约；前端三页直接依赖 |
| `docs/spec/ui/ui.md` | §3.26「乐器丢失与恢复入口」写的是「乐器管理 → 乐器行「丢失」→ Modal」「lost 筛选 → 「恢复」→ Modal」 | §3.26 改为三段式 IA：①**丢失记录列表**（`/instrument-loss-manage`：每行 乐器 SN/类别 + 丢失时间 + 状态徽标 丢失中/已恢复 + 责任方·比例 + 金额，整行进详情；**行内无危险按钮**；页首唯一「＋ 登记丢失」入口；分页 20/页触底加载）；②**登记页**（`/instrument-loss-register`：SN 检索选择乐器 + 责任方/比例/赔偿/承担/描述/照片≤6 → `POST /instruments/:id/lost`）；③**详情页**（`/instrument-loss-detail?id=`：记录全文 + 照片预览；丢失中显示「恢复上架」→ 恢复表单（损坏/说明/照片）→ `POST /instruments/:id/restore`；已恢复显示恢复留痕且无恢复按钮）。保留「危险色仅用于确认动作」的约束 | 用户可见 IA 变更必须落到 UI 规格，否则下一位实现者按旧 Modal 形态回归 |
| `docs/cases/instrument-loss.md` | LS-01「入口：员工小程序 → 乐器管理 → 找到乐器 → 「丢失」」；LS-05「入口：乐器管理 → 筛选 lost → 「恢复上架」」 | LS-01 入口改为「丢失记录列表 → 「＋ 登记丢失」→ 登记页（乐器用 SN 检索选择）」；LS-05 入口改为「丢失记录列表 → 记录详情 → 「恢复上架」」；其余（结算口径 LS-03、恢复语义 LS-05、作废项 LS-05a）**逐字不动** | 用例文档是 ui/api 的上游；入口描述必须与实现一致 |

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 台账列表分页/状态过滤/乐器摘要/详情端点（集成测试，Postgres fixture） | `backend/handlers/instrument_loss.go`、`backend/handlers/instrument_loss_list_2615_test.go`、`backend/main.go` | `GET /api/instrument-loss`、`GET /api/instrument-loss/:id` | 3 条记录（2 未恢复 + 1 已恢复，不同乐器）；另一租户 token 请求同 id | backend（Postgres 测试库） | `LOSS_LIST_TOTAL=3`、`LOSS_LIST_PAGE_LEN=2`、`LOSS_LIST_STATUS_OPEN=2`、`LOSS_LIST_STATUS_RESTORED=1`、`LOSS_LIST_BRIEF_SN=1`、`LOSS_LIST_PHOTOS_ARRAY=1`、`LOSS_GET_CODE=20000`、`LOSS_GET_BRIEF_SN=1`、`LOSS_GET_MISSING_CODE=40400`、`LOSS_GET_CROSS_TENANT_CODE=40400` | `bash scripts/verify-2615.sh r1 > docs/evidence/2615/R1-api.txt` |
| R2 列表页 render：记录行信息 + 单一登记入口 + 无行内危险按钮 + 行点击进详情 | `frontend-mobile/src/pages/InstrumentLossManage.jsx`、`frontend-mobile/scripts/page-smoke/instrument-loss-2615.test.jsx` | H5 `/instrument-loss-manage`（`render(` + `fireEvent`） | 3 条记录（2 丢失中 / 1 已恢复） | H5 + weapp 共用 `.jsx`（jsdom） | `LOSS_LIST_ROWS=3`、`LOSS_LIST_SN_SHOWN=1`、`LOSS_LIST_TIME_SHOWN=1`、`LOSS_LIST_STATUS_BADGE=2`、`LOSS_LIST_DANGER_BUTTON_IN_ROW=0`、`LOSS_LIST_REGISTER_ENTRY=1`、`LOSS_LIST_ROW_NAV=1`、`LOSS_LIST_RENDER_FAIL=0` | `bash scripts/verify-2615.sh r2 > docs/evidence/2615/R2-list.txt` |
| R3 登记页 render：SN 检索选择器 + 校验 + 提交载荷 + 返回列表 | `frontend-mobile/src/pages/InstrumentLossRegister.jsx`、`frontend-mobile/scripts/page-smoke/instrument-loss-2615.test.jsx` | H5 `/instrument-loss-register`（`render(` + `fireEvent`） | 未选乐器提交 / 描述为空 / 正常提交 | H5 + weapp 共用 `.jsx`（jsdom） | `LOSS_REG_SELECTOR_QUERY=1`、`LOSS_REG_PARTY_RATIO_DEFAULT=1`、`LOSS_REG_VALIDATE_NO_INSTRUMENT=1`、`LOSS_REG_VALIDATE_DESC_REQUIRED=1`、`LOSS_REG_SUBMIT_PAYLOAD=1`、`LOSS_REG_SUBMIT_BACK=1`、`LOSS_REG_RENDER_FAIL=0` | `bash scripts/verify-2615.sh r3 > docs/evidence/2615/R3-register.txt` |
| R4 详情页 render：记录全文 + 照片 + 恢复提交 + 已恢复态隐藏恢复按钮 | `frontend-mobile/src/pages/InstrumentLossDetail.jsx`、`frontend-mobile/scripts/page-smoke/instrument-loss-2615.test.jsx` | H5 `/instrument-loss-detail?id=<id>`（`render(` + `fireEvent`） | 丢失中记录 / 已恢复记录 / 提交恢复 | H5 + weapp 共用 `.jsx`（jsdom） | `LOSS_DETAIL_FIELDS=1`、`LOSS_DETAIL_PHOTOS=1`、`LOSS_DETAIL_OPEN_HAS_RESTORE=1`、`LOSS_DETAIL_RESTORED_HIDES_RESTORE=1`、`LOSS_DETAIL_RESTORE_PAYLOAD=1`、`LOSS_DETAIL_RENDER_FAIL=0` | `bash scripts/verify-2615.sh r4 > docs/evidence/2615/R4-detail.txt` |
| R5 跨端接线 + 构建 + 产物可达性 + ESLint | `frontend-mobile/src/app.config.ts`、`frontend-mobile/src/App-H5.jsx`、`frontend-mobile/src/platform/navigation.js`、`scripts/verify-2615.sh` | `build:weapp` / `build:h5` / 静态接线检查 / `eslint` | 修后工作树 | weapp + H5 | `WEAPP_PAGES_REGISTERED=2`、`H5_PAGES_REGISTERED=2`、`NAV_MAP_ENTRIES=2`、`WEAPP_BUILD_OK=1`、`H5_BUILD_OK=1`、`WXML_NATIVE_TAG=0`、`ESLINT_NOUNDEF=0` | `bash scripts/verify-2615.sh r5 > docs/evidence/2615/R5-build-wiring.txt` |
| R6 pre-ready gate（四源一致性 + 合同完整性 + 远端可达） | `docs/evidence/2615/pre-ready-2615.json` | `pre-ready-check.sh 2615` | 修后干净树 + 最新 Accepted Plan | backend + weapp + H5 | exit 0、`overall=PASS`、P12 每行 `pass=true`、P13 提交在 `origin/issue-2615` | R6（gate 命令见 Evidence Contract） |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | integration | handler + Postgres 测试库 + 双租户 token；断言 `total/page/status/brief/photos` 与 404 负例 |
| R2 | render | `@testing-library/react` 的 `render(` / `fireEvent` 真实渲染列表页 |
| R3 | render | 同上，登记页（选择器 + 校验 + 提交载荷） |
| R4 | render | 同上，详情页（丢失中/已恢复两态 + 恢复提交） |
| R5 | build | `taro build --type weapp` / `build:h5` + 路由注册静态断言 + wxml 原标签探针 + `eslint` |
| R6 | gate | `pre-ready-check.sh` 报告 |

### Input Truth & Tool Preflight

- **Source of truth**：
  - 丢失业务语义 = `docs/cases/instrument-loss.md`（LS-01/03/05/05a：登记字段、结算口径、恢复仅上架无冲正）；
  - 记录行字段唯一来源 = `models.InstrumentLossRecord`（`backend/models/models.go:1450-1478`）；
  - 乐器摘要字段唯一来源 = `models.Instrument`（`sn`/`category_name`/`level_name`/`cover_image`/`stock_status`）；
  - 端点事实 = `backend/main.go:440-442`（lost/restore/list）+ 本单新增 `:id`；
  - 跨端路由事实 = `app.config.ts` + `App-H5.jsx` + `platform/navigation.js`（三处必须同时登记）。
- **实施前差异核对（先跑，结果入 Work Summary）**：
  - `grep -n "Limit(200)" backend/handlers/instrument_loss.go` → 期望 1 命中（List 基线）；
  - `grep -n "instrument-loss" backend/main.go` → 修前只有 1 条（`GET /instrument-loss`），修后 2 条；
  - `grep -rn "lossApi" frontend-pc/src --include=*.jsx` → 0（响应归一化无既有消费方）；
  - `grep -n "btnDangerStyle" frontend-mobile/src/pages/InstrumentLossManage.jsx` → 修前 2 命中（列表行 + 登记确认），修后列表文件内 0 命中「行内危险按钮」；
  - `grep -c "instrument-loss-register\|instrument-loss-detail" frontend-mobile/src/app.config.ts` → 修前 0，修后各 2（weapp + h5）。
- **损坏输入拒绝规则**：
  - `status` 取 `open|restored|all` 之外 → `400/40002`（不得静默按 all 处理）；
  - `page < 1` → 归 1；`pageSize` clamp 1..100（`0`/负数/超大值不得改变结果集语义为全表）；
  - `instrument_brief` 未命中（乐器被删）→ 输出 `null`，前端显示 `SN: -` 而不是崩溃；
  - `photos` 列是非法 JSON 字符串 → 归一化为 `[]`，不得返回 500；
  - 详情 `id` 非 UUID/不存在/他人租户 → 一律 `404/40400`（不得 500、不得 403 泄漏存在性）。
- **工具盲区**：
  - jsdom 无真实布局与原生选择器：登记页的 SN 检索、拍照上传只断言「请求 URL 与提交载荷」，不断言相册/相机 UI；
  - `build:weapp` 只证明可编译且产物含三页 wxml/js，不证明真机渲染；真机走查按 `waitForConfirm` 另记（不进本矩阵）；
  - 后端集成测试用 `testfixtures.SetupTestDB`（Postgres），不覆盖生产数据分布（1000 台规模的分页性能由 `pageSize` 上限与 `LIMIT/OFFSET` 语义保证，不做压测）。
- **契约影响**：新增/变更 2 个 API 端点 → Work Summary 必须含 `### 契约三向核对`（pre-ready P7），且 `docs/spec/api/api.md` 与实现字段逐字一致。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 列表数据源 = `GET /api/instrument-loss`（记录为第一类实体），**不再**用 `/instruments?stock_status=lost` 当主列表；在管乐器只在登记页选择器里出现 | 本计划锁定 |
| D2 | 分页口径：`page` 从 1 起，`pageSize` 默认 20、上限 100，`total` 为真实计数；`status` 默认 `all`，`open=true` 兼容映射 `open` | 本计划锁定 |
| D3 | 响应归一化：`photos` / `restore_photos` 输出**字符串数组**（空 → `[]`）；`instrument_brief` 未命中 → `null`。PC `lossApi.list` 无调用方（grep 0），不构成破坏性变更 | 本计划锁定 |
| D4 | 详情端点越权/不存在统一 `404/40400`（不区分），`tid==""` 仍 `403/40300`（与 List 一致）；租户过滤用 `middleware.GetTenantID(ctx)`，**禁止** `c.GetString("tenant_id")` | 本计划锁定 |
| D5 | 三页拆分：列表 / 登记 / 详情各为独立路由（AGENTS「多步流程=独立页面」）。详情页内的**恢复表单**是同一页面内的表单展开（无独立「成功页」步骤，提交后 toast + 返回列表），认定为规则中的「非步骤 UI 状态」，不另开第四页 | 本计划锁定 |
| D6 | 危险色（`#DC2626`）只允许出现在「提交丢失登记」「确认恢复」这两个确认动作按钮上；列表行、操作入口一律不用危险色 | 本计划锁定 |
| D7 | 照片上传复用现有链路：`Taro.chooseImage` → `platform.uploadFile(/upload)` → `file_key`，≤6 张；不引入 H5 原生 `<input type="file">`（weapp 不渲染） | 本计划锁定 |
| D8 | 详情页参数用 `useSearchParams().get('id')`（weapp 无路径参数），query 由 `toWeappRoute` 原样透传 | 本计划锁定 |
| D9 | 列表返回后刷新用 `useDidShow`（platform 封装；H5 幂等），不依赖 `useEffect` 重挂载 | 本计划锁定 |
| D10 | 后端复用 `clampPageSize` 与既有 `loadLossInstrument` 语义；不新增表/列/迁移，不改 `models.InstrumentLossRecord` | 本计划锁定 |
| D11 | 重命令（`go test` / `build:weapp` / `build:h5` / `vitest` / `eslint`）经 `scripts/heavy-run.sh` 取锁；`gate` 模式只跑定向（本单 Go 测试 + 本单渲染用例 + 定向 eslint），全量构建在 R5 一次性产出 | 本计划锁定 |
| D12 | 若执行期发现 `InstrumentLossManage.jsx` / `instrument_loss.go` 同 hunk 被他人改动（如 #2520 后续、#2510 相关）→ HOLD 并回 Architect，禁止就地改写他人语义 | 本计划锁定 |

### Out of Scope

- 列表的过滤 Tab（全部/丢失中/已恢复）：服务端已支持 `status`，但 UI 首版不做切换控件（默认 `all`），需要时另立。
- 在管乐器的批量操作（批量登记丢失）、扫码选择乐器（SN 扫描）：登记页首版只做 SN 文本检索 + 点选。
- PC 后台（`frontend-pc`）的丢失台账页面：本单只改移动端与后端端点；`lossApi.list` 保持可用。
- 丢失结算口径（LS-03/03a/03b）、恢复语义（LS-05，仅上架无冲正）与作废项（LS-05a）：逐字不改。
- 追偿字段/状态（LS-04）：仍为线下处理。
- 真机像素级走查与体验版分发（`waitForConfirm` 部署侧事项）。
- 把 `frontend-mobile/scripts/page-smoke/routes.js` 的 criticalRoutes 扩到三个新页（本单已有专门渲染用例，避免重复跑全页冒烟）。

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | `setupLossFixture`（Postgres + 双租户 + `sites.id != sites.org_id`）；同一租户 3 条 `InstrumentLossRecord`（2 未恢复、1 已恢复，`instrument_id` 各异，SN `SN-2615-1/2/3`），`photos='["k1","k2"]'` | `GET /api/instrument-loss?page=1&pageSize=2&status=...`、`GET /api/instrument-loss/:id` | `docs/evidence/2615/R1-api.txt` | `LOSS_LIST_TOTAL=3`、`LOSS_LIST_PAGE_LEN=2`、`LOSS_LIST_STATUS_OPEN=2`、`LOSS_LIST_STATUS_RESTORED=1`、`LOSS_LIST_BRIEF_SN=1`、`LOSS_LIST_PHOTOS_ARRAY=1`、`LOSS_GET_CODE=20000`、`LOSS_GET_BRIEF_SN=1`、`LOSS_GET_MISSING_CODE=40400`、`LOSS_GET_CROSS_TENANT_CODE=40400` | `status=open` 仍返回 3 条、`total` 仍为 `len(list)`、`photos` 仍为字符串、`instrument_brief` 缺失、他人租户可读到记录（200）→ FAIL |
| R2 | fetch 桩：`/instrument-loss?page=1&pageSize=20&status=all` → 3 条（2 未恢复 1 已恢复，含 `instrument_brief.sn`）；`/instrument-loss?page=2...` → `[]` | H5 `/instrument-loss-manage`：`render(<MemoryRouter><InstrumentLossManage/></MemoryRouter>)` + `fireEvent.click` 行 / 入口 | `docs/evidence/2615/R2-list.txt` | `LOSS_LIST_ROWS=3`、`LOSS_LIST_SN_SHOWN=1`、`LOSS_LIST_TIME_SHOWN=1`、`LOSS_LIST_STATUS_BADGE=2`、`LOSS_LIST_DANGER_BUTTON_IN_ROW=0`、`LOSS_LIST_REGISTER_ENTRY=1`、`LOSS_LIST_ROW_NAV=1`、`LOSS_LIST_RENDER_FAIL=0` | 行内出现红色危险按钮、缺 SN/时间、状态徽标只 1 种、点行未跳 `/instrument-loss-detail?id=...`、入口缺失 → FAIL |
| R3 | fetch 桩：`/instruments?sn=SN-2615&page=1&pageSize=20` → 1 条；`/instruments/<id>/lost` → `{code:20000}` | H5 `/instrument-loss-register`：`render(` + `fireEvent.input`（SN 检索）+ `fireEvent.click`（选项/提交） | `docs/evidence/2615/R3-register.txt` | `LOSS_REG_SELECTOR_QUERY=1`、`LOSS_REG_PARTY_RATIO_DEFAULT=1`、`LOSS_REG_VALIDATE_NO_INSTRUMENT=1`、`LOSS_REG_VALIDATE_DESC_REQUIRED=1`、`LOSS_REG_SUBMIT_PAYLOAD=1`、`LOSS_REG_SUBMIT_BACK=1`、`LOSS_REG_RENDER_FAIL=0` | 未选乐器仍发请求、描述为空仍提交、提交体缺 `responsible_party/user_ratio/description`、提交后未返回列表 → FAIL |
| R4 | fetch 桩：`/instrument-loss/rec-open`（未恢复，`photos=["k1","k2"]`）、`/instrument-loss/rec-done`（已恢复，`restored_damaged=true`）、`/instruments/<id>/restore` → `{code:20000,data:{restored_damaged:true}}` | H5 `/instrument-loss-detail?id=rec-open` 与 `?id=rec-done`：`render(` + `fireEvent.click` | `docs/evidence/2615/R4-detail.txt` | `LOSS_DETAIL_FIELDS=1`、`LOSS_DETAIL_PHOTOS=1`、`LOSS_DETAIL_OPEN_HAS_RESTORE=1`、`LOSS_DETAIL_RESTORED_HIDES_RESTORE=1`、`LOSS_DETAIL_RESTORE_PAYLOAD=1`、`LOSS_DETAIL_RENDER_FAIL=0` | 描述/责任方/比例/金额/时间缺一、照片不渲染、已恢复仍显示恢复按钮、恢复请求体缺 `damaged/photos` → FAIL |
| R5 | 修后工作树（`~/.nvm/nvm.sh` 切 22，不可用 → exit 2） | `bash scripts/verify-2615.sh r5`（静态接线 + `build:weapp` + `build:h5` + wxml 探针 + eslint） | `docs/evidence/2615/R5-build-wiring.txt` | `WEAPP_PAGES_REGISTERED=2`、`H5_PAGES_REGISTERED=2`、`NAV_MAP_ENTRIES=2`、`WEAPP_BUILD_OK=1`、`H5_BUILD_OK=1`、`WXML_NATIVE_TAG=0`、`ESLINT_NOUNDEF=0` | 任一页未注册（构建缺模块）、nav 映射缺项、新页 wxml 含原生 HTML 交互标签、eslint `no-undef` → FAIL |
| R6 | 修后干净树 + 最新 Accepted Plan + `docs/evidence/2615/evidence.json`（`dirty=false`） | `pre-ready-check.sh 2615`（canonical 四参） | `docs/evidence/2615/pre-ready-2615.json` | exit 0、`overall=PASS`、P12 每行 `pass=true`、P13 在 `origin/issue-2615`、P17 plan 快照一致 | 任一 P 项失败 / evidence 缺行 / `dirty=true` → gate 非 0，禁止 ready |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2615.sh r1 > docs/evidence/2615/R1-api.txt` | `docs/evidence/2615/R1-api.txt` | exit 0 且含 `LOSS_LIST_TOTAL=3`、`LOSS_LIST_PAGE_LEN=2`、`LOSS_LIST_STATUS_OPEN=2`、`LOSS_LIST_STATUS_RESTORED=1`、`LOSS_LIST_BRIEF_SN=1`、`LOSS_LIST_PHOTOS_ARRAY=1`、`LOSS_GET_CODE=20000`、`LOSS_GET_BRIEF_SN=1`、`LOSS_GET_MISSING_CODE=40400`、`LOSS_GET_CROSS_TENANT_CODE=40400` | 十项 token 全为期望值 | 分页失效、`total` 非真实计数、`photos` 仍字符串、brief 缺失、跨租户 200 → FAIL | dev |
| R2 | `bash scripts/verify-2615.sh r2 > docs/evidence/2615/R2-list.txt` | `docs/evidence/2615/R2-list.txt` | exit 0 且含 `LOSS_LIST_ROWS=3`、`LOSS_LIST_SN_SHOWN=1`、`LOSS_LIST_TIME_SHOWN=1`、`LOSS_LIST_STATUS_BADGE=2`、`LOSS_LIST_DANGER_BUTTON_IN_ROW=0`、`LOSS_LIST_REGISTER_ENTRY=1`、`LOSS_LIST_ROW_NAV=1`、`LOSS_LIST_RENDER_FAIL=0` | 八项 token 全为期望值 | 行内危险按钮、缺 SN/时间、点行不跳详情、无登记入口 → FAIL | dev |
| R3 | `bash scripts/verify-2615.sh r3 > docs/evidence/2615/R3-register.txt` | `docs/evidence/2615/R3-register.txt` | exit 0 且含 `LOSS_REG_SELECTOR_QUERY=1`、`LOSS_REG_PARTY_RATIO_DEFAULT=1`、`LOSS_REG_VALIDATE_NO_INSTRUMENT=1`、`LOSS_REG_VALIDATE_DESC_REQUIRED=1`、`LOSS_REG_SUBMIT_PAYLOAD=1`、`LOSS_REG_SUBMIT_BACK=1`、`LOSS_REG_RENDER_FAIL=0` | 七项 token 全为期望值 | 缺校验、提交体字段缺失、提交后未返回 → FAIL | dev |
| R4 | `bash scripts/verify-2615.sh r4 > docs/evidence/2615/R4-detail.txt` | `docs/evidence/2615/R4-detail.txt` | exit 0 且含 `LOSS_DETAIL_FIELDS=1`、`LOSS_DETAIL_PHOTOS=1`、`LOSS_DETAIL_OPEN_HAS_RESTORE=1`、`LOSS_DETAIL_RESTORED_HIDES_RESTORE=1`、`LOSS_DETAIL_RESTORE_PAYLOAD=1`、`LOSS_DETAIL_RENDER_FAIL=0` | 六项 token 全为期望值 | 字段缺失、照片不渲染、已恢复仍可恢复、恢复载荷缺字段 → FAIL | dev |
| R5 | `bash scripts/verify-2615.sh r5 > docs/evidence/2615/R5-build-wiring.txt` | `docs/evidence/2615/R5-build-wiring.txt` | exit 0 且含 `WEAPP_PAGES_REGISTERED=2`、`H5_PAGES_REGISTERED=2`、`NAV_MAP_ENTRIES=2`、`WEAPP_BUILD_OK=1`、`H5_BUILD_OK=1`、`WXML_NATIVE_TAG=0`、`ESLINT_NOUNDEF=0` | 七项 token 全为期望值 | 页面未注册、构建失败、wxml 含原生标签、eslint no-undef → FAIL | dev |
| R6 | `~/.config/opencode/scripts/pre-ready-check.sh 2615 --summary-file docs/evidence/2615/summary.md --changed-files docs/evidence/2615/changed-files.txt --evidence-file docs/evidence/2615/evidence.json --affected-cmd "bash scripts/verify-2615.sh gate" --report docs/evidence/2615/pre-ready-2615.json` | `docs/evidence/2615/pre-ready-2615.json` | exit 0 且 `jq -e '.overall == "PASS"' docs/evidence/2615/pre-ready-2615.json` 成立；P12 对 `docs/evidence/2615/evidence.json` 每行 `pass=true` | `overall=PASS` 且证据行全 `pass=true` | 任一 P 项失败 / evidence 缺行 / `dirty=true` / plan 快照不一致 → gate 非 0，禁止 ready | dev |

### Artifact Provenance

- 生成器：`scripts/verify-2615.sh`（`r1`–`r5` 与 `gate`；`r1` 走 `cd backend && go test ./handlers/ -run '^TestInstrumentLoss2615'`，`r2`–`r4` 走 `npx vitest run --config vitest.config.js scripts/page-smoke/instrument-loss-2615.test.jsx`）。
- 每个文本 artifact 首行必须为 `# build=1.0.<fix 短码> # commit=<40 位 sha> # dirty=false`，其下一行 `GENERATED_AT_HEAD=<40 位 sha>`；锚点为本单 fix commit（可用 `FIX_COMMIT` 覆盖），证据必须在 fix commit 之后生成。
- 逐条清单（全部位于 `docs/evidence/2615/`）：
  - `docs/evidence/2615/R1-api.txt`
  - `docs/evidence/2615/R2-list.txt`
  - `docs/evidence/2615/R3-register.txt`
  - `docs/evidence/2615/R4-detail.txt`
  - `docs/evidence/2615/R5-build-wiring.txt`
  - `docs/evidence/2615/evidence.json`
  - `docs/evidence/2615/pre-ready-2615.json`
  - `docs/evidence/2615/summary.md`
  - `docs/evidence/2615/changed-files.txt`
- `evidence.json` 必含键：`schema_version`、`issue`、`build`、`commit`、`dirty`、`plan_sha256`、`environment`、`rows[]`（`id`/`command`/`exit`/`artifacts`/`assert`/`actual`/`expected`/`negative`/`negative_result`）。
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Files（变更文件清单）

- `backend/handlers/instrument_loss.go`
- `backend/handlers/instrument_loss_list_2615_test.go`
- `backend/main.go`
- `frontend-mobile/src/pages/InstrumentLossManage.jsx`
- `frontend-mobile/src/pages/InstrumentLossRegister.jsx`
- `frontend-mobile/src/pages/InstrumentLossDetail.jsx`
- `frontend-mobile/src/pages/instrument-loss-register/index.tsx`
- `frontend-mobile/src/pages/instrument-loss-detail/index.tsx`
- `frontend-mobile/src/pages-weapp/instrument-loss-register/index.tsx`
- `frontend-mobile/src/pages-weapp/instrument-loss-register/index.config.ts`
- `frontend-mobile/src/pages-weapp/instrument-loss-detail/index.tsx`
- `frontend-mobile/src/pages-weapp/instrument-loss-detail/index.config.ts`
- `frontend-mobile/src/app.config.ts`
- `frontend-mobile/src/App-H5.jsx`
- `frontend-mobile/src/platform/navigation.js`
- `frontend-mobile/scripts/page-smoke/instrument-loss-2615.test.jsx`
- `scripts/verify-2615.sh`
- `docs/spec/api/api.md`
- `docs/spec/ui/ui.md`
- `docs/cases/instrument-loss.md`
- `docs/evidence/2615/plan.md`
- `docs/evidence/2615/R1-api.txt`
- `docs/evidence/2615/R2-list.txt`
- `docs/evidence/2615/R3-register.txt`
- `docs/evidence/2615/R4-detail.txt`
- `docs/evidence/2615/R5-build-wiring.txt`
- `docs/evidence/2615/evidence.json`
- `docs/evidence/2615/pre-ready-2615.json`
- `docs/evidence/2615/summary.md`
- `docs/evidence/2615/changed-files.txt`

### Workload

- Size: L
- Basis: 后端 3 文件（List/Get/路由 + 独立测试文件）+ 前端 12 文件（3 页面 + 2 H5 薄壳 + 4 weapp 薄壳/配置 + 3 处接线）+ 3 份文档（api/ui/cases）+ 1 个渲染测试文件 + 1 个证据脚本；6 条证据行；无 migration、无跨仓依赖，但含新增 API 端点（契约变更）
- Confidence: medium

### Dependencies

无

*Model: deepseek/deepseek-flash*

