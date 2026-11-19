# R2 人工回执 —— 直接访问无邮箱用户的重设密码页：提交兜底禁用

> executor: user
> 核对后把文末 `RESULT: PENDING` 改为 `RESULT: PASS`（或 FAIL + 原因）。

## 步骤

1. 手动访问 `/staff/<无邮箱用户 id>/reset-password`（可从列表进入后改 URL，或从历史记录）；
2. 观察页面与提交按钮。

| # | 核对项 | 期望 | 勾选 |
|---|---|---|---|
| 1 | 顶部 Alert | 出现；标题「该账号未配置邮箱」，正文含「直接设置密码」 | [x] |
| 2 | 「确认重置」按钮 | 禁用（灰显，不可点击） | [x] |
| 3 | 点击/回车尝试提交 | 无 `POST /users/reset-password` 请求 | [x] |

## 回执

- 操作人：user (Owner)
- 时间：2026-11-19
- 环境：预生产 PC（1.0.88 / `935f980ae`）
- 访问地址：`https://preweb.cadenzayueqi.com/staff/ca5deecc-87b2-44d4-9fa6-36ca35920d08/reset-password`
- 结果：页面出现「该账号未配置邮箱」提示，**提交按钮禁用**；尝试提交无 `POST /api/users/reset-password` 请求。
- 截图或 Network 片段：Owner 口头确认（2026-11-19 会话），无截图留存。

RESULT: PASS
