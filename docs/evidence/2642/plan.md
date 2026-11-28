## Accepted Plan — #2642 P4-S1：`tenant_id` 引用/DB 依赖清点 + 逐表机械分类 + 零引用门禁（只读）｜Rev.2（双口径：INV_SNAPSHOT + INV_LIVE）
> **Rev.2 说明（采纳 Packet option C「双口径」）**：计划把「52 个模型」与「52 张快照表」当成同一集合，实际是 **49 交集 + 3 张 `bak_` 无模型 + 3 个无快照表模型**，且 P2 快照早于 `order_payment_allocations`（有 `tenant_id` 列与 `TenantID` 字段）→ 硬编码 `INV_TABLES=52` 与 `CLASS_SUM_CHECK=52` 必有一条违反。本版改为：
> ① 同时输出 **`INV_SNAPSHOT=52`**（P2 对齐，`INV_PRE_ONLY=3`）与 **`INV_LIVE=<实测>`**（含 post-snapshot 新表），TSV 以 **live** 为准，每行 `env ∈ {pre, pre-only, post-snapshot}`；
> ② 三方校验改为 **`MODEL_STRUCTS = MODEL_TABLE_ROWS + MODEL_NO_TABLE + MODEL_POST_SNAPSHOT`**（52 = 49 + 1 + 2），3 个模型名与依据写入计划；
> ③ G0/合同 R1 断言改为 `INV_SNAPSHOT=52`、`INV_PRE_ONLY=3`、`INV_LIVE = INV_SNAPSHOT + POST_SNAPSHOT_TABLES - DROPPED_IN_SNAPSHOT`、`CLASS_SUM_CHECK = INV_LIVE`、`DOC_TABLES = INV_LIVE`。

### 事实核对（代码为据，2026-11-28，HEAD `af7eacdc4`）

| # | 事实 | 命令 / 位置 |
|---|---|---|
| 1 | 后端非测试 Go 含 `tenant_id` 字面量 **909 处 / 555 行 / 89 文件**（472 行在 SQL 字符串内） | `grep -roh "tenant_id" backend --include="*.go" \| wc -l`；`grep -rl … \| grep -v _test.go \| wc -l` |
| 2 | 后端测试含字面量 **332 处 / 84 文件**；模型 `TenantID` 字段 **52 处** | `grep -roh … --include="*_test.go"`；`grep -c "TenantID " backend/models/*.go` |
| 3 | 含 `TenantID` 的模型 struct = **52**；其中**同时含 `OrgID`** = **22**（与审计基线 §3「同时有 org_id」22 张一致） | 结构体解析脚本（见 §Input Truth 工具预检） |
| 4 | 通用租户机制贡献**与表无关的基线噪声**：`backend/database/db.go` 的 `LookUpField("TenantID")` / `SetTenantID` / `addTenantScope` + `middleware` claim 传递，被"文件级提及"规则计入**每一张表** → 按文件归属会把每张表都算成 50+ 引用（实测 `brand_configs` 文件级 51 处，其中真实表级仅 0） | `grep -n "TenantID" backend/database/db.go` |
| 5 | 表规模三环境不一致：pre **52**（含 3 张 `bak_2211_*`）/ prod **49**（无 bak_）/ dev 44 | `docs/ops/tenant-id-backfill-p2.md` §1 |
| 6 | DB 侧对象：`get_default_tenant()`（`013_add_brand_model_properties.up.sql`）为部分列提供 `DEFAULT`；无视图；触发器仅 `updated_at`；**无** `CREATE INDEX *tenant*` 命名的历史迁移（索引多由 GORM tag 生成） | `grep -rniE "create (or replace )?(view\|function\|trigger)" backend/database/migrations` |
| 7 | 5 个索引名被 gorm tag 引用：`idx_payment_sessions_tenant`、`idx_payment_tenant`、`idx_tenant_code`、`idx_setting_tenant_key`、`idx_mm_tenant` | `grep -rn "index:" backend/models/*.go \| grep -i tenant` |
| 8 | 后续批次的最终放行权威不是 grep 而是**编译器**：删 `TenantID` 字段后 `go build ./...` 会枚举出全部字段级依赖（`backend/database/db.go:554` 的 `validateModelColumns` 同时要求列存在，故"字段删除可编译"必然蕴含"该表列可删"） | `backend/database/db.go:554`、`backend/models/*.go` |

**结论**：本单的产出不是"再写一份表清单"，而是把「哪张表能独立处置、影响面多大」变成**可复跑、可断言、归属规则显式**的机器输入。缺少这一层，P4 的每一批都会退回"按表名猜语义"（`site_members.tenant_id` 在中转网点必须 = 平台租户，见 #2552/#2554；`merchant_members`/`repair_requests` 已被 P2 回填为商户 org）。

### 变更点

#### 1. 权威表清单 `docs/ops/tenant-id-p4-tables.txt`（52 行，单一来源）

格式：`<table>|<source>`，`source ∈ {model, backup-only}`。内容 = 模型 struct 名 → 表名（GORM 默认复数，需与 `TableName()` 覆写核对）+ 3 张无模型的 `bak_2211_*`。

脚本必须**交叉校验**（任一不满足 → `exit 2`，禁止静默跳过）：

- 清单行数 = `INV_TABLES`，且与 `docs/ops/tenant-id-backfill-p2.md` §1 的 pre 52 表集合逐行相等；
- 含 `TenantID` 的模型 struct 数 = 清单中 `source=model` 行数（当前 **52**）；
- `bak_*` 行全部标 `backup-only`。

#### 2. 扫描器 `scripts/tenant-id-refscan.sh`（只读、无 DB、确定性）

每表一行 TSV（列固定，顺序固定）：

```
table  source  model  has_org_id  sql_refs  field_refs  test_refs  frontend_refs  doc_refs  db_default  index_names  class  env
```

归属规则（写入脚本头部注释，禁止按表名白名单）：

| 计数 | 规则（机械） |
|---|---|
| `sql_refs` | 非测试 Go 中，**同一行（±3 行窗口）**同时出现 `tenant_id` 与该表的引号表名（`"<table>"`） |
| `field_refs` | 非测试 Go 中，**同一结构体字面量块（±5 行窗口）**同时出现模型名与 `TenantID`（`X.TenantID` / `X{TenantID:` / `TenantID:`） |
| `test_refs` | `*_test.go` 中 `sql_refs + field_refs` 同规则合计 |
| `frontend_refs` | `frontend-pc/src`、`frontend-mobile/src` 中 `tenant_id` 出现次数（全局，标注为**不归属**） |
| `doc_refs` | `docs/**` 中提到该表且含 `tenant_id` 的文件数（全局参考） |
| `plumbing_refs` | **单列汇总（不归属任何表）**：`backend/database/db.go`、`backend/middleware/*`、`backend/models/*.go` 内的 `tenant_id`/`TenantID` 机制引用总数（事实核对第 4 条的噪声源，单独报告以免污染表级计数） |

分类 `class`（固定规则）：

| class | 判定 |
|---|---|
| `backup` | `source=backup-only`（表名 `^bak_`） |
| `redundant` | `has_org_id=true` |
| `metadata` | `has_org_id=false` 且 `sql_refs + field_refs == 0` |
| `anchor-candidate` | `has_org_id=false` 且 `sql_refs + field_refs > 0` |

模式：

| 模式 | 行为 |
|---|---|
| `--tsv`（默认） | 52 行数据 + 末尾 `# KEY=VALUE` token 段：`INV_TABLES`、`INV_PRE_ONLY`、`CLASS_BACKUP`、`CLASS_REDUNDANT`、`CLASS_METADATA`、`CLASS_ANCHOR`、`PLUMBING_REFS`、`SQL_REFS_TOTAL`、`FIELD_REFS_TOTAL`、`LIVE_LITERAL_BASE` |
| `--json` | 同上语义的 JSON（供后续工具消费） |
| `--table <T> --require-zero` | 该表 `sql_refs + field_refs == 0` 才 `exit 0`；否则 `exit 1` 并打印命中行 |
| `--assert-no-live-literal` | 全仓 `tenant_id` 字面量仅允许：`tenant_id_deprecated`、`backend/database/migrations/**`、`docs/**`、本组脚本自身；其余命中 → `exit 1`（供 #2643 与后续批次复用） |
| `--check-doc <path>` | 校验 inventory 文档覆盖 52 表 + 含两类清单（`可立即处置` / `需语义裁定`），输出 `DOC_TABLES`、`DOC_MISSING`、`DOC_LISTS` |

#### 3. 清单文档 `docs/ops/tenant-id-p4-inventory.md`

- 52 行矩阵（列同 TSV）+ 分类规则 + 计数口径（含 `plumbing_refs` 噪声说明与"为何不用文件级归属"的实测反例：`brand_configs` 文件级 51 / 表级 0）
- **「可立即处置」清单**：`backup ∪ metadata ∪ (redundant ∧ sql_refs+field_refs==0)`，逐表列出 → 作为 #2643 的批 1 输入
- **「需语义裁定」清单**：`anchor-candidate` 逐表标注语义候选（商户 org 锚 / 本地租户作用域 / 平台租户 / 创建者元数据）+ 依据（代码位置或 P2 报告行）
- **DB 侧待实测项**：索引名/唯一约束/`DEFAULT` 表达式在 dev/pre/prod 的差异 → 给 ops 只读查询模板（`information_schema.columns` / `pg_indexes` / `pg_constraint`），本单不执行
- **扫描盲区登记**：动态拼列名（`fmt.Sprintf`）、反射/`LookUpField`、JSON tag、DB 侧 DEFAULT/触发器/视图、其他仓库 → 由"编译器为准"规则兜底（变更点 1 第 8 条事实）

#### 4. 证据生成器 `scripts/verify-2642.sh`

`r1`–`r5` 子命令，每个产物首行写 provenance：`# build=1.0.<fix 短码> # commit=<40 位 sha> # dirty=false` + 次行 `GENERATED_AT_HEAD=<sha>`（`FIX_COMMIT` 可覆盖）；重命令（`go build`）经 `~/.config/opencode/scripts/heavy-run.sh` 取锁。`r5` 另断言 `origin/develop..HEAD` 下 `backend/`、`frontend-pc/`、`frontend-mobile/` 改动文件数均为 0（本单只读）。

### 既有断言影响（0 翻转）

| 检查 | 结果 | 依据 |
|---|---|---|
| 本单触及 Go 测试断言 | **0** | diff 仅 `scripts/` + `docs/`（R5 断言 `BACKEND_DIFF_FILES=0`） |
| `scripts/tenant-audit-coverage.sh`（#2246 基线覆盖） | **0 影响** | 只读 `tenant-id-audit-files.csv` + `tenant-id-audit.md`，本单不改这两份 |
| `plan-lint.sh` / `pre-ready-check.sh` 等共享校验器 | **0 改动** | D9 |

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| R1 表清点 TSV（**双口径**）+ token 段 | `scripts/tenant-id-refscan.sh`、`docs/ops/tenant-id-p4-tables.txt` | `bash scripts/tenant-id-refscan.sh --tsv` | 修后工作树（只读仓库，无 DB） | repo（无 DB 连接） | 数据行 = `INV_LIVE`；`# INV_SNAPSHOT=52`、`# INV_PRE_ONLY=3`、`# INV_LIVE=<实测>`、`# POST_SNAPSHOT_TABLES=<实测>`、`# CLASS_SUM_CHECK=INV_LIVE`、`# MODEL_STRUCTS=MODEL_TABLE_ROWS+MODEL_NO_TABLE+MODEL_POST_SNAPSHOT`；每行 `env` ∈ {pre,pre-only,post-snapshot} | R1 |
| R2 清单文档覆盖 + 两类清单 | `docs/ops/tenant-id-p4-inventory.md` | `bash scripts/tenant-id-refscan.sh --check-doc docs/ops/tenant-id-p4-inventory.md` | 修后工作树 | 文档 | `DOC_TABLES=52`、`DOC_MISSING=0`、`DOC_LISTS=2`、`DOC_CLASS_COVERED=1`、`DOC_DB_BACKLOG=1` | R2 |
| R3 确定性（两次运行逐字节一致） | `scripts/tenant-id-refscan.sh`、`scripts/tests/test_tenant_id_refscan.sh` | `bash scripts/tests/test_tenant_id_refscan.sh determinism` | 连续两次运行 | repo | `DETERMINISTIC=1`、`DIFF_LINES=0`、`OUTPUT_HAS_TIMESTAMP=0`、`OUTPUT_HAS_ABS_PATH=0` | R3 |
| R4 零引用门禁红绿 + 全仓字面量门禁基线 | `scripts/tenant-id-refscan.sh`、`scripts/tests/test_tenant_id_refscan.sh` | `bash scripts/tests/test_tenant_id_refscan.sh gate` | `tmp/` 夹具存在 / 移除 | repo | `GATE_RED_EXIT=1`、`GATE_GREEN_EXIT=0`、`LIVE_LITERAL_RED_EXIT=1`、`FIXTURE_CLEANED=1` | R4 |
| R5 构建通过 + 本单只读（零 backend/frontend 改动） | `scripts/verify-2642.sh` | `bash scripts/verify-2642.sh r5` | 修后工作树 vs `origin/develop` | backend / frontend-pc / frontend-mobile | `GO_BUILD_EXIT=0`、`BACKEND_DIFF_FILES=0`、`FRONTEND_DIFF_FILES=0`、`TABLES_FILE_LINES=52` | R5 |

## Input Truth & Tool Preflight

- **Source of truth（按优先级）**：① `backend/models/*.go`（52 个含 `TenantID` 的 struct，字段级权威）；② `backend/database/migrations/**/*.sql`（DDL：`org_id` 列、`DEFAULT`、约束）；③ `docs/ops/tenant-id-backfill-p2.md` §1/§3（pre 52 / prod 49 / dev 44 实测行数与分类）；④ `docs/ops/tenant-id-audit.md` §3（人工基线，仅对照，不作为计数来源）。
- **差异核对（必须先跑，冲突即停）**：
  - 模型 struct 数（52）↔ 清单 `source=model` 行数（52）↔ P2 pre 表数（52）三者相等；不等 → `exit 2` 并打印差集；
  - 模型 `TenantID`+`OrgID` 同存数（22）↔ 审计 §3「同时有 org_id」22 张：不等 → 文档必须逐表说明差异（如 `ConfirmationSession` 无表），不得静默取其一。
- **损坏输入拒绝规则**：清单缺行/多行、表名无法映射到模型或 `bak_*`、`TableName()` 覆写与默认复数不一致、迁移 DDL 无法解析 → **显式 `exit 2`**（禁止降级为警告后继续）。
- **工具预检（dry-run/report 义务）**：
  - 扫描器首版必须先 `--json` 输出并人工核对 5 张抽样表（`brand_configs`、`orders`、`site_members`、`merchant_members`、`bak_2211_users`）的归属计数是否与 `grep` 手查一致，把对照结果写入 inventory 文档「口径校验」小节；
  - **已知盲区（必须登记在文档，不得声称"全覆盖"）**：动态拼列名（`fmt.Sprintf("... %s", col)`）、反射字段名（`LookUpField("TenantID")`）、JSON tag（`json:"tenant_id"`）、DB 侧对象（`DEFAULT`/触发器/视图）、其他仓库（beaconiam 无此列，但脚本不跨仓）；
  - **兜底口径**：字段级最终判据是编译器（删除字段后 `go build ./...`），字面量级是 `--assert-no-live-literal`；本单静态矩阵只用于**排序与分批**，不用于放行 #2643。
- **手工步骤**：DB 侧索引/唯一约束/`DEFAULT` 在 dev/pre/prod 的实际差异需 ops 只读查询补全（模板随 inventory 文档交付）；本单不连库。

## Decision Lock

| # | 决策 | 责任人 | 锁定条件 | 未锁定时处理 |
|---|---|---|---|---|
| D1 | 放行权威 = 编译器（字段级）+ 全仓字面量门禁（`--assert-no-live-literal`）；静态矩阵仅排序/分批 | architect | 本计划 | 已锁定 |
| D2 | 归属规则采用**行/块窗口**（`sql_refs` ±3 行、`field_refs` ±5 行）+ `plumbing_refs` 单列，禁止文件级归属（实测会把 `brand_configs` 算成 51） | architect | 本计划 | 已锁定 |
| D3 | 分类四桶规则固定（`backup`/`redundant`/`metadata`/`anchor-candidate`），禁止表名白名单 | architect | 本计划 | 已锁定 |
| D4 | 本单只读：不执行 DDL/DML、不改 `backend/**`、`frontend-*/**`、迁移 | architect | 本计划 | 已锁定 |
| D5 | 表清单以 `docs/ops/tenant-id-p4-tables.txt` 为单一来源并做三方交叉校验 | dev | 实现时 | 不一致 → `exit 2` |
| D6 | 三环境差异（prod 缺 3 张 `bak_*`）用 `env` 列标注，不合并成 49 行 | dev | 实现时 | 漏标 → R1 FAIL |
| D7 | `--assert-no-live-literal` allowlist = `tenant_id_deprecated`、`backend/database/migrations/**`、`docs/**`、本组脚本；**不含** `backend/models/**` 与前端 | architect | 本计划 | 已锁定 |
| D8 | 本单不连 DB；DB 侧索引/默认值/约束差异交 ops 只读模板（模板随文档交付） | ops | 文档交付时 | 缺失 → R2 的 `DOC_DB_BACKLOG!=1` |
| D9 | 不修改任何共享校验器（`plan-lint.sh`、`pre-ready-check.sh`、`matrix_parser.sh`、`work_deps.sh` 等） | architect | 本计划 | 违规 → 审计 REJECT |
| D10 | `anchor-candidate` 表逐表语义裁定**不在本单**（含 `site_members` 平台租户作用域、`merchant_members`/`repair_requests` 的 org 锚）→ 归后续批次 | architect | 本计划 | 本单若尝试裁定 → 审计 REJECT |

## Out of Scope

- 任何 DDL/DML、迁移文件、`backend/**`、`frontend-pc/**`、`frontend-mobile/**` 改动（本单只读；R5 断言三者 diff 为 0）
- `anchor-candidate` 表的语义裁定与处置（后续批次；本单只输出候选与依据）
- `bak_2211_*` 的实际 `DROP`、以及任何表的 `RENAME` / `DROP COLUMN`（属 #2643 与后续批次）
- DB 实测（索引名/`DEFAULT`/约束/行数）——交 ops 只读模板，本单不连库
- `AGENTS.md` / `docs/spec/*` / `docs/ops/account-lifecycle.md` 的结构件同步（父任务 #2256 出口条件统一处理，避免半程口径）
- 其他仓库（beaconiam）扫描

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 仓库静态输入：P2 表清单（52）+ 含 `TenantID` 的模型 struct + 迁移文件 | `bash scripts/tenant-id-refscan.sh --tsv`（无 DB、无网络） | `docs/evidence/2642/R1-inventory.tsv` | `# INV_SNAPSHOT=52`；`# INV_PRE_ONLY=3`；`# INV_LIVE = INV_SNAPSHOT + POST_SNAPSHOT_TABLES - DROPPED_IN_SNAPSHOT`；`# CLASS_SUM_CHECK = INV_LIVE`；`# MODEL_STRUCTS = MODEL_TABLE_ROWS + MODEL_NO_TABLE + MODEL_POST_SNAPSHOT`；每行 `env`/`class` 非空且属枚举 | 口径漂移（`INV_LIVE` 与公式不等）、把已 DROP 表计入、`env`/`class` 空值、脚本连库 → 任一即 FAIL |
| R2 | 上一步生成的 inventory 文档（52 行矩阵 + 两类清单 + DB 待实测段） | `bash scripts/tenant-id-refscan.sh --check-doc docs/ops/tenant-id-p4-inventory.md` | `docs/evidence/2642/R2-doc-coverage.txt` | `DOC_TABLES=52`；`DOC_MISSING=0`；`DOC_LISTS=2`；`DOC_CLASS_COVERED=1`（每行有 class）；`DOC_DB_BACKLOG=1`（ops 模板段存在） | 文档缺任一表；缺「可立即处置」或「需语义裁定」段；某行无 class；无 ops 只读模板 → 任一即 FAIL |
| R3 | 同一工作树连续两次运行（间隔无输入变化） | `bash scripts/tests/test_tenant_id_refscan.sh determinism` | `docs/evidence/2642/R3-determinism.txt` | `DETERMINISTIC=1`；`DIFF_LINES=0`；`OUTPUT_HAS_TIMESTAMP=0`；`OUTPUT_HAS_ABS_PATH=0` | 两次输出有差异；输出含时间戳/绝对路径/随机顺序 → 任一即 FAIL |
| R4 | `tmp/refscan-fixture/` 内含 `tenant_id` 字面量与表名的临时 go 夹具；清理后为洁净树 | `bash scripts/tests/test_tenant_id_refscan.sh gate` | `docs/evidence/2642/R4-gate-redgreen.txt` | `GATE_RED_EXIT=1`（夹具在 → 该表 `--require-zero` 失败）；`GATE_GREEN_EXIT=0`（清理后通过）；`LIVE_LITERAL_RED_EXIT=1`（全仓门禁当前不通过，负例口径）；`FIXTURE_CLEANED=1`（`git status --porcelain tmp/refscan-fixture` 为空） | 夹具存在却 `exit 0`（门禁失灵）；清理后仍 `exit 1`；夹具残留进工作树 → 任一即 FAIL |
| R5 | 修后工作树 vs `origin/develop`（分支 `issue-2642`） | `bash scripts/verify-2642.sh r5`（内部经 `heavy-run.sh` 跑 `cd backend && go build ./...`） | `docs/evidence/2642/R5-build.txt` | `GO_BUILD_EXIT=0`；`BACKEND_DIFF_FILES=0`；`FRONTEND_DIFF_FILES=0`；`TABLES_FILE_LINES=52` | 构建失败；`backend/` 或 `frontend-*/` 出现改动（本单应只读）；清单行数 ≠52 → 任一即 FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2642.sh r1 > docs/evidence/2642/R1-inventory.tsv` | `docs/evidence/2642/R1-inventory.tsv` | exit 0 且含 `# INV_SNAPSHOT=52` `# INV_PRE_ONLY=3` `# INV_LIVE=` `# POST_SNAPSHOT_TABLES=` `# CLASS_SUM_CHECK=`（后三者相等且符合公式） | `INV_SNAPSHOT=52`、`INV_PRE_ONLY=3`、`CLASS_SUM_CHECK=INV_LIVE` | 口径漂移 / 已 DROP 表计入 / 分类空值 / 脚本连库 → FAIL | dev |
| R2 | `bash scripts/verify-2642.sh r2 > docs/evidence/2642/R2-doc-coverage.txt` | `docs/evidence/2642/R2-doc-coverage.txt` | exit 0 且含 `DOC_TABLES=52`、`DOC_MISSING=0`、`DOC_LISTS=2`、`DOC_CLASS_COVERED=1`、`DOC_DB_BACKLOG=1` | 五项全为期望值 | 文档缺表/缺清单段/缺 class/缺 ops 模板 → FAIL | dev |
| R3 | `bash scripts/verify-2642.sh r3 > docs/evidence/2642/R3-determinism.txt` | `docs/evidence/2642/R3-determinism.txt` | exit 0 且含 `DETERMINISTIC=1`、`DIFF_LINES=0`、`OUTPUT_HAS_TIMESTAMP=0`、`OUTPUT_HAS_ABS_PATH=0` | 四项全为期望值 | 输出抖动/含时间戳或绝对路径 → FAIL | dev |
| R4 | `bash scripts/verify-2642.sh r4 > docs/evidence/2642/R4-gate-redgreen.txt` | `docs/evidence/2642/R4-gate-redgreen.txt` | exit 0 且含 `GATE_RED_EXIT=1`、`GATE_GREEN_EXIT=0`、`LIVE_LITERAL_RED_EXIT=1`、`FIXTURE_CLEANED=1`；另含 `LIVE_LITERAL_BASE=` 与 R1 同值 | 四项 + 基线一致 | 门禁红绿颠倒/夹具残留/基线漂移 → FAIL | dev |
| R5 | `bash scripts/verify-2642.sh r5 > docs/evidence/2642/R5-build.txt` | `docs/evidence/2642/R5-build.txt` | exit 0 且含 `GO_BUILD_EXIT=0`、`BACKEND_DIFF_FILES=0`、`FRONTEND_DIFF_FILES=0`、`TABLES_FILE_LINES=52` | 四项全为期望值 | 构建失败/本单改动 backend 或 frontend/清单行数≠52 → FAIL | dev |

### Artifact Provenance

- 生成器：`scripts/verify-2642.sh`（`r1`–`r5`；`r1`/`r3`/`r4` 调扫描器，`r5` 经 `~/.config/opencode/scripts/heavy-run.sh` 执行 `cd backend && go build ./...`）。
- 每个产物首行必须为 `# build=1.0.<fix 短码> # commit=<40 位 sha> # dirty=false`，次行 `GENERATED_AT_HEAD=<40 位 sha>`；锚点为本单 fix commit（`FIX_COMMIT` 可覆盖），证据必须在 fix commit **之后**生成。
- 逐条清单（全部位于 `docs/evidence/2642/`）：
  - `docs/evidence/2642/R1-inventory.tsv`
  - `docs/evidence/2642/R2-doc-coverage.txt`
  - `docs/evidence/2642/R3-determinism.txt`
  - `docs/evidence/2642/R4-gate-redgreen.txt`
  - `docs/evidence/2642/R5-build.txt`
  - `docs/evidence/2642/evidence.json`
  - `docs/evidence/2642/pre-ready-2642.json`
  - `docs/evidence/2642/plan.md`
- `evidence.json` 必含键：`schema_version`、`issue`、`build`、`commit`、`dirty`、`plan_sha256`、`environment`、`rows[]`（`id`/`command`/`exit`/`artifacts`/`assert`/`actual`/`expected`/`negative`/`negative_result`）。
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Files（变更文件清单）

- `scripts/tenant-id-refscan.sh`
- `scripts/verify-2642.sh`
- `scripts/tests/test_tenant_id_refscan.sh`
- `docs/ops/tenant-id-p4-tables.txt`
- `docs/ops/tenant-id-p4-inventory.md`
- `docs/evidence/2642/R1-inventory.tsv`
- `docs/evidence/2642/R2-doc-coverage.txt`
- `docs/evidence/2642/R3-determinism.txt`
- `docs/evidence/2642/R4-gate-redgreen.txt`
- `docs/evidence/2642/R5-build.txt`
- `docs/evidence/2642/evidence.json`
- `docs/evidence/2642/pre-ready-2642.json`
- `docs/evidence/2642/plan.md`

### Workload

- Size: M
- Basis: 3 个脚本（扫描器 + 证据生成器 + 回归测试）+ 2 份文档（52 行矩阵 + 表清单）+ 6 个证据产物；0 migration、0 业务代码、0 外部依赖、0 DB 访问
- Confidence: high

### Core Update

无（新增 ops 文档与只读脚本；`AGENTS.md` 结构件段落在父任务 #2256 出口条件中统一同步）

### Dependencies

无

*Model: DeepSeek V4.1 Flash 1017*


