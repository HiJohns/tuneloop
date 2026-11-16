# #2387 R1 — 复制行为（http 非安全上下文 + https/安全上下文）

executor: dev（headless Chromium / Playwright 1.64.0，自动化浏览器实证）
commit: a4d609eab4353c7efa5b958c516d33b5f718e3c5
date: 2026-11-16

## 说明

本文件是**自动化浏览器验证回执**（非人工点击回执）：在真实 Chromium 中加载
`frontend-pc/src/utils/clipboard.js` 的原样源码，分别以非安全上下文与安全上下文
运行 `copyText(password)` 连续 3 次，断言返回成功且复制内容 == 密码。

- 非安全上下文：`http://172.31.5.179:<port>`（LAN IP，`isSecureContext=false`，
  `navigator.clipboard` 不可用）→ 必须走 `execCommand` 降级。
- 安全上下文：`http://127.0.0.1:<port>`（`isSecureContext=true`）→ 走 Clipboard API。
- 内容断言：非安全上下文拦截 `document.execCommand('copy')`，记录被选中的 textarea
  值；安全上下文用 `navigator.clipboard.readText()` 读回。

完整脚本：`tmp/verify-clipboard-2387.mjs`（仓库 tmp，按项目规范不入库）。

## 运行命令

```bash
export LD_LIBRARY_PATH=$PWD/tmp/libs/root/usr/lib/x86_64-linux-gnu
export PLAYWRIGHT_SKIP_VALIDATE_HOST_REQUIREMENTS=1
export TMPDIR=$PWD/tmp
export FONTCONFIG_FILE=$PWD/tmp/fonts.conf
node tmp/verify-clipboard-2387.mjs
```

## 结果（原始输出）

```json
{
  "password": "Pwd-2387-abc-XYZ",
  "insecure": {
    "isSecureContext": false,
    "clipboardAvailable": false,
    "results": [true, true, true],
    "execCommandValues": ["Pwd-2387-abc-XYZ", "Pwd-2387-abc-XYZ", "Pwd-2387-abc-XYZ"]
  },
  "secure": {
    "isSecureContext": true,
    "clipboardAvailable": true,
    "results": [true, true, true],
    "execCommandValues": [],
    "clipboardContent": "Pwd-2387-abc-XYZ"
  }
}
```

## 断言核对

| 断言 | 结果 |
|---|---|
| 非安全上下文 `navigator.clipboard` 不可用 | ✅ `clipboardAvailable=false` |
| 非安全上下文连续 3 次复制成功 | ✅ `results=[true,true,true]` |
| 非安全上下文复制的文本 == 密码（3 次） | ✅ `execCommandValues` 3 次均为密码 |
| 安全上下文连续 3 次复制成功 | ✅ `results=[true,true,true]` |
| 安全上下文剪贴板内容 == 密码 | ✅ `clipboardContent` == 密码 |
| 关闭前可重复复制（弹窗无置灰/无已复制缓存） | ✅ `InitialPasswordModal` 无 disabled/复制状态；每次点击调用 `copyText` |

RESULT: PASS
