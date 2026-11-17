# R2 人工回执 —— 创建表单提交负载（user_type）

> executor: user
> DevTools → Network，提交一次创建，核对后把 `RESULT: PENDING` 改为 `RESULT: PASS`（或 FAIL + 原因）。

| # | 核对项 | 期望 | 勾选 |
|---|---|---|---|
| 1 | `POST /api/users` 请求体含 `user_type: "merchant_direct"` | 是 | [x] |
| 2 | 请求体不含其它 `user_type` 取值 | 是 | [x] |
| 3 | 创建成功（200/201，无 400） | 是 | [x] |

- 操作人：user (Owner)
- 时间：2026-11-17
- 备注：Owner 2026-11-17 实测：创建 shanghuyuangong3 成功；POST /api/users 请求体含 user_type="merchant_direct"。

RESULT: PASS
