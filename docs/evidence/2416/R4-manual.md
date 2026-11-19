# R4-manual 真机回执 — #2416（S2 迁移）

> executor: user
> 完成对应操作后把文末 `RESULT: PENDING` 改为 `RESULT: PASS`（或 FAIL + 原因）。

核对项（Rev.2 R4）：体验版真机抽查 ≥5 个页面，触发弹窗确认：
- 白底卡片、主色填充按钮；
- 自定义文案逐字保留（`重试视频/仅提交照片`、`继续支付/重试`）；
- 深色模式下不反色；长文案可滚动；confirm 返回值正确。

- 操作人：user (Owner)
- 时间 / 版本：2026-11-19 / weapp 体验版 `v1.0.935f980ae`（预生产 1.0.88）
- 页面清单与截图/Network 片段：

| # | 场景 | 结果 |
|---|---|---|
| 1 | 购物车 → 点删除 | **FAIL**：弹窗完全不出现，乐器未删除 |
| 2 | 我的 → 地址管理 → 删除 | **FAIL**：弹窗不出现，地址未删除 |
| 3 | 顾客订单详情 → 取消订单 | **FAIL**：弹窗不出现，订单未取消 |

服务端复核（预生产 `journalctl -u tuneloop-pre`，Owner IP 近 90 分钟）：仅见 `GET /api/user/addresses`，**无** `DELETE /api/user/addresses/:id`、**无**取消订单请求 → 执行流停在 `await dialog.confirm(...)`，Promise 永不 resolve。

结论：**FAIL（阻断）** —— 不是样式不达标，而是 weapp 端 `dialog.confirm/alert` 整体不响应。根因见 **#2431**（DialogHost 挂在 App 层，小程序无 App 视图层 → 组件 JS 层 mounted 但 View 不在任何页面可视树，`isDialogHostMounted()` 恒为 true，原生兜底永不触发）。

`RESULT: PENDING` → 本次签为 FAIL，需 #2431 修复后重测。
