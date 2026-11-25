## Work Summary — #2502（移动端「我的」身份徽标补齐商户名；H5 同口径）

> 计划：Accepted Plan **v2**（2026-11-25T14:10:07Z），plan_sha256=`0bb29afb53e8e0db02220bc7e7fe0a5cce10eb50ead5d5f0a2626be6648e1781`。
> 本轮（Round 1 审计 REJECT 后）补齐：R9 自持 `build:h5`、R10 自持 `build:weapp`、R11 全量页烟、R13 遗留 `USER + org` token、R12 gate 重置。

### 修改内容

**v1（首次实现，commit `813a0c5e6`）**

- `frontend-mobile/src/utils/identity.js`（新增，单一 owner）：`formatOrgIdentity(merchantName, siteName)`（` · ` 连接 + `filter(Boolean)`）、`roleBadgeTextFromRoles(roles, isStaff)`（管理员/员工/维修师傅，多角色 ` · ` 连接，无匹配回退「员工」，非员工「顾客」）、`identityBadgeText({tenantName, siteName, roles, isStaff})`；页面不再自拼口径。
- `frontend-mobile/src/pages-weapp/Profile.jsx`：`roleBadges/roleBadgeText` 改由 helper 产出（行为不变）；徽标文本改为 `identityBadgeText({ tenantName: user?.tenant_name, siteName: user?.site_name, roles: myRoles, isStaff })`，渲染条件与 11px 药丸样式未改。
- `frontend-mobile/src/pages/Profile.jsx`（H5）：员工上下文在姓名行之后、会员等级行之前新增徽标行，文案与 weapp 完全一致（同一 helper）。
- `frontend-mobile/scripts/page-smoke/profile-identity-badge-2502.test.jsx`（新增）：R1–R5 用例（RTL `render` + `waitFor` 页级文本断言，`process.stdout.write` 输出机器键值）。
- `scripts/verify-2502.sh`（新增）：证据生成器（node22；重命令经 host 锁 `heavy-run.sh`；provenance 由最新 fix commit 派生）；`kv()` 区分「空值」与「缺行」，保证 `FORMAT_EMPTY=` 可判定。
- 文档（Core Update）：`docs/spec/ui/ui.md` 新增「头部身份徽标（#2502）」；`docs/spec/api/api.md` 登记 `/users/me` 的 `site_id`/`site_name`/`tenant_name` 上下文语义；`docs/cases/h5-weapp-differences.md` 新增 A23。

**v2（Round 1 审计闭环，commit `84343702a` + `405cb8f20`）**

- `frontend-mobile/src/utils/identity.js`：新增 `isStaffBadgeVisible(role)`（`role` 非空且非 `USER`/`GUEST`），与 weapp `isStaff`、后端 `GetBusinessRole` 同口径（D6）。修复审计 Bug 3：H5 徽标此前用 `isStaffRole(token)`（`oid`/`tid` 非空即员工），遗留 `role=USER + oid/tid` token 会渲染组织名，与 weapp/后端相反。
- `frontend-mobile/src/pages/Profile.jsx`：徽标可见性改用 `isStaffBadgeVisible(parseJWT(token).role)`；`myRoles` 拉取等其他 `isStaff` 用途保持原状（D7 不扩面）。
- `frontend-mobile/src/pages-weapp/Profile.jsx`：徽标文本同样改用 `isStaffBadgeVisible(claims.role)`，两端可见性判定收口到同一 helper。
- `frontend-mobile/scripts/page-smoke/profile-identity-badge-2502.test.jsx`：新增 `TestProfileBadgeLegacyCustomer_2502`（R13，遗留 `USER + oid/tid`）。
- `scripts/verify-2502.sh`：新增 `r9`（`build:h5` + `npm run build`）、`r10`（`build:weapp`）、`r11`（`npm run smoke`）、`r12`（校验 canonical gate 报告）、`r13`（遗留 token 两端负例）；R9/R10 断言基于 Taro 产物实际形态（minify + CJK `\uXXXX` 转义）。
- `docs/spec/ui/ui.md` §2.5a：补充「可见性（#2502 D6）」条目（审计 Bug 3 要求：判定口径变更须同步规格）。

### Commit

- `813a0c5e6` — fix(issue-2502): unify mobile profile identity badge as merchant · site · role（v1）
- `84343702a` — fix(issue-2502): role-based badge visibility + self-owned build/smoke evidence rows（v2；最新 fix commit）
- `405cb8f20` — fix(issue-2502): assert weapp/H5 bundle contents with escaped Taro output
- evidence commits：plan 快照 + R1–R13 产物 + evidence.json + gate 报告 + 本 Summary

### Remote

remote: origin/issue-2502; fix=405cb8f20 pushed=yes; evidence= pushed=yes（P13 校验通过）

### 验证层级（Issue #70）

本单按 **独立 Issue** 口径自持全部目标端验证（Issue 正文的 `Parent: #2455` 是列表项，gate 的 `^Parent:` 精确匹配不成立 → 报告不按子任务处理，与 v2 计划「tiering 为独立 Issue 口径」一致）：

- 定向页烟：`npx vitest run --config vitest.config.js scripts/page-smoke/profile-identity-badge-2502.test.jsx`（6 用例，R1–R5 + R13）
- 静态检查：`npx eslint src/utils/identity.js src/pages/Profile.jsx src/pages-weapp/Profile.jsx`
- **目标端构建/烟测（v2 新增，不依赖父任务 gate）**：`npm run build:h5`（R9）、`npm run build:weapp`（R10）、`npm run smoke`（R11，117 passed / 0 failed）
- 无后端 / 无 Go / 无 migration 改动。

P5 引文（与 `docs/evidence/2502/pre-ready-2502.json` 的 `detail` 逐字一致）：

```
P5   N/A    no go.mod/package.json and no --full-gate-cmd: full gate not applicable
```

### 红绿实证（bug fix）

- 页级红绿（R1 vs 旧表达式）：同一 fixture（`/users/me` = `{tenant_name:'cadenza', site_name:'卡丹萨乐器'}`、roles `['site_admin']`）下，旧表达式 `[[site_name || tenant_name]]` 产出 `卡丹萨乐器 · 管理员`（商户名被丢弃，用例以 `OLD_TEXT_ABSENT` 负例断言其**不得出现**）；新 helper 产出 `cadenza · 卡丹萨乐器 · 管理员`。
- 纯函数红绿（R5）：`OLD_TEXT=卡丹萨乐器`（不含 `cadenza`）→ `NEW_TEXT=cadenza · 卡丹萨乐器`，`RED_GREEN=1`。
- H5 红绿（R3）：修复前 H5 头部无任何徽标（`H5_BADGE_TEXT` 取不到值 → 该行 FAIL）；修复后 `H5_BADGE_TEXT=cadenza · 卡丹萨乐器 · 管理员`、`PARITY=1`。
- **R13 红绿（审计 Bug 3）**：fixture = token `role=USER` **且** `oid`/`tid` 非空 + `/users/me` 带 `tenant_name`/`site_name`。修复前 H5 的 `isStaffRole(token)` 返回 true → 渲染 `cadenza · 卡丹萨乐器 · …`（红）；修复后 `isStaffBadgeVisible('USER')===false` → 两端都不渲染组织名（绿），断言 `ORG_ABSENT_BOTH_ENDS=1`。
- **R10 红绿（证据形态）**：初版断言用未压缩函数名 `identityBadgeText`，在 Taro weapp 产物中计数为 0（`HELPER_REF_COUNT=0`，FAIL）；改为断言产物中稳定形态（`tenantName:` 调用点、转义角色标签三元、`tenant_name`/`site_name` 字段）后 `HELPER_CALL_REFS=1`、`ROLE_LABEL_TRIPLE=1`、`TENANT_NAME_REFS=1`、`SITE_NAME_REFS=1` → PASS。

### Accepted Plan v2 对照

| 计划项 | 状态 |
|---|---|
| 变更点 1 单一 owner `utils/identity.js`（纯函数、无 IO） | 完成（v2 增 `isStaffBadgeVisible`，仍为单一 owner） |
| 变更点 2 weapp Profile 改用 helper（角色标签行为不变） | 完成 |
| 变更点 3 H5 Profile 新增同口径徽标行 | 完成 |
| 变更点 4 页烟测试（R1–R5 + v2 的 R13） | 完成（6 用例） |
| 变更点 5 `scripts/verify-2502.sh`（v2：r1–r7、r9–r13） | 完成 |
| 变更点 6 文档同步（ui/api/h5-weapp-differences） | 完成（ui.md 另补 D6 可见性） |
| v2 矩阵 R9/R10/R11/R13/R12 | 完成（产物见下） |
| Decision Lock D1–D7 | 遵守（D6 可见性、D7 不扩面） |
| Out of Scope（AccountSelect / PC / 后端 / 其他 site_name 展示点） | 未触碰（`git diff` 无这些文件） |

### 同类路径扫描（Bug Reflection Matrix 1–8）

| # | 位置 | 结论 | 本单处置 |
|---|---|---|---|
| 1 | `frontend-mobile/src/pages-weapp/Profile.jsx` | 网点名优先、商户名被丢弃 | 已修（R1/R2） |
| 2 | `frontend-mobile/src/pages/Profile.jsx`（H5 头部） | 无身份徽标 | 已修（R3） |
| 3 | AccountSelect.jsx | #2455 R7 所有权（服务端 label） | 不改（D4） |
| 4 | `frontend-pc/src/pages/UserProfile.jsx` | #2455 R8 所有权 | 不改（D4） |
| 5 | `wx-accounts` 的 contexts[].label | #2455 后端所有权 | 不改（D4） |
| 6 | 乐器详情 `tenant_name`+`site_name` | 已满足可辨识（实体归属） | 不改 |
| 7 | StaffInstruments.jsx / Cart.jsx / Checkout.jsx 等网点名 | 库存/发货仓归属 | 不改 |
| 8 | 维修类页面网点名 | 工单/师傅/中转目标地址 | 不改 |

扫描命令：`grep -rn "site_name" frontend-mobile/src`（30 处）逐条归类，仅 #1/#2 属员工身份徽标。
同类判定扫描（v2）：`grep -rn "isStaffRole(" frontend-mobile/src` 共 5 处；本单只改徽标可见性（`utils/identity.js` 的 `isStaffBadgeVisible`），金刚区/BottomNav 等既有 `isStaffRole` 判定保持不变（D7，收敛另立 Issue）。

### 矩阵 17/17

| 变更点 | 状态 | 证据 |
|---|---|---|
| R1 weapp 徽标三段 | PASS | `docs/evidence/2502/R1-weapp-badge.txt`（BADGE_TEXT=cadenza · 卡丹萨乐器 · 管理员、OLD_TEXT_ABSENT=1） |
| R2 缺 tenant_name 回退单项 | PASS | `docs/evidence/2502/R2-fallback.txt`（BADGE_TEXT=卡丹萨乐器 · 管理员、BADGE_TEXT_ONLY_ROLE=管理员、NO_UNDEFINED=1、NO_LEADING_SEP=1） |
| R3 H5 与 weapp 同口径 | PASS | `docs/evidence/2502/R3-h5-badge.txt`（H5_BADGE_TEXT=cadenza · 卡丹萨乐器 · 管理员、PARITY=1） |
| R4 顾客上下文不泄漏组织名 | PASS | `docs/evidence/2502/R4-customer.txt`（CUSTOMER_BADGE_ABSENT=1、NO_TENANT_LEAK=1） |
| R5 纯函数语义 + 红绿对照 | PASS | `docs/evidence/2502/R5-helper-redgreen.txt`（FORMAT_FULL/FORMAT_SINGLE/FORMAT_EMPTY、ROLE_MULTI/ROLE_FALLBACK、OLD_TEXT=卡丹萨乐器、NEW_TEXT=cadenza · 卡丹萨乐器、RED_GREEN=1） |
| R6 静态检查 | PASS | `docs/evidence/2502/R6-lint.txt`（ESLINT_EXIT=0、NO_UNDEF=0） |
| R7 规格/接口/差异文档 | PASS | `docs/evidence/2502/R7-docs.txt`（UI_DOC=1、API_DOC=1、DIFF_DOC=1） |
| R8 pre-ready gate | PASS | `docs/evidence/2502/pre-ready-2502.json`（overall=PASS、provenance.dirty=false） |
| docs/evidence/2502/R9-build-h5.txt | PASS | 该产物即 R9 证据（BUILD_H5_OK、H5_BUNDLE_HASH=assets/index-Cml1qTK1.js） |
| docs/evidence/2502/R10-build-weapp.txt | PASS | 该产物即 R10 证据（BUILD_WEAPP_OK、HELPER_CALL_REFS=1、ROLE_LABEL_TRIPLE=1） |
| docs/evidence/2502/R11-smoke.txt | PASS | 该产物即 R11 证据（SMOKE_EXIT=0、TESTS_PASSED=117、TESTS_FAIL=0） |
| docs/evidence/2502/R13-legacy-customer.txt | PASS | 该产物即 R13 证据（TEST_EXIT=0、ORG_ABSENT_BOTH_ENDS=1） |
| R9 build:h5（自持） | PASS | `docs/evidence/2502/R9-build-h5.txt`（BUILD_EXIT=0、VITE_BUILD_EXIT=0、H5_INDEX_HTML=present、H5_BUNDLE_HASH=assets/index-Cml1qTK1.js、DIST_ASSETS_WITH_ROLE_LABEL=1） |
| R10 build:weapp（自持） | PASS | `docs/evidence/2502/R10-build-weapp.txt`（BUILD_EXIT=0、WEAPP_PROFILE_BUNDLE=present、37845 bytes、HELPER_CALL_REFS=1、ROLE_LABEL_TRIPLE=1、TENANT_NAME_REFS=1、SITE_NAME_REFS=1） |
| R11 全量页烟 | PASS | `docs/evidence/2502/R11-smoke.txt`（SMOKE_EXIT=0、TESTS_PASSED=117、TESTS_FAIL=0） |
| R13 遗留 USER+org | PASS | `docs/evidence/2502/R13-legacy-customer.txt`（ORG_ABSENT_BOTH_ENDS=1） |
| R12 gate reset | PASS | `docs/evidence/2502/pre-ready-2502.json`（overall=PASS、provenance.dirty=false；P5 引文见上，逐字一致） |

### Evidence Contract

`docs/evidence/2502/evidence.json`（schema v1，build=1.0.405cb8f2、commit=405cb8f20、dirty=false、plan_sha256=0bb29afb…）13 行全部 `pass=true`：

| 行 | 结果 | 产物 |
|---|---|---|
| R1 | PASS | `docs/evidence/2502/R1-weapp-badge.txt` |
| R2 | PASS | `docs/evidence/2502/R2-fallback.txt` |
| R3 | PASS | `docs/evidence/2502/R3-h5-badge.txt` |
| R4 | PASS | `docs/evidence/2502/R4-customer.txt` |
| R5 | PASS | `docs/evidence/2502/R5-helper-redgreen.txt` |
| R6 | PASS | `docs/evidence/2502/R6-lint.txt` |
| R7 | PASS | `docs/evidence/2502/R7-docs.txt` |
| R8 | PASS | `docs/evidence/2502/pre-ready-2502.json` |
| R9 | PASS | `docs/evidence/2502/R9-build-h5.txt` |
| R10 | PASS | `docs/evidence/2502/R10-build-weapp.txt` |
| R11 | PASS | `docs/evidence/2502/R11-smoke.txt` |
| R12 | PASS | `docs/evidence/2502/pre-ready-2502.json` |
| R13 | PASS | `docs/evidence/2502/R13-legacy-customer.txt` |

### G0 Golden Acceptance

| 行 | artifact | assertions |
|---|---|---|
| R1 | `docs/evidence/2502/R1-weapp-badge.txt` | TEST_EXIT=0、BADGE_TEXT=cadenza · 卡丹萨乐器 · 管理员、OLD_TEXT_ABSENT=1 |
| R2 | `docs/evidence/2502/R2-fallback.txt` | BADGE_TEXT=卡丹萨乐器 · 管理员、BADGE_TEXT_ONLY_ROLE=管理员、NO_UNDEFINED=1、NO_LEADING_SEP=1 |
| R3 | `docs/evidence/2502/R3-h5-badge.txt` | H5_BADGE_TEXT 与 R1 逐字相同、PARITY=1 |
| R4 | `docs/evidence/2502/R4-customer.txt` | CUSTOMER_BADGE_ABSENT=1、NO_TENANT_LEAK=1 |
| R5 | `docs/evidence/2502/R5-helper-redgreen.txt` | FORMAT_FULL=a · b、FORMAT_SINGLE=b、FORMAT_EMPTY=（空值）、ROLE_MULTI=管理员 · 维修师傅、ROLE_FALLBACK=员工、OLD_TEXT 不含 cadenza、NEW_TEXT=cadenza · 卡丹萨乐器、RED_GREEN=1 |
| R6 | `docs/evidence/2502/R6-lint.txt` | ESLINT_EXIT=0、NO_UNDEF=0 |
| R7 | `docs/evidence/2502/R7-docs.txt` | UI_DOC=1、API_DOC=1、DIFF_DOC=1（含条目原文摘录） |
| R8 | `docs/evidence/2502/pre-ready-2502.json` | overall=PASS、provenance.dirty=false |
| R9 | `docs/evidence/2502/R9-build-h5.txt` | BUILD_EXIT=0、VITE_BUILD_EXIT=0、H5_INDEX_HTML=present、H5_BUNDLE_HASH 非 missing、DIST_ASSETS_WITH_ROLE_LABEL=1、TARO_H5_HELPER_FILES=1、BUILD_H5_OK |
| R10 | `docs/evidence/2502/R10-build-weapp.txt` | BUILD_EXIT=0、WEAPP_PROFILE_BUNDLE=present、HELPER_CALL_REFS=1、ROLE_LABEL_TRIPLE=1、TENANT_NAME_REFS=1、SITE_NAME_REFS=1、BUILD_WEAPP_OK |
| R11 | `docs/evidence/2502/R11-smoke.txt` | SMOKE_EXIT=0、TESTS_PASSED=117、TESTS_FAIL=0 |
| R12 | `docs/evidence/2502/pre-ready-2502.json` | overall=PASS、provenance.dirty=false |
| R13 | `docs/evidence/2502/R13-legacy-customer.txt` | TEST_EXIT=0、ORG_ABSENT_BOTH_ENDS=1 |

### Core Update 完成情况

| 文档 | 落地 |
|---|---|
| `docs/spec/ui/ui.md` | 新增「头部身份徽标（#2502）」：口径、数据源、回退、职位映射、双端一致、**可见性（D6）** |
| `docs/spec/api/api.md` | 新增「GET 组织上下文字段（#2449 / #2117 / #2502）」：`site_id`/`site_name`/`tenant_name` 语义与「前端不得自拼组织名」 |
| `docs/cases/h5-weapp-differences.md` | 新增 A23：员工徽标两端同文案；weapp 顾客徽标 H5 无（可接受差异） |

### 契约三向核对

| 方向 | 内容 | 结论 |
|---|---|---|
| 计划/契约 | `GET /users/me` 已返回 `tenant_name`（JWT tid → merchants.name，#2117）与 `site_id`/`site_name`（JWT oid → sites.name，#2449）；本单只登记文档，不改后端契约 | 一致（后端零改动） |
| 文档 | `docs/spec/api/api.md`「GET 组织上下文字段（#2449 / #2117 / #2502）」；`docs/spec/ui/ui.md` §2.5a；`docs/cases/h5-weapp-differences.md` A23 | 已落（R7 artifact 附原文） |
| 消费方 | H5 `frontend-mobile/src/pages/Profile.jsx` 与 weapp `frontend-mobile/src/pages-weapp/Profile.jsx` 只读上述字段，经 `frontend-mobile/src/utils/identity.js` 单一 helper 渲染 | 一致（R1–R5、R13） |

### Dependency Order / 验收规则

- `## Dependencies` 为空；#2455 的 PC/服务端所有权文件（AccountSelect.jsx、frontend-pc/src/pages/UserProfile.jsx、wx-accounts label）本单未触碰，也不依赖其合入。
- 未修改任何验收工具或规则；Decision Lock D1–D7 均遵守。
- H5/weapp 差异已按 AGENTS.md 强制要求先登记 A23 再实现。

### 人工/部署侧

- 微信真机渲染（`pages-weapp/profile` 徽标文案）与预生产 H5 实测由 ops 在预生产回评，Developer 无真机/预生产执行授权；跟踪单：`[Ops-Followup of #2502]` #2503（label `waitForConfirm`，无 `status:*`）。

### Pre-ready gate

命令（干净树 + `origin/issue-2502` 已 push）：

```
pre-ready-check.sh 2502 --summary-file docs/evidence/2502/work-summary.md \
  --changed-files docs/evidence/2502/changed-files.txt \
  --evidence-file docs/evidence/2502/evidence.json \
  --affected-cmd "cd frontend-mobile && npx vitest run --config vitest.config.js scripts/page-smoke/profile-identity-badge-2502.test.jsx" \
  --remote-ref origin/issue-2502 \
  --report docs/evidence/2502/pre-ready-2502.json
```

关键输出（完整报告见 `docs/evidence/2502/pre-ready-2502.json`）：

```
P1   PASS   plan has Verification Matrix / Input Truth & Tool Preflight / Decision Lock / Out of Scope
P2   PASS   Work Summary covers all 17 matrix rows with PASS/N/A
P3   PASS   all referenced evidence paths exist and are durable
P4   PASS   all 1 affected command(s) passed
P5   N/A    no go.mod/package.json and no --full-gate-cmd: full gate not applicable
P6   PASS   all changed files are declared in the Accepted Plan
P7   PASS   contract three-way section present
P12  PASS   Evidence Contract complete: 13 row(s) match the plan and have durable artifacts
P13  PASS   fix commit and evidence artifacts are on origin/issue-2502
P14  PASS   artifact provenance matches evidence (13 artifacts)
P16  PASS   UI rows use render evidence or a recorded static exemption
P17  PASS   plan snapshot and Accepted Plan comment match the gated plan
RESULT: PASS
```

plan 来源：Accepted Plan v2 评论（2026-11-25T14:10:07Z），plan_sha256=`0bb29afb53e8e0db02220bc7e7fe0a5cce10eb50ead5d5f0a2626be6648e1781`（Accepted Plan 评论 / plan 快照 / gate 报告 / evidence.json 四源一致）。

### R8/R12 bootstrap 说明（证据形态）

`docs/evidence/2502/pre-ready-2502.json` 既是 R8/R12 的产物又是 gate 的输入：首次运行因该工件（及 R9–R13 产物）尚未提交而 P3/P12 FAIL（报告仍被写出）；提交并 push 全部产物后复跑即 PASS。两次运行都记录在 git 历史与 report 的 `timestamp` 中，未修改任何断言迁就结果。

*Model: DeepSeek V4.1 Flash 1017*
