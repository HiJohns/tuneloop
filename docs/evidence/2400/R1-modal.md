# #2400 R1 — 自动生成密码：创建成功后一次性密码弹窗出现

executor: dev（headless Chromium + Vite dev + `/api/**` stub，真实 `/staff` 创建流程）
commit: 26e741e77768811db0b4da585292e90d11f43917

## 方法

1. `http://172.31.5.179:4599/staff`（system_admin 会话），点击「创建用户」；
2. 选择「商户直属员工」，填姓名/登录用户名/手机号；密码模式默认「自动生成」；
3. 提交；`POST /api/users` stub 返回 `{id, username, initial_password: "Abcd1234Efgh"}`（12 位）；
4. 断言弹窗数量、标题、identifier、密码长度。

## 红证（修复前，HEAD=26e741e77^）

```json
{ "modalCount": 0, "hasTitle": false, "hasPlaintext": false, "messages": [] }
```

即：`POST /api/users` 成功但弹窗不出现（`resetCreateState()` 在同一次回调内把 `open` 置回 false）。

## 绿证（修复后，HEAD=26e741e77）

```json
{
  "modalCount": 1,
  "title": "已创建 · 初始密码仅显示一次",
  "hasIdentifier": true,
  "hasPassword": true,
  "passwordLengthOk": true
}
```

RESULT: PASS
