# #2400 R2 — 关闭弹窗后的清理与列表刷新

executor: dev（headless Chromium + Vite dev + `/api/**` stub）
commit: 26e741e77768811db0b4da585292e90d11f43917

## 方法

承接 R1：点击「我已保存，关闭」→ 等待弹窗 hidden → 断言可见性、DOM 明文、列表与表单状态；
随后再次点击「创建用户」，检查姓名/登录名输入框是否为空（`resetCreateForm` 生效）。

## 结果

```json
{
  "modalVisible": false,
  "modalWrapHidden": true,
  "plaintextGone": true,
  "listHasNewMember": true,
  "formNameValue": "",
  "formUsernameValue": ""
}
```

| 断言 | 结果 |
|---|---|
| 关闭后弹窗不可见 | ✅ `modalVisible=false`、`modalWrapHidden=true` |
| 关闭后 DOM 无明文密码 | ✅ `plaintextGone=true` |
| 列表已刷新（含新成员） | ✅ `listHasNewMember=true` |
| 再次进入创建表单为空 | ✅ 姓名/登录名均为空 |

RESULT: PASS
