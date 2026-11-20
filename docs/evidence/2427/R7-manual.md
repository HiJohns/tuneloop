# R7 人工回执 — #2427 人员筛选（直属开关即时生效 + 空网点全量）

> executor: architect
> 关联：#2427（#2067 直属筛选 / #2065 三视图）。请在 PC 人员管理页（本地 `cd frontend-pc && npm run dev`，
> 或已部署该 fix 的 `preweb`）逐项执行后，把 `RESULT: PENDING` 改为 `RESULT: PASS`；
> 任一项不符写 `RESULT: FAIL` 并附现象（不得未执行就写 PASS）。勾选方式：把 `[ ]` 改成 `[x]`。

## 前置条件

- PC 前端包含 fix commit（`/api/config` 的 `build` 或页面 JS hash 与该提交一致），账号为 **商户管理员**（`businessRole=merchant_admin`）。
- 商户下同时存在：至少 1 名网点成员、至少 1 名商户直属员工（或维修师傅），否则「只看直属」与「全量」无法区分。
- 浏览器开好 DevTools → Network（筛选 `personnel`）。

## 步骤与期望

### 1) 直属开关即时生效（R2/R7 主目标）

1. 打开「平台管理 → 人员管理」（`/staff`），不勾选任何项，记录当前列表条数（`total`）。
2. **勾选**「只看直属员工（含维修师傅）」。
   - [ ] **不点「搜索」**，1 秒内列表自动刷新（Network 出现新的 `GET /api/admin/personnel?...&direct=true`）。
   - [ ] 结果只剩商户直属员工 + 维修师傅（网点成员消失），条数与步骤 1 不同。
   - [ ] 若原本在第 2 页，刷新后回到第 1 页。
3. **取消勾选**。
   - [ ] 立即再次刷新，请求 URL 不含 `direct=true`，列表恢复为步骤 1 的全量。

### 2) 空网点搜索不再是空列表

4. 「网点」下拉选择任一网点 → 点「搜索」→ 记录该网点条数。
5. **清空**「网点」下拉 → 点「搜索」。
   - [ ] 列表恢复为全量（非 0 条），**不是空列表**。
   - [ ] Network 请求 URL **不含** `site_id`（更不得出现 `site_id=null`）。
   - [ ] 后端日志无 `invalid input syntax for type uuid`（22P02）。

### 3) 非法参数显式拒绝（可选用 curl，与 R3 对齐）

6. 用同会话 token 直连：`GET /api/admin/personnel?site_id=null`
   - [ ] 返回 HTTP 400 + `{"code":40002,...}`（不是 200 + 0 条）。
   - [ ] 后端日志出现 `[Personnel] WARN: invalid site_id="null"`。

## 结论

RESULT: PENDING
执行人: （填写姓名/角色）
执行时间: （YYYY-MM-DD HH:MM）
备注: （异常现象、截图路径或 Network 请求 URL）
