## Accepted Plan — #2477 网点名称归一化（v2，按 Owner 裁 B 收缩：不预检、不加本地唯一索引）

> **v2 说明**：v1 建立在本单早期「本地预检 + 本地唯一索引」的裁定上；Owner 2026-11-26 裁 **B**（重名唯一权威 = IAM `CreateOrganization`，本地不查缓存预检）后，R2 的 `IAM_CALLS=0`、R5 的 `findSiteBySimilarName` 预检、R7 的 `uq_sites_tenant_name` 迁移全部成为**越权实现**。v2 删除这三项，只保留「归一化」。

### 变更点

1. **唯一归一化入口** `NormalizeSiteName(s string) string`（`backend/handlers/site_name_2477.go`）：
   - `strings.TrimSpace` → 连续空白折叠为单空格（含 `\u3000` 全角空白、`\t`、`\n`）→ 内部不保留首尾空白；
   - **不做**大小写折叠（大小写差异由 IAM 组织名唯一性判定，属对方语义）。
2. **CreateSite**：`req.Name` 归一化后再写 IAM 与本地 `sites.name`（两处**同串**，防 `sites.name ≠ organizations.name` 漂移）；归一化后为空 → `400/40002`「网点名称不能为空」。
3. **UpdateSite**：沿用 #2457 的 `strings.TrimSpace` 但改为调用同一函数（避免两套规则）；本地同名预检（#2457 已合入）保持不变（`IAM_CALLS=0`），本单只替换归一化实现。
4. **只读近似重名清单**：`scripts/site-name-similar-2477.sh`（无副作用）输出「归一化后同串」的存量行，供 ops 参考，不在服务端参与判定。
5. **文档**：`docs/spec/api/api.md` §4.5.1 增注「名称入库前归一化（trim + 空白折叠）；重名判定权威 = IAM」。

### Verification Matrix

| 变更点 | 文件/组件 | 入口 | 触发状态 | 平台 | 期望 | 证据 |
|---|---|---|---|---|---|---|
| 归一化纯函数 | `backend/handlers/site_name_2477.go` | `go test ./handlers -run TestNormalizeSiteName2477` | `" 海淀店 "`、`"海淀\u3000\u3000店"`、`"海淀\t店"`、`"   "` | backend（单元） | `NORM_TRIM=1`、`NORM_FULLWIDTH=1`、`NORM_TAB=1`、`NORM_EMPTY=1`、`NORM_CASE_KEPT=1` | R1 |
| CreateSite 归一化写两处 | `backend/handlers/site.go` | `POST /api/merchant/sites` | `{"name":"  海淀\u3000店  "}` | backend（集成） | `IAM_NAME_TRIMMED=1`、`LOCAL_NAME_TRIMMED=1`、`NAMES_EQUAL=1`、`IAM_CALLS=1`（**不预检**） | R2 |
| 空名拒绝 | 同上 | 同上 | `{"name":"   "}` | backend | `HTTP=400`、`CODE=40002`、`MSG_ZH=1`、`IAM_CALLS=0`（归一化后为空，不调 IAM） | R3 |
| UpdateSite 共用归一化 | `backend/handlers/site.go` | `PUT /api/merchant/sites/:id` | `{"name":" 新 店 "}` | backend | 本地与 IAM 均写归一化名；`IAM_CALLS=0`（#2457 本地预检保持） | R4 |
| 只读近似清单 | `scripts/site-name-similar-2477.sh` | 脚本 | 预生产只读 | ops | `READONLY=1`、输出「归一化同串」分组 | R5 |
| 构建 + gofmt | `backend` | `go build ./...` / `gofmt -l` | 修后树 | backend | `GO_BUILD_OK=1`、`GOFMT_DIFF=0` | R6 |

### Input Truth & Tool Preflight

- **权威口径**：Owner 2026-11-26 裁 B（IAM 判重、不预检）+ #2455 Rev.6（`IAM_CALLS=1`、DB 唯一索引为存储层兜底）。
- **代码现状**（`origin/develop`）：`site.go:331` 与 `:487` 原样写 `req.Name`；`site.go:604` UpdateSite 已 `TrimSpace`；`UpdateSite` 本地同名预检已存在（#2457）。
- **不做**：大小写折叠、Unicode NFC 全量规范化（仅空白类）；本地预检；新建唯一索引。
- **工具**：单元测试可直接跑（纯函数）；集成测试走 `testfixtures`；清单脚本只读。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 判重权威 | **IAM**（裁 B）；本地**不预检**、**不加**唯一索引 |
| D2 | 归一化范围 | trim + 空白折叠（含全角空白/制表/换行）；**不做**大小写与全量 NFC |
| D3 | 写库一致性 | 归一化后**同一字符串**写 IAM 与本地 `sites.name` |
| D4 | 空名 | 归一化后为空 → `400/40002`，且**不调用 IAM** |
| D5 | 存量 | 只读清单，不自动改存量名 |

### Out of Scope

- 本地重名预检与本地唯一索引（裁 B 已否决；存储层兜底索引归 #2455）。
- 大小写/Unicode 全量规范化。
- 存量网点改名（只读清单输出后由 ops 决定）。

### G0 Golden Acceptance

| 行 | fixture | 执行 | 产物 | 断言 | 负例 |
|---|---|---|---|---|---|
| R1 | 4 组字符串 | `go test ./handlers -run TestNormalizeSiteName2477 -v` | `docs/evidence/2477/R1-normalize.txt` | `NORM_TRIM=1`、`NORM_FULLWIDTH=1`、`NORM_TAB=1`、`NORM_EMPTY=1`、`NORM_CASE_KEPT=1` | 任一规则缺失、或误做大小写折叠 → FAIL |
| R2 | 既有商户 + 归一化名 | `POST /api/merchant/sites` | `docs/evidence/2477/R2-create.txt` | `IAM_NAME_TRIMMED=1`、`LOCAL_NAME_TRIMMED=1`、`NAMES_EQUAL=1`、`IAM_CALLS=1` | 本地/IAM 名字不一致、或出现本地预检（`IAM_CALLS=0`）→ FAIL |
| R3 | 全空白名 | 同上 | `docs/evidence/2477/R3-empty.txt` | `HTTP=400`、`CODE=40002`、`MSG_ZH=1`、`IAM_CALLS=0` | 调了 IAM 或返回 500 → FAIL |
| R4 | 既有网点改名（含空白） | `PUT /api/merchant/sites/:id` | `docs/evidence/2477/R4-update.txt` | 归一化名落两处、`IAM_CALLS=0`（#2457 预检） | 两处不一致或误调 IAM → FAIL |
| R5 | 预生产只读 | `bash scripts/site-name-similar-2477.sh` | `docs/evidence/2477/R5-similar.txt` | `READONLY=1` 且输出分组 | 脚本写库 → FAIL |
| R6 | 修后树 | `bash scripts/verify-2477.sh r6` | `docs/evidence/2477/R6-build.txt` | `GO_BUILD_OK=1`、`GOFMT_DIFF=0` | 构建失败/gofmt 差异 → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2477.sh r1 > docs/evidence/2477/R1-normalize.txt` | `docs/evidence/2477/R1-normalize.txt` | exit 0 且含 `NORM_TRIM=1` `NORM_FULLWIDTH=1` `NORM_TAB=1` `NORM_EMPTY=1` `NORM_CASE_KEPT=1` | PASS（5 项 token） | 规则缺失/大小写被折叠 → FAIL | dev |
| R2 | `bash scripts/verify-2477.sh r2 > docs/evidence/2477/R2-create.txt` | `docs/evidence/2477/R2-create.txt` | exit 0 且含 `IAM_NAME_TRIMMED=1` `LOCAL_NAME_TRIMMED=1` `NAMES_EQUAL=1` `IAM_CALLS=1` | PASS（4 项 token） | 名字不一致/本地预检 → FAIL | dev |
| R3 | `bash scripts/verify-2477.sh r3 > docs/evidence/2477/R3-empty.txt` | `docs/evidence/2477/R3-empty.txt` | exit 0 且含 `HTTP=400` `CODE=40002` `MSG_ZH=1` `IAM_CALLS=0` | PASS（4 项 token） | 调 IAM/500 → FAIL | dev |
| R4 | `bash scripts/verify-2477.sh r4 > docs/evidence/2477/R4-update.txt` | `docs/evidence/2477/R4-update.txt` | exit 0 且含 `IAM_CALLS=0` 与两处名字相等 | PASS | 误调 IAM/不一致 → FAIL | dev |
| R5 | `bash scripts/verify-2477.sh r5 > docs/evidence/2477/R5-similar.txt` | `docs/evidence/2477/R5-similar.txt` | exit 0 且含 `READONLY=1` | PASS | 写库 → FAIL | dev |
| R6 | `bash scripts/verify-2477.sh r6 > docs/evidence/2477/R6-build.txt` | `docs/evidence/2477/R6-build.txt` | exit 0 且含 `GO_BUILD_OK=1` `GOFMT_DIFF=0` | PASS（2 项 token） | 构建失败 → FAIL | dev |

### Evidence Types

| R | 类型 | 说明 |
|---|---|---|
| R1 | unit | 纯函数表驱动 |
| R2/R3/R4 | integration | handler + fake IAM（计数） |
| R5 | static | 只读脚本 |
| R6 | build | go build + gofmt |

### Artifact Provenance

- R1–R6 首行：`# build=1.0.<短码> commit=<40位> dirty=false`（三 token：`# build=1.0.` / `# commit=` / `# dirty=false`）
- `evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$`
- 产物：
  - `docs/evidence/2477/R1-normalize.txt`
  - `docs/evidence/2477/R2-create.txt`
  - `docs/evidence/2477/R3-empty.txt`
  - `docs/evidence/2477/R4-update.txt`
  - `docs/evidence/2477/R5-similar.txt`
  - `docs/evidence/2477/R6-build.txt`

### Dependencies

### 依赖与关联说明（非硬依赖）

- #2455（同商户唯一索引 + 编辑预检）：其 R2 已锁定 `IAM_CALLS=1`；本单不与其冲突。
- #2457（UpdateSite 归一化顺序）：本单只是把 `TrimSpace` 换成统一函数。

### Workload

- Basis: 1 个纯函数文件 + 1 个 handler 两处调用 + 1 个只读脚本 + 1 份文档 + 1 个证据 harness（6 行）；无迁移、无跨仓。

### Files（变更文件清单）

- `backend/handlers/site_name_2477.go`
- `backend/handlers/site_name_2477_test.go`
- `backend/handlers/site.go`
- `scripts/site-name-similar-2477.sh`
- `scripts/verify-2477.sh`
- `docs/spec/api/api.md`
- `docs/evidence/2477/plan.md`
- `docs/evidence/2477/R1-normalize.txt`
- `docs/evidence/2477/R2-create.txt`
- `docs/evidence/2477/R3-empty.txt`
- `docs/evidence/2477/R4-update.txt`
- `docs/evidence/2477/R5-similar.txt`
- `docs/evidence/2477/R6-build.txt`

