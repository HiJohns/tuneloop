# R3-invite 人工回执 — #2402

> executor: user
> 完成对应操作后把文末 RESULT: PENDING 改为 RESULT: PASS（或 FAIL + 原因）。

核对项见 Issue #2402 Rev.2「目标交互」表；R1 需 Network 无邀请请求；R2 需弹窗关闭且无请求；R3 请求必须是 inviteToMerchant 或合法 site。

- 操作人：user (Owner)
- 时间：2026-11-19
- 环境：预生产 PC `https://preweb.cadenzayueqi.com`（1.0.88 / `935f980ae`）
- 输入：创建员工，手机号 `20261117`（已存在账号「张鸿波」，同 tenant `bd6bfa4b…`）
- 备注：Owner 实测出现「该手机号/邮箱已注册」引导弹窗；点击「发送加入邀请」后发出
  `POST /api/admin/merchants/060bb8cf-8794-4eb3-af92-1416d5453f92/invites` → **201**；
  **未**出现 `/api/sites/undefined/invites/send`。

服务端复核（预生产 `tuneloop_pre_snapshot`，只读）：

```
id        b1207a19-bc42-4189-a952-a6589c8999b9
org_id    bd6bfa4b-bd6c-4d18-8848-98e9e2c7b3ae
role/kind merchant_staff / merchant_staff
invitee   identifier=20261117 user_id=98cad058-5cb3-4627-aade-14903f10e87a（张鸿波）
status    pending
created   2026-11-19 15:51:54+00
```

日志：

```
23:51:54 [GIN] 201 | POST "/api/admin/merchants/060bb8cf-8794-4eb3-af92-1416d5453f92/invites"
```

RESULT: PASS
