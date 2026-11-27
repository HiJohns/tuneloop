## Accepted Plan — #2518（收敛：删除客户端管理；租户管理并入商户管理）

### Owner 裁定（2026-11-25）

| 页面 | 裁定 |
|---|---|
| 客户端管理 `/system/clients` | **删除**（本项目没有「客户端」实体，仅有 OAuth client，属 IAM 概念，平台端不承载） |
| 租户管理 `/system/tenants` | **删除独立页面**：**租户 ≡ 商户**，统一由 **商户管理 `/merchants`** 承载（列表 + 新建都在该页完成） |

因此本单不是单纯 A/B，而是「**客户端=删除；租户=并页**」。

### 变更点

1. 删除 `frontend-pc/src/pages/ClientManagement.jsx` 与 `App.jsx` 中 `/system/clients` 路由；
2. 删除 `frontend-pc/src/pages/TenantManagement.jsx` 与 `App.jsx` 中 `/system/tenants` 路由；
2.5 删除 `frontend-pc/src/services/api.js` 中 **0 调用方**的 `iamAdminApi`（`getClients/createClient/updateClient/deleteClient` + `getTenants/createTenant/getTenant/updateTenant/deleteTenant`，共 9 条 `/system/clients|tenants` 封装），保证 `grep -rn "system/clients\|system/tenants" frontend-pc/src` 为 **0**；
3. `docs/spec/ui/ui.md` §5「系统管理」删除这两条子菜单条目，并补一句「租户即商户，统一由『商户管理』承载」；
4. 确认 **商户管理 `/merchants`** 的「列表 + 新建」仍在（菜单 `sysPermBits:[6]` 已有入口）；该页「新建假成功」属 #2489 R6（判 `res.code`），不在本单重复实现；
5. 清理引用：残留范围 = **`frontend-pc/src`（0 命中）** + **`docs/spec/ui/ui.md` §5（条目删除）**；`docs/archive/**` 为**历史归档快照，不修改**；`docs/spec/ui/ui.md:3744` 的历史路由表（路径为 `/tenants`，非 `/system/tenants`）**不在本单范围**；
6. **#2516 随之失效**（`POST /system/tenants` 缺失在无该页后不再可触发）→ 在本单落地后关闭 #2516。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| 删除客户端页 | `frontend-pc/src/App.jsx` | `/system/clients` | 直接访问该路径 | PC | 路由不存在（404/无匹配），无 `ClientManagement` 引用 | R1 |
| 删除租户页 | `frontend-pc/src/App.jsx` | `/system/tenants` | 直接访问该路径 | PC | 路由不存在，无 `TenantManagement` 引用 | R2 |
| 商户管理保留 | `frontend-pc/src/App.jsx`、`frontend-pc/src/pages/MerchantManagement.jsx` | `/merchants` | 打开菜单 | PC | 菜单项存在（`sysPermBits:[6]`）；页面含列表与「新建」入口 | R3 |
| 文档同步 | `docs/spec/ui/ui.md` | 文档 | 静态 | 文档 | §5 无 clients/tenants 子菜单；含「租户即商户」说明 | R4 |
| 构建与 lint | `frontend-pc` | `npm run build` + `npx eslint` | 修后树 | PC | 均 exit 0 | R5 |

## Input Truth & Tool Preflight

- **Source of truth**：Owner 2026-11-25 裁定（客户端删除、租户并入商户）；`backend/main.go:918-919` 仅有 `GET`（无 POST/PUT），佐证两页均无写能力。
- **差异核对**：删除前先 `grep -rn "TenantManagement\|ClientManagement"` 列出全部引用点，确保 Files 覆盖（含 `App.jsx` 的 import 与 route）。
- **损坏输入拒绝**：若仍有其它页面 `navigate('/system/tenants')`，必须先改为 `/merchants`，不得留死链。
- **工具盲区**：静态 grep 不能证明浏览器不可达 → R6 由 ops 回执（访问旧 URL 应落 404/首页）。

## Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 客户端页 | 删除（无实体） |
| D2 | 租户页 | 删除；**租户 ≡ 商户**，统一 `/merchants` |
| D3 | 商户管理能力 | 列表 + 新建保留；「新建假成功」归 #2489 R6 |
| D4 | #2516 处置 | 本单落地后关闭（页面已不存在） |

## Out of Scope

- `docs/archive/**` 历史归档报告不随页面删除改写（保留历史快照可追溯性）；
- `docs/spec/ui/ui.md:3744` 历史路由表中的 `/tenants` 条目（非 `/system/tenants`）。

- 不改 IAM 侧 namespace/tenant 管理能力；
- 不新增「租户」概念页面；
- 不改商户管理的表单校验与文案（归 #2489/#2504）。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 修后树 | `grep -rn "ClientManagement\|system/clients" frontend-pc/src` | `docs/evidence/2518/R1-clients-removed.txt` | 命中数 **0** | 仍有路由/import → FAIL |
| R2 | 修后树 | `grep -rn "TenantManagement\|system/tenants" frontend-pc/src` | `docs/evidence/2518/R2-tenants-removed.txt` | 命中数 **0** | 仍有引用 → FAIL |
| R3 | 修后树 | `grep -n "'/merchants'" frontend-pc/src/App.jsx` + 页面文件存在 | `docs/evidence/2518/R3-merchants-kept.txt` | 菜单项存在（含 `sysPermBits`）、`MerchantManagement.jsx` 存在 | 菜单缺失 → FAIL |
| R4 | `docs/spec/ui/ui.md` | `grep` | `docs/evidence/2518/R4-docs.txt` | 两页条目已删 + 含「租户即商户」说明 | 文档仍列两页 → FAIL |
| R5 | 修后树 | `bash scripts/verify-2518.sh r5` | `docs/evidence/2518/R5-build-lint.txt` | `BUILD_OK`、`ESLINT_OK` | 任一失败 → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2518.sh r1` | docs/evidence/2518/R1-clients-removed.txt | exit 0；含 `CLIENT_REFS=0` | PASS | 仍有引用 → FAIL | dev |
| R2 | `bash scripts/verify-2518.sh r2` | docs/evidence/2518/R2-tenants-removed.txt | exit 0；含 `TENANT_REFS=0` | PASS | 仍有引用 → FAIL | dev |
| R3 | `bash scripts/verify-2518.sh r3` | docs/evidence/2518/R3-merchants-kept.txt | exit 0；含 `MERCHANTS_MENU=1` 与 `PAGE_EXISTS=1` | PASS | 菜单缺失 → FAIL | dev |
| R4 | `bash scripts/verify-2518.sh r4` | docs/evidence/2518/R4-docs.txt | exit 0；含 `DOC_REMOVED=1` 与 `TENANT_ALIAS=1` | PASS | 文档残留 → FAIL | dev |
| R5 | `bash scripts/verify-2518.sh r5` | docs/evidence/2518/R5-build-lint.txt | exit 0；含 `BUILD_OK` 与 `ESLINT_OK` | PASS | 构建失败 → FAIL | dev |

## Artifact Provenance

- R1–R5（脚本生成，首行 provenance）：写 `# build=1.0.<短码>`、`# commit=<40 位 sha>`、`# dirty=false`。
  - docs/evidence/2518/R1-clients-removed.txt
  - docs/evidence/2518/R2-tenants-removed.txt
  - docs/evidence/2518/R3-merchants-kept.txt
  - docs/evidence/2518/R4-docs.txt
  - docs/evidence/2518/R5-build-lint.txt
- gate（由 /work 执行）：`~/.config/opencode/scripts/pre-ready-check.sh 2518 --summary-file … --changed-files … --evidence-file docs/evidence/2518/evidence.json --report docs/evidence/2518/pre-ready-2518.json`。
- `docs/evidence/2518/evidence.json` 的 build 字段须匹配 evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$。

## Dependencies

- none

### 关联说明（非声明）

- #2516（租户页 POST 404 假成功）→ 本单删除页面后失效，落地即关闭；
- #2489 R7（租户页防御断言）→ 页面删除后该行不再需要转换，由本单承接。

### Workload

- Size: S
- Basis: 删除 2 个页面 + 2 条路由 + 文档 1 处 + 1 个验证脚本 + 5 条证据行；无后端改动
- Confidence: high

### Files（变更文件清单）

- `frontend-pc/src/App.jsx`
- `frontend-pc/src/pages/MerchantManagement.jsx`
- `frontend-pc/src/services/api.js`
- `frontend-pc/src/pages/ClientManagement.jsx`
- `frontend-pc/src/pages/TenantManagement.jsx`
- `docs/spec/ui/ui.md`
- `scripts/verify-2518.sh`
- `docs/evidence/2518/R1-clients-removed.txt`
- `docs/evidence/2518/R2-tenants-removed.txt`
- `docs/evidence/2518/R3-merchants-kept.txt`
- `docs/evidence/2518/R4-docs.txt`
- `docs/evidence/2518/R5-build-lint.txt`
- `docs/evidence/2518/pre-ready-2518.json`
- `docs/evidence/2518/evidence.json`

*Model: DeepSeek V4.1 Flash 1017*


