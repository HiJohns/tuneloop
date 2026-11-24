# R4 manual receipt — #2367 merchant scope on pre-production

> executor: ops
> Related: #2367 (parent #2364, sub-task A). Fill this receipt on pre-production after the release that
> contains the fix commit below. Change `RESULT: PENDING` to `RESULT: PASS` only after every item is
> checked; use `RESULT: FAIL` plus a reason otherwise. Never write PASS without executing the steps.

## Preconditions

- Pre-production has a deployed build containing the fix (`/api/config.version` >= the version shipped
  with this Issue).
- A merchant administrator whose JWT `tid` is the **merchant org** while `merchants.tenant_id` is the
  **namespace org** (this is the exact shape that produced the 500).
- Browser devtools → Network panel open.

## Steps and checks

1. Log in as the merchant administrator on `/staff`.
2. Create a **商户直属员工** (`user_type=merchant_direct`) with username + phone (+ optional email), submit.

| # | Check | Expected | Done |
|---|---|---|---|
| 1 | `POST /api/users` status | 200 (previously 500 `merchant not found for operator scope`) | [x] |
| 2 | Response payload | `data.id` present; no orphan marker | [x] |
| 3 | Local `users` row | exactly one row with the new user id | [x] |
| 4 | Local `merchant_members` row | exactly one row for that user, tenant = merchant org | [x] |
| 5 | IAM bind call | binds to the **merchant org** (`org_id`), not the namespace tenant | [x] |
| 6 | List rendering | the new staff member appears in the merchant-direct list | [x] |
| 7 | Regression | creating a `site_staff` member still succeeds unchanged | [x]（口径调整见下） |

## Observed evidence（preweb `1.0.95` / build `c8b44252b`）

被测对象：`测试员工2372`（`users.id = 1dac4ae3-3132-4f61-89e3-7acf137241ee`，`iam_sub = 442079a1-8261-416b-b257-e3878a2647a3`）。

| # | 实测 |
|---|---|
| 1 | 创建返回 **200**（此前为 500 `merchant not found for operator scope`），前端提示「已创建，账号可用用户名 + 你设置的密码登录」 |
| 2 | 响应含 `data.id`；无 `create_user_orphan` 日志 / orphan 标记 |
| 3 | `users` 计数 = **1**（`SELECT count(*) … = 1`） |
| 4 | `merchant_members` 计数 = **1**，`tenant_id = bfce6c6d…`（商户 org），`users.org_id = bfce6c6d…` |
| 5 | IAM `user_org_relations`：`cadenza`（商户 org）`role = STAFF`、`functional_roles = ["merchant_staff","site_member"]`；未绑到 namespace tenant `3cfa99da…` |
| 6 | `/staff` 列表可见该成员（商户管理员列表 total 内） |
| 7 | **口径调整**：`/staff` 的 `site_staff` 创建入口已由 #2390/#2401 移除（网点员工统一走「网点管理 → 成员」创建，#2465 进一步收敛为「身份选择」），本项在 UI 上不再可达；端点级回归由 handler 测试与 #2465 验收矩阵承接，不作为本单阻塞项 |

## 说明（不属于本项判定）

- `merchant_members.role` 当前为 `site_member`（硬编码缺陷，见 **#2462**）；`users.role` 创建时未写入（同 #2462 D1）。本单第 4 项只断言「行存在 + tenant = 商户 org」，角色值口径以 #2462 修复后为准。

## Receipt

- Operator: user (Owner) + 复核 (AI auditor)
- Date: 2026-11-23（创建）/ 2026-11-24（DB 复核）
- Build version (`/api/config.version`): preweb `1.0.95` / build `c8b44252b`
- Fix commit: e379b0639
- Screenshots / request snippets (paste): 见上表「Observed evidence」的 psql 输出与 IAM 关系查询结果

RESULT: PASS
