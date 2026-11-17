# #2390 R3 — 网点成员管理引导链接（运行时）

executor: dev（headless Chromium + Vite dev + API mock，自动化运行时验证）
commit: d757bcb16e93560c5e4dda39c589d4ee0a3345df

## 方法

同 R1：创建表单顶部引导文案存在；点击「前往网点管理」后读取 URL。

## 断言与结果

| 断言 | 结果 |
|---|---|
| 引导文案存在 | ✅ `guideText=true`（「网点员工请在「网点管理 → 成员」中创建」） |
| 点击后跳转网点管理 | ✅ `afterGuideUrl=http://127.0.0.1:4599/organization/sites` |

RESULT: PASS
