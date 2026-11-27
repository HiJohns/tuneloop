## Work Summary — #2615 丢失登记页信息架构收敛（记录列表 + 单一登记入口 + 详情页恢复）

### 变更内容

**后端（List 归一化 + 新增详情端点）**
- `List`：`page`/`pageSize`（默认 20，上限 100）+ `status=open|restored|all`（非法值 → 400/40002，不静默按 all）+ `open=true` 兼容映射；`total` 改为真实 `COUNT(*)`（原为 `len(list)`，且旧实现硬编码 `Limit(200)`）；响应行 `photos`/`restore_photos` 由 jsonb 字符串归一化为字符串数组（空值/非法 JSON → `[]`），并新增 `instrument_brief`（一次 `IN` 批量查询，禁 N+1；未命中 → `null`）。
- 新增 `Get`（`GET /api/instrument-loss/:id`）：tenant 过滤走 middleware.GetTenantID；不存在 / 非法 id / 他人租户统一 404/40400（不区分，不泄漏存在性），`tid==""` → 403/40300。
- 路由：`backend/main.go` 在 `GET /instrument-loss` 之后注册 `GET /instrument-loss/:id`（`ExplicitTenantScope` + `RequireCusPerm("instrument:read")`）。

**移动端（H5 + weapp 共用 .jsx，三页面拆分）**
- `InstrumentLossManage` 重写为**丢失记录列表**：删除「在管乐器」段与行内红色「丢失登记」按钮；行 = 封面 + SN + 类别/级别 + 丢失时间 + 状态徽标（丢失中/已恢复）+ 责任方·比例 + 赔偿/承担；整行点进详情；页首**唯一**「＋ 登记丢失」入口；20/页触底加载、`useDidShow` 回页刷新第一页。
- 新增 `InstrumentLossRegister`：SN 检索（300ms 防抖）选乐器 + 责任方/比例默认联动（用户=100%，其他=0）+ 赔偿/承担 + 描述必填 + 照片 ≤6（Taro.chooseImage → platform.uploadFile）→ `POST /instruments/:id/lost` → 回列表。
- 新增 `InstrumentLossDetail`：记录全文 + 照片多图预览；丢失中 → 「恢复上架」展开恢复表单（损坏/说明/照片）→ `POST /instruments/:id/restore`；已恢复 → 只展示恢复留痕，**不渲染恢复按钮**。参数用 useSearchParams().get('id')（禁 `useParams`）。
- 跨端接线：`frontend-mobile/src/app.config.ts`（`weappPages`/`h5Pages` 各 +2）、`frontend-mobile/src/App-H5.jsx`（+2 路由）、`frontend-mobile/src/platform/navigation.js`（+2 映射）、H5 薄壳 ×2、weapp 薄壳 + index.config.ts ×2。

**文档（Core Update）**
- `docs/spec/api/api.md` 新增 §5.12（列表/详情契约 + 既有登记/恢复端点回链）。
- `docs/spec/ui/ui.md` §3.26 改为三段式 IA。
- `docs/cases/instrument-loss.md` LS-01 / LS-05 入口改写（其余口径逐字不动）。

### 涉及文件
后端：`backend/handlers/instrument_loss.go`、`backend/handlers/instrument_loss_list_2615_test.go`、`backend/main.go`
前端：`frontend-mobile/src/pages/InstrumentLossManage.jsx`、`frontend-mobile/src/pages/InstrumentLossRegister.jsx`、`frontend-mobile/src/pages/InstrumentLossDetail.jsx`、`frontend-mobile/src/pages/instrument-loss-register/index.tsx`、`frontend-mobile/src/pages/instrument-loss-detail/index.tsx`、`frontend-mobile/src/pages-weapp/instrument-loss-register/index.tsx`、`frontend-mobile/src/pages-weapp/instrument-loss-register/index.config.ts`、`frontend-mobile/src/pages-weapp/instrument-loss-detail/index.tsx`、`frontend-mobile/src/pages-weapp/instrument-loss-detail/index.config.ts`、`frontend-mobile/src/app.config.ts`、`frontend-mobile/src/App-H5.jsx`、`frontend-mobile/src/platform/navigation.js`、`frontend-mobile/scripts/page-smoke/instrument-loss-2615.test.jsx`
脚本/文档：`scripts/verify-2615.sh`、`docs/spec/api/api.md`、`docs/spec/ui/ui.md`、`docs/cases/instrument-loss.md`

### Commit
- fix：`d7caf4f79`（IA 收敛主体）、`474380094`（证据脚本按矩阵行切分产物）——latest fix commit 为 `474380094`
- evidence：`docs(evidence): issue-2615`（汇总见 Remote 行）

### 验证层级
- 本单无 Parent（独立 Issue）→ 本单同时执行定向与全量：
  - 定向：`go test ./handlers/ -run '^TestInstrumentLoss2615'`（Postgres fixture）+ 三页渲染用例（vitest + @testing-library/react，`render`/`fireEvent`）+ `build:weapp` / `build:h5` + eslint。
  - 全量：pre-ready P5 全量 gate（HEAD vs 基线失败集对比），报告 `docs/evidence/2615/pre-ready-2615.json`。

### 矩阵 6/6

| 行 | 状态 | 证据 |
|---|---|---|
| R1 台账列表分页/状态过滤/乐器摘要/详情端点（集成测试，Postgres fixture） | PASS | `docs/evidence/2615/R1-api.txt`：LOSS_LIST_TOTAL=3、LOSS_LIST_PAGE_LEN=2、LOSS_LIST_STATUS_OPEN=2、LOSS_LIST_STATUS_RESTORED=1、LOSS_LIST_BRIEF_SN=1、LOSS_LIST_PHOTOS_ARRAY=1、LOSS_GET_CODE=20000、LOSS_GET_BRIEF_SN=1、LOSS_GET_MISSING_CODE=40400、LOSS_GET_CROSS_TENANT_CODE=40400 |
| R2 列表页 render：记录行信息 + 单一登记入口 + 无行内危险按钮 + 行点击进详情 | PASS | `docs/evidence/2615/R2-list.txt`：LOSS_LIST_ROWS=3、LOSS_LIST_SN_SHOWN=1、LOSS_LIST_TIME_SHOWN=1、LOSS_LIST_STATUS_BADGE=2、LOSS_LIST_DANGER_BUTTON_IN_ROW=0、LOSS_LIST_REGISTER_ENTRY=1、LOSS_LIST_ROW_NAV=1、LOSS_LIST_RENDER_FAIL=0 |
| R3 登记页 render：SN 检索选择器 + 校验 + 提交载荷 + 返回列表 | PASS | `docs/evidence/2615/R3-register.txt`：LOSS_REG_SELECTOR_QUERY=1、LOSS_REG_PARTY_RATIO_DEFAULT=1、LOSS_REG_VALIDATE_NO_INSTRUMENT=1、LOSS_REG_VALIDATE_DESC_REQUIRED=1、LOSS_REG_SUBMIT_PAYLOAD=1、LOSS_REG_SUBMIT_BACK=1、LOSS_REG_RENDER_FAIL=0 |
| R4 详情页 render：记录全文 + 照片 + 恢复提交 + 已恢复态隐藏恢复按钮 | PASS | `docs/evidence/2615/R4-detail.txt`：LOSS_DETAIL_FIELDS=1、LOSS_DETAIL_PHOTOS=1、LOSS_DETAIL_OPEN_HAS_RESTORE=1、LOSS_DETAIL_RESTORED_HIDES_RESTORE=1、LOSS_DETAIL_RESTORE_PAYLOAD=1、LOSS_DETAIL_RENDER_FAIL=0 |
| R5 跨端接线 + 构建 + 产物可达性 + ESLint | PASS | `docs/evidence/2615/R5-build-wiring.txt`：WEAPP_PAGES_REGISTERED=2、H5_PAGES_REGISTERED=2、H5_ROUTES_REGISTERED=2、NAV_MAP_ENTRIES=2、WEAPP_BUILD_OK=1、H5_BUILD_OK=1、WXML_NATIVE_TAG=0、WEAPP_PAGE_WXML_PRODUCED=3、ESLINT_NOUNDEF=0 |
| R6 pre-ready gate（四源一致性 + 合同完整性 + 远端可达） | PASS | `docs/evidence/2615/pre-ready-2615.json`：overall=PASS（见下方 Pre-ready gate 段） |

### Evidence Contract

| 行 | 结果 | 产物 | 关键断言 |
|---|---|---|---|
| R1 | PASS | `docs/evidence/2615/R1-api.txt` | 10 个 token 全为期望值；负例 status=bogus → 400/40002，缺失/非法 id/跨租户 → 404/40400 |
| R2 | PASS | `docs/evidence/2615/R2-list.txt` | 8 个 token 全为期望值；负例 行内 button 计数 = 0 |
| R3 | PASS | `docs/evidence/2615/R3-register.txt` | 7 个 token 全为期望值；负例 未选乐器/描述空均不发请求 |
| R4 | PASS | `docs/evidence/2615/R4-detail.txt` | 6 个 token 全为期望值；负例 已恢复记录无恢复按钮 |
| R5 | PASS | `docs/evidence/2615/R5-build-wiring.txt` | 7 个 token 全为期望值；负例 wxml 原生标签 = 0、eslint no-undef = 0 |
| R6 | PASS | `docs/evidence/2615/pre-ready-2615.json` | overall=PASS、P12 每行 pass=true、P13 提交在 origin/issue-2615 |

机读合同：`docs/evidence/2615/evidence.json`（`dirty=false`，每个合同行恰好一条记录，`pass=true`；R2/R3/R4 产物按行前缀切分，不复用其它行断言）。

### 契约三向核对

| 维度 | Plan / `docs/spec/api/api.md` §5.12 | Handler 实现 | 前端调用 |
|---|---|---|---|
| 路径 | `GET /api/instrument-loss`、`GET /api/instrument-loss/:id`、`POST /api/instruments/:id/lost`、`POST /api/instruments/:id/restore` | `backend/main.go` 注册 4 条（本单新增 `:id`） | 列表 → `GET /instrument-loss?page&pageSize&status`；详情 → `GET /instrument-loss/<id>` + `POST /instruments/<id>/restore`；登记 → `GET /instruments?sn&exclude_status` + `POST /instruments/<id>/lost` |
| 参数名 | `page`/`pageSize`/`status`/`instrument_id`/`open`（兼容）；登记体 `description`/`responsible_party`/`user_ratio`/`compensation_cents`/`user_burden_cents`/`photos`；恢复体 `damaged`/`description`/`photos` | `List` 用 c.DefaultQuery 的 page/pageSize/status/instrument_id/open；登记/恢复沿用既有 `ShouldBindJSON` 字段（未改语义） | query/body 逐字段一致；渲染用例断言发出的载荷含 `responsible_party`/`user_ratio`/`description`/`photos` 与 `damaged`/`photos` |
| 响应/错误 | `{code,data:{list,total,page,page_size}}`；行内 `photos`/`restore_photos` 数组 + `instrument_brief`；详情 404/40400、无 tid 403/40300；非法 status 400/40002 | `lossRecordJSON` 输出上述全部字段（`instrument_brief` 未命中为 `null`）；`Get` 404/40400 与 403/40300；`List` 400/40002 | 列表读 d.data.list/d.data.total；详情读 d.data；两处均判 `code === 20000` 并对错误 `resolveErrorMessage` 提示（不静默） |

既有消费方复核：`grep -rn "lossApi" frontend-pc/src --include=*.jsx` = 0（`photos` 数组化不破坏现网页面）；`open=true` 兼容分支由 R1 用例覆盖（`LOSS_LIST_STATUS_OPEN=2`）。

### 红绿实证

RED（基线的 List 行为，绑本单新测试 → 编译即失败，因为基线没有详情端点）：

```text
$ git checkout bdd14b69d -- backend/handlers/instrument_loss.go backend/main.go
$ ( cd backend && go test ./handlers/ -run '^TestInstrumentLoss2615' -count=1 )
# tuneloop-backend/handlers [tuneloop-backend/handlers.test]
handlers/instrument_loss_list_2615_test.go:44:34: h.Get undefined (type *InstrumentLossHandler has no field or method Get)
handlers/instrument_loss_list_2615_test.go:226:44: h.Get undefined (type *InstrumentLossHandler has no field or method Get)
FAIL	tuneloop-backend/handlers [build failed]
```

RED（基线静态事实，对应被测行为的「旧期望」）：

```text
$ git show bdd14b69d:backend/handlers/instrument_loss.go | grep -n 'Limit(200)\|"total": len(list)'
420:	if err := query.Order("created_at DESC").Limit(200).Find(&list).Error; err != nil {
424:	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": list, "total": len(list)}})

$ git show bdd14b69d:frontend-mobile/src/pages/InstrumentLossManage.jsx | grep -n 'btnDangerStyle' | head -3
294:              <Button onClick={() => toggle(inst.id, 'lost')} style={btnDangerStyle}>
```

GREEN（本单 fix commit `474380094`，同一测试与三页渲染用例）：`RESULT: PASS (r1)` / `RESULT: PASS (r2)` / `RESULT: PASS (r3)` / `RESULT: PASS (r4)`，见 `docs/evidence/2615/R1-api.txt`、`docs/evidence/2615/R2-list.txt`、`docs/evidence/2615/R3-register.txt`、`docs/evidence/2615/R4-detail.txt`（行内危险按钮计数 `LOSS_LIST_DANGER_BUTTON_IN_ROW=0` 即旧行为的否定断言）。

### 同类路径对称扫描

- `grep -rn "Limit(200)" backend/handlers/` → 0（旧无分页 List 已消除；其它台账端点为独立功能，不在本单范围）。
- `grep -n "btnDangerStyle" frontend-mobile/src/pages/InstrumentLossManage.jsx` → 0（列表行无危险按钮）；登记页/详情页各保留 1 个确认动作按钮（Decision Lock D6）。
- `grep -c "instrument-loss" frontend-mobile/src/app.config.ts frontend-mobile/src/platform/navigation.js frontend-mobile/src/App-H5.jsx` → 每处列表/登记/详情 3 条（新增 2 条），三处同时登记（#1989 构建前置）。

### 人工/部署侧（不进本矩阵）

- 真机像素级走查与体验版分发（`waitForConfirm` 部署侧事项）：由 Owner 在预生产体验版核对；Developer 无生产/微信后台权限，未声称已完成。

### Pre-ready gate

- 命令：`~/.config/opencode/scripts/pre-ready-check.sh 2615 --summary-file docs/evidence/2615/summary.md --changed-files docs/evidence/2615/changed-files.txt --evidence-file docs/evidence/2615/evidence.json --affected-cmd "bash scripts/verify-2615.sh gate" --report docs/evidence/2615/pre-ready-2615.json`
- 报告：`docs/evidence/2615/pre-ready-2615.json`
- 结果：`RESULT: PASS`（overall=PASS；关键输出见报告与下方清单）。

```text
@@GATE_SUMMARY@@
```

*Model: deepseek/deepseek-flash*
