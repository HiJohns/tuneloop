# R7 人工回执 — #2427 人员筛选（直属开关即时生效 + 空网点全量）

> executor: architect
> 关联：#2427（#2067 直属筛选 / #2065 三视图）。请在 PC 人员管理页（本地 `cd frontend-pc && npm run dev`，
> 或已部署该 fix 的 `preweb`）逐项执行后，把 `RESULT: PENDING` 改为 `RESULT: PASS`；
> 任一项不符写 `RESULT: FAIL` 并附现象（不得未执行就写 PASS）。勾选方式：把 `[ ]` 改成 `[x]`。

## 前置条件

- PC 前端包含 fix commit（`/api/config` 的 `build` 或页面 JS hash 与该提交一致），账号为 **商户管理员**（`businessRole=merchant_admin`）。
- 商户下同时存在：至少 1 名网点成员、至少 1 名商户直属员工（或维修师傅），否则「只看直属」与「全量」无法区分。
- 浏览器开好 DevTools → Network（筛选 `personnel`）。

> 本次执行环境：preweb `1.0.95`（构建 `c8b44252b`，含 fix `b334fa7ff`）；账号 = cadenza 商户管理员。

## 步骤与期望

### 1) 直属开关即时生效（R2/R7 主目标）

1. 打开「平台管理 → 人员管理」（`/staff`），不勾选任何项，记录当前列表条数（`total`）→ **8**。
2. **勾选**「只看直属员工（含维修师傅）」。
   - [x] **不点「搜索」**，1 秒内列表自动刷新（Network 出现新的 `GET /api/admin/personnel?...&direct=true`）。
   - [x] 结果只剩商户直属员工 + 维修师傅（网点成员消失），条数与步骤 1 不同 → **6**（8 → 6）。
   - [x] 若原本在第 2 页，刷新后回到第 1 页（本次未处于第 2 页，请求均为 `page=1`）。
3. **取消勾选**。
   - [x] 立即再次刷新，请求 URL 不含 `direct=true`，列表恢复为步骤 1 的全量 → **8**。

服务端日志（`tuneloop-pre`，Owner 会话 IP `99.229.7.13`）：

```
23:29:41 GET "/api/admin/personnel?direct=true&page=1&page_size=10"  200
23:29:49 GET "/api/admin/personnel?page=1&page_size=10"               200   ← 取消勾选后立即回到全量
```

### 2) 空网点搜索不再是空列表

4. 「网点」下拉选择任一网点 → 点「搜索」→ 记录该网点条数。
   - 所选项：**卡丹萨乐器**（`sites.id = 0a0fb375-aabe-4573-994e-e7d56c134e25`）→ **5** 条。
5. **清空**「网点」下拉 → 点「搜索」。
   - [x] 列表恢复为全量（非 0 条），**不是空列表** → **total = 8**。
   - [x] Network 请求 URL **不含** `site_id`（更不得出现 `site_id=null`）。
   - [x] 后端日志无 `invalid input syntax for type uuid`（22P02）。

服务端日志：

```
23:33:46 GET "/api/admin/personnel?site_id=0a0fb375-aabe-4573-994e-e7d56c134e25&page=1&page_size=10"  200
23:34:09 GET "/api/admin/personnel?page=1&page_size=10"                                                200   ← 清空网点后无 site_id
```

40 分钟窗口内 `grep -iE 'personnel|22P02|invalid input syntax'` 无任何 `22P02` / `invalid site_id` 记录。

### 3) 非法参数显式拒绝（可选用 curl，与 R3 对齐）

6. 用同会话 token 直连：`GET /api/admin/personnel?site_id=null`
   - [ ] 返回 HTTP 400 + `{"code":40002,...}`（不是 200 + 0 条）。**未执行 curl（可选项）**；代码侧为 `backend/handlers/personnel.go:65-66`（非法 UUID → `WARN` + `400/40002`），本次窗口日志无 `invalid site_id` 记录。
   - [ ] 后端日志出现 `[Personnel] WARN: invalid site_id="null"`。**未触发**（前端已不再发送 `site_id=null`，第 5 步已实证）。

## 结论

RESULT: PASS
执行人: user (Owner)
执行时间: 2026-11-23 23:29–23:34（Asia/Shanghai）
备注: 环境 preweb `1.0.95` / build `c8b44252b`。§1 实测 8→6→8（自动刷新、`direct=true`）；§2 网点=卡丹萨乐器 5 条 → 清空后 8 条且请求无 `site_id`；§3 curl 未执行（可选项），代码与日志侧核对一致。
