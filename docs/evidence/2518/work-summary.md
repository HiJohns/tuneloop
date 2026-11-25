## Work Summary — #2518（收敛：删除客户端管理；租户管理并入商户管理）

> 计划：Accepted Plan（2026-11-25T20:18:20Z），plan_sha256=`2b2a0df76e33f23a02ed08f3f08515c823930a068830d16e3adbc219b1d52aad`。

### 修改内容

- `frontend-pc/src/App.jsx`：删除 `ClientManagement` / `TenantManagement` 两条 import 与 `/system/clients`、`/system/tenants` 两条 `<Route>`（D1/D2）。
- `frontend-pc/src/pages/ClientManagement.jsx`、`frontend-pc/src/pages/TenantManagement.jsx`：删除（客户端无实体；租户 ≡ 商户）。
- `frontend-pc/src/services/api.js`：删除 0 调用方的 `iamAdminApi`（9 条 `/system/clients|/system/tenants` 封装；`grep -rn iamAdminApi frontend-pc/src` 删除后为 0 命中）。
- `docs/spec/ui/ui.md` §5「系统管理」：删除「客户端管理」「租户管理」两条子菜单，新增「**租户即商户（#2518）**」说明（列表 + 新建统一由 `/merchants` 承载）；同时把「纯管理菜单（商户/客户端）」改为「（商户/权限/操作日志）」，避免文档残留不存在的菜单概念。
- `scripts/verify-2518.sh`（新增）：R1–R5 证据生成器（静态 grep + PC build/eslint；重命令经 host 锁；provenance 由最新 fix commit 派生）。
- 商户管理 `/merchants` 未改动：menuConfig `sysPermBits:[6]` 入口与页面「创建商户」按钮均在（R3）；「新建假成功」归 #2489 R6，不在本单扩面（D3）。

### Commit

- `ccb0c8b8e` — fix(issue-2518): drop client/tenant admin pages, keep merchants as tenant owner（最新 fix commit）
- evidence commits：plan 快照 + R1–R5 产物 + evidence.json + gate 报告 + 本 Summary

### Remote

remote: origin/issue-2518; fix=ccb0c8b8e pushed=yes; evidence= pushed=yes（P13 校验）

### 验证层级（Issue #70）

- 本单为独立 Issue（无 Parent 声明），无后端 / 无 Go / 无 migration 改动。
- 定向验收：静态 grep（R1/R2）、菜单与页面入口断言（R3）、文档断言（R4）、`frontend-pc` 全量 `npm run build` + `npx eslint`（R5，重命令经 host 锁）。
- 未运行后端测试（无后端改动）。

P5 引文（与 `docs/evidence/2518/pre-ready-2518.json` 的 `detail` 逐字一致）：

```
P5   N/A    no go.mod/package.json and no --full-gate-cmd: full gate not applicable
```

### 红绿实证（删除类修复）

- R1：删除前 `grep -rnE "ClientManagement|system/clients" frontend-pc/src` 命中 13 行（App.jsx import+route、api.js 4 条封装、页面自身 6 处）→ 删除后 `CLIENT_REFS=0`、`CLIENT_PAGE_FILE=0`。
- R2：删除前同一命令命中 12 行（App.jsx import+route、api.js 5 条封装、页面自身 5 处）→ 删除后 `TENANT_REFS=0`、`TENANT_PAGE_FILE=0`。
- R3（保持项负例）：菜单行必须仍在（`MENU_ENTRY=352`）；若为 `missing`、页面缺失或无「创建商户」入口 → 生成器 rc=1。
- R5：首跑因 worktree 缺 `frontend-pc/node_modules`（`vite: not found`，BUILD_EXIT=127）判 FAIL；补齐依赖后 `BUILD_EXIT=0`、`ESLINT_EXIT=0`、`NO_UNDEF=0` → PASS。

### Accepted Plan 对照

| 计划项 | 状态 |
|---|---|
| 变更点 1 删除客户端页 + `/system/clients` 路由 | 完成（R1） |
| 变更点 2 删除租户页 + `/system/tenants` 路由 | 完成（R2） |
| 变更点 2.5 删除 0 调用方 `iamAdminApi` | 完成（R1/R2 命中 0；`iamAdminApi` 无引用） |
| 变更点 3 ui.md §5 删条目 + 补「租户即商户」 | 完成（R4） |
| 变更点 4 商户管理列表+新建仍在 | 完成（R3，未改该页） |
| 变更点 5 清理引用（`frontend-pc/src` 0 命中；`docs/archive/**`、ui.md:3744 `/tenants` 不动） | 完成（D 范围内） |
| 变更点 6 / D4 #2516 随之失效 | 本单落地后关闭 #2516（见「人工/部署侧」） |
| Decision Lock D1–D4 | 遵守 |
| Out of Scope | 未触碰 `docs/archive/**`、ui.md 历史路由表 `/tenants`、IAM 能力、商户表单 |

### 同类路径扫描

- `grep -rn "ClientManagement\|TenantManagement\|system/clients\|system/tenants\|iamAdminApi" frontend-pc/src` → **0 命中**（删除后）。
- `grep -rn "clients\|tenants" frontend-pc/src --include='*.js' --include='*.jsx'` → 除本单删除项外无残留（menuPermissions/菜单配置本就没有这两个入口，与 Issue 描述一致）。
- 文档侧：`grep -n "客户端\|租户管理" docs/spec/ui/ui.md` → 仅保留 #2518 说明段与历史路由表 `/tenants`（Out of Scope 明确不改）。

### 矩阵 5/5

| 变更点 | 状态 | 证据 |
|---|---|---|
| 删除客户端页 | PASS | `docs/evidence/2518/R1-clients-removed.txt`（CLIENT_REFS=0、CLIENT_PAGE_FILE=0） |
| 删除租户页 | PASS | `docs/evidence/2518/R2-tenants-removed.txt`（TENANT_REFS=0、TENANT_PAGE_FILE=0） |
| 商户管理保留 | PASS | `docs/evidence/2518/R3-merchants-kept.txt`（MENU_ENTRY=352、MERCHANTS_MENU=1、PAGE_EXISTS=1、CREATE_ENTRY=1） |
| 文档同步 | PASS | `docs/evidence/2518/R4-docs.txt`（DOC_REMOVED=1、TENANT_ALIAS=1，附 §5 原文） |
| 构建与 lint | PASS | `docs/evidence/2518/R5-build-lint.txt`（BUILD_EXIT=0、ESLINT_EXIT=0、NO_UNDEF=0、BUILD_OK、ESLINT_OK） |

### Evidence Contract

`docs/evidence/2518/evidence.json`（schema v1，build=1.0.ccb0c8b8、commit=ccb0c8b8e、dirty=false、plan_sha256=2b2a0df7…）5 行全部 `pass=true`：R1/R2/R3/R4/R5 → 产物 `docs/evidence/2518/` 对应 txt。

### G0 Golden Acceptance

| 行 | artifact | assertions |
|---|---|---|
| R1 | `docs/evidence/2518/R1-clients-removed.txt` | CLIENT_REFS=0、CLIENT_PAGE_FILE=0；负例：仍有引用 |
| R2 | `docs/evidence/2518/R2-tenants-removed.txt` | TENANT_REFS=0、TENANT_PAGE_FILE=0；负例：仍有引用 |
| R3 | `docs/evidence/2518/R3-merchants-kept.txt` | MENU_ENTRY=352 + 菜单行原文、MERCHANTS_MENU=1、PAGE_EXISTS=1、CREATE_ENTRY=1 |
| R4 | `docs/evidence/2518/R4-docs.txt` | DOC_REMOVED=1、TENANT_ALIAS=1 + §5 原文（无 clients/tenants 子菜单） |
| R5 | `docs/evidence/2518/R5-build-lint.txt` | BUILD_EXIT=0、ESLINT_EXIT=0、NO_UNDEF=0、BUILD_OK、ESLINT_OK |

### Dependency Order / 验收规则

- `## Dependencies` 为空；未修改任何验收工具或规则；D1–D4 均遵守。
- 关联：#2516 在本单落地后失效（租户页已删除，`POST /system/tenants` 不再可触发）；#2489 R7（租户页防御断言）随页面删除不再需要。

### 人工/部署侧

- 浏览器直连旧 URL 的落点（404/首页）与菜单目视，由 ops 在预生产回评（Developer 无浏览器/预生产执行授权）。
- `#2516` 关闭动作：本单落地后由 Developer 在 Issue 内关闭并说明「页面已删除」。

### Pre-ready gate

命令（干净树 + `origin/issue-2518` 已 push）：

```
pre-ready-check.sh 2518 --summary-file docs/evidence/2518/work-summary.md \
  --changed-files docs/evidence/2518/changed-files.txt \
  --evidence-file docs/evidence/2518/evidence.json \
  --affected-cmd "cd frontend-pc && npx eslint src/App.jsx src/services/api.js" \
  --remote-ref origin/issue-2518 \
  --report docs/evidence/2518/pre-ready-2518.json
```

关键输出（完整报告见 `docs/evidence/2518/pre-ready-2518.json`）：

```
P1   PASS   plan has Verification Matrix / Input Truth & Tool Preflight / Decision Lock / Out of Scope
P2   PASS   Work Summary covers all 5 matrix rows with PASS/N/A
P3   PASS   all referenced evidence paths exist and are durable
P4   PASS   all 1 affected command(s) passed
P5   N/A    no go.mod/package.json and no --full-gate-cmd: full gate not applicable
P6   PASS   all changed files are declared in the Accepted Plan
P8   N/A    no panel/clarify files changed
P12  PASS   Evidence Contract complete: 5 row(s) match the plan and have durable artifacts
P14  PASS   artifact provenance matches evidence
P17  PASS   plan snapshot and Accepted Plan comment match the gated plan
RESULT: PASS
```

plan 来源：Accepted Plan 评论（2026-11-25T20:18:20Z），四源（Accepted Plan 评论 / plan 快照 / gate 报告 / evidence.json）plan_sha256 一致。

*Model: DeepSeek V4.1 Flash 1017*
