# 测试库生命周期（handlers）

> #2341。适用于 `backend/handlers` 测试套件；生产路径不受影响。

## 初始化顺序

1. `TestMain` 解析 run 库名（`TUNELOOP_TEST_DB` 或按节点隔离的 `tuneloop_test_<node>_<hash>`）。
2. `EnsureTemplate` 确保共享模板 `tuneloop_test_template` 存在且未过期；缺失/过期时在 `pg_advisory_lock` 下重建：
   **`CREATE DATABASE` → AutoMigrate(模型) → PostMigrate（iam_sub / 纯 SQL 表）→ seed（system default pricing template）→ 写 meta 戳**。
3. `CREATE DATABASE <run> TEMPLATE tuneloop_test_template` 秒级克隆。
4. `database.InitDB(run)` → `AssertCentsSchema`（金额列必须 bigint）→ `m.Run()`。

> **限制**：模板**不应用** `backend/database/migrations`。迁移链不是 fresh-install-safe（001 无 `IF NOT EXISTS`；`20260630003` 的 `ALTER appeals` 早于 `20260706002` 建表），由 **#2342** 承接修复。模板只保证「模型 schema + 业务种子」正确。

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
| `FATAL: ensure template ... build failed` | 看模板构建日志；确认 PG 可达、模型可 AutoMigrate；修复后 `FORCE_TEMPLATE_REBUILD=1` 重跑 |
| `cents schema drifted` | 有测试改了共享列类型（如迁移测试）；删除模板或 `FORCE_TEMPLATE_REBUILD=1` 重建 |
| 模板 meta 过期（代码改模型/种子） | 自动重建；无需手工干预 |
| 多节点同时首建 | `pg_advisory_lock(7341001)` 串行，第二节点等待后复用 |
| `go: could not import ... no such file`（GOCACHE 损坏，多 agent 并发构建） | `go clean -cache` 或使用独立 `GOCACHE=<repo>/tmp/gocache` |
| 套件很慢 | 本地 overlayfs 下 TRUNCATE 慢；模板克隆已避免全量重建；分片方案暂缓（#2341 Out of Scope） |

## 相关

- #2335：dev PG 暴露面/免密（测试**不得**使用 `jobmaster-postgres:5432` 共享库做 TRUNCATE）。
- #2342：迁移链 fresh-install 可重放（模板不含 migrations 的根因）。
