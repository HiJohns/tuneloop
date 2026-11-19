# R3-manual 真机回执 — #2426（needs_merge 展示）

> executor: user
> 完成对应操作后把文末 `RESULT: PENDING` 改为 `RESULT: PASS`（或 FAIL + 原因）。

核对项：体验版真机扫码触发 needs_merge，确认：
- 图标 / 标题「该微信已绑定其他账户」/ 正文 / 绑定编号各自独占一行；
- 绑定编号为脱敏值（前 6 + **** + 后 4），不显示完整 openid；
- PC 端 2 秒内出现「请在小程序端确认合并」并停止轮询。

- 操作人：user (Owner)
- 时间 / 版本：2026-11-19 / 预生产 1.0.89，weapp 体验版 `v1.0.5aa9737a3`
- 结果：
  - ✅ 标题 / 正文 / 绑定编号**三行分离**（不再挤成一行）；
  - ✅ 绑定编号为**脱敏值**，完整 openid 不再展示；
  - ⚠️ PC 端提示本轮**未复测**（由 R2 单测覆盖 `classifyBindPollStatus` 的 `needs_merge → stop + warning`）；
  - ⚠️ 同页发现新缺陷：该页「返回」按钮点击**无响应** → 已建 **#2434**。
- 截图：

RESULT: PASS
