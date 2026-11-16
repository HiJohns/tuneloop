# R1 人工回执 —— 无邮箱用户：列表行「重设密码」入口拦截

> executor: user
> 核对后把文末 `RESULT: PENDING` 改为 `RESULT: PASS`（或 FAIL + 原因）。打开 DevTools → Network。

## 步骤

1. 人员管理列表找到 `email=''` 的用户（可用 #2388 R1 创建的无邮箱账号）；
2. 点击该行「重设密码」。

| # | 核对项 | 期望 | 勾选 |
|---|---|---|---|
| 1 | 是否出现提示 | 是：「该账号未配置邮箱，需管理员直设密码（功能对接中）」 | [ ] |
| 2 | 是否跳转到 `/staff/:id/reset-password` | 否（停留在列表） | [ ] |
| 3 | Network 是否有 `POST /users/reset-password` | 否 | [ ] |

## 回执

- 操作人 / 时间 / 版本：
- 截图或 Network 片段：

RESULT: PENDING
