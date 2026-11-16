# R3 人工回执 —— #2372 重设密码入口：无邮箱账号（不置灰 + 明确文案）

> Owner 在预生产（`preweb.cadenzayueqi.com`）按下列步骤核对后，把文末 `RESULT: PENDING` 改为 `RESULT: PASS`（或 `FAIL` + 原因）。
> 契约：`grep -q 'RESULT: PASS' docs/evidence/2372/R3-reset-entry.md`（见 #2372 Accepted Plan 的 R3 行）。

## 前置

- 预生产存在一个 `email = ''` 的员工账号（可用 R2 步骤创建的 `staff_manual_01`，其邮箱为空）。
- 列表页勾选该用户。

## 步骤与核对项

1. 在「人员管理」列表勾选该无邮箱用户。
2. 点击「重设密码」。

| # | 核对项 | 期望 | 勾选 |
|---|---|---|---|
| 1 | 「重设密码」按钮是否可点击（未置灰） | 是 | [ ] |
| 2 | 点击后是否出现提示 | 是，含「该账号未配置邮箱，重置需管理员直设密码」 | [ ] |
| 3 | 是否**未**发出 `POST /users/reset-password` 请求 | 是（Network 无该请求） | [ ] |
| 4 | 有邮箱用户点击「重设密码」是否仍正常发邮件 | 是（回归） | [ ] |

## 回执

- 操作人：
- 时间：
- 关键截图（提示文案 + Network 面板）：

RESULT: PENDING
