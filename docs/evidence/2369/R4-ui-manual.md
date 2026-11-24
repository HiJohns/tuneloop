# R4 manual receipt — #2369 IAM orphan cleanup panel

> executor: ops
> Related: #2369 (parent #2364). Fill this receipt in pre-production (or local dev) after deploying
> a build that contains the IAM orphan panel. Change `RESULT: PENDING` to `RESULT: PASS` only after
> every item below is checked; use `RESULT: FAIL` with a reason otherwise.

## Preconditions

- PC admin build containing #2369 is deployed; log in as a system administrator (namespace admin).
- At least one orphan account exists (created by a failed creation whose rollback purge was rejected),
  or create one per `docs/ops/pending-user-orphan-cleanup.md` section 3.
- Browser devtools → Network panel open.

> 执行环境：preweb `1.0.95` / build `c8b44252b`；系统管理员会话；菜单已改名 **系统 → 账户管理**（`/system/user-management`）。
> 测试数据：为满足「pending + `users.org_id IS NULL` + 本地无行 + ≥60 分钟」四条判定，人工插入 2 个一次性 IAM 账号（`orphan_test_2469` / `orphan_test_2469b`，`created_at` 回填 65/70 分钟前），并**各补 1 条组织关系**（`cadenza`）——原因见下方「阻塞发现」。

## Steps and checks

1. Go to **System → 账户管理** (`/system/user-management`).
2. Click **IAM 孤儿账号** in the toolbar.

| # | Check | Expected | Done |
|---|---|---|---|
| 1 | Panel opens and lists orphan accounts | Table shows username / name / email / phone / created_at / minutes pending | [x]（**带组织关系的 fixture**；真实无关系孤儿不可见 → 见下） |
| 2 | Request uses `GET /api/admin/iam-orphans` | 200 with `data.list` matching the IAM-side SQL | [x] |
| 3 | Single cleanup asks for confirmation | Second confirmation dialog appears; cancel does nothing | [x] |
| 4 | Single cleanup after confirm | `POST /api/admin/iam-orphans/purge` with `user_ids:[id]`; success toast; list refreshes and row disappears | [x] |
| 5 | Bulk cleanup asks for confirmation | Selecting rows enables 批量清理; confirmation shows the count | [x] |
| 6 | Bulk cleanup after confirm | `purged` equals the selected count; list refreshes | [x] |
| 7 | Accounts with a local row are rejected | If any input maps to an existing local user, it appears under `failed` and is not deleted | **N/A**（UI 不可达，见下） |
| 8 | Re-created user succeeds | Creating the same username/email again succeeds (no more 409) | [x] |

## Observed evidence

`GET /api/admin/iam-orphans`（2 条 fixture，均 `pending` / `org_id NULL` / 本地无行 / 67~70 分钟）：

```json
{"code":20000,"data":{"list":[
  {"id":"1dd0d85e-…","username":"orphan_test_2469","name":"孤儿测试账号","email":"orphan_test_2469@example.com","phone":"","created_at":"2026-11-24T20:56:31+08:00","age_minutes":317},
  {"id":"88628468-…","username":"orphan_test_2469b","name":"孤儿测试账号B","email":"orphan_test_2469b@example.com","phone":"","created_at":"2026-11-24T20:53:42+08:00","age_minutes":320}
],"min_age_minute":60,"scanned":3,"total":2},"message":"success"}
```

- 列表渲染：登录名 / 姓名 / 邮箱 / 手机（空显示 `-`）/ 创建时间（本地时区）/ 已挂起（317、320 分钟）——字段齐全；
- 单项清理：二次确认出现，**取消无变化**；确认后 `purge` 成功、列表刷新少一行；
- 批量清理：勾选后按钮显示计数，确认后 `purged` 与选中数一致，列表清空（`total=0`）；
- 清理后 IAM 侧核对：`orphan_test_2469b` 已物理删除；同名重建 `orphan_test_2469` **成功**（新 `iam_sub = 0a7ded75…`，无 409）。

## 阻塞发现（第 1 项无法用真实孤儿验证）→ #2470

IAM 中实际有 2 个 `status='pending'` 账号（含本次插入的**无组织关系**那个），但接口返回 `scanned=1`：无关系的用户被 beaconiam `ListUsers` 的可见性过滤（`id IN (SELECT user_id FROM user_org_relations WHERE org_id IN (…) AND is_active)`，`user_handler.go:128-134`）**直接排除**。

- 「bind 失败」型真实孤儿恰恰没有组织关系 → 面板永远扫不到 → 同邮箱重试持续 409，功能设计目的落空；
- 因此第 1 项只能用「人工补组织关系」的 fixture 验证；真实形态待 **#2470** 修复后重验。

## 第 7 项 N/A 的原因

`多大的`（本地有行、有组织关系）在 `orphanFromPending` 阶段就被 `localIAMSubs` 正确排除，**不会出现在列表**，因此 UI 上无法构造「提交 purge 后被判 `failed`」的输入；该分支由 handler 单测覆盖（`iam_orphan_2364_test.go`），不阻塞本单。

## Receipt

- Operator: user (Owner) + 复核 (AI auditor)
- Date: 2026-11-24
- Build version (`/api/config.version`): preweb `1.0.95` / `c8b44252b`
- Screenshots / request snippets (paste): 见上「Observed evidence」

RESULT: PENDING

> 待补：第 1 项在 #2470（无关系孤儿不可见）修复后，用真实形态的孤儿重验，再把 `RESULT` 置 PASS。
> 其余 7 项（2–8，其中 7 为 N/A）已实测通过。
