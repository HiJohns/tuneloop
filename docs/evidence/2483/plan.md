## Accepted Plan — #2483（双归属员工编辑页不再锁死：`site` 优先于 `direct`，并暴露双归属提示）

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| 后端 `is_direct_staff = merchant_members 存在 **或** 无网点`，双归属时 **两个 flag 同时为 true**（#2429 设计） | `backend/handlers/user_staff.go:1022`（`isDirectStaff := mm.MerchantID != "" \|\| !isSiteStaff`）、`:1041-1046` |
| 前端模式判定把 `is_direct_staff` 放在 `site` 之前 → 双归属解析为 `direct` | `frontend-pc/src/utils/staffForm.js:4-10` |
| `direct` 模式：`showSite=false`、`showMerchantReadonly=true` → 网点下拉不渲染、只读「直属商户」，**无法调动网点** | `frontend-pc/src/utils/staffForm.js:13-20`、`frontend-pc/src/pages/StaffEdit.jsx:184-195` |
| 既有断言把「direct wins」钉死（#2429 R6） | `frontend-pc/src/utils/staffForm.test.js:28-32`（`dual identity resolves to direct (direct wins)`） |
| 角色下拉为空源于 `users.role=''` 存量 + `resolveInitialRole` 只取 `user.role` | `frontend-pc/src/pages/StaffEdit.jsx:34-39`、`:90`；存量见 #2462 |
| 直属员工管理页（可移除直属登记）路由存在 | `frontend-pc/src/App.jsx:726`（`/staff/direct`） |

**结论**：字段模式优先级错误 —— 「有 `site_members` 行」这一事实应优先于「有 `merchant_members` 行」。修复 = 翻转优先级 + 双归属显式提示与指引入口 + 角色下拉回退链；**后端不改**（`is_direct_staff` 语义保持 #2429，双 true 依然成立）。

### 变更点

1. `frontend-pc/src/utils/staffForm.js`（唯一判定出口）：
   ```
   technician → is_site_staff === true || site_id 存在 → site → direct
   ```
   - `is_direct_staff` **不再**抢占 `site`；保留给 `showMerchantReadonly` 的 `direct` 分支；
   - 新增导出 `isDualIdentity(user)` → `user.is_direct_staff === true && (user.is_site_staff === true || Boolean(user.site_id))`（供提示行使用，纯函数可测）。
2. `frontend-pc/src/pages/StaffEdit.jsx`：
   - 双归属（`isDualIdentity`）时在「归属网点」上方渲染**持久提示**：`Alert type="warning"` 文案「该员工同时登记为商户直属员工；此处修改归属网点不会移除直属登记，如需退出直属域请到「直属员工管理」移除」，并带跳转按钮 `navigate('/staff/direct')`；
   - `resolveInitialRole(user)` 回退链改为 `user.role || user.site_role || user.merchant_role || ''`（解决角色下拉为空）；
   - **不改** `site_id` 未变化时不回传的逻辑（#2429 已修，避免误触发 IAM 重绑）。
3. `frontend-pc/src/utils/staffForm.test.js`：**翻转既有断言**（#2429 R6 / 原第 28-32 行）
   - 旧：`dual identity resolves to direct (direct wins)` → 期望 `mode==='direct'`、`showSite===false`
   - 新：`dual identity resolves to site (site wins, #2483)` → 期望 `mode==='site'`、`showSite===true`、`requireSite===true`
   - 理由：双归属用户在列表已显示「所属网点」，编辑页锁死为直属实为**不可操作**（#2483 现象）；Owner 期望 1 明确要求「存在活跃 `site_members` 行 → 判为 site」。
   - 另新增：`technician + site` → `technician`（保持 #2429 优先级）；`isDualIdentity` 四态用例。
4. 不改后端（`is_direct_staff`/`is_site_staff` 语义与 `UpdateUser` 网点迁移逻辑均保持原样）。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 模式优先级纯函数 | `frontend-pc/src/utils/staffForm.js` | `node --test frontend-pc/src/utils/staffForm.test.js` | 双归属 / 纯网点 / 纯直属 / 维修师+网点 | PC | 双归属 → `site`；纯网点 → `site`；纯直属 → `direct`；维修师+网点 → `technician` | R1 |
| R2 红绿（旧实现必红） | `frontend-pc/src/utils/staffForm.js`、`frontend-pc/src/utils/staffForm.test.js` | 同上（旧实现 vs 新实现） | 双归属 fixture | PC | 旧实现：`dual identity … site` 断言 FAIL；新实现 PASS → `RED_GREEN=1` | R2 |
| R3 编辑页字段可见性 | `frontend-pc/src/pages/StaffEdit.jsx` | `/staff/:id/edit`（编辑用户） | 双归属用户（`is_direct_staff=true` + `is_site_staff=true`） | PC | 渲染「归属网点」`Select`（可改/可清空），不渲染只读「直属商户」；`SITE_SELECTABLE=1` | R3 |
| R4 角色下拉回退链 | `frontend-pc/src/pages/StaffEdit.jsx` | 同上 | `users.role=''`、`site_role='site_member'` | PC | 角色下拉预填 `site_member`（非空）；`ROLE_FALLBACK=1` | R4 |
| R5 双归属显式提示与指引入口 | `frontend-pc/src/pages/StaffEdit.jsx`、`frontend-pc/src/utils/staffForm.js` | 同上 | 双归属 | PC | 出现 `Alert type="warning"` 提示 + 跳转 `/staff/direct` 按钮；`DUAL_HINT=1`、`GUIDE_LINK=1` | R5 |
| R6 既有断言翻转登记 | `frontend-pc/src/utils/staffForm.test.js` | `bash scripts/verify-2483.sh r6` | 修后树 | PC | `FLIPPED_ASSERT=1`（`dual identity … site` 存在、`direct wins` 已不存在）、`BACKEND_UNCHANGED=1`（`backend/` 无 diff） | R6 |
| R7 构建与 lint | `frontend-pc` | `cd frontend-pc && npm run build` + `npx eslint` 改动文件 | 修后树 | PC | `PC_BUILD=0`、`LINT_EXIT=0` | R7 |
| R8 gate 四源 + 全量 PC 构建 | `docs/evidence/2483/pre-ready-2483.json`、`docs/evidence/2483/plan.md` | `--full-gate-cmd "cd frontend-pc && npm run build"` + `plan-source.sh --issue 2483` | 修后树（干净） | PC | `GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS` | R8 |

## Input Truth & Tool Preflight

- **Source of truth**：后端 `GetStaffUser` 返回的 `is_direct_staff` / `is_site_staff` / `site_id` / `site_memberships`（`user_staff.go:1022-1047`）——**双归属两 flag 均 true** 是既有事实，本单只改前端如何消费。
- **差异核对**：`grep -rn "is_direct_staff\|is_site_staff" frontend-pc/src` → 仅 `staffForm.js` 与 `StaffEdit.jsx` 消费；`resolveStaffFormMode` 是唯一判定出口（#2429 D? 单一出口）。
- **损坏输入拒绝**：`user` 为 `null`/`{}` → `direct`（保持既有兜底）；`site_id=''` 且 `is_site_staff=false` → `direct`；`is_site_staff=true` 但 `site_id=''`（多网点主行缺失）→ 仍判 `site`（由 `site_memberships` 断言，不猜）。
- **工具盲区**：PC 无组件测试运行器 → 纯函数用 `node --test`（`frontend-pc/src/utils/staffForm.test.js` 已存在，Node 直跑 `.js` 需 `--experimental-...`？实测该文件为 ESM，PC `package.json` 有 `"type":"module"` → 可直接 `node --test`）；UI 接线用静态断言；真实截图由 `[Ops-Followup of #2483]` 承载。
- **影响面预检**：`/staff/direct`（直属员工管理）与 `DirectMemberManagement` 不消费 `resolveStaffFormMode`（独立页面）→ 不受影响；`StaffManagement` 创建表单复用 `resolveStaffFormMode`？`grep -n "resolveStaffFormMode" frontend-pc/src` → 仅 `StaffEdit.jsx` 使用 → 影响面收敛。
- **语义变更登记（强制）**：本单**翻转** #2429 R6 的既有断言 `dual identity resolves to direct (direct wins)` → `site wins`；已在 `## Dependencies` 说明并通知 #2429。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 优先级：`technician` → `site`（有 `site_members` 或 `site_id`）→ `direct` | Owner / Architect | 本计划锁定（Owner 期望 1） | 若 Owner 要求「直属优先」→ 回 Architect |
| D2 | 后端不变：`is_direct_staff`/`is_site_staff` 双 true 语义保持 #2429；不改 `UpdateUser` 网点迁移 | Architect | 本计划锁定 | 阻塞实现 |
| D3 | 双归属的「退出直属域」不在本单提供一键 API；只提供**持久提示 + 跳转直属员工管理**入口 | Architect | 本计划锁定 | 若要求一键退出（删 `merchant_members`）→ 另立 Issue（涉及 IAM 解绑） |
| D4 | 角色下拉回退链 `users.role || site_role || merchant_role`（前端只读回填，不改后端存量） | Architect | 本计划锁定 | 阻塞实现 |
| D5 | UI 行证据 = 静态接线断言 + 纯函数单测；真实渲染由 Ops 截图回评（**Owner 豁免**，`evidence_type: static`） | Architect | owner_approved（沿用本仓前端豁免惯例） | 若审计要求 render → 另立前端测试基建 Issue |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R3 | static | 编辑页字段可见性接线结构断言，Owner 豁免见 D5 |
| R4 | static | 角色回退链结构断言，Owner 豁免见 D5 |
| R5 | static | 双归属提示与入口结构断言，Owner 豁免见 D5 |
| R6 | static | 断言翻转与后端无 diff 度量，Owner 豁免见 D5 |

## Out of Scope

- 一键「退出直属域」（删除 `merchant_members` + IAM 解绑）→ 若需要另立 Issue（D3）。
- 存量 `users.role=''` 数据回填（#2462 领域，本单只做前端回退显示）。
- 人员管理列表「角色」列口径（#2462 既有实现）。
- 维修师与员工身份合并（#2465）。
- 移动端页面（本单仅 PC）。
- 后端任何改动。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | `{is_direct_staff:true,is_site_staff:true,site_id:'s1'}`、`{is_site_staff:true,site_id:'s1'}`、`{is_direct_staff:true}`、`{role:'repair_technician',is_site_staff:true}` | `node --test` | `docs/evidence/2483/R1-priority.txt` | `DUAL=site`、`SITE_ONLY=site`、`DIRECT_ONLY=direct`、`TECH=technician`、`DUAL_SHOW_SITE=1` | 双归属 → direct → FAIL |
| R2 | 同 R1 双归属 fixture × 旧/新实现 | 两个版本 | `docs/evidence/2483/R2-red-green.txt` | `OLD_FAIL=1`、`NEW_PASS=1`、`RED_GREEN=1` | 旧实现也 PASS（断言抓不到）→ FAIL |
| R3 | 修后 `StaffEdit.jsx`（`rules.showSite` / `showMerchantReadonly` 渲染分支） | 静态断言 | `docs/evidence/2483/R3-fields.txt` | `SITE_SELECTABLE=1`、`MERCHANT_READONLY_HIDDEN_ON_DUAL=1` | 双归属仍只读直属 → FAIL |
| R4 | `resolveInitialRole({role:'', site_role:'site_member'})`、`({role:'', site_role:'', merchant_role:'merchant_staff'})` | 纯函数 + 静态断言 | `docs/evidence/2483/R4-role-fallback.txt` | `ROLE_FALLBACK=site_member`、`ROLE_FALLBACK2=merchant_staff` | 仍为空 → FAIL |
| R5 | 修后 `StaffEdit.jsx` + `staffForm.js`（`isDualIdentity`） | 静态断言 | `docs/evidence/2483/R5-dual-hint.txt` | `DUAL_HINT=1`、`GUIDE_LINK=/staff/direct` | 无提示/无入口 → FAIL |
| R6 | 修后 `staffForm.test.js` + `git diff --stat backend/` | `bash scripts/verify-2483.sh r6` | `docs/evidence/2483/R6-flip.txt` | `FLIPPED_ASSERT=1`、`DIRECT_WINS_GONE=1`、`BACKEND_UNCHANGED=1` | 旧断言仍在或 backend 有 diff → FAIL |
| R7 | 修后 `frontend-pc` | `npm run build` + `npx eslint` | `docs/evidence/2483/R7-build-lint.txt` | `PC_BUILD=0`、`LINT_EXIT=0` | 任一非 0 → FAIL |
| R8 | 修后干净树 + gate | 读校验 | `docs/evidence/2483/R8-gate.txt` | `GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS` | `P5=NA` → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2483.sh r1` | `docs/evidence/2483/R1-priority.txt` | exit 0；grep `DUAL=site`、`SITE_ONLY=site`、`TECH=technician` | PASS | `DUAL=direct` → FAIL | dev |
| R2 | `bash scripts/verify-2483.sh r2` | `docs/evidence/2483/R2-red-green.txt` | exit 0；grep `OLD_FAIL=1`、`RED_GREEN=1` | PASS | 旧版 PASS → FAIL | dev |
| R3 | `bash scripts/verify-2483.sh r3` | `docs/evidence/2483/R3-fields.txt` | exit 0；grep `SITE_SELECTABLE=1`、`MERCHANT_READONLY_HIDDEN_ON_DUAL=1` | PASS | 双归属只读 → FAIL | dev |
| R4 | `bash scripts/verify-2483.sh r4` | `docs/evidence/2483/R4-role-fallback.txt` | exit 0；grep `ROLE_FALLBACK=site_member`、`ROLE_FALLBACK2=merchant_staff` | PASS | 空角色 → FAIL | dev |
| R5 | `bash scripts/verify-2483.sh r5` | `docs/evidence/2483/R5-dual-hint.txt` | exit 0；grep `DUAL_HINT=1`、`GUIDE_LINK=/staff/direct` | PASS | 无提示 → FAIL | dev |
| R6 | `bash scripts/verify-2483.sh r6` | `docs/evidence/2483/R6-flip.txt` | exit 0；grep `FLIPPED_ASSERT=1`、`BACKEND_UNCHANGED=1` | PASS | 旧断言残留 → FAIL | dev |
| R7 | `bash scripts/verify-2483.sh r7` | `docs/evidence/2483/R7-build-lint.txt` | exit 0；grep `PC_BUILD=0`、`LINT_EXIT=0` | PASS | 任一非 0 → FAIL | dev |
| R8 | `bash scripts/verify-2483.sh r8` | `docs/evidence/2483/R8-gate.txt` | exit 0；grep `GATE_RESULT=PASS`、`PLAN_SOURCE=PASS` | PASS | `P5=NA` → FAIL | dev |

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2483/R1-priority.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2483/R2-red-green.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2483/R3-fields.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2483/R4-role-fallback.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2483/R5-dual-hint.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2483/R6-flip.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2483/R7-build-lint.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2483/R8-gate.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| # | 缺陷面 | 触发条件 | 根因 | 修复点 | 覆盖行 | 负例断言 |
|---|---|---|---|---|---|---|
| B1 | 双归属编辑页锁死无法调网点 | `merchant_members` + `site_members` 并存 | 模式优先级 `direct` 抢在 `site` 前 | 变更点 1 | R1/R3 | `DUAL=direct` → R1 失败 |
| B2 | 列表说在网点、编辑页说直属（口径矛盾） | 双归属 | 同上 | 变更点 1 | R1/R3 | `MERCHANT_READONLY_HIDDEN_ON_DUAL=0` → R3 失败 |
| B3 | 角色下拉为空 | `users.role=''` 存量 | 只取 `user.role` | 变更点 2 | R4 | 空角色 → R4 失败 |
| B4 | 用户不知「双归属」状态 | 双归属 | 无提示 | 变更点 2 | R5 | `DUAL_HINT=0` → R5 失败 |
| B5 | 不知道如何退出直属域 | 双归属 | 无入口 | 变更点 2 | R5 | `GUIDE_LINK` 缺失 → R5 失败 |
| B6 | 旧断言把错误语义钉死 | 改优先级 | #2429 R6 断言未翻转 | 变更点 3 | R6 | `DIRECT_WINS_GONE=0` → R6 失败 |
| B7 | 顺手改后端造成双写不一致 | 改判定时 | 无边界声明 | D2 | R6 | `BACKEND_UNCHANGED=0` → R6 失败 |
| B8 | 维修师身份被 `site` 抢走 | 维修师+网点 | 优先级顺序错误 | 变更点 1 | R1 | `TECH=site` → R1 失败 |
| B9 | 维护新语义后 `#2429` 与本单冲突 | 两单同文件 | 无依赖登记 | `## Dependencies` 说明 | R6 | #2429 未记录翻转 → R6 失败 |

### Workload

- Size: S
- Basis: 1 个纯函数文件（优先级 + `isDualIdentity`）+ 1 个页面接线（提示/回退链）+ 1 个测试文件（翻转 + 新增）+ 1 个 verify 脚本 + 8 条证据行；无后端、无 migration
- Confidence: high

### Files（变更文件清单）

- `frontend-pc/src/utils/staffForm.js`
- `frontend-pc/src/utils/staffForm.test.js`
- `frontend-pc/src/pages/StaffEdit.jsx`
- `scripts/verify-2483.sh`
- `docs/evidence/2483/plan.md`
- `docs/evidence/2483/pre-ready-2483.json`
- `docs/evidence/2483/R1-priority.txt`
- `docs/evidence/2483/R2-red-green.txt`
- `docs/evidence/2483/R3-fields.txt`
- `docs/evidence/2483/R4-role-fallback.txt`
- `docs/evidence/2483/R5-dual-hint.txt`
- `docs/evidence/2483/R6-flip.txt`
- `docs/evidence/2483/R7-build-lint.txt`
- `docs/evidence/2483/R8-gate.txt`
- `docs/evidence/2483/evidence.json`

## Dependencies

- 无

### 依赖与关联说明（非硬依赖）

- **#2429（accepted，已在 develop：fix `5d10dc2ef`）**：本单**翻转**其 R6 的语义「双归属 → direct 优先」为「双归属 → site 优先」。生效时机：本单合入后 #2429 的 R6 描述以本单为准（已在本单 `## Bug Reflection Matrix` B6/B9 与 R6 证据登记；实现时同步翻转 `staffForm.test.js` 断言）。
- **#2472 / #2481（Ops）**：本单是 #2472「site_member 网点间调动」预生产验证的前置（王五/南哥/挺好听 均为双归属）。
- **#2462 / #2465**：人员身份收敛；本单不改身份语义，只改编辑页判定与提示。
- **#2482**：`site_members.roles` 压缩问题（不同层，不互相阻塞）。
- **部署侧**：预生产「双归属员工编辑页可切换网点」截图回评由 `[Ops-Followup of #2483]` 承载。

*Model: DeepSeek V4.1 Flash 1017*

