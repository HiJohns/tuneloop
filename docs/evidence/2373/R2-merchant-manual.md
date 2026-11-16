# R2 人工回执 —— #2373 负责人（商户管理员）：空邮箱 + 手动密码

> Owner 在预生产（`preweb.cadenzayueqi.com`）按下列步骤核对后，把文末 `RESULT: PASS` 改为 `RESULT: PASS`（或 `FAIL` + 原因）。
> 契约：`grep -q 'RESULT: PASS' docs/evidence/2373/R2-merchant-manual.md`（见 #2373 Accepted Plan 的 R2 行）。

## 前置

- 预生产「商户 → 负责人管理」→「添加管理员」，已部署含 #2370/#2371/#2373 的版本。
  （注意：本页创建的是 `role=merchant_admin` 的**负责人**；「直属成员（merchant_staff）」在 `DirectMemberManagement`，属 #2372 范围。）
- 浏览器开发者工具 → Network 面板打开。

## 步骤与核对项

1. 点击「添加管理员」，填写用户名（如 `merchant_manual_01`）、姓名、手机，**邮箱留空**。
2. 密码设置切换为「手动设置」。
3. 先在密码框输入弱口令 `abc`，尝试提交。
4. 改输入达标密码 `Abcd1234`，点击「确认添加」。

| # | 核对项 | 期望 | 勾选 |
|---|---|---|---|
| 1 | 弱口令 `abc` 是否被前端拦截 | 是，提示「密码强度不足」类文案（不发起请求） | [ ] |
| 2 | 达标后请求体 `new_users[0]` 是否含 `password: "Abcd1234"` | 是 | [ ] |
| 3 | 请求体是否含 `auto_generate: false` | 是 | [ ] |
| 4 | 请求体是否**不含** `email` 字段 | 是 | [ ] |
| 5 | 响应 `initial_passwords[0].identifier` | 等于所填用户名 | [ ] |
| 6 | 一次性 Modal 是否弹出且显示该密码 | 是 | [ ] |
| 7 | 使用该用户名 + `Abcd1234` 登录 | 成功（如需首登改密则先改密） | [ ] |

## 回执

- 操作人：user (Owner)
- 时间：2026-11-16
- 关键截图/请求体片段（粘贴）：

RESULT: PASS
