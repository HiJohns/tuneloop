# build=1.0.d43b97d2a # commit=d43b97d2a0ccadf3be58c446a093436bf355b7b4 # dirty=false
# R7-role-column 人工回执 — #2375

> executor: user
> 完成对应操作后把文末 `RESULT: PENDING` 改为 `RESULT: PASS`（或 FAIL + 原因）。

核对项（Rev.2 R7）：以商户管理员登录 PC → `/staff` 列表 → 「角色」列可见，且值与后端 `role` 一致；编辑任一用户角色保存后列表即时更新；Network 无 4xx/5xx。

- 操作人：user (Owner)
- 时间：2026-11-24
- 版本 / 截图或 Network 片段：preweb `1.0.95` / build `c8b44252b`
- 核对结果：`/staff` 列表「角色」列可见且值来自后端 `role`；编辑用户改角色保存后列表即时更新（audit `user.role_update`：`{"from":"","to":"site_member"}`）。
- 备注：浏览器侧 `PUT /api/users/<id>` 返回 HTTP 200，但响应 `data.role_errors` 非空（IAM 角色同步 400 `role is required`，另立 **#2461** 跟踪）。R7 判定范围为「角色列展示 + 即时更新」，不覆盖 IAM 侧生效性，故 PASS。

RESULT: PASS
