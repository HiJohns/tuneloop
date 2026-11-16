# R6 真机回执（#2333 评测页入参 + 提交/轮询）

executor: user
date: 2026-11-15
environment: weapp 体验版 / 预生产后端 1.0.85

## 入参生效

- 唯一入参 `order_id`（可选 `return_path`）；页面不接收 `tier` / `risk_amount_cents`。
- 进入页面后按 `order_id` 拉取 `GET /orders/:id/credit-status`，标的金额显示后端返回的 `risk_amount_cents`（= 乐器原价 ¥1.00），页面不自算。

## 提交与轮询

| 环节 | 观测 | 结论 |
|---|---|---|
| 提交（补建） | 下单即 `credit_state=pending` 但服务单为空 → 页面调用 `POST /orders/:id/credit-assessment` 补建（修复前此处不补建，金额显示「—」） | ✅ |
| 轮询 | `pending` 时退避轮询（1.5s→8s），mock 返回 approved 后停止并展示「评测通过」 | ✅ |
| 返回 | 「返回继续支付」跳回 `return_path`（支付页） | ✅ |

> 该页 URL 亦作为微信支付分申请提交的页面地址（S0 #2328）：`/credit-assessment`（H5）、`pages-weapp/credit-assessment/index`（小程序）。

RESULT: PASS
