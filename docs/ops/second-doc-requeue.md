# 第二证件复审批次补建（#2312）

> 适用：历史用户已上传第二证件（`users.id_photo_other` 非空）但 `id_photo_other_verified=false`，且没有 pending/approved `second_doc` 批次。

## 1. Dry-run

```bash
cd /opt/tuneloop/apps/tuneloop && source .env
./service/tuneloop --second-doc-requeue --dry-run
```

输出 `[RequeueSecondDoc] dry run: N users would get a second_doc review batch`，不写库。

## 2. 执行

```bash
./service/tuneloop --second-doc-requeue
```

为每个候选用户创建 1 条 `face_capture_batches(kind=second_doc,status=pending)`；已有 pending/approved second_doc 批次的用户跳过（幂等）。

## 3. 核对

```sql
SELECT count(*) FROM face_capture_batches WHERE kind='second_doc' AND status='pending';
SELECT user_id, count(*) FROM face_capture_batches
 WHERE kind='second_doc' AND status IN ('pending','approved')
 GROUP BY user_id HAVING count(*) > 1;
```

第二条查询应为空；重复执行补建应输出 `created 0`。

## 4. 回滚

补建只新增 `second_doc` pending 批次，不修改用户字段。回滚按执行日志中的批次 ID（或执行时间窗口）删除：

```sql
DELETE FROM face_capture_batches
 WHERE kind='second_doc' AND status='pending' AND created_at >= '<执行开始时间>';
```

执行前请先导出受影响批次 ID 并以 `status='rejected'` 软回滚（优先），避免直接删除审计记录。
