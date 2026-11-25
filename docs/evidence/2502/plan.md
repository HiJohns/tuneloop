## Accepted Plan — #2502 v2（移动端「我的」身份徽标 + R9/R10/R11/R13/R12 自持证据；H5 同口径）

> **一句话**：`/users/me` **已经**同时返回 `tenant_name` 与 `site_name`（#2117 / #2449），但 weapp 徽标写成 `site_name || tenant_name`（网点名优先 → 商户名被完全丢弃），H5 头部则**没有任何身份徽标**；本单把两端统一为 `商户名 · 网点名 · 职位`，用**单一 helper** 收口，不改后端、不碰 AccountSelect/PC（#2455 所有权）。

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| weapp 徽标网点优先：`[[user?.site_name \|\| user?.tenant_name], roleBadgeText]` → 两者都在时商户名被丢弃 | `frontend-mobile/src/pages-weapp/Profile.jsx:426` |
| H5 头部只有姓名（+ 顾客手机号），**无任何角色/组织徽标** | `frontend-mobile/src/pages/Profile.jsx:315-345` |
| `/users/me` 已返回 `tenant_name`（JWT `tid` → `merchants.name`，#2117） | `backend/handlers/user_staff.go:1427-1438` |
| `/users/me` 已返回 `site_id`/`site_name`（JWT `oid` → `sites.name`，#2449；顾客上下文不返回） | `backend/handlers/user_staff.go:1394-1425` |
| 回退分支（本地无 `users` 行）只给 `site_name`，无 `tenant_name` → 徽标必须能回退单项 | `backend/handlers/user_staff.go:1291-1310` |
| 展示口径权威：员工上下文 → `{商户名} · {网点名}` | `docs/spec/ui/ui.md:1522`（§2.24） |
| #2455（**未合入 develop**，`origin/issue-2455` @ cbb11a0a4）已在 PC `identityLabel` 与服务端 `contexts[].label` 落同一口径；本单只补移动端 Profile，不碰其所有权文件 | `git show origin/issue-2455:frontend-pc/src/pages/UserProfile.jsx` |
| H5/weapp `Profile` 是双实现（C3 已登记漂移），本 Bug 的成因就是两端各自拼口径 | `docs/cases/h5-weapp-differences.md:70` |
| 后端既有用例已锁定字段语义（顾客上下文不得返回 `tenant_name`） | `backend/handlers/user_me_tenant_name_2117_test.go:61-90` |

### 变更点

1. **新增单一 owner** `frontend-mobile/src/utils/identity.js`（纯函数、无 IO）：

   ```js
   export const ROLE_BADGE_LABELS = [
     ['site_admin', '管理员'],
     ['site_member', '员工'],
     ['repair_technician', '维修师傅'],
   ]
   export function formatOrgIdentity(merchantName, siteName) {
     return [merchantName, siteName].filter(Boolean).join(' · ')
   }
   export function roleBadgeTextFromRoles(roles = [], isStaff = false) {
     if (!isStaff) return '顾客'
     const labels = ROLE_BADGE_LABELS.filter(([code]) => roles.includes(code)).map(([, label]) => label)
     return labels.length > 0 ? labels.join(' · ') : '员工'
   }
   export function identityBadgeText({ tenantName, siteName, roles = [], isStaff = false } = {}) {
     return [formatOrgIdentity(tenantName, siteName), roleBadgeTextFromRoles(roles, isStaff)].filter(Boolean).join(' · ')
   }
   ```

   两个页面只做拼装，**禁止**各自再写 `filter(Boolean).join(...)` 口径。

2. **weapp** `frontend-mobile/src/pages-weapp/Profile.jsx`：
   - `roleBadges`/`roleBadgeText`（219-223 行）改由 `roleBadgeTextFromRoles(myRoles, isStaff)` 产生（**行为不变**：管理员/员工/维修师傅以 ` · ` 连接，无角色回退「员工」，非员工为「顾客」）；
   - 426 行徽标文本改为 `identityBadgeText({ tenantName: user?.tenant_name, siteName: user?.site_name, roles: myRoles, isStaff })`；
   - 渲染条件 `(isStaff || hasMultipleContexts)`、内联样式（11px 药丸）保持不变。

3. **H5** `frontend-mobile/src/pages/Profile.jsx`：员工上下文在姓名行**之后、会员等级行之前**新增一行徽标，文案与 weapp 完全一致：

   ```jsx
   {isStaff && (
     <Text className="text-xs text-zinc-400 mt-1">
       {identityBadgeText({ tenantName: user?.tenant_name, siteName: user?.site_name, roles: myRoles, isStaff })}
     </Text>
   )}
   ```

   使用既有 `myRoles`（§#2089 已取 `/site-members/me`）；`className` 只用安全类（`text-xs`/`text-zinc-400`/`mt-1`），不得引入 #1831 禁区（任意值/变体/透明度/分数）。

4. **新增测试** `frontend-mobile/scripts/page-smoke/profile-identity-badge-2502.test.jsx`（`vitest` + `@testing-library/react` 的 `render`/`cleanup`，jsdom；用 `process.stdout.write` 输出 `KEY=value` 机器行，因为 `scripts/page-smoke/setup.js` 已把 `console.log` 打桩）：
   - `TestProfileBadgeWeapp_2502`：mock `/users/me` = `{tenant_name:'cadenza', site_name:'卡丹萨乐器'}`、`/site-members/me` = `{roles:['site_admin'], sites:[]}` → 断言页面文本**精确包含** `cadenza · 卡丹萨乐器 · 管理员`；
   - `TestProfileBadgeWeappFallback_2502`：`/users/me` = `{site_name:'卡丹萨乐器'}` → 仅 `卡丹萨乐器 · 管理员`；`/users/me` = `{}` → 仅 `管理员`；两者均断言不含 `undefined`/`null`/前导 ` · `；
   - `TestProfileBadgeH5_2502`：同 R1 fixture 渲染 `src/pages/Profile.jsx`（`MemoryRouter` 包裹）→ 与 weapp 完全相同的文案；
   - `TestProfileBadgeCustomer_2502`：token `role=USER` + `/users/me` 含 `tenant_name`/`site_name` → 两端都**不渲染**商户名/网点名；
   - `TestIdentityHelperRedGreen_2502`：`formatOrgIdentity` 空串/`null`/`undefined`/单段，`roleBadgeTextFromRoles` 多角色/回退，并**同一 fixture** 对照旧表达式 `[[site||tenant]]` 与新版 `identityBadgeText(...)`，证明旧文本丢商户名（红）而新文本保留（绿）。

5. **新增证据生成器** `scripts/verify-2502.sh`：子命令 `r1..r7`，node22 解析沿用 `scripts/verify-2354.sh:67` 的 `ls -d "$HOME"/.nvm/versions/node/v22*/bin`；每个产物首行写 `# build=1.0.<short> # commit=<sha> # dirty=false`（口径同 `scripts/verify-2354.sh` 的 `PROVENANCE`）。R8 由 canonical `pre-ready-check.sh` 产出 `docs/evidence/2502/pre-ready-2502.json`。

6. **文档同步**：见 `## Core Update`。

## Dependencies

- 无

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 weapp 徽标三段 | `frontend-mobile/src/pages-weapp/Profile.jsx` | `/pages-weapp/profile`（`pages-weapp/profile/index` 薄壳 + `PageShell`） | JWT `role=STAFF`；`/users/me` = `{tenant_name:'cadenza', site_name:'卡丹萨乐器'}`；`/site-members/me` roles `['site_admin']` | weapp | render 后页面文本精确包含 `cadenza · 卡丹萨乐器 · 管理员` | R1 |
| R2 缺 `tenant_name` 回退单项 | `frontend-mobile/src/pages-weapp/Profile.jsx` | 同上 | `/users/me` = `{site_name:'卡丹萨乐器'}`；以及 `/users/me` = `{}` | weapp | render 后文本为 `卡丹萨乐器 · 管理员` / 仅 `管理员`；不含 `undefined`、`null`、前导 ` · ` | R2 |
| R3 H5 与 weapp 同口径 | `frontend-mobile/src/pages/Profile.jsx` | `/profile` | 同 R1 fixture（`MemoryRouter` + `render`） | H5 | render 后文本与 R1 **逐字相同**（`PARITY=1`） | R3 |
| R4 顾客上下文不泄漏组织名 | `frontend-mobile/src/pages/Profile.jsx`、`frontend-mobile/src/pages-weapp/Profile.jsx` | `/profile`、`/pages-weapp/profile` | JWT `role=USER`，`/users/me` 故意带 `tenant_name`/`site_name` | H5 + weapp | 两端都不出现商户名/网点名（render 负例） | R4 |
| R5 纯函数语义 + 红绿对照 | `frontend-mobile/src/utils/identity.js` | 单元级（无路由） | `''`/`null`/`undefined`/单段/多角色 | 双端共用 | `formatOrgIdentity('a','b')='a · b'`、`formatOrgIdentity('','b')='b'`、`formatOrgIdentity(null,null)=''`；旧表达式文本 `卡丹萨乐器`（红） vs 新文本 `cadenza · 卡丹萨乐器`（绿） | R5 |
| R6 静态检查 | `frontend-mobile/src/utils/identity.js`、两个 Profile 组件 | `npx eslint` | 修后树 | 双端 | `ESLINT_EXIT=0`、`no-undef` 计数 0 | R6 |
| R7 规格/接口/差异文档 | `docs/spec/ui/ui.md`、`docs/spec/api/api.md`、`docs/cases/h5-weapp-differences.md` | `grep` | 修后树 | 文档 | 三处条目齐备（`UI_DOC=1`/`API_DOC=1`/`DIFF_DOC=1`） | R7 |
| R8 pre-ready gate | `docs/evidence/2502/pre-ready-2502.json` | `pre-ready-check.sh 2502` | 修后树（commit + push 之后） | 全量口径 | 报告 `overall=PASS`、`provenance.dirty=false`（P12/P16 全 PASS） | R8 |
| `docs/evidence/2502/R9-build-h5.txt` | `bash scripts/verify-2502.sh r9` | `# build=1.0.<short> # commit=<sha> # dirty=false` |
| `docs/evidence/2502/R10-build-weapp.txt` | `bash scripts/verify-2502.sh r10` | 同上 |
| `docs/evidence/2502/R11-smoke.txt` | `bash scripts/verify-2502.sh r11` | 同上 |
| `docs/evidence/2502/R13-legacy-customer.txt` | `bash scripts/verify-2502.sh r13` | 同上 |
| R9 build:h5（自持） | `frontend-mobile/src/pages/Profile.jsx`、`utils/identity.js` | `npm run build:h5`（Node 22） | 修后树 | H5 | exit 0 且 `frontend-mobile/dist/index.html` 含新 hash | R9 |
| R10 build:weapp（自持） | `frontend-mobile/src/pages-weapp/Profile.jsx`、`utils/identity.js` | `npm run build:weapp`（Node 22） | 修后树 | weapp | exit 0 且 `dist-weapp/pages-weapp/profile/index.js` 含新 helper 文案路径 | R10 |
| R11 全量页烟 | 两个 Profile + BottomNav 平台层 | `npm run smoke`（Node 22） | 修后树 | 双端 | `SMOKE_EXIT=0`、`TESTS_FAIL=0` | R11 |
| R13 遗留 USER+org | 两个 Profile | 页烟 fixture：token `role=USER` + `oid`/`tid` + `/users/me` 带 `tenant_name`/`site_name` | 修后树 | H5 + weapp | `ORG_ABSENT_BOTH_ENDS=1` | R13 |
| R12 gate reset | `docs/evidence/2502/pre-ready-2502.json` | `pre-ready-check.sh 2502 …` | 修后树（commit+push 后） | 全量口径 | `overall=PASS` 且 Summary 的 P5 引文与报告 `detail` 逐字一致 | R12 |

## G0 Golden Acceptance

| 行 | fixture（精确输入） | entry（生产入口） | artifact | assertions（可执行断言） | negative | provenance |
|---|---|---|---|---|---|---|
| R1 | `/users/me`=`{tenant_name:'cadenza',site_name:'卡丹萨乐器'}` + `/site-members/me`=`{roles:['site_admin'],sites:[]}` + token `role=STAFF,oid=org-1,tid=tenant-1` | `src/pages-weapp/profile`（→ `pages-weapp/Profile.jsx`） | `docs/evidence/2502/R1-weapp-badge.txt` | `BADGE_TEXT=cadenza · 卡丹萨乐器 · 管理员`；vitest exit 0 | 旧文本 `卡丹萨乐器 · 管理员` 出现 → FAIL | `# build=1.0.<short> # commit=<sha> # dirty=false` |
| R2 | `/users/me`=`{site_name:'卡丹萨乐器'}`；再 `{}` | 同 R1 | `docs/evidence/2502/R2-fallback.txt` | `BADGE_TEXT=卡丹萨乐器 · 管理员`、`BADGE_TEXT=管理员`、`NO_UNDEFINED=1`、`NO_LEADING_SEP=1` | 文本含 `undefined`/`null` 或以 ` · ` 开头 → FAIL | 同上 |
| R3 | 同 R1 fixture | `src/pages/Profile.jsx`（`/profile`） | `docs/evidence/2502/R3-h5-badge.txt` | `H5_BADGE_TEXT=cadenza · 卡丹萨乐器 · 管理员`、`PARITY=1` | `H5_BADGE_TEXT=` 为空（H5 仍无徽标）→ FAIL | 同上 |
| R4 | token `role=USER`；`/users/me`=`{tenant_name:'cadenza',site_name:'卡丹萨乐器'}` | 两个 Profile 入口 | `docs/evidence/2502/R4-customer.txt` | `CUSTOMER_BADGE_ABSENT=1`、`NO_TENANT_LEAK=1` | 顾客页面出现 `cadenza` 或 `卡丹萨乐器` → FAIL | 同上 |
| R5 | 纯函数入参 `('a','b')`、`('','b')`、`(null,null)`、roles `['site_admin','repair_technician']`、roles `[]` | `frontend-mobile/src/utils/identity.js` | `docs/evidence/2502/R5-helper-redgreen.txt` | `FORMAT_FULL=a · b`、`FORMAT_SINGLE=b`、`FORMAT_EMPTY=`、`ROLE_MULTI=管理员 · 维修师傅`、`ROLE_FALLBACK=员工`、`OLD_TEXT=卡丹萨乐器`、`NEW_TEXT=cadenza · 卡丹萨乐器`、`RED_GREEN=1` | 旧表达式已含商户名（`OLD_TEXT` 含 `cadenza`）→ FAIL | 同上 |
| R6 | 修后树 | `npx eslint`（3 个改动 JS 文件） | `docs/evidence/2502/R6-lint.txt` | `ESLINT_EXIT=0`、`NO_UNDEF=0` | `ESLINT_EXIT` 非 0 或 `NO_UNDEF>0` → FAIL | 同上 |
| R7 | 修后树 | `docs/spec/ui/ui.md` §2.5a、`docs/spec/api/api.md` §2.4.1、`docs/cases/h5-weapp-differences.md` A23 | `docs/evidence/2502/R7-docs.txt` | `UI_DOC=1`、`API_DOC=1`、`DIFF_DOC=1` | 任一文档缺条目 → FAIL | 同上 |
| R8 | 修后树 + `docs/evidence/2502/evidence.json` | `pre-ready-check.sh 2502`（P1–P18） | `docs/evidence/2502/pre-ready-2502.json` | `jq -r '.overall'` = `PASS`；`jq -r '.provenance.dirty'` = `false` | `P12`/`P16` 任一 FAIL 或 `dirty=true` → FAIL | JSON `.provenance.{build,commit,dirty}` |

## Artifact Provenance

- 所有文本产物首行统一为 `# build=1.0.<git short> # commit=<40 位 sha> # dirty=false`，由 `scripts/verify-2502.sh` 从 `git rev-parse` 派生（口径同 `scripts/verify-2354.sh` 的 `PROVENANCE`）。
- 全部产物仓库可见，位于 `docs/evidence/2502/`。

| 产物 | 生成者 | provenance 形态 |
|---|---|---|
| `docs/evidence/2502/R1-weapp-badge.txt` | `bash scripts/verify-2502.sh r1` | `# build=1.0.<short> # commit=<sha> # dirty=false` |
| `docs/evidence/2502/R2-fallback.txt` | `bash scripts/verify-2502.sh r2` | 同上 |
| `docs/evidence/2502/R3-h5-badge.txt` | `bash scripts/verify-2502.sh r3` | 同上 |
| `docs/evidence/2502/R4-customer.txt` | `bash scripts/verify-2502.sh r4` | 同上 |
| `docs/evidence/2502/R5-helper-redgreen.txt` | `bash scripts/verify-2502.sh r5` | 同上 |
| `docs/evidence/2502/R6-lint.txt` | `bash scripts/verify-2502.sh r6` | 同上 |
| `docs/evidence/2502/R7-docs.txt` | `bash scripts/verify-2502.sh r7` | 同上 |
| `docs/evidence/2502/pre-ready-2502.json` | `pre-ready-check.sh --report` | JSON `.provenance.{build,commit,dirty}`（gate 自填，与 `evidence.json` 对齐） |
| `docs/evidence/2502/R9-build-h5.txt` | `bash scripts/verify-2502.sh r9` | `# build=1.0.<short> # commit=<sha> # dirty=false` |
| `docs/evidence/2502/R10-build-weapp.txt` | `bash scripts/verify-2502.sh r10` | 同上 |
| `docs/evidence/2502/R11-smoke.txt` | `bash scripts/verify-2502.sh r11` | 同上 |
| `docs/evidence/2502/R13-legacy-customer.txt` | `bash scripts/verify-2502.sh r13` | 同上 |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Input Truth & Tool Preflight

- **Source of truth（字段）**：`GET /api/users/me` 的 `tenant_name`（JWT `tid` → `merchants.name`，#2117）、`site_id`/`site_name`（JWT `oid` → `sites.name`，#2449）。后端已有用例锁定语义（`user_me_tenant_name_2117_test.go`、`current_user_site_2449_test.go`），**本单不改后端**。
- **Source of truth（展示口径）**：`docs/spec/ui/ui.md:1522`「员工上下文 → `{商户名} · {网点名}`」，以及 #2455 在 `docs/spec/api/api.md` 定义的 `contexts[].label = 商户名 · 网点名`（`origin/issue-2455`）。本单只是把该口径补到移动端 Profile 徽标。
- **差异核对**：`grep -rn "site_name" frontend-mobile/src` 共 30 处；除本单两处徽标外均为**实体归属**（乐器/工单/发货仓/中转目标的所属网点），语义不是员工身份上下文 → 不改（逐条见 `## Bug Reflection Matrix`）。
- **损坏输入拒绝**：`/users/me` 中 `tenant_name`/`site_name` 为 `''`、`null` 或缺键 → 徽标回退单项；两者都缺 → 只显示职位；**禁止**渲染 `undefined`、`null`、` · ` 前导/尾随分隔符（R2/R5 断言）。
- **工具盲区**（v2 更新）：父 #2455 的 gate 只覆盖 `go` + `frontend-pc`（其 `R9-build.txt` 实测），**不覆盖 frontend-mobile**；本单自持 `build:h5`/`build:weapp`/`smoke`（R9/R10/R11）。tiering 为**独立 Issue 口径**（正文父链接仅列表项），Summary 只引用报告原文。原说明：
  - vitest 烟测走 `taro-shim`（`@tarojs/components` → HTML）+ jsdom，**不能**验证微信真机渲染；weapp 侧以「同一 helper 产出同一文案」（R5 纯函数）+ 小程序构建由父/批次 gate 覆盖；真机由 ops 在预生产回评。
  - `textContent` 断言是页级黑盒（不依赖 class/style），因此样式微调不会造成假红/假绿。
  - `npx eslint` 仅静态规则，不覆盖运行时行为。
- **版本/上下文口径**：`tenant_name`/`site_name` 跟随**当前 JWT 上下文**（切换身份后变化），本单不改变解析逻辑。
- **构建前置**：`scripts/verify-2502.sh` 需 node 22（`~/.nvm/versions/node/v22*/bin`），其它 node 版本不保证 Taro/vitest 兼容（AGENTS.md「Node.js 版本要求」）。

## Decision Lock

| D6 | 徽标可见性判定 | `role` 非空且非 `USER`/`GUEST`（与 weapp/后端一致），不引入新的角色枚举 |
| D7 | H5 `isStaffRole` 漂移 | 本单只修徽标可见性（R13 范围）；金刚区/BottomNav 等处口径收敛**另立 Issue**，不在本单扩面 |

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 徽标口径 = `商户名 · 网点名 · 职位`，分隔符 ` · `（**target_semantics**；权威 = `docs/spec/ui/ui.md:1522` + #2455 `api.md` 的 `label`） | Architect | 已锁定：R1/R3 用**精确文本**断言，混用 `-`/`/` 会失败 | 需要换分隔符 → 回 Architect 修计划，不得在 /work 改断言 |
| D2 | 数据源 = `/users/me` 的 `tenant_name`（JWT `tid`）+ `site_name`（JWT `oid`）；前端**不得**自拼 `sites`/`merchants` 查询 | Architect | 已锁定：后端 #2117/#2449 已实现并有回归用例 | 需要新字段/新语义 → 回 Architect，本单不做 |
| D3 | H5 补同口径徽标（Issue 验收「H5 与 weapp 渲染一致」+ 修复方向 1）；weapp-only 的「顾客」徽标（`login_contexts`）**不**移植到 H5 | Architect | 已锁定：差异登记为 `docs/cases/h5-weapp-differences.md` A23 | Owner 要求 H5 也做顾客徽标 → 另立 Issue（需要 H5 侧 `login_contexts` 机制） |
| D4 | 与 #2455 的所有权边界：`AccountSelect.jsx`、`frontend-pc/src/pages/UserProfile.jsx`、`wx-accounts` 服务端 `label` 归 #2455（`origin/issue-2455` @ cbb11a0a4 已改，**未合入 develop**）；本单不碰这些文件，也**不依赖**其合入（口径早已存在于 develop 的 `ui.md` §2.24） | Architect | 已锁定：`### Files` 不含上述文件 | 若 #2455 先合入导致 `ui.md` 追加位置冲突 → /work 把追加段放到其 §2.24 bullet **之后**（rebase 重排，不改语义），并在 Work Summary 记录 |
| D5 | 职位标签映射沿用 weapp 现状（`site_admin`→管理员、`site_member`→员工、`repair_technician`→维修师傅；无任何角色回退「员工」） | Architect | 已锁定：R5 断言 `ROLE_MULTI`/`ROLE_FALLBACK` | 需要新标签/新角色 → 另立 Issue |

## Out of Scope

- `frontend-mobile/src/pages-weapp/account-select/AccountSelect.jsx`（#2455 R7 所有权；其 label 已改为服务端组合，本单不改）
- `frontend-pc/src/pages/UserProfile.jsx` 及 PC 端任何文件（#2455 R8 所有权；本单只做移动端）
- 后端 `GetCurrentUser`、`wx-accounts` 的 `label` 组合逻辑（本单只消费既有字段，不改契约）
- 其他 `site_name` 展示点（`Detail.jsx`、`Cart.jsx`、`Checkout.jsx`、`StaffInstruments.jsx`、`StaffInstrumentDetail.jsx`、`RepairRequestDetail.jsx`、`RepairServiceDetail.jsx`、`RepairScan.jsx`、`ReturnConfirm.jsx`）= 实体归属信息，不是员工身份徽标，不改
- weapp 多身份顾客「顾客」徽标移植到 H5（H5 无 `login_contexts`/切换身份；登记 A23）
- H5/weapp `Profile` 的整体双实现收敛（C3，umbrella #2172）

## Core Update

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/ui/ui.md` §2.5a | 只描述顾客/员工金刚区与权限，**无头部身份徽标**定义 | 新增「头部身份徽标（#2502）」：员工上下文显示 `{商户名} · {网点名} · {职位}`；数据源 `GET /users/me` 的 `tenant_name`/`site_name`；缺任一组织段回退单项、均缺只显示职位；**禁止** `undefined` 与前导 ` · `；H5（`pages/Profile.jsx`）与 weapp（`pages-weapp/Profile.jsx`）同文案同顺序；weapp 多身份顾客徽标 H5 无（见 A23） | 规格缺失是本 Bug 的直接成因（两端各自拼口径，漂移无判据） |
| `docs/spec/api/api.md` §2.4.1 | GET `/users/me` 仅列 `username`/`name`/`nickname`/`phone`/`email` 与 `needs_credentials`，**未登记** `site_id`/`site_name`/`tenant_name` | 追加：员工上下文按 JWT `oid` 返回 `site_id`/`site_name`（#2449）、按 JWT `tid` 返回 `tenant_name`（`merchants.name`，#2117）；顾客上下文不返回 `tenant_name`；移动端身份徽标据此渲染（#2502） | 徽标数据来源需成为可引用契约，避免前端再自拼组织名 |
| `docs/cases/h5-weapp-differences.md` §2 | 无身份徽标条目 | 新增 **A23**：员工身份徽标两端同文案（`商户名 · 网点名 · 职位`）；weapp 多身份顾客「顾客」徽标 H5 无（无 `login_contexts`），判定为可接受差异 | AGENTS.md 强制：新增 H5↔weapp 差异必须先登记 |

## Bug Reflection Matrix

| # | 同类路径 | 现状 | 本单处置 | 依据 |
|---|---|---|---|---|
| 1 | `frontend-mobile/src/pages-weapp/Profile.jsx:426` | 网点名优先，商户名被丢弃 | **修**（R1/R2） | 本 Bug |
| 2 | `frontend-mobile/src/pages/Profile.jsx`（H5 头部） | 无任何身份徽标 | **修**（R3） | Issue 验收「H5 与 weapp 一致」 |
| 3 | `AccountSelect.jsx`（`contexts[].label` / legacy label） | 自拼 / `-` 分隔 | **不改**：#2455 R7 已改为服务端 label（未合入） | D4 所有权 |
| 4 | `frontend-pc/src/pages/UserProfile.jsx`「关联网点」 | 只显示 `sites.name` | **不改**：#2455 R8 已改 `identityLabel` | D4 所有权 |
| 5 | `wx-accounts` 服务端 `contexts[].label` | 已组合 `商户名 · 网点名` | **不改**：#2455 后端所有权 | D4 所有权 |
| 6 | `Detail.jsx:308`、`pages-weapp/Detail.jsx:288`、`components/InstrumentInfo.jsx:74-83` | 乐器详情**同时**展示 `tenant_name` 与 `site_name` | **不改**（已满足可辨识；语义=乐器归属） | 现状核对 |
| 7 | `StaffInstruments.jsx:173`、`StaffInstrumentDetail.jsx:265`、`Cart.jsx:368`、`Checkout.jsx:1108` | 只显示网点名 | **不改**：展示库存/发货仓归属，非员工身份；订单卡片另有商户名 | 现状核对 |
| 8 | `RepairRequestDetail.jsx:348/610/816`、`RepairServiceDetail.jsx:637`、`RepairScan.jsx:142`、`ReturnConfirm.jsx:217` | 只显示网点名 | **不改**：工单/师傅/中转目标地址，非身份徽标 | 现状核对 |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2502.sh r1` | `docs/evidence/2502/R1-weapp-badge.txt` | exit 0；grep `BADGE_TEXT=cadenza · 卡丹萨乐器 · 管理员` | PASS | 徽标仍只含网点名（`BADGE_TEXT=卡丹萨乐器 · 管理员`）→ FAIL | dev |
| R2 | `bash scripts/verify-2502.sh r2` | `docs/evidence/2502/R2-fallback.txt` | exit 0；grep `BADGE_TEXT=卡丹萨乐器 · 管理员`、`NO_UNDEFINED=1`、`NO_LEADING_SEP=1` | PASS | 文本出现 `undefined` 或以 ` · ` 开头 → FAIL | dev |
| R3 | `bash scripts/verify-2502.sh r3` | `docs/evidence/2502/R3-h5-badge.txt` | exit 0；grep `H5_BADGE_TEXT=cadenza · 卡丹萨乐器 · 管理员`、`PARITY=1` | PASS | `H5_BADGE_TEXT=` 为空（H5 仍无徽标）→ FAIL | dev |
| R4 | `bash scripts/verify-2502.sh r4` | `docs/evidence/2502/R4-customer.txt` | exit 0；grep `CUSTOMER_BADGE_ABSENT=1`、`NO_TENANT_LEAK=1` | PASS | 顾客上下文页面出现商户名或网点名 → FAIL | dev |
| R5 | `bash scripts/verify-2502.sh r5` | `docs/evidence/2502/R5-helper-redgreen.txt` | exit 0；grep `RED_GREEN=1`、`OLD_TEXT=卡丹萨乐器`、`NEW_TEXT=cadenza · 卡丹萨乐器`、`FORMAT_EMPTY=` | PASS | 旧表达式已含商户名（`OLD_TEXT` 含 `cadenza`）→ FAIL | dev |
| R6 | `bash scripts/verify-2502.sh r6` | `docs/evidence/2502/R6-lint.txt` | exit 0；grep `ESLINT_EXIT=0`、`NO_UNDEF=0` | PASS | `ESLINT_EXIT` 非 0 或 `NO_UNDEF` 大于 0 → FAIL | dev |
| R7 | `bash scripts/verify-2502.sh r7` | `docs/evidence/2502/R7-docs.txt` | exit 0；grep `UI_DOC=1`、`API_DOC=1`、`DIFF_DOC=1` | PASS | 任一文档缺条目（对应键为 0）→ FAIL | dev |
| R8 | `~/.config/opencode/scripts/pre-ready-check.sh 2502 --summary-file docs/evidence/2502/work-summary.md --changed-files docs/evidence/2502/changed-files.txt --evidence-file docs/evidence/2502/evidence.json --report docs/evidence/2502/pre-ready-2502.json` | `docs/evidence/2502/pre-ready-2502.json` | exit 0；jq -r '.overall' = PASS；jq -r '.provenance.dirty' = false | PASS | P12/P16 任一 FAIL 或 dirty=true → FAIL | dev |
| R9 | `bash scripts/verify-2502.sh r9` | docs/evidence/2502/R9-build-h5.txt | exit 0；含 `BUILD_H5_OK` | PASS | 构建失败 → FAIL | dev |
| R10 | `bash scripts/verify-2502.sh r10` | docs/evidence/2502/R10-build-weapp.txt | exit 0；含 `BUILD_WEAPP_OK` | PASS | 构建失败 → FAIL | dev |
| R11 | `bash scripts/verify-2502.sh r11` | docs/evidence/2502/R11-smoke.txt | exit 0；含 `SMOKE_EXIT=0` 与 `TESTS_FAIL=0` | PASS | 任一失败 → FAIL | dev |
| R13 | `bash scripts/verify-2502.sh r13` | docs/evidence/2502/R13-legacy-customer.txt | exit 0；含 `ORG_ABSENT_BOTH_ENDS=1` | PASS | 渲染商户/网点名 → FAIL | dev |
| R12 | `bash scripts/verify-2502.sh r12` | docs/evidence/2502/pre-ready-2502.json | 报告 `overall=PASS` | PASS | 缺报告 → FAIL | architect |

### Workload

- Size: M
- Basis: 1 个新纯函数模块 + 2 个 UI 文件（weapp/H5 双实现各 1 处拼装）+ 1 个新页烟测试文件 + 1 个证据生成脚本 + 3 篇文档（ui/api/h5-weapp-differences）+ 8 条证据行（R1–R8）；无后端改动、无 migration、无外部 API 变更
- Confidence: high

### Files（变更文件清单）

- `docs/evidence/2502/R9-build-h5.txt`
- `docs/evidence/2502/R10-build-weapp.txt`
- `docs/evidence/2502/R11-smoke.txt`
- `docs/evidence/2502/R13-legacy-customer.txt`

- `frontend-mobile/src/utils/identity.js`
- `frontend-mobile/src/pages/Profile.jsx`
- `frontend-mobile/src/pages-weapp/Profile.jsx`
- `frontend-mobile/scripts/page-smoke/profile-identity-badge-2502.test.jsx`
- `scripts/verify-2502.sh`
- `docs/spec/ui/ui.md`
- `docs/spec/api/api.md`
- `docs/cases/h5-weapp-differences.md`
- `docs/evidence/2502/plan.md`
- `docs/evidence/2502/evidence.json`
- `docs/evidence/2502/work-summary.md`
- `docs/evidence/2502/changed-files.txt`
- `docs/evidence/2502/pre-ready-2502.json`
- `docs/evidence/2502/R1-weapp-badge.txt`
- `docs/evidence/2502/R2-fallback.txt`
- `docs/evidence/2502/R3-h5-badge.txt`
- `docs/evidence/2502/R4-customer.txt`
- `docs/evidence/2502/R5-helper-redgreen.txt`
- `docs/evidence/2502/R6-lint.txt`
- `docs/evidence/2502/R7-docs.txt`

*Model: DeepSeek V4.1 Flash 1017*


