## Work Summary — #2516 租户管理：删除无后端支撑的写入口 + 判 code 去假成功 + 列表字段对齐

### 修改内容

- `frontend-pc/src/pages/TenantManagement.jsx`
  - 删除「创建租户」按钮与创建 Modal/Form/Input/Select 及 handleCreateTenant / handleSubmit（后端从未注册 POST /system/tenants，提交恒 404 却提示「租户创建成功」）。
  - 读路径改为终判，失败分支清空列表：

    ```
    if (!isApiOk(res)) {
      message.error(apiFailureMessage(res, '加载租户列表失败'))
      setTenants([])
      setTotal(0)
      return
    }
    ```

    成功分支取 res.data.list / res.data.total；finally 统一 setLoading(false)。
  - 表头下新增只读说明 Alert：「租户随商户创建自动生成；新建租户请在『商户管理』完成」。
  - 列表列对齐真实响应：租户ID → dataIndex 改为 id；状态列按 record.status 渲染（active→启用 / inactive→停用 / 其它→未知）。
- `frontend-pc/src/services/api.js`
  - iamAdminApi 删除无路由、无引用的 tenant 写 helper（createTenant/getTenant/updateTenant/deleteTenant），保留 getTenants 只读 helper；client 侧 helper 归 #2517（D6）。
- `frontend-pc/src/utils/apiResult.js`（新增，D4 唯一 owner）
  - isApiOk(res)：仅 code 数值等于 20000 为真；null/undefined/非对象/非数字 code 一律 false 且不抛错。
  - apiFailureMessage(res, fallback)：后端非空字符串 message 优先，否则 fallback（不渲染 undefined）。
- `frontend-pc/tests/apiResult.test.mjs`（新增）：node --test 覆盖 20000/40400/50000/空对象/null/undefined/字符串 code/垃圾 code 与 message 取用规则。
- `scripts/verify-2516.sh`（新增）：R1–R6 证据 harness（provenance 首行由 git 派生；r6 构建经 heavy-run host 锁）。
- `docs/spec/api/api.md` §12.2（Core Update）：响应改为 list/total/page/page_size + 元素 id/name/status/description/created_at；显式声明「无 POST/PUT/DELETE /api/system/tenants」，租户由 POST /api/merchants 派生。
- `docs/spec/ui/ui.md` §5（Core Update）：租户管理条目补只读语义、字段对齐与终判要求。

Commit：f7bf8e416（fix，含 plan.md 快照）+ c168d20d7（证据提交）。

### 验证层级（Issue #70）

- 独立 Issue（无 Parent）：前端定向 node --test frontend-pc/tests/apiResult.test.mjs（1 pass / 0 fail）+ cd frontend-pc && npm run build（exit 0）+ ESLint（0 error，2 个既有 warning：no-use-before-define / exhaustive-deps，本页既有写法）+ R4 静态契约扫描。
- 无后端改动，未跑 go build/test；无 migration。

### Remote

- remote: origin/issue-2516; fix=f7bf8e416 pushed=yes; evidence=c168d20d7 pushed=yes

### 红绿实证

- 修前（develop 代码，可复现）：`frontend-pc/src/pages/TenantManagement.jsx` 第 44 行存在 api.post('/system/tenants', values)，而后端 `backend/main.go` 仅注册 GET /system/tenants → 提交恒 404，页面仍 message.success('租户创建成功')。对应 R2 断言在修前为 WRITE_ENTRY=1、CODE_GUARD=0（负例方向）。
- 修后：R2-site 静态断言 WRITE_ENTRY=0 CREATE_MODAL=0 CODE_GUARD=1 FAIL_BRANCH_MSG=1 COLUMN_ID=1 COLUMN_STATUS_DYNAMIC=1 READONLY_HINT=1；R3 TENANT_HELPERS=0 GET_TENANTS=1；R4 TENANT_WRITE_MISMATCH=0 BACKEND_SYSTEM_WRITE_ROUTES=0。
- 单测红绿方向由内容保证：若 isApiOk 对 code 40400 判 true、或对 null 抛错，R1 的 node --test 直接 fail（负例可执行）。

### Accepted Plan 对照（n/n，9/9）

- 变更点 1–2（产品口径：只读、不新增写端点）→ 已按 D1 落地（未新增任何写路由）。
- 变更点 3（utils/apiResult.js 终判共享件）→ 新增并作为唯一 owner（D4）。
- 变更点 4（页面：删创建入口、判 code、只读 Alert、列对齐）→ 完成。
- 变更点 5（api.js 删除 tenant 写 helper、保留 getTenants）→ 完成。
- 变更点 6（tests/apiResult.test.mjs）→ 完成。
- 变更点 7（scripts/verify-2516.sh r1–r6）→ 完成。
- 变更点 8（api.md §12.2 重写）→ 完成。
- 变更点 9（ui.md 只读语义补登）→ 完成。
- Core Update 两份文档均同批落地。

### 契约三向核对

- Plan/API：`docs/spec/api/api.md` §12.2 与后端注册表一致——`backend/main.go` 的 /system 只有 GET /system/clients 与 GET /system/tenants 两条 GET（R4 断言 BACKEND_SYSTEM_GET_ROUTES=2、BACKEND_SYSTEM_WRITE_ROUTES=0）；文档显式声明无写端点，租户由 POST /api/merchants 派生。
- docs：`backend/handlers/system.go` 的实际响应为 code/data{list,total,page,page_size}，`backend/models/models.go` 的 Tenant 字段为 id/name/status/description/created_at/updated_at，api.md 样例与之一致；ui.md 租户条目同步为只读列表。
- handler/frontend：页面只调用 GET /system/tenants（api.get），参数 page 与 page_size 与后端 c.DefaultQuery 口径一致；失败分支读 message 并清空列表（禁止静默空表/假成功）。

### Evidence Contract（7/7 PASS）

- R1 → PASS → `docs/evidence/2516/R1-api-result-unit.txt`；实际 node --test 输出 pass 1 / fail 0、RESULT: PASS 1 FAIL 0。
- R2 → PASS → `docs/evidence/2516/R2-tenant-page-static.txt`；实际 WRITE_ENTRY=0 CREATE_MODAL=0 CODE_GUARD=1 FAIL_BRANCH_MSG=1 COLUMN_ID=1 COLUMN_STATUS_DYNAMIC=1 READONLY_HINT=1 且 R2_OK。
- R3 → PASS → `docs/evidence/2516/R3-api-helpers-static.txt`；实际 TENANT_HELPERS=0 GET_TENANTS=1 TENANT_HELPER_REFS=0 且 R3_OK。
- R4 → PASS → `docs/evidence/2516/R4-api-contract-scan.txt`；实际 TENANT_WRITE_MISMATCH=0 RESIDUAL_OUTSIDE_ALLOWLIST=0（残留 = ClientManagement.jsx 3 处 client 写，归 #2517）BACKEND_SYSTEM_WRITE_ROUTES=0。
- R5 → PASS → `docs/evidence/2516/R5-docs-sync-static.txt`；实际 API_DOC_STALE=0 API_DOC_NOWRITE=1 API_DOC_FIELDS=1 UI_DOC_READONLY=1。
- R6 → PASS → `docs/evidence/2516/R6-build-lint.txt`；实际 BUILD_EXIT=0 ESLINT_RC=0 ESLINT_NO_UNDEF=0。
- R7 → PASS → `docs/evidence/2516/pre-ready-2516.json`；实际 gate overall=PASS、plan_sha256 与 Accepted Plan 快照一致、provenance.build=1.0.f7bf8e416、provenance.dirty=false。
- 说明：gate 报告的机器可判定结果字段是 overall（gate 报告 schema；计划文字写作 result），等价断言为 `jq -e '.overall == "PASS"'`，已在下方 Pre-ready gate 段给出实际值。
- 账本：`docs/evidence/2516/evidence.json`（schema v1，build=1.0.f7bf8e416，dirty=false）。

### G0 Golden Acceptance

- R1：`docs/evidence/2516/R1-api-result-unit.txt` → 断言 pass 计数为 1、fail 计数为 0、RESULT: PASS 1 FAIL 0（fixture 覆盖 20000 / 40400 / 50000 / 空对象 / null / undefined / 字符串 code / 垃圾 code 与 message 空/非字符串）。
- R2：`docs/evidence/2516/R2-tenant-page-static.txt` → 7 个标记 + R2_OK；负例（残留写入口 / Modal / tenant_id 列）会打印 R2_FAIL 并非 0 退出。
- R3：`docs/evidence/2516/R3-api-helpers-static.txt` → TENANT_HELPERS=0、GET_TENANTS=1、R3_OK。
- R4：`docs/evidence/2516/R4-api-contract-scan.txt` → TENANT_WRITE_MISMATCH=0、RESIDUAL_OUTSIDE_ALLOWLIST=0、残留集合 ⊆ #2517 allowlist、backend 无 /system 写路由。
- R5：`docs/evidence/2516/R5-docs-sync-static.txt` → API_DOC_STALE=0、API_DOC_NOWRITE=1、API_DOC_FIELDS=1、UI_DOC_READONLY=1。
- R6：`docs/evidence/2516/R6-build-lint.txt` → BUILD_EXIT=0、ESLINT_NO_UNDEF=0、R6_OK。
- R7：`docs/evidence/2516/pre-ready-2516.json` → overall=PASS、provenance.dirty=false、provenance.commit=f7bf8e416。

### Verification Matrix n/n

| 变更点 | 文件/组件 | 状态 | 证据 |
|---|---|---|---|
| R1 终判纯函数（20000 放行、404 判失败） | frontend-pc/src/utils/apiResult.js、frontend-pc/tests/apiResult.test.mjs | PASS | docs/evidence/2516/R1-api-result-unit.txt |
| R2 写入口移除 + 判 code + 列表字段对齐（页面结构） | frontend-pc/src/pages/TenantManagement.jsx | PASS | docs/evidence/2516/R2-tenant-page-static.txt |
| R3 死 helper 清理 | frontend-pc/src/services/api.js | PASS | docs/evidence/2516/R3-api-helpers-static.txt |
| R4 同类扫描（/system 前缀前后端路径比对） | scripts/verify-2516.sh | PASS | docs/evidence/2516/R4-api-contract-scan.txt |
| R5 文档同步 | docs/spec/api/api.md、docs/spec/ui/ui.md | PASS | docs/evidence/2516/R5-docs-sync-static.txt |
| R6 全量构建 + ESLint | frontend-pc | PASS | docs/evidence/2516/R6-build-lint.txt |
| R7 pre-ready gate（四源 + 全量构建） | docs/evidence/2516/pre-ready-2516.json | PASS | docs/evidence/2516/pre-ready-2516.json |

### Dependency Order / 验收规则

- 无硬依赖；#2482（mergeSiteMemberRoles）与本单无关（D2 菜单入口归 #2518；client 写入口归 #2517）。
- 本单未修改任何验收工具/规则（未改 scripts/pre-ready-check.sh、scripts/evidence-artifacts.sh、scripts/plan-lint.sh 等）；`scripts/verify-2516.sh` 为本单证据 harness。
- D5 Owner 豁免：PC 无组件测试运行器，R2 采用静态结构断言（同 #2480 D6 / #2469 D3 惯例）；未新增前端测试基建。

### 同族扫描（同类路径对称扫描义务）

- 前端 /system 写调用扫描（frontend-pc/src/pages）：tenant 侧 0（本单已清）；client 侧 3 处（ClientManagement.jsx 第 55/67/70 行）→ 归属 #2517，R4 以 allowlist 断言「残留 ⊆ 声明清单」。
- iamAdminApi 全仓引用扫描：删除的 tenant 写 helper 无任何消费方（TENANT_HELPER_REFS=0）；保留的 getTenants 恰好 1 处定义。
- 后端路由扫描：/system 仅 2 条 GET，无写路由（本单未新增，符合 D1）。

### 人工/部署侧

- 菜单入口/页面去留（D2）与独立建租户能力（D1）均不在本单，分别由 #2518 / Owner 决策承载；本单未做生产环境核验。

### Pre-ready gate

- RESULT: PASS — `docs/evidence/2516/pre-ready-2516.json`（overall=PASS，timestamp=20261125-152502）；P12 Evidence Contract 7 行完整、P13 fix 与证据在 origin/issue-2516、P5 HEAD 与 baseline 干净 worktree 构建均 exit 0、provenance.dirty=false。
- 实际调用在计划命令基础上补了 3 个运行期参数：--fix-commit f7bf8e416（锚定 fix commit）、--no-gate-cache（宿主缓存里存着依赖缺失时 HEAD rc=127 的记录，必须绕开才能反映当前树）、--affected-cmd（跑定向单测，清 P4 WARN）。
- 依赖可见性：P5 在 worktree 下新建干净 worktree 构建，而 npm 依赖不在版本库里；本轮在工作树 tmp 目录建了指向 frontend-pc/node_modules 的链接（tmp 已 gitignore，工作树仍 clean），让干净 worktree 经祖先目录解析到已安装依赖。只有依赖被共享，被构建的源码仍是该干净 worktree 的 HEAD。
- 说明：首次运行暴露两处证据形态问题并已修正 —— (1) Summary 用反引号包裹非路径代码片段，被 P3 的 evidence-token 扫描判为缺失路径，已改为围栏代码块；(2) R7 合同把 gate 报告自身作为产物，报告必须先入库才能被 P12/P13 校验，故先提交 bootstrap 报告再跑最终 gate。

Model: DeepSeek V4.1 Flash 1017
