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

## Steps and checks

1. Go to **System → User Management** (`/system/user-management`).
2. Click **IAM 孤儿账号** in the toolbar.

| # | Check | Expected | Done |
|---|---|---|---|
| 1 | Panel opens and lists orphan accounts | Table shows username / name / email / phone / created_at / minutes pending | [ ] |
| 2 | Request uses `GET /api/admin/iam-orphans` | 200 with `data.list` matching the IAM-side SQL | [ ] |
| 3 | Single cleanup asks for confirmation | Second confirmation dialog appears; cancel does nothing | [ ] |
| 4 | Single cleanup after confirm | `POST /api/admin/iam-orphans/purge` with `user_ids:[id]`; success toast; list refreshes and row disappears | [ ] |
| 5 | Bulk cleanup asks for confirmation | Selecting rows enables 批量清理; confirmation shows the count | [ ] |
| 6 | Bulk cleanup after confirm | `purged` equals the selected count; list refreshes | [ ] |
| 7 | Accounts with a local row are rejected | If any input maps to an existing local user, it appears under `failed` and is not deleted | [ ] |
| 8 | Re-created user succeeds | Creating the same username/email again succeeds (no more 409) | [ ] |

## Receipt

- Operator:
- Date:
- Build version (`/api/config.version`):
- Screenshots / request snippets (paste):

RESULT: PENDING
