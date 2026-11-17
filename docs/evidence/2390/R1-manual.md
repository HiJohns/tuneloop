# #2390 R1 — 创建入口文案（运行时）

executor: dev（headless Chromium + Vite dev + API mock，自动化运行时验证）
commit: d757bcb16e93560c5e4dda39c589d4ee0a3345df

## 方法

真实 PC 前端（`npm --prefix frontend-pc run dev`，端口 4599）在 Chromium 中打开
`/staff`；`/api/**` 由 Playwright 拦截返回 stub（不依赖后端）。注入 system_admin
会话（ProtectedRoute 放行），断言列表态与创建表单态文案。

## 断言与结果

| 断言 | 结果 |
|---|---|
| 列表页出现「创建员工」 | ✅ `listHasCreateStaff=true` |
| 列表页无「创建用户」 | ✅ `listHasCreateUser=false` |
| 表单卡片标题为「创建员工」 | ✅ `cardTitles=["创建员工\n返回列表"]` |
| 表单态无「创建用户」 | ✅ `formHasCreateUser=false` |

原始输出：

```json
{
  "url": "http://127.0.0.1:4599/staff",
  "checks": {
    "listHasCreateStaff": true,
    "listHasCreateUser": false,
    "buttons": ["", "退出", "邀请管理", "直属成员管理", "批量导入", "创建员工", "搜 索"],
    "formHasCreateStaff": true,
    "formHasCreateUser": false,
    "cardTitles": ["创建员工\n返回列表"]
  }
}
```

RESULT: PASS
