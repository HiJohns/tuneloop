# #2387 R1 — 生产组件 InitialPasswordModal 的复制交互（非安全上下文 + 安全上下文）

executor: dev（headless Chromium + Vite dev：真实 `InitialPasswordModal` 组件挂载；自动化点击）
commit: a4d609eab4353c7efa5b958c516d33b5f718e3c5
date: 2026-11-16
复审说明: 按 Round 1 审计 B1 的允许路径「在页面注入 `InitialPasswordModal` 最小组件挂载」执行；
完整「创建 → 弹窗」链路当前不可达，根因是独立缺陷 **#2400**（成功后 `resetCreateState()` 立即关闭弹窗，
有 `POST /api/users` 200 + 无 Modal 的浏览器实证）。

## 方法

1. `frontend-pc/qa-modal.html`（仅本地 tmp 用途，未提交）通过 Vite dev 加载真实生产组件
   `frontend-pc/src/components/InitialPasswordModal.jsx`，`open=true`、`passwords=[{identifier:'qa2387', password:'Pwd-2387-abc-XYZ'}]`。
2. 同一 Chromium 实例分别访问：
   - 非安全上下文：`http://172.31.5.179:4599/qa-modal.html`（`isSecureContext=false`，`navigator.clipboard` 不可用）；
   - 安全上下文：`http://127.0.0.1:4599/qa-modal.html`（`isSecureContext=true`，授予 clipboard 权限）。
3. 每个上下文点击弹窗内「复制」按钮 **3 次**，记录：按钮 `disabled`、每次出现的 `antd message`、
   提交给 `execCommand('copy')` 的 textarea 值（非安全上下文）/ `navigator.clipboard.readText()`（安全上下文）。
4. 回退项：把两条复制路径都置为失败后再点一次，断言 `window.getSelection()` 选中密码文本并提示手动复制。

## 红证（修复前实现，同环境）

旧实现仅 `navigator.clipboard.writeText`，非安全上下文 `navigator.clipboard` 不存在：

```json
{
  "isSecureContext": false,
  "clipboardAvailable": false,
  "results": ["fail:TypeError", "fail:TypeError", "fail:TypeError"]
}
```

## 绿证（生产组件真实交互）

```json
{
  "insecure": {
    "isSecureContext": false,
    "clipboardAvailable": false,
    "clicks": [
      { "disabled": false, "lastNotice": "已复制" },
      { "disabled": false, "lastNotice": "已复制" },
      { "disabled": false, "lastNotice": "已复制" }
    ],
    "copyValues": ["Pwd-2387-abc-XYZ", "Pwd-2387-abc-XYZ", "Pwd-2387-abc-XYZ"],
    "fallbackSelection": "Pwd-2387-abc-XYZ",
    "fallbackNotice": "复制失败，已选中密码文本，请按 Ctrl+C 手动复制"
  },
  "secure": {
    "isSecureContext": true,
    "clipboardAvailable": true,
    "clicks": [
      { "disabled": false, "lastNotice": "已复制" },
      { "disabled": false, "lastNotice": "已复制" },
      { "disabled": false, "lastNotice": "已复制" }
    ],
    "readText": "Pwd-2387-abc-XYZ",
    "fallbackSelection": "Pwd-2387-abc-XYZ",
    "fallbackNotice": "复制失败，已选中密码文本，请按 Ctrl+C 手动复制"
  }
}
```

## 断言核对

| 断言 | 非安全上下文 | 安全上下文 |
|---|---|---|
| 弹窗（生产组件）真实渲染 | ✅ | ✅ |
| 连续 3 次点击均出现「已复制」 | ✅ 3/3 | ✅ 3/3 |
| 复制内容 == 密码 | ✅ textarea 值 3/3 == 密码 | ✅ `readText()` == 密码 |
| 按钮关闭前始终可点（`disabled=false`） | ✅ 3/3 | ✅ 3/3 |
| 两条路径都失败时自动选中文本 | ✅ 选中 == 密码 + 提示手动复制 | ✅ 选中 == 密码 + 提示手动复制 |

RESULT: PASS
