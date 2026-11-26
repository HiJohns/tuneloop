PROVENANCE: fix=b67d95af40c50eca38a8d95f6531367a7710aadb request=2512-20261126T111600Z ts=2026-11-26T11:40:00Z

# Verification Request 2512-20261126T111600Z — 结果摘要

- request_id: `2512-20261126T111600Z`
- fix_commit: `b67d95af40c50eca38a8d95f6531367a7710aadb`
- source_tree: `origin/issue-2512 @ b67d95af4`（隔离 fix-commit 树，**未**与 develop / 同族兄弟分支合并）
- worktree: `/tmp/opencode/tuneloop/reverify-2512`（detached HEAD = fix_commit，tracked 文件干净）
- toolchain: bootstrap `go1.22.5`，`go.mod` 要求 `go 1.25.0` → `GOTOOLCHAIN=auto` 使用缓存 toolchain `go1.25.0`
- 环境：`backend/.env` 由本机 dev 环境复制（gitignored），仅用于让包测试连到 PostgreSQL；未改动业务代码

## 命令与结果

| # | 命令 | exit | 产物 | 说明 |
|---|---|---|---|---|
| 1 | `cd backend && go build ./...` | 0 | `command-1.txt` | 锁等待 92.0s，持锁 30.9s |
| 2 | `cd backend && go test ./... -count=1 -timeout 30m` | 0 | `full-gate.txt` | 锁等待 130.7s，持锁 927.2s，总耗时 1059s |

## 全量 gate 统计

- `BUILD_RC=0`、`FULL_GATE_EXIT=0`
- `FAIL_COUNT=0`、`PANIC_COUNT=0`
- `PACKAGE_OK_COUNT=8`：`database`(3.0s)、`handlers`(887.2s)、`middleware`(0.03s)、`models`(0.01s)、`service`(0.02s)、`services`(13.5s)、`services/tencentcloud`(0.01s)、`services/wechatpay`(0.07s)
- `packages without tests`：9（`?  no test files`）
- 锁来源：`heavy-lock.sh --scope host`（host scope，单命令包裹，无嵌套锁）
