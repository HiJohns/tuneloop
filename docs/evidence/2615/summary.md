## Work Summary — #2615 (Round 3 / Rev.3 D8) 丢失登记页信息架构收敛


### 本轮（Round 3，Rev.3 D8）变更：legacy 无参调用兼容 200

Rev.3 采纳 Investigation Packet 选项 1（后端兼容旧调用方）。Round 2 审计阻断项 B1 的根因是
`GET /api/instrument-loss` 默认页大小从旧的 `Limit(200)` 收到 20，而 PC 台账
（frontend-pc InstrumentStock.jsx:384 的 lossApi.list() 无参调用）仍是无参调用且表格无服务端分页
→ >20 条的租户静默少显示最多 180 条。

| 变更点 | 实现 |
|---|---|
| D8 legacy 判定 | `backend/handlers/instrument_loss.go`：`page` 与 `pageSize` **均未显式传**（用 c.GetQuery 判定参数是否存在，而非取值）→ `legacyLossPageSize=200`；**只缺其一**仍走新默认（`page=1` / `pageSize=20`）；显式 `pageSize` 仍 `clamp` 到 100 |
| 单测 | `backend/handlers/instrument_loss_list_2615_test.go` 新增 `TestInstrumentLoss2615_LegacyNoParams`（200 条记录夹具）：无参 → 200 行/200 total/page_size=200；仅 `page=1` → 20（负例）；`pageSize=1000` → clamp 100 |
| 契约文档 | `docs/spec/api/api.md` §5.12.1 登记 legacy 例外（实现与文档逐字一致） |
| 证据断言 | `scripts/verify-2615.sh` r1/gate 增 `LOSS_LEGACY_NO_PARAMS_ROWS=200`、`LOSS_LEGACY_NO_PARAMS_TOTAL=200`、`LOSS_LEGACY_PAGE_ONLY_SIZE=20` |

**红绿实证（D8）**：同一条命令、同一 harness，只回退实现（`git stash push -- backend/handlers/instrument_loss.go`）：

```text
缺陷版（HEAD^=f313bf78f 的 List）：
  LOSS_LEGACY_NO_PARAMS_ROWS=20        ← PC 台账被截断
  LOSS_LEGACY_NO_PARAMS_TOTAL=200
  --- FAIL: TestInstrumentLoss2615_LegacyNoParams (legacy no-params broke PC ledger: rows=20 total=200 page_size=20 (want 200))
修复版 b56bea461：
  LOSS_LEGACY_NO_PARAMS_ROWS=200
  LOSS_LEGACY_NO_PARAMS_TOTAL=200
  LOSS_LEGACY_PAGE_ONLY_SIZE=20
  --- PASS: TestInstrumentLoss2615_LegacyNoParams
  --- PASS: TestInstrumentLoss2615_ListAndGet
```

**范围声明**：本轮只落 Rev.3 的 D8 例外与其 3 个断言（+ 契约文档一行），未改矩阵行数、未改
Decision Lock、未改共享验收工具，也未把默认页大小改回 200（新移动端仍显式 `pageSize=20`）。
PC 台账分页 UI 仍按 Rev.2 说明另立 Issue（Out of Scope）。

### Remote

- `origin/issue-2615`；本轮 fix `b56bea461`（Round 2 的 `5e3d9c53b` 为其祖先）+ 证据提交，均已 push。
- 分支 `issue-2615`（基于 Accepted Plan 基线 `bdd14b69d`）。

### 针对审计意见的改进（Round 1 阻断项逐条）

| 审计项 | 根因分类 | 修复 |
|---|---|---|
| **B1** 详情页「现场照片 / 恢复照片」用裸 `file_key` 作 `<Image src>` 与 `previewImage` urls → weapp 不显示、H5 404；且 R4 断言把裸 key 写成期望 | 执行 | `frontend-mobile/src/pages/InstrumentLossDetail.jsx` 引入 `frontend-mobile/src/utils/media.js` 的 photoSrc：`photoUrls = photos.map(photoSrc)`、`restorePhotoUrls = restorePhotosSaved.map(photoSrc)`；缩略图 `src={photoSrc(key)}`，预览 `previewImage({ urls: photoUrls, current: photoSrc(key) })`（恢复照片同）。R4 断言改为「src / preview url = env.apiBaseUrl origin + `/uploads/media/<key>`」，并新增「裸 key 作 src 计数 = 0」的负例。 |
| **B2** 列表行封面 instrument_brief.cover_image（`/uploads/media/` 下的相对路径 相对路径）未过 `photoSrc` → weapp 缩略图不显示 | 执行 | `frontend-mobile/src/pages/InstrumentLossManage.jsx` 行内封面改为 photoSrc(brief.cover_image)；同类路径一并修：详情页头部封面（同一 Issue 新增代码）同样归一化。R2 夹具改用两种真实形态（相对路径 相对路径 /uploads/media/cover-1.webp 与裸 key 裸 key cover-2.webp），断言渲染 src 为 origin + `/uploads/media/<key>` 且裸形态计数 = 0。 |
| **B3** `lossRecordJSON` 丢弃模型 5 个字段（`reversed_at` / `reversed_amount_cents` / `deducted_damage_cents` / `deducted_idle_cents` / `reverse_note`）→ 旧 `GET /instrument-loss` 字段静默消失 | 执行 | 补齐这 5 个字段（列已作废但「保留以兼容历史行」，零成本、无 migration），并在 R1 增加 `LOSS_RESPONSE_KEEPS_REVERSED_FIELDS=1`（列表每行 + 详情都断言）。 |

附带（审计非阻断备注，同批顺手处理）：

- `scripts/verify-2615.sh` 的 `ESLINT_CMD` 文案不再打印 `<2609 changed files>` 占位符，改为真实文件列表。

### 未处理（非阻断，需 Architect / 后续 Issue）

- **计划内部矛盾（危险色）**：变更点 3 写「状态徽标 `丢失中`（`#DC2626`）」，Decision Lock D6 写「列表行一律不用危险色」。Round 1 审计已判定**非阻断**并要求 Architect 消歧；本轮按变更点 3 保持红字徽标，不改口径（改 D6 或改徽标属计划变更，超出 fixreview 范围）。
- **P16（UI render 断言）未生效**：计划把 `evidence_type: render` 放在单独的 `### Evidence Types` 表，未内联到矩阵行 → gate 的 P16 为 N/A。实际证据确实使用 `render(`/`fireEvent`/`querySelector`（非 grep）。内联属计划变更，建议 Architect 在后续计划模板中修正。
- i18n 硬编码中文（新三页）属 i18n Phase 2 目录推进范围；详情页恢复按钮位于卡片内而非「底部行动条」属视觉差异（不影响可达性）。

### 涉及文件

- `backend/handlers/instrument_loss.go`（`lossRecordJSON` 补 5 个历史字段）
- `backend/handlers/instrument_loss_list_2615_test.go`（`LOSS_RESPONSE_KEEPS_REVERSED_FIELDS`）
- `frontend-mobile/src/pages/InstrumentLossDetail.jsx`（`photoSrc`：封面 / 现场照片 / 恢复照片 / 预览 urls）
- `frontend-mobile/src/pages/InstrumentLossManage.jsx`（行内封面 `photoSrc`）
- `frontend-mobile/scripts/page-smoke/instrument-loss-2615.test.jsx`（R2/R4 断言改为归一化 URL + 裸 key 负例）
- `scripts/verify-2615.sh`（合同 token + ESLINT_CMD 文案）
- 证据：`docs/evidence/2615/R1-api.txt`、`docs/evidence/2615/R2-list.txt`、`docs/evidence/2615/R3-register.txt`、`docs/evidence/2615/R4-detail.txt`、`docs/evidence/2615/R5-build-wiring.txt`、`docs/evidence/2615/evidence.json`、`docs/evidence/2615/pre-ready-2615.json`、`docs/evidence/2615/summary.md`、`docs/evidence/2615/changed-files.txt`

无新增文件（未改计划 Files / 矩阵 / 验收工具）。

### 自证（修复类必填）

**① 红绿实证（同一 harness，缺陷版 vs 修复版）**

后端（`cd backend && go test ./handlers/ -run '^TestInstrumentLoss2615' -count=1 -v`）：

```text
缺陷版（HEAD^ = Round 1 的 lossRecordJSON）：
  LOSS_RESPONSE_KEEPS_REVERSED_FIELDS=0
  list row drops legacy field "reverse_note": 54f46f55-…
  detail drops legacy field "reversed_at" / "reversed_amount_cents" / "deducted_damage_cents" / "deducted_idle_cents" / "reverse_note"
  --- FAIL: TestInstrumentLoss2615_ListAndGet
修复版 5e3d9c53b：LOSS_RESPONSE_KEEPS_REVERSED_FIELDS=1，--- PASS: TestInstrumentLoss2615_ListAndGet
```

前端（`cd frontend-mobile && npx vitest run --config vitest.config.js scripts/page-smoke/instrument-loss-2615.test.jsx`）：

```text
缺陷版（页面回退到 Round 1）：
  LOSS_LIST_COVER_SRC_ABSOLUTE=0   LOSS_LIST_COVER_RAW_INPUT=1
  LOSS_DETAIL_PHOTOS=0             LOSS_DETAIL_PHOTO_RAW_KEY=2
  LOSS_DETAIL_RESTORE_PHOTO_SRC_ABSOLUTE=0  LOSS_DETAIL_RESTORE_PHOTO_RAW_KEY=1
  Tests 2 failed | 2 passed（preview 断言因找不到照片控件抛错）
修复版 5e3d9c53b：
  LOSS_LIST_COVER_SRC_ABSOLUTE=1   LOSS_LIST_COVER_RAW_INPUT=0
  LOSS_DETAIL_PHOTOS=1             LOSS_DETAIL_PHOTO_SRC_ABSOLUTE=1   LOSS_DETAIL_PHOTO_RAW_KEY=0
  LOSS_DETAIL_PHOTO_PREVIEW_URLS_ABSOLUTE=1
  LOSS_DETAIL_RESTORE_PHOTO_SRC_ABSOLUTE=1  LOSS_DETAIL_RESTORE_PHOTO_RAW_KEY=0
  LOSS_DETAIL_RESTORE_PHOTO_PREVIEW_URLS_ABSOLUTE=1
  Tests 4 passed (4)
```

**② 真实 `<Image src>` / preview urls 取值**（jsdom 下 env.apiBaseUrl = /api，故 origin 为空串；weapp 构建注入 `https://<host>/api`，同一条 `photoSrc` 会产出绝对 URL）：

```text
夹具：photos=['k1','k2']（裸 key）、restore_photos=['r1']、cover_image='/uploads/media/cover-1.webp' 与 'cover-2.webp'
渲染：<Image src="/uploads/media/k1">、<Image src="/uploads/media/k2">、<Image src="/uploads/media/r1">
      <Image src="/uploads/media/cover-1.webp">、<Image src="/uploads/media/cover-2.webp">
preview：previewImage({ urls: ['/uploads/media/k1','/uploads/media/k2'], current: '/uploads/media/k1' })
         → 覆盖层 <img src> = /uploads/media/k1（断言读真实 DOM 覆盖层）
```

**③ 全量编译与定向回归**

```text
$ cd backend && go build ./...                 → BUILD_RC=0
$ cd backend && go test ./... -run '^$' -count=1 → 全包编译冒烟通过（各包 ok [no tests to run]）
$ cd backend && go test ./... -count=1 -timeout 20m -p 2
  FAIL tuneloop-backend/handlers      ← TestMain: pq: database "tuneloop" does not exist（本机 localhost:5432 无该库）
  FAIL tuneloop-backend/services      ← TestBulkImportIAMRole2540: 同上（DB 不可用）
  其余包全 ok
```

上述 2 个包共 3 条失败属**本机环境既有**、与本次改动无关：同机对 `origin/develop` 基线跑 `scripts/batch-gate.sh --baseline origin/develop`（批次 `2026-11-27-2622`）得到 `FAIL_COUNT=3 / BASELINE_FAIL_COUNT=3 / FULL_GATE_NEW_FAILURES=0`，失败集合与本单完全一致（无新增失败、无新增 panic）。本单改动仅触及 `backend/handlers/instrument_loss.go` 的响应字段与两个移动端页面的媒体路径，不涉及这两个失败点的代码路径。

**④ 根因分层声明**：应用层（后端响应字段 + 移动端媒体 URL 归一化 + 自有 harness 断言），无库层/依赖变更，无 migration。

**⑤ 范围声明**：仅修复审计意见所列 B1/B2/B3 与其验收行（R1+/R2+/R4+），未改判定口径、未改共享验收工具/规则、未做 Out of Scope 的过滤 Tab / 批量扫码 / PC 台账。

### 矩阵 6/6

| 行 | 状态 | 证据 |
|---|---|---|
| R1 台账列表分页/状态过滤/乐器摘要/详情端点（集成测试，Postgres fixture） | PASS | `docs/evidence/2615/R1-api.txt`：LOSS_LIST_TOTAL=3、LOSS_LIST_PAGE_LEN=2、LOSS_LIST_STATUS_OPEN=2、LOSS_LIST_STATUS_RESTORED=1、LOSS_LIST_BRIEF_SN=1、LOSS_LIST_PHOTOS_ARRAY=1、LOSS_GET_CODE=20000、LOSS_GET_BRIEF_SN=1、LOSS_GET_MISSING_CODE=40400、LOSS_GET_CROSS_TENANT_CODE=40400、**LOSS_LEGACY_NO_PARAMS_ROWS=200、LOSS_LEGACY_NO_PARAMS_TOTAL=200、LOSS_LEGACY_PAGE_ONLY_SIZE=20**；另含 LOSS_RESPONSE_KEEPS_REVERSED_FIELDS=1 |
| R2 列表页 render：记录行信息 + 单一登记入口 + 无行内危险按钮 + 行点击进详情 | PASS | `docs/evidence/2615/R2-list.txt`：LOSS_LIST_ROWS=3、LOSS_LIST_SN_SHOWN=1、LOSS_LIST_TIME_SHOWN=1、LOSS_LIST_STATUS_BADGE=2、LOSS_LIST_DANGER_BUTTON_IN_ROW=0、LOSS_LIST_REGISTER_ENTRY=1、LOSS_LIST_ROW_NAV=1、LOSS_LIST_RENDER_FAIL=0；新增 LOSS_LIST_COVER_SRC_ABSOLUTE=1、LOSS_LIST_COVER_RAW_INPUT=0 |
| R3 登记页 render：SN 检索选择器 + 校验 + 提交载荷 + 返回列表 | PASS | `docs/evidence/2615/R3-register.txt`：LOSS_REG_SELECTOR_QUERY=1、LOSS_REG_PARTY_RATIO_DEFAULT=1、LOSS_REG_VALIDATE_NO_INSTRUMENT=1、LOSS_REG_VALIDATE_DESC_REQUIRED=1、LOSS_REG_SUBMIT_PAYLOAD=1、LOSS_REG_SUBMIT_BACK=1、LOSS_REG_RENDER_FAIL=0 |
| R4 详情页 render：记录全文 + 照片 + 恢复提交 + 已恢复态隐藏恢复按钮 | PASS | `docs/evidence/2615/R4-detail.txt`：LOSS_DETAIL_FIELDS=1、LOSS_DETAIL_PHOTOS=1、LOSS_DETAIL_PHOTO_SRC_ABSOLUTE=1、LOSS_DETAIL_PHOTO_RAW_KEY=0、LOSS_DETAIL_PHOTO_PREVIEW_URLS_ABSOLUTE=1、LOSS_DETAIL_RESTORE_PHOTO_SRC_ABSOLUTE=1、LOSS_DETAIL_RESTORE_PHOTO_RAW_KEY=0、LOSS_DETAIL_RESTORE_PHOTO_PREVIEW_URLS_ABSOLUTE=1、LOSS_DETAIL_OPEN_HAS_RESTORE=1、LOSS_DETAIL_RESTORED_HIDES_RESTORE=1、LOSS_DETAIL_RESTORE_PAYLOAD=1、LOSS_DETAIL_RENDER_FAIL=0 |
| R5 跨端接线 + 构建 + 产物可达性 + ESLint | PASS | `docs/evidence/2615/R5-build-wiring.txt`：WEAPP_PAGES_REGISTERED=2、H5_PAGES_REGISTERED=2、H5_ROUTES_REGISTERED=2、NAV_MAP_ENTRIES=2、WEAPP_BUILD_OK=1、H5_BUILD_OK=1、WXML_NATIVE_TAG=0、WEAPP_PAGE_WXML_PRODUCED=3、ESLINT_NOUNDEF=0 |
| R6 pre-ready gate（四源一致性 + 合同完整性 + 远端可达） | PASS | `docs/evidence/2615/pre-ready-2615.json`：overall=PASS（见下方 Pre-ready gate） |

未完成行：无。

### 流程可达性（Step 2.5 沙盘复核）

| 角色 | 入口 | 看到什么 | 能做什么 | 控件 | 目标端可见可点 |
|---|---|---|---|---|---|
| 网点员工（staff） | 列表页「＋ 登记丢失」 | 丢失记录行（封面/SN/类别/丢失时间/状态徽标/责任方·比例/赔偿） | 登记丢失 / 进详情 | `Button` / 整行 `View onClick` | 是（weapp wxml 原生标签 0，`WEAPP_PAGE_WXML_PRODUCED=3`） |
| 网点员工 | 列表行 → 详情 | 记录全文 + 现场照片 + 恢复留痕 | 看照片（预览）/ 恢复上架 | `View onClick` + `Image` + `Button` | 是（照片 src 为 origin 绝对/根路径，weapp 可加载） |
| 网点员工 | 详情（丢失中） | 恢复表单 | 确认恢复 | `Button`（危险色仅确认动作） | 是 |

### Evidence Contract

| 行 | 结果 | 产物 | 关键断言 |
|---|---|---|---|
| R1 | PASS | `docs/evidence/2615/R1-api.txt` | 十三项合同 token（含 LOSS_LEGACY_NO_PARAMS_ROWS=200 / LOSS_LEGACY_NO_PARAMS_TOTAL=200 / LOSS_LEGACY_PAGE_ONLY_SIZE=20）+ LOSS_RESPONSE_KEEPS_REVERSED_FIELDS=1；负例 status=bogus→400/40002、缺失/非法 id/跨租户→404/40400、无参调用被截断到 20 行 |
| R2 | PASS | `docs/evidence/2615/R2-list.txt` | 八项合同 token + 封面归一化两项；负例行内按钮 = 0、裸 key 计数 = 0 |
| R3 | PASS | `docs/evidence/2615/R3-register.txt` | 七项合同 token；负例 未选乐器/描述空均不发请求 |
| R4 | PASS | `docs/evidence/2615/R4-detail.txt` | 六项合同 token + 照片/恢复照片/预览归一化六项；负例 裸 key src 计数 = 0、已恢复无恢复按钮 |
| R5 | PASS | `docs/evidence/2615/R5-build-wiring.txt` | 七项合同 token；负例 wxml 原生标签 = 0、eslint no-undef = 0 |
| R6 | PASS | `docs/evidence/2615/pre-ready-2615.json` | overall=PASS、P12 每行 pass=true、P13 提交在 origin/issue-2615 |

机读合同：`docs/evidence/2615/evidence.json`（build=1.0.b56bea461、commit=b56bea461、dirty=false、plan_sha256=93957ba4…，6 行各一条记录且 pass=true；R2/R3/R4 产物按行前缀切分，未互相复用断言）。

### 契约三向核对

| 维度 | Plan / `docs/spec/api/api.md` §5.12 | Handler 实现 | 前端调用 |
|---|---|---|---|
| 路径 | `GET /api/instrument-loss`、`GET /api/instrument-loss/:id`、`POST /api/instruments/:id/lost`、`POST /api/instruments/:id/restore` | `backend/main.go` 注册 4 条 | 列表 → `GET /instrument-loss?page&pageSize&status`；详情 → `GET /instrument-loss/<id>` + `POST /instruments/<id>/restore`；登记 → `GET /instruments?sn&exclude_status` + `POST /instruments/<id>/lost` |
| 参数名 | `page`/`pageSize`/`status`/`instrument_id`/`open`（兼容）；page+pageSize 皆缺 → legacy page=1&pageSize=200（Rev.3 D8，api.md §5.12.1 已登记） | 用 c.GetQuery 判定参数存在性 + `legacyLossPageSize=200`；只缺其一走新默认、显式值仍 clamp 100 | 移动端三页显式传 `page&pageSize=20`（不受 legacy 影响）；PC 台账 lossApi.list() 无参 → 200 行上限（不回退） |
| 响应/错误 | `{code,data:{list,total,page,page_size}}`；行内 `photos`/`restore_photos` 数组 + `instrument_brief`；详情 404/40400、无 tid 403/40300；非法 status 400/40002 | `lossRecordJSON` 现输出全部模型字段（含 5 个历史冲正列）、`instrument_brief` 未命中为 `null` | 列表读 data.list / data.total；详情读 data；均判 `code === 20000` 并对错误提示（不静默） |

媒体契约（本次修复的核心）：`/upload` 返回的 `file_key` 是裸文件名、instruments.cover_image 是 `/uploads/media/` 下的相对路径 相对路径 —— 两者都必须经 `frontend-mobile/src/utils/media.js` 的 photoSrc 补 origin 后交给 `<Image>`（weapp `<Image>` 不吃相对路径）。

### 同类路径对称扫描

- `grep -n "photoSrc" frontend-mobile/src/pages/InstrumentLoss*.jsx` → 详情 4 处（封面 / 现场照片 src / 现场照片 preview / 恢复照片 src+preview）、列表 1 处（封面）；本 Issue 三页内已无裸 `src={key}` / src={brief.cover_image}。
- `grep -n "reversed_at\|reverse_note" backend/handlers/instrument_loss.go` → 已补齐；`grep -rn "lossRecordJSON" backend/` 仅 List/Get 两个消费方，均走同一函数。
- 其它移动端页面（`MyLeases` / `StaffOrders` / `Search` / `Checkout` 等）的封面/照片归一化属**存量**问题、不在本 Issue 新增代码内；本轮不改（超出审计意见范围），如需系统清理应由 Owner 另立 Issue。

### 人工/部署侧（不进本矩阵）

| 事项 | 负责方 | 回执要求 |
|---|---|---|
| 预生产体验版真机走查（列表缩略图可见 / 详情照片可点开 / 恢复后状态变「已恢复」） | Owner | 关单时按 `[Ops-Followup of #2615]`（`waitForConfirm`，无 `status:*`）另记 |

### Pre-ready gate

- 命令：`~/.config/opencode/scripts/pre-ready-check.sh 2615 --summary-file docs/evidence/2615/summary.md --changed-files docs/evidence/2615/changed-files.txt --evidence-file docs/evidence/2615/evidence.json --affected-cmd "bash scripts/verify-2615.sh gate" --report docs/evidence/2615/pre-ready-2615.json`
- 报告：`docs/evidence/2615/pre-ready-2615.json`
- 结果：`RESULT: PASS`（overall=PASS）。P1/P2/P3/P4/P6/P7/P11/P12/P13/P14/P17/P18 PASS；P5 N/A（仓库根无 go.mod/package.json，全量编译与定向回归见上文）；P8/P9/P10/P15/P16 N/A。
- 关键输出：`P12 PASS Evidence Contract complete: 6 row(s) match the plan and have durable artifacts`、`P14 PASS artifact provenance matches evidence (6 artifacts)`、`P13 PASS fix commit and evidence artifacts are on origin/issue-2615`、`P17 PASS plan snapshot and Accepted Plan comment match the gated plan`。

Model: deepseek/deepseek-flash
