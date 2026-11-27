# Reverification Result — #2482 (request 2482-20261125T005013Z)

- fix_commit: `719e649291800235e48194c8ef10ad0420a3c70a`
- worktree: `/tmp/opencode/tuneloop/reverify-2482` @ `origin/issue-2482` = `b710680ed`
  (backend tree 与 fix commit 完全一致：`git diff --stat 719e6492 b710680ed -- backend/` 为空)
- 执行时间（UTC）: 2026-11-25T00:52:36Z → 2026-11-25T01:20:27Z
- 唯一变更：新增本目录证据文件（未改业务代码）

## 命令与退出码

| # | 命令 | exit | log |
|---|---|---|---|
| 1 | `cd backend && go test ./handlers -run '_2482$' -count=1 -v` | 0 | `command-1.log` |
| 2 | `cd backend && go test ./handlers -run '_2482$|_2475$|_2375$' -count=1` | 0 | `command-2.log` |
| 3 | `cd backend && go build ./...` | 0 | `command-3.log` |
| 4 | `cd backend && go test ./handlers -count=1 -timeout 15m` | 0 | `command-4.log` |
| 5 | `cd backend && go test ./services -count=1 -timeout 15m` | 0 | `command-5.log` |

`FAIL_COUNT=0`、`PANIC_COUNT=0`（全部 5 条命令日志中 `^--- FAIL` 与 `^panic:` 均为 0 行）。

## 复核要点逐项键值（源：command-1.log）

1. `mergeSiteMemberRoles` 纯函数：`CASE1=site_admin,repair_technician`、`CASE2=repair_technician`、
   `CASE3=repair_technician,site_admin`、`CASE4=site_admin`、`CASE5=repair_technician`、`ORDER_STABLE=1`
2. handler 合并：`ROLES_JSON=["site_admin","repair_technician"]`、`ROLE=site_admin`、`RED_GREEN=1`、
   `IAM_ASSIGN_CALLS=1`、`IAM_UNASSIGN_CALLS=0`、`DUAL_SOURCE_OK=1`
3. 多网点逐行：`ROW1_OK=1`、`ROW2_OK=1`、`ROW1_JSON=["site_admin","repair_technician"]`、`ROW2_JSON=["site_admin"]`
4. invariant：`NO_22P02=1`、`INVARIANT=1`、`ROLES_NOT_NULL=1`
5. 不翻转：`IAM_ROLE_CALLS=0`、`ROLES_UNCHANGED=1`、`NO_OLD_RESIDUE=1`
6. 全量：`FULL_SUITE_EXIT=0`、`FAIL_COUNT=0`、`PANIC_COUNT=0`

## 环境

- DB 仅注入 `POSTGRES_*` + `TUNELOOP_DB=tuneloop_debug`（来自 `backend/.env`）。
- 按请求方提示 `unset` 了 `BEACONIAM_INTERNAL_URL BEACONIAM_EXTERNAL_URL IAM_URL IAM_NAMESPACE
  IAM_SECRET IAM_PC_CLIENT_ID IAM_PC_CLIENT_SECRET IAM_CLIENT_ID IAM_CLIENT_SECRET`，
  使 `skipIfNoIAM` E2E 用例按设计 skip（不再出现 `Test_CreateSite_CusPerm` /
  `Test_CreateMerchant_CusPerm` 基线红）。
- 重命令统一经 `~/.config/opencode/scripts/build-lock`（host scope）执行。
