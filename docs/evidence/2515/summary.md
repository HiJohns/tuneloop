## Work Summary — #2515 `GET /api/instrument-loss` 未按 oid 隔离（网点账号可见本商户全部丢失台账）

- Issue: #2515（独立 Issue，无 Parent；Size M）
- Worktree / branch: `/tmp/opencode/tuneloop/wt-2515` / `issue-2515`
- Accepted Plan: `plan_sha256=c3d4ae2062fd45928e866898e1dfc00e06852853ee7a6125058c4738f86c812f`
- Fix commit: `254dbaecf76bfc0b506ebf2c6a4f7af037b0b484`
- Evidence commit: 见下方 Remote 行（`docs(evidence): issue-2515 …`）

### 修改内容

1. **读侧收敛（唯一改动点）**：`backend/handlers/instrument_loss.go` 的 `List` 保留租户过滤，新增站点收敛——`business_role ∈ {site_admin, site_member}` 时用子查询 `instruments WHERE tenant_id = ?` 再经 **`applyInstrumentListScope`（#1911，唯一权威）**，以 `instrument_id IN (?)` 与既有 `instrument_id` / `open` 条件取交集。商户/平台/系统管理员不进入该分支（保持全量）。
2. **口径**：站点归属使用 `instruments.site_id`（**归属站点**）；**不使用** `current_site_id`（调拨会改写 → 历史凭证随乐器搬家，会制造新的跨网点泄漏）。`site_id IS NULL` 的历史乐器对网点账号不可见（fail-closed，D5）。
3. **写侧未改**：`loadLossInstrument` 与丢失登记/找回逻辑零改动（D3，避免与 #2510 冲突）。
4. **测试**（新增 `backend/handlers/instrument_loss_2515_test.go`）：夹具强制 ID 域分离（`sites.id ≠ sites.org_id ≠ tenant_id`），并把 `instA.current_site_id` 指向 B 站；4 个用例覆盖网点隔离 + 红证据、商户/平台、多站点员工、跨站指名负例与漂移口径排除。
5. **文档**：`docs/spec/api/api.md` 新增 §5.12（3 个端点的路径/权限/参数/响应 + 可见性口径表 + `current_site_id` 禁用理由）；`docs/cases/instrument-loss.md` LS-04 补「可见范围」，角色矩阵补网点/平台范围。
6. **证据生成器**：`scripts/verify-2515.sh`（r1–r6 + gate），每条首行 provenance，重命令经 heavy-run。

### 涉及文件

| 文件 | 变更 |
|---|---|
| `backend/handlers/instrument_loss.go` | `List` 按乐器可见性收敛（复用 `applyInstrumentListScope`） |
| `backend/handlers/instrument_loss_2515_test.go` | 新增 4 个用例（R1–R4） |
| `scripts/verify-2515.sh` | Evidence Contract 运行器 r1–r6 + gate |
| `docs/spec/api/api.md` | 新增 §5.12（端点 + 可见性口径） |
| `docs/cases/instrument-loss.md` | LS-04 可见范围 + 角色矩阵 |
| `docs/evidence/2515/*` | plan / summary / changed-files / R1–R6 产物 / evidence.json / gate 报告 |

### Verification Matrix（7/7）

- R1 网点隔离主诉 + 红证据 → PASS → `docs/evidence/2515/R1-site-scope.txt`
- R2 商户 / 平台不回归 → PASS → `docs/evidence/2515/R2-merchant-wide.txt`
- R3 多站点员工（#1911）不回归 → PASS → `docs/evidence/2515/R3-multisite.txt`
- R4 负例：跨站点指名 + 漂移口径排除 → PASS → `docs/evidence/2515/R4-negative.txt`
- R5 既有丢失套件回归 → PASS → `docs/evidence/2515/R5-regression.txt`
- R6 构建 + 同类扫描 → PASS → `docs/evidence/2515/R6-build-scan.txt`
- R7 pre-ready gate（四源 + 定向测试 + 构建） → PASS → `docs/evidence/2515/pre-ready-2515.json`

### Evidence Contract

- R1 → PASS → `docs/evidence/2515/R1-site-scope.txt`
- R2 → PASS → `docs/evidence/2515/R2-merchant-wide.txt`
- R3 → PASS → `docs/evidence/2515/R3-multisite.txt`
- R4 → PASS → `docs/evidence/2515/R4-negative.txt`
- R5 → PASS → `docs/evidence/2515/R5-regression.txt`
- R6 → PASS → `docs/evidence/2515/R6-build-scan.txt`
- R7 → PASS → `docs/evidence/2515/pre-ready-2515.json`
- Evidence index: `docs/evidence/2515/evidence.json`（schema_version=1，build=1.0.254dbaecf，commit=254dbaecf76bfc0b506ebf2c6a4f7af037b0b484，dirty=false，7 行各 1 条）

### G0 Golden Acceptance

| 行 | artifact | 关键断言 | 负例 |
|---|---|---|---|
| R1 | `docs/evidence/2515/R1-site-scope.txt` | `R1_OK`、`SITE_A_IDS=[recA]`、`CROSS_SITE_LEAK=0`、`OLD_PREDICATE_LEAK=2`、`RED_CONFIRMED=1` | 含 recB → FAIL |
| R2 | `docs/evidence/2515/R2-merchant-wide.txt` | `R2_OK`、`MERCHANT_SEES=2`、`PLATFORM_SEES=2` | 只见 1 条 → FAIL |
| R3 | `docs/evidence/2515/R3-multisite.txt` | `R3_OK`、`MULTISITE_SEES=2` | 只见 1 条 → FAIL |
| R4 | `docs/evidence/2515/R4-negative.txt` | `R4_OK`、`CROSS_INSTRUMENT_ROWS=0`、`SAME_SITE_ROWS=1`、`DRIFT_SCOPE=1` | 跨站返回 >0 或 DRIFT_SCOPE=0 → FAIL |
| R5 | `docs/evidence/2515/R5-regression.txt` | `R5_OK`、`REGRESSION_PASS=1`、`FAIL_COUNT=0` | 任一既有用例 FAIL → FAIL |
| R6 | `docs/evidence/2515/R6-build-scan.txt` | `R6_OK`、`BUILD_RC=0`、`LIST_TENANT_ONLY=0`、`LIST_USES_SHARED_SCOPE=1`、`ROLE_GATE_PARITY=1`、`CURRENT_SITE_ID_IN_LIST=0` | tenant-only / 漏角色 / 引用 current_site_id → FAIL |
| R7 | `docs/evidence/2515/pre-ready-2515.json` | `GATE_RESULT=PASS`、每行 `pass=true`、`dirty=false`、P13 在 `origin/issue-2515` | 任一 P 失败 → 不得 ready |

### 红/绿实证

- 红（修复前谓词）：同夹具下 `OLD_PREDICATE_LEAK=2` —— `tenant_id` only 的查询会返回 A、B 两站记录（`RED_CONFIRMED=1` 要求同时「旧谓词=2」且「新查询无泄漏」）。
- 绿（修复后）：网点 A 账号 `SITE_A_IDS=[recA]`、`CROSS_SITE_LEAK=0`；`R6` 的 `LIST_TENANT_ONLY=0` + `LIST_USES_SHARED_SCOPE=1` 固定代码形态。
- 漂移负例：`instA.current_site_id` 指向 B 站，同站查询仍返回 recA（`DRIFT_SCOPE=1`）→ 证明判定未用 `current_site_id`。

### 契约三向核对

- Plan / API：`GET /api/instrument-loss?instrument_id=&open=` 返回 `{list,total}`（结构不变）；网点级按归属站点收敛，商户/平台全量。
- docs：`docs/spec/api/api.md` §5.12 已登记三端点与可见性口径表；`docs/cases/instrument-loss.md` LS-04 + 角色矩阵同步。
- handler：`instrument_loss.go` 的 `List` 用 `applyInstrumentListScope`（与乐器列表同源）；前端零改动（返回结构不变）。

### 证据来源

| session | repo | build_version | fix commit | artifact |
|---|---|---|---|---|
| —（本机 worktree `/tmp/opencode/tuneloop/wt-2515`） | /home/coder/tuneloop | 1.0.254dbaecf | 254dbaecf | `docs/evidence/2515/R1-site-scope.txt` … `docs/evidence/2515/R6-build-scan.txt` |

### 人工 / 部署侧

- 生产数据抽样（`instruments.site_id IS NULL` 占比、跨站点丢失记录数）不在本单（D6，dev 无样本）：责任方 Owner/部署侧，回执要求：抽样 SQL 结果 + 「历史 NULL 站点台账是否补录」结论。

### 依赖顺序 / 验收规则

- `## Dependencies`：无硬依赖；写侧 `loadLossInstrument` 留给 #2510（D3）。
- 未修改任何验收工具/规则；`scripts/verify-2515.sh` 为本 Issue 新增运行器。
- 未新增列 / 未写 migration（D4）；`site_id IS NULL` fail-closed（D5）。

Model: DeepSeek V4.1 Flash 1017
