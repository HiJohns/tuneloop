# 测试库生命周期（handlers）

> #2341（模板库 + fail-fast + dirty 自愈）、#2342（迁移链 fresh-install 可重放）。
> 适用于 `backend/handlers` 测试套件；生产路径不受影响。

## 初始化顺序

1. `TestMain` 解析 run 库名（`TUNELOOP_TEST_DB` 或按节点隔离的 `tuneloop_test_<node>_<hash>`）。
2. `EnsureTemplate` 确保共享模板 `tuneloop_test_template` 存在且未过期；缺失/过期时在 `pg_advisory_lock` 下重建，顺序与生产 fresh-install 同构：
   **`CREATE DATABASE` → 迁移链重放（幂等化 shim，#2342）→ AutoMigrate（仅缺表）→ PostAutoMigrate（iam_sub / 纯 SQL 表）→ seed（system default pricing template）→ 写 meta 戳**。
3. `CREATE DATABASE <run> TEMPLATE tuneloop_test_template` 秒级克隆。
4. `database.InitDB(run)` → `AssertCentsSchema`（金额列必须 bigint）→ `m.Run()`。

meta 戳（`_test_template_meta`）= 模型清单 hash + seed 指纹 + **迁移链指纹/up 文件数/最终 schema 版本**；任一项变化即判定过期并重建。

## 迁移链重放与幂等化 shim（#2342）

`backend/database/migrations/` 的 217 个 up 文件**不修改**（golang-migrate 只按版本 + dirty 追踪，没有内容校验和；改历史文件对已应用库零收益、对 merge 冲突高成本）。测试模板构建器在内存中把迁移 SQL 施加幂等化变换后整链重放，磁盘文件保持原样，生产 `migrate` 路径不引用该实现。

| 原语句 | 改写 | 目的 |
|---|---|---|
| `CREATE TABLE x` | `CREATE TABLE IF NOT EXISTS x` | 001 等历史建表与 AutoMigrate/重复重放共存 |
| `CREATE [UNIQUE] INDEX i ON t` | `CREATE [UNIQUE] INDEX IF NOT EXISTS i ON t` | 索引可重复建立 |
| 同上，但目标表尚未创建 | 包进 `DO $$ ... IF to_regclass('t') IS NOT NULL THEN EXECUTE ...` | `20260630003` 先给 `appeals` 建索引，而 `appeals` 由更晚的 `20260706002` 创建 |
| `ALTER TABLE t ...` | `ALTER TABLE IF EXISTS t ...` | 先 ALTER 后建表的历史语序 |
| `ALTER TABLE t ADD COLUMN c` | `ADD COLUMN IF NOT EXISTS c` | 列重复添加 |
| `INSERT/UPDATE/DELETE/WITH ...` 且引用表可能未建 | 包进 `DO $$ ... IF to_regclass(...) IS NOT NULL THEN EXECUTE ...` | 早期回填早于建表；空库重放时无数据可搬，跳过等价 |

最终 schema = 迁移链 ∪ AutoMigrate 缺表，与长寿命库终态一致。重放成功后 meta 记录 `migrations_hash / migrations_count / migrations_version`，日志输出：

```text
[test-template] migrations replayed: files=399 up=217 statements=1730 ... version=20261112003
```

### 失败处理（禁止静默跳过）

- 重放失败 → `FATAL: ensure template: template: build failed: ...`，**进程退出，不降级为「跳过迁移」**；报错含失败语句与 SQLSTATE，便于定位。
- 规则不覆盖的新语句形态（D5）→ 按失败语句逐例补规则（`CREATE/ALTER/索引/DML 守卫`），不得放宽为 `|| true`。
- 迁移目录缺失或为空 → `template: migrations signature` / `migration source: no .sql files` 显式报错。
- 证据模式（仅诊断）：`TUNELOOP_TEMPLATE_MIGRATION_SHIM=off` 会跳过变换按原始文件重放，用于复现 #2341 的红（`relation "appeals" does not exist`）；指纹里带 `shim=off`，不会与正常模板互相判定为未过期。

## fail-fast（金额列）

克隆后扫描所有模型内 `models.Cents` 列；任何列不是 `bigint`（例如被测试降级为 `numeric(10,2)`）→ `FATAL: cents schema drifted (expected bigint): <table.column>=<type>; rebuild the template with FORCE_TEMPLATE_REBUILD=1`。
原因：`Cents.Scan` 对 numeric 按「元」×100，会静默产生 100 倍金额。

## 清理

- 套件结束：关闭连接池 → `DROP DATABASE <run> WITH (FORCE)`；**模板保留**为共享 fixture（可随时重建）。
- 调试时设置 `KEEP_TEST_DB=1` 可保留 run 库。
- 强制重建模板：`FORCE_TEMPLATE_REBUILD=1 go test ./handlers/ ...`（会先删模板）。

## 排障

| 现象 | 处理 |
|---|---|
| `FATAL: ensure template ... build failed: shimmed migrations: up: ...` | 迁移链重放失败：按报错语句补 shim 规则（#2342 D5），**不得**改为跳过；确认后 `FORCE_TEMPLATE_REBUILD=1` 重跑 |
| `relation "..." does not exist` 且 env 带 `TUNELOOP_TEMPLATE_MIGRATION_SHIM=off` | 这是证据模式的预期红（#2341），正常构建不要设置该变量 |
| `cents schema drifted` | 有测试改了共享列类型（如迁移测试）；删除模板或 `FORCE_TEMPLATE_REBUILD=1` 重建 |
| 模板 meta 过期（代码改模型/种子/迁移文件） | 自动重建；无需手工干预 |
| 迁移文件改动后仍复用旧模板 | 理论上不会：`migrations_hash` 基于幂等化后的内容参与判定；若怀疑，`FORCE_TEMPLATE_REBUILD=1` |
| 多节点同时首建 | `pg_advisory_lock(7341001)` 串行，第二节点等待后复用 |
| `go: could not import ... no such file`（GOCACHE 损坏，多 agent 并发构建） | `go clean -cache` 或使用独立 `GOCACHE=<repo>/tmp/gocache` |
| 套件很慢 | 本地 overlayfs 下 TRUNCATE 慢；模板克隆已避免全量重建；分片方案暂缓（#2341 Out of Scope） |

## 相关

- #2335：dev PG 暴露面/免密（测试**不得**使用 `jobmaster-postgres:5432` 共享库做 TRUNCATE）。
- #2341：模板库 + meta 戳 + 金额列 fail-fast（本文件的原始来源）。
- #2342：迁移链 fresh-install 可重放（幂等化 shim 的设计与验证）。
- #2344：严格 schema（= 生产形状）下 6 个既有 fixture 的现代化（独立工作流）。
