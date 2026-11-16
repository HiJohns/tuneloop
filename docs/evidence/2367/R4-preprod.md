# #2367 R4 — 预生产复测（ops 待办工件）

executor: ops
RESULT: PENDING
issue: #2367
created: 2026-11-16
owner: ops / 系统管理员

## 说明

本工件由 Developer 按 Accepted Plan Rev.2 D2 生成：Developer 无预生产部署权限
（发布类命令仅用户显式发起），因此**不复测、不填写完成结论**。实际复测在 ops
完成部署后回填本文件，或在本 Issue 关闭时通过 `[Ops-Followup of #2367]`
（`waitForConfirm`，无 `status:*`）跟踪。

## 待办步骤

1. 将 `issue-2367`（fix commit `bdc92af30`）合入 `develop` 并发布预生产（由用户/ops 发起）。
2. 以预生产**商户管理员**登录 `https://preweb.cadenzayueqi.com`，进入 `/staff`。
3. 创建「商户直属员工」（`user_type=merchant_direct`，自动生成初始密码）。
4. 断言：
   - HTTP 200（不再是 `failed to create user: merchant not found for operator scope`）；
   - 新账号出现在 `/staff` 列表；
   - 本地库 `users` 1 行（`tenant_id`/`org_id` = 商户 org）、`merchant_members` 1 行（`tenant_id` = 商户 org）。

## 回执要求

- 回填：执行人、执行时间、环境（预生产）、HTTP 状态、用户 ID、库内断言结果。
- 若仍 500：附 `journalctl -u tuneloop-pre --since` 中 `[CreateUser] #2247` /
  `merchant not found for operator scope` 日志片段，并保持本文件为 PENDING。
- 本文件不得写入完成结论以外的伪造结果；回填时把 `RESULT: PENDING` 改为实际结论。
