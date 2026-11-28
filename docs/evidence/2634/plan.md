## Accepted Plan — #2634 [Gap] PC 库管工作台「扫码验收」弹窗：补入口 + 可录入/可产出结果的控件（零新依赖，复用既有验收动作）

Tags: bug, code:frontend

### 背景与现状核对（代码为据）

| # | 事实 | 依据 |
|---|---|---|
| 1 | PC 库管工作台 `/warehouse` 的「扫码验收」弹窗**没有任何入口**：`handleOpenScanner` 只有定义、**0 处调用**（全仓 grep），工具栏也没有扫码按钮 | `frontend-pc/src/pages/WarehouseManagement.jsx:72`（定义）；`grep -rn 'handleOpenScanner' frontend-pc/src` → 1 命中 |
| 2 | 该页 `QrScanner` 的 import 已被注释，但 `handleOpenScanner` 内仍 `new QrScanner(...)` → 一旦被调用即 `ReferenceError`（基线即坏） | 同文件 `:6`（注释 import）、`:80`（`new QrScanner`）；`frontend-pc/package.json` 无 `qr-scanner` 依赖 |
| 3 | 弹窗体内只有 `<video>` 相机预览 + `{scannedData && …}` 展示块，**没有可录入、可产出结果的控件** | 同文件 `:435-470`（Modal 体）；`scannedData` 仅由已死的 `handleScanResult` 赋值 |
| 4 | 列表关键字搜索框是**列表筛选**（`Input.Search` → `setKeyword`），不是扫描结果录入 | 同文件 `:357`；`fetchOrders` 用 `keyword` 参数 |
| 5 | 列表已有两条**验收动作**且已接后端：`shipped` → `handleConfirmDelivery`（`PUT /api/warehouse/orders/:id/delivery`）；`returning` → `handleInspectReturn`（`PUT /api/warehouse/orders/:id/return-inspect`，内部 `prompt` 取 SN） | `frontend-pc/src/pages/WarehouseManagement.jsx:150-175`；`backend/main.go:743`、`:818` |
| 6 | 列表 API 支持按 **订单号 / 乐器 SN / 客户姓名 / 电话** 的关键字检索（`ILIKE`），响应含 `order_no`、`instrument_sn`、`status`、`user_name`、`total` | `backend/main.go:741`（`GET /warehouse/orders`）；`backend/handlers/warehouse.go:74-110`（`keyword` / `page` / `pageSize`）、`:117-160`（`warehouseOrder` 字段 + `code:20000`） |
| 7 | 断网/未知异常的用户文案唯一出口 = `userFacingErrorMessage`（#2609 落地，§1.3） | `frontend-pc/src/services/apiError.js`；`docs/spec/api/api.md` §1.3 |
| 8 | PC 端**无** `@testing-library` / `jsdom` / 组件测试基建（devDeps 只有 vite/eslint/tailwind 等）→ 组件级点击断言不可做；确定性逻辑必须抽纯函数用 `node --test` 覆盖 | `frontend-pc/package.json` devDependencies；既有 `frontend-pc/tests/*.test.mjs` 模式 |
| 9 | `docs/spec/ui/ui.md` §3.12 只定义**移动端** `/staff/orders` 扫码；PC `/warehouse` 仅有路由表一行（`:3806`），无扫码验收规格 | `docs/spec/ui/ui.md:2709`、`:3806` |

**结论**：这是**入口缺失 + 控件缺失 + 死调用**三合一缺陷，且基线本就是坏的（打开即 `ReferenceError`，且根本打不开）。修法必须**零新依赖**（`qr-scanner` 引入需独立评估体积/许可/离线，另立 #2634 之外的决策），把弹窗做成「手动录入 → 定位订单 → 复用列表同款验收动作」，并由**纯函数**承载匹配逻辑以取得可机器判定的行为证据。

### 角色 × 状态沙盘（前端流程可达性，强制）

| 步骤 | 内容 |
|---|---|
| 1 谁能进 | 侧边「交易管理 → 库管工作台」（`/warehouse`），权限 `cusPermCodes: ['instrument:read','instrument:update']`（网点员工 / 商户管理员）；无权限由 `ProtectedRoute` 拦截（不变） |
| 2 看到什么 | 工具栏新增「扫码验收」按钮 → 弹窗：相机预览（可用时）/ 不可用提示 / 固定提示「暂不支持自动识别，请手动输入订单号或乐器序列号」/ 录入框 / 「查询并定位」/ 候选列表（多条时）/ 订单摘要（订单号、状态、乐器+SN、客户） |
| 3 能做什么 | ① 手动录入订单号或 SN → 查询定位；② `shipped` 订单 → 「确认收货」；③ `returning` 订单 → 「通过」/「拒绝」；④ 其它状态 → 仅「查看详情」；⑤ 关闭弹窗（停止相机轨道） |
| 4 哪个控件完成 | ① `Input`（绑定 `scanCode`）+ `Button`「查询并定位」；② Button → 复用 `handleConfirmDelivery(id, instrument_sn)`；③ Button → 复用 `handleInspectReturn(id, true/false, instrument_sn)`；④ Button → 复用 `handleViewDetails(order)`；⑤ Modal `onCancel` + footer「关闭」→ `handleCloseScan()` |
| 5 目标端可见可点吗 | PC（Web DOM）—— 控件为 antd `Button`/`Input`/`Select`，构建后进 `frontend-pc/dist` 产物；真实点击链路由 `[Ops-Followup of #2634]` 在预生产回评（D7：`evidence_type: static` + Owner 豁免，PC 无组件测试基建） |

### 变更点

1. **新增入口按钮**：工具栏 `Space` 内加 `<Button icon={<QrcodeOutlined />} onClick={handleOpenScanner}>扫码验收</Button>`（`QrcodeOutlined` 已在 import 中，当前未使用）。
2. **删除死代码**：移除注释掉的 `QrScanner` import 行、`new QrScanner(...)`、`handleScanResult`、`scannerRef`、`scannedData`；新增 `streamRef` 保存 `MediaStream`，在 `handleCloseScan` 与卸载时 `getTracks().forEach(t => t.stop())`（防相机灯常亮/泄漏）。
3. **相机降级**：`handleOpenScanner` 先 `setScanModalVisible(true)`，再 `if (!navigator.mediaDevices?.getUserMedia)` → 记录 `scanCameraError='当前浏览器不支持相机预览，请手动输入'`；`getUserMedia` reject → `scanCameraError='相机不可用（无权限或被占用），请手动输入'`；**任何相机失败都不得阻断录入/查询**。
4. **录入 + 查询定位**：`scanCode` state + `Input`（`onPressEnter` 与「查询并定位」按钮同入口 `handleScanQuery`）。查询走既有 `GET /warehouse/orders?keyword=<code>&page=1&pageSize=20`（不新增端点）；失败用 `userFacingErrorMessage(err, '查询订单失败')`。
5. **纯函数匹配（确定性逻辑抽取点）**：新增 `frontend-pc/src/utils/scanResolve.js` 导出 `resolveScanCandidates(orders, rawCode)`：
   - `code = String(rawCode ?? '').trim()`；空串 → `{ kind:'none', list: [] }`；`orders` 非数组 → 视为 `[]`；
   - `exact` = `order_no` 或 `instrument_sn` 与 `code` **忽略大小写全等**；命中 → `{ kind:'exact', list: 按 updated_at 倒序取前 5 }`；
   - 否则 `fuzzy` = `order_no` / `instrument_sn` **包含** `code`（忽略大小写）→ `{ kind:'fuzzy', list: 前 5 }`；
   - 否则 `{ kind:'none', list: [] }`。
6. **结果面板与动作复用**：
   - `exact` 且 1 条 → 直接展示摘要；`exact`/`fuzzy` 多条 → `Select` 候选（展示 `order_no · instrument_sn · 状态中文`），选中项展示摘要；
   - `fuzzy` → 顶部 `Alert`「未精确匹配，以下为相近订单，请确认」；`none` → `Alert`「未找到匹配订单，请确认订单号或乐器序列号」（**持久**，非一次性 toast）；
   - `shipped` → 「确认收货」→ `handleConfirmDelivery(o.id, o.instrument_sn)`；`returning` → 「通过」/「拒绝」→ `handleInspectReturn(o.id, true|false, o.instrument_sn)`；其它状态 → `Alert`「当前状态「<中文>」无需验收」+ 「查看详情」→ `handleViewDetails(o)`；
   - 三个动作成功后 `handleCloseScan()`（列表由 handler 内既有 `fetchOrders()` 刷新）。
7. **`handleInspectReturn` 兼容扩展**：签名改为 `(orderId, passed, instrumentSn)`，内部 `const sn = (instrumentSn || '').trim() || prompt('请输入乐器序列号:')`；**列表行调用点不传第三参**（行为不变），弹窗传已定位的 SN（不再二次 prompt）。SN 为空时弹窗内动作按钮 `disabled` + Tooltip「缺少乐器序列号，请在列表行手动确认」。
8. **证据脚本** `scripts/verify-2634.sh`（`r1`–`r4`，沿用 `scripts/verify-2609.sh` / `scripts/verify-2612.sh` 结构：provenance 首行、`heavy-run.sh` 取锁、`PRE_REF`/`FIX_REF` 分离）。
9. **不新增依赖、不新增 API、不改后端**：`qr-scanner` 依赖引入（方案 A）不在本单（见 Out of Scope）。

### 既有断言影响（0 翻转）

| 检查 | 结果 | 依据 |
|---|---|---|
| 断言该页扫码弹窗行为的测试 | **0 处**（无组件测试基建） | 事实 8；`grep -rn "扫码验收\|handleOpenScanner" frontend-pc/tests` → 无命中 |
| 列表行「确认收货」（`handleConfirmDelivery`）调用点 | **不变**（签名未变） | 变更点 6/7 |
| 列表行「通过/拒绝」（`handleInspectReturn`）调用点 | **行为不变**（仍走 `prompt`，仅新增可选第三参） | 变更点 7 |
| 关键字筛选 `Input.Search`（`:357`） | **不变**（弹窗录入是独立控件，不复用该 state） | 事实 4 |

### Files（变更文件清单）

- `frontend-pc/src/pages/WarehouseManagement.jsx`
- `frontend-pc/src/utils/scanResolve.js`
- `frontend-pc/tests/scanResolve.test.mjs`
- `scripts/verify-2634.sh`
- `docs/evidence/2634/plan.md`
- `docs/evidence/2634/R1-prefix-defect.txt`
- `docs/evidence/2634/R2-resolve-unit.txt`
- `docs/evidence/2634/R3-wiring.txt`
- `docs/evidence/2634/R4-build-lint.txt`
- `docs/evidence/2634/evidence.json`
- `docs/evidence/2634/pre-ready-2634.json`

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| R1 缺陷实证（修前快照） | `frontend-pc/src/pages/WarehouseManagement.jsx` | `bash scripts/verify-2634.sh r1` | 修前 `PRE_REF` | PC（静态） | `PRE_OPEN_SCANNER_CALLERS=0`、`PRE_SCAN_ENTRY_BUTTON=0`、`PRE_SCAN_INPUT_BOUND=0`、`PRE_QR_IMPORT_COMMENTED=1`、`PRE_QR_NEW_CALLS=1`、`PRE_MANUAL_HINT=0` | R1 |
| R2 纯函数匹配行为 | `frontend-pc/src/utils/scanResolve.js` | `bash scripts/verify-2634.sh r2` | `node --test`（无 DB/DOM） | PC（纯逻辑） | `SCAN_RESOLVE_CASES=9`、`EXACT_ORDER_NO=1`、`EXACT_SN=1`、`CASE_INSENSITIVE=1`、`FUZZY_FALLBACK=1`、`EMPTY_CODE_NONE=1`、`NO_MATCH_NONE=1`、`CANDIDATE_CAP_5=1`、`NULL_LIST_GUARD=1`、`EXIT=0` | R2 |
| R3 弹窗接线（入口 + 录入 + 复用动作 + 相机降级 + 死码清零） | `frontend-pc/src/pages/WarehouseManagement.jsx` | `bash scripts/verify-2634.sh r3` | 修后工作树 | PC（静态） | `ENTRY_BUTTON=1`、`OPEN_SCANNER_CALLERS=1`、`SCAN_INPUT_BOUND=1`、`SCAN_SUBMIT=1`、`SCAN_API_KEYWORD=1`、`ACTION_REUSE_DELIVERY=1`、`ACTION_REUSE_INSPECT=1`、`CAMERA_GUARD=1`、`STREAM_CLEANUP=1`、`HELPER_USED=1`、`MANUAL_HINT=1`、`QR_DEAD_CALL=0`、`QR_IMPORT_COMMENTED=0`、`LEGACY_SCAN_STATE=0` | R3 |
| R4 构建 + ESLint + 产物入口探针 | `frontend-pc/src/pages/WarehouseManagement.jsx` | `bash scripts/verify-2634.sh r4` | 修后工作树（依赖已装） | PC 构建产物 | `PC_BUILD=0`、`LINT_EXIT=0`、`BUNDLE_ENTRY=1`、`BUNDLE_QUERY=1`、`BUNDLE_HINT=1`、`BUILD_LINT_OK=1` | R4 |

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 修前 `PRE_REF`（最早 `fix(issue-2634)` 的父提交）的 `WarehouseManagement.jsx` | `bash scripts/verify-2634.sh r1` | `docs/evidence/2634/R1-prefix-defect.txt` | `PRE_OPEN_SCANNER_CALLERS=0`、`PRE_SCAN_ENTRY_BUTTON=0`、`PRE_SCAN_INPUT_BOUND=0`、`PRE_QR_IMPORT_COMMENTED=1`、`PRE_QR_NEW_CALLS=1`、`PRE_MANUAL_HINT=0` | 修前快照里已存在入口/录入控件（说明 `PRE_REF` 取错，基线不可信）→ FAIL |
| R2 | 9 个固定 fixture 订单（含 `order_no` / `instrument_sn` / 大小写混合 / 空串 / 无匹配 / >5 条候选 / `null` 列表） | `bash scripts/verify-2634.sh r2` | `docs/evidence/2634/R2-resolve-unit.txt` | `SCAN_RESOLVE_CASES=9`、`EXACT_ORDER_NO=1`、`EXACT_SN=1`、`CASE_INSENSITIVE=1`、`FUZZY_FALLBACK=1`、`EMPTY_CODE_NONE=1`、`NO_MATCH_NONE=1`、`CANDIDATE_CAP_5=1`、`NULL_LIST_GUARD=1`、`EXIT=0` | 精确匹配退回模糊、SN 不可匹配、空串变成全量、候选超过 5 条、`null` 列表抛错 → 任一即 FAIL |
| R3 | 修后工作树（`frontend-pc/src/pages/WarehouseManagement.jsx` + `frontend-pc/src/utils/scanResolve.js`） | `bash scripts/verify-2634.sh r3` | `docs/evidence/2634/R3-wiring.txt` | `ENTRY_BUTTON=1`、`OPEN_SCANNER_CALLERS=1`、`SCAN_INPUT_BOUND=1`、`SCAN_SUBMIT=1`、`SCAN_API_KEYWORD=1`、`ACTION_REUSE_DELIVERY=1`、`ACTION_REUSE_INSPECT=1`、`CAMERA_GUARD=1`、`STREAM_CLEANUP=1`、`HELPER_USED=1`、`MANUAL_HINT=1`、`QR_DEAD_CALL=0`、`QR_IMPORT_COMMENTED=0`、`LEGACY_SCAN_STATE=0` | 仍无入口（`OPEN_SCANNER_CALLERS=0`）、无录入控件、动作另写一套（不复用 handler）、相机失败阻断录入、`new QrScanner` 残留、`scannedData`/`scannerRef` 残留 → 任一即 FAIL |
| R4 | 修后工作树（`frontend-pc` 依赖已装） | `bash scripts/verify-2634.sh r4` | `docs/evidence/2634/R4-build-lint.txt` | `PC_BUILD=0`、`LINT_EXIT=0`、`BUNDLE_ENTRY=1`、`BUNDLE_QUERY=1`、`BUNDLE_HINT=1`、`BUILD_LINT_OK=1` | 构建失败、ESLint 非 0（`no-undef`/`no-unused-vars` 错误）、产物里找不到「扫码验收」/「查询并定位」/「暂不支持自动识别」→ 任一即 FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2634.sh r1 > docs/evidence/2634/R1-prefix-defect.txt` | `docs/evidence/2634/R1-prefix-defect.txt` | exit 0 且含 `PRE_OPEN_SCANNER_CALLERS=0` `PRE_SCAN_ENTRY_BUTTON=0` `PRE_SCAN_INPUT_BOUND=0` `PRE_QR_IMPORT_COMMENTED=1` `PRE_QR_NEW_CALLS=1` `PRE_MANUAL_HINT=0` | PASS（6 项 token 全为期望值） | 修前快照已含入口/录入控件（`PRE_REF` 取错）→ FAIL | dev |
| R2 | `bash scripts/verify-2634.sh r2 > docs/evidence/2634/R2-resolve-unit.txt` | `docs/evidence/2634/R2-resolve-unit.txt` | exit 0 且含 `SCAN_RESOLVE_CASES=9` `EXACT_ORDER_NO=1` `EXACT_SN=1` `CASE_INSENSITIVE=1` `FUZZY_FALLBACK=1` `EMPTY_CODE_NONE=1` `NO_MATCH_NONE=1` `CANDIDATE_CAP_5=1` `NULL_LIST_GUARD=1` `EXIT=0` | PASS（10 项 token 全为期望值） | 精确退化模糊、SN 不可匹配、空串全量、候选 >5、`null` 抛错 → FAIL | dev |
| R3 | `bash scripts/verify-2634.sh r3 > docs/evidence/2634/R3-wiring.txt` | `docs/evidence/2634/R3-wiring.txt` | exit 0 且含 `ENTRY_BUTTON=1` `OPEN_SCANNER_CALLERS=1` `SCAN_INPUT_BOUND=1` `SCAN_SUBMIT=1` `SCAN_API_KEYWORD=1` `ACTION_REUSE_DELIVERY=1` `ACTION_REUSE_INSPECT=1` `CAMERA_GUARD=1` `STREAM_CLEANUP=1` `HELPER_USED=1` `MANUAL_HINT=1` `QR_DEAD_CALL=0` `QR_IMPORT_COMMENTED=0` `LEGACY_SCAN_STATE=0` | PASS（14 项 token 全为期望值） | 无入口、无录入、动作不复用、相机失败阻断、死码残留 → FAIL | dev |
| R4 | `bash scripts/verify-2634.sh r4 > docs/evidence/2634/R4-build-lint.txt` | `docs/evidence/2634/R4-build-lint.txt` | exit 0 且含 `PC_BUILD=0` `LINT_EXIT=0` `BUNDLE_ENTRY=1` `BUNDLE_QUERY=1` `BUNDLE_HINT=1` `BUILD_LINT_OK=1` | PASS（6 项 token 全为期望值） | 构建失败 / ESLint 非 0 / 产物缺入口文案 → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2634.sh`（`r1`–`r4`）：provenance 首行、`heavy-run.sh` 取锁（`npm run build` 属重命令）。
- **两个锚点分离**（同 #2612 Rev.3 口径）：
  - `PRE_REF` = **最早** `fix(issue-2634)` 提交的父提交（= 本单分支修前基线；脚本口径 `git rev-parse "$(git log --format=%H --grep='^fix(issue-2634)' | tail -1)^"`，可用同名环境变量覆盖）；
  - `FIX_REF` = **最新** `fix(issue-2634)` 提交（provenance 的 `# commit=`）。
  - 每个 artifact 首行：`# build=1.0.<fix短码> # commit=<FIX_REF 40 位> # dirty=false`，次行 `# tree=<HEAD sha>`，第三行 `# pre_ref=<PRE_REF sha>`。
- R1 是**修前**快照，必须在 fix commit **之后**用 `git show <PRE_REF>:<path>` 读取（不得读工作树，也不得读 `FIX_REF^`）。
- 逐条清单（全部位于 `docs/evidence/2634/`）：
  - `docs/evidence/2634/R1-prefix-defect.txt`
  - `docs/evidence/2634/R2-resolve-unit.txt`
  - `docs/evidence/2634/R3-wiring.txt`
  - `docs/evidence/2634/R4-build-lint.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2634 --summary-file tmp/pre-ready-2634-summary.md --changed-files tmp/pre-ready-2634-changed.txt --evidence-file docs/evidence/2634/evidence.json --report docs/evidence/2634/pre-ready-2634.json`
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Input Truth & Tool Preflight

- **Source of truth**：
  - 页面现状 = `frontend-pc/src/pages/WarehouseManagement.jsx`（`PRE_REF` 快照 + 修后工作树）；
  - 列表/动作 API = `backend/main.go:741/743/818` + `backend/handlers/warehouse.go:74-175`（**不新增端点、不改后端**）；
  - 文案出口 = `frontend-pc/src/services/apiError.js` 的 `userFacingErrorMessage`（#2609）；契约 = `docs/spec/api/api.md` §1.3；
  - 路由/权限 = `frontend-pc/src/App.jsx:753`（`/warehouse`，`instrument:read` + `instrument:update`）。
- **实施前差异核对（先跑，结果入 Work Summary）**：
  - `grep -rn 'handleOpenScanner\|new QrScanner\|scannedData' frontend-pc/src/pages/WarehouseManagement.jsx` → 修前必须能复现事实 1–3（0 调用 / 1 死调用 / 1 预览块）；
  - `grep -n 'qr-scanner' frontend-pc/package.json` → 无命中（零依赖前提）；
  - `git merge-base --is-ancestor origin/develop HEAD` 且 `## Dependencies` #2612 已 `done` → 否则 **HOLD**（见 D1）。
- **损坏输入拒绝规则**：
  - 空串 / 纯空白录入 → 不查询、不弹「未找到」，直接提示「请输入订单号或乐器序列号」；
  - `orders` 为 `null`/非数组 → 视为空列表，不得抛 `TypeError`；
  - 候选 > 5 → 只展示前 5 条并提示「匹配到 N 条，仅列出前 5 条」；
  - 动作按钮在 `instrument_sn` 缺失时**必须 disabled**（禁止用空 SN 调后端）。
- **工具盲区（Work Summary 必须记录）**：
  - PC 无组件测试基建（事实 8）→ 点击链路只由静态接线断言 + 构建产物探针覆盖；真实浏览器点击/相机权限由 `[Ops-Followup of #2634]` 在预生产回评；
  - 静态断言只能证明「代码存在且形态符合」，不能证明视觉排版；
  - 逐行中文措辞由评审 rubric 判定（动词 + 宾语，与所在操作一致）。
- **契约影响**：零契约变更（复用既有 `GET /warehouse/orders`、`PUT /warehouse/orders/:id/delivery`、`PUT /warehouse/orders/:id/return-inspect`）→ Work Summary 需 `### 契约三向核对`（pre-ready P7）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | **依赖 #2612**：同一文件（`WarehouseManagement.jsx`）在 #2612 里被改（3 处 toast + 删除 `QrScanner` 死码）；本单必须在 #2612 `done` 后再开工（基座 = `origin/develop` 已含其改动），避免同区冲突与 lint 口径分裂。未满足 → `/work` HOLD | 本计划锁定 |
| D2 | **零新依赖**：不引入 `qr-scanner`（方案 A）；弹窗为「手动录入 + 相机预览（辅助）」；自动识别另立评估 Issue（不在本单） | 本计划锁定 |
| D3 | **提交语义 = 定位 + 复用列表动作**（不自动验收）：录入并命中订单后展示摘要 + 与列表行**完全相同**的按钮（确认收货 / 通过 / 拒绝），点击调用既有 handler 与既有 API；不做任何自动判断/自动通过 | Architect（本计划锁定；若 Owner 要求改为「提交即自动验收」，须回 Architect 改计划） |
| D4 | 匹配口径 = 精确（`order_no` / `instrument_sn`，忽略大小写）优先 → 模糊包含兜底 → 无匹配；候选上限 5；逻辑落在纯函数 `resolveScanCandidates`（可 `node --test`） | 本计划锁定 |
| D5 | 不改后端、不新增 API 端点、不改 `docs/spec/api/api.md` | 本计划锁定 |
| D6 | 相机不可用（无 API / 拒绝权限 / 被占用）→ **不得阻断**录入与验收动作，只显示提示 | 本计划锁定 |
| D7 | UI 证据 = 源码结构断言 + 构建产物探针（PC 无组件测试基建）；真实浏览器点击/相机权限由 Ops 在预生产回评（**Owner 豁免**，`evidence_type: static`） | Architect |
| D8 | `PRE_REF` / `FIX_REF` 分离（同 #2612 Rev.3）：修前快照用最早 fix 提交的父提交，provenance 用最新 fix | 本计划锁定 |
| D9 | 重命令（`npm run build`）经 `heavy-run.sh`；`npx eslint` 只覆盖本单改动文件 | 本计划锁定 |
| D10 | `docs/spec/ui/ui.md` 未定义 PC 库管工作台扫码验收（事实 9）→ 本单不改核心文档（Case C：非冲突），规格补齐另议 | Architect |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | static | 修前缺陷实证（无入口 / 无录入 / 死调用）；Owner 豁免见 D7 |
| R2 | unit | 纯函数 `resolveScanCandidates`（`node --test`，9 个 fixture） |
| R3 | static | 修后接线结构断言（入口/录入/动作复用/相机降级/死码清零）；Owner 豁免见 D7 |
| R4 | build | `npm run build` + `npx eslint` + 产物文案探针 |

### Out of Scope

- **引入 `qr-scanner`（方案 A，自动识别二维码）**：需先评估打包体积/许可证/CSP/离线可用性 → 另立评估 Issue（本单不引入依赖）。
- 后端任何改动（含给扫码新增端点/字段）。
- `frontend-mobile`（H5 / weapp）的扫码流程（已由 `platform.scanQRCode()` 覆盖，见 `docs/spec/ui/ui.md` §3.12）。
- 把列表关键字筛选框改成扫码录入（保持现有筛选语义不变）。
- `docs/spec/ui/ui.md` 补 PC 库管工作台扫码验收规格（D10；本单不改核心文档）。
- 为 PC 引入 `@testing-library` / jsdom 组件测试基建（另立 Issue，本单用纯函数 + 静态断言）。
- 真实浏览器点击/相机权限/离线表现复验 → `[Ops-Followup of #2634]`（closing 时建，`waitForConfirm`，无 `status:*`）。

### Dependencies

- #2612

### Workload

- Size: M
- Basis: 1 个页面文件改造（入口 + 弹窗 + 动作复用 + 相机轨道清理）+ 1 个纯函数模块 + 1 个单测（9 fixture）+ 1 个证据脚本（`r1`–`r4`）+ 4 条证据行 + 1 条 gate；零后端、零新依赖、零新 API、零契约变更
- Confidence: high

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状（symptom） | 库管工作台看不到「扫码验收」入口；即使打开弹窗也只有相机预览，没有任何可录入/可产出结果的控件，验收无法推进 | 事实 1/3 |
| 2 | 触发条件（trigger） | 员工需要按订单号/SN 核对并验收（`shipped` 确认收货、`returning` 通过/拒绝）时 | 事实 1/5 |
| 3 | 根因（root cause） | 功能只做了一半：`handleOpenScanner` 无调用点（入口缺失）+ 依赖未安装但保留 `new QrScanner`（死调用）+ 弹窗无录入控件 | 事实 1/2/3 |
| 4 | 测试盲点（test blind spot） | PC 无组件测试基建，且该页无任何扫码相关断言；构建通过 ≠ 控件可见可点（#2122 同类教训） | 事实 8 |
| 5 | 同类扫描（same-class paths） | 仅此一处（全仓 `handleOpenScanner` 1 命中）；移动端扫码有独立实现，不在本单 | 事实 1、`grep -rn '扫码验收'` |
| 6 | 修复层（fix layer） | 入口层 + 弹窗控件层；匹配逻辑抽纯函数；动作复用既有 handler/API（不下沉到后端） | 变更点 1–7 |
| 7 | 红绿实证（red-green） | 修前 `PRE_OPEN_SCANNER_CALLERS=0` / `PRE_SCAN_INPUT_BOUND=0` / `PRE_QR_NEW_CALLS=1`；修后对应 token 翻转（1/1/0），纯函数 9 fixture 覆盖精确/模糊/空/无匹配/上限/`null` | R1–R3 |
| 8 | 防复发（prevention） | ① 入口与控件由静态接线断言 + 构建产物探针双钉；② 死码/注释 import 断言为 0；③ 匹配逻辑纯函数化，后续改口径必过单测 | R2–R4、D4 |
| 9 | 影响面（blast radius） | 仅该页扫码弹窗 + `handleInspectReturn` 新增可选第三参（列表行行为不变）；不动列表筛选、发货、定损、导出 | 既有断言影响表 |
| 10 | 未修边界（deferred） | 自动二维码识别（`qr-scanner`）、组件测试基建、ui.md 规格 → 另立；真实点击/相机 → `[Ops-Followup of #2634]` | Out of Scope |

*Model: DeepSeek V4.1 Flash 1017*

