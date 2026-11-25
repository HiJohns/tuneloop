## Accepted Plan — #2516（租户管理：删除无后端支撑的写入口 + 判 code 去假成功 + 列表字段对齐）

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| 后端只有 `GET /system/clients`、`GET /system/tenants`，**无任何写路由** | `backend/main.go:918-919` |
| `GetTenants` 实际返回 `{code:20000, data:{list,total,page,page_size}}` | `backend/handlers/system.go:46-68` |
| `models.Tenant` 字段为 `id/name/status/description/created_at/updated_at` | `backend/models/models.go:788-795` |
| `tenants.id` = **IAM 组织 ID**；租户行是 IAM 组织镜像（商户创建时 upsert，缺失时按 IAM 回填） | `backend/handlers/merchant.go:389-397`、`backend/handlers/public.go:23-47` |
| 前端「创建租户」调 `POST /system/tenants` → 恒 404 | `frontend-pc/src/pages/TenantManagement.jsx:42-52` |
| `request()` 对 404 **返回对象而非抛错** → 未判 `code` 即 `message.success` = 假成功 | `frontend-pc/src/services/api.js:331-341` |
| 列表拉取未判 `code`（404 时静默空表） | `frontend-pc/src/pages/TenantManagement.jsx:23-35` |
| 列表列 `dataIndex: 'tenant_id'` 与实际字段 `id` 不符 → **租户ID 列恒空**；状态列恒写死「启用」 | `frontend-pc/src/pages/TenantManagement.jsx:60-71` |
| `iamAdminApi` 汇总了 tenant/client 的 5 个写 helper，全仓**无引用** | `frontend-pc/src/services/api.js:651-664` |
| `docs/spec/api/api.md` §12.2 仍写旧响应 `data.tenants[]`（`tenant_id/tenant_name/org_count`），与 #2160 后的 `{list,total}` 不符 | `docs/spec/api/api.md:6525-6545` |
| `docs/spec/ui/ui.md` 把租户管理列为系统管理子菜单（bit 6），**但无创建语义** | `docs/spec/ui/ui.md:3150` |
| 该页**无菜单入口**，仅 `<Route path="/system/tenants">` 可直达（菜单入口/页面去留另立 #2518） | `frontend-pc/src/App.jsx:735` |

**结论**：租户（= IAM 组织镜像）由**商户创建**派生（`POST /merchants`，需 `sys_perm tenant_create`）。平台端从未有、规范也从未定义「独立建租户」端点，因此正确修法不是补 `POST /system/tenants`（会在无 IAM 组织的情况下写入悬空 `tenants` 行，破坏 `tenants.id = org id` 不变量），而是**移除该写入口 + 判 code 去假成功 + 让只读列表与真实响应对齐**。

### 变更点

**A. 决策（产品口径锁定）**
1. 平台端「租户管理」= **只读列表**（租户随「商户管理」创建商户自动生成）。
2. 不新增任何 `/system/tenants` 写端点（`POST/PUT/DELETE`），不改 IAM 客户端。

**B. 代码**
3. 新增 `frontend-pc/src/utils/apiResult.js`（共享终判，唯一 owner）：
   - `isApiOk(res)` → `Boolean(res) && Number(res.code) === 20000`；
   - `apiFailureMessage(res, fallback = '请求失败，请重试')` → `res.message` 为非空字符串则用之，否则 fallback；
   - 覆盖 `request()` 的 404/409「返回对象不抛错」语义（`api.js:331-353`），供本页与 #2517 复用。
4. `frontend-pc/src/pages/TenantManagement.jsx`：
   - 删除「创建租户」按钮、`Modal`/`Form`/`Input`/`Select`/`Option`/`PlusOutlined` 相关代码与 `handleCreateTenant`/`handleSubmit`；
   - `fetchTenants` 改为 `const res = await api.get(...)`；`if (!isApiOk(res)) { message.error(apiFailureMessage(res, '加载租户列表失败')); setTenants([]); setTotal(0); return }`；成功分支取 `res.data.list` / `res.data.total`；
   - 表头下方加静态说明 `Alert`：「租户随商户创建自动生成；新建租户请在『商户管理』完成」；
   - 列表列对齐真实响应：`dataIndex: 'id'`（租户ID）、状态列按 `record.status` 渲染（`active` → 启用 / `inactive` → 停用 / 其它 → 未知），`created_at` 保持 `formatBeijingDate`。
5. `frontend-pc/src/services/api.js`：从 `iamAdminApi` 删除 `createTenant`/`getTenant`/`updateTenant`/`deleteTenant`（无后端路由、全仓无引用），保留 `getTenants`（供后续只读消费）。
6. 新增 `frontend-pc/tests/apiResult.test.mjs`（`node --test`）：`isApiOk({code:20000})=true`；`{code:40400}`/`{code:50000}`/`{}`/`null`/`undefined`/`{code:'40400'}` → `false`；`apiFailureMessage({code:40400,message:'未找到相关数据'})` 取 message、`message:''` / 非字符串 → fallback。
7. 新增 `scripts/verify-2516.sh`（r1–r6 证据生成器，首行写 provenance）。

**C. 文档**
8. `docs/spec/api/api.md` §12.2 重写：分页参数 `page`/`page_size`；响应 `{list,total,page,page_size}`，元素字段 `id/name/status/description/created_at`；显式声明**无** `POST/PUT/DELETE /system/tenants`，租户由 `POST /api/merchants` 创建商户时自动 upsert。
9. `docs/spec/ui/ui.md` §5 系统管理 租户管理条目下补一行只读语义（创建走商户管理；列表字段对齐 `GET /system/tenants`）。

- Tags: bug, scope:pc, scope:frontend

### Workload

- Size: M
- Basis: 4 个前端代码/测试文件 + 1 个证据脚本 + 2 个核心文档 + 7 条证据行；无 migration、无后端改动、无外部依赖
- Confidence: high

## Core Update

| 文档路径 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | §12.2 写 `data.tenants[]`、`tenant_id/tenant_name/org_count`（#2160 之前形态） | 改为 `{list,total,page,page_size}` + `id/name/status/description/created_at`，并注明「无写端点，租户由 POST /api/merchants 派生」 | 文档陈旧误导前端（本 bug 的来源之一），且需固化「无独立建租户」口径 |
| `docs/spec/ui/ui.md` | §5 系统管理仅列「租户管理 (`/system/tenants`) — sys_perm bit 6」 | 该条目下补「只读列表」语义：无新建/编辑/删除；创建商户时生成；列表字段对齐实际响应 | 移除创建入口属 UI 行为变更，必须在 UI 规格中留痕，避免后续再次「补按钮」 |

## Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 触发条件 | 平台管理员直达 `/system/tenants` → 点「创建租户」→ 填名称/Owner → 提交 → `POST /system/tenants` → 404 → `request()` 返回 `{code:40400}` → 未判 code → toast「租户创建成功」 | `TenantManagement.jsx:42-52`、`api.js:331-341` |
| 2 | 根因 | 前端按陈旧文档/旧页面写接口，后端从未注册写路由；`request()` 对 404 不抛错使缺陷静默；列表拉取同样未判 code，404 表现为「空租户列表」 | `api.js:331-341`、`main.go:918-919` |
| 3 | 为何未被发现 | PC 无组件测试运行器；页面无菜单入口，手工回归难以触达；构建通过不代表路径存在 | `frontend-pc/package.json`（无 test 脚本）、`App.jsx` menuConfig 缺项 |
| 4 | 同类扫描结果 | `/system` 前缀同族：客户端页 3 处写调用（#2517）；`iamAdminApi` 另有 3 个 client 写 helper + 3 个 tenant 写 helper（本单删 tenant 侧） | `ClientManagement.jsx:55/67/70`、`api.js:651-664` |
| 5 | 防复发（前端） | 引入 `isApiOk`/`apiFailureMessage` 终判共享件；本页所有写路径删除，只剩 1 个读路径且必判 code | 本计划 R1/R2 |
| 6 | 防复发（文档） | api.md 固化「无写端点」+ 真实响应；ui.md 固化只读语义 | 本计划 R5 |
| 7 | 回归测试 | `node --test` 覆盖 40400/50000/null/非数字 code 判失败；静态断言写入口与假成功提示为 0 | 本计划 R1/R2 |
| 8 | 假阴性反例 | 若把 `{code:40400}` 判为成功、或重新出现 `api.post('/system/tenants'`、或列表仍用 `dataIndex: 'tenant_id'` → 断言必须 FAIL | R1/R2 负例列 |
| 9 | 影响面 | `GET /system/tenants` 无其它消费方；`iamAdminApi` 全仓无 import；删除 helper 不产生悬空引用 | `grep -rn "iamAdminApi" frontend-pc/src` 仅定义处 |
| 10 | 未修边界（已立项） | 客户端页写入口 404 → #2517；菜单入口/页面去留 → #2518；`POST /system/tenants`（如需）= 未立项，需 Owner 决策（D1） | #2517、#2518 |

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 终判纯函数（20000 放行、404 判失败） | `frontend-pc/src/utils/apiResult.js`、`frontend-pc/tests/apiResult.test.mjs` | `node --test` | `{code:20000}` / `{code:40400}` / `{code:50000}` / `{}` / `null` / `{code:'40400'}`；`message` 为空/非字符串 | Node 20 | `isApiOk` 仅 20000 为 true；`apiFailureMessage` 优先 message 否则 fallback；负例：40400 判 true → FAIL | R1 |
| R2 写入口移除 + 判 code + 列表字段对齐（页面结构） | `frontend-pc/src/pages/TenantManagement.jsx` | `/system/tenants` URL 直达 | 修后源码结构（创建按钮/Modal 已删；读路径判 code；列 `id`/`status`） | PC | `WRITE_ENTRY=0`、`CREATE_MODAL=0`、`CODE_GUARD=1`、`FAIL_BRANCH_MSG=1`、`COLUMN_ID=1`、`COLUMN_STATUS_DYNAMIC=1`、`READONLY_HINT=1`（静态结构断言 + Owner 豁免 D5） | R2 |
| R3 死 helper 清理 | `frontend-pc/src/services/api.js` | `iamAdminApi` | 全仓引用扫描 | PC | 4 个 tenant 写 helper 命中 0；`getTenants` 保留 1 处；负例：残留 `createTenant` → FAIL | R3 |
| R4 同类扫描（`/system` 前缀前后端路径比对） | `scripts/verify-2516.sh` | 文本比对（前端调用 ↔ 后端注册表） | 修后全仓 `frontend-pc/src` | 前端 + 后端路由表 | `TENANT_WRITE_MISMATCH=0`；`RESIDUAL_OUTSIDE_ALLOWLIST=0`；残留集合 ⊆ 声明的 #2517 清单 | R4 |
| R5 文档同步 | `docs/spec/api/api.md`、`docs/spec/ui/ui.md` | §12.2、§5 系统管理 | 修后文本 | 文档 | api.md 含 `list,total,page,page_size` 与「无 POST/PUT/DELETE」；ui.md 租户条目含「只读」；负例：仍写 `data.tenants` → FAIL | R5 |
| R6 全量构建 + ESLint | `frontend-pc`（vite build / eslint） | `cd frontend-pc` | 修后干净树 | PC | `npm run build` exit 0；`npx eslint` 无 `no-undef` 错误；负例：构建 exit != 0 → FAIL | R6 |
| R7 pre-ready gate（四源 + 全量构建） | `docs/evidence/2516/pre-ready-2516.json` | `pre-ready-check.sh 2516` | 修后树（干净）+ 最新 Accepted Plan | PC | `GATE_RESULT=PASS`、P12 evidence 完整、P13 修后提交在 `origin/issue-2516` | R7 |

## Input Truth & Tool Preflight

- **Source of truth（唯一权威）**：后端注册表 `backend/main.go`（`/system` 仅 918-919 两条 GET）；`backend/handlers/system.go:46-68` 的响应形态；`backend/models/models.go:788-795` 的 `Tenant` 字段；`backend/handlers/merchant.go:389-397`（租户随商户创建 upsert）与 `backend/handlers/public.go:23-47`（租户=IAM 组织名镜像）证明「无独立建租户」。
- **差异核对（修前实测命令，均在 /analyze 只读执行）**：
  - `grep -n 'system' backend/main.go` → 仅 918/919 两条 GET（无 POST）；
  - `grep -rn "'/system\|\`/system" frontend-pc/src --include='*.js' --include='*.jsx'` → tenant 写调用 1 处、client 写调用 3 处；
  - `sed -n '331,353p' frontend-pc/src/services/api.js` → 404/409 返回对象、不抛错；
  - `sed -n '6525,6545p' docs/spec/api/api.md` → 旧响应 `data.tenants[]`；
  - `awk '/const menuConfig = \[/,/^  \]/' frontend-pc/src/App.jsx | grep -oE "key: '[^']+'"` → 无 `/system/tenants`、无 `/system/clients` 菜单项（入口缺失 → #2518）。
- **损坏输入拒绝规则**：`isApiOk(null|undefined|{}|{code:'x'})` 必须为 `false` 且不抛错；`apiFailureMessage` 的 `message` 非字符串/空白 → fallback（禁止把 `undefined` 渲染进提示）；页面在失败分支必须清空 `tenants/total`，不得沿用上一次数据。
- **工具盲区声明**：
  - R4 扫描是**文本级**：只覆盖 `/system` 前缀与 `api.js` 中 `iamAdminApi` 的静态字面量路径；动态拼接、后端 group 前缀、模板字符串中的条件拼接可能漏检 → 全仓全量比对不在本单（Out of Scope）。
  - PC **无组件测试运行器**（`frontend-pc/package.json` 只有 dev/build/lint/preview；`ImportResultModal.test.jsx` 为无 runner 的历史残留）→ R2 采用**静态结构断言**，并在 Decision Lock D5 记录 Owner 豁免；本页当前无菜单入口，图表级渲染截图的入口决策归 #2518。
- **证据时序**：所有 artifact 必须在 fix commit **之后**由 `scripts/verify-2516.sh` 重新生成；`evidence.json` 的 `build/commit/dirty` 由脚本从 `git rev-parse` 派生，禁止手改。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 产品口径：平台端**不**提供独立建租户，租户随「商户管理」创建商户派生（`POST /merchants`）；本单删除创建入口，不新增写端点 | Architect | 本计划锁定（代码为据：`tenants.id = IAM org id`；api.md 无写端点） | 若 Owner 需要独立建租户 → 回 Architect 另立 Issue（需 beaconiam 组织/命名空间语义 + 权限位），本单不猜做 |
| D2 | 菜单入口/页面去留**不在本单**，交由 #2518 决策；本单不新增、不删除 menu 项 | Product Owner | 待 #2518 | 不阻塞本单：无论 #2518 选「暴露只读」或「删除页面」，本单修后的只读列表均为正确前置 |
| D3 | 列表字段口径以**实际响应**为准（`id/name/status/created_at`），api.md 同步为 `{list,total,page,page_size}` | Architect | 本计划锁定（#2160 已改响应，文档未同步） | — |
| D4 | 终判共享件唯一 owner = `frontend-pc/src/utils/apiResult.js`；本页与 #2517 只消费，禁止各页自写 `res.code` 判定/提示拼接 | Architect | 本计划锁定 | 阻塞实现 |
| D5 | R2 证据形态：静态结构断言 + **Owner 豁免**（沿用本仓前端豁免惯例：PC 无组件测试运行器；参照 #2480 D6 / #2469 D3） | Architect | Owner 豁免 | 若审计要求 render → 另立前端测试基建 Issue（Out of Scope） |
| D6 | 同类扫描范围：仅 `/system` 前缀 + `iamAdminApi` 静态字面量；client 侧残留以 #2517 承接，Allowlist 断言为「残留 ⊆ 声明清单」 | Architect | 本计划锁定 | 若要求全仓比对工具 → 另立 Issue |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R2 | static | 租户页源码结构断言（写入口/判 code/列映射）；Owner 豁免见 D5（PC 无组件测试 runner） |
| R4 | static+scan | 文本级路径比对（可失败），非渲染证据 |
| R5 | static | 文档段落断言（可失败） |

## Out of Scope

- `/system/clients` 写入口 404、`redirect_uris` 字符串 `.join`、状态列写死、api.md §12.1 陈旧 → **#2517**（已创建，同批同类扫描产物）。
- 客户端管理/租户管理的**菜单入口**与页面去留 → **#2518**（已创建；本单 D2 明确不处理）。
- 新增 `POST/PUT/DELETE /system/tenants`（含 IAM 组织/命名空间创建语义与权限位设计）→ 未立项，需 Owner 决策（D1）。
- 全仓前后端路径全量比对工具（跨全部前缀、含动态拼接解析）→ 不在本单（D6）。本单仅 `/system` 前缀 + `iamAdminApi` 文本扫描。
- 租户停用/删除/编辑能力（后端无对应端点）。
- PC 组件测试基建（vitest/@testing-library/jsdom 引入）→ 另立 Issue。
- mobile 端（H5 / 微信小程序）：不涉及该页面。
- `GetTenants` 不加租户隔离（平台级端点，`sys_perm tenant_list` 语义），不改后端行为。

## G0 Golden Acceptance

| 行 ID | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） | assertions（含负例） |
|---|---|---|---|---|
| R1 | `{code:20000}`、`{code:40400,message:'未找到相关数据'}`、`{code:50000}`、`{}`、`null`、`undefined`、`{code:'40400'}`、`{message:''}`、`{message:123}` | `frontend-pc/src/utils/apiResult.js` 经 `node --test` | `docs/evidence/2516/R1-api-result-unit.txt` | 断言 `isApiOk(20000)=true`、`40400/50000/{}/null/undefined/'40400' = false`；`apiFailureMessage` 取中文 message、空/非字符串回 fallback；负例：40400 判 true 或 null 抛错 → FAIL |
| R2 | 修后 `TenantManagement.jsx` 源码（唯一文件） | `/system/tenants` 页面组件 | `docs/evidence/2516/R2-tenant-page-static.txt` | `WRITE_ENTRY=0`、`CREATE_MODAL=0`、`CODE_GUARD=1`、`FAIL_BRANCH_MSG=1`、`COLUMN_ID=1`、`COLUMN_STATUS_DYNAMIC=1`、`READONLY_HINT=1`；负例：残留 `api.post('/system/tenants'` 或 `dataIndex: 'tenant_id'` → FAIL |
| R3 | 修后 `frontend-pc/src` 全树 + `api.js` | `iamAdminApi` 导出 | `docs/evidence/2516/R3-api-helpers-static.txt` | `createTenant/getTenant/updateTenant/deleteTenant` 命中 0、`getTenants` 命中 1；负例：任一写 helper 残留 → FAIL |
| R4 | 修后全树前端 `/system` 调用集 + `backend/main.go` 路由集 | 文本比对脚本 | `docs/evidence/2516/R4-api-contract-scan.txt` | `TENANT_WRITE_MISMATCH=0`、`RESIDUAL_OUTSIDE_ALLOWLIST=0`，残留集合打印且 ⊆ #2517 清单；负例：出现新的 `/system` 写调用不在 allowlist → FAIL |
| R5 | 修后 `api.md` §12.2、`ui.md` §5 | 文档正文 | `docs/evidence/2516/R5-docs-sync-static.txt` | api.md 含 `page_size`、`list`、`无 POST`；不含 `data.tenants`；ui.md 租户条目含「只读」；负例：仍写 `data.tenants` → FAIL |
| R6 | 修后干净树 | `cd frontend-pc` | `docs/evidence/2516/R6-build-lint.txt` | `BUILD_EXIT=0`、`ESLINT_NO_UNDEF=0`；负例：构建非 0 或 `no-undef` 计数 > 0 → FAIL |
| R7 | 修后树 + 最新 Accepted Plan + evidence.json | `pre-ready-check.sh 2516` | `docs/evidence/2516/pre-ready-2516.json` | `GATE_RESULT=PASS`、每条合同行 `pass=true`、`dirty=false`、P13 在 `origin/issue-2516`；负例：任一 P 失败或 evidence 缺行 → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2516.sh r1 > docs/evidence/2516/R1-api-result-unit.txt` | `docs/evidence/2516/R1-api-result-unit.txt` | exit 0；`grep -q '^# pass 1$'` 且 `grep -q 'FAIL 0'` | `node --test` 全通过（`# fail 0`），provenance 首行 `# build=1.0.* # commit=* # dirty=false` | 任一条断言反值（如 `40400` 判 true、`null` 抛错）→ 脚本非 0 退出 | dev |
| R2 | `bash scripts/verify-2516.sh r2 > docs/evidence/2516/R2-tenant-page-static.txt` | `docs/evidence/2516/R2-tenant-page-static.txt` | exit 0；`grep -q '^R2_OK$'` 且 `grep -q 'WRITE_ENTRY=0'` | 7 个标记全部为期望值（`WRITE_ENTRY=0`、`CREATE_MODAL=0`、`CODE_GUARD=1`、`FAIL_BRANCH_MSG=1`、`COLUMN_ID=1`、`COLUMN_STATUS_DYNAMIC=1`、`READONLY_HINT=1`） | 残留 `api.post('/system/tenants'`、保留 `Modal`、或 `dataIndex: 'tenant_id'` → 输出 `R2_FAIL` 且非 0 退出 | dev |
| R3 | `bash scripts/verify-2516.sh r3 > docs/evidence/2516/R3-api-helpers-static.txt` | `docs/evidence/2516/R3-api-helpers-static.txt` | exit 0；`grep -q '^R3_OK$'` 且 `grep -q 'TENANT_HELPERS=0'` | `TENANT_HELPERS=0`、`GET_TENANTS=1` | 任一 tenant 写 helper 残留 → `R3_FAIL` 且非 0 退出 | dev |
| R4 | `bash scripts/verify-2516.sh r4 > docs/evidence/2516/R4-api-contract-scan.txt` | `docs/evidence/2516/R4-api-contract-scan.txt` | exit 0；`grep -q '^R4_OK$'` 且 `grep -q 'RESIDUAL_OUTSIDE_ALLOWLIST=0'` | `TENANT_WRITE_MISMATCH=0`、`RESIDUAL_OUTSIDE_ALLOWLIST=0`，残留清单 ⊆ `#2517` 声明的 client 三写 | 出现新的 `/system` 写调用（非 allowlist）→ 输出该调用并非 0 退出 | dev |
| R5 | `bash scripts/verify-2516.sh r5 > docs/evidence/2516/R5-docs-sync-static.txt` | `docs/evidence/2516/R5-docs-sync-static.txt` | exit 0；`grep -q '^R5_OK$'` 且 `grep -q 'API_DOC_STALE=0'` | `API_DOC_STALE=0`、`API_DOC_NOWRITE=1`、`UI_DOC_READONLY=1` | api.md 仍含 `data.tenants` 或缺「无 POST」→ `R5_FAIL` 且非 0 退出 | dev |
| R6 | `bash scripts/verify-2516.sh r6 > docs/evidence/2516/R6-build-lint.txt` | `docs/evidence/2516/R6-build-lint.txt` | exit 0；`grep -q '^R6_OK$'` 且 `grep -q 'BUILD_EXIT=0'` | `BUILD_EXIT=0`、`ESLINT_NO_UNDEF=0` | 构建非 0 或 ESLint `no-undef` 计数 > 0 → `R6_FAIL` 且非 0 退出 | dev |
| R7 | `~/.config/opencode/scripts/pre-ready-check.sh 2516 --summary-file docs/evidence/2516/summary.md --changed-files docs/evidence/2516/changed-files.txt --evidence-file docs/evidence/2516/evidence.json --full-gate-cmd "cd frontend-pc && npm run build" --report docs/evidence/2516/pre-ready-2516.json` | `docs/evidence/2516/pre-ready-2516.json` | exit 0；`GATE_RESULT=PASS`，`jq -e '.result == "PASS"'`，evidence 每条合同行 `pass=true` | `GATE_RESULT=PASS`、`dirty=false`、P13 提交在 `origin/issue-2516` | 任一 P 项失败 / evidence.json 缺行 / `dirty=true` → gate 非 0，禁止 ready | dev |

### Files（变更文件清单）

| 类别 | 路径 |
|---|---|
| 页面修复 | `frontend-pc/src/pages/TenantManagement.jsx` |
| API 客户端 | `frontend-pc/src/services/api.js` |
| 共享终判（新增） | `frontend-pc/src/utils/apiResult.js` |
| 单测（新增） | `frontend-pc/tests/apiResult.test.mjs` |
| 证据生成器（新增） | `scripts/verify-2516.sh` |
| 核心文档 | `docs/spec/api/api.md` |
| 核心文档 | `docs/spec/ui/ui.md` |
| 证据（计划） | `docs/evidence/2516/plan.md` |
| 证据（摘要/清单） | `docs/evidence/2516/summary.md` |
| 证据（清单） | `docs/evidence/2516/changed-files.txt` |
| 证据（合同汇总） | `docs/evidence/2516/evidence.json` |
| 证据 R1 | `docs/evidence/2516/R1-api-result-unit.txt` |
| 证据 R2 | `docs/evidence/2516/R2-tenant-page-static.txt` |
| 证据 R3 | `docs/evidence/2516/R3-api-helpers-static.txt` |
| 证据 R4 | `docs/evidence/2516/R4-api-contract-scan.txt` |
| 证据 R5 | `docs/evidence/2516/R5-docs-sync-static.txt` |
| 证据 R6 | `docs/evidence/2516/R6-build-lint.txt` |
| Gate 报告 | `docs/evidence/2516/pre-ready-2516.json` |

## Artifact Provenance

- 生成器：`scripts/verify-2516.sh`（r1–r6），每个文本 artifact 首行必须为
  `# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`（锚定本单 fix commit，`FIX_REF` 传入；证据收尾提交不改变锚点）。
- 逐条清单（全部位于 `docs/evidence/2516/`，均须带上述 provenance 首行）：
  - `docs/evidence/2516/R1-api-result-unit.txt`（`node --test frontend-pc/tests/apiResult.test.mjs` 输出）
  - `docs/evidence/2516/R2-tenant-page-static.txt`（页面源码结构断言输出）
  - `docs/evidence/2516/R3-api-helpers-static.txt`（helper 清理断言输出）
  - `docs/evidence/2516/R4-api-contract-scan.txt`（`/system` 路径比对输出）
  - `docs/evidence/2516/R5-docs-sync-static.txt`（文档段落断言输出）
  - `docs/evidence/2516/R6-build-lint.txt`（`npm run build` + `npx eslint` 输出）
  - `docs/evidence/2516/pre-ready-2516.json`（gate 报告，JSON 自带 `.provenance.{build,commit,dirty}`）
- `docs/evidence/2516/evidence.json` 字段口径：`build` 形如 `1.0.<7-40 位短码>`，
  `commit` 为 40 位 sha，`dirty:false`；evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$
- 证据重生成纪律：fix commit 之后统一重跑 `scripts/verify-2516.sh r1` … `r6`，禁止只改 `build/commit` 字段冒充新证据。

*Model: DeepSeek V4.1 Flash 1017*

