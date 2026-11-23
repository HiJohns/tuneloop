# R5-assertions — #2363（三测试文件断言迁移）

> 依据：Accepted Plan Rev.2 D4/D5。`settlement_flow_test.go` / `settlement_diff_test.go` /
> `state_machine_e2e_test.go` 的退款断言迁移已由 **#2424 Phase B（Rev.6）** 完成，并在
> `docs/evidence/2424/R4-derivation.md` 逐项记录 old→new。本单只新增历史标记/分账户断言，
> 不重复改写已迁移断言（避免二次口径漂移）。

## 1. settlement_diff_test.go（#2424 已完成迁移 + 本单新增 R3）

| 断言 | old | new |
|---|---:|---:|
| PointsOverCap TotalRefund | 666.67 | 660.00（乐币 160 + 现金毛额 500） |
| PointsOverCap CashRefundable | 506.67 | 500.00（毛额） |
| PointsOverCap Payable/Net | — | 460.00 / 40.00 |
| PointsWithinCap CashRefundable | 666.67 | 500.00 |
| PointsWithinCap Payable/Net | — | 300.00 / 200.00 |
| 本单 R3 `CashRefundableGross − PayableShortfallGross = NetCash` | — | 500 − 460 = 40 |

## 2. state_machine_e2e_test.go（#2424 已完成迁移）

| 断言 | old | new |
|---|---:|---:|
| LateReturn 退款基数 | `CashRefundable`（毛额 2220） | `NetCash`（1750） |
| LateReturn 区间 | 1500–2000 | 1500–2000（净额不变） |
| DamageAccept | `CashRefundable` 2500 | 保持不变（段模型回退 #1743，spec 跳过） |

## 3. settlement_flow_test.go（本单新增 R1）

| 断言 | old | new |
|---|---|---|
| GET `/user/settlements/:id` 响应 | 无 `legacy_marker`/`legacy_reason` | 新增两字段（只读回显，金额不变） |
| GET `/user/settlements/:id/calculate` 响应 | 仅 `data`（持久化字节） | 新增两字段；`data` 字节不变（#1738） |
| 无标记订单 | — | 两字段为空，不编造原因 |

## 4. 负例

- 回显逻辑修改金额 → R1 测试 `CashRefundable` 断言失败；
- 用毛额冒充实际退款 → R3 `NetCash` 等式失败；
- 把无标记订单填默认原因 → R1 空标记子用例失败。
