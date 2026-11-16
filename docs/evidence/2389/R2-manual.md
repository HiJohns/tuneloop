# R2 人工回执 —— 直接访问无邮箱用户的重设密码页：提交兜底禁用

> executor: user
> 核对后把文末 `RESULT: PENDING` 改为 `RESULT: PASS`（或 FAIL + 原因）。

## 步骤

1. 手动访问 `/staff/<无邮箱用户 id>/reset-password`（可从列表进入后改 URL，或从历史记录）；
2. 观察页面与提交按钮。

| # | 核对项 | 期望 | 勾选 |
|---|---|---|---|
| 1 | 顶部 Alert | 出现「该账号未配置邮箱，无法通过邮件重置密码」 | [ ] |
| 2 | 「确认重置」按钮 | 禁用（灰显，不可点击） | [ ] |
| 3 | 点击/回车尝试提交 | 无 `POST /users/reset-password` 请求 | [ ] |

## 回执

- 操作人 / 时间 / 版本：
- 截图或 Network 片段：

RESULT: PENDING
