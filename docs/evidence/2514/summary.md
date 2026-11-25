## Work Summary — #2514 批量设价路径 404：接通真实端点 + 共享 payload 构建器 + 仅保留可用语义

### 修改内容

- `frontend-pc/src/utils/batchPricing.js`（新增，唯一 owner，D2）
  - `BATCH_PRICE_TYPE_OPTIONS` / `BATCH_PRICE_OPERATOR_OPTIONS`：UI 选项与后端能力同源，只暴露「日租金 + 设置为」。
  - `buildBatchPricingPayload(ids, { price_type, amount })` → `{payload}` 或 `{error}`：
    - 空选择 / 非数组 ids → 「请先选择乐器」；
    - `price_type !== 'daily_rate'` → 「批量设价当前仅支持『日租金』+『设置为』」；
    - 金额空 / NaN / 非有限数 / ≤0 → 「金额必须大于 0」（后端对 ≤0 会静默跳过，必须前置拦截）；
    - 合法 → `{items: ids.map(id => ({ id, base_daily_rate: Number(amount) }))}`（单位元）。
  - `batchPricingFeedback(res, requested)` → `{level, text}`：`data.updated >= requested` → `success`「已修改 N 个乐器价格」，否则 `warning`「已修改 X/N 个乐器，其余未更新（详见后端日志）」；`res` 为 null / 无 data.updated 按 0 处理。
- `frontend-pc/src/pages/InstrumentStock.jsx`（库存监控 `/site/stock`）
  - 删除自写 api.put('/instruments/batch/price', { ids, price_type, amount, operator })，改走 `buildBatchPricingPayload` + pricingApi.batchSetPricing（即 `PUT /instruments/batch-pricing`）。
  - `error` 分支提示并 return；成功分支用 `batchPricingFeedback` 决定 success/warning；仍 `loadData()` 刷新。
  - 模态选项改为共享常量，新增说明行「当前仅支持按日租金批量设置为固定金额」。
- `frontend-pc/src/pages/admin/instrument/List.jsx`（乐器列表 `/instruments/list`）
  - 同一套改动（复用同一构建器/反馈函数，未复制映射逻辑）；`handleBatchPriceSubmit` 失败分支由 `throw` 改为 message.error + return，避免把后端 message 再包一层。
- `frontend-pc/tests/batchPricing.test.mjs`（新增）：`node --test` 6 个用例覆盖合法映射、空 ids/非数组、非法 price_type、金额 0/NaN/负数/缺失、`updated==requested` / 部分 / 缺失、选项同源。
- 核心文档（Core Update）：`docs/spec/api/api.md` 新增 §5.6.2 批量设价——路径 `PUT /api/instruments/batch-pricing`、权限、`items` 必填、`base_daily_rate` 单位元、响应 data.updated、已知边界（逐条失败只回计数 / 周月租与增减乘未实现 → #2519）、历史 ghost 路径说明。
- `scripts/verify-2514.sh`（新增）：R1–R6 证据 harness + `gate` 子命令（供 `--full-gate-cmd`）。

Commit：`8f7dc8c05`（fix，含 plan 快照）。

### 验证层级（Issue #70）

- 独立 Issue（无 Parent）：前端定向 `node --test frontend-pc/tests/batchPricing.test.mjs`（6 pass / 0 fail）+ `npm run build`（exit 0）+ ESLint 改动 4 个文件（0 error；10 个既有 warning）+ R4 路径扫描（前端写调用 ↔ 后端注册表）。
- 无后端改动、无 migration，未跑 go build/test。

### Remote

- remote: origin/issue-2514; fix=8f7dc8c05 pushed=yes; evidence=<evidence commit> pushed=yes

### 红绿实证

- 修前（`origin/develop`，可复现）：两页均 api.put('/instruments/batch/price', { ids, price_type, amount, operator })；backend/main.go 第 589 行 只注册 `PUT /instruments/batch-pricing`，且 handler 绑定 `{items:[{id, base_daily_rate}]}` → 路径 404；即使改对路径，payload 也会因 `items` 缺失 400。
- 修后：R2/R3 断言 `DEAD_PATH=0`（无 api.put('/instruments/batch/price' 调用形态）、`SHARED_BUILDER=1`、`USES_SHARED_API=1`、`OPTIONS_FROM_SHARED=1`、`PARTIAL_WARN=1`；R4 `BATCH_PRICE_CALLS=0`、`RESIDUAL_OUTSIDE_ALLOWLIST=0`、后端 `BACKEND_BATCH_PRICING_ROUTE=1`、`BACKEND_GHOST_ROUTE=0`。
- 单测红绿由内容保证：R1 用 `node --test` + 真实模块探针；若 amount=0 仍生成 payload、或部分更新报 success、或选项里重新出现 weekly_rate，R1 直接 fail。

### Accepted Plan 对照（6/6 变更点，n/n）

1. 共享构建器 frontend-pc/src/utils/batchPricing.js（选项 + payload + 反馈）→ 完成（唯一 owner）。
2. 单测 frontend-pc/tests/batchPricing.test.mjs（node --test）→ 完成（6 用例）。
3. 库存监控页接线 → 完成。
4. 乐器列表页接线（复用同一实现）→ 完成。
5. api.md §5.6.2 契约 → 完成（含 #2519 边界与 ghost 路径说明）。
6. `scripts/verify-2514.sh`（r1–r6 + gate）→ 完成。

说明：计划文字写 instrumentsApi.batchSetPricing，实际共享客户端方法挂在 pricingApi.batchSetPricing（frontend-pc/src/services/api.js（第 563 行），`PUT /instruments/batch-pricing`）——本单复用既有方法，未新增客户端方法（符合「影响面：api.js 已有 batchSetPricing」）。

### Pre-ready gate

- RESULT: PASS → `docs/evidence/2514/pre-ready-2514.json`。
- 合同 7 行（R1–R7）逐行 PASS；P13 fix 与证据在 `origin/issue-2514`；P5 `--full-gate-cmd "bash scripts/verify-2514.sh gate"`（HEAD clean worktree 构建 + lint）。
- 运行期参数：`--fix-commit 8f7dc8c05`、`--no-gate-cache`（宿主缓存存着依赖缺失时的旧 rc=127 记录）、`--affected-cmd "node --test frontend-pc/tests/batchPricing.test.mjs"`。
- 依赖可见性：P5 在 worktree 下新建干净 worktree 构建，npm 依赖不在版本库；本轮在 worktree 的 tmp 目录建了指向已安装 node_modules 的链接（tmp 已 gitignore，工作树仍 clean）。只共享依赖，构建源码仍来自该干净 worktree 的 HEAD。

### Evidence Contract（7/7 PASS）

- R1 → PASS → `docs/evidence/2514/R1-batch-pricing-fn.txt`（`# fail 0`、`ITEM_KEYS=id,base_daily_rate`、`UNSUPPORTED_REJECTED=1`、`ZERO_REJECTED=1`、`FEEDBACK_WARN=1`）
- R2 → PASS → `docs/evidence/2514/R2-stock-page-static.txt`
- R3 → PASS → `docs/evidence/2514/R3-list-page-static.txt`
- R4 → PASS → `docs/evidence/2514/R4-path-scan.txt`
- R5 → PASS → `docs/evidence/2514/R5-docs-sync.txt`
- R6 → PASS → `docs/evidence/2514/R6-build-lint.txt`
- R7 → PASS → `docs/evidence/2514/pre-ready-2514.json`
- 账本：`docs/evidence/2514/evidence.json`（schema v1，build=1.0.8f7dc8c05、dirty=false）。

### G0 Golden Acceptance

- R1：合法输入 2 条 items、键集合 `id,base_daily_rate`、金额 100（元）；空 ids / weekly_rate / 0 / NaN 全部前置拒绝且无 payload；`updated=2/2` → success，`1/2`、缺失 → warning。
- R2/R3：五点结构断言（死路径 0、共享构建器、共享 API、选项同源、部分失败提示）+ 额外检查（旧价格类型/操作符 0、范围说明 1）。
- R4：`BATCH_PRICE_CALLS=0`；残留集合仅 `/instruments/batch/status`（#2519）与 `DELETE /instruments/batch`；后端无 ghost 路由。
- R5：文档含 `items` 必填、元单位、#2519 边界与 ghost 路径提示。
- R6：`BUILD_EXIT=0`、`ESLINT_NO_UNDEF=0`。
- R7：gate overall=PASS、零 FAIL、dirty=false。

### Verification Matrix n/n

| 变更点 | 状态 | 证据 |
|---|---|---|
| R1 共享 payload/反馈纯函数 | PASS | docs/evidence/2514/R1-batch-pricing-fn.txt |
| R2 库存监控页接线 | PASS | docs/evidence/2514/R2-stock-page-static.txt |
| R3 乐器列表页接线 | PASS | docs/evidence/2514/R3-list-page-static.txt |
| R4 同类路径扫描 | PASS | docs/evidence/2514/R4-path-scan.txt |
| R5 文档契约 | PASS | docs/evidence/2514/R5-docs-sync.txt |
| R6 全量构建 + ESLint | PASS | docs/evidence/2514/R6-build-lint.txt |
| R7 pre-ready gate | PASS | docs/evidence/2514/pre-ready-2514.json |

无 RISK 行。

### Dependency Order / 验收规则

- 无前置层依赖；本单未修改任何验收工具/规则（pre-ready-check.sh / evidence-artifacts.sh / plan-lint.sh 均未改）。
- D5 Owner 豁免沿用：PC 无组件测试运行器 → R2/R3 静态结构断言 + `evidence_type: static`（同 #2469/#2480 惯例），真实浏览器点击验证在 Out of Scope。

### 同族扫描（同类路径对称扫描义务）

- 前端 `/instruments/batch*` 写调用全集（R4 打印）：`batch-pricing` 3 处（api.js 定义 + RentSetting.jsx + Detail.jsx，均正确）、`batch/status` 1 处（后端无端点 → #2519）、`DELETE /instruments/batch` 2 处（后端存在）、`batch-import` 1 处（后端存在，已核验）。
- 非 batch 的 `/instruments` 写调用 20 处（路径参数形态的既有端点）已列出，本单未改动。
- 后端 ghost 路由扫描：`/instruments/batch/price` 在 `backend/main.go` 命中 0。

### Issue 问题点覆盖

1. 前端路径改为 `/instruments/batch-pricing` 并写进 api.md → 完成（C 变更点）。
2. 页面补 `code !== 20000` 分支、禁止假成功 → 完成（两页均先判 code，再按 data.updated 决定 success/warning）。
3. 回归：成功返回 20000 后刷新列表、失败显示具体原因 → 完成（成功路径仍 `loadData()` / `fetchInstruments()`；失败显示后端 message 或前置校验文案）。
4. 同类扫描并列出其它 404 风险点 → 完成（R4 + 上节清单；`batch/status` → #2519）。

### 人工/部署侧

- 浏览器真机点击验证（端到端批量设价）不在本单（Out of Scope，D5）；生产环境核验由部署侧承担（Developer 无生产访问权）。

*Model: DeepSeek V4.1 Flash 1017*
