# #2400 R3 — 手动密码路径不弹一次性弹窗（回归 #2388）

executor: dev（headless Chromium + Vite dev + `/api/**` stub）
commit: 26e741e77768811db0b4da585292e90d11f43917

## 方法

同 R1，密码模式选「手动设置」，填写 `Abcd1234` 后提交；断言无弹窗且提示为手动密码文案。

## 结果

```json
{
  "modalCount": 0,
  "hasManualMessage": true,
  "hasAutoMessage": false
}
```

| 断言 | 结果 |
|---|---|
| 不出现一次性密码弹窗 | ✅ `modalCount=0` |
| 显示手动密码文案 | ✅「已创建，账号可用用户名 + 你设置的密码登录」 |
| 不出现自动生成文案 | ✅ `hasAutoMessage=false` |

RESULT: PASS
