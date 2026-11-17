# #2390 R2 — 「网点员工」选项移除（运行时）

executor: dev（headless Chromium + Vite dev + API mock，自动化运行时验证）
commit: d757bcb16e93560c5e4dda39c589d4ee0a3345df

## 方法

同 R1：`/staff` → 点击「创建员工」进入表单，读取 `.ant-radio-wrapper` 选项集合与默认选中项。

## 断言与结果

| 断言 | 结果 |
|---|---|
| 无「网点员工」单选项 | ✅ `hasSiteStaffOption=false` |
| 存在「商户直属员工」选项 | ✅ `hasMerchantDirectOption=true` |
| 默认选中「商户直属员工」 | ✅ `checkedRadio=["商户直属员工","自动生成"]`（后者为密码模式） |

原始输出：

```json
{
  "radioLabels": ["商户直属员工", "自动生成", "手动设置"],
  "checkedRadio": ["商户直属员工", "自动生成"],
  "hasSiteStaffOption": false,
  "hasMerchantDirectOption": true
}
```

RESULT: PASS
