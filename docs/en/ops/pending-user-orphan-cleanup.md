# IAM orphan account cleanup SOP (pending-user-orphan-cleanup)

> Related: #2369 (parent #2364). Audience: system administrators (namespace admins) + ops.
> Purpose: when user creation fails locally after IAM created the account, the rollback purge is
> rejected by IAM (only namespace admins may purge), leaving a **pending IAM account with no local
> row**. Retrying the same email/username then always returns 409. This SOP covers detection and cleanup.

## 1. What counts as an orphan (four conditions, all required)

| # | Condition | Notes |
|---|---|---|
| 1 | IAM `users.status = 'pending'` | never activated |
| 2 | IAM `users.org_id IS NULL` | not bound to a concrete organization |
| 3 | No tuneloop local `users` row with `iam_sub = <id>` | local transaction rolled back — this is what separates orphans from legitimate invitations |
| 4 | `created_at` older than **60 minutes** | avoids racing an in-flight creation |

> Do **not** identify orphans by "pending + no org membership" alone: ordinary invite users are also
> pending with no org relation, but they have a local `users` row and must be excluded.

## 2. Pre-production instance (2026-11-16)

```text
[CreateUser] rollback purge failed for 6a08ff3b-...: PurgeUser returned status 403:
             {"error":"only namespace admin can purge users"}
[CreateUser] 500 → retrying the same email/username returned 409 five times (already registered in IAM)
```

- Symptom: repeated create failures (500); retries with the same email/username always 409.
- Root cause: tuneloop's rollback called `DELETE /api/v1/users/{id}/purge` with **service credentials**;
  IAM allows only namespace admins (or the user themselves), so it returned 403 and the account stayed.
- Resolution: clean up the `iam_user_id` per section 4, then recreate the user (succeeds first try).

## 3. Detection (read-only)

### 3.1 UI (recommended)

1. PC admin → **System → User Management** (`/system/user-management`).
2. Click **“IAM 孤儿账号” (IAM orphan accounts)** in the toolbar.
3. The panel lists username / name / email / phone / created_at / minutes pending, with single and
   bulk cleanup actions.

### 3.2 API (equivalent, script-friendly)

```bash
# Requires a system-admin token (cookie or Bearer)
curl -s -H "Authorization: Bearer $ADMIN_TOKEN" \
  https://preweb.cadenzayueqi.com/api/admin/iam-orphans | jq '.data'
```

Returns `data.list[]` (orphans), `data.scanned` (pending users scanned) and `data.min_age_minute` (60).

### 3.3 IAM-side verification SQL (read-only)

```sql
-- IAM database (pre: beaconiam_pre_snapshot / prod: beaconiam)
SELECT id, username, email, phone, status, org_id, created_at
FROM users
WHERE status = 'pending'
  AND org_id IS NULL
  AND created_at < now() - interval '60 minutes'
ORDER BY created_at;
```

### 3.4 Local verification SQL (read-only)

```sql
-- tuneloop database
SELECT id, iam_sub, username, email, status
FROM users
WHERE iam_sub = '<IAM_USER_ID>';
-- 0 rows = orphan (safe to clean); rows = legitimate invite/account, must NOT be purged (D5)
```

## 4. Cleanup steps

### 4.1 UI (recommended)

1. Open the orphan panel and cross-check against the SQL in section 3.
2. Single row: click “清理” → confirm. Bulk: select rows → “批量清理” → confirm.
3. Success shows “已清理 N 个”; failures are summarized (details in the browser console under
   `[iam-orphans] failed entries`).
4. Click “刷新” to confirm the list is empty.

### 4.2 API (equivalent)

```bash
curl -s -X POST -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"user_ids":["<IAM_USER_ID>"]}' \
  https://preweb.cadenzayueqi.com/api/admin/iam-orphans/purge | jq '.data'
# Expected: {"purged":1,"failed":[],"notes":["IAM purge does not clean user_task_queues ..."]}
```

Server-enforced guards:

- Any ID present in local `users.iam_sub` is rejected into `failed` and **not** deleted (D5).
- Non-orphans (non-empty org_id / too young / not pending) go to `failed`, never deleted.
- Empty `user_ids` → 400 `40001` (never a silent success).

## 5. `user_task_queues` follow-up (IAM side)

`DELETE /api/v1/users/{id}/purge` does **not** clean `user_task_queues`. If the account had queued bind
tasks (`bind_org` / `role`), review and delete them manually in the IAM database, otherwise the queue
keeps pending work for a user that no longer exists.

```sql
-- 1) Review (IAM database)
SELECT id, user_id, org_id, role, task_type, status, created_at
FROM user_task_queues
WHERE user_id = '<IAM_USER_ID>'
ORDER BY created_at;

-- 2) Confirm the user is gone (purge succeeded)
SELECT count(*) FROM users WHERE id = '<IAM_USER_ID>';  -- expected 0

-- 3) Delete leftover queue rows (only when step 2 returned 0)
DELETE FROM user_task_queues WHERE user_id = '<IAM_USER_ID>';
```

- **Red line**: the tuneloop backend never connects to the IAM database
  (`grep -rn user_task_queues backend` must return 0 hits). Queue cleanup is IAM-side ops work only.
- After bulk cleanups, find historical "queue points at a deleted user" leftovers with:
  `SELECT user_id, count(*) FROM user_task_queues q
   WHERE NOT EXISTS (SELECT 1 FROM users u WHERE u.id = q.user_id) GROUP BY user_id;`

## 6. Prevention and monitoring

- Failed creations now return `data.orphan=true` + `data.iam_user_id` (#2369 R1) and log the marker
  `create_user_orphan`:

  ```bash
  journalctl -u tuneloop-pre --since '1 hour ago' --no-pager | grep create_user_orphan
  ```

- On seeing the marker: verify per section 3, then clean per section 4; the retried creation succeeds.
- The panel enforces a 60-minute age threshold, so just-failed accounts appear only after ~1 hour.
