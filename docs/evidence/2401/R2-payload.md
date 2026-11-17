# R2 人工回执 —— 创建请求固定 user_type=merchant_direct

> executor: user
> DevTools → Network，提交一次创建，核对后把 `RESULT: PENDING` 改为 `RESULT: PASS`。

| # | 核对项 | 期望 | 勾选 |
|---|---|---|---|
| 1 | `POST /api/users` 请求体含 `user_type: "merchant_direct"` | 是 | [ ] |
| 2 | 请求体不含其它 `user_type` 取值 | 是 | [ ] |
| 3 | 创建成功（200/201，无 400） | 是 | [ ] |

RESULT: PENDING
