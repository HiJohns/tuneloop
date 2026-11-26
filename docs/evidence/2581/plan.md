## Accepted Plan — #2581 网点重名冲突：中文具体原因 + 40900 持久提示全链路口径锁定

> 来源：#2496（#2480 预生产复验）Owner 2026-11-26 在 `1.0.102` 复验第 8/9 项判 FAIL：「网点管理 → 新建网点」第二次用同名「海淀店」，用户看到的是**通用**「数据冲突，请刷新后重试」，读不出「商户内网点名重复」这一真实原因。

### 背景与现状核对（代码为据）

| # | 事实 | 位置 |
|---|---|---|
| 1 | `CreateSite` 的 IAM 组织名冲突分支返回 `409/40900`，但 message 是**英文**：`"organization name already exists: " + err.Error()` | `backend/handlers/site.go:340-350` |
| 2 | 该分支用**字符串匹配**判定冲突（`"name conflict"` / `"already exists"`），而 IAM 客户端已提供哨兵 `ErrOrganizationNameConflict`（#2552 已裁定「handlers 用 errors.Is 映射，禁止字符串匹配」） | `backend/handlers/site.go:342`、`backend/services/iam_client.go:67-72,356-364` |
| 3 | 同一 handler 的**改名**分支早已返回中文（`网点名已存在` / `网点名已存在（IAM 组织名冲突）`）→ 创建分支是唯一口径离群点 | `backend/handlers/site.go:625-627,665-668` |
| 4 | 前端 `resolveErrorMessage` 口径 = 「**含 CJK** 的后端 message 原样透传；ASCII 一律落 code 家族」→ 英文 message 被 `40900` 通用文案「数据冲突，请刷新后重试」遮蔽 | `frontend-pc/src/services/errorMessages.js:3-9,30`、`frontend-pc/tests/errorMessages.test.mjs:18-41` |
| 5 | 前端 **409 从不抛异常**：`resolveHttpFailure` 对 409 返回对象（`kind=result`）→ `handleSubmit` 的 try 分支 `isBusinessCorrectableConflict(result)` → `setFormConflict` → 渲染 `FormConflictAlert`（常驻）；catch 分支同样按 rubric 判定 `ApiError`（#2563） | `frontend-pc/src/services/apiError.js:39-49`、`frontend-pc/src/pages/SiteManagement.jsx:283-287,311-319,505` |
| 6 | 预生产 `1.0.102`（`876b85881`）**已包含** #2480（`f44853fe1`）与 #2563（`d16262e32`）——`git merge-base --is-ancestor` 均成立 → 源码口径下 409 应落常驻提示；Owner 观测到的「数秒消失的 toast」与源码不符 | `git merge-base --is-ancestor f44853fe1 876b85881` / `... d16262e32 876b85881` → 0 |

**结论**：本单的**代码缺陷**只有一条——`CreateSite` 重名分支的 message 是 ASCII 英文，被前端 CJK 优先口径判为「无具体原因」而降级为 `40900` 通用文案。前端「40900 → 常驻提示」链路在源码中已闭合（事实 5），因此本单**不重写页面逻辑**，而是（a）修后端文案，（b）用回归用例把「409 不抛错 / 中文透传 / 常驻提示接线」钉死（防止再次回归），（c）把「预生产仍出现 toast」作为部署产物新鲜度核对项交部署侧复验（D5）。

### 变更点

1. **后端重名文案（唯一实现点）** — `backend/handlers/site.go`：
   - 判据改为 `errors.Is(err, services.ErrOrganizationNameConflict)`；**保留**既有字符串匹配作为兼容兜底（真实 IAM 历史 message 可能是 `already exists`），两者取或；
   - message 改为中文具体原因：`fmt.Sprintf("该商户下已存在同名网点「%s」，请换一个名称", strings.TrimSpace(req.Name))`，`code` 仍为 `40900`、HTTP 仍为 `409`；
   - 其余分支（50000 等）不动；不新增本地重名预检（归 #2477，见 D3）。
2. **前端**：**不改生产代码**。仅新增 `frontend-pc/tests/siteConflict2581.test.mjs`，把三段口径钉死：
   - 409 + 中文 message → `kind=result`、`code=40900`、message **逐字透传**（不被 `ERROR_CODE_MAP[40900]` 遮蔽）、`isBusinessCorrectableConflict=true`、`conflictText` 原样返回；
   - **负例**：409 + 英文 message（旧实现）→ 落通用「数据冲突，请刷新后重试」（这正是本缺陷的用户可见形态，作为红样保留）；
   - catch 路径：带 `code=40900` 的 `ApiError` 同样命中 rubric（#2563 语义不回退）。
3. **回归测试** — `backend/handlers/site_create_conflict_2581_test.go`：复用 #2479 的 `testfixtures` / `testutil.TestActor` / `postCreateSite2479` / `seedMerchantForSiteCreate2479` 脚手架，新增 fake IAM（第 1 次 `POST /organizations` 返回 200，第 2 次返回 409）；覆盖同商户重名（R1）与跨商户同名（R2）。
4. **证据 harness** — `scripts/verify-2581.sh`（`r1`–`r5`），重命令经 `~/.config/opencode/scripts/heavy-run.sh` 取锁；产物落 `docs/evidence/2581/`，首行 provenance `# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`。
5. **文档** — `docs/spec/api/api.md` §4.5.1 补 409 语义与中文文案口径（见 `## Core Update`）。

### Files（变更文件清单）

- `backend/handlers/site.go`
- `backend/handlers/site_create_conflict_2581_test.go`
- `frontend-pc/tests/siteConflict2581.test.mjs`
- `frontend-pc/src/pages/SiteManagement.jsx`
- `scripts/verify-2581.sh`
- `docs/spec/api/api.md`
- `docs/evidence/2581/plan.md`
- `docs/evidence/2581/R1-create-duplicate.txt`
- `docs/evidence/2581/R2-cross-merchant.txt`
- `docs/evidence/2581/R3-frontend-chain.txt`
- `docs/evidence/2581/R4-page-wiring.txt`
- `docs/evidence/2581/R5-build-lint.txt`
- `docs/evidence/2581/evidence.json`
- `docs/evidence/2581/pre-ready-2581.json`

### Core Update

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | §4.5.1「创建网点」只有请求/响应与 #2266、组织模型注，**未写重名错误的 code 与文案口径** | 在 §4.5.1 注区新增一条：同一商户（`sites.tenant_id` = 商户 `org_id`）内已存在同名网点 / IAM 组织名冲突 → `409/40900`，`message` 为中文具体原因 `该商户下已存在同名网点「<name>」，请换一个名称`（与 §4.5.2 改名口径一致）；PC 端据 `40900` 走表单内持久提示（`FormConflictAlert`），不得降级为 toast；跨商户同名仍允许（唯一性 scope = 商户内） | 后端 message 语义变更后不同步即与实现漂移，审计以 api.md 为口径会判 REJECT |

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| R1 重名 409 中文原因 + 不落第二个网点 | `backend/handlers/site.go`、`backend/handlers/site_create_conflict_2581_test.go` | `POST /api/merchant/sites` | 同商户已建「海淀店」，再提交同名 | backend（Postgres 测试库） | `DUP_STATUS=409`、`DUP_CODE=40900`、`DUP_MSG_CJK=1`、`DUP_MSG_HAS_NAME=1`、`DUP_SITES_COUNT=1`、`DUP_ORG_CALLS=2`、`RED_ENGLISH_MSG_PRE=1` | R1 |
| R2 跨商户同名仍允许（防预检过宽） | `backend/handlers/site_create_conflict_2581_test.go` | `POST /api/merchant/sites` | 商户 A、商户 B 各自建「海淀店」 | backend（Postgres 测试库） | `CROSS_FIRST=201`、`CROSS_SECOND=201`、`CROSS_TENANTS=2`、`CROSS_SAME_NAME=1` | R2 |
| R3 409 + 中文 message 的纯函数链路（透传不被遮蔽 / rubric / catch 路径） | `frontend-pc/tests/siteConflict2581.test.mjs` | `node --test`（经 verify r3） | 409 body `40900` + 中文文案；负例 409 body 英文文案；catch 路径带 code 的 ApiError | PC（Node 纯函数） | `CHAIN_KIND=result`、`CHAIN_CODE=40900`、`CHAIN_MSG_CJK=1`、`CHAIN_TEXT_MATCH=1`、`CHAIN_RUBRIC=1`、`NEG_ENGLISH_GENERIC=1`、`NODE_TEST_FAIL=0` | R3 |
| R4 页面接线：冲突落常驻 Alert、非 toast（源码结构断言） | `frontend-pc/src/pages/SiteManagement.jsx` | 源码结构断言（`evidence_type: static`，Owner 豁免） | 修后源码 | PC | `SITE_ALERT_MOUNTED=1`、`SITE_CONFLICT_BEFORE_TOAST=1`、`SITE_TOAST_ON_CONFLICT=0`、`SITE_CATCH_RUBRIC=1`、`SITE_TIMER_DISMISS=0` | R4 |
| R5 构建 / lint / 前端单测回归 | `scripts/verify-2581.sh` | `cd backend && go build` + `eslint` + `node --test` | 修后工作树 | 双端 | `GO_BUILD_OK=1`、`ESLINT_NOUNDEF=0`、`NODE_TEST_FAIL=0` | R5 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | integration | Go 定向测试 + Postgres 测试库 + fake IAM，断言 HTTP/code/message 语言与库内行数 |
| R2 | integration | 同上，跨租户同名（防止 #2477 预检过宽） |
| R3 | unit | `node --test` 纯函数链路（409 → rubric → 文案） |
| R4 | static | PC 无组件测试运行器；按 **Owner 豁免**（沿用 #2480 D6 / #2578 D7 / #2528 / #2526 D6）以源码结构断言 + 纯函数单测 + 构建为 PC 证据，真实点击/形态归部署侧复验 |
| R5 | build | `go build ./...` + `eslint`（no-undef）+ `node --test` |

### Input Truth & Tool Preflight

- **Source of truth**：
  - 用户可见文案口径 = `frontend-pc/src/services/errorMessages.js` 的 L2「含 CJK 的 message 原样透传」（`docs/spec/api/api.md:39-42` 同源契约）；
  - 冲突判据唯一权威 = `services.ErrOrganizationNameConflict`（`backend/services/iam_client.go:67-72`，#2552）；
  - 前端常驻提示唯一 owner = `frontend-pc/src/utils/formConflict.js` + `frontend-pc/src/components/FormConflictAlert.jsx`（禁止页面自写 code 白名单）。
- **差异核对（实施前先跑，记录到 Work Summary）**：
  - `grep -n "organization name already exists" backend/handlers/site.go` → 修后应为 0；
  - `grep -n "ErrOrganizationNameConflict" backend/handlers/site.go` → 修后 ≥1；
  - `grep -c "message.error" frontend-pc/src/pages/SiteManagement.jsx` → 修后仍为 1（仅 catch 兜底），且冲突分支先 `return`；
  - `grep -n "site-form-conflict" frontend-pc/src/pages/SiteManagement.jsx` → 1（Alert 唯一挂载点）。
- **损坏输入拒绝**：`name` 纯空白 → 仍由既有的既有路径决定（本单不改绑定校验）；IAM 返回非 409 冲突（网络/权限）→ 仍走既有 500 分支，不冒充当重名（禁止静默吞错）。
- **工具盲区**：fake IAM 不实现「组织名唯一性」语义，因此 R2 只能证明「turboloop 本地不拦截跨商户同名」，不能证明 IAM 侧父级隔离；IAM 侧语义由 beaconiam 组织模型保证，不在本单范围。PC 无渲染测试运行器 → 真实形态（常驻 vs toast）归部署侧复验（D5）。
- **契约影响**：本单改 `docs/spec/api/api.md` + handler 文案 → **Work Summary 必须含 `### 契约三向核对`**（pre-ready P7）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 重名文案统一为 `该商户下已存在同名网点「<name>」，请换一个名称`（说明绑定范围=商户内 + 具体冲突名 + 可行动建议）；与 §4.5.2 改名口径、#2477 计划文案一致 | 本计划锁定 |
| D2 | 冲突判定以哨兵 `errors.Is(err, services.ErrOrganizationNameConflict)` 为主，保留既有字符串匹配兜底，**不**改 409→其它状态的映射 | 本计划锁定（#2552 口径 + 兼容历史 message） |
| D3 | 本地重名**预检/名称归一化/唯一索引**归 #2477（与 #2455 绑定），本单**不实现**，避免两单在同一分支重复落地；#2581 只保证「IAM 报冲突时的用户可见文案」 | 本计划锁定（#2477 为 holdon，不构成本单依赖） |
| D4 | 前端不重写：`SiteManagement.jsx` 现有 40900 → `setFormConflict` → `FormConflictAlert` 链路保持不变，仅以测试与结构断言锁定；**禁止**在本单新增第二套冲突判定 | 本计划锁定 |
| D5 | 预生产「仍为 toast」属**部署侧复验**项：源码已证明 409 不抛错（事实 5）。复验前须先核对 PC 产物新鲜度（页面 bundle 是否含 `site-form-conflict`、nginx 挂载 inode 是否刷新）——产物陈旧则重新发布，不视为代码缺陷；复验回评由 `[Ops-Followup of #2581]`（`waitForConfirm`）承载 | Owner / ops |
| D6 | PC 证据形态 = 纯函数单测 + 源码结构断言 + 构建；真实渲染/形态走查归部署侧。**Owner 豁免**（沿用 #2480 D6 / #2578 D7 / #2528 / #2526 D6） | Owner 已豁免（同族先例） |
| D7 | 重命令（`go build`/`go test`/`node --test`/`eslint`）一律经 `heavy-run.sh` 取锁，单条 ≤5 分钟；禁止把全量 `go test ./...` 作为合同行 | 本计划锁定 |

### Out of Scope

- **重名预检 / 名称归一化 / 唯一索引**（`lower(trim(name))`、空白变体拦截、迁移）→ **#2477**（holdon，依赖 #2455），本单不实现、不改其判定。
- `sites` 表唯一约束与存量脏数据清理 → #2455 / #2477。
- 其余 handler 的英文 `40900` message（`CreateMerchant` 的 `Merchant name conflict`、`instrument_loss`、`repair_service`、`platform_staff` 等）不在本单；PC 表单类页面文案收敛由 **#2489** 承接，若审计要求逐条收敛则回 Architect 另立 Issue。
- UI 形态/规格文档（`docs/spec/ui/ui.md`）不新增持久提示规格：本单不改变 UI 形态，`FormConflictAlert` 的共享规格归 #2489 族。
- 商户重名（`merchants.name`）与 #2459 的改名口径。
- H5 / 微信小程序任何改动（该缺陷为 PC 后台路径）。
- 全量 `go test ./...` 作为合同行（由 pre-ready P5 baseline 差分承担）。

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 商户 M（`org_id`=T，tenant=T）；fake IAM 第 1 次 `POST /organizations` → 200（org O1），第 2 次 → 409；请求 `{"name":"海淀店"}` 两次 | `POST /api/merchant/sites`（`TestCreateSiteDuplicateName2581`） | `docs/evidence/2581/R1-create-duplicate.txt` | `DUP_STATUS=409`、`DUP_CODE=40900`、`DUP_MSG_CJK=1`、`DUP_MSG_HAS_NAME=1`（message 含「海淀店」）、`DUP_SITES_COUNT=1`（库内该 tenant 仍只有 1 行）、`DUP_ORG_CALLS=2`、`RED_ENGLISH_MSG_PRE=1`（fix^ 源码含英文 message 字面量） | message 含 `organization name already exists` / `already exists`（ASCII 未清除）、`DUP_SITES_COUNT=2`（落了第二个网点）、HTTP≠409 或 code≠40900 → 任一即 FAIL |
| R2 | 商户 A（org=TA）、商户 B（org=TB）；fake IAM 每次 `POST /organizations` 返回递增 org | `POST /api/merchant/sites`（`TestCreateSiteCrossMerchantSameName2581`） | `docs/evidence/2581/R2-cross-merchant.txt` | `CROSS_FIRST=201`、`CROSS_SECOND=201`、`CROSS_TENANTS=2`、`CROSS_SAME_NAME=1`（两行同名不同 tenant） | 第二次 409（跨商户被误拦）或两行同 tenant → FAIL |
| R3 | 409 body：`{code:40900,message:"该商户下已存在同名网点「海淀店」，请换一个名称"}`；负例 `{code:40900,message:"organization name already exists: iam organization name conflict"}`；catch：`new Error` 挂 `code=40900` | `node --test frontend-pc/tests/siteConflict2581.test.mjs`（`verify-2581.sh r3`） | `docs/evidence/2581/R3-frontend-chain.txt` | `CHAIN_KIND=result`、`CHAIN_CODE=40900`、`CHAIN_MSG_CJK=1`、`CHAIN_TEXT_MATCH=1`、`CHAIN_RUBRIC=1`、`NEG_ENGLISH_GENERIC=1`、`NODE_TEST_FAIL=0` | 中文 message 被替换成「数据冲突，请刷新后重试」（`CHAIN_TEXT_MATCH=0`）、409 变成 throw（`CHAIN_KIND≠result`）、catch 的 ApiError 不被 rubric 命中 → 任一即 FAIL |
| R4 | 修后 `SiteManagement.jsx` 源码 | `bash scripts/verify-2581.sh r4` | `docs/evidence/2581/R4-page-wiring.txt` | `SITE_ALERT_MOUNTED=1`、`SITE_CONFLICT_BEFORE_TOAST=1`、`SITE_TOAST_ON_CONFLICT=0`、`SITE_CATCH_RUBRIC=1`、`SITE_TIMER_DISMISS=0` | 冲突分支内出现 `message.error`、冲突分支缺少 `return`、Alert 未挂载、用定时器自动消失 → 任一即 FAIL |
| R5 | 修后工作树 | `bash scripts/verify-2581.sh r5` | `docs/evidence/2581/R5-build-lint.txt` | `GO_BUILD_OK=1`、`ESLINT_NOUNDEF=0`、`NODE_TEST_FAIL=0` | `go build` 失败、eslint 出现 `no-undef`、前端单测失败 → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2581.sh r1` | `docs/evidence/2581/R1-create-duplicate.txt` | exit 0 且含 `DUP_STATUS=409` `DUP_CODE=40900` `DUP_MSG_CJK=1` `DUP_MSG_HAS_NAME=1` `DUP_SITES_COUNT=1` `DUP_ORG_CALLS=2` `RED_ENGLISH_MSG_PRE=1` | PASS（七项 token 全为期望值） | message 含 ASCII `already exists`、`DUP_SITES_COUNT=2`、HTTP≠409 → FAIL | dev |
| R2 | `bash scripts/verify-2581.sh r2` | `docs/evidence/2581/R2-cross-merchant.txt` | exit 0 且含 `CROSS_FIRST=201` `CROSS_SECOND=201` `CROSS_TENANTS=2` `CROSS_SAME_NAME=1` | PASS（四项 token 全为期望值） | 第二次请求 409、两行同 tenant → FAIL | dev |
| R3 | `bash scripts/verify-2581.sh r3` | `docs/evidence/2581/R3-frontend-chain.txt` | exit 0 且含 `CHAIN_KIND=result` `CHAIN_CODE=40900` `CHAIN_MSG_CJK=1` `CHAIN_TEXT_MATCH=1` `CHAIN_RUBRIC=1` `NEG_ENGLISH_GENERIC=1` `NODE_TEST_FAIL=0` | PASS（七项 token 全为期望值） | 中文被通用文案遮蔽、409 被抛成异常、catch 的 ApiError 不命中 rubric → FAIL | dev |
| R4 | `bash scripts/verify-2581.sh r4` | `docs/evidence/2581/R4-page-wiring.txt` | exit 0 且含 `SITE_ALERT_MOUNTED=1` `SITE_CONFLICT_BEFORE_TOAST=1` `SITE_TOAST_ON_CONFLICT=0` `SITE_CATCH_RUBRIC=1` `SITE_TIMER_DISMISS=0` | PASS（五项 token 全为期望值） | 冲突分支出现 toast、缺 `return`、Alert 未挂载、定时器自动消失 → FAIL | dev |
| R5 | `bash scripts/verify-2581.sh r5` | `docs/evidence/2581/R5-build-lint.txt` | exit 0 且含 `GO_BUILD_OK=1` `ESLINT_NOUNDEF=0` `NODE_TEST_FAIL=0` | PASS（三项 token 全为期望值） | `go build` 失败、eslint `no-undef` > 0、前端单测失败 → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2581.sh`（`r1`–`r5`），每个文本 artifact 首行：`# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，另起一行 `GENERATED_AT_HEAD=<40 位 sha>`，证据须在 fix commit **之后**生成。
- 逐条清单（全部位于 `docs/evidence/2581/`）：
  - `docs/evidence/2581/R1-create-duplicate.txt`
  - `docs/evidence/2581/R2-cross-merchant.txt`
  - `docs/evidence/2581/R3-frontend-chain.txt`
  - `docs/evidence/2581/R4-page-wiring.txt`
  - `docs/evidence/2581/R5-build-lint.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2581 --summary-file tmp/pre-ready-2581-summary.md --changed-files tmp/pre-ready-2581-changed.txt --evidence-file docs/evidence/2581/evidence.json --report docs/evidence/2581/pre-ready-2581.json`
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Dependencies

无

### Workload

- Size: M
- Basis: 后端 2 文件（handler 文案 + 1 个集成测试）+ 前端 1 个纯函数测试文件 + api.md 文档 + 1 个 5 行证据 harness；无 migration、无外部依赖、5 条证据行
- Confidence: high

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状（symptom） | 网点重名提交后用户只看到通用「数据冲突，请刷新后重试」，读不出「商户内网点名重复」 | #2496 评论（`1.0.102` 复验 FAIL）、`frontend-pc/src/services/errorMessages.js:30` |
| 2 | 触发条件（trigger） | 同一商户（`sites.tenant_id` = 商户 org）内第二次提交同名网点 → 后端 `409/40900` | `backend/handlers/site.go:340-350` |
| 3 | 根因（root cause） | 后端 message 为 ASCII 英文；前端 CJK 优先口径判其「无具体原因」→ 落 `ERROR_CODE_MAP[40900]` 通用文案。**不是**前端丢了冲突，而是后端没给出可展示原因 | `frontend-pc/src/services/errorMessages.js:3-9`、`frontend-pc/tests/errorMessages.test.mjs:18-41` |
| 4 | 测试盲点（test blind spot） | 既有测试只断言「英文 message → 通用文案」（反向把缺陷固化为期望），**从未**断言 `CreateSite` 409 的 message 语言，也没有任何用例覆盖 `CreateSite` 重名分支 | `frontend-pc/tests/apiError.test.mjs:41-52`、`backend/handlers/org_management_test.go`（无重名用例） |
| 5 | 同类扫描（same-class paths） | 同 handler `UpdateSite` 已中文（`网点名已存在`）、`transit_site_admin.go` 已中文（`同名组织已存在，请更换名称后重试`）→ 创建分支是离群点；`CreateMerchant`（`Merchant name conflict`）、`instrument_loss`、`repair_service`、`platform_staff` 仍英文（Out of Scope / #2489 族） | `backend/handlers/site.go:625-627,665-668`、`backend/handlers/transit_site_admin.go:120-124`、`backend/handlers/merchant.go:346-352` |
| 6 | 修复层（fix layer） | 落在 handler 文案 + 哨兵判据（`errors.Is`），不下沉到前端补丁；前端只由测试锁定既有口径，避免出现第二套冲突判定 | 本计划 R1 / D2 / D4 |
| 7 | 红绿实证（red-green） | 红：`git show <fix^>:backend/handlers/site.go` 含 `organization name already exists: ` 字面量（`RED_ENGLISH_MSG_PRE=1`）；绿：修后同一场景返回含「海淀店」的中文 message（`DUP_MSG_CJK=1` `DUP_MSG_HAS_NAME=1`） | 本计划 R1 |
| 8 | 防复发（prevention） | test 断言 message **语言 + 具体名**（不止 code）；`api.md` §4.5.1 写入文案口径；R3/R4 token 锁死「409 → 常驻提示」链路，禁止回退成 toast | 本计划 R1/R3/R4 + Core Update |
| 9 | 影响面（blast radius） | 只改 `CreateSite` 的 409 message 字符串与判据；HTTP 状态/code/控制流均不变，`sites` 写入路径不变；跨商户同名行为有 R2 守卫 | 本计划 R1/R2 |
| 10 | 未修边界（deferred） | 本地预检/归一化/唯一索引 → #2477（+ #2455）；其余英文 40900 文案 → #2489 族 / 另行立单；预生产形态复验 → `[Ops-Followup of #2581]`（`waitForConfirm`，无 `status:*`） | 本计划 Out of Scope、D3、D5 |

*Model: deepseek/deepseek-flash*

