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
| 1 | `POST /api/users` status | 200 (previously 500 `merchant not found for operator scope`) | [ ] |
| 2 | Response payload | `data.id` present; no orphan marker | [ ] |
| 3 | Local `users` row | exactly one row with the new user id | [ ] |
| 4 | Local `merchant_members` row | exactly one row for that user, tenant = merchant org | [ ] |
| 5 | IAM bind call | binds to the **merchant org** (`org_id`), not the namespace tenant | [ ] |
| 6 | List rendering | the new staff member appears in the merchant-direct list | [ ] |
| 7 | Regression | creating a `site_staff` member still succeeds unchanged | [ ] |

## Verification SQL (read-only)

```sql
-- tuneloop database (pre-production: tuneloop_pre_snapshot)
SELECT id, username, name, tenant_id, org_id FROM users WHERE username = '<username>';
SELECT user_id, tenant_id, merchant_id, role FROM merchant_members WHERE user_id = '<user id>';
```

## Receipt

- Operator:
- Date:
- Build version (`/api/config.version`):
- Fix commit: e379b0639
- Screenshots / request snippets (paste):

RESULT: PENDING
